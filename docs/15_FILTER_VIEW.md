# Saved Table Views

Saved table views are the persistence model for reusable filtering, search, sorting, and table display preferences.

The feature is intentionally generic. IDS activity history is the first target, but the same model applies to patents, citations, inventor stats, and future tables.

## Storage Model

Saved views are stored in SQLite in `saved_table_view`.

Fields:

* `id` : Stable saved-view id.
* `owner` : User/account owner. Empty API input defaults to `local` until authentication exists.
* `table_type` : Table the view belongs to.
* `name` : User-facing saved-view name.
* `scope` : Visibility scope. Current default is `user`; `system` and `team` are reserved for built-in or shared views.
* `is_default` : Whether this is the owner/table/scope default view.
* `view_json` : Structured table state.
* `created_at` / `updated_at` : Audit timestamps.

Only one default view is allowed per `owner`, `table_type`, and `scope`. Saving a new default clears the previous default in that group.

## Supported Tables

Initial `table_type` values:

* `ids_activity_history`
* `patents`
* `citations`
* `inventor_stats`

Each table should validate its own `view_json` fields before applying a saved view. The generic persistence layer only validates that the table type is known and that `view_json` is a valid JSON object.

## View JSON Shape

Recommended shape:

```json
{
  "search": "office action",
  "filters": {
    "status": ["failed", "needs_attention"],
    "activityType": ["ids_submitted"]
  },
  "sort": [
    { "field": "activityDate", "direction": "desc" }
  ],
  "columns": {
    "visible": ["activityDate", "activityType", "user", "status"],
    "order": ["activityDate", "activityType", "user", "status"]
  }
}
```

Sorting is part of the saved view rather than a separate preference because views like `Expiring Soon`, `Newest Citations`, or `Top Inventors` depend on both filters and sort order.

## API

List views:

```http
GET /table_views?table_type=ids_activity_history&owner=local
```

Create or update a view:

```http
POST /table_views
Content-Type: application/json

{
  "table_type": "ids_activity_history",
  "name": "Needs Attention",
  "is_default": true,
  "view_json": {
    "search": "failed",
    "filters": {
      "status": ["failed", "needs_attention"]
    },
    "sort": [
      { "field": "activityDate", "direction": "desc" }
    ]
  }
}
```

Fetch one view:

```http
GET /table_views/{id}?owner=local
```

Delete one view:

```http
DELETE /table_views/{id}?owner=local
```

RPC methods mirror the HTTP endpoints:

* `table_view.list`
* `table_view.get`
* `table_view.save`
* `table_view.delete`

## Interface Options

Recommended first interface:

* Default chips or tabs above the table: `All`, `Recent`, `My Activity`, `Needs Attention`, `Submitted`.
* Free-form search near the top of the table.
* `Saved views` dropdown beside the search box.
* `Save current view` action in the saved-views dropdown or future filter panel.

Pros:

* Common defaults stay visible and fast.
* User-created views have a predictable home.
* Search, filters, sorting, and columns can be restored together.
* The same pattern scales to patents, citations, and stats tables.

Cons:

* The UI needs clear active-state styling when a saved view is modified.
* A long saved-view list may eventually require a manage dialog.
* Table-specific validation is still required before applying saved JSON.

## Default View Approaches

### All Activity

Pros:

* Least surprising for audit/history pages.
* No records are hidden by default.
* Best legal/compliance posture.

Cons:

* Can be noisy.
* Users may need to filter frequently.

### Recent Activity

Pros:

* Easier to scan.
* Better fit for operational daily use.
* Can improve perceived performance for large histories.

Cons:

* Hides older activity unless clearly labeled.
* Riskier for audit workflows.

### My Activity

Pros:

* Useful for individual task tracking.
* Reduces team noise.

Cons:

* Bad default for supervisors or matter reviewers.
* Hides team activity.

### Needs Attention

Pros:

* Highly actionable.
* Surfaces failed, rejected, incomplete, or unresolved events.

Cons:

* Not a true history view.
* Depends on reliable status classification.

## Current Recommendation

Use `All` as the initial built-in default for IDS activity history because it is least surprising and safest for audit workflows.

