package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"patentmine/internal/domain"
)

// legacySchemaSQL is the pre-migration-F shape: a standalone project_ids table,
// a membership without ids_* columns, and a relation without foreign keys.
const legacySchemaSQL = `
CREATE TABLE patent (
	number TEXT PRIMARY KEY, country TEXT NOT NULL, serial TEXT NOT NULL,
	kind TEXT NOT NULL, title TEXT NOT NULL, abstract TEXT NOT NULL,
	assignee TEXT NOT NULL, inventors TEXT NOT NULL, fetch_state TEXT NOT NULL,
	source TEXT NOT NULL, application_date TEXT NOT NULL, publication_date TEXT NOT NULL,
	grant_date TEXT NOT NULL, fetched_at TEXT NOT NULL, display_number TEXT NOT NULL DEFAULT '',
	first_claim TEXT NOT NULL DEFAULT '', expiration_date TEXT NOT NULL DEFAULT '',
	expiration_source TEXT NOT NULL DEFAULT '', source_url TEXT NOT NULL DEFAULT '');
CREATE TABLE relation (from_number TEXT NOT NULL, to_number TEXT NOT NULL,
	kind TEXT NOT NULL, PRIMARY KEY (from_number, to_number, kind));
CREATE TABLE project (id TEXT PRIMARY KEY, name TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE membership (project_id TEXT NOT NULL, patent_number TEXT NOT NULL,
	state TEXT NOT NULL, added_at TEXT NOT NULL, PRIMARY KEY (project_id, patent_number));
CREATE TABLE project_ids (id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id TEXT NOT NULL, patent_number TEXT NOT NULL,
	kind_code TEXT NOT NULL DEFAULT '', country_code TEXT NOT NULL DEFAULT '',
	in_full INTEGER NOT NULL DEFAULT 0, relevant_passages TEXT NOT NULL DEFAULT '',
	notes TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'pending',
	added_at TEXT NOT NULL, UNIQUE (project_id, patent_number));
`

