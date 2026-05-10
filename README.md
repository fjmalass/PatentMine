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

## Filtering

### Text / Inventor Filter

Type `/` followed by a term to filter the patent list by any text field (title, number, assignee, inventor):

```
/Smith
```

Filter by a specific inventor from the command line:

```
:inventorfilter John Smith
```

Clear the inventor filter:

```
:inventorfilter clear
```

You can also select an inventor in the **Inventors** popup (`l` from detail view) and press `Enter` — this sets the inventor filter directly.

### Classification Filter

Filter by CPC/USPC classification prefix:

```
:classfilter H04N
```

AND filter — patent must match both prefixes:

```
:classfilter H04N && G06F
```

OR filter — patent must match at least one prefix:

```
:classfilter H04N || G06F
```

Clear the classification filter (also clears the text/inventor filter):

```
:classfilter clear
```

You can also expand a classification entry from the **Classifications** popup (`l` from any patent view) and press `Enter` to filter the list to that code directly.

The active class filter is shown in **light blue** in the header, separate from the text filter.

### Status Filter

The patent list defaults to showing only **stored** patents. The active status is always visible in the header (`status:stored`).

Show patents by status:

```
:statusfilter stored      ← default
:statusfilter ignored
:statusfilter under-review
:statusfilter all         ← show everything except cached imports
```

Invalid status values produce an error.

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