Expose `Recent`, `My Activity`, `Needs Attention`, and `Submitted` as prominent built-in views or chips. Store user-created named views in `saved_table_view`, and treat built-ins as code/config until there is a reason to let users edit or share them.

## Telemetry And Metrics

Filter and saved-view usage is tracked in two places:

* Metrics counters/gauges for aggregate usage and complexity.
* Activity JSONL records for durable, queryable usage events.

The API records table-query telemetry for patent lists, citation/relation lists, and history queries. Complexity is currently computed as:

```text
search term count + active filter count + sort field count + column preference count
```

Saved views use the same complexity model by reading `view_json.search`, `view_json.filters`, `view_json.sort`, and `view_json.columns`.

Aggregate metric examples:

* `api.table_filter.patents.query_total`
* `api.table_filter.patents.complexity_total`
* `api.table_filter.patents.last_complexity`
* `api.table_filter.patents.search_used_total`
* `api.table_filter.patents.filter_field.ids_status.total`
* `api.table_filter.patents.sort_field.expires.total`
* `api.table_view.ids_activity_history.select_total`
* `api.table_view.ids_activity_history.save_total`
* `api.table_view.ids_activity_history.delete_total`
* `api.table_view.ids_activity_history.view.view_123.select_total`

Activity actions:

* `table_filter.apply` : Records table type, query path, search term count, filter count, sort count, column count, active fields, and complexity.
* `table_view.select` : Records which saved view was fetched for use.
* `table_view.save` : Records saved-view creation or update, including complexity.
* `table_view.delete` : Records saved-view removal.

These activity records are intentionally not part of the high-signal history feed. They remain available through `/activity/raw` for product analysis, while aggregate metrics are available through `/metricsz` and `/metrics`.

## TUI Activity History

Press `H` in the TUI to open Activity History.

Inside the history overlay:

* `/` : Enter a free-form filter. The filter matches rendered details, action, entity, status, project name, metadata, and timestamp text in the currently loaded history window.
* `.` : Toggle sort order between newest-first and oldest-first.
* `c` : Clear the local history filter and restore newest-first sorting.
* `Enter` : Replay the selected history entry.
* `q` / `Esc` : Close the overlay.

This is currently a local overlay filter over the loaded history window, not a backend saved view application. Applying, clearing, or toggling sort emits `table_filter.apply` activity telemetry with `source=tui.history_overlay`, `table_type=ids_activity_history`, search term count, sort direction, result count, and total loaded count.

### Filtering by action type

The overlay `/` filter is a substring match over the rendered row (action name, entity, status, project, details, timestamp, metadata JSON). To narrow to a specific action type, type the action constant (or any unique substring) into the `/` prompt.

**Keystrokes:**

1. `H` — open Activity History overlay.
2. `/` — enter filter mode (cursor moves to filter prompt at top).
3. Type the action substring (see table below).
4. `Enter` — apply. Status line shows `Filter: "<query>"`.
5. `c` — clear filter and restore default view.

**Action substrings (match against `rec.Action`):**

| Goal | Type after `/` |
|---|---|
| Only tag assignments | `patent.tag_assign` |
| Only tag removals | `patent.tag_remove` |
| Any tag activity | `tag` |
| Membership state changes | `membership.set_state` |
| IDS saves only | `ids.entry.save` |
| IDS deletes only | `ids.entry.delete` |
| Any IDS mutation | `ids.entry` |
| Filter applications | `filter.apply` |
| Project switches | `project.switch` |
| Notes (save/delete/export) | `notes.` |
| Patent focus events | `ui.focus` |

Examples:

* `/patent.tag_assign` → only `Tag "<name>": <patent>` rows.
* `/ids.entry` → both `IDS save` and `IDS delete` rows.
* `/Switch Project` → matches the rendered details column for project switches (also works because `historyRecordMatches` searches rendered text).

**Unified `:filter` across all views.**

`:filter <expr>` is the one filter command across every list-like surface — catalog, citations, family, history overlay, stats overlays. The expression is stored opaquely on a shared `FilterSortSettings`; each surface decides how to interpret it (server query vs. local substring match).

Keystroke contract (same in every view):