// writeLegacyDB builds a database at path in the pre-migration-F shape with a
// little sample data, then closes it.
func writeLegacyDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	for _, stmt := range []string{
		legacySchemaSQL,
		`INSERT INTO project VALUES ('p-1', 'Proj', '2026-01-01T00:00:00Z')`,
		`INSERT INTO patent (number,country,serial,kind,title,abstract,assignee,inventors,
			fetch_state,source,application_date,publication_date,grant_date,fetched_at)
			VALUES ('US0000001B2','US','0000001','B2','T','A','Acme','[]','cached','file','','','','')`,
		`INSERT INTO patent (number,country,serial,kind,title,abstract,assignee,inventors,
			fetch_state,source,application_date,publication_date,grant_date,fetched_at)
			VALUES ('US0000002B2','US','0000002','B2','T2','A','Acme','[]','cached','file','','','','')`,
		// A member with a curated IDS entry.
		`INSERT INTO membership VALUES ('p-1','US0000001B2','unknown','2026-02-01T00:00:00Z')`,
		`INSERT INTO project_ids (project_id,patent_number,notes,status,added_at)
			VALUES ('p-1','US0000001B2','keep this','submitted','2026-03-01T00:00:00Z')`,
		// An IDS entry whose patent was never made a project member.
		`INSERT INTO project_ids (project_id,patent_number,status,added_at)
			VALUES ('p-1','US0000002B2','pending','2026-03-02T00:00:00Z')`,
		`INSERT INTO relation VALUES ('US0000001B2','US0000002B2','cites')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed legacy db: %v", err)
		}
	}
}

// v3SchemaSQL is a faithful subset of the schema_version=3 shape: the patent
// table with its full column set, the corpus/user tables that FK patent(number),
// and a couple of re-fetchable source tables. Enough to exercise the real
// v3→v4 upgrade through Open().
const v3SchemaSQL = `
CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO schema_meta VALUES ('schema_version','3');
CREATE TABLE patent (
	number TEXT PRIMARY KEY, country TEXT NOT NULL DEFAULT '', serial TEXT NOT NULL DEFAULT '',
	kind TEXT NOT NULL DEFAULT '', display_number TEXT NOT NULL DEFAULT '', title TEXT NOT NULL DEFAULT '',
	abstract TEXT NOT NULL DEFAULT '', assignee TEXT NOT NULL DEFAULT '', inventors TEXT NOT NULL DEFAULT '[]',
	fetch_state TEXT NOT NULL, source TEXT NOT NULL DEFAULT '', application_date TEXT NOT NULL DEFAULT '',
	publication_date TEXT NOT NULL DEFAULT '', grant_date TEXT NOT NULL DEFAULT '', fetched_at TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL DEFAULT '', first_claim TEXT NOT NULL DEFAULT '', expiration_date TEXT NOT NULL DEFAULT '',
	expiration_source TEXT NOT NULL DEFAULT '', source_url TEXT NOT NULL DEFAULT '',
	classifications TEXT NOT NULL DEFAULT '[]', classifications_text TEXT NOT NULL DEFAULT '');
CREATE TABLE document (number TEXT PRIMARY KEY, record_number TEXT NOT NULL REFERENCES patent(number) ON DELETE CASCADE,
	country TEXT DEFAULT '', serial TEXT DEFAULT '', kind TEXT DEFAULT '', stage TEXT NOT NULL,
	dated TEXT DEFAULT '', source TEXT DEFAULT '', source_ref TEXT DEFAULT '');
CREATE TABLE relation (from_number TEXT NOT NULL REFERENCES patent(number) ON DELETE CASCADE,
	to_number TEXT NOT NULL REFERENCES patent(number) ON DELETE CASCADE, kind TEXT NOT NULL,
	source TEXT DEFAULT '', source_ref TEXT DEFAULT '', observed_at TEXT DEFAULT '',
	PRIMARY KEY (from_number, to_number, kind));
CREATE TABLE project (id TEXT PRIMARY KEY, name TEXT NOT NULL, created_at TEXT NOT NULL,
	application_number TEXT DEFAULT '', filing_date TEXT DEFAULT '', art_unit TEXT DEFAULT '', attorney_docket_number TEXT DEFAULT '');
CREATE TABLE membership (project_id TEXT NOT NULL REFERENCES project(id) ON DELETE CASCADE,
	patent_number TEXT NOT NULL REFERENCES patent(number) ON DELETE CASCADE, state TEXT NOT NULL, added_at TEXT NOT NULL,
	ids_kind_code TEXT DEFAULT '', ids_in_full INTEGER DEFAULT 0, ids_relevant_passages TEXT DEFAULT '',
	ids_notes TEXT DEFAULT '', ids_status TEXT DEFAULT '', ids_added_at TEXT DEFAULT '', ids_submitted_at TEXT DEFAULT '',
	PRIMARY KEY (project_id, patent_number));
CREATE TABLE authority_identifier (authority TEXT NOT NULL, identifier_type TEXT NOT NULL, identifier TEXT NOT NULL,
	raw_identifier TEXT DEFAULT '', record_number TEXT NOT NULL REFERENCES patent(number) ON DELETE CASCADE,
	document_number TEXT DEFAULT '', country TEXT DEFAULT '', kind TEXT DEFAULT '', dated TEXT DEFAULT '',
	source TEXT DEFAULT '', confidence INTEGER DEFAULT 100, created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
	PRIMARY KEY (authority, identifier_type, identifier));
CREATE VIRTUAL TABLE patent_fts USING fts5(title, abstract, classifications_text, content='patent', content_rowid='rowid');
CREATE TRIGGER patent_fts_insert AFTER INSERT ON patent BEGIN
	INSERT INTO patent_fts(rowid,title,abstract,classifications_text) VALUES(new.rowid,new.title,new.abstract,new.classifications_text); END;
`

func TestMigrateV2ToV3BackfillsMembershipProvenance(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v2.db")

	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		t.Fatalf("open v2 db: %v", err)
	}
	defer func() { _ = db.Close() }()

	for _, stmt := range []string{
		`CREATE TABLE schema_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`INSERT INTO schema_meta VALUES ('schema_version','2')`,
		`CREATE TABLE patent (number TEXT PRIMARY KEY)`,
		`CREATE TABLE project (id TEXT PRIMARY KEY)`,
		`CREATE TABLE membership (project_id TEXT NOT NULL, patent_number TEXT NOT NULL, state TEXT NOT NULL, added_at TEXT NOT NULL, PRIMARY KEY (project_id, patent_number))`,
		`INSERT INTO patent VALUES ('US0000001B2')`,
		`INSERT INTO project VALUES ('p-1')`,
		`INSERT INTO membership VALUES ('p-1', 'US0000001B2', 'unknown', '2026-01-01T00:00:00Z')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed v2: %v\n%s", err, stmt)
		}
	}

	repo := &Repo{writer: db}
	if err := repo.migrateV2ToV3(ctx); err != nil {
		t.Fatalf("migrateV2ToV3: %v", err)
	}

	var method string
	if err := db.QueryRowContext(ctx, `SELECT added_method FROM membership_provenance WHERE project_id='p-1' AND patent_number='US0000001B2'`).Scan(&method); err != nil {
		t.Fatalf("query provenance: %v", err)
	}
	if method != "direct" {
		t.Fatalf("added_method = %q, want direct", method)
	}

	var version string
	if err := db.QueryRowContext(ctx, `SELECT value FROM schema_meta WHERE key='schema_version'`).Scan(&version); err != nil {
		t.Fatalf("query schema version: %v", err)
	}
	if version != "3" {
		t.Fatalf("schema_version = %q, want 3", version)
	}
}

// TestMigrateV3ToV4PreservesData opens a v3 database through the real Open()
// path and asserts the upgrade preserves the corpus and user data, assigns
// surrogate ids, recreates the source tables at the new shape, and leaves the
// database with sound foreign keys.
func TestMigrateV3ToV4PreservesData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v3.db")

	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		t.Fatalf("open v3 db: %v", err)
	}
	for _, stmt := range []string{
		v3SchemaSQL,
		`INSERT INTO patent (number,fetch_state,title) VALUES ('US20020106191A1','stub','Pub'),('US7000001B2','cached','Grant')`,
		`INSERT INTO document (number,record_number,stage) VALUES ('US20020106191A1','US20020106191A1','publication'),('US7000001B2','US7000001B2','grant')`,
		`INSERT INTO relation (from_number,to_number,kind) VALUES ('US7000001B2','US20020106191A1','cites')`,
		`INSERT INTO project VALUES ('p1','Proj','t','','','','')`,
		`INSERT INTO membership (project_id,patent_number,state,added_at,ids_notes) VALUES ('p1','US20020106191A1','under_review','t','keep me')`,
		`INSERT INTO authority_identifier (authority,identifier_type,identifier,record_number,created_at,updated_at) VALUES ('US','publication','20020106191','US20020106191A1','t','t')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed v3: %v\n%s", err, stmt)
		}
	}
	_ = db.Close()

	repo, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open (migrate v3→v4): %v", err)
	}
	defer func() { _ = repo.Close() }()

	count := func(q string) int {
		var n int
		if err := repo.reader.QueryRowContext(ctx, q).Scan(&n); err != nil {
			t.Fatalf("count %q: %v", q, err)
		}
		return n
	}
	if v := count(`SELECT value FROM schema_meta WHERE key='schema_version'`); v != 17 {
		t.Fatalf("schema_version = %d, want 17", v)
	}
	if n := count(`SELECT COUNT(*) FROM record`); n != 2 {
		t.Fatalf("records = %d, want 2", n)
	}
	if n := count(`SELECT COUNT(*) FROM record WHERE id != ''`); n != 2 {
		t.Fatalf("records with surrogate id = %d, want 2", n)
	}
	if n := count(`SELECT COUNT(*) FROM membership WHERE ids_notes='keep me'`); n != 1 {
		t.Fatalf("curated membership preserved = %d, want 1", n)
	}
	if n := count(`SELECT COUNT(*) FROM relation`); n != 1 {
		t.Fatalf("relations preserved = %d, want 1", n)
	}
	// New-shape source tables exist (recreated empty by schema.sql).
	if n := count(`SELECT COUNT(*) FROM source_bib`); n != 0 {
		t.Fatalf("source_bib rows = %d, want 0", n)
	}
	// The corpus resolves through the surrogate and FK integrity holds.
	if _, err := repo.Patent(ctx, domain.MustParsePatentNumber("US7000001B2")); err != nil {
		t.Fatalf("Patent after migrate: %v", err)
	}
	rows, err := repo.reader.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("fk check: %v", err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		t.Fatalf("foreign_key_check reported violations after migration")
	}
}

