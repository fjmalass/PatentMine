# PatentMine

PatentMine is a local terminal app for collecting, reviewing, and annotating patent records.

## Requirements

- Go 1.24 or newer
- Rust/Cargo, used only to install `cargo-make`
- `cargo-make`

Install `cargo-make` with Cargo:

```sh
cargo install cargo-make
```

After installing, the task runner is available as either `makers` or `cargo make`.

## Basic Commands

Run the TUI:

```sh
makers run
```

Show CLI flags:

```sh
makers cli-help
```

Run the test suite:

```sh
makers test
```

Format, test, and build:

```sh
makers check
```

Build the binary:

```sh
makers build
```

Reset the local database:

```sh
makers reset-db
```

Show available project tasks:

```sh
makers --list-all-steps
```

## Logging

PatentMine writes logs to a dated file in the `logs/` directory. By default, `logs/patentmine.log` becomes `logs/patentmine-YYYY-MM-DD.log`, for example `logs/patentmine-2026-05-08.log`. Runs on the same date append to the same file.

Configure the base log path and retention:

```sh
go run -buildvcs=false ./cmd/patentmine --log-file ./logs/patentmine.log --max-logs 5
```

The `makers run` task reads the same settings from environment variables:

```sh
PATENTMINE_LOG_FILE=./logs/patentmine.log PATENTMINE_MAX_LOGS=5 makers run
```

Back up the current dated log:

```sh
makers backup-log
```

Log backups are stored under `backups/logs` by default. Keep at most five log backups by default, or override it:

```sh
PATENTMINE_MAX_LOG_BACKUPS=10 makers backup-log
```

## Header Bar

Every screen shows a persistent header line:

```
PatentMine  PROJECT: My Invention (myproject) · WIP  Patent List · filter:Smith, sort:status asc  · class:H04N
```

| Segment | Description |
|---------|-------------|
| `PatentMine` | App name (blue) |
| `PROJECT: <name> (<id>)` | Active project name and ID |
| `· WIP` / `· Provisional` / … | Application stage badge — color-coded (yellow = WIP, cyan = provisional, lavender = filed, blue = published, green = granted) |
| `· N unpaid` | Appears in yellow when the project has outstanding invoices |
| Screen title | Current view name, colored per screen (e.g. blue for list, cyan for detail, orange for citations) |
| `filter:<term>` | Active text or inventor filter |
| `sort:<col> <order>[,<col2>]` | Active sort (e.g. `sort:status asc,expiration`) |
| `class:<expr>` | Active classification filter, shown in light blue |

## Help System

### Full help screen

```
:help
```

Opens an interactive scrollable screen listing every command and shortcut, organized into sections (Navigation, Patents, Refresh & Citations, Views, Filters & Sort, Family, Project, Prosecution & Invoices, General).

**Search within help** — press `/` to activate the search bar, then type:

```
refresh          ← substring match
ref*             ← prefix match (anything starting with "ref")
project invoice  ← multi-word substring
```

`j`/`k` scroll through results. `esc` clears the query. `q` closes help.

### Context popup

Press `?` from any screen to open a small overlay showing only the keys relevant to the current view, plus global shortcuts.

## Filtering

All filters share the `:filter` command.

## Shortcuts

- `[j]` / `[k]` or **[↓]** / **[↑]**: Move selection.
- `[enter]` or `[o]`: Open patent detail.
- `[f]`: Toggle jump mode (shortcuts for fields/rows).
- `[L]`: Jump to Classifications.
- `[t]`: Jump to Full Text.
- `[n]`: Jump to Notes.
- `[ctrl+r]`: Jump to References.
- `[w]`: Open in browser.
- `[D]`: Delete patent.
- `[A]`: Add to IDS.
- `[u]`: Undo the last change.
- `[U]`: Redo the last undone change.
- `[?]`: Show help.
- `[q]`: Quit or back.

### Text / Inventor Filter

Type `/` followed by a term to filter the patent list by any text field (title, number, assignee, inventor):

```
/Smith
```

Filter by inventor from the command bar:

```
:filter inventor John Smith
```

Clear:

```
:filter inventor clear
```

You can also select an inventor in the **Inventors** popup (`l` from detail view) and press `Enter` to set the filter directly.

### Classification Filter

Filter by CPC/USPC prefix:

```
:filter class H04N
```

AND — patent must match both:

```
:filter class H04N && G06F
```

OR — patent must match at least one:

```
:filter class H04N || G06F
```

Clear:

```
:filter class clear
```

Expand a classification entry from the **Classifications** popup (`l` from any patent view) and press `Enter` to filter to that code directly.

The active class filter is shown in **light blue** in the header.

### Status Filter

The patent list defaults to **stored** patents only.

```
:filter status stored        ← default
:filter status ignored
:filter status under_review
:filter status cached        ← patents fetched as citation background data
:filter status none          ← show everything
```

Cycle the status of the selected list row inline with `s` (stored → under-review → ignored → stored).

### Reset all filters

```
:filter clear
```

Resets status, class, and inventor filters to defaults.

## Interactive Sorting & Navigation

In the main patent list, you can navigate and sort using Vim-like shortcuts:

