package sqlite

import (
	"context"
	"fmt"
	"strings"
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
	if version == "5" {
		if err := r.migrateV5ToV6(ctx); err != nil {
			return fmt.Errorf("store/sqlite: migrate v5 to v6: %w", err)
		}
		version = "6"
	}
	if version == "6" {
		if err := r.migrateV6ToV7(ctx); err != nil {
			return fmt.Errorf("store/sqlite: migrate v6 to v7: %w", err)
		}
		version = "7"
	}
	if version == "7" {
		if err := r.migrateV7ToV8(ctx); err != nil {
			return fmt.Errorf("store/sqlite: migrate v7 to v8: %w", err)
		}
		version = "8"
	}
	if version == "8" {
		if err := r.migrateV8ToV9(ctx); err != nil {
			return fmt.Errorf("store/sqlite: migrate v8 to v9: %w", err)
		}
		version = "9"
	}
	if version == "9" {
		if err := r.migrateV9ToV10(ctx); err != nil {
			return fmt.Errorf("store/sqlite: migrate v9 to v10: %w", err)
		}
		version = "10"
	}
	if version == "10" {
		if err := r.migrateV10ToV11(ctx); err != nil {
			return fmt.Errorf("store/sqlite: migrate v10 to v11: %w", err)
		}
		version = "11"
	}
	if version != "11" {
		return fmt.Errorf("store/sqlite: unsupported schema version %q; expected 11", version)
	}
	return nil
}

// migrateV6ToV7 introduces the drafting subsystem tables (office_action, draft,
// draft_section, draft_claim). It is purely additive — the tables are created
// here so the version bump is self-contained, and schema.sql recreates them
// idempotently on the following apply. No data backfill is needed.
func (r *Repo) migrateV6ToV7(ctx context.Context) error {
	if err := r.Backup(ctx, r.path+".v6-to-v7.bak"); err != nil {
		return fmt.Errorf("store/sqlite: migrate v6 to v7: backup: %w", err)
	}
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: migrate v6 to v7: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range migrationV6ToV7Statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("store/sqlite: migrate v6 to v7: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE schema_meta SET value = '7' WHERE key = 'schema_version'`); err != nil {
		return fmt.Errorf("store/sqlite: migrate v6 to v7: set version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: migrate v6 to v7: commit: %w", err)
	}
	return nil
}

// migrationV6ToV7Statements create the drafting tables and indexes. Kept in sync
// with schema.sql (idempotent CREATE ... IF NOT EXISTS).
var migrationV6ToV7Statements = []string{
	`CREATE TABLE IF NOT EXISTS office_action (
	    id                 TEXT PRIMARY KEY,
	    project_id         TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
	    application_number TEXT NOT NULL DEFAULT '',
	    mail_date          TEXT NOT NULL DEFAULT '',
	    oa_type            TEXT NOT NULL DEFAULT '',
	    examiner           TEXT NOT NULL DEFAULT '',
	    art_unit           TEXT NOT NULL DEFAULT '',
	    blob_path          TEXT NOT NULL DEFAULT '',
	    blob_hash          TEXT NOT NULL DEFAULT '',
	    extracted_text     TEXT NOT NULL DEFAULT '',
	    notes              TEXT NOT NULL DEFAULT '',
	    source             TEXT NOT NULL DEFAULT '',
	    imported_at        TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_office_action_project ON office_action (project_id, mail_date DESC)`,
	`CREATE TABLE IF NOT EXISTS draft (
	    id                TEXT PRIMARY KEY,
	    project_id        TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
	    kind              TEXT NOT NULL,
	    title             TEXT NOT NULL DEFAULT '',
	    status            TEXT NOT NULL DEFAULT 'draft',
	    office_action_id  TEXT REFERENCES office_action (id) ON DELETE SET NULL,
	    created_at        TEXT NOT NULL,
	    updated_at        TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_draft_project ON draft (project_id, updated_at DESC)`,
	`CREATE TABLE IF NOT EXISTS draft_section (
	    draft_id     TEXT NOT NULL REFERENCES draft (id) ON DELETE CASCADE,
	    ordinal      INTEGER NOT NULL,
	    kind         TEXT NOT NULL DEFAULT '',
	    heading      TEXT NOT NULL DEFAULT '',
	    body         TEXT NOT NULL DEFAULT '',
	    pinned_json  TEXT NOT NULL DEFAULT '[]',
	    ai_provider  TEXT NOT NULL DEFAULT '',
	    ai_model     TEXT NOT NULL DEFAULT '',
	    generated_at TEXT NOT NULL DEFAULT '',
	    human_edited INTEGER NOT NULL DEFAULT 0,
	    PRIMARY KEY (draft_id, ordinal)
	)`,
	`CREATE TABLE IF NOT EXISTS draft_claim (
	    draft_id   TEXT NOT NULL REFERENCES draft (id) ON DELETE CASCADE,
	    ordinal    INTEGER NOT NULL,
	    number     INTEGER NOT NULL DEFAULT 0,
	    claim_type TEXT NOT NULL DEFAULT '',
	    depends_on INTEGER NOT NULL DEFAULT 0,
	    status     TEXT NOT NULL DEFAULT 'original',
	    base_text  TEXT NOT NULL DEFAULT '',
	    text       TEXT NOT NULL DEFAULT '',
	    PRIMARY KEY (draft_id, ordinal)
	)`,
}

