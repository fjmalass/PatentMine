package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

// seedOfficeAction creates a project, an office action under it, and the given
// patents as records, returning the office action id. Patents must exist as
// records before they can be assigned (FK patent_office_action.patent_number ->
// record(number)).
func seedOfficeAction(t *testing.T, repo *Repo, project, oaID string, patents ...domain.PatentNumber) string {
	t.Helper()
	ctx := context.Background()
	seedProject(t, repo, project)
	now := time.Now().UTC().Truncate(time.Second)
	oa := domain.OfficeAction{
		ID: oaID, Project: domain.ProjectID(project), ApplicationNumber: "16/123,456",
		MailDate: now, Type: domain.OANonFinal, Source: domain.OASourceManual, ImportedAt: now,
	}
	if err := repo.SaveOfficeAction(ctx, oa); err != nil {
		t.Fatalf("SaveOfficeAction: %v", err)
	}
	for _, p := range patents {
		if err := repo.SavePatent(ctx, samplePatent(p.Normalized())); err != nil {
			t.Fatalf("SavePatent %s: %v", p, err)
		}
	}
	return oaID
}

func TestOfficeActionPatentRoundTrip(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	p1 := domain.MustParsePatentNumber("US0000001B2")
	p2 := domain.MustParsePatentNumber("US0000002B2")
	oaID := seedOfficeAction(t, repo, "proj1", "oa1", p1, p2)

	assignedAt := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if err := repo.AssignPatentsToOfficeAction(ctx, oaID, []domain.PatentNumber{p1, p2}, assignedAt); err != nil {
		t.Fatalf("AssignPatentsToOfficeAction: %v", err)
	}

	links, err := repo.PatentsForOfficeAction(ctx, oaID)
	if err != nil {
		t.Fatalf("PatentsForOfficeAction: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("PatentsForOfficeAction returned %d links, want 2", len(links))
	}
	for _, l := range links {
		if l.Status != domain.OAReviewToReview {
			t.Errorf("link %s status = %q, want to_review", l.Patent, l.Status)
		}
		if !l.AssignedAt.Equal(assignedAt) {
			t.Errorf("link %s assigned_at = %v, want %v", l.Patent, l.AssignedAt, assignedAt)
		}
		if !l.ReviewedAt.IsZero() {
			t.Errorf("link %s reviewed_at = %v, want zero before review", l.Patent, l.ReviewedAt)
		}
	}

	// The reverse lookup annotates the patent list.
	byPatent, err := repo.OfficeActionsForPatents(ctx, []domain.PatentNumber{p1, p2})
	if err != nil {
		t.Fatalf("OfficeActionsForPatents: %v", err)
	}
	if got := byPatent[p1.Normalized()]; len(got) != 1 || got[0].OfficeActionID != oaID {
		t.Errorf("OfficeActionsForPatents[%s] = %+v, want one link to %s", p1, got, oaID)
	}

	// Counts for the office-action list.
	counts, err := repo.OfficeActionAssignedCounts(ctx, "proj1")
	if err != nil {
		t.Fatalf("OfficeActionAssignedCounts: %v", err)
	}
	if counts[oaID] != 2 {
		t.Errorf("assigned count = %d, want 2", counts[oaID])
	}

	// Mark one reviewed: status flips and reviewed_at is stamped.
	reviewedAt := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	if err := repo.SetOfficeActionReviewStatus(ctx, oaID, p1, domain.OAReviewReviewed, reviewedAt); err != nil {
		t.Fatalf("SetOfficeActionReviewStatus: %v", err)
	}
	links, err = repo.PatentsForOfficeAction(ctx, oaID)
	if err != nil {
		t.Fatalf("PatentsForOfficeAction after review: %v", err)
	}
	for _, l := range links {
		if l.Patent.Equal(p1) {
			if l.Status != domain.OAReviewReviewed || l.ReviewedAt.IsZero() {
				t.Errorf("reviewed link = %+v, want reviewed with reviewed_at stamped", l)
			}
		}
	}

	// Release one: it drops out, the other remains.
	if err := repo.ReleasePatentsFromOfficeAction(ctx, oaID, []domain.PatentNumber{p1}); err != nil {
		t.Fatalf("ReleasePatentsFromOfficeAction: %v", err)
	}
	links, err = repo.PatentsForOfficeAction(ctx, oaID)
	if err != nil {
		t.Fatalf("PatentsForOfficeAction after release: %v", err)
	}
	if len(links) != 1 || !links[0].Patent.Equal(p2) {
		t.Fatalf("after release links = %+v, want only %s", links, p2)
	}
}

func TestSetOfficeActionReviewStatusNotFound(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	p1 := domain.MustParsePatentNumber("US0000001B2")
	oaID := seedOfficeAction(t, repo, "proj1", "oa1", p1)
	// p1 exists but was never assigned to the action.
	err := repo.SetOfficeActionReviewStatus(ctx, oaID, p1, domain.OAReviewReviewed, time.Now())
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("SetOfficeActionReviewStatus on unassigned patent = %v, want ErrNotFound", err)
	}
}

