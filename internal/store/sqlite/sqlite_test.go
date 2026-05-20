package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

// openTestRepo opens a fresh database in a temp directory.
func openTestRepo(t *testing.T) *Repo {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	repo, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func samplePatent(number string) domain.Patent {
	return domain.Patent{
		Number:     domain.MustParsePatentNumber(number),
		Title:      "A method for " + number,
		Abstract:   "abstract text",
		Assignee:   "Acme Corp",
		Inventors:  []domain.Inventor{"Ada Lovelace", "Alan Turing"},
		FetchState: domain.FetchCached,
		Source:     domain.SourceFile,
		GrantDate:  time.Date(2023, 3, 21, 0, 0, 0, 0, time.UTC),
		FetchedAt:  time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC),
	}
}

func TestPatentRoundTrip(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	want := samplePatent("US11611785B2")
	if err := repo.SavePatent(ctx, want); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	got, err := repo.Patent(ctx, want.Number)
	if err != nil {
		t.Fatalf("Patent: %v", err)
	}
	if got.Number != want.Number || got.Title != want.Title {
		t.Fatalf("Patent = %+v, want %+v", got, want)
	}
	if len(got.Inventors) != 2 || got.Inventors[0] != "Ada Lovelace" {
		t.Fatalf("inventors not round-tripped: %v", got.Inventors)
	}
	if !got.GrantDate.Equal(want.GrantDate) {
		t.Fatalf("grant date = %v, want %v", got.GrantDate, want.GrantDate)
	}
}

func TestPatentUpsert(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	p := samplePatent("US11611785B2")
	if err := repo.SavePatent(ctx, p); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	p.Title = "Updated title"
	if err := repo.SavePatent(ctx, p); err != nil {
		t.Fatalf("SavePatent update: %v", err)
	}
	n, err := repo.CountPatents(ctx, store.PatentQuery{})
	if err != nil {
		t.Fatalf("CountPatents: %v", err)
	}
	if n != 1 {
		t.Fatalf("CountPatents = %d, want 1 after upsert", n)
	}
	got, err := repo.Patent(ctx, p.Number)
	if err != nil {
		t.Fatalf("Patent: %v", err)
	}
	if got.Title != "Updated title" {
		t.Fatalf("title = %q, want updated", got.Title)
	}
}

func TestPatentNotFound(t *testing.T) {
	repo := openTestRepo(t)
	_, err := repo.Patent(context.Background(), domain.MustParsePatentNumber("US0000000A1"))
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Patent missing: err = %v, want ErrNotFound", err)
	}
}