// migrateV7ToV8 grows the office-action subsystem into a prosecution-matter
// workspace. It is purely additive: a matter-scoped document table (with a
// backfill of the existing single office-action blob into one document row), a
// matter_type stage on project, and a response deadline + status on office
// action. No data is destroyed; existing office-action blobs become their
// project's first matter document.
func (r *Repo) migrateV7ToV8(ctx context.Context) error {
	if err := r.Backup(ctx, r.path+".v7-to-v8.bak"); err != nil {
		return fmt.Errorf("store/sqlite: migrate v7 to v8: backup: %w", err)
	}
	// The new columns are added with ALTER (not idempotent), so each is gated on
	// its absence: a real v7 database lacks them, while a database carrying the
	// current schema.sql with only the version marker rolled back already has
	// them. Build the statement list accordingly, then create the new table and
	// backfill in one transaction.
	stmts := make([]string, 0, len(migrationV7ToV8Statements)+3)
	for col, alter := range map[string]string{
		"project.matter_type":        `ALTER TABLE project ADD COLUMN matter_type TEXT NOT NULL DEFAULT ''`,
		"office_action.response_due": `ALTER TABLE office_action ADD COLUMN response_due TEXT NOT NULL DEFAULT ''`,
		"office_action.status":       `ALTER TABLE office_action ADD COLUMN status TEXT NOT NULL DEFAULT 'open'`,
	} {
		table, column, _ := strings.Cut(col, ".")
		has, err := r.columnExists(ctx, table, column)
		if err != nil {
			return fmt.Errorf("store/sqlite: migrate v7 to v8: detect %s: %w", col, err)
		}
		if !has {
			stmts = append(stmts, alter)
		}
	}
	stmts = append(stmts, migrationV7ToV8Statements...)

	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: migrate v7 to v8: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("store/sqlite: migrate v7 to v8: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE schema_meta SET value = '8' WHERE key = 'schema_version'`); err != nil {
		return fmt.Errorf("store/sqlite: migrate v7 to v8: set version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: migrate v7 to v8: commit: %w", err)
	}
	return nil
}

