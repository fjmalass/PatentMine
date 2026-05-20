package engine

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/store"
	"patentmine/internal/store/sqlite"
)

// newTestEngine builds an engine over a fresh temp database, returning the
// store too so tests can seed documents directly.
func newTestEngine(t *testing.T, ingest IngestFactory) (*Engine, *sqlite.Repo) {
	t.Helper()
	repo, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "engine.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	eng := New(ctx, repo, ingest)
	t.Cleanup(func() {
		eng.Close()
		cancel()
		_ = repo.Close()
	})
	return eng, repo
}

func TestEngineProjectLifecycle(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	ctx := context.Background()

	project, err := eng.CreateProject(ctx, "Litigation A")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if project.ID == "" {
		t.Fatal("CreateProject returned empty id")
	}
	projects, err := eng.Projects(ctx)
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	if len(projects) != 1 || projects[0].Name != "Litigation A" {
		t.Fatalf("Projects = %v, want one named project", projects)
	}
	if _, err := eng.CreateProject(ctx, ""); err == nil {
		t.Fatal("CreateProject with empty name should fail")
	}
}

// TestEngineAddToProjectCreatesStubForUnknownPatent covers adding a patent that
// has never been fetched: the engine records it as a stub so the membership
// foreign key is satisfied.
func TestEngineAddToProjectCreatesStubForUnknownPatent(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	ctx := context.Background()

	project, err := eng.CreateProject(ctx, "P")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	number := domain.MustParsePatentNumber("AU607081B2")
	if _, err := eng.AddToProject(ctx, project.ID, number); err != nil {
		t.Fatalf("AddToProject for an unfetched patent: %v", err)
	}
	stub, err := eng.Patent(ctx, number)
	if err != nil {
		t.Fatalf("Patent after add: %v", err)
	}
	if !stub.IsStub() {
		t.Fatalf("added patent fetch state = %q, want stub", stub.FetchState)
	}
}

