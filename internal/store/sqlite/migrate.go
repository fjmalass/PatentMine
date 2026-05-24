package sqlite

import (
	"context"
	"fmt"
)

// migrateF folds the legacy project_ids table into membership (as inline ids_*
// columns) and rebuilds relation and membership with ON DELETE CASCADE foreign
// keys. It is a one-way migration: the presence of the project_ids table is the
// signal that it has not run yet, so a database that has already been migrated
// — or was created fresh with the current schema — is left untouched.
//
// Because the migration drops and recreates tables, the database is first
// copied to "<path>.bak" with VACUUM INTO, giving the user a clean restore
// point if anything goes wrong.
func (r *Repo) migrateF(ctx context.Context) error {
	needed, err := r.needsMigrationF(ctx)
	if err != nil {
		return err
	}
	if !needed {
		return nil
	}
	if err := r.backupBeforeMigration(ctx); err != nil {
		return fmt.Errorf("store/sqlite: migrate F: backup: %w", err)
	}
	return r.runMigrationF(ctx)
}

// needsMigrationF reports whether the legacy project_ids table is still present.
func (r *Repo) needsMigrationF(ctx context.Context) (bool, error) {
	var n int
	if err := r.writer.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'project_ids'`).
		Scan(&n); err != nil {
		return false, fmt.Errorf("store/sqlite: migrate F: detect: %w", err)
	}
	return n > 0, nil
}

// backupBeforeMigration writes a clean copy of the database to "<path>.bak".
// An in-memory database (used by tests) has no on-disk path and is skipped.
func (r *Repo) backupBeforeMigration(ctx context.Context) error {
	return r.Backup(ctx, r.path+".bak")
}

// runMigrationF performs the table rebuild on a dedicated connection with
// foreign-key enforcement disabled (SQLite cannot rebuild a referenced table
// with it on), inside a single transaction.
func (r *Repo) runMigrationF(ctx context.Context) error {
	conn, err := r.writer.Conn(ctx)
	if err != nil {
		return fmt.Errorf("store/sqlite: migrate F: connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("store/sqlite: migrate F: disable fk: %w", err)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: migrate F: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range migrationFStatements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("store/sqlite: migrate F: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: migrate F: commit: %w", err)
	}

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("store/sqlite: migrate F: re-enable fk: %w", err)
	}
	return nil
}

// migrationFStatements rebuilds membership (merging in project_ids) and
// relation (adding cascade foreign keys), then drops project_ids.
var migrationFStatements = []string{
	`CREATE TABLE membership_new (
		project_id            TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
		patent_number         TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
		state                 TEXT NOT NULL,
		added_at              TEXT NOT NULL,
		ids_kind_code         TEXT NOT NULL DEFAULT '',
		ids_country_code      TEXT NOT NULL DEFAULT '',
		ids_in_full           INTEGER NOT NULL DEFAULT 0,
		ids_relevant_passages TEXT NOT NULL DEFAULT '',
		ids_notes             TEXT NOT NULL DEFAULT '',
		ids_status            TEXT NOT NULL DEFAULT '',
		ids_added_at          TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (project_id, patent_number)
	)`,
	// Every existing membership, decorated with its curated IDS data when one
	// exists in project_ids.
	`INSERT INTO membership_new (project_id, patent_number, state, added_at,
		ids_kind_code, ids_country_code, ids_in_full, ids_relevant_passages,
		ids_notes, ids_status, ids_added_at)
	 SELECT m.project_id, m.patent_number, m.state, m.added_at,
		COALESCE(pid.kind_code, ''), COALESCE(pid.country_code, ''),
		COALESCE(pid.in_full, 0), COALESCE(pid.relevant_passages, ''),
		COALESCE(pid.notes, ''), COALESCE(pid.status, ''), COALESCE(pid.added_at, '')
	 FROM membership m
	 LEFT JOIN project_ids pid
		ON pid.project_id = m.project_id AND pid.patent_number = m.patent_number`,
	// IDS entries whose patent was never a project member: synthesize a
	// membership row for them so no curated data is lost.
	`INSERT INTO membership_new (project_id, patent_number, state, added_at,
		ids_kind_code, ids_country_code, ids_in_full, ids_relevant_passages,
		ids_notes, ids_status, ids_added_at)
	 SELECT pid.project_id, pid.patent_number, 'unknown', pid.added_at,
		pid.kind_code, pid.country_code, pid.in_full, pid.relevant_passages,
		pid.notes, pid.status, pid.added_at
	 FROM project_ids pid
	 WHERE NOT EXISTS (
		SELECT 1 FROM membership m
		WHERE m.project_id = pid.project_id AND m.patent_number = pid.patent_number
	 )`,
	`DROP TABLE membership`,
	`ALTER TABLE membership_new RENAME TO membership`,
	`CREATE INDEX idx_membership_project ON membership (project_id, state)`,

	`CREATE TABLE relation_new (
		from_number TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
		to_number   TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
		kind        TEXT NOT NULL,
		PRIMARY KEY (from_number, to_number, kind)
	)`,
	// Copy edges, dropping any whose endpoints are not real patents — those
	// would violate the new foreign keys.
	`INSERT INTO relation_new (from_number, to_number, kind)
	 SELECT from_number, to_number, kind FROM relation
	 WHERE from_number IN (SELECT number FROM patent)
	   AND to_number IN (SELECT number FROM patent)`,
	`DROP TABLE relation`,
	`ALTER TABLE relation_new RENAME TO relation`,
	`CREATE INDEX idx_relation_from ON relation (from_number, kind)`,
	`CREATE INDEX idx_relation_to ON relation (to_number, kind)`,

	`DROP TABLE project_ids`,
}