// migrationV7ToV8Statements create the prosecution-matter workspace tables
// (matter_document, matter_event) and backfill one matter document per existing
// office action that has a stored blob. Kept in sync with schema.sql (idempotent
// CREATE ... IF NOT EXISTS). The new project and office-action columns are added
// separately in migrateV7ToV8 (column-gated ALTERs). response_due is left empty
// for pre-existing actions — new imports compute it from the mail date and type
// in the engine.
var migrationV7ToV8Statements = []string{
	`CREATE TABLE IF NOT EXISTS matter_document (
	    id               TEXT PRIMARY KEY,
	    project_id       TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
	    office_action_id TEXT NOT NULL DEFAULT '',
	    kind             TEXT NOT NULL DEFAULT 'other',
	    display_name     TEXT NOT NULL DEFAULT '',
	    blob_path        TEXT NOT NULL DEFAULT '',
	    blob_hash        TEXT NOT NULL DEFAULT '',
	    extracted_text   TEXT NOT NULL DEFAULT '',
	    added_at         TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_matter_document_project ON matter_document (project_id, added_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_matter_document_oa ON matter_document (office_action_id)`,
	`CREATE TABLE IF NOT EXISTS matter_event (
	    id               TEXT PRIMARY KEY,
	    project_id       TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
	    office_action_id TEXT NOT NULL DEFAULT '',
	    kind             TEXT NOT NULL DEFAULT 'note',
	    party            TEXT NOT NULL DEFAULT '',
	    occurred_at      TEXT NOT NULL DEFAULT '',
	    due_at           TEXT NOT NULL DEFAULT '',
	    summary          TEXT NOT NULL DEFAULT '',
	    comment          TEXT NOT NULL DEFAULT '',
	    created_at       TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_matter_event_project ON matter_event (project_id, occurred_at DESC)`,
	`CREATE TABLE IF NOT EXISTS time_entry (
	    id               TEXT PRIMARY KEY,
	    project_id       TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
	    office_action_id TEXT NOT NULL DEFAULT '',
	    activity         TEXT NOT NULL DEFAULT '',
	    source           TEXT NOT NULL DEFAULT 'manual',
	    started_at       TEXT NOT NULL DEFAULT '',
	    ended_at         TEXT NOT NULL DEFAULT '',
	    seconds          INTEGER NOT NULL DEFAULT 0,
	    validated        INTEGER NOT NULL DEFAULT 0,
	    validated_at     TEXT NOT NULL DEFAULT '',
	    note             TEXT NOT NULL DEFAULT '',
	    created_at       TEXT NOT NULL DEFAULT '',
	    updated_at       TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_time_entry_project ON time_entry (project_id, validated)`,
	`CREATE TABLE IF NOT EXISTS ai_usage (
	    id                TEXT PRIMARY KEY,
	    project_id        TEXT NOT NULL REFERENCES project (id) ON DELETE CASCADE,
	    office_action_id  TEXT NOT NULL DEFAULT '',
	    draft_id          TEXT NOT NULL DEFAULT '',
	    provider          TEXT NOT NULL DEFAULT '',
	    model             TEXT NOT NULL DEFAULT '',
	    prompts           INTEGER NOT NULL DEFAULT 0,
	    prompt_tokens     INTEGER NOT NULL DEFAULT 0,
	    completion_tokens INTEGER NOT NULL DEFAULT 0,
	    total_tokens      INTEGER NOT NULL DEFAULT 0,
	    created_at        TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_ai_usage_project ON ai_usage (project_id, created_at DESC)`,
	`CREATE TABLE IF NOT EXISTS deadline (
	    id               TEXT PRIMARY KEY,
	    kind             TEXT NOT NULL DEFAULT 'custom',
	    patent_number    TEXT NOT NULL DEFAULT '',
	    project_id       TEXT NOT NULL DEFAULT '',
	    office_action_id TEXT NOT NULL DEFAULT '',
	    title            TEXT NOT NULL DEFAULT '',
	    window_opens     TEXT NOT NULL DEFAULT '',
	    due_date         TEXT NOT NULL DEFAULT '',
	    grace_ends       TEXT NOT NULL DEFAULT '',
	    status           TEXT NOT NULL DEFAULT 'pending',
	    created_at       TEXT NOT NULL DEFAULT '',
	    updated_at       TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE INDEX IF NOT EXISTS idx_deadline_status_due ON deadline (status, due_date)`,
	`CREATE INDEX IF NOT EXISTS idx_deadline_patent ON deadline (patent_number)`,
	`CREATE TABLE IF NOT EXISTS reminder_log (
	    subject        TEXT NOT NULL,
	    threshold_days INTEGER NOT NULL,
	    channel        TEXT NOT NULL,
	    sent_at        TEXT NOT NULL DEFAULT '',
	    PRIMARY KEY (subject, threshold_days, channel)
	)`,
	// Backfill: the existing single office-action blob becomes its first matter
	// document so nothing imported under the old model is lost.
	`INSERT INTO matter_document
	    (id, project_id, office_action_id, kind, display_name, blob_path, blob_hash, extracted_text, added_at)
	 SELECT 'mdoc-' || id, project_id, id, 'oa',
	        CASE WHEN mail_date != '' THEN 'Office Action ' || substr(mail_date, 1, 10) ELSE 'Office Action' END,
	        blob_path, blob_hash, extracted_text, imported_at
	 FROM office_action
	 WHERE blob_path != ''`,
}

