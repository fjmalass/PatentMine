# Changelog

All notable changes to PatentMine are documented here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

---

## [v0.5.1] — 2026-05-29

### Added
- Patent expiration now reports the earliest-term parent: Source Application, Grant number, Grant date, Inventors, and Title (CLI and TUI overlay). Grant number falls back to parsing the USPTO grant XML file name when the API omits it.
- CLI prints the connected daemon's version and warns on a CLI/daemon version mismatch.

### Changed
- Lifecycle CLI subcommands renamed to `start-server`/`stop-server`, `start-api`/`stop-api`, `start-tui`/`stop-tui` (each writes/clears a pid file). Added `force-stop-daemon` and a forceful `restart-daemon`.
- `:` command prompt: `Tab` autocompletes the highlighted command, `Enter` runs typed input, and aliases run as typed. Wider command popup with an aliases column.

### Fixed
- Expiration: earliest-term parent was resolved via an OR query that let an application serial collide with an unrelated grant number; parents are now looked up strictly by application number.
- `:expiration` is available from the catalog/citations/family panes, not only detail.

---

## [v0.5.0] — 2026-05-27

### Database
- **Major `record_id` surrogate key migration (migrateG)**: Introduced a stable UUID-style `record_id` as the primary key on the `patent` table. All referencing tables — including `membership`, `patent_tag`, `project_patent_note`, `source_diff`, and others — were atomically rebuilt to use `record_id` instead of the human-readable `patent_number`. This is a foundational change that:
  - Enables reliable USPTO application-based loading and tracking (an application can be a first-class record even before a grant number is assigned).
  - Decouples internal foreign keys and graph edges from display numbers that can change or be rewritten by the crawler.
  - For `membership` specifically: the primary key and references changed from `(project_id, patent_number)` to `(project_id, record_id)`.
- **Membership `review_state` column rename (migrateH)**: The legacy `state` column on the `membership` table was renamed to `review_state` for naming consistency with the `ReviewState` domain type and the rest of the daemon/TUI model. The supporting index `idx_membership_project` on `(project_id, review_state)` was recreated under the new name. This migration is a no-op on already-migrated or fresh databases.
- The `membership` table retains its full curated IDS columns (`ids_kind_code`, `ids_in_full`, `ids_relevant_passages`, `ids_notes`, `ids_status`, `ids_added_at`, `ids_submitted_at`) that were folded in from the legacy `project_ids` table (via earlier migration F).
- All migrations are idempotent where possible, perform necessary backups for destructive rebuilds (F/G), and preserve data across the key and column renames.

### Added
- `domain.AllSourceModes()` helper returning the canonical list of source modes. This serves as the single source of truth and eliminates duplication in the engine (which previously maintained a parallel raw-string list due to import cycle constraints with the crawl package).

### Fixed
- `:add.uspto` (and equivalent USPTO-source paths) now populates the full comma-delimited `Classifications` list (normalized to the same compact form as Google) instead of only a single classification.
- Multi-selection followed by the `L` (explicit lookup / force re-fetch) command now correctly respects the active `source.mode` (including `uspto-only`, `google-only`, etc.). Previously it bypassed the mode and always loaded via Google.
- Loading popup shown for multi-selection `L`: now uses `DynamicSize` / `PctSize` to occupy a large portion of the screen (≈90% width, generous height targeting the lower ~65% area). Long status messages and error reports are wrapped via lipgloss instead of hard-truncated with `render.Truncate` / `Pad`.
- Brittle hardcoded source-mode string literals (`"uspto-only"`, `"google-only"`, etc.) removed from the RPC `crawlFamily` handler. Replaced with a proper `switch`/`case` over `domain.SourceMode*` constants (plus comment explaining the policy application for Only modes).

### Changed
- Source mode handling centralized: engine metrics observation now iterates `domain.AllSourceModes()` instead of a duplicated `sourceModeNames` slice.
- Various TUI / engine slop cleanups, column/emoji sizing fixes, stats pane refinements, USPTO XML full-text integration (disclosure/claims now drive the `T` viewer), duplicate `:add` handling (now logged as a no-op with metric), and TOML grant viewer command wiring.
- UI sizing and popup behavior for multi-selection flows improved overall.
- Review state naming and database column/index consistency aligned across the membership table, domain model, and daemon.

### Fixed (additional)
- Duplicate `:add` operations now surface as a clean warning + metric instead of silent or erroneous behavior.
- Multiple TUI rendering and alignment issues in stats, tables, and overlays.

---

## [v0.4.3] — 2026

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
