# TODO

Deferred / backlog items. Newest on top.

## Assignee / assignment history

**Done (foundation, schema v6):** `assignee_history` table keyed by `record.id`;
`domain.NormalizeAssignee` + history/group/rollup types; `RebuildAssigneeHistory`
(at-grant owner + recorded chain, `is_latest` = current owner(s)); `AssigneeHistory`
and `ProjectAssignees` reads; engine rebuild hooked into `FetchUSPTOAssignments`;
unit tests. See in-code `TODO(assignee-…)` markers.

**Remaining wiring** (see `TODO(assignee-engine)` in `internal/engine/engine_uspto.go`
and `TODO(assignee-commands)` in `internal/command/catalog.go`):
- Engine: `AssigneeHistory`, `ProjectAssignees`, `FetchUSPTOAssignmentsBatch`
  (skip-expired), `SearchAssigneeAssignments`.
- Crawl: `SearchUSPTOAssignmentsByAssignee` (ODP `assigneeBag.name` query).
- proto + rpc + REST methods for the four engine entry points.
- TUI: enrich `cmdOpenAssignees` to render the per-patent timeline; add
  `export.assignees`, `find.assignee`, `fetch.uspto.assignments.file`.
  (Shipped: `open.assignees.project` rollup view, `fetch.uspto.assignments.project`
  batch.)
- `TODO(assignee-atgrant)`: take joint at-grant owners from `uspto_grant_party`
  (role=assignee) instead of only `record.assignee`.
- `TODO(assignee-backfill)`: one-time backfill of `assignee_history` from existing
  `uspto_assignment` rows so old corpora populate without a re-fetch.

- **Manual assignee aliases.** The assignee history feature (see
  `ASSIGNEE_HISTORY_PLAN.md`) dedups on the auto `assignee_norm` key only and
  accepts some name fragmentation. Add an `assignee_alias` mapping + curation so
  variants like `Apple Inc` / `Apple Computer, Inc.` can be merged manually.
  Revisit only if real reports show too much splitting. Mirrors the existing
  source-reconciliation pattern (`SourceDiff` / `ReconciliableFields`).