// TestAssignPatentsIsIdempotent guards that re-assigning an already-linked patent
// preserves its review progress instead of resetting it to to_review.
func TestAssignPatentsIsIdempotent(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	p1 := domain.MustParsePatentNumber("US0000001B2")
	oaID := seedOfficeAction(t, repo, "proj1", "oa1", p1)

	if err := repo.AssignPatentsToOfficeAction(ctx, oaID, []domain.PatentNumber{p1}, time.Now()); err != nil {
		t.Fatalf("first assign: %v", err)
	}
	if err := repo.SetOfficeActionReviewStatus(ctx, oaID, p1, domain.OAReviewReviewed, time.Now()); err != nil {
		t.Fatalf("set reviewed: %v", err)
	}
	if err := repo.AssignPatentsToOfficeAction(ctx, oaID, []domain.PatentNumber{p1}, time.Now()); err != nil {
		t.Fatalf("re-assign: %v", err)
	}
	links, err := repo.PatentsForOfficeAction(ctx, oaID)
	if err != nil {
		t.Fatalf("PatentsForOfficeAction: %v", err)
	}
	if len(links) != 1 || links[0].Status != domain.OAReviewReviewed {
		t.Fatalf("after re-assign links = %+v, want a single reviewed link (progress preserved)", links)
	}
}

