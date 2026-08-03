# Programming Bulk Updates

Bulk updates must behave like one logical mutation from the UI's point of view.
This note documents the IDS bulk-status bug and the rules we must follow to avoid
repeating it.

## What Went Wrong

The IDS column failed to update reliably because several layers treated one bulk
operation as multiple independent mutations:

- The TUI changed from per-patent `IDSEntryChangedMsg` to a batch
  `IDSEntriesChangedMsg`, but not every visible pane handled the batch message.
- Reintroducing per-patent fan-out caused conflicting multiple pane `Update()`
  calls for one user action.
- `Engine.BulkSetIDSStatus` called `SaveIDSEntry` for each patent, and
  `SaveIDSEntry` emitted `db.changed` each time. That let the UI refresh during
  a partially completed batch.
- The IDS status picker cleared visual selection by calling
  `SaveVisualSelection()` before the user picked a status, so repeated bulk
  applies fell back to a single cursor row.
- `store.Cache` did not flush patent-list rows on `SaveIDSEntry` or
  `DeleteIDSEntry`, so a refresh could reload stale `PatentRow.IDSEntry` values
  and overwrite the correct optimistic pane update.

## Rules

1. Use One Message Per Logical Bulk Operation

   A bulk mutation should emit one domain message containing all changed rows,
   for example `IDSEntriesChangedMsg`. Do not fan out the batch into N single-row
   messages unless there is a specific compatibility requirement and tests prove
   no pane sees conflicting state.

2. Every Pane That Renders The Data Must Handle The Batch Message

   If a pane renders a field affected by a bulk update, add explicit handling for
   the batch message in that pane's `Update()` method. For IDS, catalog,
   citations, detail, and IDS detail must update from `IDSEntriesChangedMsg` as
   applicable.

3. The Engine Should Announce Once Per Bulk Operation

   Shared single-row helpers may need an internal `announce bool` or equivalent.
   Bulk methods should suppress per-item `db.changed` events and announce once
   after the batch completes. If a partial failure occurs after some rows were
   saved, announce once before returning the error.

4. Cache Invalidation Is Part Of The Mutation

   Any write that changes data included in `PatentRow` must flush the patent-list
   cache. This includes project-scoped fields such as review state, tags, IDS
   entries, and notes if they become row-visible later.

5. Do Not Consume Selection Before Confirmation/Input Completes

   Opening a picker or confirmation overlay must not clear visual selection.
   Capture a copy of the selected patent numbers for the overlay, but leave the
   pane selection state intact unless the final action explicitly needs to clear
   it.

6. Copy Slices Stored In Overlay State

   Overlay constructors and outgoing messages should copy selected patent slices:

   ```go
   patents: append([]domain.PatentNumber(nil), patents...)
   ```

   This prevents later pane state changes from mutating the operation target.

## Required Tests For Future Bulk Updates

Add tests at the layer where the bug can happen:

- App routing: one bulk result broadcasts one batch message, not N conflicting
  single-row messages.
- Pane update: one batch `Update()` changes multiple visible rows.
- Engine events: one bulk method returns all changed entries and emits one
  `db.changed` for the logical operation.
- Cache: list rows reflect first and subsequent writes, including delete/clear.
- UI state: opening the picker/overlay twice preserves the same multi-selection
  behavior.

## IDS-Specific Checklist

When changing IDS bulk behavior, verify all of these:

- `BulkSetIDSStatusCmd` returns one `IDSEntriesChangedMsg` with every saved
  entry.
- `App.Update(IDSEntriesChangedMsg)` broadcasts only the batch message.
- `Catalog.Update(IDSEntriesChangedMsg)` applies all matching entries in one
  pass and reloads only when relevant entries are off-page.
- `Citations`, `Detail`, and `IDSDetail` do not rely on per-entry fan-out.
- `Engine.BulkSetIDSStatus` suppresses per-entry `db.changed` from
  `SaveIDSEntry` and announces once after the batch.
- `store.Cache.SaveIDSEntry` and `store.Cache.DeleteIDSEntry` flush listings.
- Opening `IDSStatusPicker` does not clear visual selection.
