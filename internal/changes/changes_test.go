package changes

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/storage"
)

// fakeRepo implements just enough of storage.Repository for change tests. The
// embedded nil interface panics if any unimplemented method is called, which
// keeps the fake honest about what the changes under test actually touch.
type fakeRepo struct {
	storage.Repository
	citStates map[string]string
	patStates map[string]string
	notes     int
	failCit   bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{citStates: map[string]string{}, patStates: map[string]string{}}
}

func edgeKey(e domain.CitationEdge) string {
	return e.SourcePatent + "->" + e.TargetPatent
}

func (r *fakeRepo) UpdateCitationReviewStates(_ context.Context, _ string, updates []storage.CitationStateUpdate) ([]storage.CitationStateUpdate, error) {
	if r.failCit {
		return nil, errors.New("boom")
	}
	prior := make([]storage.CitationStateUpdate, len(updates))
	for i, u := range updates {
		prior[i] = storage.CitationStateUpdate{Edge: u.Edge, ReviewState: r.citStates[edgeKey(u.Edge)]}
	}
	for _, u := range updates {
		r.citStates[edgeKey(u.Edge)] = u.ReviewState
	}
	return prior, nil
}

func (r *fakeRepo) UpdatePatentReviewStates(_ context.Context, _ string, updates []storage.PatentStateUpdate) ([]storage.PatentStateUpdate, error) {
	prior := make([]storage.PatentStateUpdate, len(updates))
	for i, u := range updates {
		prior[i] = storage.PatentStateUpdate{Number: u.Number, ReviewState: r.patStates[u.Number]}
	}
	for _, u := range updates {
		r.patStates[u.Number] = u.ReviewState
	}
	return prior, nil
}

func (r *fakeRepo) AddNote(_ context.Context, _ string, _, _ string) (domain.ResearchNote, error) {
	r.notes++
	return domain.ResearchNote{ID: int64(r.notes)}, nil
}

