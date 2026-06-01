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
		ids_in_full           INTEGER NOT NULL DEFAULT 0,
		ids_relevant_passages TEXT NOT NULL DEFAULT '',
		ids_notes             TEXT NOT NULL DEFAULT '',
		ids_status            TEXT NOT NULL DEFAULT '',
		ids_added_at          TEXT NOT NULL DEFAULT '',
		ids_submitted_at      TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (project_id, patent_number)
	)`,
	// Every existing membership, decorated with its curated IDS data when one
	// exists in project_ids. ids_submitted_at is backfilled to added_at when
	// the legacy entry was already submitted, so we keep a plausible timestamp.
	`INSERT INTO membership_new (project_id, patent_number, state, added_at,
		ids_kind_code, ids_in_full, ids_relevant_passages,
		ids_notes, ids_status, ids_added_at, ids_submitted_at)
	 SELECT m.project_id, m.patent_number, m.state, m.added_at,
		COALESCE(pid.kind_code, ''),
		COALESCE(pid.in_full, 0), COALESCE(pid.relevant_passages, ''),
		COALESCE(pid.notes, ''), COALESCE(pid.status, ''), COALESCE(pid.added_at, ''),
		CASE WHEN COALESCE(pid.status, '') = 'submitted'
		     THEN COALESCE(pid.added_at, '') ELSE '' END
	 FROM membership m
	 LEFT JOIN project_ids pid
		ON pid.project_id = m.project_id AND pid.patent_number = m.patent_number`,
	// IDS entries whose patent was never a project member: synthesize a
	// membership row for them so no curated data is lost.
	`INSERT INTO membership_new (project_id, patent_number, state, added_at,
		ids_kind_code, ids_in_full, ids_relevant_passages,
		ids_notes, ids_status, ids_added_at, ids_submitted_at)
	 SELECT pid.project_id, pid.patent_number, 'unknown', pid.added_at,
		pid.kind_code, pid.in_full, pid.relevant_passages,
		pid.notes, pid.status, pid.added_at,
		CASE WHEN pid.status = 'submitted' THEN pid.added_at ELSE '' END
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

// migrate runs the version-chain structural migrations BEFORE schema.sql is
// (re)applied, so a rename-based upgrade is not blocked by schema.sql having
// already created the new-shape tables. A fresh database (no schema_meta yet)
// is left for schema.sql to create at the current version.
func (r *Repo) migrate(ctx context.Context) error {
	hasMeta, err := r.tableExists(ctx, "schema_meta")
	if err != nil {
		return fmt.Errorf("store/sqlite: migrate: detect schema_meta: %w", err)
	}
	if !hasMeta {
		return nil // brand-new database; schema.sql writes the current version
	}
	var version string
	if err := r.writer.QueryRowContext(ctx,
		`SELECT value FROM schema_meta WHERE key = 'schema_version'`).Scan(&version); err != nil {
		return fmt.Errorf("store/sqlite: migrate: read version: %w", err)
	}
	if version == "2" {
		if err := r.migrateV2ToV3(ctx); err != nil {
			return fmt.Errorf("store/sqlite: migrate v2 to v3: %w", err)
		}
		version = "3"
	}
	if version == "3" {
		if err := r.migrateV3ToV4(ctx); err != nil {
			return fmt.Errorf("store/sqlite: migrate v3 to v4: %w", err)
		}
		version = "4"
	}
	if version == "4" {
		if err := r.migrateV4ToV5(ctx); err != nil {
			return fmt.Errorf("store/sqlite: migrate v4 to v5: %w", err)
		}
		version = "5"
	}
	if version != "5" {
		return fmt.Errorf("store/sqlite: unsupported schema version %q; expected 5", version)
	}
	return nil
}