- **Column Focus:** Use `[h]` / `[l]` or **Left** / **Right** arrow keys to move the yellow highlight across the column headers (Number, Title, Inventor, etc.).
- **Interactive Sort:** Press `[.]` (dot) to sort the list by the currently focused column. Pressing `[.]` again toggles between **Ascending** (`▴`) and **Descending** (`▾`) order.
- **Vim-like Navigation:**
    - `[j]` / `[k]` or **[↓]** / **[↑]**: Move row selection. Supports multipliers (e.g., `10j` moves down 10 rows).
    - `[h]` / `[l]`: Move column focus. Supports multipliers (e.g., `3l` moves focus 3 columns to the right).
    - `[gg]` / `[G]`: Jump to the very top or bottom of the list.
    - `[ctrl+f]` / `[ctrl+d]`: Page down and page up.
    - `[number]g`: Jump to a specific row number (e.g., `10g`).
    - `[/]`: Search the list (**Smart Case**: all-lowercase is case-insensitive; any uppercase character makes it case-sensitive). Use `[n]` for the next match.

## Visual Mode & Bulk Actions

Select and process multiple records at once:

- **Toggle Visual Mode:** Press `[v]` or `[V]` to start a multi-line selection. Move with `[j]`/`[k]` to expand the range (highlighted in deep blue).
- **Select All:** Press `[%]` to select every item in the current list.
- **Confirm & Execute:** With multiple items selected, press:
    - `[y]` or `[enter]`: **Store** all selected citations (triggers a confirmation popup and background download).
    - `[i]`: **Ignore** all selected items.
    - `[r]`: **Mark under review** all selected items.
    - `[s]`: **Cycle Status** for all selected items.
- **Cancel:** Press `[esc]` to clear the selection and exit visual mode.

### Sort

Sort by a single column:

```
:sort status
:sort date desc
:sort expiration asc
```

Sort by two columns (primary, then secondary):

```
:sort status,expiration
:sort status desc,expiration asc
```

Supported columns: `number`, `title`, `date`, `status`, `assignee`, `inventor`, `class`, `expiration`. Patents with no expiration date sort last.

## Undo, Redo & Recordings

Every data change — review-state updates (single and bulk), tag edits, notes,
date edits, IDS edits, deletes — is applied through a single change layer. That
layer makes the screen and the database stay in sync, and it powers undo/redo
and recordings.

### Undo / Redo

- `[u]`: Undo the last change.
- `[U]`: Redo the last undone change.

Review-state changes and IDS edits are fully reversible. Some changes (notes,
tags, date edits, deletes) cannot be undone; pressing `[u]` on one reports
`this change cannot be undone` rather than guessing.

Bulk actions are one transaction — they undo as a single step.

### Recordings

A recording captures every change you apply so the exact sequence can be
replayed later, against any project.

```
:rec start              begin capturing changes
:rec stop [path]        stop capturing and write the recording to a file
:rec play [path]        replay a saved recording
:rec list               show saved recordings
```

Recordings live under the **`recordings/`** root directory (created at startup
alongside `db/` and `logs/`). When `path` is omitted, `:rec stop` and `:rec
play` use a dated file in that root:

```
recordings/recording-YYYY-MM-DD.json
```

So `:rec stop` with no argument saves today's recording, and `:rec play` with
no argument replays it.

While a recording is active, a red `● REC` badge appears in the header bar.
Run `:rec` with no arguments to open a popup showing recording status and how
to stop it.

Replaying a recording re-applies each change in order; if a change fails it
stops and reports how many were applied.

## Activity Log

Every meaningful database change is appended to a monthly JSON-lines file:

```
logs/activity_2026_05.jsonl   ← current month
logs/activity_2026_06.jsonl   ← next month (created automatically)
```

Each line is a JSON object:

```json
{"time":"2026-05-10T14:32:11Z","level":"INFO","msg":"activity","project":"default","action":"patent.add","subject":"US11611785B2"}
{"time":"2026-05-10T15:01:44Z","level":"INFO","msg":"activity","project":"default","action":"note","subject":"US11611785B2","note":"started prior art review for claim 3"}
```

### Annotating work

Add a timestamped note tied to the current project and patent:

```
:note started reviewing claim 3 against H04N prior art
```

Notes are plain lines in the JSONL file. Delete by opening the file in any editor and removing the line.

### Querying with jq

**Today's activity:**
```sh
jq 'select(.time | startswith("2026-05-10"))' logs/activity_2026_05.jsonl
```

**Last 7 days (this month's file):**
```sh
jq -r '[.time, .action, .subject, .note] | @tsv' logs/activity_2026_05.jsonl | tail -50
```

**All notes:**
```sh
jq 'select(.action == "note")' logs/activity_2026_05.jsonl
```

**Everything touching one patent:**
```sh
jq 'select(.subject == "US11611785B2")' logs/activity_*.jsonl
```

**Status changes only:**
```sh
jq 'select(.action == "patent.status" or .action == "citation.status")' logs/activity_*.jsonl
```

**Daily summary — count of actions per day:**
```sh
jq -r '.time[:10]' logs/activity_*.jsonl | sort | uniq -c
```

**Last month's file:**
```sh
jq -r '[.time, .action, .subject] | @tsv' logs/activity_2026_04.jsonl
```

### Rotation

Files rotate monthly. Old files are never deleted automatically — keep or archive them as needed. To query across all months at once, use the glob `logs/activity_*.jsonl`.

## Task Reference

- `makers run`: launch the PatentMine TUI
- `makers cli-help`: show PatentMine CLI flags
- `makers test`: run all Go tests
- `makers fmt`: format Go files
- `makers build`: build `./patentmine`
- `makers check`: run `fmt`, `test`, and `build`
- `makers tidy`: resolve Go module dependencies
- `makers logs`: print the last 80 TUI log lines
- `makers reset-db`: remove the local `db/` directory
- `makers backup`: create a timestamped backup
- `makers backup-log`: create a dated backup of the current log
- `makers list-backups`: list existing backups
- `makers clean`: remove the built binary
