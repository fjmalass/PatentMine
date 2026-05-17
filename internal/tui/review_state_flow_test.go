package tui

import (
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/storage"
)

// TestBulkImportThrobberPersistsUntilBatchDrains feeds the async results of a
// 2-patent bulk import one at a time. The throbber must stay up until the last
// result, and every result must reload the list so imported patents appear.
func TestBulkImportThrobberPersistsUntilBatchDrains(t *testing.T) {
	m, repo, ctx := newSeededDeleteModel(t, 5)
	m.loading = true
	m.pendingDetails = 2

	updated, _ := m.Update(refreshDetailsResultMsg{message: "imported A"})
	m = updated.(*Model)
	if !m.loading {
		t.Fatal("throbber cleared after first of 2 imports")
	}
	if m.pendingDetails != 1 {
		t.Fatalf("pendingDetails: want 1, got %d", m.pendingDetails)
	}

	// A patent lands in the DB between async results.
	if err := repo.UpsertPatentBundle(ctx, "default", domain.PatentBundle{
		Patent: domain.Patent{Number: "US9999", Title: "Late import"},
	}); err != nil {
		t.Fatal(err)
	}

	updated, _ = m.Update(refreshDetailsResultMsg{message: "imported B"})
	m = updated.(*Model)
	if m.loading {
		t.Fatal("throbber still up after the final import")
	}
	if m.pendingDetails != 0 {
		t.Fatalf("pendingDetails: want 0, got %d", m.pendingDetails)
	}
	found := false
	for _, p := range m.patents {
		if p.Number == "US9999" {
			found = true
		}
	}
	if !found {
		t.Fatal("list not reloaded on async result — imported patent US9999 missing")
	}
}

