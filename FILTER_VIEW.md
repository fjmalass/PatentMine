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