// TestMigrateV4ToV5BackfillsGrantKind reproduces the export bug: a granted
// record whose grant document was stored digits-only (kind-less) while the
// authoritative kind lives in uspto_application.grant_kind. The v4→v5 migration
// must stamp the kind onto the grant document and the record display number so
// the Google-linkable number derived at export carries it.
func TestMigrateV4ToV5BackfillsGrantKind(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v4.db")

	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		t.Fatalf("open v4 db: %v", err)
	}
	for _, stmt := range []string{
		schemaSQL,
		// Mark the database as v4 so Open() runs the v4→v5 migration.
		`UPDATE schema_meta SET value = '4' WHERE key = 'schema_version'`,
		`INSERT INTO record (id, number, country, serial, kind, display_number, fetch_state)
		 VALUES ('rid1', 'US14047231', 'US', '14047231', '', 'US09658068', 'cached')`,
		`INSERT INTO document (number, record_number, country, serial, kind, stage)
		 VALUES ('US14047231', 'US14047231', 'US', '14047231', '', 'application')`,
		`INSERT INTO document (number, record_number, country, serial, kind, stage)
		 VALUES ('US09658068', 'US14047231', 'US', '09658068', '', 'grant')`,
		`INSERT INTO uspto_application (application_number, record_id, grant_doc_number, grant_kind, fetched_at)
		 VALUES ('14047231', 'rid1', '09658068', 'B2', 't')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed v4: %v\n%s", err, stmt)
		}
	}
	_ = db.Close()

	repo, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open (migrate v4→v5): %v", err)
	}
	defer func() { _ = repo.Close() }()

	scan := func(q string) string {
		var s string
		if err := repo.reader.QueryRowContext(ctx, q).Scan(&s); err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		return s
	}
	if v := scan(`SELECT value FROM schema_meta WHERE key='schema_version'`); v != "17" {
		t.Fatalf("schema_version = %q, want 17", v)
	}
	if k := scan(`SELECT kind FROM document WHERE record_number='US14047231' AND stage='grant'`); k != "B2" {
		t.Fatalf("grant document kind = %q, want B2", k)
	}
	if n := scan(`SELECT number FROM document WHERE record_number='US14047231' AND stage='grant'`); n != "US09658068B2" {
		t.Fatalf("grant document number = %q, want US09658068B2", n)
	}
	if d := scan(`SELECT display_number FROM record WHERE number='US14047231'`); d != "US09658068B2" {
		t.Fatalf("record display_number = %q, want US09658068B2", d)
	}

	// End to end: the number derived for Google export now carries the kind.
	p, err := repo.Patent(ctx, domain.MustParsePatentNumber("US14047231"))
	if err != nil {
		t.Fatalf("Patent after migrate: %v", err)
	}
	if got := p.GrantedNumber().Normalized(); got != "US09658068B2" {
		t.Fatalf("GrantedNumber() = %q, want US09658068B2", got)
	}
}

func TestMigrateV7ToV8BackfillsMatterDocument(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v7.db")

	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		t.Fatalf("open v7 db: %v", err)
	}
	for _, stmt := range []string{
		schemaSQL,
		// Mark the database as v7 so Open() runs the v7→v8 migration. The column
		// gates make the ALTERs no-ops here (the current schema already has them),
		// exercising the matter_document backfill specifically.
		`UPDATE schema_meta SET value = '7' WHERE key = 'schema_version'`,
		`INSERT INTO project (id, name, created_at) VALUES ('p-1', 'Proj', '2026-01-01T00:00:00Z')`,
		`INSERT INTO office_action (id, project_id, mail_date, oa_type, examiner, blob_path, blob_hash, extracted_text, imported_at)
		 VALUES ('oa-1', 'p-1', '2026-01-09T00:00:00Z', 'non_final', 'Smith', '/docs/oa-1.pdf', 'deadbeef', 'rejection text', '2026-01-10T00:00:00Z')`,
		// An office action with no stored blob must NOT be backfilled.
		`INSERT INTO office_action (id, project_id, mail_date, oa_type) VALUES ('oa-2', 'p-1', '2026-02-01T00:00:00Z', 'final')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed v7: %v\n%s", err, stmt)
		}
	}
	_ = db.Close()

	repo, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open (migrate v7→v8): %v", err)
	}
	defer func() { _ = repo.Close() }()

	scan := func(q string) string {
		var s string
		if err := repo.reader.QueryRowContext(ctx, q).Scan(&s); err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		return s
	}
	if v := scan(`SELECT value FROM schema_meta WHERE key='schema_version'`); v != "17" {
		t.Fatalf("schema_version = %q, want 17", v)
	}

	docs, err := repo.ListMatterDocuments(ctx, "p-1")
	if err != nil {
		t.Fatalf("ListMatterDocuments: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("matter documents = %d, want 1 (only the OA with a blob)", len(docs))
	}
	d := docs[0]
	if d.OfficeActionID != "oa-1" || d.Kind != domain.MatterDocOA {
		t.Fatalf("backfilled doc = {oa:%q kind:%q}, want {oa-1 oa}", d.OfficeActionID, d.Kind)
	}
	if d.BlobPath != "/docs/oa-1.pdf" || d.BlobHash != "deadbeef" || d.ExtractedText != "rejection text" {
		t.Fatalf("backfilled doc blob/text not carried over: %+v", d)
	}
	if d.Project != "p-1" {
		t.Fatalf("backfilled doc project = %q, want p-1", d.Project)
	}
}