// TestMergeRecordsPreservesUserData is the regression guard for the merge
// data-loss bug: before recordNumberChildTables, MergeRecords silently
// CASCADE-dropped the absorbed record's tags, notes, renewals (and would have
// dropped office-action review links). A merge must carry all of them onto the
// surviving record.
func TestMergeRecordsPreservesUserData(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	keep := domain.MustParsePatentNumber("US16000001")
	absorb := domain.MustParsePatentNumber("US0000001B2")
	oaID := seedOfficeAction(t, repo, "proj1", "oa1", keep, absorb)

	// The absorbed record carries every kind of number-keyed user data.
	tag, err := repo.CreateTag(ctx, "proj1", "prior-art")
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if err := repo.TagPatents(ctx, tag.ID, []domain.PatentNumber{absorb}, time.Now()); err != nil {
		t.Fatalf("TagPatents: %v", err)
	}
	if _, err := repo.SavePatentNote(ctx, domain.PatentNote{
		Project: "proj1", Patent: absorb, Markdown: "distinguishable",
	}); err != nil {
		t.Fatalf("SavePatentNote: %v", err)
	}
	if err := repo.SavePatentRenewal(ctx, domain.PatentRenewal{
		PatentNumber: absorb, EntitySize: "small", IsTracked: true,
	}); err != nil {
		t.Fatalf("SavePatentRenewal: %v", err)
	}
	if err := repo.AssignPatentsToOfficeAction(ctx, oaID, []domain.PatentNumber{absorb}, time.Now()); err != nil {
		t.Fatalf("AssignPatentsToOfficeAction: %v", err)
	}

	if err := repo.MergeRecords(ctx, keep, absorb); err != nil {
		t.Fatalf("MergeRecords: %v", err)
	}

	// Tag followed to keep.
	tags, err := repo.PatentTags(ctx, "proj1", keep)
	if err != nil {
		t.Fatalf("PatentTags(keep): %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "prior-art" {
		t.Errorf("tags on keep = %+v, want [prior-art] (tag lost on merge)", tags)
	}
	// Note followed to keep.
	if n := countRows(t, repo, `SELECT COUNT(*) FROM project_patent_note WHERE patent_number = ?`, keep.Normalized()); n != 1 {
		t.Errorf("notes on keep = %d, want 1 (note lost on merge)", n)
	}
	// Renewal followed to keep.
	renewal, err := repo.PatentRenewal(ctx, keep)
	if err != nil {
		t.Fatalf("PatentRenewal(keep): %v", err)
	}
	if !renewal.IsTracked || renewal.EntitySize != "small" {
		t.Errorf("renewal on keep = %+v, want the absorbed tracked/small config", renewal)
	}
	// Office-action review link followed to keep.
	links, err := repo.PatentsForOfficeAction(ctx, oaID)
	if err != nil {
		t.Fatalf("PatentsForOfficeAction: %v", err)
	}
	if len(links) != 1 || !links[0].Patent.Equal(keep) {
		t.Errorf("OA links after merge = %+v, want one pointing at keep", links)
	}
	// Nothing left dangling under the absorbed number.
	if n := countRows(t, repo, `SELECT COUNT(*) FROM patent_office_action WHERE patent_number = ?`, absorb.Normalized()); n != 0 {
		t.Errorf("OA links still under absorbed number = %d, want 0", n)
	}
}

func TestSoftDeleteDropsOfficeActionLinks(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	p1 := domain.MustParsePatentNumber("US0000001B2")
	oaID := seedOfficeAction(t, repo, "proj1", "oa1", p1)
	if err := repo.AssignPatentsToOfficeAction(ctx, oaID, []domain.PatentNumber{p1}, time.Now()); err != nil {
		t.Fatalf("AssignPatentsToOfficeAction: %v", err)
	}
	if err := repo.SoftDeletePatent(ctx, p1); err != nil {
		t.Fatalf("SoftDeletePatent: %v", err)
	}
	links, err := repo.PatentsForOfficeAction(ctx, oaID)
	if err != nil {
		t.Fatalf("PatentsForOfficeAction: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("after soft delete links = %+v, want none", links)
	}
}

// TestListPatentsAttachesOfficeActions guards the OA column data path: a
// project patent list enriches each row with the office actions it's assigned to
// (label + review status), mirroring how tags are attached.
func TestListPatentsAttachesOfficeActions(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	p1 := domain.MustParsePatentNumber("US0000001B2")
	oaID := seedOfficeAction(t, repo, "proj1", "oa1", p1)

	oa, err := repo.OfficeAction(ctx, oaID)
	if err != nil {
		t.Fatalf("OfficeAction: %v", err)
	}
	oa.Name = "Final OA"
	if err := repo.SaveOfficeAction(ctx, oa); err != nil {
		t.Fatalf("SaveOfficeAction: %v", err)
	}
	// The project list returns only project members, so enroll p1.
	if err := repo.AddMembership(ctx, domain.Membership{
		Project: "proj1", Patent: p1, ReviewState: domain.ReviewStateUnknown, AddedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}
	if err := repo.AssignPatentsToOfficeAction(ctx, oaID, []domain.PatentNumber{p1}, time.Now()); err != nil {
		t.Fatalf("AssignPatentsToOfficeAction: %v", err)
	}

	rows, err := repo.ListPatents(ctx, store.PatentQuery{Project: "proj1"})
	if err != nil {
		t.Fatalf("ListPatents: %v", err)
	}
	var found *domain.PatentRow
	for i := range rows {
		if rows[i].Number.Equal(p1) {
			found = &rows[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("patent %s not present in project list", p1)
	}
	if len(found.OfficeActions) != 1 {
		t.Fatalf("row.OfficeActions = %+v, want exactly one", found.OfficeActions)
	}
	if got := found.OfficeActions[0]; got.Name != "Final OA" || got.Status != domain.OAReviewToReview || got.OfficeActionID != oaID {
		t.Fatalf("row.OfficeActions[0] = %+v, want Final OA / to_review / %s", got, oaID)
	}
}

// TestListPatentsOAFilter guards the `oa:` filter field: none/any presence,
// status, and name matching, scoped to the project.
func TestListPatentsOAFilter(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	assigned := domain.MustParsePatentNumber("US0000001B2")
	unassigned := domain.MustParsePatentNumber("US0000002B2")
	oaID := seedOfficeAction(t, repo, "proj1", "oa1", assigned, unassigned)

	oa, err := repo.OfficeAction(ctx, oaID)
	if err != nil {
		t.Fatalf("OfficeAction: %v", err)
	}
	oa.Name = "Final OA"
	if err := repo.SaveOfficeAction(ctx, oa); err != nil {
		t.Fatalf("SaveOfficeAction: %v", err)
	}
	for _, p := range []domain.PatentNumber{assigned, unassigned} {
		if err := repo.AddMembership(ctx, domain.Membership{
			Project: "proj1", Patent: p, ReviewState: domain.ReviewStateUnknown, AddedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("AddMembership %s: %v", p, err)
		}
	}
	if err := repo.AssignPatentsToOfficeAction(ctx, oaID, []domain.PatentNumber{assigned}, time.Now()); err != nil {
		t.Fatalf("AssignPatentsToOfficeAction: %v", err)
	}

	list := func(filter string) []domain.PatentNumber {
		t.Helper()
		rows, err := repo.ListPatents(ctx, store.PatentQuery{Project: "proj1", Filter: filter})
		if err != nil {
			t.Fatalf("ListPatents %q: %v", filter, err)
		}
		out := make([]domain.PatentNumber, len(rows))
		for i, r := range rows {
			out[i] = r.Number
		}
		return out
	}
	onlyAssigned := func(filter string) {
		t.Helper()
		got := list(filter)
		if len(got) != 1 || !got[0].Equal(assigned) {
			t.Fatalf("filter %q = %v, want only %s", filter, got, assigned)
		}
	}

	onlyAssigned("oa:any")
	onlyAssigned(`oa:"Final OA"`)
	onlyAssigned("oa:fin") // case-insensitive substring
	onlyAssigned("oa:to_review")

	if got := list("oa:none"); len(got) != 1 || !got[0].Equal(unassigned) {
		t.Fatalf(`oa:none = %v, want only %s`, got, unassigned)
	}
	if got := list("oa:reviewed"); len(got) != 0 {
		t.Fatalf("oa:reviewed = %v, want none (nothing reviewed yet)", got)
	}

	// After review, oa:reviewed matches and oa:to_review no longer does.
	if err := repo.SetOfficeActionReviewStatus(ctx, oaID, assigned, domain.OAReviewReviewed, time.Now()); err != nil {
		t.Fatalf("SetOfficeActionReviewStatus: %v", err)
	}
	onlyAssigned("oa:reviewed")
	if got := list("oa:to_review"); len(got) != 0 {
		t.Fatalf("oa:to_review after review = %v, want none", got)
	}
}

// TestListPatentsSortByOfficeActions guards the OA column's sortability: patents
// group by their assigned office action's name, with unassigned (empty) sorting
// to one end.
func TestListPatentsSortByOfficeActions(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	assigned := domain.MustParsePatentNumber("US0000001B2")
	unassigned := domain.MustParsePatentNumber("US0000002B2")
	oaID := seedOfficeAction(t, repo, "proj1", "oa1", assigned, unassigned)
	oa, err := repo.OfficeAction(ctx, oaID)
	if err != nil {
		t.Fatalf("OfficeAction: %v", err)
	}
	oa.Name = "Final OA"
	if err := repo.SaveOfficeAction(ctx, oa); err != nil {
		t.Fatalf("SaveOfficeAction: %v", err)
	}
	for _, p := range []domain.PatentNumber{assigned, unassigned} {
		if err := repo.AddMembership(ctx, domain.Membership{
			Project: "proj1", Patent: p, ReviewState: domain.ReviewStateUnknown, AddedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("AddMembership %s: %v", p, err)
		}
	}
	if err := repo.AssignPatentsToOfficeAction(ctx, oaID, []domain.PatentNumber{assigned}, time.Now()); err != nil {
		t.Fatalf("AssignPatentsToOfficeAction: %v", err)
	}

	first := func(asc bool) domain.PatentNumber {
		t.Helper()
		rows, err := repo.ListPatents(ctx, store.PatentQuery{
			Project: "proj1", SortColumn: domain.SortByOfficeActions, SortAscending: asc,
		})
		if err != nil {
			t.Fatalf("ListPatents sort oa asc=%v: %v", asc, err)
		}
		if len(rows) != 2 {
			t.Fatalf("ListPatents returned %d rows, want 2", len(rows))
		}
		return rows[0].Number
	}
	if got := first(false); !got.Equal(assigned) {
		t.Fatalf("desc sort first = %s, want assigned %s", got, assigned)
	}
	if got := first(true); !got.Equal(unassigned) {
		t.Fatalf("asc sort first = %s, want unassigned %s", got, unassigned)
	}
}

// countRows runs a COUNT(*) query and returns the scalar.
func countRows(t *testing.T, repo *Repo, query string, args ...any) int {
	t.Helper()
	var n int
	if err := repo.reader.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
}
