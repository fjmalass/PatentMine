# PatentMine REST API

The HTTP server (`internal/api`) is a **thin client**: it owns no database and no
business logic. Every route translates an HTTP request into a single `proto` call
to the daemon — exactly as the TUI does. Both frontends are built from the same
command catalog (`internal/command/catalog.go`), so an action is defined once and
every frontend stays in step.

- **Base server:** `internal/api/server.go` (`NewServer` → `routes()`)
- **Route table:** `internal/api/routes.go`
- **Command catalog (source of truth):** `internal/command/catalog.go`
- **Proto method names:** `internal/proto/proto.go`

## Conventions

- All responses are JSON unless noted (`text/markdown`, `text/plain` for metrics/notes).
- Errors return `{"error": "..."}` with an HTTP status mapped from the proto error
  code: `NotFound/NoMethod → 404`, `BadParams/Parse/InvalidReq → 400`,
  `Busy → 503`, otherwise `500`.
- Per-call timeout is 20s (`apiCallTimeout`); notes export uses 30s.
- `{number}` is a patent number parsed by `domain.ParsePatentNumber`; `{id}` is a
  project ID.
- List endpoints page with `?limit=&offset=` and accept filter/sort query params
  (see `handlePatentList` / `handleRelations`).

---

## Endpoint reference

### Health & introspection

| Method | Path | Proto method | Notes |
|---|---|---|---|
| GET | `/healthz` | `ping` | Daemon liveness ping. |
| GET | `/commands` | — | Returns the full command registry (`registry.All()`). |
| GET | `/metrics` | `metrics.get` | Prometheus exposition format (`text/plain`). |
| GET | `/metricsz` | `metrics.get` | In-memory timing/counter snapshot as JSON. |
| POST | `/metrics` | `metrics.push` | Merge a client-side snapshot. Body: `component, snapshot`. |

### Activity & history (served from the local JSONL journal, not proto)

| Method | Path | Notes |
|---|---|---|
| GET | `/activity` | Alias of `/activity/raw`. |
| GET | `/activity/raw` | Raw activity records. Query: `limit, component, action, entity, since`. |
| POST | `/activity` | Record a UI/browser activity event. Body: `action, entity, entity_id, status, duration_ms, before, after, attributes`. |
| GET | `/activity/replay_history` | Capped log of rows opened from replay UI. Query: `limit`. |
| GET | `/history` | Grouped replay/history feed. Query: `raw_limit` (or `limit`), `component`, `since`. |

### Patents

| Method | Path | Proto method | Notes |
|---|---|---|---|
| GET | `/patents` | `patent.list` | Query: `project, filter, review_state\|state, ids_status, search, classification\|class, classification_code, inventor, assignee\|owner, limit, offset, sort_column, sort_ascending`. |
| GET | `/patents/orphans` | `patent.orphan_list` | Patents with no project membership. Query: `limit, offset`. |
| GET | `/patents/columns` | `patent.table_columns` | Column schema. Query: `project`. |
| GET | `/patents/{number}` | `patent.get` | One patent. |
| GET | `/patents/{number}/relations` | `patent.relations` | Family-graph edges of one `kind` (default `cites`). Same filter/sort params as `/patents`. |
| GET | `/patents/{number}/family` | `patent.family_graph` | Bounded parent/child DAG. Query: `depth, max_nodes\|limit, country` (repeatable). |
| PUT | `/patents/{number}/review_state` | `review_state.set` | Body: `project, state`. |
| GET | `/patents/{number}/diffs` | `source.diffs.list` | Source-comparison discrepancies. |
| POST | `/patents/{number}/diffs/resolve` | `source.resolve_diffs` | Body: `diffs[]`. |
| GET | `/patents/{number}/inventors` | `patent.inventor_stats` | Inventor stats for one patent. Query: `project`. |
| GET | `/patents/{number}/bibs` | `source.bibs.list` | Each source's full bibliographic snapshot. |
| GET | `/patents/{number}/expiration` | `uspto.expiration.calculate` | Compute (and persist) statutory U.S. expiration. Query: `project, refresh`. |
| DELETE | `/patents` | `patent.delete_bulk` | Permanently remove patents. Body: `patents[]`. |

### USPTO data (per patent)

