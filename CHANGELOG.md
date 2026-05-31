# Changelog

All notable changes to PatentMine are documented here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

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
