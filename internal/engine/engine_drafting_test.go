package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"patentmine/internal/ai"
	"patentmine/internal/domain"
)

// fakeDrafter echoes a canned completion and records the last prompt it saw, so
// tests can assert the engine grounded the prompt correctly.
type fakeDrafter struct {
	reply      string
	lastPrompt string
}

func (f *fakeDrafter) Complete(_ context.Context, prompt string) (string, error) {
	f.lastPrompt = prompt
	return f.reply, nil
}
func (f *fakeDrafter) Provider() ai.Provider { return ai.Provider("fake") }
func (f *fakeDrafter) Model() string         { return "fake-1" }

// fakeExtractor is a fakeDrafter that also satisfies ai.Extractor, so tests can
// exercise the AI/OCR text-extraction path for scanned documents.
type fakeExtractor struct {
	fakeDrafter
	extractReply string
}

func (f *fakeExtractor) ExtractText(_ context.Context, _ []byte, _ string) (string, error) {
	return f.extractReply, nil
}

func newDraftEngine(t *testing.T, drafter ai.Drafter) (*Engine, domain.ProjectID) {
	t.Helper()
	eng, repo := newTestEngine(t, nil)
	if drafter != nil {
		WithDrafter(drafter)(eng)
	}
	WithDocsExportDir(t.TempDir())(eng)
	ctx := context.Background()
	proj := domain.Project{ID: "p1", Name: "Matter 1", CreatedAt: time.Now().UTC(),
		ApplicationNumber: "16/123,456", Inventors: []string{"Ada Lovelace"}, AttorneyDocketNumber: "ACME-001"}
	if err := repo.SaveProject(ctx, proj); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	return eng, proj.ID
}

func TestCreateDraftSeedsSkeleton(t *testing.T) {
	eng, proj := newDraftEngine(t, nil)
	d, err := eng.CreateDraft(context.Background(), proj, domain.DraftProvisional, "My Invention")
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if d.ID == "" || len(d.Sections) == 0 {
		t.Fatalf("draft not seeded: %+v", d)
	}
	if d.Kind != domain.DraftProvisional || d.Title != "My Invention" {
		t.Errorf("draft header wrong: %+v", d)
	}
}

func TestCreateDraftRejectsBadKindAndProject(t *testing.T) {
	eng, proj := newDraftEngine(t, nil)
	if _, err := eng.CreateDraft(context.Background(), proj, domain.DraftKind("bogus"), ""); err == nil {
		t.Error("expected error for bad kind")
	}
	if _, err := eng.CreateDraft(context.Background(), "nope", domain.DraftProvisional, ""); err == nil {
		t.Error("expected error for unknown project")
	}
}

