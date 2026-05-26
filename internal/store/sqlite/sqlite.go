// Package sqlite is the SQLite-backed implementation of store.Repository.
package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strings"
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
	writer  *sql.DB
	reader  *sql.DB
	path    string
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

	r := &Repo{writer: writer, reader: reader, path: path, metrics: metrics}
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
	if err := r.rejectObsoleteSchema(ctx); err != nil {
		return err
	}
	if _, err := r.writer.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("store/sqlite: init schema: %w", err)
	}
	if err := r.requireSchemaVersion(ctx); err != nil {
		return err
	}
	if err := r.ensureClassificationsFTSColumn(ctx); err != nil {
		return err
	}
	if err := r.syncFTS(ctx); err != nil {
		return err
	}

	// Best-effort: ensure the Option A reconciliation columns exist on
	// source_diff for databases created before the columns were added to
	// schema.sql. SQLite ALTER ADD COLUMN is safe and idempotent in effect.
	// We swallow "duplicate column" errors so old DBs start cleanly.
	_ = r.ensureReconciledColumns(ctx)

	return nil
}

// ensureReconciledColumns adds the reconciled_* columns (Option A) if the
// source_diff table exists but the columns are missing. Non-fatal.
func (r *Repo) ensureReconciledColumns(ctx context.Context) error {
	// Only relevant if the table was created by an older schema.sql.
	// We don't check existence of table here (the main schema CREATE already ran).
	stmts := []string{
		`ALTER TABLE source_diff ADD COLUMN reconciled_at TEXT`,
		`ALTER TABLE source_diff ADD COLUMN reconciled_by TEXT`,
		`ALTER TABLE source_diff ADD COLUMN reconciled_choice TEXT`,
	}
	for _, stmt := range stmts {
		if _, err := r.writer.ExecContext(ctx, stmt); err != nil {
			msg := err.Error()
			if !strings.Contains(msg, "duplicate column") && !strings.Contains(msg, "already exists") {
				// Unexpected; surface for diagnostics but do not fail startup.
				return fmt.Errorf("ensure reconciled columns: %w", err)
			}
		}
	}
	return nil
}

func (r *Repo) rejectObsoleteSchema(ctx context.Context) error {
	hasMeta, err := r.tableExists(ctx, "schema_meta")
	if err != nil {
		return fmt.Errorf("store/sqlite: inspect schema attrs: %w", err)
	}
	if hasMeta {
		return nil
	}
	hasTables, err := r.hasApplicationTables(ctx)
	if err != nil {
		return err
	}
	if hasTables {
		return errors.New("store/sqlite: obsolete database schema detected; recreate the database or export/rebuild for schema v2")
	}
	return nil
}

func (r *Repo) requireSchemaVersion(ctx context.Context) error {
	var version string
	if err := r.writer.QueryRowContext(ctx,
		`SELECT value FROM schema_meta WHERE key = 'schema_version'`).Scan(&version); err != nil {
		return fmt.Errorf("store/sqlite: read schema version: %w", err)
	}
	if version != "2" {
		return fmt.Errorf("store/sqlite: unsupported schema version %q; expected 2", version)
	}
	return nil
}

