# PatentMine: Database Implementation

How PatentMine persists everything: a single **SQLite** file driven through a
pure-Go driver, an embedded versioned schema, and a deliberate
**pointer-in-row / bytes-on-disk** split that keeps the database small and
backups fast.

This is the storage companion to the architecture manual — see
[README §2 (data model)](./README.md#2-the-data-model--value-type-system) and
[README §3–§4 (lifecycle, deletion)](./README.md#3-the-patent-lifecycle) for the
identity/indirection concepts this doc grounds in the actual tables.

Related docs:

- [README.md](./README.md) — architecture, lifecycle, and indirection model.
- [ACTIVITY.md](./ACTIVITY.md) — the JSONL activity journal used for delete backup/replay.
- [USPTO_CONFIG_LOADING.md](./USPTO_CONFIG_LOADING.md) — how the USPTO source tables get filled.
- [FILTER_VIEW.md](./FILTER_VIEW.md) — the saved-table-view storage model.

> [!NOTE]
> Everything here lives under [`internal/store`](./internal/store): the
> backend-agnostic `store.Repository` interface (`repository.go`) and its only
> implementation, [`internal/store/sqlite`](./internal/store/sqlite). The engine
> talks to the interface, never to SQLite directly, so the persistence layer is
> swappable in principle.

---

## 1. Engine, driver, and file

- **Driver:** [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) — a
  **pure-Go, CGo-free** SQLite. The build needs no C toolchain and cross-compiles
  cleanly. It is registered as the `sqlite` `database/sql` driver; `Open` asserts
  the driver is present and fails loudly if the import was dropped.
- **One file:** the whole store is a single SQLite database, by default
  `patentmine.db` in the PatentMine home directory (`config.DBPath`). Run
  `patentmine paths` to print the resolved location. WAL mode adds the usual
  `-wal` / `-shm` sidecar files at runtime.
- **Backup:** `patentmine db backup [--out=<path>] [--rsync=<dest>]` makes a
  consistent hot copy of the file (the rsync target also defaults from
  `PATENTMINE_DB_BACKUP_RSYNC`); because bytes live on disk and only pointers in
  rows (see §5), the file stays small and the copy is quick.

---

## 2. Connection architecture & concurrency

`Repo` ([`sqlite.go`](./internal/store/sqlite/sqlite.go)) opens **two pools to
the same file**:

| Pool | `MaxOpenConns` | Role |
| --- | --- | --- |
| `writer` | **1** | Every write serializes — SQLite allows only one writer. |
| `reader` | **4** | Listings/lookups run concurrently and never queue behind a write. |

Every connection is opened with the same pragmas (via the DSN):

```
journal_mode(WAL)   busy_timeout(5000)   foreign_keys(ON)
```

- **WAL** lets the reader pool and the single writer proceed concurrently.
- **busy_timeout(5000)** makes a contended write wait up to 5 s rather than fail.
- **foreign_keys(ON)** enforces the `ON DELETE CASCADE` integrity in §8.

**Concurrency model.** The daemon already serves each RPC in its own goroutine,
so engine methods are written to be concurrency-safe; the single-writer pool is
what actually serializes mutations. Read-heavy work (catalog listing, the
full-text scan, coverage reports) fans out across the reader pool. This is why,
for example, the cross-patent full-text search can read many bodies in parallel
safely — they are reader-pool queries; only on-demand XML ingest takes the
writer.

---

## 3. Schema management & migrations

The schema is a single **embedded** file,
[`schema.sql`](./internal/store/sqlite/schema.sql) (`//go:embed`), written with
`CREATE TABLE IF NOT EXISTS` so re-applying it is idempotent. A `schema_meta`
key/value table pins the version:

```sql
INSERT INTO schema_meta (key, value) VALUES ('schema_version', '13')
ON CONFLICT(key) DO NOTHING;
```

`initSchema` runs, in order:

1. **`rejectObsoleteSchema`** — a pre-`schema_meta` (v1) database is refused with
   a clear "recreate/rebuild" message rather than silently corrupted.
2. **`migrate`** — version-chain **structural** migrations (`migrate.go`), run
   *before* `schema.sql` so rename-based upgrades (`… RENAME TO …`) aren't blocked
   by `schema.sql` having already created the new-shape tables. It walks the
   version chain (2→3→…→13).
3. **`schema.sql`** — creates anything still missing at the current version.
4. **`requireSchemaVersion`** — hard-fails unless the version is exactly **`13`**,
   so a binary never runs against a schema it doesn't understand.
5. **Best-effort `ALTER … ADD COLUMN` backfills** — additive columns introduced
   after a given version (e.g. `office_action.notes`, the `uspto_application`
   expiration columns, `source_diff.reconciled_*`, `record.classifications_text`)
   are added idempotently; "duplicate column" is swallowed. A removed column
   (`uspto_xml_download.source_url`) is dropped the same way.
6. **FTS sync** — `record_fts` (see §6) is rebuilt when it is out of step with
   `record`, including a drop/recreate when the indexed column set changed.

The net effect: an older database upgrades in place on open; a far-too-old one is
refused; a current one is a no-op.

---

## 4. The identity model (record · document · relation)

The schema implements the indirection described in
[README §2](./README.md#2-the-data-model--value-type-system). The domain type
`domain.Patent` is persisted as a **`record`** row (the table is named `record`,
not `patent`):

- **`record`** — the entity: a stable surrogate **`id`** (never changes) plus the
  unique canonical business **`number`**, and *projection columns* holding the
  reconciled/chosen display values (`title`, `abstract`, `assignee`, `inventors`
  JSON, `display_number`, dates, `classifications` JSON, …). Because the chosen
  values are denormalized onto the row, listing and detail render **without
  merging sources at read time**.
- **`document`** — one row per life-stage document (application / publication /
  grant), each with its `stage`, `dated`, and own `number`, `REFERENCES record
  (number) ON DELETE CASCADE`. `RecordOf` resolves any document number back to
  its canonical record — this is the indirection that prevents duplicate records
  across stages.
- **`relation`** — the family/citation graph as `(from_number, to_number, kind)`
  edges keyed on canonical record numbers.
- **`record_alias`** — alternate spellings/numbers that resolve to a record.

**Keying rule (by design):** source-derived tables key off the surrogate **`id`**
(resolved at write time, immune to number churn); the citation graph and
user data key off the canonical **`number`** (already canonicalized upstream).
`record_fts` is a full-text index over the record projection columns (§6).

---

## 5. Pointer-in-row, bytes-on-disk

Large source artifacts are **not** stored as blobs in the database. A row keeps a
**pointer + hash + extracted text**, while the bytes live on disk. This is the
same convention across crawl snapshots, USPTO XML, and matter documents, and it
is what keeps the DB lean and backups fast.

| Artifact | On disk | In the database |
| --- | --- | --- |
| **USPTO XML** (grant / pgpub) | the patents dir (`config.PatentsDir`), tracked in `uspto_xml_download` | parsed text + structured rows (below) |
| **Parsed patent body** | — | `uspto_grant_body` (`abstract_text`, `description_text`, `claims_text`, `claims_json`) keyed by `(application_number, kind)` |
| **Office actions / matter documents** | docs export dir (`<base>/<project>/…`) | `office_action` / `matter_document` rows: `blob_path`, `blob_hash` (SHA-256), `extracted_text`, metadata |
| **Drafts / responses** | rendered `.docx` written to the export dir | `draft` / `draft_section` / `draft_claim` rows (the editable source of truth) |

So the **searchable full text** of a patent is the plain text in
**`uspto_grant_body`**, produced once at XML ingest — that's what the full-text
viewer renders and what the cross-patent `Ctrl+F` search scans. A patent only has
searchable text after its XML has been ingested (hence the "not ingested"
coverage count in the search dock).

---

## 6. Full-text search storage

There are two distinct full-text mechanisms, for two different jobs:

1. **Catalog search → `record_fts`.** An FTS5 **external-content** virtual table
   over `record(title, abstract, classifications_text)`, kept in sync by
   `AFTER INSERT/DELETE/UPDATE` triggers on `record`. This backs the `/` catalog
   find/filter and `search:` filter field — fast, indexed, ranked.
2. **Patent body search → `uspto_grant_body` columns.** The claims/abstract/
   description plain text. The full-text viewer and the cross-patent search read
   these columns and substring-match in Go (preserving section locators like
   `Claim 5` / `Disclosure ¶0045` for the quicklist and jump-to-occurrence).
   `USPTOGrantBodySize` reads only `length(...)` for cheap coverage reports.

> [!NOTE]
> Body search is a substring scan today, which is ideal for structured,
> locator-preserving results over a selection. The scale-up path (whole-corpus,
> ranked, snippet search) is a second FTS5 index over `uspto_grant_body`, behind
> the same `fulltext.search` RPC — not an external `grep`/`rg`, since the text
> lives in SQLite and external markup search would lose the section structure.

---

## 7. Table catalog (schema v13)

Grouped by concern (≈50 tables; see `schema.sql` for columns):

| Domain | Tables |
| --- | --- |
| **Identity & graph** | `record`, `document`, `relation`, `record_alias`, `record_fts` |
| **Source reconciliation** | `source_bib`, `source_snapshot`, `source_diff`, `refresh_run`, `refresh_run_item` |
| **USPTO source data** | `uspto_application`, `uspto_party`, `uspto_event`, `uspto_continuity`, `uspto_foreign_priority`, `uspto_document`, `uspto_bulk_file`, `uspto_xml_download` |
| **USPTO parsed body** | `uspto_grant_body`, `uspto_drawing`, `uspto_grant_citation`, `uspto_grant_classification`, `uspto_grant_relation`, `uspto_grant_party` |
| **Assignments & ownership** | `uspto_assignment`, `uspto_assignment_party`, `assignee_history` |
| **Projects & curation** | `project`, `project_inventor`, `project_examiner`, `membership`, `membership_provenance`, `project_patent_note` |
| **Tags & classification** | `tag`, `patent_tag`, `classification_definition` |
| **Prosecution-matter workspace** | `office_action`, `matter_document`, `matter_document_office_action`, `matter_document_tag`, `matter_event`, `conflict` |
| **Drafting** | `draft`, `draft_section`, `draft_claim` |
| **Time, AI, deadlines** | `time_entry`, `ai_usage`, `deadline`, `reminder_log`, `patent_renewal` |
| **Infrastructure** | `schema_meta`, `saved_table_view`, `mutation_group`, `mutation_item` |

The `sqlite` package mirrors these groups one file per domain
(`patent.go`, `document.go`, `source_data.go`, `uspto_grant.go`,
`uspto_assignment.go`, `project.go`, `tag.go`, `ids.go`, `drafting.go`,
`deadline.go`, `conflict.go`, `mutation.go`, `table_view.go`, `patent_note.go`,
`classification.go`, `node.go`, `backup.go`).

---

## 8. Integrity, deletion & cascades

`foreign_keys(ON)` makes the schema's `ON DELETE CASCADE` constraints real:
deleting a `record` row purges its `document`, `relation`, `patent_tag`,
`membership`, and other child rows automatically. Tag *definitions* (`tag`) are
preserved even when assignments (`patent_tag`) cascade away, so the project
taxonomy survives.

On patent deletion the **engine** first writes a **full snapshot** (record +
documents + relations) to the activity journal as the record's `Before` payload,
*then* invokes the store's `DeletePatent` (`patent.go`), which hand-purges
incoming/outgoing `relation` rows (edges can point at not-yet-crawled stubs) and
removes the record row (cascades clear the rest). Because the snapshot is written
first, the purge is recoverable (see §9).

---

## 9. Backup & recovery

Two complementary mechanisms:

- **Whole-file backup** — `patentmine db backup` (and the `cmd/patentmine/db.go`
  family) copy the SQLite file; restore is a file swap.
- **Semantic replay** — every destructive operation writes a `Before` snapshot to
  the JSONL activity journal (`observability.Record`). A deleted patent can be
  **fully** replayed (metadata + documents + relations) or **soft**-replayed
  (re-created as a `FetchStub` with only its relation edges, healing the graph
  without heavy text). See [ACTIVITY.md](./ACTIVITY.md).

---

## 10. Conventions

- **Time** is stored as RFC3339 **UTC** strings (`encodeTime`/`decodeTime`); the
  zero time is the empty string, so "unset" and "epoch" never collide.
- **Lists** (inventors, classifications, claims) are JSON-encoded text columns;
  `record.classifications_text` is a space-joined denormalization so FTS can index
  it.
- **Metrics** — when opened with `OpenWithMetrics`, each store call records a
  duration under `store.sqlite.<name>` for the metrics overlay.
- **Layering** — `engine` → `store.Repository` → `sqlite.Repo`. RPC/web handlers
  never touch SQL; they call the engine, which calls the repository interface.

---

## 11. Inspecting the database

It's a plain SQLite file, so `sqlite3 "$(patentmine paths | grep -i db)"` works
for ad-hoc queries. Two handy ones:

```sql
-- How much full text is stored, and across how many bodies?
SELECT COUNT(*) AS bodies,
       SUM(length(coalesce(abstract_text,''))
         + length(coalesce(description_text,''))
         + length(coalesce(claims_text,''))) AS text_bytes
FROM uspto_grant_body;

-- Schema version this file is on.
SELECT value FROM schema_meta WHERE key = 'schema_version';
```

> [!WARNING]
> Read freely, but prefer the TUI/CLI/RPC for **writes** — direct edits bypass the
> activity journal, FTS triggers fire only on `record` changes, and a hand-edited
> `schema_version` will trip `requireSchemaVersion` on next open.
