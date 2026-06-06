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
	"patentmine/internal/observability"
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

func TestOfficeActionObservability(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, nil)
	metrics := observability.NewMetrics()
	observed, err := observability.Open(filepath.Join(t.TempDir(), "logs"), "test", "test-version")
	if err != nil {
		t.Fatalf("Open observability: %v", err)
	}
	t.Cleanup(func() { _ = observed.Close() })
	WithMetrics(metrics)(eng)
	WithActivityRecorder(observed.Activity)(eng)

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
	if _, err := eng.OfficeAction(ctx, oa.ID); err != nil {
		t.Fatalf("OfficeAction: %v", err)
	}
	if _, err := eng.ListOfficeActions(ctx, proj); err != nil {
		t.Fatalf("ListOfficeActions: %v", err)
	}
	if _, err := eng.SaveOfficeActionNotes(ctx, oa.ID, "traverse"); err != nil {
		t.Fatalf("SaveOfficeActionNotes: %v", err)
	}
	updated, err := eng.UpdateOfficeActionMeta(ctx, oa.ID, "New Name", "Jane Doe", mailed.Format(domain.DateLayout), domain.OAFinal, "2151", "16/123,456", domain.OAStatusResponded)
	if err != nil {
		t.Fatalf("UpdateOfficeActionMeta: %v", err)
	}
	if updated.Status != domain.OAStatusResponded {
		t.Fatalf("updated status = %q, want responded", updated.Status)
	}
	if updated.StatusChangedAt.IsZero() || !updated.StatusChangedAt.After(oa.StatusChangedAt) {
		t.Fatalf("StatusChangedAt not bumped on status change: was %v, now %v", oa.StatusChangedAt, updated.StatusChangedAt)
	}
	if _, err := eng.DeleteOfficeAction(ctx, oa.ID, false); err != nil {
		t.Fatalf("DeleteOfficeAction: %v", err)
	}

	snap := metrics.Snapshot()
	for _, name := range []string{
		observability.MetricOfficeActionImport,
		observability.MetricOfficeActionGet,
		observability.MetricOfficeActionList,
		observability.MetricOfficeActionSaveNotes,
		observability.MetricOfficeActionUpdate,
		observability.MetricOfficeActionDelete,
	} {
		if snap.Timings[name].Count != 1 {
			t.Fatalf("timing %s count = %d, want 1", name, snap.Timings[name].Count)
		}
	}
	for _, name := range []string{
		observability.MetricOfficeActionImportTotal,
		observability.MetricOfficeActionGetTotal,
		observability.MetricOfficeActionListTotal,
		observability.MetricOfficeActionSaveNotesTotal,
		observability.MetricOfficeActionUpdateTotal,
		observability.MetricOfficeActionDeleteTotal,
	} {
		if snap.Counters[name] != 1 {
			t.Fatalf("counter %s = %d, want 1", name, snap.Counters[name])
		}
	}
	if snap.Gauges[observability.MetricOfficeActionListReturned] != 1 {
		t.Fatalf("list returned gauge = %d, want 1", snap.Gauges[observability.MetricOfficeActionListReturned])
	}

	records, err := observability.ReadActivityRecords(observed.LogsDir, observability.ActivityQuery{
		Limit:  10,
		Entity: observability.EntityOfficeAction,
	})
	if err != nil {
		t.Fatalf("ReadActivityRecords: %v", err)
	}
	seen := make(map[string]bool)
	for _, rec := range records {
		seen[rec.Action] = true
		if rec.Status != observability.StatusCommitted && rec.Status != observability.StatusObserved {
			t.Fatalf("unexpected office-action status %q", rec.Status)
		}
	}
	for _, action := range []string{
		observability.ActionOfficeActionImport,
		observability.ActionOfficeActionGet,
		observability.ActionOfficeActionList,
		observability.ActionOfficeActionSaveNotes,
		observability.ActionOfficeActionUpdate,
		observability.ActionOfficeActionDelete,
	} {
		if !seen[action] {
			t.Fatalf("missing activity action %s in %#v", action, records)
		}
	}
}