func (r *Repo) hasApplicationTables(ctx context.Context) (bool, error) {
	var count int
	if err := r.writer.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_master
		WHERE type IN ('table', 'view')
		  AND name NOT LIKE 'sqlite_%'
		  AND name != 'schema_meta'`).Scan(&count); err != nil {
		return false, fmt.Errorf("store/sqlite: inspect existing schema: %w", err)
	}
	return count > 0, nil
}

func (r *Repo) tableExists(ctx context.Context, name string) (bool, error) {
	var count int
	if err := r.writer.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *Repo) columnExists(ctx context.Context, table, column string) (bool, error) {
	rows, err := r.writer.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name, typeStr string
		var notnull, pk int
		var dfltVal any
		if err := rows.Scan(&cid, &name, &typeStr, &notnull, &dfltVal, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

// ensureClassificationsFTSColumn makes sure the patent table has a
// classifications_text column and that patent_fts indexes it. SQLite cannot
// add a column to an existing external-content FTS5 table in place, so we
// drop the FTS table along with its triggers and recreate them with the
// extra column. The patent rows are then backfilled and the FTS index is
// rebuilt by syncFTS.
func (r *Repo) ensureClassificationsFTSColumn(ctx context.Context) error {
	hasCol, err := r.columnExists(ctx, "patent", "classifications_text")
	if err != nil {
		return fmt.Errorf("store/sqlite: detect classifications_text: %w", err)
	}
	if !hasCol {
		if _, err := r.writer.ExecContext(ctx,
			`ALTER TABLE patent ADD COLUMN classifications_text TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("store/sqlite: add classifications_text: %w", err)
		}
	}
	// Backfill classifications_text from the JSON column. Idempotent — only
	// rewrites rows that don't already match the derived value.
	if _, err := r.writer.ExecContext(ctx, `
		UPDATE patent
		SET classifications_text = COALESCE((
			SELECT GROUP_CONCAT(REPLACE(value, '/', ' '), ' ')
			FROM json_each(patent.classifications)
		), '')
		WHERE classifications_text != COALESCE((
			SELECT GROUP_CONCAT(REPLACE(value, '/', ' '), ' ')
			FROM json_each(patent.classifications)
		), '')`); err != nil {
		return fmt.Errorf("store/sqlite: backfill classifications_text: %w", err)
	}

	// Is the FTS already indexing classifications_text?
	var ftsCols string
	if err := r.writer.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='patent_fts'`).Scan(&ftsCols); err != nil {
		return fmt.Errorf("store/sqlite: inspect patent_fts: %w", err)
	}
	if !sqlContainsClassificationsText(ftsCols) {
		// Drop the FTS table and its content-sync triggers, then recreate them
		// with the additional column. The rebuild below repopulates content.
		stmts := []string{
			`DROP TRIGGER IF EXISTS patent_fts_insert`,
			`DROP TRIGGER IF EXISTS patent_fts_delete`,
			`DROP TRIGGER IF EXISTS patent_fts_update`,
			`DROP TABLE IF EXISTS patent_fts`,
			`CREATE VIRTUAL TABLE patent_fts USING fts5 (
				title, abstract, classifications_text,
				content='patent', content_rowid='rowid')`,
			`CREATE TRIGGER patent_fts_insert AFTER INSERT ON patent BEGIN
				INSERT INTO patent_fts (rowid, title, abstract, classifications_text)
				VALUES (new.rowid, new.title, new.abstract, new.classifications_text);
			END`,
			`CREATE TRIGGER patent_fts_delete AFTER DELETE ON patent BEGIN
				INSERT INTO patent_fts (patent_fts, rowid, title, abstract, classifications_text)
				VALUES ('delete', old.rowid, old.title, old.abstract, old.classifications_text);
			END`,
			`CREATE TRIGGER patent_fts_update AFTER UPDATE ON patent BEGIN
				INSERT INTO patent_fts (patent_fts, rowid, title, abstract, classifications_text)
				VALUES ('delete', old.rowid, old.title, old.abstract, old.classifications_text);
				INSERT INTO patent_fts (rowid, title, abstract, classifications_text)
				VALUES (new.rowid, new.title, new.abstract, new.classifications_text);
			END`,
			`INSERT INTO patent_fts(patent_fts) VALUES('rebuild')`,
		}
		for _, s := range stmts {
			if _, err := r.writer.ExecContext(ctx, s); err != nil {
				return fmt.Errorf("store/sqlite: rebuild patent_fts with classifications_text: %w", err)
			}
		}
	}
	return nil
}

func sqlContainsClassificationsText(sql string) bool {
	for i := 0; i+len("classifications_text") <= len(sql); i++ {
		if sql[i:i+len("classifications_text")] == "classifications_text" {
			return true
		}
	}
	return false
}

// syncFTS rebuilds the patent_fts full-text index from the patent table when
// the two are out of step — typically on a database created before the index
// existed. An already-consistent index is left untouched.
func (r *Repo) syncFTS(ctx context.Context) error {
	var patentCount, ftsCount int
	if err := r.writer.QueryRowContext(ctx, `SELECT count(*) FROM patent`).Scan(&patentCount); err != nil {
		return fmt.Errorf("store/sqlite: count patents: %w", err)
	}
	if err := r.writer.QueryRowContext(ctx, `SELECT count(*) FROM patent_fts`).Scan(&ftsCount); err != nil {
		return fmt.Errorf("store/sqlite: count patent_fts: %w", err)
	}
	if patentCount == ftsCount {
		return nil
	}
	if _, err := r.writer.ExecContext(ctx, `INSERT INTO patent_fts(patent_fts) VALUES('rebuild')`); err != nil {
		return fmt.Errorf("store/sqlite: rebuild patent_fts: %w", err)
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
