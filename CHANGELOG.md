# Changelog

All notable changes to PatentMine are documented here.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).
Versions follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