func TestObsoleteSchemaFailsClearly(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	writeLegacyDB(t, path)

	_, err := Open(ctx, path)
	if err == nil || !strings.Contains(err.Error(), "obsolete database schema") {
		t.Fatalf("Open legacy schema err = %v, want obsolete schema error", err)
	}
}

func TestMigrateV10ToV11(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v10.db")

	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		t.Fatalf("open v10 db: %v", err)
	}
	for _, stmt := range []string{
		schemaSQL,
		`UPDATE schema_meta SET value = '10' WHERE key = 'schema_version'`,
		`ALTER TABLE office_action DROP COLUMN name`,
		`ALTER TABLE office_action DROP COLUMN last_opened_at`,
		`ALTER TABLE matter_document DROP COLUMN last_opened_at`,
		`INSERT INTO project (id, name, created_at) VALUES ('p-1', 'Proj', '2026-01-01T00:00:00Z')`,
		`INSERT INTO office_action (id, project_id, mail_date, oa_type) VALUES ('oa-1', 'p-1', '2026-01-09T00:00:00Z', 'non_final')`,
		`INSERT INTO matter_document (id, project_id, office_action_id, kind, display_name) VALUES ('doc-1', 'p-1', 'oa-1', 'reference', 'doc1')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed v10: %v\n%s", err, stmt)
		}
	}
	_ = db.Close()

	repo, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open (migrate v10→v11): %v", err)
	}
	defer func() { _ = repo.Close() }()

	scan := func(q string) string {
		var s string
		if err := repo.reader.QueryRowContext(ctx, q).Scan(&s); err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		return s
	}
	if v := scan(`SELECT value FROM schema_meta WHERE key='schema_version'`); v != "17" {
		t.Fatalf("schema_version = %q, want 17", v)
	}

	// Verify columns were added with empty string default values
	if n := scan(`SELECT name FROM office_action WHERE id='oa-1'`); n != "" {
		t.Fatalf("office_action name = %q, want empty string", n)
	}
	if lo := scan(`SELECT last_opened_at FROM office_action WHERE id='oa-1'`); lo != "" {
		t.Fatalf("office_action last_opened_at = %q, want empty string", lo)
	}
	if lo := scan(`SELECT last_opened_at FROM matter_document WHERE id='doc-1'`); lo != "" {
		t.Fatalf("matter_document last_opened_at = %q, want empty string", lo)
	}
}

func TestMigrateV11ToV12BackfillsOriginStage(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v11.db")

	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		t.Fatalf("open v11 db: %v", err)
	}
	for _, stmt := range []string{
		schemaSQL,
		`UPDATE schema_meta SET value = '11' WHERE key = 'schema_version'`,
		// Simulate the pre-v12 shape: the origin/stage axes did not exist yet.
		`DROP INDEX IF EXISTS idx_matter_document_stage`,
		`ALTER TABLE matter_document DROP COLUMN origin`,
		`ALTER TABLE matter_document DROP COLUMN stage`,
		`INSERT INTO project (id, name, created_at) VALUES ('p-1', 'Proj', '2026-01-01T00:00:00Z')`,
		`INSERT INTO matter_document (id, project_id, kind, display_name) VALUES ('d-oa', 'p-1', 'oa', 'oa')`,
		`INSERT INTO matter_document (id, project_id, kind, display_name) VALUES ('d-ref', 'p-1', 'reference', 'ref')`,
		`INSERT INTO matter_document (id, project_id, kind, display_name) VALUES ('d-resp', 'p-1', 'response', 'resp')`,
		`INSERT INTO matter_document (id, project_id, kind, display_name) VALUES ('d-ids', 'p-1', 'ids', 'ids')`,
		`INSERT INTO matter_document (id, project_id, kind, display_name) VALUES ('d-weird', 'p-1', 'weirdkind', 'weird')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed v11: %v\n%s", err, stmt)
		}
	}
	_ = db.Close()

	repo, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open (migrate v11→v12): %v", err)
	}
	defer func() { _ = repo.Close() }()

	scan := func(q string) string {
		var s string
		if err := repo.reader.QueryRowContext(ctx, q).Scan(&s); err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		return s
	}
	if v := scan(`SELECT value FROM schema_meta WHERE key='schema_version'`); v != "17" {
		t.Fatalf("schema_version = %q, want 17", v)
	}
	// Each row's origin/stage is inferred from its kind (domain.InferOriginStage).
	for _, tc := range []struct{ id, origin, stage string }{
		{"d-oa", "office", "received"},
		{"d-ref", "third_party", "received"},
		{"d-resp", "firm", "draft"},
		{"d-ids", "firm", "filed"},
		{"d-weird", "firm", "received"}, // unknown kind → default catch-all
	} {
		o := scan(`SELECT origin FROM matter_document WHERE id='` + tc.id + `'`)
		s := scan(`SELECT stage FROM matter_document WHERE id='` + tc.id + `'`)
		if o != tc.origin || s != tc.stage {
			t.Fatalf("%s backfilled to {origin:%q stage:%q}, want {%q %q}", tc.id, o, s, tc.origin, tc.stage)
		}
	}
}