// TestEngineAddToProjectAutoFetchesNewStub checks that adding a never-seen
// patent enqueues a single-patent fetch (depth 0) so the record fills in.
func TestEngineAddToProjectAutoFetchesNewStub(t *testing.T) {
	gotDepth := make(chan int, 1)
	factory := func(_ domain.PatentNumber, depth int, _ domain.CrawlProfile, _ bool) Job {
		gotDepth <- depth
		return JobFunc(func(context.Context, JobID, func(proto.Event)) error { return nil })
	}
	eng, _ := newTestEngine(t, factory)
	ctx := context.Background()

	project, err := eng.CreateProject(ctx, "P")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := eng.AddToProject(ctx, project.ID, domain.MustParsePatentNumber("AU607081B2")); err != nil {
		t.Fatalf("AddToProject: %v", err)
	}
	select {
	case depth := <-gotDepth:
		if depth != 0 {
			t.Fatalf("auto-fetch depth = %d, want 0 (root only)", depth)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("AddToProject did not auto-fetch a newly added patent")
	}
}

type fakeImporter struct{ path string }

func (f *fakeImporter) ImportFile(_ context.Context, path string) error {
	f.path = path
	return nil
}

func TestEngineImportFileDelegatesToImporter(t *testing.T) {
	repo, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "import.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() { _ = repo.Close() }()

	fake := &fakeImporter{}
	eng := New(ctx, repo, nil, WithFileImporter(fake))
	defer eng.Close()

	if err := eng.ImportFile(ctx, "fixtures/US11611785B2.json"); err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	if fake.path != "fixtures/US11611785B2.json" {
		t.Fatalf("importer received path %q", fake.path)
	}
}

func TestEngineImportFileWithoutImporterFails(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	if err := eng.ImportFile(context.Background(), "x.json"); err == nil {
		t.Fatal("ImportFile without a configured importer should fail")
	}
}

func TestEngineEnforcesReviewStateTransitions(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	ctx := context.Background()

	patent := domain.Patent{
		Number:     domain.MustParsePatentNumber("US11611785B2"),
		FetchState: domain.FetchCached,
	}
	if err := eng.SavePatent(ctx, patent); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	project, err := eng.CreateProject(ctx, "P")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := eng.AddToProject(ctx, project.ID, patent.Number); err != nil {
		t.Fatalf("AddToProject: %v", err)
	}

	// stored -> deleted is allowed.
	if err := eng.SetReviewState(ctx, project.ID, patent.Number, domain.ReviewStateDeleted); err != nil {
		t.Fatalf("stored->deleted: %v", err)
	}
	// deleted -> ignored is rejected by the domain transition rules.
	if err := eng.SetReviewState(ctx, project.ID, patent.Number, domain.ReviewStateIgnored); err == nil {
		t.Fatal("deleted->ignored should be rejected")
	}
	// deleted -> stored (undelete) is allowed.
	if err := eng.SetReviewState(ctx, project.ID, patent.Number, domain.ReviewStateStored); err != nil {
		t.Fatalf("deleted->stored: %v", err)
	}
}

func TestEngineIngestEmitsProgressAndDone(t *testing.T) {
	factory := func(root domain.PatentNumber, _ int, _ domain.CrawlProfile, _ bool) Job {
		return JobFunc(func(_ context.Context, id JobID, emit func(proto.Event)) error {
			emit(proto.NewEvent(proto.EventIngestProgress, proto.IngestProgress{
				JobID: string(id), IngestedCount: 1, Message: "crawled " + root.String(),
			}))
			return nil
		})
	}
	eng, _ := newTestEngine(t, factory)

	events, unsub := eng.Subscribe()
	defer unsub()

	jobID, err := eng.StartFamilyIngest(domain.MustParsePatentNumber("US11611785B2"), 1, domain.CrawlProfileAll, false)
	if err != nil {
		t.Fatalf("StartFamilyIngest: %v", err)
	}

	var gotProgress, gotDone bool
	deadline := time.After(2 * time.Second)
	for !gotProgress || !gotDone {
		select {
		case ev := <-events:
			switch ev.Method {
			case proto.EventIngestProgress:
				gotProgress = true
			case proto.EventIngestDone:
				var done proto.IngestDone
				if err := json.Unmarshal(ev.Params, &done); err != nil {
					t.Fatalf("decode done: %v", err)
				}
				if done.JobID != string(jobID) {
					t.Fatalf("done job id = %q, want %q", done.JobID, jobID)
				}
				if done.Error != "" {
					t.Fatalf("job reported error: %s", done.Error)
				}
				gotDone = true
			}
		case <-deadline:
			t.Fatalf("timed out (progress=%v done=%v)", gotProgress, gotDone)
		}
	}
}

func TestEngineSetReviewStateKeepsUserStateAfterAutoFetchFailure(t *testing.T) {
	factory := func(_ domain.PatentNumber, _ int, _ domain.CrawlProfile, _ bool) Job {
		return JobFunc(func(_ context.Context, _ JobID, _ func(proto.Event)) error {
			return errors.New("not found")
		})
	}
	eng, repo := newTestEngine(t, factory)
	ctx := context.Background()

	project, err := eng.CreateProject(ctx, "P")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	patent := domain.MustParsePatentNumber("US9999999B2")
	if err := eng.SetReviewState(ctx, project.ID, patent, domain.ReviewStateUnderReview); err != nil {
		t.Fatalf("SetReviewState: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		membership, err := repo.Membership(ctx, project.ID, patent)
		if err == nil {
			if membership.ReviewState != domain.ReviewStateUnderReview {
				t.Fatalf("Membership state = %q, want under_review", membership.ReviewState)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Membership disappeared after auto-fetch failure: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	stored, err := repo.Patent(ctx, patent)
	if err != nil {
		t.Fatalf("Patent after failed auto-fetch: %v", err)
	}
	if stored.FetchState != domain.FetchStub {
		t.Fatalf("Patent fetch state = %q, want stub", stored.FetchState)
	}
}

func TestEngineIngestWithoutFactoryFails(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	if _, err := eng.StartFamilyIngest(domain.MustParsePatentNumber("US11611785B2"), 1, domain.CrawlProfileAll, false); err == nil {
		t.Fatal("StartFamilyIngest without a factory should fail")
	}
}

func TestEngineExportIDSUsesOnlyCuratedEntries(t *testing.T) {
	eng, repo := newTestEngine(t, nil)
	ctx := context.Background()

	project, err := eng.CreateProject(ctx, "Filing")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	numbers := []string{"US0000001B2", "US0000002B2", "US0000003B2"}
	for _, n := range numbers {
		number := domain.MustParsePatentNumber(n)
		if err := eng.SavePatent(ctx, domain.Patent{
			Number: number, Title: "Title " + n, FetchState: domain.FetchCached,
		}); err != nil {
			t.Fatalf("SavePatent %s: %v", n, err)
		}
		// A granted patent has a grant document — what the IDS discloses.
		if err := repo.SaveDocument(ctx, number, domain.Document{
			Number: number, Stage: domain.StageGrant,
		}); err != nil {
			t.Fatalf("SaveDocument %s: %v", n, err)
		}
		if _, err := eng.AddToProject(ctx, project.ID, number); err != nil {
			t.Fatalf("AddToProject %s: %v", n, err)
		}
	}
	for _, n := range []string{"US0000001B2", "US0000003B2"} {
		entry := domain.IDSEntry{Project: project.ID, Patent: domain.MustParsePatentNumber(n), Status: domain.IDSEntryPending}
		if _, err := eng.SaveIDSEntry(ctx, entry); err != nil {
			t.Fatalf("SaveIDSEntry %s: %v", n, err)
		}
	}

	ids, err := eng.ExportIDS(ctx, project.ID)
	if err != nil {
		t.Fatalf("ExportIDS: %v", err)
	}
	if ids.Status != domain.IDSDraft {
		t.Fatalf("IDS status = %s, want draft", ids.Status)
	}
	if len(ids.Entries) != 2 {
		t.Fatalf("IDS has %d entries, want 2 curated entries", len(ids.Entries))
	}
	for _, e := range ids.Entries {
		if e.Number.Serial == "0000002" {
			t.Fatal("uncurated patent must not appear on the IDS")
		}
	}
}

func TestEngineExportIDSUsesPublishedDocumentOnly(t *testing.T) {
	eng, repo := newTestEngine(t, nil)
	ctx := context.Background()
	project, err := eng.CreateProject(ctx, "Filing")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// A granted record, keyed by its application number, with both documents.
	granted := domain.MustParsePatentNumber("US16000020")
	grantDoc := domain.MustParsePatentNumber("US0000020B2")
	if err := eng.SavePatent(ctx, domain.Patent{
		Number: granted, Title: "Granted", FetchState: domain.FetchCached,
	}); err != nil {
		t.Fatalf("SavePatent granted: %v", err)
	}
	for _, d := range []domain.Document{
		{Number: granted, Stage: domain.StageApplication},
		{Number: grantDoc, Stage: domain.StageGrant},
	} {
		if err := repo.SaveDocument(ctx, granted, d); err != nil {
			t.Fatalf("SaveDocument: %v", err)
		}
	}
	if _, err := eng.AddToProject(ctx, project.ID, granted); err != nil {
		t.Fatalf("AddToProject granted: %v", err)
	}
	if _, err := eng.SaveIDSEntry(ctx, domain.IDSEntry{Project: project.ID, Patent: granted, Status: domain.IDSEntryPending}); err != nil {
		t.Fatalf("SaveIDSEntry granted: %v", err)
	}

	// A record with only an application — nothing publishable to disclose.
	pending := domain.MustParsePatentNumber("US16000021")
	if err := eng.SavePatent(ctx, domain.Patent{
		Number: pending, Title: "Pending", FetchState: domain.FetchCached,
	}); err != nil {
		t.Fatalf("SavePatent pending: %v", err)
	}
	if err := repo.SaveDocument(ctx, pending, domain.Document{
		Number: pending, Stage: domain.StageApplication,
	}); err != nil {
		t.Fatalf("SaveDocument pending: %v", err)
	}
	if _, err := eng.AddToProject(ctx, project.ID, pending); err != nil {
		t.Fatalf("AddToProject pending: %v", err)
	}
	if _, err := eng.SaveIDSEntry(ctx, domain.IDSEntry{Project: project.ID, Patent: pending, Status: domain.IDSEntryPending}); err != nil {
		t.Fatalf("SaveIDSEntry pending: %v", err)
	}

	ids, err := eng.ExportIDS(ctx, project.ID)
	if err != nil {
		t.Fatalf("ExportIDS: %v", err)
	}
	if len(ids.Entries) != 1 {
		t.Fatalf("IDS has %d entries, want 1 (the application-only record excluded)", len(ids.Entries))
	}
	if !ids.Entries[0].Number.Equal(grantDoc) {
		t.Fatalf("IDS entry = %s, want the grant %s — never the application",
			ids.Entries[0].Number, grantDoc)
	}
}

func TestEngineExportIDSUnknownProject(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	if _, err := eng.ExportIDS(context.Background(), "no-such-project"); err == nil {
		t.Fatal("ExportIDS for an unknown project should fail")
	}
}

func TestEngineResolvesDocumentNumberToRecord(t *testing.T) {
	eng, repo := newTestEngine(t, nil)
	ctx := context.Background()

	// A record keyed by its grant number, also carrying an application document.
	record := domain.MustParsePatentNumber("US0000030B2")
	application := domain.MustParsePatentNumber("US16000030")
	if err := eng.SavePatent(ctx, domain.Patent{Number: record, FetchState: domain.FetchCached}); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	for _, d := range []domain.Document{
		{Number: record, Stage: domain.StageGrant},
		{Number: application, Stage: domain.StageApplication},
	} {
		if err := repo.SaveDocument(ctx, record, d); err != nil {
			t.Fatalf("SaveDocument: %v", err)
		}
	}
	project, err := eng.CreateProject(ctx, "P")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Adding by the application number must resolve to the record.
	if _, err := eng.AddToProject(ctx, project.ID, application); err != nil {
		t.Fatalf("AddToProject by application number: %v", err)
	}
	members, err := repo.Memberships(ctx, project.ID)
	if err != nil {
		t.Fatalf("Memberships: %v", err)
	}
	if len(members) != 1 || !members[0].Patent.Equal(record) {
		t.Fatalf("membership = %v, want one pointing at record %s", members, record)
	}

	// Changing state by the application number resolves too.
	if err := eng.SetReviewState(ctx, project.ID, application, domain.ReviewStateUnderReview); err != nil {
		t.Fatalf("SetReviewState by application number: %v", err)
	}

	// Fetching by the application number returns the unified record.
	got, err := eng.Patent(ctx, application)
	if err != nil {
		t.Fatalf("Patent by application number: %v", err)
	}
	if !got.Number.Equal(record) {
		t.Fatalf("Patent.Number = %s, want the record %s", got.Number, record)
	}
}

func TestEngineRelationsRejectsInvalidKind(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	_, _, err := eng.Relations(context.Background(), store.PatentQuery{
		Relation:     domain.MustParsePatentNumber("US0000001B2"),
		RelationKind: domain.RelationKind("bogus"),
	})
	if err == nil {
		t.Fatal("Relations with an invalid kind should fail")
	}
}

func TestEngineReviewStateOf(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	ctx := context.Background()

	patent := domain.Patent{
		Number:     domain.MustParsePatentNumber("US11611785B2"),
		FetchState: domain.FetchCached,
	}
	if err := eng.SavePatent(ctx, patent); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	project, err := eng.CreateProject(ctx, "P")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// A patent that is not yet a member reports ok=false, not an error.
	if _, ok, err := eng.ReviewStateOf(ctx, project.ID, patent.Number); err != nil || ok {
		t.Fatalf("ReviewStateOf before add = (ok %v, err %v), want ok false", ok, err)
	}
	if _, err := eng.AddToProject(ctx, project.ID, patent.Number); err != nil {
		t.Fatalf("AddToProject: %v", err)
	}
	state, ok, err := eng.ReviewStateOf(ctx, project.ID, patent.Number)
	if err != nil || !ok || state != domain.ReviewStateStored {
		t.Fatalf("ReviewStateOf = (%q, %v, %v), want (stored, true, nil)", state, ok, err)
	}
	// With no project named the call is a quiet no-op.
	if _, ok, err := eng.ReviewStateOf(ctx, "", patent.Number); err != nil || ok {
		t.Fatalf("ReviewStateOf with no project = (ok %v, err %v), want ok false", ok, err)
	}
}

func TestEngineTagging(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	ctx := context.Background()

	patent := domain.Patent{
		Number:     domain.MustParsePatentNumber("US11611785B2"),
		FetchState: domain.FetchCached,
	}
	if err := eng.SavePatent(ctx, patent); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	project, err := eng.CreateProject(ctx, "P")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// AssignTag creates the named tag and links it to the patent.
	if err := eng.AssignTag(ctx, project.ID, patent.Number, "key-reference"); err != nil {
		t.Fatalf("AssignTag: %v", err)
	}
	tags, err := eng.PatentTags(ctx, project.ID, patent.Number)
	if err != nil {
		t.Fatalf("PatentTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "key-reference" {
		t.Fatalf("PatentTags = %v, want one key-reference tag", tags)
	}

	// RemoveTag matches the name case-insensitively.
	if err := eng.RemoveTag(ctx, project.ID, patent.Number, "KEY-REFERENCE"); err != nil {
		t.Fatalf("RemoveTag: %v", err)
	}
	tags, err = eng.PatentTags(ctx, project.ID, patent.Number)
	if err != nil {
		t.Fatalf("PatentTags after remove: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("PatentTags = %v, want empty after remove", tags)
	}

	// Removing a tag the patent does not carry is an error.
	if err := eng.RemoveTag(ctx, project.ID, patent.Number, "missing"); err == nil {
		t.Fatal("RemoveTag of an unassigned tag should fail")
	}
	// A blank name is rejected on assign.
	if err := eng.AssignTag(ctx, project.ID, patent.Number, "  "); err == nil {
		t.Fatal("AssignTag with a blank name should fail")
	}
}
