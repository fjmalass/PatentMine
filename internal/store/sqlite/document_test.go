package sqlite

import (
	"context"
	"errors"
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

func TestDocumentRoundTrip(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	record := samplePatent("US0000001B2")
	if err := repo.SavePatent(ctx, record); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	app := domain.Document{
		Number: domain.MustParsePatentNumber("US16000001"),
		Stage:  domain.StageApplication,
	}
	grant := domain.Document{
		Number: domain.MustParsePatentNumber("US0000001B2"),
		Stage:  domain.StageGrant,
	}
	for _, d := range []domain.Document{app, grant} {
		if err := repo.SaveDocument(ctx, record.Number, d); err != nil {
			t.Fatalf("SaveDocument %s: %v", d.Number, err)
		}
	}

	docs, err := repo.Documents(ctx, record.Number)
	if err != nil {
		t.Fatalf("Documents: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("Documents returned %d, want 2", len(docs))
	}

	// Patent loads its document set for the detail view.
	got, err := repo.Patent(ctx, record.Number)
	if err != nil {
		t.Fatalf("Patent: %v", err)
	}
	if len(got.Documents) != 2 {
		t.Fatalf("Patent.Documents = %d, want 2", len(got.Documents))
	}
}

func TestRecordOfMapsEveryNumber(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	record := domain.MustParsePatentNumber("US16000001") // keyed by the application
	if err := repo.SavePatent(ctx, samplePatent("US16000001")); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	appNumber := domain.MustParsePatentNumber("US16000001")
	grantNumber := domain.MustParsePatentNumber("US0000001B2")
	for _, d := range []domain.Document{
		{Number: appNumber, Stage: domain.StageApplication},
		{Number: grantNumber, Stage: domain.StageGrant},
	} {
		if err := repo.SaveDocument(ctx, record, d); err != nil {
			t.Fatalf("SaveDocument: %v", err)
		}
	}

	// Either the application or the grant number resolves to the same record.
	for _, n := range []domain.PatentNumber{appNumber, grantNumber} {
		got, err := repo.RecordOf(ctx, n)
		if err != nil {
			t.Fatalf("RecordOf(%s): %v", n, err)
		}
		if !got.Equal(record) {
			t.Fatalf("RecordOf(%s) = %s, want %s", n, got, record)
		}
	}
	if _, err := repo.RecordOf(ctx, domain.MustParsePatentNumber("US9999999B2")); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("RecordOf(unknown) err = %v, want ErrNotFound", err)
	}
}

func TestMergeRecords(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	keep := domain.MustParsePatentNumber("US16000001")
	absorb := domain.MustParsePatentNumber("US0000001B2")
	if err := repo.SavePatent(ctx, samplePatent("US16000001")); err != nil {
		t.Fatalf("SavePatent keep: %v", err)
	}
	if err := repo.SavePatent(ctx, samplePatent("US0000001B2")); err != nil {
		t.Fatalf("SavePatent absorb: %v", err)
	}
	if err := repo.SaveDocument(ctx, keep, domain.Document{Number: keep, Stage: domain.StageApplication}); err != nil {
		t.Fatalf("SaveDocument keep: %v", err)
	}
	if err := repo.SaveDocument(ctx, absorb, domain.Document{Number: absorb, Stage: domain.StageGrant}); err != nil {
		t.Fatalf("SaveDocument absorb: %v", err)
	}
	// The absorbed record carries a project membership and a relation.
	project := domain.Project{ID: "p-1", Name: "P"}
	if err := repo.SaveProject(ctx, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	if err := repo.AddMembership(ctx, domain.Membership{
		Project: project.ID, Patent: absorb, ReviewState: domain.ReviewStateUnknown,
	}); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}
	if err := repo.SavePatent(ctx, samplePatent("US0000002B2")); err != nil {
		t.Fatalf("SavePatent relation target: %v", err)
	}
	if err := repo.SaveRelation(ctx, domain.Relation{
		From: absorb, To: domain.MustParsePatentNumber("US0000002B2"), Kind: domain.RelationCites,
	}); err != nil {
		t.Fatalf("SaveRelation: %v", err)
	}

	if err := repo.MergeRecords(ctx, keep, absorb); err != nil {
		t.Fatalf("MergeRecords: %v", err)
	}

	// The absorbed record's documents now belong to keep.
	docs, err := repo.Documents(ctx, keep)
	if err != nil {
		t.Fatalf("Documents: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("kept record has %d documents, want 2 after merge", len(docs))
	}
	// Its membership and relation were repointed.
	members, err := repo.Memberships(ctx, project.ID)
	if err != nil {
		t.Fatalf("Memberships: %v", err)
	}
	if len(members) != 1 || !members[0].Patent.Equal(keep) {
		t.Fatalf("membership = %v, want one pointing at the kept record", members)
	}
	rels, err := repo.Relations(ctx, keep, domain.RelationCites)
	if err != nil {
		t.Fatalf("Relations: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("kept record has %d cites edges, want 1 after merge", len(rels))
	}
	// The absorbed patent row is gone from the patent table.
	var count int
	if err := repo.reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM patent WHERE number = ?", absorb.Normalized()).Scan(&count); err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 0 {
		t.Fatalf("absorbed patent row still present in patent table")
	}

	// But repo.Patent now gracefully resolves the absorbed alias to the kept record
	got, err := repo.Patent(ctx, absorb)
	if err != nil {
		t.Fatalf("repo.Patent failed on absorbed alias: %v", err)
	}
	if !got.Number.Equal(keep) {
		t.Errorf("repo.Patent(absorb) = %s, want %s (resolved keep record)", got.Number, keep)
	}
}

func TestCheckCollisions(t *testing.T) {
	repo := openTestRepo(t)
	defer repo.Close()
	ctx := context.Background()

	p1 := samplePatent("US8898930B1")
	p1.Title = "Title A"
	p2 := samplePatent("US8898930B2")
	p2.Title = "Title B"

	if err := repo.SavePatent(ctx, p1); err != nil {
		t.Fatalf("SavePatent p1: %v", err)
	}
	if err := repo.SavePatent(ctx, p2); err != nil {
		t.Fatalf("SavePatent p2: %v", err)
	}

	d1 := domain.Document{Number: p1.Number, Stage: domain.StagePublication}
	d2 := domain.Document{Number: p2.Number, Stage: domain.StageGrant}

	if err := repo.SaveDocument(ctx, p1.Number, d1); err != nil {
		t.Fatalf("SaveDocument d1: %v", err)
	}
	if err := repo.SaveDocument(ctx, p2.Number, d2); err != nil {
		t.Fatalf("SaveDocument d2: %v", err)
	}

	collisions, err := repo.CheckCollisions(ctx)
	if err != nil {
		t.Fatalf("CheckCollisions: %v", err)
	}

	if len(collisions) != 1 {
		t.Fatalf("expected 1 collision group, got %d", len(collisions))
	}

	col := collisions[0]
	if col.ApplicationNumber != "US8898930" {
		t.Errorf("expected ApplicationNumber US8898930, got %s", col.ApplicationNumber)
	}

	if len(col.RecordNumbers) != 2 {
		t.Fatalf("expected 2 record numbers in collision group, got %d", len(col.RecordNumbers))
	}

	if !col.RecordNumbers[0].Equal(p1.Number) || !col.RecordNumbers[1].Equal(p2.Number) {
		t.Errorf("unexpected record numbers in collision group: %v", col.RecordNumbers)
	}
}