| Keystroke | Effect |
|---|---|
| `:filter <expr>` | Set filter expression. |
| `:filter` or `:filter.clear` | Clear filter. |
| `/` | Inline filter prompt (legacy shortcut, same effect as `:filter`). |
| `.` | Toggle sort direction (or cycle column where applicable). |
| `c` | Clear filter + restore default sort. |

History overlay-specific interpretation: the expression runs as a case-insensitive substring match over the rendered row (action, entity, status, project, details, timestamp, metadata JSON). Use any action substring to narrow:

| Goal | Type |
|---|---|
| Tag assignments only | `:filter patent.tag_assign` |
| Any tag activity | `:filter tag` |
| IDS save/delete | `:filter ids.entry` |
| Filter applications | `:filter filter.apply` |
| Project switches | `:filter project.switch` |
| Notes (save/delete/export) | `:filter notes.` |
| Patent focus events | `:filter ui.focus` |

Catalog/citations/family interpret the same expression as the existing `filterexpr` grammar (e.g. `assignee:foo AND year:2024`, or `provenance:system` for project listing). The expression is opaque to `FilterSortSettings`; each pane's match callback parses or substring-matches as it sees fit.

## Generic FilterSortSettings

To avoid reinventing filter/sort state per pane and overlay, all list-like surfaces share a single `FilterSortSettings` container plus one dispatcher for the `:filter` command. The name is intentionally explicit — it holds the filter expression *and* the sort direction *and* the inline-prompt sub-state for any table-like view. No buzzword shorthand.

### Why

Today filter/sort logic is duplicated across at least eight files:

* `pane/catalog.go`, `pane/citations.go`, `pane/family_graph.go` — own `applyFilter` + `PatentFilter`.
* `overlay/history.go` — own `filterMode`, `filterInput`, `filterQuery`, `sortAscending`, `handleFilterKey`.
* `overlay/all_assignees_history.go`, `overlay/classification_stats.go`, `overlay/inventor_stats.go`, `overlay/patent_classification_view.go` — own `sortAscending` toggles.

Each rewrites prompt handling, status bar rendering, help hints, and telemetry emission. The refactor collapses all of this into one container.

### Package layout

`internal/tui/listui/filter_sort_settings.go` (new, no app deps):

```go
package listui

// FilterSortSettings is the shared filter + sort + inline-prompt container
// for any list-like surface (pane or overlay). Query is opaque: each surface's
// match callback decides how to interpret it (filterexpr grammar, plain
// substring, or anything else).
type FilterSortSettings struct {
    Table       string // "history", "catalog_patents", "all_assignees_history", ...
    Query       string
    SortColumn  string
    SortAsc     bool
    PromptOpen  bool
    PromptBuf   string
}

func (s *FilterSortSettings) HandleKey(msg tea.KeyMsg) (changed, handled bool)
func (s *FilterSortSettings) BarView(w int, theme render.Theme) string
func (s *FilterSortSettings) HelpHints() []string
func (s *FilterSortSettings) IsActive() bool
func (s *FilterSortSettings) SetQuery(q string)
func (s *FilterSortSettings) Clear()
```

Generic helper for callers that hold a typed slice:

```go
func Apply[T any](src []T, s FilterSortSettings, match func(T, string) bool, less func(T, T) bool) []T
```

`internal/tui/listui/memory.go` (per-session persistence):

```go
type FilterSortMemory struct{ byKey map[string]*FilterSortSettings }
func (m *FilterSortMemory) Get(table, owner string) *FilterSortSettings
func (m *FilterSortMemory) Save(s *FilterSortSettings, owner string)
```

`App` holds one `FilterSortMemory`. Reopening any surface restores its filter and sort within the session. Cross-session persistence (serialize to `saved_table_view` SQLite) is a follow-up — see Storage Model section above.

### Opt-in interfaces

```go
type FilterSortHolder    interface { FilterSort() *listui.FilterSortSettings }
type FilterSortRefresher interface { ReapplyFilterSort() tea.Cmd }
```

Any pane or overlay that wants `:filter` implements both — roughly ten lines of glue per surface.

### Single dispatcher

`internal/tui/app_actions.go::invoke` handles `command.Filter` once for every surface:

