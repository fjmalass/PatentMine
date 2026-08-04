# Changelog

All notable changes to PatentMine are documented here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- **Drafting subsystem** (`internal/domain/drafting.go`, `internal/export/docx`, `internal/engine/engine_drafting.go`): author **provisional** and **non-provisional first applications** and **office-action responses** as project-scoped, section-structured documents and render them to **.docx**. One `Draft` model (kind = provisional / nonprovisional / oa_response) seeds the conventional section skeleton at creation. Schema bumped to **v7** with additive tables `office_action`, `draft`, `draft_section`, `draft_claim` (migration `migrateV6ToV7`).
- **Pure-Go .docx writer** (`internal/export/docx`): a dependency-free WordprocessingML generator (stdlib `archive/zip`), preserving the single-static-binary build. Claim-amendment markup (MPEP 714 underline-insertion / strikethrough-deletion + status identifiers) is rendered **deterministically in code** from a word-level diff of `base_text` vs `text`, never by the model.
- **Office-action import**: `ImportOfficeAction` records the examiner document with the bytes on disk (SHA-256 hashed) and metadata in the row (the `source_snapshot` convention), plus extracted text for grounding. Manual import is the baseline path; a `.txt` source doubles as extracted text.
- **Grounded AI drafting** (`internal/ai/drafting.go`): `BuildDraftPrompt` assembles a source-blocked, anti-hallucination prompt (use only provided material; reproduce pinned spans verbatim; mark unsupported statements `‹NEEDS ATTORNEY INPUT›`). `VerifyDraft` / `VerifyVerbatim` mechanically verify pinned quotes were reproduced. Both Gemini (cloud) and Ollama (local) gained a generic `Complete` + `Model` and satisfy the new `ai.Drafter` interface; `ai.NewDrafter` selects the provider from config.
- **`patentmine draft` CLI** (`new` / `list` / `show` / `export` / `ai` / `oa-import` / `oa-list`) over the daemon RPC, and **`cargo make draft` / `draft-setup` / `draft-demo`** tasks in `Makefile.toml`.
- **RPC**: `draft.create`, `draft.get`, `draft.list`, `draft.save`, `draft.delete`, `draft.export.docx`, `draft.section.ai`, `office_action.import`, `office_action.list`, `office_action.get`, `office_action.save_notes`.
- **TUI**: `:officeaction.add [path]` imports an Office Action from any directory into the active project. With no argument it opens a **file-picker** overlay (a small hand-rolled directory browser — `.pdf`/`.txt`; it keys off `DirEntry.IsDir()` and never stats, so a broken symlink in the browsed directory can't crash it, unlike `bubbles/filepicker`). The file is copied into the docs export store. The TUI/daemon share a filesystem (unix socket), so the picker browses locally and only the path crosses RPC.
- **Office-action notes**: `office_action.notes` column + `engine.SaveOfficeActionNotes` / `office_action.save_notes` RPC.
- **TUI `:officeaction.list`**: opens the project's office-action **table**; selecting one opens a **split editor** — examiner extracted text on the left (read-only, vim-navigable), attorney notes on the right (editable, opens in vim NORMAL). `ctrl-w h/l/w` switches panes (nvim-style), `ctrl+s` saves the notes, `esc` closes. Read-only buffers (`vimBuffer.readOnly`) accept vim motions/search but block edits; plain-mode `j/k` now insert literally (was a cursor-motion quirk).
- **Docs**: `12_DRAFTING.md` and `13_TUI_OFFICE_ACTION.md` (linked from `01_README.md`).
- **Prosecution-matter workspace — multi-document + examiner metadata** (schema **v8**, migration `migrateV7ToV8`): a matter (project) now holds **many documents** — the office action, its response, cited references, the spec — not one blob. New `matter_document` table (project-scoped, optional `office_action_id` link, `kind`, renameable `display_name`, extracted text); the v7 single office-action blob is **backfilled** as the matter's first document. Projects gain a **`matter_type`** stage (provisional / nonprovisional / in_prosecution / issued) and office actions gain a **`response_due`** deadline (auto-computed from mail date + type) and **`status`**.
- **PDF text extraction** (`internal/pdf`, `github.com/ledongthuc/pdf`): imported `.pdf` documents are text-extracted (de-hyphenated, whitespace-normalized) so they can be read and copied from; the extractor recovers from the library's panics and returns empty (not an error) for scanned/image-only PDFs (no OCR).
- **TUI**: `:officeaction.add` now opens a **metadata form** (examiner, mail date, type, art unit, application number — pre-filled from the project) before import, so the examiner name is captured. New `:matter.document.add` files a supporting document, `:matter.document.open` lists the matter's documents (enter views the text in a read-only vim viewer, `r` renames, `d` deletes), and `:matter.type.set <type>` records the prosecution stage.
- **RPC**: `matter.document.import` / `.list` / `.rename` / `.delete`, `project.set_matter_type`. Engine: `ImportMatterDocument`, `ListMatterDocuments`, `RenameMatterDocument`, `DeleteMatterDocument`, `SetMatterType`; `ImportOfficeAction` now seeds the response deadline and registers the OA's document.
- **AI/OCR text extraction for scanned documents**: USPTO office actions are commonly scanned, image-only PDFs the pure-Go extractor reads as 0 chars. New `ai.Extractor` interface + Gemini multimodal `ExtractText` (base64 inline document, 3-min client) transcribes them on demand. Engine `ExtractDocumentText` runs it per document and saves the text back; RPC `matter.document.extract`; in `:matter.document.open` press **`e`** to extract a scanned document's text (4-min client timeout, kept separate from the fast import). The extraction is recorded as an `matter_document.extract` activity + `engine.extract_document_text` duration metric + a slog line (provider/model/chars) — billable provenance, reusing the existing observability stack (no new logger).
- **Response generation — copy from documents into a draft** (`:officeaction.respond`): creates a first-class `DraftOAResponse` linked to the latest office action and opens a **split response editor** — the matter's documents on the left (read-only, `ctrl-n`/`ctrl-p` cycles which one), the response's **REMARKS** on the right (editable). A shared **vim yank register** (`yy`/`Nyy` yank — allowed in read-only panes — and `p`/`P` paste, in `internal/tui/overlay/vim_buffer.go`) copies a passage from the source into the response; `ctrl+s` saves the draft, `ctrl-e` exports it to **.docx** via the existing renderer. The same shared register now also lets the `:officeaction.list` notes editor copy examiner text into the notes.
- **Communications log** (`matter_event` table): a per-matter running record of emails, phone calls, examiner interviews, filings, and notes — who it was with (`party`) and what happened (`comment`). `:matter.comm.log` opens an entry form; `:matter.comm.open` lists the log (newest first; enter views the full comment, d deletes). RPC `matter.event.add` / `.list` / `.delete`; engine `AddMatterEvent` / `ListMatterEvents` / `DeleteMatterEvent` with `matter_event.add` / `.delete` activity records.
- **Time tracking + AI usage** (`time_entry`, `ai_usage` tables): the engine records billable work — AI drafting (`draft.section.ai`) and OCR (`matter.document.extract`) calls write an `ai_usage` row + an auto **unvalidated** `time_entry` (activity `ai`), and the split editors **auto-capture reading vs writing** time (left pane = reading, right = writing) on close. Full layering: domain `TimeEntry`/`AIUsage`/`TimeSummary`, store CRUD + summaries, engine `LogTime`/`UpdateTimeEntry`/`DeleteTimeEntry` + `recordAICall`, RPC `time_entry.log`/`.list`/`.unvalidated`/`.update`/`.delete`/`.summary`. **TUI**: `:time.validate` opens a **review queue** (correct activity/duration/note, then validate or delete — captured time is a draft until a human signs off); `:time.show` is the billing readout (time by activity + validated/unvalidated split + AI calls/tokens); `:time.log <activity> <duration> [note]` adds a manual (validated) entry; reopening a matter with unvalidated time **prompts to review it**. Durations only for now (rates later).
- **Office Actions pane** (`:officeaction.list`, new `ScopeMatterOA` / `ScopeMatterOADetail`): the office-action surface is now a first-class **pane** — a navigable table (mailed · type · examiner · **response-due countdown** · status) with `a` add, `R` respond, `/` filter, and **`enter` drill-in** to a **detail pane** showing the OA metadata + deadline, the matter's document/communication counts, and the time + AI-usage tally (with an "unvalidated — review before billing" cue); detail keys `f`/`c`/`R`/`enter` reach documents / communications / response / the notes editor. Replaces the prior list overlay.
- **Deadlines + reminders** (unified `deadline` model + `internal/remind`): one `deadline` table (kinds `oa_response` / `maintenance_fee` / `annuity` / `custom`) and one reminder engine cover prosecution responses **and** patent renewals. `:renewal.track <patent>` derives a granted patent's **U.S. maintenance-fee** schedule from its grant date (3.5 / 7.5 / 11.5 yr, with the 6-month pay window and grace); office-action response dates are synthesized into the same view. `:deadline.show` is the cross-matter docket (overdue/due-soon cues; `p` done, `x` dismiss), and opening a matter shows an in-app banner when something is due soon. A **pluggable `Notifier`** (`remind.Notifier`) sends email reminders over **SMTP** at **2 months / 15 days / 7 days** before due (+ overdue), deduped via `reminder_log`, fired by a daily loop — opt-in via `PATENTMINE_REMINDER_EMAIL_*` config; the default is a no-op (in-app surface only). Dates only — fee dollar amounts (entity-size dependent) are intentionally not encoded. RPC `deadline.list` / `.track_renewals` / `.set_status` / `.remind`. **Docketing assistance, not legal advice — verify all dates.**

### Changed
- **Consolidated the modal-vim editor** into one shared `vimBuffer` core (`internal/tui/overlay/vim_buffer.go`). `TextArea` and `PatentNoteEditor` were near-duplicate hand-rolled vim editors (~1,605 lines, two copies); both now delegate to the shared buffer (~1,110 lines total, editor logic in one place). Same editing behavior; this is the reusable core the office-action notes editor builds on.
- **Renamed `PATENTMINE_IDS_EXPORT_DIR` → `PATENTMINE_DOCS_EXPORT_DIR`** now that the directory holds IDS PDF bundles, rendered drafts (`.docx`), and imported office-action files — not just IDS. The old variable is still honored as a **deprecated fallback**, so existing `.env` setups keep working. Internally, `config.IDSExportDir` → `DocsExportDir` and `engine.WithIDSExportDir`/`WithDraftExportDir` collapse into one `engine.WithDocsExportDir`.

## [v0.12.0] — 2026-06-03

### Added
- **Patent kind display and stage detection**: new `DisplayString()`, `Stage()`, `IsGrant()`, `IsPublication()`, `IsApplication()` methods on `PatentNumber`. US serials formatted with commas (`US11,795,648B2`), applications as `XX/YYY,YYY`, publications as `YYYY/NNNNNNN`. Kind codes beyond `B*` recognised as grants (`E*`, `S*`, `P*` excluding `PL`). Reissue numbers auto-prefixed with `US`.
- **KIND column** in catalog detail, inventor stats, classification stats, and assignee history overlays — sortable stage glyph column (trophy grant, page publication, pencil application).
- **Sorting by kind** via `SortByKind` with SQL rank ordering (Grants→Publications→Applications).
- **Family graph export**: `ExportFamilyGraph()` generates transitively reduced Mermaid flowchart + ASCII preview + timeline diagram with PDF rendering via mermaid.ink. Exposed as `:export.family.mermaid` and `y` key binding.
- **Relation type annotations** on family graph edges: continuity types (CON, CIP, DIV, PROV, REISSUE) extracted from USPTO data with dedicated glyphs.
- **Tree-based family graph rendering**: box-drawing tree layout with Root/Parents/Children sections, cycle detection, alternating row colors.
- **Stage-based colour styling**: `StyleStageGrant` (green/bold), `StyleStagePublication` (dim), `StyleStageApplication` (warn) on `Theme`.
- **Assignee history fallback**: falls back to at-grant assignee when no USPTO assignment chain exists.
- **Worker pool panic recovery**: `recover()` guard logs and reports crawl panics instead of crashing.
- **USPTO Assignment API warning**: health check warns when assignment API key is active but not activated; status bar shows warning glyph.
- **Auto-force crawl of stub patents**: engine auto-crawls `FetchStub` patents during crawl/expiration/USPTO lookup.
- **Crawl depth in status messages**: loading overlays show depth (e.g. "Crawling US12345678 (depth 4)").

### Changed
- **Patent number regex expanded**: accepts alphabetical serial prefixes (`RE`, `D`, `PP`) for reissues, designs, plant patents.
- **`GuessStage()` broadened**: uses full `IsGrant()`/`IsPublication()`/`IsApplication()` instead of narrow `strings.HasPrefix(n.Kind, "B")`.
- **USPTO crawl queries**: delegate kind-code routing to `HasExplicitGrantKind()`/`HasExplicitPublicationKind()`.
- **Google Patents scraper**: deduplicates kindless vs. kind-coded references; overrides guessed `StageApplication` to `StageGrant` for metadata labeled as patent.
- **USPTO relations**: `relationsFromUSPTOContinuity()` no longer inserts redundant bidirectional `RelationChild`.
- **Family graph capacity**: max depth 6→10, max nodes 200→400.
- **Inconsistency detection relaxed**: flags when *either* parent or child is unseen (not both).
- **REST API**: assignee routes consolidated to `AllAssigneesHistory`; old `/assignees/stats` is a compatibility alias.

### Fixed
- **Stalls when crawling with multiple potential sources**: `Bus.Publish()` resilient to closed subscriber channels; worker pool recovers from panics.
- **USPTO assignment warning not displayed**: health check surfaces warning when assignment API not activated.
- **Duplicate kindless documents from Google Patents**: drops redundant kindless versions when kind-coded version is present.
- **Empty assignee history**: falls back to at-grant assignee from `record` table instead of returning empty list.
- **USPTO strict query for ambiguous kind codes**: searches all identifier fields when kind code is absent/unrecognised.

## [v0.11.0] — 2026-06-02

### Added
- **Web UI**: new React/TypeScript frontend (Vite-based) with catalog, detail, family, IDS, metrics, notes, relations, and settings views; served via REST API static file endpoint.
- **Assignee Timeline overlay**: interactive timeline and batch USPTO assignment fetch overlays (`:open.assignees-project`, `:fetch.uspto.assignments-project`).
- **REST API**: security middleware, session management, AI endpoint, filters/events endpoints, static file serving.
- **REST API engine parity tests**: comprehensive test suite validating REST API behavior matches engine behavior.
- **Benchmarking infrastructure**: `BENCH.md` and `scripts/bench-api.sh` for API performance benchmarking.
- **Tableview enhancements**: improved sort/filter interactions.
- **Prompt overlay tests**: test coverage for the prompt overlay component.

### Changed
- **Stats overlays refactored**: classification, inventor, and assignee stats overlays consolidated and simplified.
- **All Notes pane and Orphans pane refactored**: improved layout and interaction patterns.
- **Project pane improvements**: last-visited tracking and UI cleanup.
- **Assignee naming cleaned up**: command naming, catalog entries, and documentation standardized.

### Fixed
- **Inventor search**: SQLite inventor query now correctly matches search terms.

## [v0.10.1] — 2026-06-01

### Fixed
- **Multi-select failures lost per-number context**: `MultiCrawlCmd` now attributes each error to its patent number instead of concatenating opaque strings, and the `Loading` overlay propagates startup/validation errors through `WithErrors`, displaying them live and in the done summary.
- **`MergeRecords` silently discarded source-derived data and curated review states**: merging a record into another (`:add` hitting an existing record) previously cascade-deleted the absorbed record's source-side tables (`source_bib`, `source_diff`, `uspto_application`, `uspto_document`, etc.) and overwrote a curated membership review state with an auto-default. Fixed by repointing source-derived rows via surrogate record id before the absorb record deletion and promoting the absorbed record's non-auto review state onto keep's membership row.
- **`Ctrl+C` in loading overlay killed the app**: the `Loading` overlay no longer binds `ctrl+C` to `tea.Quit`, preventing accidental process termination during crawls.
- **`:add` inconsistencies between application/grant/publication in Google and USPTO-only modes**: fixed Google data loading inconsistencies across patent stages and USPTO-only `:add` from Google-imported data; pre-1976 grants now show a clear message instead of a generic failure.

### Added
- **Assignee history tracking**: new `assignee_history` table (schema v5→v6 migration) records the full ownership timeline per record from at-grant assignee and recorded USPTO assignments. `RebuildAssigneeHistory` populates it lazily on (re)fetch. New commands `:open.assignees-project` (project-wide assignee rollup overlay with current-vs.-all coverage) and `:fetch.uspto.assignments-project` (batch assignment fetch for the active project). New `11_DAEMON_ASSIGNEE_FLOW.md` and `10_TUI_ASSIGNEE_FLOW.md` documentation.

### Changed
- **Command catalog split**: the monolithic `Default()` registry in `internal/command/catalog.go` was factored into per-domain files (`catalog_view.go`, `catalog_patent.go`, `catalog_crawl.go`, `catalog_project.go`, `catalog_tags.go`, `catalog_classification.go`).
- **Proto/RPC type extraction**: all method parameter, result, and event types moved out of the monolithic `internal/proto/proto.go` and `internal/rpc/server.go` into domain-specific files (e.g. `proto_uspto.go`, `server_added.go`, etc.), reducing the monoliths by thousands of lines and making the API surface navigable.

## [v0.10.0] — 2026-06-01

### Fixed
- **`:add.file` silently dropped zero-padded grant numbers**: a list entry like `US09658068B2` (leading-zero–padded grant number) parsed to an identity distinct from `US9658068B2`, and Google Patents only serves the un-padded form — so the crawl 404'd and the not-found cleanup deleted the membership, leaving the patent missing with no record, no stub, and no message. Root cause fixed in `domain.ParsePatentNumber`: a number carrying a kind code (e.g. `B2`/`A1`) is a grant/publication identifier whose leading zeros are insignificant padding and are now stripped, so `US09658068B2` and `US9658068B2` are one identity that resolves cleanly. Bare application serials (no kind code, e.g. `09/658,068`) keep their leading `09` series digits, which are significant.
- **`:add.file` outcome was a transient status line**: a partial import (some numbers not found or ambiguous) flashed a one-line status that was easy to miss, so it looked like patents had silently vanished. It now ends with a result popup summarizing how many of how many were added, with a per-number reason for each failure.
- **Grant kind code dropped on ingest**: USPTO grant ingestion built the grant document's number from the digits-only grant number while the kind code (`B2`) arrived separately in `grant_kind`, so the stored grant document was kind-less. This made `:export.added` emit numbers like `US09658068` instead of `US09658068B2`, which Google Patents will not resolve. Fixed at the source (`uspto_grant.go` now merges `grant_kind` into the grant document number) and added a `v4`→`v5` data migration that backfills existing kind-less grant documents from `uspto_application.grant_kind` (kind-only stamp; the canonical number is upgraded only where it would not collide with a divergent doc-centric record).

### Added
- **Bulk patent lists**: new `:add.file <path>` (aliases `add-file`, `load`) loads a plain-text patent list into the active project with manual provenance, and `:export.added [path]` (aliases `export-added`, `add.export`) writes a project's manually-added patents back out to such a file. `:export.added` takes an optional path; with none it opens a confirmation popup showing the default save location, and with an explicit path it warns before overwriting an existing file. After writing it shows a result popup with the patent count and destination. Exported numbers use each record's grant-stage, kind-coded form (`GrantedNumber()`, e.g. `US11611785B2`) for Google Patents compatibility. REST export returns the count in the `X-Patent-Count` header. The format carries a mandatory magic header (`# patentmine added-patents v1`) so wrong-kind files are rejected; one number per line, `#` comments allowed. Round-trips cleanly. New `internal/patentlist` package (with tests), `added.export`/`added.import` proto methods, REST routes `GET /projects/{id}/added/export` and `POST /projects/{id}/added`, activity records (`added.export`/`added.import`) and metrics. Provenance icons documented in `04_TUI_ADD_FLOW.md`.

## [v0.9.1] — 2026-05-31

### Fixed
- **REST-API**: completed missing API endpoints (crawl, metrics, notes, patents, projects, USPTO), added `05_REST_API.md` documentation, removed Google crawl source switches, fixed browser tests and detail-pane patent-number display

### Changed
- Standardized all date/time formatting across the codebase to use centralized layout constants (`DateLayout`, `DateTimeLayout`, etc.) in `internal/domain/patent.go` — replacing inline layout literals in crawl, uspto, export, TUI, RPC, and store layers

## [v0.9.0] — 2026-05-31

**Major database update.** Automatic migration on first run.

### Added
- **Record Identifiers**: new `rec.id` unique identifier replacing ambiguous name/record/patent references across the codebase
- **Date Format Centralization**: canonical date/time layout constants (`DateLayout`, `DateTimeLayout`, `FileTimestampLayout`, `CompactDateLayout`, `USDateLayout`) in `internal/domain/patent.go` — single source of truth for every date format the app parses, stores, or displays
- New `:source.bibs` overlay for browsing bibliographic source data
- New `source_data` table and domain model for structured source metadata
- Unit tests for migration and SQLite store

### Changed
- Database schema overhaul: new `node` / `source_data` tables replace scattered record-keeping, with updated `patent`, `document`, and `uspto_grant` tables aligned to the `rec.id` model
- All date/time formatting across crawl, proto, uspto, observability, and TUI layers now uses centralized constants instead of repeated layout literals
- Source data domain types extended with comparison fields and document metadata

## [v0.8.1] — 2026-05-31

**Database schema changed** (v2 → v3). Automatic migration on first run.

### Fixed
- `TUI` `command` `help` wrapping rows
- USPTO provenance tracking: membership records now correctly capture source provider and mode for citation and family-graph promotions

### Added
- Reintroduced expiration-date columns (`patent_term_adjustment_days`, `patent_term_extension_days`, `terminal_disclaimer_date`, `earliest_term_filing_date`, `computed_expiration_date`) on `uspto_application` table
- New `:expiration` command to compute and display patent term expiration based on USPTO statutory inputs

### Changed
- Renamed CLI commands for consistency (WIP): `:fetch.uspto.grant`/`:fetch.uspto.pgpub` → `:fetch.grant`/`:fetch.pgpub`
- Cleaned up TUI layout, detail pane spacing, and status bar alignment
## [v0.8.0] — 2026-05-30

**Database schema changed** (v2 → v3). Automatic migration on first run.

### Added
- **Membership Provenance Tracking**:
  - New `membership_provenance` table records how each patent entered a project: `added_method` (`direct`, `related`, `neighbors`, `citation`), `parent_patent_number`, `source_provider` (`uspto`/`google`/`file`), and `source_mode` (`compare`/`uspto-first`/etc.).
  - Membership provenance backfilled for existing databases via automatic v2→v3 migration.
  - Detail pane now displays a human-readable Provenance field with distinct glyphs for each ingestion method (Direct, Promoted from Citation, Neighbor, etc.) including parent patent, source provider, and source mode.
  - New PROVENANCE column in the patent table (sortable, project-scoped).
- **Provenance Filter**:
  - `:filter provenance:manual` / `:filter prov:related` / `:filter provenance:system` predicates with aliases `prov:` and `provenance:`. Supports keywords `manual`/`direct`, `related`, `neighbors`, `system`/`auto`.
  - Full integration with the boolean filter DSL (`:filter provenance:system and state:under_review`).
- **Review State Provenance Awareness**:
  - Review state changes from Citations and Family Graph panes now record the correct `added_method` (e.g. `citation`, `related`) in the membership provenance.
- **`replay` CLI Command**:
  - New `patentmine replay` sub-command for replaying history entries.
- **USPTO Crawl Refinements**:
  - API queries switched from `earliestPublicationNumber` to `publicationNumber` with fallback unmarshalling for backward compatibility.
  - Enhanced logging around query formulation and patent matching.
- **Test Coverage**:
  - SQLite store tests for provenance read/write, migration, and schema integrity.
  - Test for `earliestPublicationNumber` fallback in USPTO response parsing.
  - Unit tests for provenance filter parsing.

### Changed
- `Membership` domain struct extended with `AddedMethod`, `ParentPatentNumber`, `SourceProvider`, `SourceMode`.
- `PatentResult` RPC struct now includes the full `Membership` object.
- Theme engine gains `ProvenanceGlyph()` method for rendering provenance indicators.
- USPTO API query field changed from `earliestPublicationNumber` to `publicationNumber`.
- `ReviewStateParams` RPC struct extended with optional `AddedMethod` field.

### Fixed
- USPTO crawl parsing correctly handles responses that only include `earliestPublicationNumber` without `publicationNumber`.

## [v0.7.1] — 2026-05-30

No database schema changes.

### Added
- **USPTO Loading & Source Configuration**:
  - Full integration with the USPTO Open Data Portal (ODP) for bibliographic metadata search and USPTO bulk datasets for grant / pre-grant XML files.
  - Support for `PATENTMINE_USPTO_API_KEY` with standard `.env` configuration file loading and secure `file:path` secret indirection.
  - Four runtime source-provider policy modes: `compare` (fetch from both USPTO and Google), `uspto-first` (default with fallback), `uspto-only` (strict provenance), and `google-only`. Configurable via environment/`.env` or dynamically via `:source.mode`.
- **`:add` Command Family**:
  - Support for `:add` (honors current source mode), `:add.uspto` (force USPTO), and `:add.google` (force Google).
  - Multi-form input handling: works on cursor row, visual selections, single typed patent number, and space-separated batch numbers.
  - Parallel background execution of batch patent additions.
- **USPTOCandidatePicker Overlay**:
  - Interactive TUI popup picker (80% × 80%) allowing the user to select the exact application when USPTO search yields multiple candidates.
- **Manual & Auto XML Fetching**:
  - Auto-fetch and ingest of USPTO XML following crawls and membership additions.
  - New explicit commands `:fetch.uspto.grant` and `:fetch.uspto.pgpub` supporting parallel fetches.
  - Direct detail-pane keyboard shortcut (**Enter** on `PGPub URL`, `Grant URL`, `PGPub XML`, or `Grant XML` rows) to fetch, parse, and ingest XML documents.
  - XML download caching tracked via a persistent `uspto_xml_download` table.
- **Expanded Browse Commands**:
  - Explicit browser open commands: `:browse.uspto`, `:browse.uspto.grant`, `:browse.uspto.pgpub`, and `:browse.google`.
  - Automatic `api_key=<key>` query-param injection when opening ODP URLs in the external browser.
- **Parsed Body & Full-Text Integration**:
  - Granular database storage for parsed USPTO XML sections: claims (individual statements), abstract, description, drawings, classifications, and relations.
  - Full-text viewer (`T` / `:open.fulltext`) prioritizing parsed USPTO XML text over external Google crawl fallback.
  - USPTO patent citation normalization: parses references from XML, populates the regular `relation` table (`cites` edges), and generates stubs for uncrawled references to construct a complete project citation graph.
- **Telemetry & Metrics**:
  - Structured logs, activity journal entries, and custom metrics around XML fetch/cache/parse operations, database saves, and runtime source mode changes.

### Changed
- Active project filter default updated to `:filter fetch_state:cached` (along with active default filters).
- Configuration loading refined to target `PATENTMINE_USPTO_API_KEY`.
- Splash screen upgraded to display live background daemon connection status.

### Fixed
- Keyboard shortcuts for active `review_state` selections.
- Splash screen USPTO connectivity checks and general table rendering alignment.

---

## [v0.4.2] — 2026

### Added
- `check` command: connectivity tests for USPTO API, Backblaze B2 (native API and rclone)
- `.env` configuration file with file-based secret indirection (`file:path` syntax)
- Makefile tasks: `check`, `check-uspto`, `check-b2`, `check-b2-api`, `check-rclone-b2`

### Changed
- Makefile.toml: DRY version/linker-flag computation with reusable `PM_VERSION_SCRIPT`
- Config resolution supports file-based (`file:`) key references for secrets
- `outputs/` and `exports/` directories added to `.gitignore`
- `PATENTMINE_IDS_EXPORT_DIR` configurable in Makefile.toml

### Fixed
- RPC server `cmd/kill` handler now correctly targets the running server

---

## [v0.4.1] — 2026

### Added
- IDS export: configurable `PATENTMINE_IDS_EXPORT_DIR` in Makefile.toml (defaults to `./exports`)

### Changed
- Refined version string injection and update/activity logging

---

## [v0.4.0] — 2026

### Added
- IDS export: generate SB0008a/b/c/PC PDF forms populated with patent references
- Patent notes: per-patent markdown notes with listing, full-text search, and bulk export
- All Notes pane: global view of all notes across the project
- `:fetch_state` and `:country` filter predicates
- `Z` shortcut to open notes editor for selected patent
- Consolidation of stats/inventor/assignee/history subtable rendering

### Changed
- FetchState/ReviewState model clarified with slug/cached/unknown/under_review/ignored/active/deleted states

### Fixed
- `ga` select-all now captures patents across all pages, not just the current page
- Emoji glyph alignment in status line and history header

---

## [v0.3.4] — 2025

### Added
- Collapse/expand toggle for family and references sections in the detail pane
- `backup` task in Makefile.toml: archives logs, SQLite DB, and Makefile.toml to a timestamped archive

---

## [v0.3.3] — 2025

### Fixed
- Emoji glyph alignment in status line and history header

---

## [v0.3.2] — 2025

### Added
- History popup: JSONL-backed change history viewer with cursor indicator and icon header
- Classification and assignee distribution stats panel for the active project
- Makefile.toml: task runner config wired for build/run/test workflows

### Changed
- Tags and review_state writes unified onto a single debounced path

### Fixed
- History popup alignment and cursor indicator rendering

---

## [v0.3.1] — 2025

### Added
- Filter DSL: `and`/`or`/`not` boolean operators over classification/state predicates
- Inventor filter: `:inventor *glob*` pattern matching
- Assignee filter: `:assignee` predicate support
- README updated with new filter command reference

---

## [v0.3.0] — 2025 — Complete Refactor

Complete architectural rewrite with clean separation of concerns.

### Architecture
- Layered design: domain → store → engine → proto/RPC → TUI; each layer depends only downward
- Daemon/client split over JSON-RPC; fixed race conditions in the TUI update loop
- Unified `Pane` interface (Scope, Title, Init, Command, Handles, Update, View, Selection); all screens are composable panes pushed onto a navigation stack
- Structured logging and metrics wired throughout via `internal/observability`

### Added
- Per-patent markdown notes with add/edit/delete and full-text search
- Modal navigation: normal/visual/jump modes; `ga` select-all, `G`/`gg`, visual range selection
- CPC classification hierarchy browser and bulk classification operations
- Bulk tag assignment with debounced write path
- FTS5 full-text search across title/abstract/claims via SQLite FTS5 virtual table
- Engine-level batch patent metadata write
- Patent family relations display
- Jump mode: quick-jump to any visible item by label
- Inventor search and filter
- Open patent in external browser from TUI

---

## [v0.2.1] and earlier

See git log for pre-refactor history.
