// Package sqlite is the SQLite-backed implementation of store.Repository.
package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver

	"patentmine/internal/store"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const (
	driverName     = "sqlite"
	maxReaderConns = 4
)

// Repo is the SQLite-backed store.Repository. It keeps two handles to the same
// file: writer is capped at a single connection so every write serializes
// (SQLite permits only one writer), while reader allows a small pool so
// listings do not queue behind a write. WAL mode lets readers and the writer
// proceed concurrently.
type Repo struct {
	writer *sql.DB
	reader *sql.DB
}

// compile-time assertion that Repo satisfies the interface.
var _ store.Repository = (*Repo)(nil)

// dsn builds a connection string with the pragmas every connection needs.
func dsn(path string) string {
	return fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)",
		path,
	)
}

// Open opens the database at path (creating it if absent) and applies any
// pending migrations.
func Open(ctx context.Context, path string) (*Repo, error) {
	writer, err := sql.Open(driverName, dsn(path))
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: open writer: %w", err)
	}
	writer.SetMaxOpenConns(1)

	reader, err := sql.Open(driverName, dsn(path))
	if err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("store/sqlite: open reader: %w", err)
	}
	reader.SetMaxOpenConns(maxReaderConns)

	r := &Repo{writer: writer, reader: reader}
	if err := r.migrate(ctx); err != nil {
		_ = r.Close()
		return nil, err
	}
	return r, nil
}

// Close releases both connection pools.
func (r *Repo) Close() error {
	werr := r.writer.Close()
	rerr := r.reader.Close()
	if werr != nil {
		return werr
	}
	return rerr
}

// migrate applies every embedded migration file not yet recorded, in name
// order, each inside its own transaction.
func (r *Repo) migrate(ctx context.Context) error {
	if _, err := r.writer.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migration (
			version    TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`); err != nil {
		return fmt.Errorf("store/sqlite: create migration table: %w", err)
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("store/sqlite: read migrations: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		var applied int
		if err := r.writer.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migration WHERE version = ?`, name,
		).Scan(&applied); err != nil {
			return fmt.Errorf("store/sqlite: check migration %s: %w", name, err)
		}
		if applied > 0 {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("store/sqlite: read migration %s: %w", name, err)
		}
		if err := r.applyMigration(ctx, name, string(body)); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) applyMigration(ctx context.Context, name, body string) error {
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: begin migration %s: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, body); err != nil {
		return fmt.Errorf("store/sqlite: run migration %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migration (version, applied_at) VALUES (?, ?)`,
		name, encodeTime(time.Now()),
	); err != nil {
		return fmt.Errorf("store/sqlite: record migration %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: commit migration %s: %w", name, err)
	}
	return nil
}

// encodeTime renders a time as RFC3339 UTC, or "" for the zero value.
func encodeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// decodeTime parses a value written by encodeTime.
func decodeTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}
