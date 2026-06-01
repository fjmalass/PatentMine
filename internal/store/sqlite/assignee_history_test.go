package sqlite

import (
	"context"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

// seedAssigneePatent saves a granted record plus its linked uspto_application so
// the assignee-history rebuild can join assignments back to the surrogate id.
func seedAssigneePatent(t *testing.T, repo *Repo, ctx context.Context, number, appNum, assignee string) domain.PatentNumber {
	t.Helper()
	n := domain.MustParsePatentNumber(number)
	batch := store.NodeBatch{
		Patent: domain.Patent{
			Number:     n,
			Title:      "Test " + number,
			Assignee:   assignee,
			FetchState: domain.FetchCached,
			Source:     domain.SourceUSPTO,
			GrantDate:  time.Date(2023, 3, 21, 0, 0, 0, 0, time.UTC),
		},
		USPTOApplication: &domain.USPTOApplication{
			ApplicationNumber: appNum,
			RecordNumber:      n,
		},
	}
	if err := repo.SaveNode(ctx, batch); err != nil {
		t.Fatalf("SaveNode(%s): %v", number, err)
	}
	return n
}

func chainAssignment(appNum string, ordinal int, recorded, assignor, assignee string) domain.USPTOAssignment {
	return domain.USPTOAssignment{
		ApplicationNumber:      appNum,
		Ordinal:                ordinal,
		ReelAndFrameNumber:     "1000/100" + string(rune('0'+ordinal)),
		ConveyanceText:         "ASSIGNMENT OF ASSIGNORS INTEREST",
		AssignmentRecordedDate: recorded,
		Parties: []domain.USPTOAssignmentParty{
			{ApplicationNumber: appNum, AssignmentOrdinal: ordinal, Role: "assignor", Ordinal: 0, NameText: assignor},
			{ApplicationNumber: appNum, AssignmentOrdinal: ordinal, Role: "assignee", Ordinal: 0, NameText: assignee},
		},
	}
}

func TestRebuildAssigneeHistory(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()
	n := seedAssigneePatent(t, repo, ctx, "US17812078", "17812078", "Acme Corporation")

	assignments := []domain.USPTOAssignment{
		chainAssignment("17812078", 0, "2024-01-15", "Acme Corporation", "Globex LLC"),
		chainAssignment("17812078", 1, "2025-06-10", "Globex LLC", "Initech Inc"),
	}
	if err := repo.SaveUSPTOAssignments(ctx, "17812078", assignments); err != nil {
		t.Fatalf("SaveUSPTOAssignments: %v", err)
	}
	if err := repo.RebuildAssigneeHistoryForNumber(ctx, n, "2026-06-01 00:00:00"); err != nil {
		t.Fatalf("RebuildAssigneeHistoryForNumber: %v", err)
	}

	hist, err := repo.AssigneeHistory(ctx, n)
	if err != nil {
		t.Fatalf("AssigneeHistory: %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("history entries = %d, want 3 (at_grant + 2 assignments): %+v", len(hist), hist)
	}
	// Oldest first: at_grant (Acme, grant date), then the two assignments.
	if hist[0].PullType != domain.PullTypeAtGrant || hist[0].AssigneeName != "Acme Corporation" {
		t.Errorf("entry[0] = %+v, want at_grant Acme Corporation", hist[0])
	}
	if hist[0].EffectiveDate != "2023-03-21" {
		t.Errorf("at_grant effective_date = %q, want 2023-03-21", hist[0].EffectiveDate)
	}

	// Exactly one current owner: the most-recent assignment's assignee.
	var latest []domain.AssigneeHistoryEntry
	for _, e := range hist {
		if e.IsLatest {
			latest = append(latest, e)
		}
	}
	if len(latest) != 1 {
		t.Fatalf("latest rows = %d, want 1: %+v", len(latest), latest)
	}
	if latest[0].AssigneeName != "Initech Inc" || latest[0].AssigneeNorm != "INITECH" {
		t.Errorf("current owner = %q (norm %q), want Initech Inc / INITECH", latest[0].AssigneeName, latest[0].AssigneeNorm)
	}
}

func TestProjectAssignees(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	if err := repo.SaveProject(ctx, domain.Project{ID: "p-assignee", Name: "Assignees"}); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	n := seedAssigneePatent(t, repo, ctx, "US17812078", "17812078", "Acme Corporation")
	if err := repo.AddMembership(ctx, domain.Membership{
		Project: "p-assignee", Patent: n, ReviewState: domain.ReviewStateUnknown, AddedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}
	if err := repo.SaveUSPTOAssignments(ctx, "17812078", []domain.USPTOAssignment{
		chainAssignment("17812078", 0, "2024-01-15", "Acme Corporation", "Globex LLC"),
		chainAssignment("17812078", 1, "2025-06-10", "Globex LLC", "Initech Inc"),
	}); err != nil {
		t.Fatalf("SaveUSPTOAssignments: %v", err)
	}
	if err := repo.RebuildAssigneeHistoryForNumber(ctx, n, "2026-06-01 00:00:00"); err != nil {
		t.Fatalf("RebuildAssigneeHistoryForNumber: %v", err)
	}

	rollup, err := repo.ProjectAssignees(ctx, "p-assignee")
	if err != nil {
		t.Fatalf("ProjectAssignees: %v", err)
	}
	if rollup.LivePatents != 1 || rollup.ExpiredPatents != 0 || rollup.NotFetched != 0 {
		t.Errorf("counts: live=%d expired=%d notFetched=%d, want 1/0/0", rollup.LivePatents, rollup.ExpiredPatents, rollup.NotFetched)
	}
	if len(rollup.CurrentOwners) != 1 {
		t.Fatalf("current owners = %d, want 1: %+v", len(rollup.CurrentOwners), rollup.CurrentOwners)
	}
	if rollup.CurrentOwners[0].Display != "Initech Inc" || rollup.CurrentOwners[0].Patents != 1 {
		t.Errorf("current owner = %+v, want Initech Inc x1", rollup.CurrentOwners[0])
	}
	if len(rollup.AllAssignees) != 3 {
		t.Errorf("all assignees = %d, want 3 (Acme, Globex, Initech): %+v", len(rollup.AllAssignees), rollup.AllAssignees)
	}
}