func TestPatentReviewApplyUndoRedo(t *testing.T) {
	repo := newFakeRepo()
	repo.patStates["US1"] = "stored"
	repo.patStates["US2"] = "stored"
	h := NewHistory(repo)
	ctx := context.Background()

	if _, err := h.Apply(ctx, SetPatentReviewState("p", []string{"US1", "US2"}, "ignored")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if repo.patStates["US1"] != "ignored" || repo.patStates["US2"] != "ignored" {
		t.Fatalf("apply did not move both patents: %v", repo.patStates)
	}

	if _, _, err := h.Undo(ctx); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if repo.patStates["US1"] != "stored" || repo.patStates["US2"] != "stored" {
		t.Fatalf("undo did not restore prior states: %v", repo.patStates)
	}

	if _, _, err := h.Redo(ctx); err != nil {
		t.Fatalf("redo: %v", err)
	}
	if repo.patStates["US1"] != "ignored" {
		t.Fatalf("redo did not re-apply: %v", repo.patStates)
	}
}

func TestCitationReviewReversePerEdge(t *testing.T) {
	repo := newFakeRepo()
	e1 := domain.CitationEdge{SourcePatent: "A", TargetPatent: "B"}
	e2 := domain.CitationEdge{SourcePatent: "A", TargetPatent: "C"}
	repo.citStates[edgeKey(e1)] = "stored"
	repo.citStates[edgeKey(e2)] = "under_review"
	h := NewHistory(repo)
	ctx := context.Background()

	if _, err := h.Apply(ctx, SetCitationReviewState("p", []domain.CitationEdge{e1, e2}, "ignored")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, _, err := h.Undo(ctx); err != nil {
		t.Fatalf("undo: %v", err)
	}
	// undo must restore each edge to its own distinct prior state.
	if repo.citStates[edgeKey(e1)] != "stored" || repo.citStates[edgeKey(e2)] != "under_review" {
		t.Fatalf("undo did not restore per-edge prior states: %v", repo.citStates)
	}
}

func TestApplyFailureLeavesHistoryEmpty(t *testing.T) {
	repo := newFakeRepo()
	repo.failCit = true
	h := NewHistory(repo)
	if _, err := h.Apply(context.Background(), SetCitationReviewState("p", []domain.CitationEdge{{SourcePatent: "A", TargetPatent: "B"}}, "ignored")); err == nil {
		t.Fatal("expected apply to fail")
	}
	if h.CanUndo() {
		t.Fatal("a failed apply must not be recorded for undo")
	}
}

func TestUndoEmptyHistory(t *testing.T) {
	h := NewHistory(newFakeRepo())
	if _, _, err := h.Undo(context.Background()); !errors.Is(err, ErrNothingToUndo) {
		t.Fatalf("expected ErrNothingToUndo, got %v", err)
	}
}

func TestNonReversibleChange(t *testing.T) {
	h := NewHistory(newFakeRepo())
	ctx := context.Background()
	if _, err := h.Apply(ctx, AddNote("p", "US1", "hello")); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, _, err := h.Undo(ctx); !errors.Is(err, ErrNotReversible) {
		t.Fatalf("expected ErrNotReversible, got %v", err)
	}
}

func TestRecordingSaveAndReplay(t *testing.T) {
	repo := newFakeRepo()
	repo.patStates["US1"] = "stored"
	e := domain.CitationEdge{SourcePatent: "A", TargetPatent: "B"}
	repo.citStates[edgeKey(e)] = "stored"
	h := NewHistory(repo)
	ctx := context.Background()

	h.StartRecording()
	if _, err := h.Apply(ctx, SetPatentReviewState("p", []string{"US1"}, "ignored")); err != nil {
		t.Fatalf("apply patent: %v", err)
	}
	if _, err := h.Apply(ctx, SetCitationReviewState("p", []domain.CitationEdge{e}, "ignored")); err != nil {
		t.Fatalf("apply citation: %v", err)
	}
	h.StopRecording()
	if h.RecordingLen() != 2 {
		t.Fatalf("expected 2 changes captured, got %d", h.RecordingLen())
	}

	path := filepath.Join(t.TempDir(), "rec.json")
	if err := h.SaveRecording(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Replay onto a fresh repo at the same starting state.
	fresh := newFakeRepo()
	fresh.patStates["US1"] = "stored"
	fresh.citStates[edgeKey(e)] = "stored"
	n, err := Replay(ctx, fresh, path)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 changes replayed, got %d", n)
	}
	if fresh.patStates["US1"] != "ignored" || fresh.citStates[edgeKey(e)] != "ignored" {
		t.Fatalf("replay did not reproduce end state: pat=%v cit=%v", fresh.patStates, fresh.citStates)
	}
}

func TestRecordingCapturesOnlyWhileActive(t *testing.T) {
	h := NewHistory(newFakeRepo())
	ctx := context.Background()
	_, _ = h.Apply(ctx, SetPatentReviewState("p", []string{"US1"}, "ignored"))
	if h.RecordingLen() != 0 {
		t.Fatal("change applied before StartRecording must not be captured")
	}
	h.StartRecording()
	_, _ = h.Apply(ctx, SetPatentReviewState("p", []string{"US2"}, "ignored"))
	h.StopRecording()
	_, _ = h.Apply(ctx, SetPatentReviewState("p", []string{"US3"}, "ignored"))
	if h.RecordingLen() != 1 {
		t.Fatalf("expected exactly 1 captured change, got %d", h.RecordingLen())
	}
}

func TestApplyClearsRedo(t *testing.T) {
	repo := newFakeRepo()
	h := NewHistory(repo)
	ctx := context.Background()
	_, _ = h.Apply(ctx, SetPatentReviewState("p", []string{"US1"}, "ignored"))
	_, _, _ = h.Undo(ctx)
	if !h.CanRedo() {
		t.Fatal("expected redo available after undo")
	}
	_, _ = h.Apply(ctx, SetPatentReviewState("p", []string{"US2"}, "ignored"))
	if h.CanRedo() {
		t.Fatal("a fresh apply must clear the redo stack")
	}
}
