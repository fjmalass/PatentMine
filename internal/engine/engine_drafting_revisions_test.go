package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/export/markdown"
)

// seedResponseDraft creates an OA-response draft with one amended claim and a
// remarks body, returning the saved draft.
func seedResponseDraft(t *testing.T, eng *Engine, proj domain.ProjectID) domain.Draft {
	t.Helper()
	ctx := context.Background()
	d, err := eng.CreateDraft(ctx, proj, domain.DraftOAResponse, "Widget Response")
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	d.Claims = []domain.DraftClaim{
		{Number: 1, Status: domain.AmendCurrentlyAmended,
			BaseText: "A widget comprising a first arm.",
			Text:     "A widget comprising a second arm."},
	}
	for i := range d.Sections {
		if d.Sections[i].Kind == domain.SectionRemarks {
			d.Sections[i].Body = "The rejection is respectfully traversed."
		}
	}
	saved, err := eng.SaveDraft(ctx, d)
	if err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	return saved
}

func TestSnapshotDraftResponseWritesFilesAndRevision(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, nil)
	d := seedResponseDraft(t, eng, proj)

	res, err := eng.SnapshotDraftResponse(ctx, d.ID, domain.RevisionManual, "first cut")
	if err != nil {
		t.Fatalf("SnapshotDraftResponse: %v", err)
	}
	if res.Revision.Revno != 1 {
		t.Errorf("revno = %d, want 1", res.Revision.Revno)
	}

	// Both files exist on disk under the matter's responses dir, with the
	// computed amendment markup.
	claims, err := os.ReadFile(res.ClaimsDoc.BlobPath)
	if err != nil {
		t.Fatalf("read claims file: %v", err)
	}
	if !strings.Contains(string(claims), "<del>first</del>") || !strings.Contains(string(claims), "<ins>second</ins>") {
		t.Errorf("new_claims.md missing amendment markup:\n%s", claims)
	}
	if filepath.Base(res.ClaimsDoc.BlobPath) != markdown.ClaimsFilename {
		t.Errorf("claims filename = %q, want %q", filepath.Base(res.ClaimsDoc.BlobPath), markdown.ClaimsFilename)
	}
	response, err := os.ReadFile(res.ResponseDoc.BlobPath)
	if err != nil {
		t.Fatalf("read response file: %v", err)
	}
	if !strings.Contains(string(response), "respectfully traversed") {
		t.Errorf("response.md missing remarks:\n%s", response)
	}

	// The two files are tracked as matter documents.
	docs, err := eng.ListMatterDocuments(ctx, proj)
	if err != nil {
		t.Fatalf("ListMatterDocuments: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 tracked documents, got %d", len(docs))
	}

	// The revision history has one entry, recoverable in full.
	revs, err := eng.ListDraftRevisions(ctx, d.ID)
	if err != nil {
		t.Fatalf("ListDraftRevisions: %v", err)
	}
	if len(revs) != 1 || revs[0].Label != "first cut" {
		t.Fatalf("unexpected revisions: %+v", revs)
	}
	full, err := eng.DraftRevision(ctx, revs[0].ID)
	if err != nil {
		t.Fatalf("DraftRevision: %v", err)
	}
	if len(full.Claims) != 1 || full.Claims[0].Number != 1 {
		t.Errorf("revision did not preserve structured claims: %+v", full.Claims)
	}
}

func TestSnapshotKeepsLatestOnDiskAndAppendsHistory(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, nil)
	d := seedResponseDraft(t, eng, proj)

	if _, err := eng.SnapshotDraftResponse(ctx, d.ID, domain.RevisionManual, "r1"); err != nil {
		t.Fatalf("snapshot 1: %v", err)
	}
	// Edit the head, then snapshot again.
	d.Claims[0].Text = "A widget comprising a third arm."
	if _, err := eng.SaveDraft(ctx, d); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}
	res2, err := eng.SnapshotDraftResponse(ctx, d.ID, domain.RevisionManual, "r2")
	if err != nil {
		t.Fatalf("snapshot 2: %v", err)
	}
	if res2.Revision.Revno != 2 {
		t.Errorf("second revno = %d, want 2", res2.Revision.Revno)
	}

	// History grows, but only the two latest files remain tracked (in place).
	revs, err := eng.ListDraftRevisions(ctx, d.ID)
	if err != nil {
		t.Fatalf("ListDraftRevisions: %v", err)
	}
	if len(revs) != 2 {
		t.Fatalf("expected 2 revisions, got %d", len(revs))
	}
	docs, err := eng.ListMatterDocuments(ctx, proj)
	if err != nil {
		t.Fatalf("ListMatterDocuments: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("expected 2 tracked documents after re-snapshot, got %d", len(docs))
	}
	// The on-disk file holds the latest text.
	claims, err := os.ReadFile(res2.ClaimsDoc.BlobPath)
	if err != nil {
		t.Fatalf("read claims: %v", err)
	}
	if !strings.Contains(string(claims), "<ins>third</ins>") {
		t.Errorf("latest new_claims.md should show the newest amendment:\n%s", claims)
	}
}

func TestRestoreDraftRevisionRollsBackHead(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, nil)
	d := seedResponseDraft(t, eng, proj)

	r1, err := eng.SnapshotDraftResponse(ctx, d.ID, domain.RevisionManual, "r1")
	if err != nil {
		t.Fatalf("snapshot 1: %v", err)
	}
	// Change the head away from r1.
	d.Claims[0].Text = "A widget comprising a third arm."
	if _, err := eng.SaveDraft(ctx, d); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	restored, err := eng.RestoreDraftRevision(ctx, d.ID, r1.Revision.ID)
	if err != nil {
		t.Fatalf("RestoreDraftRevision: %v", err)
	}
	if got := restored.Claims[0].Text; got != "A widget comprising a second arm." {
		t.Errorf("restored claim text = %q, want the r1 text", got)
	}
	// Restore is itself recorded: r1, r2(edited never snapshotted), restore.
	revs, err := eng.ListDraftRevisions(ctx, d.ID)
	if err != nil {
		t.Fatalf("ListDraftRevisions: %v", err)
	}
	if len(revs) != 2 {
		t.Fatalf("expected 2 revisions (r1 + restore checkpoint), got %d", len(revs))
	}
}
