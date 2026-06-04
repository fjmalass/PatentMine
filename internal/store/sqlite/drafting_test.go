package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

func seedProject(t *testing.T, repo *Repo, id string) {
	t.Helper()
	if err := repo.SaveProject(context.Background(), domain.Project{
		ID: domain.ProjectID(id), Name: id, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
}

func TestSaveAndLoadDraftRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepo(t)
	seedProject(t, repo, "proj1")

	now := time.Now().UTC().Truncate(time.Second)
	d := domain.Draft{
		ID:      "draft1",
		Project: "proj1",
		Kind:    domain.DraftNonprovisional,
		Title:   "Self-Cleaning Widget",
		Status:  domain.DraftStatusDraft,
		CreatedAt: now, UpdatedAt: now,
		Sections: []domain.DraftSection{
			{Ordinal: 0, Kind: domain.SectionField, Heading: "FIELD OF THE INVENTION", Body: "Rodent control."},
			{Ordinal: 1, Kind: domain.SectionSummary, Heading: "SUMMARY", Body: "A summary.",
				Pinned: []string{"verbatim phrase"}, AIProvider: "ollama", AIModel: "mistral", GeneratedAt: now, HumanEdited: true},
		},
		Claims: []domain.DraftClaim{
			{Ordinal: 0, Number: 1, Type: domain.ClaimIndependent, Status: domain.AmendOriginal, Text: "A widget."},
			{Ordinal: 1, Number: 2, Type: domain.ClaimDependent, DependsOn: 1, Status: domain.AmendOriginal, Text: "The widget of claim 1."},
		},
	}
	if err := repo.SaveDraft(ctx, d); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	got, err := repo.Draft(ctx, "draft1")
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if got.Title != d.Title || got.Kind != d.Kind || got.Status != domain.DraftStatusDraft {
		t.Errorf("draft header mismatch: %+v", got)
	}
	if len(got.Sections) != 2 || got.Sections[1].AIProvider != "ollama" || !got.Sections[1].HumanEdited {
		t.Fatalf("sections round-trip wrong: %+v", got.Sections)
	}
	if len(got.Sections[1].Pinned) != 1 || got.Sections[1].Pinned[0] != "verbatim phrase" {
		t.Errorf("pinned round-trip wrong: %+v", got.Sections[1].Pinned)
	}
	if len(got.Claims) != 2 || got.Claims[1].DependsOn != 1 || got.Claims[1].Type != domain.ClaimDependent {
		t.Fatalf("claims round-trip wrong: %+v", got.Claims)
	}
}

func TestSaveDraftReplacesChildren(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepo(t)
	seedProject(t, repo, "proj1")
	now := time.Now().UTC()

	d := domain.Draft{ID: "d", Project: "proj1", Kind: domain.DraftProvisional, CreatedAt: now, UpdatedAt: now,
		Sections: domain.DefaultSections(domain.DraftProvisional)}
	if err := repo.SaveDraft(ctx, d); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	// Re-save with fewer sections; the side table must be replaced wholesale.
	d.Sections = d.Sections[:1]
	if err := repo.SaveDraft(ctx, d); err != nil {
		t.Fatalf("SaveDraft re-save: %v", err)
	}
	got, err := repo.Draft(ctx, "d")
	if err != nil {
		t.Fatalf("Draft: %v", err)
	}
	if len(got.Sections) != 1 {
		t.Fatalf("expected 1 section after re-save, got %d", len(got.Sections))
	}
}

func TestDeleteProjectCascadesDrafts(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepo(t)
	seedProject(t, repo, "proj1")
	now := time.Now().UTC()
	if err := repo.SaveDraft(ctx, domain.Draft{ID: "d", Project: "proj1", Kind: domain.DraftProvisional, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	// Deleting the project cascades to its drafts (FK ON DELETE CASCADE).
	if _, err := repo.writer.ExecContext(ctx, `DELETE FROM project WHERE id = ?`, "proj1"); err != nil {
		t.Fatalf("delete project: %v", err)
	}
	if _, err := repo.Draft(ctx, "d"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after cascade, got %v", err)
	}
}

func TestOfficeActionRoundTripAndDraftLink(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepo(t)
	seedProject(t, repo, "proj1")
	now := time.Now().UTC().Truncate(time.Second)

	oa := domain.OfficeAction{
		ID: "oa1", Project: "proj1", ApplicationNumber: "16/123,456",
		MailDate: now, Type: domain.OANonFinal, Examiner: "Jane Doe", ArtUnit: "2151",
		BlobPath: "/tmp/oa.pdf", BlobHash: "abc", ExtractedText: "Claims 1-3 rejected.", Source: domain.OASourceManual, ImportedAt: now,
	}
	if err := repo.SaveOfficeAction(ctx, oa); err != nil {
		t.Fatalf("SaveOfficeAction: %v", err)
	}
	got, err := repo.OfficeAction(ctx, "oa1")
	if err != nil {
		t.Fatalf("OfficeAction: %v", err)
	}
	if got.Examiner != "Jane Doe" || got.Type != domain.OANonFinal || got.ExtractedText != "Claims 1-3 rejected." {
		t.Errorf("office action round-trip wrong: %+v", got)
	}

	// A response draft links to the office action.
	if err := repo.SaveDraft(ctx, domain.Draft{
		ID: "resp", Project: "proj1", Kind: domain.DraftOAResponse, OfficeActionID: "oa1",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("SaveDraft response: %v", err)
	}
	gotResp, err := repo.Draft(ctx, "resp")
	if err != nil {
		t.Fatalf("Draft resp: %v", err)
	}
	if gotResp.OfficeActionID != "oa1" {
		t.Errorf("response office_action_id = %q, want oa1", gotResp.OfficeActionID)
	}

	// Deleting the office action resets the draft's link to empty (FK SET NULL).
	if err := repo.DeleteOfficeAction(ctx, "oa1"); err != nil {
		t.Fatalf("DeleteOfficeAction: %v", err)
	}
	gotResp, err = repo.Draft(ctx, "resp")
	if err != nil {
		t.Fatalf("Draft resp after OA delete: %v", err)
	}
	if gotResp.OfficeActionID != "" {
		t.Errorf("office_action_id should be cleared, got %q", gotResp.OfficeActionID)
	}

	if list, err := repo.ListOfficeActions(ctx, "proj1"); err != nil || len(list) != 0 {
		t.Fatalf("ListOfficeActions after delete = %v, %v", list, err)
	}
}

func TestListDraftsNewestFirst(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepo(t)
	seedProject(t, repo, "proj1")
	base := time.Now().UTC()
	if err := repo.SaveDraft(ctx, domain.Draft{ID: "old", Project: "proj1", Kind: domain.DraftProvisional, CreatedAt: base, UpdatedAt: base.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveDraft(ctx, domain.Draft{ID: "new", Project: "proj1", Kind: domain.DraftNonprovisional, CreatedAt: base, UpdatedAt: base}); err != nil {
		t.Fatal(err)
	}
	list, err := repo.ListDrafts(ctx, "proj1")
	if err != nil {
		t.Fatalf("ListDrafts: %v", err)
	}
	if len(list) != 2 || list[0].ID != "new" {
		t.Fatalf("ListDrafts order wrong: %+v", list)
	}
}
