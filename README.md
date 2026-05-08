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

PatentMine writes logs to a dated file. By default, `./patentmine.log` becomes `./patentmine-YYYY-MM-DD.log`, for example `./patentmine-2026-08-19.log`. Runs on the same date append to the same file.

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

## Task Reference

- `makers run`: launch the PatentMine TUI
- `makers cli-help`: show PatentMine CLI flags
- `makers test`: run all Go tests
- `makers fmt`: format Go files
- `makers build`: build `./patentmine`
- `makers check`: run `fmt`, `test`, and `build`
- `makers tidy`: resolve Go module dependencies
- `makers logs`: print the last 80 TUI log lines
- `makers reset-db`: remove the local SQLite database files
- `makers backup`: create a timestamped backup
- `makers backup-log`: create a dated backup of the current log
- `makers list-backups`: list existing backups
- `makers clean`: remove the built binary