```go
case command.Filter:
    target := a.focusedFilterSortHolder()
    if target == nil {
        return a.usageError(command.Filter)
    }
    target.FilterSort().SetQuery(strings.Join(inv.args, " "))
    a.filterSortMemory.Save(target.FilterSort(), a.owner())
    return a, tea.Batch(target.ReapplyFilterSort(), a.emitTableFilterApply(target.FilterSort()))
```

Replaces:

* `Catalog.applyFilter` (`pane/catalog.go:261`).
* `Citations.applyFilter` (`pane/citations.go:197`).
* `FamilyGraph.applyFilter` (`pane/family_graph.go:163`).
* History overlay's `/`/`c`/`.` keymap + `handleFilterKey` (`overlay/history.go:58-232`).

### Scope

Add `ScopeFilterable` to `internal/command/catalog.go`. Replace `Filter`'s per-surface scope list with `ScopeFilterable`. Any `FilterSortHolder` reports `ScopeFilterable` via the existing `ScopeSource` hook on overlays / `Scope()` on panes.

### Telemetry

Single emitter: `App.emitTableFilterApply(*listui.FilterSortSettings)` writes `ActionTableFilterApply` (already defined) with:

* `table_type`
* `query`
* `sort_column`
* `sort_asc`
* `result_count`
* `source` (e.g. `tui.history_overlay`, `tui.catalog_pane`)

Removes `HistoryFilterAppliedMsg` and its handler in `app.go:526`. Removes per-pane telemetry calls in `applyFilter` methods.

### Per-surface responsibilities (kept, not abstracted)

Only three things stay surface-specific:

1. **Match predicate** — `func(record, query) bool`. Record types differ, so the predicate cannot be generic.
2. **Sort comparator** — column semantics differ.
3. **Column metadata** — header labels, widths.

Everything else (state, key handling, prompt, status bar, help hints, telemetry, memory) is shared.

### Opaque Query — design decision

`FilterSortSettings.Query` is opaque. `FilterSortSettings` never parses it. This avoids forcing every surface to learn `filterexpr` grammar when a plain substring is enough (e.g. history overlay). Each surface's match callback decides:

* Catalog / citations / family: `filterexpr.Parse(query)` then evaluate.
* History overlay: `strings.Contains(strings.ToLower(rendered), strings.ToLower(query))`.
* Stats overlays: future — likely substring on the row's primary column.

Trade-off: a power user typing `assignee:foo` in the history overlay gets a substring match, not a grammar match. Acceptable for now. If grammar support is wanted in history later, the match callback opts in — `FilterSortSettings` does not change.

### Migration order

Low blast radius, one surface at a time:

1. Add `internal/tui/listui/{filter_sort_settings,memory}.go` + tests. Zero callers yet.
2. Migrate **History overlay** first (smallest, isolated). Delete `filterMode` / `filterInput` / `filterQuery` / `sortAscending` and `handleFilterKey`. Add `FilterSort()` + `ReapplyFilterSort()`. Verify `H` → `:filter` and `H` → `/` still work.
3. Migrate **stats overlays** (assignee, classification, inventor, patent classification) — sort-only today, gain `:filter` for free.
4. Migrate **panes** (catalog, citations, family). Keep `PatentFilter` as the predicate engine, but route `Query` storage and telemetry through `FilterSortSettings`. `PatentFilter` becomes a parser/predicate, not a state owner.
5. Add `ScopeFilterable` in `command/catalog.go`. Replace `Filter`'s scope list.
6. Delete `HistoryFilterAppliedMsg`, the handler in `app.go:526`, and the three `applyFilter` methods.

### Wiring + tests

Per the wiring-invariants convention:

* `validateWiring`: assert `Filter` reachable in `ScopeFilterable`.
* `TestHintCoverage`: each `FilterSortHolder` surface advertises `:filter` and `:filter.clear` in its hint set.
* `listui/filter_sort_settings_test.go`: key handling (`/`, `.`, `c`, prompt edits), `BarView` snapshot, `Apply` with synthetic record type.
* `listui/memory_test.go`: save/restore per `(table, owner)`.
* Per-surface migration test: assert old keystrokes still produce the same visible rows.
