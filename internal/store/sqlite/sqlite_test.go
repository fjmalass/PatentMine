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
		Inventors:  []string{"Ada Lovelace", "Alan Turing"},
		FetchState: domain.FetchCached,
		Source:     domain.SourceFixture,
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
		Project: project.ID,
		Patent:  patent.Number,
		State:   domain.MembershipStored,
		AddedAt: time.Now(),
	}); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}

	if err := repo.SetMembershipState(ctx, project.ID, patent.Number, domain.MembershipUnderReview); err != nil {
		t.Fatalf("SetMembershipState: %v", err)
	}
	members, err := repo.Memberships(ctx, project.ID)
	if err != nil {
		t.Fatalf("Memberships: %v", err)
	}
	if len(members) != 1 || members[0].State != domain.MembershipUnderReview {
		t.Fatalf("Memberships = %v, want one under_review", members)
	}

	// Listing a project's patents filtered by state.
	page, err := repo.ListPatents(ctx, store.PatentQuery{
		Project: project.ID,
		State:   domain.MembershipUnderReview,
	})
	if err != nil {
		t.Fatalf("ListPatents by project: %v", err)
	}
	if len(page) != 1 {
		t.Fatalf("project listing = %d patents, want 1", len(page))
	}

	// State change for an absent membership reports ErrNotFound.
	err = repo.SetMembershipState(ctx, project.ID,
		domain.MustParsePatentNumber("US9999999B2"), domain.MembershipIgnored)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("SetMembershipState absent: err = %v, want ErrNotFound", err)
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