func TestExtractDocumentTextReportsNoEmbeddedTextWithoutAI(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, &fakeDrafter{})

	src := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(src, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := eng.ImportMatterDocument(ctx, ImportMatterDocumentInput{
		Project: proj, Kind: domain.MatterDocOA, SourcePath: src,
	})
	if err != nil {
		t.Fatalf("ImportMatterDocument: %v", err)
	}
	if doc.ExtractedText != "" {
		t.Fatalf("expected empty extracted text, got %q", doc.ExtractedText)
	}

	if _, err := eng.ExtractDocumentText(ctx, doc.ID); err == nil || !strings.Contains(err.Error(), "no embedded text") {
		t.Fatalf("ExtractDocumentText error = %v, want no embedded text", err)
	}
}

type fakeExtractor struct {
	fakeDrafter
	ocrReply string
	called   bool
}

func (f *fakeExtractor) ExtractText(ctx context.Context, data []byte, mimeType string) (string, error) {
	f.called = true
	return f.ocrReply, nil
}

func TestExtractDocumentTextWithAIExtractor(t *testing.T) {
	ctx := context.Background()
	fake := &fakeExtractor{ocrReply: "Extracted via OCR"}
	eng, proj := newDraftEngine(t, fake)

	src := filepath.Join(t.TempDir(), "scanned.txt")
	if err := os.WriteFile(src, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := eng.ImportMatterDocument(ctx, ImportMatterDocumentInput{
		Project: proj, Kind: domain.MatterDocOA, SourcePath: src,
	})
	if err != nil {
		t.Fatalf("ImportMatterDocument: %v", err)
	}

	res, err := eng.ExtractDocumentText(ctx, doc.ID)
	if err != nil {
		t.Fatalf("ExtractDocumentText: %v", err)
	}
	if !fake.called {
		t.Error("expected AI extractor to be called, but was not")
	}
	if res.ExtractedText != "Extracted via OCR" {
		t.Errorf("extracted text = %q, want %q", res.ExtractedText, "Extracted via OCR")
	}
}

func TestMatterDocumentImportReportsParserErrors(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, nil)

	src := filepath.Join(t.TempDir(), "legacy.xls")
	if err := os.WriteFile(src, []byte("plain text"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := eng.ImportMatterDocument(ctx, ImportMatterDocumentInput{
		Project: proj, Kind: domain.MatterDocReference, SourcePath: src,
	})
	if err == nil || !strings.Contains(err.Error(), "legacy .xls") {
		t.Fatalf("ImportMatterDocument error = %v, want legacy .xls parser error", err)
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

func TestMatterDocumentTags(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, nil)

	src := filepath.Join(t.TempDir(), "Jones.txt")
	if err := os.WriteFile(src, []byte("secondary prior art"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := eng.ImportMatterDocument(ctx, ImportMatterDocumentInput{
		Project: proj, Kind: domain.MatterDocReference, SourcePath: src, Tags: []string{"prior_art"},
	})
	if err != nil {
		t.Fatalf("ImportMatterDocument: %v", err)
	}
	if got := tagNamesForTest(doc.Tags); got != "prior_art" {
		t.Fatalf("import tags = %q, want prior_art", got)
	}

	doc, err = eng.TagMatterDocument(ctx, doc.ID, "primary_ref")
	if err != nil {
		t.Fatalf("TagMatterDocument: %v", err)
	}
	if got := tagNamesForTest(doc.Tags); got != "primary_ref,prior_art" {
		t.Fatalf("tags after add = %q", got)
	}

	docs, err := eng.ListMatterDocuments(ctx, proj)
	if err != nil {
		t.Fatalf("ListMatterDocuments: %v", err)
	}
	if len(docs) != 1 || tagNamesForTest(docs[0].Tags) != "primary_ref,prior_art" {
		t.Fatalf("listed docs = %+v", docs)
	}

	doc, err = eng.UntagMatterDocument(ctx, doc.ID, "prior_art")
	if err != nil {
		t.Fatalf("UntagMatterDocument: %v", err)
	}
	if got := tagNamesForTest(doc.Tags); got != "primary_ref" {
		t.Fatalf("tags after remove = %q", got)
	}
}

func TestMatterDocumentMultipleOfficeActions(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, nil)

	oaSrc1 := filepath.Join(t.TempDir(), "oa1.txt")
	oaSrc2 := filepath.Join(t.TempDir(), "oa2.txt")
	if err := os.WriteFile(oaSrc1, []byte("first action"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oaSrc2, []byte("second action"), 0o644); err != nil {
		t.Fatal(err)
	}
	oa1, err := eng.ImportOfficeAction(ctx, ImportOfficeActionInput{Project: proj, Type: domain.OANonFinal, SourcePath: oaSrc1})
	if err != nil {
		t.Fatalf("ImportOfficeAction oa1: %v", err)
	}
	oa2, err := eng.ImportOfficeAction(ctx, ImportOfficeActionInput{Project: proj, Type: domain.OAFinal, SourcePath: oaSrc2})
	if err != nil {
		t.Fatalf("ImportOfficeAction oa2: %v", err)
	}

	refSrc := filepath.Join(t.TempDir(), "shared-ref.txt")
	if err := os.WriteFile(refSrc, []byte("shared reference"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := eng.ImportMatterDocument(ctx, ImportMatterDocumentInput{
		Project: proj, Kind: domain.MatterDocReference, SourcePath: refSrc, OfficeActionIDs: []string{oa1.ID, oa2.ID},
	})
	if err != nil {
		t.Fatalf("ImportMatterDocument: %v", err)
	}
	if got := strings.Join(doc.OfficeActionIDs, ","); got != oa1.ID+","+oa2.ID {
		t.Fatalf("office action ids = %q", got)
	}

	doc, err = eng.UnassignMatterDocumentOfficeAction(ctx, doc.ID, oa1.ID)
	if err != nil {
		t.Fatalf("UnassignMatterDocumentOfficeAction: %v", err)
	}
	if got := strings.Join(doc.OfficeActionIDs, ","); got != oa2.ID {
		t.Fatalf("office action ids after unassign = %q", got)
	}

	doc, err = eng.AssignMatterDocumentOfficeAction(ctx, doc.ID, oa1.ID)
	if err != nil {
		t.Fatalf("AssignMatterDocumentOfficeAction: %v", err)
	}
	if !sameStrings(doc.OfficeActionIDs, []string{oa1.ID, oa2.ID}) {
		t.Fatalf("office action ids after reassign = %q", strings.Join(doc.OfficeActionIDs, ","))
	}
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
		if seen[s] < 0 {
			return false
		}
	}
	return true
}

func tagNamesForTest(tags []domain.Tag) string {
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	return strings.Join(names, ",")
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

func TestExtractDocTextReadsDocx(t *testing.T) {
	// Create a mock docx in memory
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatalf("create document.xml: %v", err)
	}
	xmlContent := `<?xml version="1.0" encoding="utf-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Hello World from Docx</w:t></w:r></w:p></w:body></w:document>`
	_, _ = w.Write([]byte(xmlContent))
	_ = zw.Close()

	// Write to temporary file
	tmpFile, err := os.CreateTemp("", "test_*.docx")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()
	_, _ = tmpFile.Write(buf.Bytes())

	txt, err := extractDocText(tmpFile.Name())
	if err != nil {
		t.Fatalf("extractDocText failed: %v", err)
	}
	if !strings.Contains(txt, "Hello World from Docx") {
		t.Errorf("expected 'Hello World from Docx' in %q", txt)
	}
}