func TestDraftSectionGroundsAndVerifies(t *testing.T) {
	ctx := context.Background()
	fake := &fakeDrafter{reply: "The field of the invention is rodent control with a self-cleaning surface."}
	eng, proj := newDraftEngine(t, fake)

	d, err := eng.CreateDraft(ctx, proj, domain.DraftNonprovisional, "Self-Cleaning Widget")
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	res, err := eng.DraftSection(ctx, DraftAIInput{
		Draft:          d.ID,
		SectionOrdinal: 0,
		Instruction:    "Draft the field section.",
		Pinned:         []string{"self-cleaning surface"},
		PriorArt:       []ai.PriorArtRef{{Number: "US9999999B2", Relevant: "Does not teach self-cleaning."}},
	})
	if err != nil {
		t.Fatalf("DraftSection: %v", err)
	}
	if res.Provider != "fake" || res.Model != "fake-1" {
		t.Errorf("provenance wrong: %+v", res)
	}
	// The pinned span is present verbatim in the reply, so no problems.
	if len(res.Problems) != 0 {
		t.Errorf("unexpected problems: %v", res.Problems)
	}
	// The engine must have grounded the prompt with the project + prior art.
	for _, want := range []string{"Self-Cleaning Widget", "Ada Lovelace", "US9999999B2", "Does not teach self-cleaning."} {
		if !strings.Contains(fake.lastPrompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestDraftSectionFlagsMissingPinned(t *testing.T) {
	ctx := context.Background()
	fake := &fakeDrafter{reply: "A paraphrase that omits the required phrase."}
	eng, proj := newDraftEngine(t, fake)
	d, _ := eng.CreateDraft(ctx, proj, domain.DraftProvisional, "X")

	res, err := eng.DraftSection(ctx, DraftAIInput{Draft: d.ID, SectionOrdinal: 0, Pinned: []string{"a very specific verbatim clause"}})
	if err != nil {
		t.Fatalf("DraftSection: %v", err)
	}
	if len(res.Problems) == 0 {
		t.Fatal("expected a problem for the un-reproduced pinned span")
	}
}

func TestDraftSectionWithoutDrafterErrors(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, nil) // no drafter wired
	d, _ := eng.CreateDraft(ctx, proj, domain.DraftProvisional, "X")
	if _, err := eng.DraftSection(ctx, DraftAIInput{Draft: d.ID, SectionOrdinal: 0}); err == nil {
		t.Fatal("expected error when no drafter configured")
	}
}

func TestExportDraftDocxWritesFile(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, nil)
	d, _ := eng.CreateDraft(ctx, proj, domain.DraftNonprovisional, "Widget")
	// Give it a claim so the listing renders.
	d.Claims = []domain.DraftClaim{{Ordinal: 0, Number: 1, Type: domain.ClaimIndependent, Status: domain.AmendOriginal, Text: "A widget."}}
	if _, err := eng.SaveDraft(ctx, d); err != nil {
		t.Fatalf("SaveDraft: %v", err)
	}

	path, err := eng.ExportDraftDocx(ctx, d.ID)
	if err != nil {
		t.Fatalf("ExportDraftDocx: %v", err)
	}
	if filepath.Ext(path) != ".docx" {
		t.Errorf("expected .docx, got %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read exported docx: %v", err)
	}
	if !validDocxContains(t, data, "Widget") || !validDocxContains(t, data, "A widget.") {
		t.Error("exported docx missing expected content")
	}
}

func TestImportOfficeActionStoresBlobAndLinks(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, nil)

	// A .txt source doubles as extracted text.
	src := filepath.Join(t.TempDir(), "oa.txt")
	if err := os.WriteFile(src, []byte("Claims 1-3 are rejected under 35 U.S.C. 103 over Smith."), 0o644); err != nil {
		t.Fatal(err)
	}
	oa, err := eng.ImportOfficeAction(ctx, ImportOfficeActionInput{
		Project: proj, ApplicationNumber: "16/123,456", Type: domain.OANonFinal,
		Examiner: "Jane Doe", ArtUnit: "2151", SourcePath: src,
	})
	if err != nil {
		t.Fatalf("ImportOfficeAction: %v", err)
	}
	if oa.BlobPath == "" || oa.BlobHash == "" {
		t.Errorf("blob not stored: %+v", oa)
	}
	if !strings.Contains(oa.ExtractedText, "rejected under 35 U.S.C. 103") {
		t.Errorf("extracted text not captured: %q", oa.ExtractedText)
	}
	if _, err := os.Stat(oa.BlobPath); err != nil {
		t.Errorf("blob file not on disk: %v", err)
	}

	// A response draft can be created and its grounding pulls the OA text.
	resp, err := eng.CreateDraft(ctx, proj, domain.DraftOAResponse, "")
	if err != nil {
		t.Fatalf("CreateDraft response: %v", err)
	}
	resp.OfficeActionID = oa.ID
	if _, err := eng.SaveDraft(ctx, resp); err != nil {
		t.Fatalf("SaveDraft response: %v", err)
	}

	fake := &fakeDrafter{reply: "Applicant respectfully traverses."}
	WithDrafter(fake)(eng)
	if _, err := eng.DraftSection(ctx, DraftAIInput{Draft: resp.ID, SectionOrdinal: 0}); err != nil {
		t.Fatalf("DraftSection: %v", err)
	}
	if !strings.Contains(fake.lastPrompt, "rejected under 35 U.S.C. 103 over Smith") {
		t.Errorf("office action text not grounded into response prompt:\n%s", fake.lastPrompt)
	}
}

func TestImportOfficeActionSeedsDeadlineAndDocument(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, nil)

	src := filepath.Join(t.TempDir(), "OfficeAction.txt")
	if err := os.WriteFile(src, []byte("rejection text"), 0o644); err != nil {
		t.Fatal(err)
	}
	mailed := time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC)
	oa, err := eng.ImportOfficeAction(ctx, ImportOfficeActionInput{
		Project: proj, Type: domain.OANonFinal, MailDate: mailed, SourcePath: src,
	})
	if err != nil {
		t.Fatalf("ImportOfficeAction: %v", err)
	}

	// The response deadline is seeded three months out and the action opens.
	wantDue := mailed.AddDate(0, 3, 0)
	if !oa.ResponseDue.Equal(wantDue) {
		t.Errorf("ResponseDue = %v, want %v", oa.ResponseDue, wantDue)
	}
	if oa.Status != domain.OAStatusOpen {
		t.Errorf("Status = %q, want open", oa.Status)
	}

	// The examiner's letter shows up in the matter's document list.
	docs, err := eng.ListMatterDocuments(ctx, proj)
	if err != nil {
		t.Fatalf("ListMatterDocuments: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("documents = %d, want 1", len(docs))
	}
	if docs[0].Kind != domain.MatterDocOA || docs[0].OfficeActionID != oa.ID {
		t.Errorf("OA document = {kind:%q oa:%q}, want {oa %s}", docs[0].Kind, docs[0].OfficeActionID, oa.ID)
	}
	if docs[0].DisplayName != "OfficeAction.txt" {
		t.Errorf("display name = %q, want the source base name", docs[0].DisplayName)
	}
}

