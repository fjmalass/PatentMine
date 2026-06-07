package engine

import (
	"context"
	"errors"
	"os"
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

func TestProvisionalCoverSheetSaveAndApprove(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, nil)

	// No cover sheet yet.
	if _, err := eng.ProvisionalCoverSheet(ctx, proj); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// Save an incomplete sheet (no signature) — approval must be refused.
	cs := domain.ProvisionalCoverSheet{
		Project:   proj,
		Title:     "Self-Cleaning Widget",
		Inventors: []domain.ProvisionalCoverSheetInventor{{GivenName: "Ada", FamilyName: "Lovelace"}},
	}
	saved, err := eng.SaveProvisionalCoverSheet(ctx, cs)
	if err != nil {
		t.Fatalf("SaveProvisionalCoverSheet: %v", err)
	}
	if saved.ID == "" || saved.Approved {
		t.Fatalf("unexpected saved sheet: %+v", saved)
	}

	res, err := eng.ApproveProvisionalCoverSheetPDF(ctx, proj)
	if err != nil {
		t.Fatalf("ApproveProvisionalCoverSheetPDF (incomplete): %v", err)
	}
	if len(res.Missing) == 0 || res.Path != "" {
		t.Fatalf("incomplete sheet should be blocked with Missing, got %+v", res)
	}

	// Complete it, then approve → renders the PDF and files it as a document.
	saved.SignerName = "Pat Attorney"
	saved.SignatureText = "/Pat Attorney/"
	if _, err := eng.SaveProvisionalCoverSheet(ctx, saved); err != nil {
		t.Fatalf("SaveProvisionalCoverSheet (complete): %v", err)
	}
	res, err = eng.ApproveProvisionalCoverSheetPDF(ctx, proj)
	if err != nil {
		t.Fatalf("ApproveProvisionalCoverSheetPDF: %v", err)
	}
	if len(res.Missing) != 0 {
		t.Fatalf("complete sheet should approve, missing: %v", res.Missing)
	}
	if !res.ProvisionalCoverSheet.Approved {
		t.Error("approved sheet should be marked Approved")
	}
	if st, err := os.Stat(res.Path); err != nil || st.Size() == 0 {
		t.Fatalf("rendered cover sheet PDF missing or empty: %v", err)
	}
	if res.Document.Kind != domain.MatterDocCoversheet {
		t.Errorf("document kind = %q, want coversheet", res.Document.Kind)
	}

	// The rendered PDF is tracked as a matter document.
	docs, err := eng.ListMatterDocuments(ctx, proj)
	if err != nil {
		t.Fatalf("ListMatterDocuments: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 tracked document, got %d", len(docs))
	}

	// Editing after approval clears the approval (must re-approve).
	reloaded, err := eng.ProvisionalCoverSheet(ctx, proj)
	if err != nil {
		t.Fatalf("ProvisionalCoverSheet: %v", err)
	}
	reloaded.Title = "Self-Cleaning Widget Mk II"
	again, err := eng.SaveProvisionalCoverSheet(ctx, reloaded)
	if err != nil {
		t.Fatalf("SaveProvisionalCoverSheet (edit): %v", err)
	}
	if again.Approved {
		t.Error("editing an approved sheet must clear Approved")
	}
}

func TestPreviewProvisionalCoverSheetRenders(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, nil)
	if _, err := eng.SaveProvisionalCoverSheet(ctx, domain.ProvisionalCoverSheet{Project: proj, Title: "X"}); err != nil {
		t.Fatalf("SaveProvisionalCoverSheet: %v", err)
	}
	pdf, err := eng.PreviewProvisionalCoverSheetPDF(ctx, proj)
	if err != nil {
		t.Fatalf("PreviewProvisionalCoverSheetPDF: %v", err)
	}
	if len(pdf) == 0 {
		t.Fatal("preview produced no PDF bytes")
	}
}