| Method | Path | Proto method | Notes |
|---|---|---|---|
| POST | `/patents/{number}/uspto/fetch` | `uspto.fetch_xml` | Download grant/pgpub XML. Query: `kind=grant\|pgpub` (default grant). |
| GET | `/patents/{number}/uspto/grant_body` | `uspto.grant_body` | Parsed grant/pgpub body. Query: `kind` (default grant, daemon falls back to pgpub). |
| POST | `/patents/{number}/uspto/assignments` | `uspto.fetch_assignments` | Fetch & store assignment chain. |
| GET | `/patents/{number}/uspto/assignments` | `uspto.assignment.list` | Persisted assignment chain. |

### Stats

| Method | Path | Proto method | Notes |
|---|---|---|---|
| GET | `/assignees` | `patent.assignee_stats` | Query: `project`. |
| GET | `/assignees/stats` | `patent.assignee_stats` | Alias of `/assignees`. |
| GET | `/classifications/stats` | `patent.classification_stats` | Query: `project`. |

### Projects

| Method | Path | Proto method | Notes |
|---|---|---|---|
| GET | `/projects` | `project.list` | |
| POST | `/projects` | `project.create` | Body: `name`. |
| PUT | `/projects/{id}` | `project.update` | Body: a `domain.Project`. |
| POST | `/projects/{id}/patents` | `membership.add` | Body: `patent, source, confirm_merge`. |
| GET | `/projects/{id}/ids` | `ids.export` | Build the IDS for a project. |
| POST | `/projects/{id}/ids/pdf` | `ids.export.pdf` | Render PTO/SB/08a+08c. Optional body overrides (`cumulative_count, fee_amount, deposit_account, signer_*`). |
| POST | `/projects/{id}/ids/pdf/preview` | `ids.export.pdf.preview` | Dry-run summary (counts, fee tier, missing fields). Same optional body as the PDF export. |
| POST | `/projects/{id}/ids/status` | `ids.entry.bulk_set_status` | Apply one status to many patents. Body: `patents[], status, default_in_full`. |
| GET | `/projects/{id}/ids/entries/{number}` | `ids.entry.get` | One curated IDS entry. |
| PUT | `/projects/{id}/ids/entries/{number}` | `ids.entry.save` | Insert/update an IDS entry. Body: a `domain.IDSEntry` (project & patent taken from path). |
| DELETE | `/projects/{id}/ids/entries/{number}` | `ids.entry.delete` | Remove an IDS entry. |
| GET | `/projects/{id}/notes` | `patent.note.list` | All notes for a project. Query: `sort_by=date\|patent`. |
| GET | `/projects/{id}/notes/export` | `patent.note.export` | `text/markdown`. Query: `sort_by=date\|patent`. |
| GET | `/projects/{id}/patents/{number}/note` | `patent.note.get` | One patent note. |
| PUT | `/projects/{id}/patents/{number}/note` | `patent.note.save` | Insert/update a note. Body: a `domain.PatentNote` (project & patent taken from path). |
| DELETE | `/projects/{id}/patents/{number}/note` | `patent.note.delete` | Remove a note. |

### Tags

| Method | Path | Proto method | Notes |
|---|---|---|---|
| POST | `/projects/{id}/tags` | `tag.create` | Body: `name`. |
| GET | `/projects/{id}/tags` | `tag.list` | |
| DELETE | `/projects/{id}/tags/{name}` | `tag.delete` | |
| POST | `/projects/{id}/patents/{number}/tags` | `patent.tag.add` (strict) | Body: `name`. |
| DELETE | `/projects/{id}/patents/{number}/tags/{name}` | `patent.tag.delete` (strict) | |
| GET | `/projects/{id}/patents/{number}/tags` | `patent.tag.list` | |

### Classifications

| Method | Path | Proto method | Notes |
|---|---|---|---|
| GET | `/classifications` | `classification.list` | |
| GET | `/classifications/{system}/{code}` | `classification.get` | |
| POST | `/classifications` | `classification.save` | Body: `classification`. |
| DELETE | `/classifications/{system}/{code}` | `classification.delete` | |
| POST | `/classifications/lookup` | `classification.lookup` | Body: `code`. |
| POST | `/classifications/by_codes` | `classification.list_by_codes` | Body: `codes[]`. |
| GET | `/projects/{id}/patents/{number}/classifications` | `patent.classification.list` | |

