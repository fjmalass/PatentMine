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
	if _, err := r.writer.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("store/sqlite: init schema: %w", err)
	}
	// Fold the legacy project_ids table into membership and add FK cascades.
	if err := r.migrateF(ctx); err != nil {
		return err
	}
	// Migrate existing database instances if they lack the created_at column in patent_tag.
	hasCreatedAt, err := r.columnExists(ctx, "patent_tag", "created_at")
	if err == nil && !hasCreatedAt {
		_, _ = r.writer.ExecContext(ctx, "ALTER TABLE patent_tag ADD COLUMN created_at TEXT NOT NULL DEFAULT ''")
	}
	// Migrate existing database instances if they lack the classifications column in patent.
	hasClassifications, err := r.columnExists(ctx, "patent", "classifications")
	if err == nil && !hasClassifications {
		_, _ = r.writer.ExecContext(ctx, "ALTER TABLE patent ADD COLUMN classifications TEXT NOT NULL DEFAULT '[]'")
	}
	if err := r.ensureClassificationsFTSColumn(ctx); err != nil {
		return err
	}
	// Migrate existing database instances if they lack the classification_definition table.
	hasClassDef, err := r.tableExists(ctx, "classification_definition")
	if err == nil && !hasClassDef {
		_, _ = r.writer.ExecContext(ctx, `
			CREATE TABLE classification_definition (
				system      TEXT NOT NULL,
				code        TEXT NOT NULL,
				section     TEXT NOT NULL DEFAULT '',
				class       TEXT NOT NULL DEFAULT '',
				subclass    TEXT NOT NULL DEFAULT '',
				main_group  TEXT NOT NULL DEFAULT '',
				subgroup    TEXT NOT NULL DEFAULT '',
				description TEXT NOT NULL DEFAULT '',
				PRIMARY KEY (system, code)
			)
		`)
	}
	if err := r.syncFTS(ctx); err != nil {
		return err
	}
	hasPatentNotes, err := r.tableExists(ctx, "project_patent_note")
	if err == nil && !hasPatentNotes {
		_, _ = r.writer.ExecContext(ctx, `
			CREATE TABLE project_patent_note (
				project_id    TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
				patent_number TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
				markdown      TEXT NOT NULL DEFAULT '',
				added_at      TEXT NOT NULL,
				updated_at    TEXT NOT NULL,
				PRIMARY KEY (project_id, patent_number)
			)
		`)
		_, _ = r.writer.ExecContext(ctx,
			`CREATE INDEX IF NOT EXISTS idx_project_patent_note_project ON project_patent_note (project_id, updated_at DESC)`)
	}
	return nil
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
