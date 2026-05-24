package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

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

func TestMigrateFFoldsProjectIDsAndBacksUp(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy.db")
	writeLegacyDB(t, path)

	repo, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open (runs migration): %v", err)
	}
	defer func() { _ = repo.Close() }()

	// The backup copy must exist.
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Fatalf("expected backup at %s.bak: %v", path, err)
	}

	// project_ids must be gone.
	var leftover int
	if err := repo.reader.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='project_ids'`).
		Scan(&leftover); err != nil {
		t.Fatalf("check project_ids: %v", err)
	}
	if leftover != 0 {
		t.Fatalf("project_ids table still present after migration")
	}

	// The curated IDS entry must have moved onto the membership row.
	entry, err := repo.IDSEntry(ctx, "p-1", domain.MustParsePatentNumber("US0000001B2"))
	if err != nil {
		t.Fatalf("IDSEntry after migration: %v", err)
	}
	if entry.Notes != "keep this" || entry.Status != domain.IDSEntrySubmitted {
		t.Fatalf("migrated IDS entry = %+v, want notes 'keep this' / submitted", entry)
	}

	// Legacy cached review state is folded back to unknown; fetch_state carries cache status.
	m, err := repo.Membership(ctx, "p-1", domain.MustParsePatentNumber("US0000001B2"))
	if err != nil {
		t.Fatalf("Membership after migration: %v", err)
	}
	if m.ReviewState != domain.ReviewStateUnknown {
		t.Fatalf("membership state = %q, want unknown", m.ReviewState)
	}

	// The orphan IDS entry must have a synthesized membership.
	orphan, err := repo.IDSEntry(ctx, "p-1", domain.MustParsePatentNumber("US0000002B2"))
	if err != nil {
		t.Fatalf("orphan IDSEntry after migration: %v", err)
	}
	if orphan.Status != domain.IDSEntryPending {
		t.Fatalf("orphan IDS entry status = %q, want pending", orphan.Status)
	}

	// The relation must survive the rebuild.
	rels, err := repo.Relations(ctx, domain.MustParsePatentNumber("US0000001B2"), domain.RelationCites)
	if err != nil {
		t.Fatalf("Relations after migration: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("got %d relations after migration, want 1", len(rels))
	}

	// Re-opening must be a no-op (migration is idempotent).
	if err := repo.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	repo2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	_ = repo2.Close()
}
