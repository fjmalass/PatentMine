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