func TestExtractDocumentTextWithAI(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, &fakeExtractor{extractReply: "OCR: Claims 1-5 are rejected."})

	// Import a document whose pure-Go extraction yields nothing (a non-PDF given
	// a .pdf name stands in for a scanned, image-only office action).
	src := filepath.Join(t.TempDir(), "scanned.pdf")
	if err := os.WriteFile(src, []byte("%PDF-1.4 not-really-extractable"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := eng.ImportMatterDocument(ctx, ImportMatterDocumentInput{
		Project: proj, Kind: domain.MatterDocOA, SourcePath: src,
	})
	if err != nil {
		t.Fatalf("ImportMatterDocument: %v", err)
	}
	if doc.ExtractedText != "" {
		t.Fatalf("expected empty extracted text before OCR, got %q", doc.ExtractedText)
	}

	got, err := eng.ExtractDocumentText(ctx, doc.ID)
	if err != nil {
		t.Fatalf("ExtractDocumentText: %v", err)
	}
	if got.ExtractedText != "OCR: Claims 1-5 are rejected." {
		t.Fatalf("extracted text = %q, want the OCR reply", got.ExtractedText)
	}
	// The text is persisted, not just returned.
	reloaded, err := eng.MatterDocument(ctx, doc.ID)
	if err != nil {
		t.Fatalf("MatterDocument: %v", err)
	}
	if reloaded.ExtractedText != got.ExtractedText {
		t.Fatalf("OCR text not persisted: %q", reloaded.ExtractedText)
	}
}

func TestExtractDocumentTextWithoutExtractorErrors(t *testing.T) {
	ctx := context.Background()
	// A plain fakeDrafter implements Drafter but not ai.Extractor.
	eng, proj := newDraftEngine(t, &fakeDrafter{})

	src := filepath.Join(t.TempDir(), "ref.txt")
	if err := os.WriteFile(src, []byte("plain text"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := eng.ImportMatterDocument(ctx, ImportMatterDocumentInput{
		Project: proj, Kind: domain.MatterDocReference, SourcePath: src,
	})
	if err != nil {
		t.Fatalf("ImportMatterDocument: %v", err)
	}
	if _, err := eng.ExtractDocumentText(ctx, doc.ID); err == nil {
		t.Fatal("ExtractDocumentText without an ai.Extractor: want error, got nil")
	}
}

func TestMatterEventAddListDelete(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, nil)

	ev, err := eng.AddMatterEvent(ctx, AddMatterEventInput{
		Project: proj, Kind: domain.MatterEventPhone, Party: "Examiner Menefee",
		Summary: "Interview re: 103 rejection", Comment: "Agreed to add the limitation from claim 5.",
	})
	if err != nil {
		t.Fatalf("AddMatterEvent: %v", err)
	}
	if ev.OccurredAt.IsZero() {
		t.Error("OccurredAt should default to now when unset")
	}

	events, err := eng.ListMatterEvents(ctx, proj)
	if err != nil {
		t.Fatalf("ListMatterEvents: %v", err)
	}
	if len(events) != 1 || events[0].Party != "Examiner Menefee" {
		t.Fatalf("events = %+v, want one with party Examiner Menefee", events)
	}

	if err := eng.DeleteMatterEvent(ctx, ev.ID); err != nil {
		t.Fatalf("DeleteMatterEvent: %v", err)
	}
	if events, _ := eng.ListMatterEvents(ctx, proj); len(events) != 0 {
		t.Fatalf("events after delete = %d, want 0", len(events))
	}
}

func TestMatterDocumentImportRenameDelete(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, nil)

	src := filepath.Join(t.TempDir(), "Smith.txt")
	if err := os.WriteFile(src, []byte("prior art disclosure"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := eng.ImportMatterDocument(ctx, ImportMatterDocumentInput{
		Project: proj, Kind: domain.MatterDocReference, SourcePath: src,
	})
	if err != nil {
		t.Fatalf("ImportMatterDocument: %v", err)
	}
	if doc.ExtractedText != "prior art disclosure" {
		t.Errorf("extracted text = %q", doc.ExtractedText)
	}
	if _, err := os.Stat(doc.BlobPath); err != nil {
		t.Errorf("blob not on disk: %v", err)
	}

	renamed, err := eng.RenameMatterDocument(ctx, doc.ID, "Smith 2019 (primary)")
	if err != nil {
		t.Fatalf("RenameMatterDocument: %v", err)
	}
	if renamed.DisplayName != "Smith 2019 (primary)" {
		t.Errorf("rename did not take: %q", renamed.DisplayName)
	}

	if err := eng.DeleteMatterDocument(ctx, doc.ID); err != nil {
		t.Fatalf("DeleteMatterDocument: %v", err)
	}
	if docs, _ := eng.ListMatterDocuments(ctx, proj); len(docs) != 0 {
		t.Errorf("documents after delete = %d, want 0", len(docs))
	}
	// A standalone document owns its blob, so it is unlinked on delete.
	if _, err := os.Stat(doc.BlobPath); !os.IsNotExist(err) {
		t.Errorf("standalone blob should be removed on delete, stat err = %v", err)
	}
}

// validDocxContains unzips a .docx and reports whether document.xml contains s.
func validDocxContains(t *testing.T, data []byte, s string) bool {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open docx: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			_ = rc.Close()
			return strings.Contains(string(b), s)
		}
	}
	return false
}
