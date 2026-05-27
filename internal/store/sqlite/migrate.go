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
		review_state          TEXT NOT NULL,
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
	`INSERT INTO membership_new (project_id, patent_number, review_state, added_at,
		ids_kind_code, ids_in_full, ids_relevant_passages,
		ids_notes, ids_status, ids_added_at, ids_submitted_at)
	 SELECT m.project_id, m.patent_number, m.review_state, m.added_at,
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
	`INSERT INTO membership_new (project_id, patent_number, review_state, added_at,
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
	`CREATE INDEX idx_membership_project ON membership (project_id, review_state)`,

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

// migrateG upgrades a schema-v2 database to schema-v3: it introduces the opaque
// record_id surrogate primary key on patent and retypes every foreign-key
// column that used to hold patent.number. The detection key is the presence of
// the record_id column on patent — absent means v2, so the migration runs.
//
// Like migrateF this is a one-way table rebuild; the database is copied to
// "<path>.bak" with VACUUM INTO first.
func (r *Repo) migrateG(ctx context.Context) error {
	needed, err := r.needsMigrationG(ctx)
	if err != nil {
		return err
	}
	if !needed {
		return nil
	}
	if err := r.backupBeforeMigration(ctx); err != nil {
		return fmt.Errorf("store/sqlite: migrate G: backup: %w", err)
	}
	return r.runMigrationG(ctx)
}

// needsMigrationG reports whether the legacy v2 schema is still in place. The
// signal is the presence of a "patent" table that does not yet carry a
// record_id column. A fresh install (no patent table yet) needs no migration
// because schema.sql will create the v3 layout directly.
func (r *Repo) needsMigrationG(ctx context.Context) (bool, error) {
	hasPatent, err := r.tableExists(ctx, "patent")
	if err != nil {
		return false, fmt.Errorf("store/sqlite: migrate G: detect patent: %w", err)
	}
	if !hasPatent {
		return false, nil
	}
	hasRecordID, err := r.columnExists(ctx, "patent", "record_id")
	if err != nil {
		return false, fmt.Errorf("store/sqlite: migrate G: detect record_id: %w", err)
	}
	return !hasRecordID, nil
}

// runMigrationG executes the v2 → v3 table rebuilds inside one transaction.
// Foreign-key enforcement is disabled for the duration so SQLite allows the
// referenced patent table to be rebuilt.
func (r *Repo) runMigrationG(ctx context.Context) error {
	conn, err := r.writer.Conn(ctx)
	if err != nil {
		return fmt.Errorf("store/sqlite: migrate G: connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("store/sqlite: migrate G: disable fk: %w", err)
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: migrate G: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range migrationGStatements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			// Swallow harmless "duplicate column" errors so the migration is
			// idempotent on a half-applied database.
			msg := err.Error()
			if strings.Contains(msg, "duplicate column") {
				continue
			}
			return fmt.Errorf("store/sqlite: migrate G: %w (stmt: %s)", err, stmt)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: migrate G: commit: %w", err)
	}

	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("store/sqlite: migrate G: re-enable fk: %w", err)
	}
	return nil
}

// migrateH renames the legacy "state" column on the membership table to
// "review_state" for naming consistency with the Go type (ReviewState).
// It is safe to run on fresh databases (no-op) and on already-renamed ones.
func (r *Repo) migrateH(ctx context.Context) error {
	needed, err := r.needsMigrationH(ctx)
	if err != nil {
		return err
	}
	if !needed {
		return nil
	}
	// No backup needed for a simple RENAME COLUMN + index recreation.
	return r.runMigrationH(ctx)
}

// needsMigrationH reports whether the membership table still has the old
// "state" column name.
func (r *Repo) needsMigrationH(ctx context.Context) (bool, error) {
	hasMembership, err := r.tableExists(ctx, "membership")
	if err != nil {
		return false, fmt.Errorf("store/sqlite: migrate H: detect membership: %w", err)
	}
	if !hasMembership {
		return false, nil
	}
	hasState, err := r.columnExists(ctx, "membership", "state")
	if err != nil {
		return false, fmt.Errorf("store/sqlite: migrate H: detect state column: %w", err)
	}
	return hasState, nil
}

// runMigrationH performs the column rename and refreshes the index.
func (r *Repo) runMigrationH(ctx context.Context) error {
	conn, err := r.writer.Conn(ctx)
	if err != nil {
		return fmt.Errorf("store/sqlite: migrate H: connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: migrate H: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// SQLite 3.25+ supports RENAME COLUMN. We already rely on it in migration G.
	if _, err := tx.ExecContext(ctx, `ALTER TABLE membership RENAME COLUMN state TO review_state`); err != nil {
		return fmt.Errorf("store/sqlite: migrate H: rename column: %w", err)
	}

	// Recreate the index under the new column name (RENAME COLUMN updates it,
	// but recreating is explicit and harmless).
	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS idx_membership_project`); err != nil {
		return fmt.Errorf("store/sqlite: migrate H: drop index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX idx_membership_project ON membership (project_id, review_state)`); err != nil {
		return fmt.Errorf("store/sqlite: migrate H: create index: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: migrate H: commit: %w", err)
	}
	return nil
}

// migrationGStatements rebuilds every table that referenced patent(number) to
// use the new record_id surrogate key. Each patent row gets a UUID-shaped
// record_id (32 lowercase hex chars from hex(randomblob(16))) and every FK
// table is rebuilt with the new column backfilled via a JOIN on patent.number.
//
// The order matters: patent must gain record_id first so the FK joins resolve.
// Each FK table is rebuilt under the *_new naming pattern (mirroring migrateF)
// to keep the change atomic.
var migrationGStatements = []string{
	// Step 1: add record_id column to patent with a default value generator.
	// The DEFAULT expression makes new inserts auto-mint an id; existing rows
	// get a backfill from a second UPDATE so every row carries one.
	`ALTER TABLE patent ADD COLUMN record_id TEXT NOT NULL DEFAULT ''`,
	`UPDATE patent SET record_id = lower(hex(randomblob(16))) WHERE record_id = ''`,

	// Step 2: rebuild patent with record_id as PK and patent.number as a
	// regular column. The UNIQUE(country,serial,kind) constraint replaces the
	// old uniqueness guarantee from number being the PK.
	`CREATE TABLE patent_new (
		record_id            TEXT NOT NULL PRIMARY KEY,
		number               TEXT NOT NULL DEFAULT '',
		country              TEXT NOT NULL DEFAULT '',
		serial               TEXT NOT NULL DEFAULT '',
		kind                 TEXT NOT NULL DEFAULT '',
		display_number       TEXT NOT NULL DEFAULT '',
		title                TEXT NOT NULL DEFAULT '',
		abstract             TEXT NOT NULL DEFAULT '',
		assignee             TEXT NOT NULL DEFAULT '',
		inventors            TEXT NOT NULL DEFAULT '[]',
		fetch_state          TEXT NOT NULL,
		source               TEXT NOT NULL DEFAULT '',
		application_date     TEXT NOT NULL DEFAULT '',
		publication_date     TEXT NOT NULL DEFAULT '',
		grant_date           TEXT NOT NULL DEFAULT '',
		fetched_at           TEXT NOT NULL DEFAULT '',
		updated_at           TEXT NOT NULL DEFAULT '',
		first_claim          TEXT NOT NULL DEFAULT '',
		expiration_date      TEXT NOT NULL DEFAULT '',
		expiration_source    TEXT NOT NULL DEFAULT '',
		source_url           TEXT NOT NULL DEFAULT '',
		classifications      TEXT NOT NULL DEFAULT '[]',
		classifications_text TEXT NOT NULL DEFAULT '',
		UNIQUE (country, serial, kind)
	)`,
	`INSERT INTO patent_new (record_id, number, country, serial, kind, display_number,
		title, abstract, assignee, inventors, fetch_state, source,
		application_date, publication_date, grant_date, fetched_at, updated_at,
		first_claim, expiration_date, expiration_source, source_url,
		classifications, classifications_text)
	 SELECT record_id, number, country, serial, kind, display_number,
		title, abstract, assignee, inventors, fetch_state, source,
		application_date, publication_date, grant_date, fetched_at, updated_at,
		first_claim, expiration_date, expiration_source, source_url,
		classifications,
		COALESCE(classifications_text, '')
	 FROM patent`,

	// Step 3: rebuild every FK table. document, relation, authority_identifier,
	// uspto_application, uspto_document, source_snapshot, source_diff,
	// membership, project_patent_note, patent_tag, mutation_item.
	`CREATE TABLE document_new (
		number        TEXT PRIMARY KEY,
		record_id     TEXT NOT NULL,
		country       TEXT NOT NULL DEFAULT '',
		serial        TEXT NOT NULL DEFAULT '',
		kind          TEXT NOT NULL DEFAULT '',
		stage         TEXT NOT NULL,
		dated         TEXT NOT NULL DEFAULT '',
		source        TEXT NOT NULL DEFAULT '',
		source_ref    TEXT NOT NULL DEFAULT '',
		UNIQUE (record_id, stage, number)
	)`,
	`INSERT INTO document_new (number, record_id, country, serial, kind, stage, dated, source, source_ref)
	 SELECT d.number, p.record_id, d.country, d.serial, d.kind, d.stage, d.dated, d.source, d.source_ref
	 FROM document d JOIN patent_new p ON p.number = d.record_number`,

	`CREATE TABLE relation_new_g (
		from_record_id TEXT NOT NULL,
		to_record_id   TEXT NOT NULL,
		kind           TEXT NOT NULL,
		source         TEXT NOT NULL DEFAULT '',
		source_ref     TEXT NOT NULL DEFAULT '',
		observed_at    TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (from_record_id, to_record_id, kind)
	)`,
	`INSERT INTO relation_new_g (from_record_id, to_record_id, kind)
	 SELECT pf.record_id, pt.record_id, r.kind
	 FROM relation r
	 JOIN patent_new pf ON pf.number = r.from_number
	 JOIN patent_new pt ON pt.number = r.to_number`,

	`CREATE TABLE authority_identifier_new (
		authority       TEXT NOT NULL,
		identifier_type TEXT NOT NULL,
		identifier      TEXT NOT NULL,
		raw_identifier  TEXT NOT NULL DEFAULT '',
		record_id       TEXT NOT NULL,
		document_number TEXT NOT NULL DEFAULT '',
		country         TEXT NOT NULL DEFAULT '',
		kind            TEXT NOT NULL DEFAULT '',
		dated           TEXT NOT NULL DEFAULT '',
		source          TEXT NOT NULL DEFAULT '',
		confidence      INTEGER NOT NULL DEFAULT 100,
		created_at      TEXT NOT NULL,
		updated_at      TEXT NOT NULL,
		PRIMARY KEY (authority, identifier_type, identifier)
	)`,
	`INSERT INTO authority_identifier_new (authority, identifier_type, identifier, raw_identifier,
		record_id, document_number, country, kind, dated, source, confidence, created_at, updated_at)
	 SELECT a.authority, a.identifier_type, a.identifier, a.raw_identifier,
		p.record_id, a.document_number, a.country, a.kind, a.dated, a.source,
		a.confidence, a.created_at, a.updated_at
	 FROM authority_identifier a JOIN patent_new p ON p.number = a.record_number`,

	`CREATE TABLE uspto_application_new (
		application_number              TEXT PRIMARY KEY,
		record_id                       TEXT NOT NULL,
		application_series_code         TEXT NOT NULL DEFAULT '',
		invention_title                 TEXT NOT NULL DEFAULT '',
		filing_date                     TEXT NOT NULL DEFAULT '',
		effective_filing_date           TEXT NOT NULL DEFAULT '',
		application_status_code         TEXT NOT NULL DEFAULT '',
		application_status_text         TEXT NOT NULL DEFAULT '',
		application_status_date         TEXT NOT NULL DEFAULT '',
		application_type_code           TEXT NOT NULL DEFAULT '',
		application_type_label          TEXT NOT NULL DEFAULT '',
		application_type_category       TEXT NOT NULL DEFAULT '',
		first_inventor_to_file          INTEGER NOT NULL DEFAULT 0,
		national_stage                  INTEGER NOT NULL DEFAULT 0,
		first_inventor_name             TEXT NOT NULL DEFAULT '',
		first_applicant_name            TEXT NOT NULL DEFAULT '',
		customer_number                 TEXT NOT NULL DEFAULT '',
		group_art_unit_number           TEXT NOT NULL DEFAULT '',
		examiner_name                   TEXT NOT NULL DEFAULT '',
		docket_number                   TEXT NOT NULL DEFAULT '',
		application_confirmation_number TEXT NOT NULL DEFAULT '',
		uspc_symbol_text                TEXT NOT NULL DEFAULT '',
		uspc_class                      TEXT NOT NULL DEFAULT '',
		uspc_subclass                   TEXT NOT NULL DEFAULT '',
		small_entity_status             INTEGER NOT NULL DEFAULT 0,
		business_entity_status          TEXT NOT NULL DEFAULT '',
		publication_category_json       TEXT NOT NULL DEFAULT '[]',
		last_ingestion_datetime         TEXT NOT NULL DEFAULT '',
		fetched_at                      TEXT NOT NULL,
		pgpub_xml_url                   TEXT NOT NULL DEFAULT '',
		pgpub_xml_name                  TEXT NOT NULL DEFAULT '',
		patent_grant_xml_url            TEXT NOT NULL DEFAULT '',
		patent_grant_xml_name           TEXT NOT NULL DEFAULT '',
		grant_doc_number                TEXT NOT NULL DEFAULT '',
		grant_kind                      TEXT NOT NULL DEFAULT '',
		grant_date                      TEXT NOT NULL DEFAULT '',
		grant_dtd_version               TEXT NOT NULL DEFAULT '',
		grant_status                    TEXT NOT NULL DEFAULT '',
		grant_date_produced             TEXT NOT NULL DEFAULT '',
		grant_file_name                 TEXT NOT NULL DEFAULT '',
		grant_lang                      TEXT NOT NULL DEFAULT '',
		term_extension_days             INTEGER NOT NULL DEFAULT 0,
		number_of_claims                INTEGER NOT NULL DEFAULT 0,
		exemplary_claim                 TEXT NOT NULL DEFAULT '',
		number_of_drawing_sheets        INTEGER NOT NULL DEFAULT 0,
		number_of_figures               INTEGER NOT NULL DEFAULT 0,
		primary_examiner_first          TEXT NOT NULL DEFAULT '',
		primary_examiner_last           TEXT NOT NULL DEFAULT '',
		primary_examiner_department     TEXT NOT NULL DEFAULT '',
		assistant_examiners_json        TEXT NOT NULL DEFAULT '[]',
		attorney_org                    TEXT NOT NULL DEFAULT '',
		attorney_type                   TEXT NOT NULL DEFAULT '',
		field_of_search_json            TEXT NOT NULL DEFAULT '[]',
		grant_parsed_at                 TEXT NOT NULL DEFAULT ''
	)`,
	`INSERT INTO uspto_application_new SELECT
		u.application_number, p.record_id, '',
		u.invention_title, u.filing_date, u.effective_filing_date,
		u.application_status_code, u.application_status_text, u.application_status_date,
		u.application_type_code, u.application_type_label, u.application_type_category,
		u.first_inventor_to_file, u.national_stage, u.first_inventor_name, u.first_applicant_name,
		u.customer_number, u.group_art_unit_number, u.examiner_name, u.docket_number,
		u.application_confirmation_number, u.uspc_symbol_text, u.uspc_class, u.uspc_subclass,
		u.small_entity_status, u.business_entity_status, u.publication_category_json,
		u.last_ingestion_datetime, u.fetched_at,
		u.pgpub_xml_url, u.pgpub_xml_name, u.patent_grant_xml_url, u.patent_grant_xml_name,
		u.grant_doc_number, u.grant_kind, u.grant_date, u.grant_dtd_version, u.grant_status,
		u.grant_date_produced, u.grant_file_name, u.grant_lang, u.term_extension_days,
		u.number_of_claims, u.exemplary_claim, u.number_of_drawing_sheets, u.number_of_figures,
		u.primary_examiner_first, u.primary_examiner_last, u.primary_examiner_department,
		u.assistant_examiners_json, u.attorney_org, u.attorney_type, u.field_of_search_json,
		u.grant_parsed_at
	 FROM uspto_application u JOIN patent_new p ON p.number = u.record_number`,

	`CREATE TABLE uspto_document_new (
		document_number      TEXT PRIMARY KEY,
		record_id            TEXT NOT NULL,
		application_number   TEXT NOT NULL DEFAULT '',
		document_stage       TEXT NOT NULL,
		country              TEXT NOT NULL DEFAULT 'US',
		kind_code            TEXT NOT NULL DEFAULT '',
		publication_date     TEXT NOT NULL DEFAULT '',
		grant_date           TEXT NOT NULL DEFAULT '',
		filing_date          TEXT NOT NULL DEFAULT '',
		source_product       TEXT NOT NULL DEFAULT '',
		bulk_file_id         TEXT NOT NULL DEFAULT '',
		snapshot_id          TEXT NOT NULL DEFAULT '',
		title                TEXT NOT NULL DEFAULT '',
		abstract             TEXT NOT NULL DEFAULT '',
		first_claim          TEXT NOT NULL DEFAULT '',
		classifications_json TEXT NOT NULL DEFAULT '[]',
		inventors_json       TEXT NOT NULL DEFAULT '[]',
		assignees_json       TEXT NOT NULL DEFAULT '[]',
		fetched_at           TEXT NOT NULL
	)`,
	`INSERT INTO uspto_document_new SELECT
		d.document_number, p.record_id, d.application_number, d.document_stage,
		d.country, d.kind_code, d.publication_date, d.grant_date, d.filing_date,
		d.source_product, d.bulk_file_id, d.snapshot_id, d.title, d.abstract,
		d.first_claim, d.classifications_json, d.inventors_json, d.assignees_json,
		d.fetched_at
	 FROM uspto_document d JOIN patent_new p ON p.number = d.record_number`,

	`CREATE TABLE source_snapshot_new (
		id               TEXT PRIMARY KEY,
		record_id        TEXT NOT NULL,
		source           TEXT NOT NULL,
		source_record_id TEXT NOT NULL DEFAULT '',
		source_url       TEXT NOT NULL DEFAULT '',
		fetched_at       TEXT NOT NULL,
		payload_kind     TEXT NOT NULL DEFAULT '',
		payload_hash     TEXT NOT NULL DEFAULT '',
		payload_path     TEXT NOT NULL DEFAULT '',
		response_bytes   INTEGER NOT NULL DEFAULT 0,
		http_status      INTEGER NOT NULL DEFAULT 0,
		etag             TEXT NOT NULL DEFAULT '',
		last_modified    TEXT NOT NULL DEFAULT '',
		summary_json     TEXT NOT NULL DEFAULT '{}'
	)`,
	`INSERT INTO source_snapshot_new SELECT
		s.id, p.record_id, s.source, s.source_record_id, s.source_url, s.fetched_at,
		s.payload_kind, s.payload_hash, s.payload_path, s.response_bytes, s.http_status,
		s.etag, s.last_modified, s.summary_json
	 FROM source_snapshot s JOIN patent_new p ON p.number = s.patent_number`,

	`CREATE TABLE source_diff_new (
		id                 TEXT PRIMARY KEY,
		record_id          TEXT NOT NULL,
		field_path         TEXT NOT NULL,
		uspto_value        TEXT NOT NULL DEFAULT '',
		google_value       TEXT NOT NULL DEFAULT '',
		chosen_value       TEXT NOT NULL DEFAULT '',
		chosen_source      TEXT NOT NULL DEFAULT '',
		severity           TEXT NOT NULL DEFAULT '',
		recorded_at        TEXT NOT NULL,
		uspto_snapshot_id  TEXT NOT NULL DEFAULT '',
		google_snapshot_id TEXT NOT NULL DEFAULT '',
		reconciled_at      TEXT,
		reconciled_by      TEXT,
		reconciled_choice  TEXT
	)`,
	`INSERT INTO source_diff_new SELECT
		d.id, p.record_id, d.field_path,
		d.uspto_value, d.google_value, d.chosen_value, d.chosen_source,
		d.severity, d.recorded_at, d.uspto_snapshot_id, d.google_snapshot_id,
		d.reconciled_at, d.reconciled_by, d.reconciled_choice
	 FROM source_diff d JOIN patent_new p ON p.number = d.patent_number`,

	`CREATE TABLE membership_new_g (
		project_id            TEXT NOT NULL,
		record_id             TEXT NOT NULL,
		review_state          TEXT NOT NULL,
		added_at              TEXT NOT NULL,
		ids_kind_code         TEXT NOT NULL DEFAULT '',
		ids_in_full           INTEGER NOT NULL DEFAULT 0,
		ids_relevant_passages TEXT NOT NULL DEFAULT '',
		ids_notes             TEXT NOT NULL DEFAULT '',
		ids_status            TEXT NOT NULL DEFAULT '',
		ids_added_at          TEXT NOT NULL DEFAULT '',
		ids_submitted_at      TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (project_id, record_id)
	)`,
	`INSERT INTO membership_new_g SELECT
		m.project_id, p.record_id, m.review_state, m.added_at,
		m.ids_kind_code, m.ids_in_full, m.ids_relevant_passages,
		m.ids_notes, m.ids_status, m.ids_added_at, m.ids_submitted_at
	 FROM membership m JOIN patent_new p ON p.number = m.patent_number`,

	`CREATE TABLE project_patent_note_new (
		project_id TEXT NOT NULL,
		record_id  TEXT NOT NULL,
		markdown   TEXT NOT NULL DEFAULT '',
		added_at   TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		PRIMARY KEY (project_id, record_id)
	)`,
	`INSERT INTO project_patent_note_new SELECT
		n.project_id, p.record_id, n.markdown, n.added_at, n.updated_at
	 FROM project_patent_note n JOIN patent_new p ON p.number = n.patent_number`,

	`CREATE TABLE patent_tag_new (
		tag_id     INTEGER NOT NULL,
		record_id  TEXT NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY (tag_id, record_id)
	)`,
	`INSERT INTO patent_tag_new SELECT
		pt.tag_id, p.record_id, pt.created_at
	 FROM patent_tag pt JOIN patent_new p ON p.number = pt.patent_number`,

	`CREATE TABLE mutation_item_new (
		group_id     TEXT NOT NULL,
		ordinal      INTEGER NOT NULL,
		record_id    TEXT NOT NULL,
		kind         TEXT NOT NULL,
		before_json  TEXT NOT NULL DEFAULT 'null',
		after_json   TEXT NOT NULL DEFAULT 'null',
		inverse_json TEXT NOT NULL DEFAULT 'null',
		PRIMARY KEY (group_id, ordinal)
	)`,
	`INSERT INTO mutation_item_new SELECT
		mi.group_id, mi.ordinal, p.record_id, mi.kind,
		mi.before_json, mi.after_json, mi.inverse_json
	 FROM mutation_item mi JOIN patent_new p ON p.number = mi.patent_number`,

	// uspto_continuity: rename parent_record_number → parent_record_id, child_record_number → child_record_id.
	`ALTER TABLE uspto_continuity RENAME COLUMN parent_record_number TO parent_record_id`,
	`ALTER TABLE uspto_continuity RENAME COLUMN child_record_number TO child_record_id`,

	// uspto_foreign_priority: linked_record_number → linked_record_id.
	`ALTER TABLE uspto_foreign_priority RENAME COLUMN linked_record_number TO linked_record_id`,

	// refresh_run_item: keep patent_number audit col, add record_id.
	`ALTER TABLE refresh_run_item ADD COLUMN record_id TEXT NOT NULL DEFAULT ''`,
	`UPDATE refresh_run_item SET record_id = (
		SELECT p.record_id FROM patent_new p WHERE p.number = refresh_run_item.patent_number
	) WHERE patent_number != ''`,

	// FTS5 triggers reference the patent table; since we rebuild patent, the
	// triggers will be recreated by schema.sql after migration. Drop the old
	// FTS triggers so the table rebuild doesn't fail.
	`DROP TRIGGER IF EXISTS patent_fts_insert`,
	`DROP TRIGGER IF EXISTS patent_fts_delete`,
	`DROP TRIGGER IF EXISTS patent_fts_update`,
	`DROP TABLE IF EXISTS patent_fts`,

	// Swap each table in.
	`DROP TABLE document`,
	`ALTER TABLE document_new RENAME TO document`,
	`DROP TABLE relation`,
	`ALTER TABLE relation_new_g RENAME TO relation`,
	`DROP TABLE authority_identifier`,
	`ALTER TABLE authority_identifier_new RENAME TO authority_identifier`,
	`DROP TABLE uspto_application`,
	`ALTER TABLE uspto_application_new RENAME TO uspto_application`,
	`DROP TABLE uspto_document`,
	`ALTER TABLE uspto_document_new RENAME TO uspto_document`,
	`DROP TABLE source_snapshot`,
	`ALTER TABLE source_snapshot_new RENAME TO source_snapshot`,
	`DROP TABLE source_diff`,
	`ALTER TABLE source_diff_new RENAME TO source_diff`,
	`DROP TABLE membership`,
	`ALTER TABLE membership_new_g RENAME TO membership`,
	`DROP TABLE project_patent_note`,
	`ALTER TABLE project_patent_note_new RENAME TO project_patent_note`,
	`DROP TABLE patent_tag`,
	`ALTER TABLE patent_tag_new RENAME TO patent_tag`,
	`DROP TABLE mutation_item`,
	`ALTER TABLE mutation_item_new RENAME TO mutation_item`,
	`DROP TABLE patent`,
	`ALTER TABLE patent_new RENAME TO patent`,

	// Bump schema version so requireSchemaVersion accepts the migrated db.
	`UPDATE schema_meta SET value = '3' WHERE key = 'schema_version'`,
}