// migrateV5ToV6 introduces the assignee_history table (the record.id-keyed
// ownership timeline). It is purely additive: the table is created here so the
// version bump is self-contained, and schema.sql recreates it idempotently on the
// following apply. No data backfill — existing databases populate assignee_history
// lazily the next time each patent's assignments are (re)fetched, which calls
// RebuildAssigneeHistory.
//
// TODO(assignee-backfill): one-time backfill of assignee_history from the
// already-stored uspto_assignment rows + record.assignee so existing corpora get a
// populated timeline without a re-fetch. Deferred to keep this migration trivial.
func (r *Repo) migrateV5ToV6(ctx context.Context) error {
	if err := r.Backup(ctx, r.path+".v5-to-v6.bak"); err != nil {
		return fmt.Errorf("store/sqlite: migrate v5 to v6: backup: %w", err)
	}
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: migrate v5 to v6: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range migrationV5ToV6Statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("store/sqlite: migrate v5 to v6: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE schema_meta SET value = '6' WHERE key = 'schema_version'`); err != nil {
		return fmt.Errorf("store/sqlite: migrate v5 to v6: set version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: migrate v5 to v6: commit: %w", err)
	}
	return nil
}

// migrationV5ToV6Statements create the assignee_history table and its indexes.
// Kept in sync with schema.sql (idempotent CREATE ... IF NOT EXISTS).
var migrationV5ToV6Statements = []string{
	`CREATE TABLE IF NOT EXISTS assignee_history (
	    record_id        TEXT NOT NULL REFERENCES record (id) ON DELETE CASCADE,
	    pull_type        TEXT NOT NULL,
	    ordinal          INTEGER NOT NULL DEFAULT 0,
	    assignee_name    TEXT NOT NULL DEFAULT '',
	    assignee_norm    TEXT NOT NULL DEFAULT '',
	    effective_date   TEXT NOT NULL DEFAULT '',
	    pulled_at        TEXT NOT NULL DEFAULT '',
	    reel_frame       TEXT NOT NULL DEFAULT '',
	    conveyance_text  TEXT NOT NULL DEFAULT '',
	    is_latest        INTEGER NOT NULL DEFAULT 0,
	    PRIMARY KEY (record_id, pull_type, ordinal)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_assignee_history_record ON assignee_history (record_id)`,
	`CREATE INDEX IF NOT EXISTS idx_assignee_history_norm   ON assignee_history (assignee_norm)`,
	`CREATE INDEX IF NOT EXISTS idx_assignee_history_latest ON assignee_history (record_id, is_latest)`,
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

// migrateV8ToV9 creates the patent_renewal table to track patent maintenance/annuity configurations.
func (r *Repo) migrateV8ToV9(ctx context.Context) error {
	if err := r.Backup(ctx, r.path+".v8-to-v9.bak"); err != nil {
		return fmt.Errorf("store/sqlite: migrate v8 to v9: backup: %w", err)
	}
	stmt := `CREATE TABLE IF NOT EXISTS patent_renewal (
		patent_number TEXT PRIMARY KEY REFERENCES record (number) ON DELETE CASCADE,
		entity_size   TEXT NOT NULL DEFAULT 'large',
		is_tracked    INTEGER NOT NULL DEFAULT 1,
		created_at    TEXT NOT NULL DEFAULT '',
		updated_at    TEXT NOT NULL DEFAULT ''
	)`
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: migrate v8 to v9: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("store/sqlite: migrate v8 to v9: create table: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE schema_meta SET value = '9' WHERE key = 'schema_version'`); err != nil {
		return fmt.Errorf("store/sqlite: migrate v8 to v9: set version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: migrate v8 to v9: commit: %w", err)
	}
	return nil
}

// migrateV9ToV10 lets preparation documents share the project tag taxonomy.
func (r *Repo) migrateV9ToV10(ctx context.Context) error {
	if err := r.Backup(ctx, r.path+".v9-to-v10.bak"); err != nil {
		return fmt.Errorf("store/sqlite: migrate v9 to v10: backup: %w", err)
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS matter_document_office_action (
		    document_id      TEXT NOT NULL REFERENCES matter_document (id) ON DELETE CASCADE,
		    office_action_id TEXT NOT NULL REFERENCES office_action (id) ON DELETE CASCADE,
		    created_at       TEXT NOT NULL,
		    PRIMARY KEY (document_id, office_action_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_matter_document_office_action_oa ON matter_document_office_action (office_action_id)`,
		`INSERT INTO matter_document_office_action (document_id, office_action_id, created_at)
		 SELECT id, office_action_id, added_at
		 FROM matter_document
		 WHERE office_action_id != ''
		 ON CONFLICT(document_id, office_action_id) DO NOTHING`,
		`CREATE TABLE IF NOT EXISTS matter_document_tag (
		    tag_id      INTEGER NOT NULL REFERENCES tag (id) ON DELETE CASCADE,
		    document_id TEXT NOT NULL REFERENCES matter_document (id) ON DELETE CASCADE,
		    created_at  TEXT NOT NULL,
		    PRIMARY KEY (tag_id, document_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_matter_document_tag_document ON matter_document_tag (document_id)`,
		`UPDATE schema_meta SET value = '10' WHERE key = 'schema_version'`,
	}
	for _, stmt := range stmts {
		if _, err := r.writer.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("store/sqlite: migrate v9 to v10: %w", err)
		}
	}
	return nil
}

// migrateV10ToV11 adds name and last_opened_at to office_action, and last_opened_at to matter_document.
func (r *Repo) migrateV10ToV11(ctx context.Context) error {
	if err := r.Backup(ctx, r.path+".v10-to-v11.bak"); err != nil {
		return fmt.Errorf("store/sqlite: migrate v10 to v11: backup: %w", err)
	}
	var stmts []string
	for col, alter := range map[string]string{
		"office_action.name":             `ALTER TABLE office_action ADD COLUMN name TEXT NOT NULL DEFAULT ''`,
		"office_action.last_opened_at":   `ALTER TABLE office_action ADD COLUMN last_opened_at TEXT NOT NULL DEFAULT ''`,
		"matter_document.last_opened_at": `ALTER TABLE matter_document ADD COLUMN last_opened_at TEXT NOT NULL DEFAULT ''`,
	} {
		table, column, _ := strings.Cut(col, ".")
		has, err := r.columnExists(ctx, table, column)
		if err != nil {
			return fmt.Errorf("store/sqlite: migrate v10 to v11: detect %s: %w", col, err)
		}
		if !has {
			stmts = append(stmts, alter)
		}
	}
	stmts = append(stmts, `UPDATE schema_meta SET value = '11' WHERE key = 'schema_version'`)
	for _, stmt := range stmts {
		if _, err := r.writer.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("store/sqlite: migrate v10 to v11: %w", err)
		}
	}
	return nil
}