// migrateV4ToV5 repairs grant documents that were stored without their kind
// code. The lighter crawl path recorded the grant number digits-only (e.g.
// "US09658068") while the authoritative kind ("B2") was kept only in
// uspto_application.grant_kind. Because :export.added derives each patent's
// Google-linkable number from its grant document, those records exported a
// kind-less number Google Patents will not serve. This backfill stamps the kind
// onto the document (and the record's display number) from the stored
// grant_kind, joining through the surrogate record id. It is data-only and
// idempotent: rows already carrying a kind are left untouched.
func (r *Repo) migrateV4ToV5(ctx context.Context) error {
	if err := r.Backup(ctx, r.path+".v4-to-v5.bak"); err != nil {
		return fmt.Errorf("store/sqlite: migrate v4 to v5: backup: %w", err)
	}
	// uspto_application holds the authoritative grant_kind. When an earlier
	// migration in the same chain (v3→v4) has dropped it for schema.sql to
	// recreate empty, there is nothing to backfill — just advance the version.
	hasUSPTO, err := r.tableExists(ctx, "uspto_application")
	if err != nil {
		return fmt.Errorf("store/sqlite: migrate v4 to v5: detect uspto_application: %w", err)
	}

	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: migrate v4 to v5: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if hasUSPTO {
		for _, stmt := range migrationV4ToV5Statements {
			if _, err := tx.ExecContext(ctx, stmt); err != nil {
				return fmt.Errorf("store/sqlite: migrate v4 to v5: %w", err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE schema_meta SET value = '5' WHERE key = 'schema_version'`); err != nil {
		return fmt.Errorf("store/sqlite: migrate v4 to v5: set version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: migrate v4 to v5: commit: %w", err)
	}
	return nil
}

// migrationV4ToV5Statements backfill grant-document kind codes from
// uspto_application.grant_kind. The join goes document.record_number →
// record.number → record.id → uspto_application.record_id, matching how the
// source-derived tables key off the stable surrogate id.
var migrationV4ToV5Statements = []string{
	// A record may already carry a properly kinded grant document (from the grant
	// XML path) alongside a stale kind-less one from the lighter crawl. Drop the
	// kind-less duplicate first so the record resolves to a single grant document.
	`DELETE FROM document
	 WHERE stage = 'grant' AND kind = ''
	   AND EXISTS (
	       SELECT 1 FROM document d2
	       WHERE d2.record_number = document.record_number
	         AND d2.stage = 'grant' AND d2.kind != ''
	   )`,
	// Stamp the authoritative grant kind onto the remaining kind-less grant
	// documents. Only the kind column is touched here: Documents() rebuilds a
	// document's number from country/serial/kind, so this alone makes the exported
	// Google number carry the kind, and it cannot collide with the document
	// number's UNIQUE/primary key (the same grant document may exist under a
	// divergent doc-centric record — see record-number identity divergence).
	`UPDATE document
	 SET kind = (
	         SELECT ua.grant_kind FROM uspto_application ua
	         JOIN record r ON r.id = ua.record_id
	         WHERE r.number = document.record_number AND ua.grant_kind != ''
	     )
	 WHERE stage = 'grant' AND kind = ''
	   AND EXISTS (
	       SELECT 1 FROM uspto_application ua
	       JOIN record r ON r.id = ua.record_id
	       WHERE r.number = document.record_number AND ua.grant_kind != ''
	   )`,
	// Bring the canonical number column in line with the now-stamped kind, but
	// only when the kinded form is not already taken by another document row, so a
	// divergent record's grant document never triggers a PK collision. Where it
	// would collide the kind column stays authoritative for the export.
	`UPDATE document
	 SET number = country || serial || kind
	 WHERE stage = 'grant' AND kind != '' AND number = country || serial
	   AND NOT EXISTS (
	       SELECT 1 FROM document d2
	       WHERE d2.number = document.country || document.serial || document.kind
	   )`,
	// Keep the record's display number aligned with its now-kinded grant document
	// so the TUI and links show the Google-resolvable number, but only when the
	// stored display number is the kind-less grant number we just corrected.
	`UPDATE record
	 SET display_number = (
	         SELECT d.country || d.serial || d.kind FROM document d
	         WHERE d.record_number = record.number AND d.stage = 'grant' AND d.kind != ''
	         ORDER BY d.number LIMIT 1
	     )
	 WHERE EXISTS (
	         SELECT 1 FROM document d
	         WHERE d.record_number = record.number AND d.stage = 'grant'
	           AND d.kind != '' AND record.display_number = d.country || d.serial
	     )`,
}

// migrateV3ToV4 introduces the surrogate record id. It is rename-based and
// data-preserving for the corpus and user data: `patent` is renamed to `record`
// with a stable `id` (UUID-like) added; its canonical `number` column keeps its
// name, and SQLite rewrites the child foreign keys
// (document/relation/membership/…) to reference record(number) automatically.
// The re-fetchable source/derived tables (uspto_*, source_*,
// authority_identifier) are dropped so schema.sql recreates them at the new
// record_id shape. Runs with foreign keys disabled (rename + drop), after a
// backup, in one transaction.
func (r *Repo) migrateV3ToV4(ctx context.Context) error {
	if err := r.backupBeforeMigration(ctx); err != nil {
		return fmt.Errorf("store/sqlite: migrate v3 to v4: backup: %w", err)
	}
	conn, err := r.writer.Conn(ctx)
	if err != nil {
		return fmt.Errorf("store/sqlite: migrate v3 to v4: connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("store/sqlite: migrate v3 to v4: disable fk: %w", err)
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: migrate v3 to v4: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range migrationV3ToV4Statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("store/sqlite: migrate v3 to v4: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: migrate v3 to v4: commit: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("store/sqlite: migrate v3 to v4: re-enable fk: %w", err)
	}
	return nil
}

// migrationV3ToV4Statements: drop the old FTS first (so the patent→record rename
// does not rewrite its triggers), drop the re-fetchable source/derived tables,
// then rename patent→record adding the surrogate id. schema.sql recreates the
// dropped tables (and record_fts) at the new shape; syncFTS repopulates the
// index.
var migrationV3ToV4Statements = []string{
	`DROP TRIGGER IF EXISTS patent_fts_insert`,
	`DROP TRIGGER IF EXISTS patent_fts_delete`,
	`DROP TRIGGER IF EXISTS patent_fts_update`,
	`DROP TABLE IF EXISTS patent_fts`,

	`DROP TABLE IF EXISTS uspto_assignment_party`,
	`DROP TABLE IF EXISTS uspto_assignment`,
	`DROP TABLE IF EXISTS uspto_party`,
	`DROP TABLE IF EXISTS uspto_event`,
	`DROP TABLE IF EXISTS uspto_continuity`,
	`DROP TABLE IF EXISTS uspto_foreign_priority`,
	`DROP TABLE IF EXISTS uspto_grant_body`,
	`DROP TABLE IF EXISTS uspto_drawing`,
	`DROP TABLE IF EXISTS uspto_grant_citation`,
	`DROP TABLE IF EXISTS uspto_grant_classification`,
	`DROP TABLE IF EXISTS uspto_grant_relation`,
	`DROP TABLE IF EXISTS uspto_grant_party`,
	`DROP TABLE IF EXISTS uspto_xml_download`,
	`DROP TABLE IF EXISTS uspto_document`,
	`DROP TABLE IF EXISTS uspto_application`,
	`DROP TABLE IF EXISTS authority_identifier`,
	`DROP TABLE IF EXISTS source_snapshot`,
	`DROP TABLE IF EXISTS source_diff`,

	// Add the surrogate id, then rename patent→record. SQLite rewrites the child
	// FK references (document/relation/membership/membership_provenance/
	// project_patent_note/patent_tag/mutation_item) to record(number). The
	// canonical `number` column keeps its name, so no child data changes.
	`ALTER TABLE patent ADD COLUMN id TEXT NOT NULL DEFAULT ''`,
	`UPDATE patent SET id = lower(hex(randomblob(16))) WHERE id = ''`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_record_id ON patent (id)`,
	`ALTER TABLE patent RENAME TO record`,

	`UPDATE schema_meta SET value = '4' WHERE key = 'schema_version'`,
}

// migrateV2ToV3 upgrades a v2 database to v3 by creating the membership_provenance
// table and backfilling existing memberships.
func (r *Repo) migrateV2ToV3(ctx context.Context) error {
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Create table if it didn't get created yet
	_, err = tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS membership_provenance (
			project_id            TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
			patent_number         TEXT NOT NULL REFERENCES patent (number) ON DELETE CASCADE,
			added_method          TEXT NOT NULL,
			parent_patent_number  TEXT,
			source_provider       TEXT NOT NULL DEFAULT '',
			source_mode           TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (project_id, patent_number),
			FOREIGN KEY (project_id, patent_number) REFERENCES membership (project_id, patent_number) ON DELETE CASCADE ON UPDATE CASCADE
		)`)
	if err != nil {
		return fmt.Errorf("create provenance table: %w", err)
	}

	// Backfill existing memberships as 'direct'
	_, err = tx.ExecContext(ctx, `
		INSERT INTO membership_provenance (project_id, patent_number, added_method)
		SELECT project_id, patent_number, 'direct'
		FROM membership
		ON CONFLICT(project_id, patent_number) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("backfill provenance: %w", err)
	}

	// Update schema version
	_, err = tx.ExecContext(ctx, `
		UPDATE schema_meta
		SET value = '3'
		WHERE key = 'schema_version'`)
	if err != nil {
		return fmt.Errorf("update schema version: %w", err)
	}

	return tx.Commit()
}