func TestMigrateV12ToV13BackfillsStatusChangedAt(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v12.db")

	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		t.Fatalf("open v12 db: %v", err)
	}
	for _, stmt := range []string{
		schemaSQL,
		`UPDATE schema_meta SET value = '12' WHERE key = 'schema_version'`,
		// Simulate the pre-v13 shape: the status_changed_at axis did not exist yet.
		`ALTER TABLE office_action DROP COLUMN status_changed_at`,
		`INSERT INTO project (id, name, created_at) VALUES ('p-1', 'Proj', '2026-01-01T00:00:00Z')`,
		`INSERT INTO office_action (id, project_id, mail_date, oa_type, imported_at) VALUES ('oa-1', 'p-1', '2026-01-09T00:00:00Z', 'non_final', '2026-01-10T00:00:00Z')`,
		`INSERT INTO office_action (id, project_id, mail_date, oa_type) VALUES ('oa-2', 'p-1', '2026-01-09T00:00:00Z', 'non_final')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed v12: %v\n%s", err, stmt)
		}
	}
	_ = db.Close()

	repo, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open (migrate v12→v13): %v", err)
	}
	defer func() { _ = repo.Close() }()

	scan := func(q string) string {
		var s string
		if err := repo.reader.QueryRowContext(ctx, q).Scan(&s); err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		return s
	}
	if v := scan(`SELECT value FROM schema_meta WHERE key='schema_version'`); v != "17" {
		t.Fatalf("schema_version = %q, want 17", v)
	}
	// A row with an import time gets its status_changed_at backfilled from it.
	if sc := scan(`SELECT status_changed_at FROM office_action WHERE id='oa-1'`); sc != "2026-01-10T00:00:00Z" {
		t.Fatalf("oa-1 status_changed_at = %q, want backfill from imported_at", sc)
	}
	// A row with no import time stays empty (nothing to seed from).
	if sc := scan(`SELECT status_changed_at FROM office_action WHERE id='oa-2'`); sc != "" {
		t.Fatalf("oa-2 status_changed_at = %q, want empty", sc)
	}
}

func TestMigrateV13ToV14CreatesOfficeActionTag(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v13.db")

	db, err := sql.Open(driverName, dsn(path))
	if err != nil {
		t.Fatalf("open v13 db: %v", err)
	}
	for _, stmt := range []string{
		schemaSQL,
		`UPDATE schema_meta SET value = '13' WHERE key = 'schema_version'`,
		// Simulate the pre-v14 shape: the office_action_tag table did not exist yet.
		`DROP TABLE office_action_tag`,
		`INSERT INTO project (id, name, created_at) VALUES ('p-1', 'Proj', '2026-01-01T00:00:00Z')`,
		`INSERT INTO office_action (id, project_id, mail_date, oa_type, imported_at) VALUES ('oa-1', 'p-1', '2026-01-09T00:00:00Z', 'non_final', '2026-01-10T00:00:00Z')`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed v13: %v\n%s", err, stmt)
		}
	}
	_ = db.Close()

	repo, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open (migrate v13→v14): %v", err)
	}
	defer func() { _ = repo.Close() }()

	scan := func(q string) string {
		var s string
		if err := repo.reader.QueryRowContext(ctx, q).Scan(&s); err != nil {
			t.Fatalf("query %q: %v", q, err)
		}
		return s
	}
	if v := scan(`SELECT value FROM schema_meta WHERE key='schema_version'`); v != "17" {
		t.Fatalf("schema_version = %q, want 17", v)
	}
	// The office_action_tag table is recreated by the migration and is usable: a
	// tag can be created and assigned to the seeded office action.
	if name := scan(`SELECT name FROM sqlite_master WHERE type='table' AND name='office_action_tag'`); name != "office_action_tag" {
		t.Fatalf("office_action_tag table missing after migrate, got %q", name)
	}
	tag, err := repo.CreateTag(ctx, "p-1", "needs_response")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := repo.TagOfficeAction(ctx, tag.ID, "oa-1", time.Time{}); err != nil {
		t.Fatalf("TagOfficeAction: %v", err)
	}
	tags, err := repo.OfficeActionTags(ctx, "p-1", "oa-1")
	if err != nil {
		t.Fatalf("OfficeActionTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "needs_response" {
		t.Fatalf("office action tags = %+v, want [needs_response]", tags)
	}
}