func TestListPatentsPagination(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	for _, n := range []string{"US0000001A1", "US0000002A1", "US0000003A1"} {
		if err := repo.SavePatent(ctx, samplePatent(n)); err != nil {
			t.Fatalf("SavePatent %s: %v", n, err)
		}
	}
	page, err := repo.ListPatents(ctx, store.PatentQuery{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("ListPatents: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("page size = %d, want 2", len(page))
	}
	if page[0].Number.Serial != "0000001" {
		t.Fatalf("first page not ordered by number: %v", page[0].Number)
	}
	rest, err := repo.ListPatents(ctx, store.PatentQuery{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("ListPatents page 2: %v", err)
	}
	if len(rest) != 1 {
		t.Fatalf("second page size = %d, want 1", len(rest))
	}
}

func TestListPatentsSearch(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	if err := repo.SavePatent(ctx, samplePatent("US0000001A1")); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	if err := repo.SavePatent(ctx, samplePatent("EP0000002A1")); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	got, err := repo.ListPatents(ctx, store.PatentQuery{Search: "ep00000"})
	if err != nil {
		t.Fatalf("ListPatents search: %v", err)
	}
	if len(got) != 1 || got[0].Number.Country != "EP" {
		t.Fatalf("search returned %v, want one EP patent", got)
	}
}

func TestRelations(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	from := domain.MustParsePatentNumber("US0000001A1")
	to := domain.MustParsePatentNumber("US0000002A1")
	rel := domain.Relation{From: from, To: to, Kind: domain.RelationCites}

	if err := repo.SaveRelation(ctx, rel); err != nil {
		t.Fatalf("SaveRelation: %v", err)
	}
	// Duplicate insert must be a no-op, not an error.
	if err := repo.SaveRelation(ctx, rel); err != nil {
		t.Fatalf("SaveRelation duplicate: %v", err)
	}
	got, err := repo.Relations(ctx, from, domain.RelationCites)
	if err != nil {
		t.Fatalf("Relations: %v", err)
	}
	if len(got) != 1 || !got[0].To.Equal(to) {
		t.Fatalf("Relations = %v, want one edge to %v", got, to)
	}
	none, err := repo.Relations(ctx, from, domain.RelationParent)
	if err != nil {
		t.Fatalf("Relations parent: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected no parent relations, got %v", none)
	}
}

func TestBidirectionalRelations(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	n1 := domain.MustParsePatentNumber("US0000001A1")
	n2 := domain.MustParsePatentNumber("US0000002A1")

	if err := repo.SavePatent(ctx, samplePatent(n1.String())); err != nil {
		t.Fatal(err)
	}
	if err := repo.SavePatent(ctx, samplePatent(n2.String())); err != nil {
		t.Fatal(err)
	}

	// Save A cites B
	if err := repo.SaveRelation(ctx, domain.Relation{From: n1, To: n2, Kind: domain.RelationCites}); err != nil {
		t.Fatal(err)
	}

	// Query citations of A -> should find B
	cites, err := repo.ListPatents(ctx, store.PatentQuery{Relation: n1, RelationKind: domain.RelationCites})
	if err != nil {
		t.Fatal(err)
	}
	if len(cites) != 1 || cites[0].Number != n2 {
		t.Errorf("citations of A: got %v, want [%v]", cites, n2)
	}

	// Query cited_by of B -> should find A
	citedBy, err := repo.ListPatents(ctx, store.PatentQuery{Relation: n2, RelationKind: domain.RelationCitedBy})
	if err != nil {
		t.Fatal(err)
	}
	if len(citedBy) != 1 || citedBy[0].Number != n1 {
		t.Errorf("cited_by of B: got %v, want [%v]", citedBy, n1)
	}
}

func TestProjectAndMembership(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	project := domain.Project{ID: "proj-1", Name: "Litigation A", CreatedAt: time.Now()}
	if err := repo.SaveProject(ctx, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	patent := samplePatent("US11611785B2")
	if err := repo.SavePatent(ctx, patent); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	if err := repo.AddMembership(ctx, domain.Membership{
		Project:     project.ID,
		Patent:      patent.Number,
		ReviewState: domain.ReviewStateStored,
		AddedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}

	if err := repo.SetReviewState(ctx, project.ID, patent.Number, domain.ReviewStateUnderReview); err != nil {
		t.Fatalf("SetReviewState: %v", err)
	}
	members, err := repo.Memberships(ctx, project.ID)
	if err != nil {
		t.Fatalf("Memberships: %v", err)
	}
	if len(members) != 1 || members[0].ReviewState != domain.ReviewStateUnderReview {
		t.Fatalf("Memberships = %v, want one under_review", members)
	}

	// Listing a project's patents filtered by state.
	page, err := repo.ListPatents(ctx, store.PatentQuery{
		Project:     project.ID,
		ReviewState: domain.ReviewStateUnderReview,
	})
	if err != nil {
		t.Fatalf("ListPatents by project: %v", err)
	}
	if len(page) != 1 {
		t.Fatalf("project listing = %d patents, want 1", len(page))
	}

	// State change for an absent membership reports ErrNotFound.
	err = repo.SetReviewState(ctx, project.ID,
		domain.MustParsePatentNumber("US9999999B2"), domain.ReviewStateIgnored)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("SetReviewState absent: err = %v, want ErrNotFound", err)
	}
}

func TestMigrationsAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reopen.db")
	ctx := context.Background()

	repo1, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open first: %v", err)
	}
	if err := repo1.SavePatent(ctx, samplePatent("US0000001A1")); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	if err := repo1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopening must not re-run migrations or lose data.
	repo2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}
	defer func() { _ = repo2.Close() }()
	if _, err := repo2.Patent(ctx, domain.MustParsePatentNumber("US0000001A1")); err != nil {
		t.Fatalf("data lost after reopen: %v", err)
	}
}

func TestTagStore(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	// Tags and assignments need an existing project and patent (foreign keys).
	project := domain.Project{ID: "p1", Name: "Project One", CreatedAt: time.Now().UTC()}
	if err := repo.SaveProject(ctx, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	patent := samplePatent("US11611785B2")
	if err := repo.SavePatent(ctx, patent); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}

	// CreateTag is idempotent on (project, name): a second call returns the row.
	tag, err := repo.CreateTag(ctx, project.ID, "prior-art")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	again, err := repo.CreateTag(ctx, project.ID, "prior-art")
	if err != nil {
		t.Fatalf("CreateTag (repeat): %v", err)
	}
	if again.ID != tag.ID {
		t.Fatalf("CreateTag not idempotent: id %d then %d", tag.ID, again.ID)
	}
	byName, err := repo.TagByName(ctx, project.ID, "PRIOR-ART")
	if err != nil {
		t.Fatalf("TagByName: %v", err)
	}
	if byName.ID != tag.ID {
		t.Fatalf("TagByName ID = %d, want %d", byName.ID, tag.ID)
	}

	// A tag assigned to a patent reads back through PatentTags.
	changed, err := repo.TagPatent(ctx, tag.ID, patent.Number, time.Now().UTC())
	if err != nil {
		t.Fatalf("TagPatent: %v", err)
	}
	if !changed {
		t.Fatal("TagPatent changed = false, want true")
	}
	changed, err = repo.TagPatent(ctx, tag.ID, patent.Number, time.Now().UTC())
	if err != nil {
		t.Fatalf("TagPatent (repeat): %v", err) // a duplicate assignment is a no-op
	}
	if changed {
		t.Fatal("TagPatent repeat changed = true, want false")
	}
	tags, err := repo.PatentTags(ctx, project.ID, patent.Number)
	if err != nil {
		t.Fatalf("PatentTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "prior-art" {
		t.Fatalf("PatentTags = %v, want one prior-art tag", tags)
	}
	oneTag, err := repo.PatentTag(ctx, project.ID, patent.Number, "PRIOR-ART")
	if err != nil {
		t.Fatalf("PatentTag: %v", err)
	}
	if oneTag.ID != tag.ID || oneTag.AssignedAt.IsZero() {
		t.Fatalf("PatentTag = %+v, want tag %d with AssignedAt", oneTag, tag.ID)
	}

	// Removing the tag clears the assignment but keeps the tag itself.
	changed, err = repo.UntagPatent(ctx, tag.ID, patent.Number)
	if err != nil {
		t.Fatalf("UntagPatent: %v", err)
	}
	if !changed {
		t.Fatal("UntagPatent changed = false, want true")
	}
	changed, err = repo.UntagPatent(ctx, tag.ID, patent.Number)
	if err != nil {
		t.Fatalf("UntagPatent repeat: %v", err)
	}
	if changed {
		t.Fatal("UntagPatent repeat changed = true, want false")
	}
	tags, err = repo.PatentTags(ctx, project.ID, patent.Number)
	if err != nil {
		t.Fatalf("PatentTags after untag: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("PatentTags = %v, want empty after untag", tags)
	}
	all, err := repo.ProjectTags(ctx, project.ID)
	if err != nil {
		t.Fatalf("ProjectTags: %v", err)
	}
	if len(all) != 1 || all[0].Name != "prior-art" {
		t.Fatalf("ProjectTags = %v, want one prior-art tag", all)
	}
}

func TestIDSEntryStoreAndPatentListing(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	project := domain.Project{ID: "p1", Name: "Project One", CreatedAt: time.Now().UTC()}
	if err := repo.SaveProject(ctx, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	patent := samplePatent("US11611785B2")
	if err := repo.SavePatent(ctx, patent); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	if err := repo.AddMembership(ctx, domain.Membership{Project: project.ID, Patent: patent.Number, ReviewState: domain.ReviewStateCached, AddedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}

	entry, err := repo.SaveIDSEntry(ctx, domain.IDSEntry{
		Project:          project.ID,
		Patent:           patent.Number,
		KindCode:         "B2",
		CountryCode:      "US",
		RelevantPassages: "col. 1",
		Notes:            "primary reference",
		Status:           domain.IDSEntrySubmitted,
	})
	if err != nil {
		t.Fatalf("SaveIDSEntry: %v", err)
	}
	if entry.ID == 0 {
		t.Fatal("SaveIDSEntry should assign an ID")
	}

	got, err := repo.IDSEntry(ctx, project.ID, patent.Number)
	if err != nil {
		t.Fatalf("IDSEntry: %v", err)
	}
	if got.Status != domain.IDSEntrySubmitted || got.CountryCode != "US" {
		t.Fatalf("IDSEntry = %+v, want submitted US entry", got)
	}

	rows, err := repo.ListPatents(ctx, store.PatentQuery{Project: project.ID})
	if err != nil {
		t.Fatalf("ListPatents: %v", err)
	}
	if len(rows) != 1 || rows[0].IDSEntry == nil {
		t.Fatalf("ListPatents IDS row = %+v, want IDS entry", rows)
	}
	if rows[0].IDSEntry.Status != domain.IDSEntrySubmitted {
		t.Fatalf("row IDS status = %q, want submitted", rows[0].IDSEntry.Status)
	}

	if err := repo.DeleteIDSEntry(ctx, project.ID, patent.Number); err != nil {
		t.Fatalf("DeleteIDSEntry: %v", err)
	}
	if _, err := repo.IDSEntry(ctx, project.ID, patent.Number); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("IDSEntry after delete: err = %v, want ErrNotFound", err)
	}
}