// TestVisualBulkReviewStateUpdatesAllSelected drives the visual bulk review-state
// flow (v -> j -> j -> s -> j -> Enter) and asserts every selected patent — not
// just the first — moves to the chosen state, both in the DB and the list view.
func TestVisualBulkReviewStateUpdatesAllSelected(t *testing.T) {
	const seeded = 10
	m, repo, ctx := newSeededDeleteModel(t, seeded)
	// Show every state so updated rows stay visible in the list after the change.
	m.listFilter = PatentFilter{ReviewState: reviewStateFilterNone}
	m = m.reloadPatents()
	targets := []string{m.patents[0].Number, m.patents[1].Number, m.patents[2].Number}

	updated, _ := m.Update(teaKey(keyVisual))
	updated, _ = updated.(*Model).Update(teaKey(keyVimDown))
	updated, _ = updated.(*Model).Update(teaKey(keyVimDown))
	updated, _ = updated.(*Model).Update(teaKey(keyReviewState))
	if updated.(*Model).mode != viewReviewStateSelect {
		t.Fatalf("s did not open review-state picker, got %q", updated.(*Model).mode)
	}
	// picker opens on "stored"; one step down selects "under_review".
	updated, _ = updated.(*Model).Update(teaKey(keyVimDown))
	updated, _ = updated.(*Model).Update(teaKey(keyEnter))
	got := updated.(*Model)

	// DB: every target moved.
	all, err := repo.ListPatents(ctx, "default", storage.ListPatentsOptions{
		ReviewStateFilter: storage.ReviewStateFilterNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	dbState := make(map[string]string, len(all))
	for _, p := range all {
		dbState[p.Number] = p.ReviewState
	}
	for _, num := range targets {
		if dbState[num] != domain.ReviewStateUnderReview {
			t.Fatalf("DB: patent %s expected %q, got %q",
				num, domain.ReviewStateUnderReview, dbState[num])
		}
	}

	// List view: every target shows the new state, not just the first.
	if got.mode != viewList {
		t.Fatalf("after Enter expected viewList, got %q", got.mode)
	}
	listState := make(map[string]string, len(got.patents))
	for _, p := range got.patents {
		listState[p.Number] = p.ReviewState
	}
	for _, num := range targets {
		if listState[num] != domain.ReviewStateUnderReview {
			t.Fatalf("list view: patent %s expected %q, got %q",
				num, domain.ReviewStateUnderReview, listState[num])
		}
	}
}

func patentReviewState(patents []domain.Patent, number string) string {
	for _, p := range patents {
		if p.Number == number {
			return p.ReviewState
		}
	}
	return ""
}

// TestCitationPopupIgnoreMovesPatent guards the bug where pressing i in the
// citation popup moved only the citation edge, leaving the target patent's own
// review state — the one the list renders — untouched. After i the patent must
// read "ignored" in both the DB and the reloaded list.
func TestCitationPopupIgnoreMovesPatent(t *testing.T) {
	m, repo, ctx := newSeededDeleteModel(t, 5)
	// Seed a citation: US0000 cites US0002.
	if err := repo.UpsertPatentBundle(ctx, "default", domain.PatentBundle{
		Patent: domain.Patent{Number: "US0000", Title: "Patent US0000"},
		Citations: []domain.CitationEdge{{
			SourcePatent: "US0000",
			TargetPatent: "US0002",
			RelationType: domain.RelationCites,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	// Show every state so an ignored row stays visible in the list.
	m.listFilter = PatentFilter{ReviewState: reviewStateFilterNone}
	m = m.reloadPatents()
	for i, p := range m.patents {
		if p.Number == "US0000" {
			m.patentSelected = i
		}
	}

	// c -> citations of US0000; Enter -> open the citation popup.
	updated, _ := m.Update(teaKey(keyCites))
	updated, _ = updated.(*Model).Update(teaKey(keyEnter))
	popup := updated.(*Model)
	if popup.mode != viewPopupPatentDetail {
		t.Fatalf("Enter did not open the citation popup, got %q", popup.mode)
	}
	if popup.pendingCitation.TargetPatent != "US0002" {
		t.Fatalf("popup opened for %q, want US0002", popup.pendingCitation.TargetPatent)
	}

	updated, _ = popup.Update(teaKey(keyIgnore))
	got := updated.(*Model)

	all, err := repo.ListPatents(ctx, "default", storage.ListPatentsOptions{
		ReviewStateFilter: storage.ReviewStateFilterNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	if st := patentReviewState(all, "US0002"); st != domain.ReviewStateIgnored {
		t.Fatalf("DB: US0002 review state = %q, want %q", st, domain.ReviewStateIgnored)
	}
	if st := patentReviewState(got.patents, "US0002"); st != domain.ReviewStateIgnored {
		t.Fatalf("list view: US0002 review state = %q, want %q", st, domain.ReviewStateIgnored)
	}
}

// TestCitationPopupUnderReviewImportsNonMember guards that under-reviewing a
// citation from the popup (key r) imports a not-yet-member target and records
// its under_review state. The under_review list filter is pure patent state,
// so an edge-only update would leave the patent invisible there.
func TestCitationPopupUnderReviewImportsNonMember(t *testing.T) {
	m, repo, ctx := newSeededDeleteModel(t, 3)
	const nonMember = "US8888"

	m.pendingBundle = domain.PatentBundle{
		Patent: domain.Patent{Number: nonMember, Title: "Cited non-member"},
	}
	m.pendingCitation = domain.CitationEdge{
		SourcePatent: m.patents[0].Number,
		TargetPatent: nonMember,
		RelationType: domain.RelationCites,
	}
	m.current = m.pendingBundle.Patent
	m.mode = viewPopupPatentDetail

	m.Update(teaKey(keyUnreview))

	underReview := listNumbers(t, repo, ctx, domain.ReviewStateUnderReview)
	if !underReview[nonMember] {
		t.Fatalf("%s missing from the under-review list after popup r", nonMember)
	}
}

// TestRestoreKeepsLiveListAndReanchorsCursor guards the rule that a navigation
// snapshot restores cursor position, not list data. restore() is handed a
// snapshot taken before a change; the live list has since changed, and restore
// must keep the live list while re-placing the cursor on the same patent.
func TestRestoreKeepsLiveListAndReanchorsCursor(t *testing.T) {
	m, _, _ := newSeededDeleteModel(t, 5)
	m.patentSelected = 2
	want := m.patents[2].Number
	snap := m.snapshot()

	// Simulate a list reload after a change: the first row is gone.
	m.patents = m.patents[1:]

	m.restore(snap)

	if len(m.patents) != 4 {
		t.Fatalf("restore reverted live list data: want 4 patents, got %d", len(m.patents))
	}
	if got := m.patents[m.patentSelected].Number; got != want {
		t.Fatalf("cursor not re-anchored: want %s, got %s at index %d", want, got, m.patentSelected)
	}
}

// seedCitationModel builds a list model where US0000 cites US0002, with every
// review state shown so a changed row stays visible. Returns the model with
// US0000 highlighted.
func seedCitationModel(t *testing.T) *Model {
	t.Helper()
	m, repo, ctx := newSeededDeleteModel(t, 5)
	if err := repo.UpsertPatentBundle(ctx, "default", domain.PatentBundle{
		Patent: domain.Patent{Number: "US0000", Title: "Patent US0000"},
		Citations: []domain.CitationEdge{{
			SourcePatent: "US0000",
			TargetPatent: "US0002",
			RelationType: domain.RelationCites,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	m.listFilter = PatentFilter{ReviewState: reviewStateFilterNone}
	m = m.reloadPatents()
	for i, p := range m.patents {
		if p.Number == "US0000" {
			m.patentSelected = i
		}
	}
	return m
}

// TestCitationPopupIgnoreReflectedAfterReturnToList drives the whole journey a
// user takes — list -> c -> Enter (popup) -> i -> esc (back to list) — and
// asserts the patent list, once the user is actually back on it, shows the
// target patent as ignored.
func TestCitationPopupIgnoreReflectedAfterReturnToList(t *testing.T) {
	m := seedCitationModel(t)

	updated, _ := m.Update(teaKey(keyCites))
	updated, _ = updated.(*Model).Update(teaKey(keyEnter))
	updated, _ = updated.(*Model).Update(teaKey(keyIgnore))
	updated, _ = updated.(*Model).Update(teaKey(keyEsc))
	got := updated.(*Model)

	if got.mode != viewList {
		t.Fatalf("esc did not return to the list, got %q", got.mode)
	}
	if st := patentReviewState(got.patents, "US0002"); st != domain.ReviewStateIgnored {
		t.Fatalf("list view after return: US0002 = %q, want %q", st, domain.ReviewStateIgnored)
	}
}

// TestCitationPopupStatusSelectorReflectedAfterReturnToList drives the status
// picker path — list -> c -> Enter (popup) -> s -> pick -> Enter -> esc -> esc
// — and asserts the change shows once the user is back on the list.
func TestCitationPopupStatusSelectorReflectedAfterReturnToList(t *testing.T) {
	m := seedCitationModel(t)

	updated, _ := m.Update(teaKey(keyCites))
	updated, _ = updated.(*Model).Update(teaKey(keyEnter))
	updated, _ = updated.(*Model).Update(teaKey(keyReviewState))
	sel := updated.(*Model)
	if sel.mode != viewReviewStateSelect {
		t.Fatalf("s did not open the review-state picker, got %q", sel.mode)
	}
	for i, s := range sel.selectableReviewStates() {
		if s == domain.ReviewStateIgnored {
			sel.reviewStateSelected = i
		}
	}
	updated, _ = sel.Update(teaKey(keyEnter))     // commit pick, back in popup
	updated, _ = updated.(*Model).Update(teaKey(keyEsc)) // popup -> citations
	updated, _ = updated.(*Model).Update(teaKey(keyEsc)) // citations -> list
	got := updated.(*Model)

	if got.mode != viewList {
		t.Fatalf("did not return to the list, got %q", got.mode)
	}
	if st := patentReviewState(got.patents, "US0002"); st != domain.ReviewStateIgnored {
		t.Fatalf("list view after return: US0002 = %q, want %q", st, domain.ReviewStateIgnored)
	}
}

// TestCitationPopupShowsPatentStateNotEdgeState guards the bug where the
// citation popup overwrote the patent's review state with the citation edge's
// state. The popup is a patent-detail view: it must show the patent's own
// review state — the value the list shows — not the edge's, so the two agree
// and the s-picker opens on the patent's real state.
func TestCitationPopupShowsPatentStateNotEdgeState(t *testing.T) {
	m, repo, ctx := newSeededDeleteModel(t, 5)
	if err := repo.UpsertPatentBundle(ctx, "default", domain.PatentBundle{
		Patent: domain.Patent{Number: "US0000", Title: "P US0000"},
		Citations: []domain.CitationEdge{{
			SourcePatent: "US0000", TargetPatent: "US0002", RelationType: domain.RelationCites,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	// Drive the edge to a state distinct from the patent's (US0002 is stored).
	edge := domain.CitationEdge{SourcePatent: "US0000", TargetPatent: "US0002", RelationType: domain.RelationCites}
	if _, err := repo.UpdateCitationReviewStates(ctx, "default",
		[]storage.CitationStateUpdate{{Edge: edge, ReviewState: domain.ReviewStateIgnored}}); err != nil {
		t.Fatal(err)
	}
	m = m.reloadPatents()
	for i, p := range m.patents {
		if p.Number == "US0000" {
			m.patentSelected = i
		}
	}

	updated, _ := m.Update(teaKey(keyCites))
	updated, _ = updated.(*Model).Update(teaKey(keyEnter))
	popup := updated.(*Model)
	if popup.mode != viewPopupPatentDetail {
		t.Fatalf("Enter did not open the popup, got %q", popup.mode)
	}
	if popup.current.ReviewState != domain.ReviewStateStored {
		t.Fatalf("popup shows review state %q (the edge state), want the patent state %q",
			popup.current.ReviewState, domain.ReviewStateStored)
	}
}
