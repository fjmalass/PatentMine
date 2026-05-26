package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
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

func TestObsoleteSchemaFailsClearly(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	writeLegacyDB(t, path)

	_, err := Open(ctx, path)
	if err == nil || !strings.Contains(err.Error(), "obsolete database schema") {
		t.Fatalf("Open legacy schema err = %v, want obsolete schema error", err)
	}
}
