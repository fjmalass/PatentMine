package sqlite

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
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

func openTestRepoWithMetrics(t testing.TB) (*Repo, *observability.Metrics) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	metrics := observability.NewMetrics()
	repo, err := OpenWithMetrics(context.Background(), path, metrics)
	if err != nil {
		t.Fatalf("OpenWithMetrics: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo, metrics
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

	// Relation endpoints are foreign keys into patent, so both records must
	// exist before an edge between them can be saved.
	if err := repo.SavePatent(ctx, samplePatent("US0000001A1")); err != nil {
		t.Fatalf("SavePatent from: %v", err)
	}
	if err := repo.SavePatent(ctx, samplePatent("US0000002A1")); err != nil {
		t.Fatalf("SavePatent to: %v", err)
	}
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
		ReviewState: domain.ReviewStateUnknown,
		AddedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}

	if err := repo.SetReviewStates(ctx, project.ID, []domain.PatentNumber{patent.Number}, domain.ReviewStateUnderReview); err != nil {
		t.Fatalf("SetReviewStates: %v", err)
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

func TestPatentClassifications(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	p1 := samplePatent("US0000001A1")
	p1.Classifications = []string{"G06F16/245", "A61K31/00"}
	if err := repo.SavePatent(ctx, p1); err != nil {
		t.Fatalf("SavePatent p1: %v", err)
	}

	p2 := samplePatent("US0000002A1")
	p2.Classifications = []string{"A61K31/00", "B64C27/00"}
	if err := repo.SavePatent(ctx, p2); err != nil {
		t.Fatalf("SavePatent p2: %v", err)
	}

	// 1. Verify roundtrip in Patent
	got1, err := repo.Patent(ctx, p1.Number)
	if err != nil {
		t.Fatalf("Patent p1: %v", err)
	}
	if len(got1.Classifications) != 2 || got1.Classifications[0] != "G06F16/245" || got1.Classifications[1] != "A61K31/00" {
		t.Fatalf("p1 Classifications roundtrip failed: %v", got1.Classifications)
	}

	// 2. Verify roundtrip in ListPatents
	rows, err := repo.ListPatents(ctx, store.PatentQuery{})
	if err != nil {
		t.Fatalf("ListPatents: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 patents, got %d", len(rows))
	}
	// Verify p1 row classifications
	var r1 domain.PatentRow
	for _, r := range rows {
		if r.Number == p1.Number {
			r1 = r
		}
	}
	if len(r1.Classifications) != 2 || r1.Classifications[0] != "G06F16/245" {
		t.Fatalf("r1 row Classifications roundtrip failed: %v", r1.Classifications)
	}

	// 3. Verify filtering by Classification prefix
	res, err := repo.ListPatents(ctx, store.PatentQuery{Classification: "G06F"})
	if err != nil {
		t.Fatalf("ListPatents filter: %v", err)
	}
	if len(res) != 1 || res[0].Number != p1.Number {
		t.Fatalf("filtering by G06F: got %v, want p1", res)
	}

	res, err = repo.ListPatents(ctx, store.PatentQuery{Classification: "A61K"})
	if err != nil {
		t.Fatalf("ListPatents filter: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("filtering by A61K: got %v, want both p1 and p2", res)
	}

	res, err = repo.ListPatents(ctx, store.PatentQuery{Classification: "B64C27/00"})
	if err != nil {
		t.Fatalf("ListPatents filter: %v", err)
	}
	if len(res) != 1 || res[0].Number != p2.Number {
		t.Fatalf("filtering by B64C27/00: got %v, want p2", res)
	}

	res, err = repo.ListPatents(ctx, store.PatentQuery{Classification: "H01L"})
	if err != nil {
		t.Fatalf("ListPatents filter: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("filtering by non-matching H01L: got %v, want empty", res)
	}

	// 4. Verify multiple classifications AND filtering (space-separated)
	res, err = repo.ListPatents(ctx, store.PatentQuery{Classification: "G06F A61K"})
	if err != nil {
		t.Fatalf("ListPatents filter: %v", err)
	}
	if len(res) != 1 || res[0].Number != p1.Number {
		t.Fatalf("filtering by multiple G06F A61K (AND): got %v, want p1", res)
	}

	// 5. Verify multiple classifications AND filtering (comma-separated)
	res, err = repo.ListPatents(ctx, store.PatentQuery{Classification: "G06F, A61K"})
	if err != nil {
		t.Fatalf("ListPatents filter: %v", err)
	}
	if len(res) != 1 || res[0].Number != p1.Number {
		t.Fatalf("filtering by multiple G06F, A61K (AND): got %v, want p1", res)
	}

	// 6. Verify multiple classifications AND filtering returns empty when no patent matches all
	res, err = repo.ListPatents(ctx, store.PatentQuery{Classification: "G06F B64C"})
	if err != nil {
		t.Fatalf("ListPatents filter: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("filtering by multiple G06F B64C (AND): got %v, want empty", res)
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
	err = repo.TagPatents(ctx, tag.ID, []domain.PatentNumber{patent.Number}, time.Now().UTC())
	if err != nil {
		t.Fatalf("TagPatents: %v", err)
	}
	err = repo.TagPatents(ctx, tag.ID, []domain.PatentNumber{patent.Number}, time.Now().UTC())
	if err != nil {
		t.Fatalf("TagPatents (repeat): %v", err) // a duplicate assignment is a no-op
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
	err = repo.UntagPatents(ctx, tag.ID, []domain.PatentNumber{patent.Number})
	if err != nil {
		t.Fatalf("UntagPatents: %v", err)
	}
	err = repo.UntagPatents(ctx, tag.ID, []domain.PatentNumber{patent.Number})
	if err != nil {
		t.Fatalf("UntagPatents repeat: %v", err)
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

func TestPatentFilterExpressionSupportsBooleanLogic(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	project := domain.Project{ID: "p1", Name: "Project One", CreatedAt: time.Now().UTC()}
	if err := repo.SaveProject(ctx, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	p1 := samplePatent("US0000001A1")
	p1.Classifications = []string{"S04A", "A61K"}
	p2 := samplePatent("US0000002A1")
	p2.Classifications = []string{"S04B"}
	p3 := samplePatent("US0000003A1")
	p3.Classifications = []string{"H01L"}
	for _, p := range []domain.Patent{p1, p2, p3} {
		if err := repo.SavePatent(ctx, p); err != nil {
			t.Fatalf("SavePatent(%s): %v", p.Number, err)
		}
	}
	for patent, state := range map[domain.PatentNumber]domain.ReviewState{
		p1.Number: domain.ReviewStateUnknown,
		p2.Number: domain.ReviewStateUnderReview,
		p3.Number: domain.ReviewStateUnknown,
	} {
		if err := repo.AddMembership(ctx, domain.Membership{Project: project.ID, Patent: patent, ReviewState: state, AddedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("AddMembership(%s): %v", patent, err)
		}
	}
	priorArt, err := repo.CreateTag(ctx, project.ID, "prior_art")
	if err != nil {
		t.Fatalf("CreateTag prior_art: %v", err)
	}
	blocker, err := repo.CreateTag(ctx, project.ID, "blocker")
	if err != nil {
		t.Fatalf("CreateTag blocker: %v", err)
	}
	if err := repo.TagPatents(ctx, priorArt.ID, []domain.PatentNumber{p1.Number, p2.Number}, time.Now().UTC()); err != nil {
		t.Fatalf("TagPatents prior_art: %v", err)
	}
	if err := repo.TagPatents(ctx, blocker.ID, []domain.PatentNumber{p1.Number}, time.Now().UTC()); err != nil {
		t.Fatalf("TagPatents blocker: %v", err)
	}
	if _, err := repo.SaveIDSEntry(ctx, domain.IDSEntry{Project: project.ID, Patent: p1.Number, Status: domain.IDSEntryPending}); err != nil {
		t.Fatalf("SaveIDSEntry p1: %v", err)
	}
	if _, err := repo.SaveIDSEntry(ctx, domain.IDSEntry{Project: project.ID, Patent: p2.Number, Status: domain.IDSEntryIgnored}); err != nil {
		t.Fatalf("SaveIDSEntry p2: %v", err)
	}

	rows, err := repo.ListPatents(ctx, store.PatentQuery{Project: project.ID, Filter: "tag:prior_art and tag:blocker"})
	if err != nil {
		t.Fatalf("ListPatents tag and tag: %v", err)
	}
	if len(rows) != 1 || rows[0].Number != p1.Number {
		t.Fatalf("tag and tag rows = %v, want only p1", rows)
	}

	rows, err = repo.ListPatents(ctx, store.PatentQuery{Project: project.ID, Filter: "(tag:prior_art or tag:blocker) and not state:under_review"})
	if err != nil {
		t.Fatalf("ListPatents boolean filter: %v", err)
	}
	if len(rows) != 1 || rows[0].Number != p1.Number {
		t.Fatalf("boolean filter rows = %v, want only p1", rows)
	}

	rows, err = repo.ListPatents(ctx, store.PatentQuery{Project: project.ID, Filter: "ids_status:pending"})
	if err != nil {
		t.Fatalf("ListPatents ids_status pending: %v", err)
	}
	if len(rows) != 1 || rows[0].Number != p1.Number {
		t.Fatalf("ids_status pending rows = %v, want only p1", rows)
	}

	rows, err = repo.ListPatents(ctx, store.PatentQuery{Project: project.ID, Filter: "ids_status:none"})
	if err != nil {
		t.Fatalf("ListPatents ids_status none: %v", err)
	}
	if len(rows) != 1 || rows[0].Number != p3.Number {
		t.Fatalf("ids_status none rows = %v, want only p3", rows)
	}

	rows, err = repo.ListPatents(ctx, store.PatentQuery{Project: project.ID, IDSStatus: "ignored"})
	if err != nil {
		t.Fatalf("ListPatents IDSStatus ignored: %v", err)
	}
	if len(rows) != 1 || rows[0].Number != p2.Number {
		t.Fatalf("IDSStatus ignored rows = %v, want only p2", rows)
	}

	rows, err = repo.ListPatents(ctx, store.PatentQuery{Filter: "class:S04*"})
	if err != nil {
		t.Fatalf("ListPatents class wildcard: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("class wildcard rows = %v, want 2", rows)
	}

	rows, err = repo.ListPatents(ctx, store.PatentQuery{Filter: `search:"method for US0000001A1" and inventor:"Ada Lovelace"`})
	if err != nil {
		t.Fatalf("ListPatents search and inventor: %v", err)
	}
	if len(rows) != 1 || rows[0].Number != p1.Number {
		t.Fatalf("search and inventor rows = %v, want only p1", rows)
	}

	rows, err = repo.ListPatents(ctx, store.PatentQuery{Filter: `inventor:*Lovelace*`})
	if err != nil {
		t.Fatalf("ListPatents inventor suffix wildcard: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("inventor suffix wildcard rows = %v, want 3", rows)
	}

	rows, err = repo.ListPatents(ctx, store.PatentQuery{Filter: `inventor:"Ada*"`})
	if err != nil {
		t.Fatalf("ListPatents inventor prefix wildcard: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("inventor prefix wildcard rows = %v, want 3", rows)
	}

	rows, err = repo.ListPatents(ctx, store.PatentQuery{Filter: `assignee:"Acme*"`})
	if err != nil {
		t.Fatalf("ListPatents assignee prefix wildcard: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("assignee prefix wildcard rows = %v, want 3", rows)
	}

	rows, err = repo.ListPatents(ctx, store.PatentQuery{Filter: `assignee:*Corp`})
	if err != nil {
		t.Fatalf("ListPatents assignee suffix wildcard: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("assignee suffix wildcard rows = %v, want 3", rows)
	}

	if _, err := repo.ListPatents(ctx, store.PatentQuery{Filter: "not state:under_review"}); err == nil {
		t.Fatal("expected state filter without project to fail")
	}
	if _, err := repo.ListPatents(ctx, store.PatentQuery{Filter: "ids_status:pending"}); err == nil {
		t.Fatal("expected ids_status filter without project to fail")
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
	if err := repo.AddMembership(ctx, domain.Membership{Project: project.ID, Patent: patent.Number, ReviewState: domain.ReviewStateUnknown, AddedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}

	entry, err := repo.SaveIDSEntry(ctx, domain.IDSEntry{
		Project:          project.ID,
		Patent:           patent.Number,
		KindCode:         "B2",
		RelevantPassages: "col. 1",
		Notes:            "primary reference",
		Status:           domain.IDSEntrySubmitted,
		SubmittedAt:      time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
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
	if got.Status != domain.IDSEntrySubmitted || got.KindCode != "B2" {
		t.Fatalf("IDSEntry = %+v, want submitted B2 entry", got)
	}
	if got.SubmittedAt.IsZero() {
		t.Fatalf("IDSEntry.SubmittedAt = zero, want stamped on submit")
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

func TestPatentNoteStoreRoundTrip(t *testing.T) {
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

	note, err := repo.SavePatentNote(ctx, domain.PatentNote{
		Project:  project.ID,
		Patent:   patent.Number,
		Markdown: "# Summary\n\n- first note",
	})
	if err != nil {
		t.Fatalf("SavePatentNote: %v", err)
	}
	if note.AddedAt.IsZero() || note.UpdatedAt.IsZero() {
		t.Fatalf("saved note timestamps = %+v, want both set", note)
	}

	got, err := repo.PatentNote(ctx, project.ID, patent.Number)
	if err != nil {
		t.Fatalf("PatentNote: %v", err)
	}
	if got.Markdown != "# Summary\n\n- first note" {
		t.Fatalf("PatentNote markdown = %q", got.Markdown)
	}

	if err := repo.DeletePatentNote(ctx, project.ID, patent.Number); err != nil {
		t.Fatalf("DeletePatentNote: %v", err)
	}
	if _, err := repo.PatentNote(ctx, project.ID, patent.Number); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("PatentNote after delete: err = %v, want ErrNotFound", err)
	}
}

func TestListPatentsSortByTags(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	project, p1, p2 := seedTagSortFixture(t, repo, ctx, 2)

	rows, err := repo.ListPatents(ctx, store.PatentQuery{Project: project.ID, SortColumn: domain.SortByTags, SortAscending: true})
	if err != nil {
		t.Fatalf("ListPatents sort asc: %v", err)
	}
	if len(rows) != 2 || rows[0].Number != p2.Number || rows[1].Number != p1.Number {
		t.Fatalf("tag asc order = %+v, want alpha then beta", rows)
	}
	rows, err = repo.ListPatents(ctx, store.PatentQuery{Project: project.ID, SortColumn: domain.SortByTags, SortAscending: false})
	if err != nil {
		t.Fatalf("ListPatents sort desc: %v", err)
	}
	if len(rows) != 2 || rows[0].Number != p1.Number || rows[1].Number != p2.Number {
		t.Fatalf("tag desc order = %+v, want beta then alpha", rows)
	}
}

func TestListPatentsSortByTagsMetrics(t *testing.T) {
	repo, metrics := openTestRepoWithMetrics(t)
	ctx := context.Background()
	project, _, _ := seedTagSortFixture(t, repo, ctx, 3)
	if _, err := repo.ListPatents(ctx, store.PatentQuery{Project: project.ID, SortColumn: domain.SortByTags, SortAscending: true, Limit: 2, Offset: 1}); err != nil {
		t.Fatalf("ListPatents sort asc: %v", err)
	}
	snap := metrics.Snapshot()
	if snap.Counters["store.sqlite.list_patents.sort_tags_total"] != 1 {
		t.Fatalf("sort_tags_total = %d, want 1", snap.Counters["store.sqlite.list_patents.sort_tags_total"])
	}
	if snap.Timings["store.sqlite.list_patents.sort_tags"].Count != 1 {
		t.Fatalf("sort_tags timing = %+v, want count 1", snap.Timings["store.sqlite.list_patents.sort_tags"])
	}
	if snap.Gauges["store.sqlite.list_patents.sort_tags.limit"] != 2 {
		t.Fatalf("sort_tags limit gauge = %d, want 2", snap.Gauges["store.sqlite.list_patents.sort_tags.limit"])
	}
	if snap.Gauges["store.sqlite.list_patents.sort_tags.offset"] != 1 {
		t.Fatalf("sort_tags offset gauge = %d, want 1", snap.Gauges["store.sqlite.list_patents.sort_tags.offset"])
	}
}

func BenchmarkListPatentsSortByTags(b *testing.B) {
	repo, _ := openTestRepoWithMetrics(b)
	ctx := context.Background()
	project, _, _ := seedTagSortFixture(b, repo, ctx, 250)
	q := store.PatentQuery{Project: project.ID, SortColumn: domain.SortByTags, SortAscending: true, Limit: 100, Offset: 0}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := repo.ListPatents(ctx, q)
		if err != nil {
			b.Fatalf("ListPatents: %v", err)
		}
		if len(rows) == 0 {
			b.Fatal("expected rows")
		}
	}
}

func seedTagSortFixture(t testing.TB, repo *Repo, ctx context.Context, count int) (domain.Project, domain.Patent, domain.Patent) {
	t.Helper()
	project := domain.Project{ID: "p1", Name: "Project One", CreatedAt: time.Now().UTC()}
	if err := repo.SaveProject(ctx, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	alpha, err := repo.CreateTag(ctx, project.ID, "alpha")
	if err != nil {
		t.Fatalf("CreateTag alpha: %v", err)
	}
	beta, err := repo.CreateTag(ctx, project.ID, "beta")
	if err != nil {
		t.Fatalf("CreateTag beta: %v", err)
	}
	var first, second domain.Patent
	for i := 0; i < count; i++ {
		number := domain.MustParsePatentNumber("US" + sevenDigits(i+1) + "A1")
		p := samplePatent(number.String())
		if err := repo.SavePatent(ctx, p); err != nil {
			t.Fatalf("SavePatent(%s): %v", p.Number, err)
		}
		if err := repo.AddMembership(ctx, domain.Membership{Project: project.ID, Patent: p.Number, ReviewState: domain.ReviewStateUnknown, AddedAt: time.Now().UTC()}); err != nil {
			t.Fatalf("AddMembership(%s): %v", p.Number, err)
		}
		tagID := alpha.ID
		if i%2 == 0 {
			tagID = beta.ID
		}
		if err := repo.TagPatents(ctx, tagID, []domain.PatentNumber{p.Number}, time.Now().UTC()); err != nil {
			t.Fatalf("TagPatents(%s): %v", p.Number, err)
		}
		if i == 0 {
			first = p
		}
		if i == 1 {
			second = p
		}
	}
	return project, first, second
}

func sevenDigits(n int) string {
	return fmt.Sprintf("%07d", n)
}

func TestBatchOperationsStore(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	// 1. Setup project and multiple patents.
	project := domain.Project{ID: "p1", Name: "Project One", CreatedAt: time.Now().UTC()}
	if err := repo.SaveProject(ctx, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}

	p1 := samplePatent("US11611785B2")
	p2 := samplePatent("US0000001B2")
	p3 := samplePatent("US0000002B2")

	for _, p := range []domain.Patent{p1, p2, p3} {
		if err := repo.SavePatent(ctx, p); err != nil {
			t.Fatalf("SavePatent: %v", err)
		}
		if err := repo.AddMembership(ctx, domain.Membership{
			Project:     project.ID,
			Patent:      p.Number,
			ReviewState: domain.ReviewStateUnknown,
			AddedAt:     time.Now().UTC(),
		}); err != nil {
			t.Fatalf("AddMembership: %v", err)
		}
	}

	// 2. Verify SetReviewStates transactional batch updates.
	err := repo.SetReviewStates(ctx, project.ID, []domain.PatentNumber{p1.Number, p2.Number}, domain.ReviewStateUnderReview)
	if err != nil {
		t.Fatalf("SetReviewStates success case failed: %v", err)
	}

	// Verify states are updated.
	for _, num := range []domain.PatentNumber{p1.Number, p2.Number} {
		m, err := repo.Membership(ctx, project.ID, num)
		if err != nil {
			t.Fatalf("Membership: %v", err)
		}
		if m.ReviewState != domain.ReviewStateUnderReview {
			t.Fatalf("expected state %s, got %s", domain.ReviewStateUnderReview, m.ReviewState)
		}
	}
	// p3 state should remain Unknown.
	m3, err := repo.Membership(ctx, project.ID, p3.Number)
	if err != nil {
		t.Fatalf("Membership: %v", err)
	}
	if m3.ReviewState != domain.ReviewStateUnknown {
		t.Fatalf("expected state %s, got %s", domain.ReviewStateUnknown, m3.ReviewState)
	}

	// 3. Verify SetReviewStates transactional safety (rollback).
	err = repo.SetReviewStates(ctx, project.ID, []domain.PatentNumber{p1.Number, p2.Number}, domain.ReviewState("INVALID_STATE"))
	if err == nil {
		t.Fatal("expected failure when setting invalid review state")
	}

	// 4. Verify TagPatents and UntagPatents transactional batch updates.
	tag, err := repo.CreateTag(ctx, project.ID, "test-tag")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}

	err = repo.TagPatents(ctx, tag.ID, []domain.PatentNumber{p1.Number, p2.Number}, time.Now().UTC())
	if err != nil {
		t.Fatalf("TagPatents failed: %v", err)
	}

	// Verify they are tagged.
	for _, num := range []domain.PatentNumber{p1.Number, p2.Number} {
		tags, err := repo.PatentTags(ctx, project.ID, num)
		if err != nil {
			t.Fatalf("PatentTags: %v", err)
		}
		if len(tags) != 1 || tags[0].Name != "test-tag" {
			t.Fatalf("expected 1 tag 'test-tag' for patent %s, got %v", num, tags)
		}
	}

	// Verify rollback on TagPatents with invalid tag ID (violating foreign key constraint).
	err = repo.TagPatents(ctx, 99999, []domain.PatentNumber{p3.Number}, time.Now().UTC())
	if err == nil {
		t.Fatal("expected error on foreign key violation for invalid tag ID, but got nil")
	}

	// Verify UntagPatents.
	err = repo.UntagPatents(ctx, tag.ID, []domain.PatentNumber{p1.Number, p2.Number})
	if err != nil {
		t.Fatalf("UntagPatents failed: %v", err)
	}

	// Verify they are untagged.
	for _, num := range []domain.PatentNumber{p1.Number, p2.Number} {
		tags, err := repo.PatentTags(ctx, project.ID, num)
		if err != nil {
			t.Fatalf("PatentTags: %v", err)
		}
		if len(tags) != 0 {
			t.Fatalf("expected 0 tags after untagging, got %v", tags)
		}
	}
}

func TestPatentInventorStats(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	// 1. Create a project
	project := domain.Project{
		ID:        "proj-1",
		Name:      "Test Project",
		CreatedAt: time.Now().UTC(),
	}
	if err := repo.SaveProject(ctx, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}

	// 2. Save patents with overlap in inventors
	p1 := samplePatent("US11611785B2") // Inventors: Ada Lovelace, Alan Turing
	p1.FetchState = domain.FetchStub
	p1.Inventors = []domain.Inventor{"Ada Lovelace", "Alan Turing"}
	if err := repo.SavePatent(ctx, p1); err != nil {
		t.Fatalf("SavePatent p1: %v", err)
	}

	p2 := samplePatent("US11611786B2") // Inventors: Ada Lovelace, Charles Babbage
	p2.FetchState = domain.FetchStub
	p2.Inventors = []domain.Inventor{"Ada Lovelace", "Charles Babbage"}
	if err := repo.SavePatent(ctx, p2); err != nil {
		t.Fatalf("SavePatent p2: %v", err)
	}

	// 3. Add memberships to project
	if err := repo.AddMembership(ctx, domain.Membership{
		Project:     project.ID,
		Patent:      p1.Number,
		ReviewState: domain.ReviewStateUnknown,
		AddedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AddMembership p1: %v", err)
	}

	if err := repo.AddMembership(ctx, domain.Membership{
		Project:     project.ID,
		Patent:      p2.Number,
		ReviewState: domain.ReviewStateUnderReview,
		AddedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AddMembership p2: %v", err)
	}

	// 4. Create tag and assign to p1
	tag, err := repo.CreateTag(ctx, project.ID, "seminal")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := repo.TagPatents(ctx, tag.ID, []domain.PatentNumber{p1.Number}, time.Now().UTC()); err != nil {
		t.Fatalf("TagPatents: %v", err)
	}

	// 5. Query stats for inventors: Ada Lovelace, Alan Turing
	stats, err := repo.PatentInventorStats(ctx, project.ID, []domain.Inventor{"Ada Lovelace", "Alan Turing"})
	if err != nil {
		t.Fatalf("PatentInventorStats: %v", err)
	}

	if len(stats) != 2 {
		t.Fatalf("expected 2 inventor stats, got %d", len(stats))
	}

	// Ada Lovelace should have: Total: 2, States: unknown=1, under_review=1, Tags: seminal=1
	ada := stats[0]
	if ada.Inventor != "Ada Lovelace" {
		t.Errorf("expected Ada Lovelace, got %s", ada.Inventor)
	}
	if ada.Total != 2 {
		t.Errorf("Ada Lovelace expected 2 total patents, got %d", ada.Total)
	}
	if ada.States["unknown"] != 1 || ada.States["under_review"] != 1 {
		t.Errorf("Ada Lovelace expected unknown=1, under_review=1 review states, got: %v", ada.States)
	}
	if ada.Tags["seminal"] != 1 {
		t.Errorf("Ada Lovelace expected tag seminal=1, got: %v", ada.Tags)
	}

	// Alan Turing should have: Total: 1, States: unknown=1, Tags: seminal=1
	alan := stats[1]
	if alan.Inventor != "Alan Turing" {
		t.Errorf("expected Alan Turing, got %s", alan.Inventor)
	}
	if alan.Total != 1 {
		t.Errorf("Alan Turing expected 1 total patent, got %d", alan.Total)
	}
	if alan.States["unknown"] != 1 {
		t.Errorf("Alan Turing expected unknown=1, got: %v", alan.States)
	}
	if alan.Tags["seminal"] != 1 {
		t.Errorf("Alan Turing expected tag seminal=1, got: %v", alan.Tags)
	}
}

func TestPatentAssigneeStats(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	project := domain.Project{ID: "proj-1", Name: "Test Project", CreatedAt: time.Now().UTC()}
	if err := repo.SaveProject(ctx, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}

	p1 := samplePatent("US11611785B2")
	p1.Assignee = "Acme Corp"
	p1.FetchState = domain.FetchStub
	p2 := samplePatent("US11611786B2")
	p2.Assignee = "Acme Corp"
	p2.FetchState = domain.FetchStub
	p3 := samplePatent("US11611787B2")
	p3.Assignee = "Beta Labs"
	p3.FetchState = domain.FetchStub
	for _, p := range []domain.Patent{p1, p2, p3} {
		if err := repo.SavePatent(ctx, p); err != nil {
			t.Fatalf("SavePatent(%s): %v", p.Number, err)
		}
	}
	if err := repo.AddMembership(ctx, domain.Membership{Project: project.ID, Patent: p1.Number, ReviewState: domain.ReviewStateUnknown, AddedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("AddMembership p1: %v", err)
	}
	if err := repo.AddMembership(ctx, domain.Membership{Project: project.ID, Patent: p2.Number, ReviewState: domain.ReviewStateUnderReview, AddedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("AddMembership p2: %v", err)
	}
	tag, err := repo.CreateTag(ctx, project.ID, "seminal")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := repo.TagPatents(ctx, tag.ID, []domain.PatentNumber{p1.Number}, time.Now().UTC()); err != nil {
		t.Fatalf("TagPatents: %v", err)
	}

	stats, err := repo.PatentAssigneeStats(ctx, project.ID)
	if err != nil {
		t.Fatalf("PatentAssigneeStats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 assignee stats row in project scope, got %d", len(stats))
	}
	if stats[0].Assignee != "Acme Corp" || stats[0].Total != 2 {
		t.Fatalf("top assignee = %+v, want Acme Corp with 2 patents", stats[0])
	}
	if stats[0].States["unknown"] != 1 || stats[0].States["under_review"] != 1 {
		t.Fatalf("Acme state counts = %v, want unknown=1 under_review=1", stats[0].States)
	}
	if stats[0].Tags["seminal"] != 1 {
		t.Fatalf("Acme tag counts = %v, want seminal=1", stats[0].Tags)
	}
}

func TestSaveMutationGroup(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	project := domain.Project{ID: "proj-1", Name: "Test Project", CreatedAt: time.Now().UTC()}
	if err := repo.SaveProject(ctx, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	patent := samplePatent("US11611785B2")
	if err := repo.SavePatent(ctx, patent); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	err := repo.SaveMutationGroup(ctx, domain.MutationGroup{
		ID:                "group-1",
		Project:           project.ID,
		Action:            "membership.set_state",
		CreatedAt:         time.Now().UTC(),
		SelectionSnapshot: []domain.PatentNumber{patent.Number},
		Attributes:        map[string]any{"target_state": domain.ReviewStateUnderReview},
	}, []domain.MutationItem{{Patent: patent.Number, Kind: "membership.state", Before: map[string]any{"state": domain.ReviewStateUnknown}, After: map[string]any{"state": domain.ReviewStateUnderReview}, Inverse: map[string]any{"state": domain.ReviewStateUnknown}}})
	if err != nil {
		t.Fatalf("SaveMutationGroup: %v", err)
	}

	var groups int
	if err := repo.reader.QueryRowContext(ctx, `SELECT COUNT(*) FROM mutation_group WHERE id = 'group-1'`).Scan(&groups); err != nil {
		t.Fatalf("count mutation_group: %v", err)
	}
	if groups != 1 {
		t.Fatalf("mutation_group rows = %d, want 1", groups)
	}
	var items int
	if err := repo.reader.QueryRowContext(ctx, `SELECT COUNT(*) FROM mutation_item WHERE group_id = 'group-1'`).Scan(&items); err != nil {
		t.Fatalf("count mutation_item: %v", err)
	}
	if items != 1 {
		t.Fatalf("mutation_item rows = %d, want 1", items)
	}
}

func TestPatentClassificationStats(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	project := domain.Project{ID: "proj-1", Name: "Test Project", CreatedAt: time.Now().UTC()}
	if err := repo.SaveProject(ctx, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	p1 := samplePatent("US11611785B2")
	p1.FetchState = domain.FetchStub
	p1.Classifications = []string{"G06F17/30", "H04N21/4345"}
	p2 := samplePatent("US11611786B2")
	p2.FetchState = domain.FetchStub
	p2.Classifications = []string{"G06F17/30"}
	p3 := samplePatent("US11611787B2")
	p3.FetchState = domain.FetchStub
	p3.Classifications = []string{"A61B5/00"}
	for _, p := range []domain.Patent{p1, p2, p3} {
		if err := repo.SavePatent(ctx, p); err != nil {
			t.Fatalf("SavePatent(%s): %v", p.Number, err)
		}
	}
	if err := repo.AddMembership(ctx, domain.Membership{Project: project.ID, Patent: p1.Number, ReviewState: domain.ReviewStateUnknown, AddedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("AddMembership p1: %v", err)
	}
	if err := repo.AddMembership(ctx, domain.Membership{Project: project.ID, Patent: p2.Number, ReviewState: domain.ReviewStateUnderReview, AddedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("AddMembership p2: %v", err)
	}
	tag, err := repo.CreateTag(ctx, project.ID, "seminal")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := repo.TagPatents(ctx, tag.ID, []domain.PatentNumber{p1.Number}, time.Now().UTC()); err != nil {
		t.Fatalf("TagPatents: %v", err)
	}
	if err := repo.SaveClassificationDefinition(ctx, domain.Classification{System: "CPC", Code: "G06F17/30", Description: "Information retrieval"}); err != nil {
		t.Fatalf("SaveClassificationDefinition: %v", err)
	}

	stats, err := repo.PatentClassificationStats(ctx, project.ID)
	if err != nil {
		t.Fatalf("PatentClassificationStats: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 classification stats rows, got %d", len(stats))
	}
	if stats[0].Classification.Code != "G06F17/30" || stats[0].Total != 2 {
		t.Fatalf("top classification = %+v, want G06F17/30 total=2", stats[0])
	}
	if stats[0].States["unknown"] != 1 || stats[0].States["under_review"] != 1 {
		t.Fatalf("classification state counts = %v, want unknown=1 under_review=1", stats[0].States)
	}
	if stats[0].Tags["seminal"] != 1 {
		t.Fatalf("classification tag counts = %v, want seminal=1", stats[0].Tags)
	}
	if stats[0].Classification.Description != "Information retrieval" {
		t.Fatalf("classification description = %q, want cached description", stats[0].Classification.Description)
	}
}

func TestDeletePatentsIsTransactional(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	p1 := samplePatent("US11611785B2")
	p2 := samplePatent("US0000001B2")
	for _, p := range []domain.Patent{p1, p2} {
		if err := repo.SavePatent(ctx, p); err != nil {
			t.Fatalf("SavePatent: %v", err)
		}
	}

	missing := domain.MustParsePatentNumber("US9999999B2")
	err := repo.DeletePatents(ctx, []domain.PatentNumber{p1.Number, missing})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("DeletePatents error = %v, want store.ErrNotFound", err)
	}

	if _, err := repo.Patent(ctx, p1.Number); err != nil {
		t.Fatalf("Patent after rolled back delete: %v", err)
	}
	if _, err := repo.Patent(ctx, p2.Number); err != nil {
		t.Fatalf("Unrelated patent after rolled back delete: %v", err)
	}

	if err := repo.DeletePatents(ctx, []domain.PatentNumber{p1.Number, p2.Number}); err != nil {
		t.Fatalf("DeletePatents success case failed: %v", err)
	}
	for _, number := range []domain.PatentNumber{p1.Number, p2.Number} {
		_, err := repo.Patent(ctx, number)
		if !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("Patent %s after delete error = %v, want store.ErrNotFound", number, err)
		}
	}
}

func TestBackupTelemetry(t *testing.T) {
	metrics := observability.NewMetrics()
	tempDB := filepath.Join(t.TempDir(), "source.db")
	repo, err := OpenWithMetrics(context.Background(), tempDB, metrics)
	if err != nil {
		t.Fatalf("OpenWithMetrics: %v", err)
	}
	defer func() { _ = repo.Close() }()

	ctx := context.Background()
	dest := filepath.Join(t.TempDir(), "backup.db")

	if err := repo.Backup(ctx, dest); err != nil {
		t.Fatalf("Backup failed: %v", err)
	}

	snap := metrics.Snapshot()
	if snap.Counters["store.sqlite.backup.total"] != 1 {
		t.Fatalf("backup.total = %d, want 1", snap.Counters["store.sqlite.backup.total"])
	}
	if snap.Timings["store.sqlite.backup.duration"].Count != 1 {
		t.Fatalf("backup.duration Timing count = %d, want 1", snap.Timings["store.sqlite.backup.duration"].Count)
	}
	if size := snap.Gauges["store.sqlite.backup.size_bytes"]; size <= 0 {
		t.Fatalf("backup.size_bytes gauge = %d, want > 0", size)
	}
}

func TestUSPTOApplicationStoreRoundTrip(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "uspto.db")
	repo, err := Open(context.Background(), tempDB)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = repo.Close() }()

	ctx := context.Background()
	n := domain.MustParsePatentNumber("US17812078")

	// Verify not found initially
	_, err = repo.USPTOApplication(ctx, n)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected store.ErrNotFound, got %v", err)
	}

	patent := domain.Patent{
		Number:     n,
		Title:      "Test Patent",
		FetchState: domain.FetchStub,
		Source:     domain.SourceUSPTO,
	}

	usptoApp := &domain.USPTOApplication{
		ApplicationNumber:     "17812078",
		RecordNumber:          n,
		InventionTitle:        "Test USPTO Application",
		FilingDate:            "2022-07-12",
		ApplicationStatusText: "Pending",
		ExaminerName:          "Smith, John",
		GroupArtUnitNumber:    "1234",
		PGPubXMLURL:           "https://api.uspto.gov/files/app.xml",
		PGPubXMLName:          "app.xml",
		PatentGrantXMLURL:     "https://api.uspto.gov/files/grant.xml",
		PatentGrantXMLName:    "grant.xml",
	}

	batch := store.NodeBatch{
		Patent:           patent,
		USPTOApplication: usptoApp,
	}

	if err := repo.SaveNode(ctx, batch); err != nil {
		t.Fatalf("SaveNode failed: %v", err)
	}

	// Retrieve and verify
	got, err := repo.USPTOApplication(ctx, n)
	if err != nil {
		t.Fatalf("USPTOApplication: %v", err)
	}

	if got.ApplicationNumber != "17812078" {
		t.Errorf("got.ApplicationNumber = %q, want 17812078", got.ApplicationNumber)
	}
	if got.InventionTitle != "Test USPTO Application" {
		t.Errorf("got.InventionTitle = %q, want Test USPTO Application", got.InventionTitle)
	}
	if got.FilingDate != "2022-07-12" {
		t.Errorf("got.FilingDate = %q, want 2022-07-12", got.FilingDate)
	}
	if got.ExaminerName != "Smith, John" {
		t.Errorf("got.ExaminerName = %q, want Smith, John", got.ExaminerName)
	}
	if got.GroupArtUnitNumber != "1234" {
		t.Errorf("got.GroupArtUnitNumber = %q, want 1234", got.GroupArtUnitNumber)
	}
	if got.PGPubXMLURL != "https://api.uspto.gov/files/app.xml" {
		t.Errorf("got.PGPubXMLURL = %q, want https://api.uspto.gov/files/app.xml", got.PGPubXMLURL)
	}
	if got.PGPubXMLName != "app.xml" {
		t.Errorf("got.PGPubXMLName = %q, want app.xml", got.PGPubXMLName)
	}
	if got.PatentGrantXMLURL != "https://api.uspto.gov/files/grant.xml" {
		t.Errorf("got.PatentGrantXMLURL = %q, want https://api.uspto.gov/files/grant.xml", got.PatentGrantXMLURL)
	}
	if got.PatentGrantXMLName != "grant.xml" {
		t.Errorf("got.PatentGrantXMLName = %q, want grant.xml", got.PatentGrantXMLName)
	}
}