### Crawl & source mode

| Method | Path | Proto method | Notes |
|---|---|---|---|
| POST | `/crawl` | `crawl.family` | Body: `root, depth, force`. |
| POST | `/import` | `import.file` | Load a patent from a local fixture file. Body: `path`. |
| GET | `/crawl/config` | `crawl.config` | Daemon crawl defaults. |
| GET | `/source_mode` | `source_mode.get` | |
| PUT | `/source_mode` | `source_mode.set` | Body: `proto.SourceModeParams`. |
| GET | `/config/source_mode` | `source_mode.get` | Alias. |
| PUT | `/config/source_mode` | `source_mode.set` | Alias. |

### Table views (persisted UI layouts)

| Method | Path | Proto method | Notes |
|---|---|---|---|
| GET | `/table_views` | `table_view.list` | Query: `owner, table_type`. |
| GET | `/table_views/{id}` | `table_view.get` | Query: `owner`. |
| POST | `/table_views` | `table_view.save` | Body: a `domain.SavedTableView`. |
| DELETE | `/table_views/{id}` | `table_view.delete` | Query: `owner`. |

---

## Consistency with the TUI

The check below is grounded in `internal/command/catalog.go`. `KindView` commands
(navigation, opening panes, visual selection, jump mode, etc.) are intentionally
**client-local** and have **no** REST equivalent — that is correct, not a gap.
The interesting question is whether every `KindEngine` command (a real daemon
operation) is reachable over REST.

### ✅ Engine commands fully covered by REST

`patent.list`, `patent.get`, `patent.relations`, `patent.family_graph`,
`project.list`, `project.create`, `membership.add` (add / add.uspto / add.google),
`review_state.set` (active / under-review / ignored / deleted), `ids.export`,
`ids.export.pdf`, `crawl.family` (crawl.all / citations / citedby / lookup / import),
`tag.create`, `tag.list`, `tag.delete`, `patent.tag.add` (strict),
`patent.tag.delete` (strict), `patent.tag.list`, `classification.list`,
`classification.lookup`.

### ✅ TUI-direct proto methods now exposed over REST

These back data loads and in-pane editors the TUI invokes outside the command
dispatch. They were previously TUI-only; REST now mirrors them:

`ids.entry.get`, `ids.entry.save`, `ids.entry.delete`, `ids.entry.bulk_set_status`,
`ids.export.pdf.preview`, `patent.note.get`, `patent.note.save`,
`patent.note.delete`, `patent.note.list`, `patent.inventor_stats`,
`source.bibs.list`, `uspto.fetch_xml`, `uspto.grant_body`,
`uspto.fetch_assignments`, `uspto.assignment.list`, `uspto.expiration.calculate`,
`import.file`, `patent.delete_bulk`, `metrics.push`.

### ❌ Engine commands the TUI exposes but REST still does NOT

| Catalog command(s) | Proto method | Missing REST route (suggested) |
|---|---|---|
| `add.related` | `membership.add_related` | `POST /projects/{id}/patents/related` |
| `delete` (single) | `patent.delete` | `DELETE /patents/{number}` (bulk `DELETE /patents` exists) |
| `patent.clear-cache` | `patent.clear_cache` | `POST /patents/{number}/clear_cache` |
| `tag` (loose) | `tag.assign` | `POST /patents/{number}/tags` (project-less) |
| `untag` (loose) | `tag.remove` | `DELETE /patents/{number}/tags/{name}` (project-less) |
| `crawl.cancel` | `crawl.cancel` | `POST /crawl/cancel` |

> Note: the project-scoped (**strict**) tag/untag endpoints exist; only the
> loose, project-less `tag.assign` / `tag.remove` variants are unexposed.

### ℹ️ Defined proto methods still used by neither frontend directly

`uspto.lookup` is declared in `proto.go` but not referenced by the TUI command
dispatch or REST. (`crawl.cancel` is wired as a catalog command and dispatched
generically.)

---

## How to keep this consistent

Because every route in `routes.go` is meant to mirror a catalog command, the
cleanest enforcement would be a startup/test assertion that every
`command.KindEngine` entry in `command.Default()` has a corresponding route. The
gaps in the ❌ table above would be the first failures to address.
