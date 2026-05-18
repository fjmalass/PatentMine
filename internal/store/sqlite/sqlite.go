// Package sqlite is the SQLite-backed implementation of store.Repository.
package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver

	"patentmine/internal/observability"
	"patentmine/internal/store"
)

//go:embed schema.sql
var schemaSQL string

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
	metrics *observability.Metrics
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
	return OpenWithMetrics(ctx, path, nil)
}

// OpenWithMetrics opens the database and wires in optional in-process metrics.
func OpenWithMetrics(ctx context.Context, path string, metrics *observability.Metrics) (*Repo, error) {
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

	r := &Repo{writer: writer, reader: reader, metrics: metrics}
	if err := r.initSchema(ctx); err != nil {
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

func (r *Repo) initSchema(ctx context.Context) error {
	if _, err := r.writer.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("store/sqlite: init schema: %w", err)
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

func (r *Repo) observeDuration(name string, start time.Time, errp *error) {
	if r.metrics == nil {
		return
	}
	failed := errp != nil && *errp != nil
	r.metrics.ObserveDuration("store.sqlite."+name, time.Since(start), failed)
}
