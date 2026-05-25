package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/store"
	"patentmine/internal/store/sqlite"
)

// newTestEngine builds an engine over a fresh temp database, returning the
// store too so tests can seed documents directly.
func newTestEngine(t *testing.T, crawl CrawlFactory) (*Engine, *sqlite.Repo) {
	t.Helper()
	repo, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "engine.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	eng := New(ctx, repo, crawl)
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
	ctx := context.Background()

	t.Run("stub patent transition rules", func(t *testing.T) {
		eng, _ := newTestEngine(t, nil)
		patent := domain.Patent{
			Number:     domain.MustParsePatentNumber("US11611785B2"),
			FetchState: domain.FetchStub,
		}
		if err := eng.SavePatent(ctx, patent); err != nil {
			t.Fatalf("SavePatent: %v", err)
		}
		project, err := eng.CreateProject(ctx, "P1")
		if err != nil {
			t.Fatalf("CreateProject: %v", err)
		}
		if _, err := eng.AddToProject(ctx, project.ID, patent.Number); err != nil {
			t.Fatalf("AddToProject: %v", err)
		}

		// Transition to active should be rejected for a stub patent.
		if err := eng.SetReviewState(ctx, project.ID, []domain.PatentNumber{patent.Number}, domain.ReviewStateActive); err == nil {
			t.Fatal("stub patent -> active should be rejected")
		}

		// Transition to deleted should be rejected for a stub patent.
		if err := eng.SetReviewState(ctx, project.ID, []domain.PatentNumber{patent.Number}, domain.ReviewStateDeleted); err == nil {
			t.Fatal("stub patent -> deleted should be rejected")
		}
	})

	t.Run("cached patent transition rules", func(t *testing.T) {
		eng, repo := newTestEngine(t, nil)
		patent := domain.Patent{
			Number:     domain.MustParsePatentNumber("US11611785B2"),
			FetchState: domain.FetchCached,
		}
		if err := eng.SavePatent(ctx, patent); err != nil {
			t.Fatalf("SavePatent: %v", err)
		}
		if err := repo.SaveDocument(ctx, patent.Number, domain.Document{
			Number: patent.Number,
			Stage:  domain.GuessStage(patent.Number),
		}); err != nil {
			t.Fatalf("SaveDocument: %v", err)
		}
		project, err := eng.CreateProject(ctx, "P2")
		if err != nil {
			t.Fatalf("CreateProject: %v", err)
		}
		if _, err := eng.AddToProject(ctx, project.ID, patent.Number); err != nil {
			t.Fatalf("AddToProject: %v", err)
		}

		// cached patent -> deleted is allowed.
		if err := eng.SetReviewState(ctx, project.ID, []domain.PatentNumber{patent.Number}, domain.ReviewStateDeleted); err != nil {
			t.Fatalf("cached patent -> deleted: %v", err)
		}

		// cached patent deleted -> ignored is allowed (transitions to/from any review state).
		if err := eng.SetReviewState(ctx, project.ID, []domain.PatentNumber{patent.Number}, domain.ReviewStateIgnored); err != nil {
			t.Fatalf("cached patent deleted -> ignored: %v", err)
		}

		// cached patent ignored -> active is allowed.
		if err := eng.SetReviewState(ctx, project.ID, []domain.PatentNumber{patent.Number}, domain.ReviewStateActive); err != nil {
			t.Fatalf("cached patent ignored -> active: %v", err)
		}
	})
}

func TestEngineCrawlEmitsProgressAndDone(t *testing.T) {
	factory := func(root domain.PatentNumber, _ int, _ domain.CrawlProfile, _ bool) Job {
		return JobFunc(func(_ context.Context, id JobID, emit func(proto.Event)) error {
			emit(proto.NewEvent(proto.EventCrawlProgress, proto.CrawlProgress{
				JobID: string(id), CrawledCount: 1, Message: "crawled " + root.String(),
			}))
			return nil
		})
	}
	eng, _ := newTestEngine(t, factory)

	events, unsub := eng.Subscribe()
	defer unsub()

	jobID, err := eng.StartFamilyCrawl(context.Background(), domain.MustParsePatentNumber("US11611785B2"), 1, domain.CrawlProfileAll, false)
	if err != nil {
		t.Fatalf("StartFamilyCrawl: %v", err)
	}

	var gotProgress, gotDone bool
	deadline := time.After(2 * time.Second)
	for !gotProgress || !gotDone {
		select {
		case ev := <-events:
			switch ev.Method {
			case proto.EventCrawlProgress:
				gotProgress = true
			case proto.EventCrawlDone:
				var done proto.CrawlDone
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
	eng, _ := newTestEngine(t, nil)
	ctx := context.Background()

	project, err := eng.CreateProject(ctx, "P")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	patent := domain.MustParsePatentNumber("US9999999B2")
	if err := eng.SetReviewState(ctx, project.ID, []domain.PatentNumber{patent}, domain.ReviewStateUnderReview); err == nil {
		t.Fatal("expected SetReviewState to fail for a stub patent")
	}
}

func TestEngineCrawlWithoutFactoryFails(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	if _, err := eng.StartFamilyCrawl(context.Background(), domain.MustParsePatentNumber("US11611785B2"), 1, domain.CrawlProfileAll, false); err == nil {
		t.Fatal("StartFamilyCrawl without a factory should fail")
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
	if err := eng.SetReviewState(ctx, project.ID, []domain.PatentNumber{application}, domain.ReviewStateUnderReview); err != nil {
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
	if err != nil || !ok || state != domain.ReviewStateUnknown {
		t.Fatalf("ReviewStateOf = (%q, %v, %v), want (stub, true, nil)", state, ok, err)
	}
	// With no project named the call is a quiet no-op.
	if _, ok, err := eng.ReviewStateOf(ctx, "", patent.Number); err != nil || ok {
		t.Fatalf("ReviewStateOf with no project = (ok %v, err %v), want ok false", ok, err)
	}
}

func TestEngineSinglePatentNoopsRecordMetricsWithoutChangeEvents(t *testing.T) {
	repo, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "metrics.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	metrics := observability.NewMetrics()
	ctx, cancel := context.WithCancel(context.Background())
	eng := New(ctx, repo, nil, WithMetrics(metrics))
	t.Cleanup(func() {
		eng.Close()
		cancel()
		_ = repo.Close()
	})

	patent := domain.Patent{Number: domain.MustParsePatentNumber("US11611785B2"), FetchState: domain.FetchCached}
	if err := eng.SavePatent(ctx, patent); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	if err := repo.SaveDocument(ctx, patent.Number, domain.Document{
		Number: patent.Number,
		Stage:  domain.GuessStage(patent.Number),
	}); err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}
	project, err := eng.CreateProject(ctx, "P")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if _, err := eng.AddToProject(ctx, project.ID, patent.Number); err != nil {
		t.Fatalf("AddToProject: %v", err)
	}
	if _, err := eng.CreateTaxonomyTag(ctx, project.ID, "prior_art"); err != nil {
		t.Fatalf("CreateTaxonomyTag: %v", err)
	}

	events, unsub := eng.Subscribe()
	defer unsub()

	if err := eng.TagPatentStrict(ctx, project.ID, []domain.PatentNumber{patent.Number}, "prior_art"); err != nil {
		t.Fatalf("TagPatentStrict: %v", err)
	}
	expectDBChanged(t, events, "tag assignment")
	if err := eng.TagPatentStrict(ctx, project.ID, []domain.PatentNumber{patent.Number}, "PRIOR_ART"); err != nil {
		t.Fatalf("TagPatentStrict repeat: %v", err)
	}
	expectNoDBChanged(t, events, "repeat tag assignment")
	if err := eng.UntagPatentStrict(ctx, project.ID, []domain.PatentNumber{patent.Number}, "PRIOR_ART"); err != nil {
		t.Fatalf("UntagPatentStrict: %v", err)
	}
	expectDBChanged(t, events, "tag removal")

	if err := eng.SetReviewState(ctx, project.ID, []domain.PatentNumber{patent.Number}, domain.ReviewStateUnderReview); err != nil {
		t.Fatalf("SetReviewState: %v", err)
	}
	expectDBChanged(t, events, "review state change")
	if err := eng.SetReviewState(ctx, project.ID, []domain.PatentNumber{patent.Number}, domain.ReviewStateUnderReview); err != nil {
		t.Fatalf("SetReviewState repeat: %v", err)
	}
	expectNoDBChanged(t, events, "repeat review state")

	snap := metrics.Snapshot()
	wantCounters := map[string]int64{
		"engine.patent_tag.assign.changed_total": 1,
		"engine.patent_tag.assign.noop_total":    1,
		"engine.patent_tag.remove.changed_total": 1,
		"engine.review_state.changed_total":      1,
		"engine.review_state.noop_total":         1,
	}
	for name, want := range wantCounters {
		if got := snap.Counters[name]; got != want {
			t.Fatalf("counter %s = %d, want %d", name, got, want)
		}
	}
	if got := snap.Timings["engine.tag_patent_strict"].Count; got != 2 {
		t.Fatalf("assign tag timing count = %d, want 2", got)
	}
	if got := snap.Timings["engine.set_review_state"].Count; got != 2 {
		t.Fatalf("review state timing count = %d, want 2", got)
	}
}

func expectDBChanged(t *testing.T, events <-chan proto.Event, label string) {
	t.Helper()
	select {
	case ev := <-events:
		if ev.Method != proto.EventDBChanged {
			t.Fatalf("%s emitted %s, want db.changed", label, ev.Method)
		}
	case <-time.After(time.Second):
		t.Fatalf("%s did not emit db.changed", label)
	}
}

func expectNoDBChanged(t *testing.T, events <-chan proto.Event, label string) {
	t.Helper()
	select {
	case ev := <-events:
		t.Fatalf("%s emitted unexpected event %s", label, ev.Method)
	case <-time.After(50 * time.Millisecond):
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

	// TagPatent creates the named tag and links it to the patent.
	if err := eng.TagPatent(ctx, project.ID, []domain.PatentNumber{patent.Number}, "key_reference"); err != nil {
		t.Fatalf("TagPatent: %v", err)
	}
	tags, err := eng.PatentTags(ctx, project.ID, patent.Number)
	if err != nil {
		t.Fatalf("PatentTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "key_reference" {
		t.Fatalf("PatentTags = %v, want one key_reference tag", tags)
	}

	// UntagPatent matches the name case-insensitively.
	if err := eng.UntagPatent(ctx, project.ID, []domain.PatentNumber{patent.Number}, "KEY_REFERENCE"); err != nil {
		t.Fatalf("UntagPatent: %v", err)
	}
	tags, err = eng.PatentTags(ctx, project.ID, patent.Number)
	if err != nil {
		t.Fatalf("PatentTags after remove: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("PatentTags = %v, want empty after remove", tags)
	}

	// Removing a tag the patent does not carry is an error.
	if err := eng.UntagPatent(ctx, project.ID, []domain.PatentNumber{patent.Number}, "missing"); err == nil {
		t.Fatal("UntagPatent of an unassigned tag should fail")
	}
	// A blank name is rejected on assign.
	if err := eng.TagPatent(ctx, project.ID, []domain.PatentNumber{patent.Number}, "  "); err == nil {
		t.Fatal("TagPatent with a blank name should fail")
	}
}

func TestDeleteBackupAndReplay(t *testing.T) {
	// 1. Initialize activity recording runtime in temp dir
	logsDir := t.TempDir()
	runtime, err := observability.Open(logsDir, "engine-test", "1.0.0")
	if err != nil {
		t.Fatalf("observability.Open: %v", err)
	}
	defer runtime.Close()

	ctx := context.Background()

	// 2. Initialize engine and SQLite repository
	repo, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "engine.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer repo.Close()

	eng := New(ctx, repo, nil, WithActivityRecorder(runtime.Activity))
	defer eng.Close()

	// 3. Populate dummy patents
	p1 := domain.MustParsePatentNumber("US11611785B2")
	p2 := domain.MustParsePatentNumber("US0000001A1")
	p3 := domain.MustParsePatentNumber("US0000002A1")

	// Save patents
	patent1 := domain.Patent{
		Number:        p1,
		DisplayNumber: p1,
		Title:         "System for Patent Mining",
		Abstract:      "An intelligent system for patent ingestion and graph crawling.",
		Assignee:      "PatentMine Inc.",
		Inventors:     []domain.Inventor{"John Doe", "Jane Smith"},
		FetchState:    domain.FetchCached,
		FirstClaim:    "Claim 1: A system comprising...",
	}
	if err := eng.SavePatent(ctx, patent1); err != nil {
		t.Fatalf("SavePatent p1: %v", err)
	}
	if err := eng.SavePatent(ctx, domain.Patent{Number: p2, DisplayNumber: p2, FetchState: domain.FetchCached}); err != nil {
		t.Fatalf("SavePatent p2: %v", err)
	}
	if err := eng.SavePatent(ctx, domain.Patent{Number: p3, DisplayNumber: p3, FetchState: domain.FetchCached}); err != nil {
		t.Fatalf("SavePatent p3: %v", err)
	}

	// 4. Save documents for p1
	doc1 := domain.Document{Number: p1, Stage: domain.StageGrant}
	doc2 := domain.Document{Number: domain.MustParsePatentNumber("US11611785"), Stage: domain.StageApplication}
	if err := repo.SaveDocument(ctx, p1, doc1); err != nil {
		t.Fatalf("SaveDocument doc1: %v", err)
	}
	if err := repo.SaveDocument(ctx, p1, doc2); err != nil {
		t.Fatalf("SaveDocument doc2: %v", err)
	}

	// 5. Create bidirectional/directed relations involving p1
	rel1 := domain.Relation{From: p1, To: p2, Kind: domain.RelationCites}
	rel2 := domain.Relation{From: p3, To: p1, Kind: domain.RelationCites}
	if err := repo.SaveRelation(ctx, rel1); err != nil {
		t.Fatalf("SaveRelation rel1: %v", err)
	}
	if err := repo.SaveRelation(ctx, rel2); err != nil {
		t.Fatalf("SaveRelation rel2: %v", err)
	}

	// Verify before deleting
	storedPatent, err := eng.Patent(ctx, p1)
	if err != nil {
		t.Fatalf("Patent before delete: %v", err)
	}
	if len(storedPatent.Documents) != 2 {
		t.Fatalf("Expected 2 documents, got %d", len(storedPatent.Documents))
	}

	storedRels, err := repo.AllRelations(ctx, p1)
	if err != nil {
		t.Fatalf("AllRelations: %v", err)
	}
	if len(storedRels) != 2 {
		t.Fatalf("Expected 2 relations, got %d", len(storedRels))
	}

	// 6. Delete P1
	if err := eng.DeletePatent(ctx, p1); err != nil {
		t.Fatalf("DeletePatent: %v", err)
	}

	// Verify completely deleted
	_, err = eng.Patent(ctx, p1)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Expected store.ErrNotFound, got %v", err)
	}
	delDocs, err := repo.Documents(ctx, p1)
	if err != nil {
		t.Fatalf("repo.Documents: %v", err)
	}
	if len(delDocs) != 0 {
		t.Fatalf("Expected documents to be deleted, got %d", len(delDocs))
	}
	delRels, err := repo.AllRelations(ctx, p1)
	if err != nil {
		t.Fatalf("repo.AllRelations: %v", err)
	}
	if len(delRels) != 0 {
		t.Fatalf("Expected relations to be deleted, got %d", len(delRels))
	}

	// 7. Flush activity records and find the delete event
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close runtime: %v", err)
	}

	files, err := os.ReadDir(logsDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var activityPath string
	for _, f := range files {
		if filepath.HasPrefix(f.Name(), "activity-") {
			activityPath = filepath.Join(logsDir, f.Name())
			break
		}
	}
	if activityPath == "" {
		t.Fatal("No activity log file found")
	}

	content, err := os.ReadFile(activityPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Find the delete action
	var deleteRecord struct {
		Action string         `json:"action"`
		Before PatentSnapshot `json:"before"`
	}

	lines := bytes.Split(content, []byte("\n"))
	found := false
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		var temp struct {
			Action string `json:"action"`
		}
		if err := json.Unmarshal(line, &temp); err == nil && temp.Action == "patent.delete" {
			if err := json.Unmarshal(line, &deleteRecord); err != nil {
				t.Fatalf("Unmarshal delete action: %v", err)
			}
			found = true
			break
		}
	}

	if !found {
		t.Fatal("Could not find patent.delete in activity journal")
	}

	snapshot := deleteRecord.Before
	if snapshot.Patent.Number != p1 {
		t.Fatalf("Snapshot patent number = %v, want %v", snapshot.Patent.Number, p1)
	}
	if len(snapshot.Patent.Documents) != 2 {
		t.Fatalf("Snapshot documents = %d, want 2", len(snapshot.Patent.Documents))
	}
	if len(snapshot.Relations) != 2 {
		t.Fatalf("Snapshot relations = %d, want 2", len(snapshot.Relations))
	}

	// 8. Replay/Full Restore
	if err := eng.RestorePatent(ctx, snapshot, false); err != nil {
		t.Fatalf("RestorePatent hard: %v", err)
	}

	// Verify full restore
	restoredPatent, err := eng.Patent(ctx, p1)
	if err != nil {
		t.Fatalf("Patent after hard restore: %v", err)
	}
	if restoredPatent.Title != patent1.Title || restoredPatent.Abstract != patent1.Abstract {
		t.Fatalf("Restored title = %q, want %q", restoredPatent.Title, patent1.Title)
	}
	if len(restoredPatent.Documents) != 2 {
		t.Fatalf("Restored documents count = %d, want 2", len(restoredPatent.Documents))
	}
	restoredRels, err := repo.AllRelations(ctx, p1)
	if err != nil {
		t.Fatalf("AllRelations after hard restore: %v", err)
	}
	if len(restoredRels) != 2 {
		t.Fatalf("Restored relations count = %d, want 2", len(restoredRels))
	}

	// 9. Hard delete again
	if err := eng.DeletePatent(ctx, p1); err != nil {
		t.Fatalf("DeletePatent again: %v", err)
	}

	// 10. Replay/Soft Restore (stub recovery)
	if err := eng.RestorePatent(ctx, snapshot, true); err != nil {
		t.Fatalf("RestorePatent soft: %v", err)
	}

	// Verify soft restore (FetchState should be FetchStub, no rich metadata, single guess-stage document)
	stubPatent, err := eng.Patent(ctx, p1)
	if err != nil {
		t.Fatalf("Patent after soft restore: %v", err)
	}
	if stubPatent.FetchState != domain.FetchStub {
		t.Fatalf("FetchState = %s, want %s", stubPatent.FetchState, domain.FetchStub)
	}
	if stubPatent.Title != "" || stubPatent.Abstract != "" {
		t.Fatalf("Metadata was not stripped: title=%q abstract=%q", stubPatent.Title, stubPatent.Abstract)
	}
	if len(stubPatent.Documents) != 1 {
		t.Fatalf("Stub documents count = %d, want 1", len(stubPatent.Documents))
	}
	if stubPatent.Documents[0].Stage != domain.GuessStage(p1) {
		t.Fatalf("Stub document stage = %s, want guess-stage", stubPatent.Documents[0].Stage)
	}

	// Relations should STILL be fully restored!
	softRels, err := repo.AllRelations(ctx, p1)
	if err != nil {
		t.Fatalf("AllRelations after soft restore: %v", err)
	}
	if len(softRels) != 2 {
		t.Fatalf("Soft restored relations count = %d, want 2", len(softRels))
	}
}

func TestEngineBatchOperations(t *testing.T) {
	eng, repo := newTestEngine(t, nil)
	ctx := context.Background()

	p1 := domain.Patent{Number: domain.MustParsePatentNumber("US11611785B2"), FetchState: domain.FetchCached}
	p2 := domain.Patent{Number: domain.MustParsePatentNumber("US0000001B2"), FetchState: domain.FetchCached}
	p3 := domain.Patent{Number: domain.MustParsePatentNumber("US0000002B2"), FetchState: domain.FetchCached}

	for _, p := range []domain.Patent{p1, p2, p3} {
		if err := eng.SavePatent(ctx, p); err != nil {
			t.Fatalf("SavePatent: %v", err)
		}
		if err := repo.SaveDocument(ctx, p.Number, domain.Document{
			Number: p.Number,
			Stage:  domain.GuessStage(p.Number),
		}); err != nil {
			t.Fatalf("SaveDocument: %v", err)
		}
	}

	project, err := eng.CreateProject(ctx, "P")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// 1. Test SetReviewState.
	err = eng.SetReviewState(ctx, project.ID, []domain.PatentNumber{p1.Number, p2.Number}, domain.ReviewStateUnderReview)
	if err != nil {
		t.Fatalf("SetReviewState: %v", err)
	}

	// Verify states are updated.
	for _, num := range []domain.PatentNumber{p1.Number, p2.Number} {
		state, _, err := eng.ReviewStateOf(ctx, project.ID, num)
		if err != nil {
			t.Fatalf("ReviewStateOf: %v", err)
		}
		if state != domain.ReviewStateUnderReview {
			t.Fatalf("expected state %s, got %s", domain.ReviewStateUnderReview, state)
		}
	}

	// Transition p1 to Deleted (allowed).
	err = eng.SetReviewState(ctx, project.ID, []domain.PatentNumber{p1.Number}, domain.ReviewStateDeleted)
	if err != nil {
		t.Fatalf("failed to transition to Deleted: %v", err)
	}

	// Transition from Deleted to Ignored is allowed under the new transition rules.
	err = eng.SetReviewState(ctx, project.ID, []domain.PatentNumber{p1.Number}, domain.ReviewStateIgnored)
	if err != nil {
		t.Fatalf("failed to transition from Deleted to Ignored: %v", err)
	}

	// 2. Test TagPatent.
	err = eng.TagPatent(ctx, project.ID, []domain.PatentNumber{p1.Number, p2.Number}, "batch_tag")
	if err != nil {
		t.Fatalf("TagPatent: %v", err)
	}

	// Verify they are tagged.
	for _, num := range []domain.PatentNumber{p1.Number, p2.Number} {
		tags, err := eng.PatentTags(ctx, project.ID, num)
		if err != nil {
			t.Fatalf("PatentTags: %v", err)
		}
		if len(tags) != 1 || tags[0].Name != "batch_tag" {
			t.Fatalf("expected tag 'batch_tag', got %v", tags)
		}
	}

	// 3. Test UntagPatent.
	err = eng.UntagPatent(ctx, project.ID, []domain.PatentNumber{p1.Number, p2.Number}, "batch_tag")
	if err != nil {
		t.Fatalf("UntagPatent: %v", err)
	}

	// Verify they are untagged.
	for _, num := range []domain.PatentNumber{p1.Number, p2.Number} {
		tags, err := eng.PatentTags(ctx, project.ID, num)
		if err != nil {
			t.Fatalf("PatentTags: %v", err)
		}
		if len(tags) != 0 {
			t.Fatalf("expected 0 tags after untagging, got %v", tags)
		}
	}
}

func TestFamilyGraphAppliesBFSCountryFilter(t *testing.T) {
	eng, repo := newTestEngine(t, nil)
	ctx := context.Background()

	root := domain.MustParsePatentNumber("US1000001A1")
	parent := domain.MustParsePatentNumber("EP2000002A1")
	child := domain.MustParsePatentNumber("JP3000003A1")

	for _, patent := range []domain.Patent{
		{Number: root, DisplayNumber: root, Title: "Root", FetchState: domain.FetchCached},
		{Number: parent, DisplayNumber: parent, Title: "Parent", FetchState: domain.FetchCached},
		{Number: child, DisplayNumber: child, Title: "Child", FetchState: domain.FetchCached},
	} {
		if err := eng.SavePatent(ctx, patent); err != nil {
			t.Fatalf("SavePatent(%s): %v", patent.Number, err)
		}
	}

	for _, rel := range []domain.Relation{
		{From: root, To: parent, Kind: domain.RelationParent},
		{From: parent, To: root, Kind: domain.RelationChild},
		{From: root, To: child, Kind: domain.RelationChild},
		{From: child, To: root, Kind: domain.RelationParent},
	} {
		if err := repo.SaveRelation(ctx, rel); err != nil {
			t.Fatalf("SaveRelation(%+v): %v", rel, err)
		}
	}

	graph, err := eng.FamilyGraph(ctx, proto.FamilyGraphParams{
		Root:      root,
		Depth:     1,
		Countries: []string{"US", "EP"},
	})
	if err != nil {
		t.Fatalf("FamilyGraph: %v", err)
	}
	if len(graph.Nodes) != 2 {
		t.Fatalf("len(graph.Nodes) = %d, want 2", len(graph.Nodes))
	}
	if len(graph.Edges) != 1 {
		t.Fatalf("len(graph.Edges) = %d, want 1 visible edge", len(graph.Edges))
	}
	if graph.HiddenByCountry != 1 {
		t.Fatalf("HiddenByCountry = %d, want 1", graph.HiddenByCountry)
	}
	if graph.Nodes[0].Patent.Number != root || graph.Nodes[0].Depth != 0 {
		t.Fatalf("root node = %+v, want root depth 0", graph.Nodes[0])
	}
	if len(graph.Nodes[0].Parents) != 1 || graph.Nodes[0].Parents[0] != parent {
		t.Fatalf("root parents = %v, want [%s]", graph.Nodes[0].Parents, parent)
	}
	if len(graph.Nodes[0].Children) != 0 {
		t.Fatalf("root children = %v, want filtered child omitted", graph.Nodes[0].Children)
	}
	if graph.Edges[0].Parent != parent || graph.Edges[0].Child != root {
		t.Fatalf("edge = %+v, want %s -> %s", graph.Edges[0], parent, root)
	}
}

func TestFamilyGraphReportsInconsistentRelations(t *testing.T) {
	metrics := observability.NewMetrics()
	repo, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "family-graph.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	eng := New(ctx, repo, nil, WithMetrics(metrics))
	t.Cleanup(func() {
		eng.Close()
		cancel()
		_ = repo.Close()
	})

	root := domain.MustParsePatentNumber("US4000004A1")
	parent := domain.MustParsePatentNumber("US5000005A1")
	for _, patent := range []domain.Patent{
		{Number: root, DisplayNumber: root, FetchState: domain.FetchCached},
		{Number: parent, DisplayNumber: parent, FetchState: domain.FetchCached},
	} {
		if err := eng.SavePatent(context.Background(), patent); err != nil {
			t.Fatalf("SavePatent(%s): %v", patent.Number, err)
		}
	}
	if err := repo.SaveRelation(context.Background(), domain.Relation{From: root, To: parent, Kind: domain.RelationParent}); err != nil {
		t.Fatalf("SaveRelation: %v", err)
	}

	graph, err := eng.FamilyGraph(context.Background(), proto.FamilyGraphParams{Root: root, Depth: 1})
	if err != nil {
		t.Fatalf("FamilyGraph: %v", err)
	}
	if graph.InconsistencyCount != 1 {
		t.Fatalf("InconsistencyCount = %d, want 1", graph.InconsistencyCount)
	}
	snap := metrics.Snapshot()
	if snap.Counters["engine.family_graph.inconsistent_edges_total"] != 1 {
		t.Fatalf("inconsistent edge counter = %d, want 1", snap.Counters["engine.family_graph.inconsistent_edges_total"])
	}
}

func TestEngineDeletePatentsWritesReplayableSnapshots(t *testing.T) {
	logsDir := filepath.Join(t.TempDir(), "logs")
	runtime, err := observability.Open(logsDir, "test", "test-version")
	if err != nil {
		t.Fatalf("Open runtime: %v", err)
	}
	defer func() { _ = runtime.Close() }()

	repo, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "engine.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = repo.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	eng := New(ctx, repo, nil, WithActivityRecorder(runtime.Activity), WithLogger(runtime.Logger))
	defer eng.Close()

	p1 := domain.Patent{Number: domain.MustParsePatentNumber("US11611785B2"), Title: "P1", FetchState: domain.FetchCached}
	p2 := domain.Patent{Number: domain.MustParsePatentNumber("US0000001B2"), Title: "P2", FetchState: domain.FetchCached}
	for _, p := range []domain.Patent{p1, p2} {
		if err := eng.SavePatent(ctx, p); err != nil {
			t.Fatalf("SavePatent: %v", err)
		}
		doc := domain.Document{Number: p.Number, Stage: domain.StageGrant}
		if err := repo.SaveDocument(ctx, p.Number, doc); err != nil {
			t.Fatalf("SaveDocument: %v", err)
		}
	}
	if err := repo.SaveRelation(ctx, domain.Relation{From: p1.Number, To: p2.Number, Kind: domain.RelationCites}); err != nil {
		t.Fatalf("SaveRelation p1->p2: %v", err)
	}
	if err := repo.SaveRelation(ctx, domain.Relation{From: p2.Number, To: p1.Number, Kind: domain.RelationCitedBy}); err != nil {
		t.Fatalf("SaveRelation p2->p1: %v", err)
	}

	if err := eng.DeletePatents(ctx, []domain.PatentNumber{p1.Number, p2.Number, p1.Number}); err != nil {
		t.Fatalf("DeletePatents: %v", err)
	}

	for _, number := range []domain.PatentNumber{p1.Number, p2.Number} {
		_, err := eng.Patent(ctx, number)
		if !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("Patent %s after delete error = %v, want store.ErrNotFound", number, err)
		}
	}

	if err := runtime.Close(); err != nil {
		t.Fatalf("Close runtime: %v", err)
	}

	files, err := os.ReadDir(logsDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var activityPath string
	for _, f := range files {
		if filepath.HasPrefix(f.Name(), "activity-") {
			activityPath = filepath.Join(logsDir, f.Name())
			break
		}
	}
	if activityPath == "" {
		t.Fatal("No activity log file found")
	}

	content, err := os.ReadFile(activityPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	type deleteRecord struct {
		Action   string         `json:"action"`
		EntityID string         `json:"entity_id"`
		Before   PatentSnapshot `json:"before"`
		Metadata map[string]any `json:"metadata"`
	}
	records := map[string]deleteRecord{}
	for _, line := range bytes.Split(content, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var rec deleteRecord
		if err := json.Unmarshal(line, &rec); err != nil || rec.Action != "patent.delete" {
			continue
		}
		records[rec.EntityID] = rec
	}

	if len(records) != 2 {
		t.Fatalf("delete record count = %d, want 2", len(records))
	}
	for _, number := range []domain.PatentNumber{p1.Number, p2.Number} {
		rec, ok := records[number.String()]
		if !ok {
			t.Fatalf("missing delete record for %s", number)
		}
		if rec.Before.Patent.Number != number {
			t.Fatalf("snapshot patent number = %v, want %v", rec.Before.Patent.Number, number)
		}
		if _, ok := rec.Metadata["batch_id"]; !ok {
			t.Fatalf("delete record for %s missing batch_id metadata", number)
		}
		if got := int(rec.Metadata["batch_size"].(float64)); got != 2 {
			t.Fatalf("batch_size = %d, want 2", got)
		}
	}

	for _, number := range []domain.PatentNumber{p1.Number, p2.Number} {
		stub := domain.Patent{Number: number, DisplayNumber: number, FetchState: domain.FetchStub}
		if err := repo.SavePatent(ctx, stub); err != nil {
			t.Fatalf("SavePatent stub %s: %v", number, err)
		}
	}
	for _, number := range []domain.PatentNumber{p1.Number, p2.Number} {
		if err := eng.RestorePatent(ctx, records[number.String()].Before, false); err != nil {
			t.Fatalf("RestorePatent %s: %v", number, err)
		}
	}

	for _, number := range []domain.PatentNumber{p1.Number, p2.Number} {
		if _, err := eng.Patent(ctx, number); err != nil {
			t.Fatalf("Patent after restore %s: %v", number, err)
		}
	}
}

func TestEngineSetReviewStateSingleBroadcast(t *testing.T) {
	eng, repo := newTestEngine(t, nil)
	ctx := context.Background()

	patent := domain.MustParsePatentNumber("US8888888B2")
	p := domain.Patent{Number: patent, FetchState: domain.FetchCached}
	if err := eng.SavePatent(ctx, p); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	if err := repo.SaveDocument(ctx, patent, domain.Document{
		Number: patent,
		Stage:  domain.GuessStage(patent),
	}); err != nil {
		t.Fatalf("SaveDocument: %v", err)
	}

	project, err := eng.CreateProject(ctx, "SingleBroadcastProj")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	events, unsub := eng.Subscribe()
	defer unsub()

	// SetReviewState on a patent that is not a member yet.
	if err := eng.SetReviewState(ctx, project.ID, []domain.PatentNumber{patent}, domain.ReviewStateUnderReview); err != nil {
		t.Fatalf("SetReviewState: %v", err)
	}

	// We expect exactly ONE EventDBChanged broadcast.
	// Since e.announceChange is only called once at the end, it should trigger exactly one leading-edge publish.
	var changeEventsCount int
	timeout := time.After(400 * time.Millisecond) // Wait longer than the 200ms debounce window
loop:
	for {
		select {
		case ev := <-events:
			if ev.Method == proto.EventDBChanged {
				changeEventsCount++
			}
		case <-timeout:
			break loop
		}
	}

	if changeEventsCount != 1 {
		t.Errorf("expected exactly 1 change event, got %d", changeEventsCount)
	}
}

func TestPatentNoteActivityLogging(t *testing.T) {
	ctx := context.Background()
	logsDir := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	repo, err := sqlite.Open(ctx, dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer repo.Close()

	runtime, err := observability.Open(logsDir, "test-engine", "1.0.0")
	if err != nil {
		t.Fatalf("observability.Open: %v", err)
	}
	defer runtime.Close()

	eng := New(ctx, repo, nil, WithActivityRecorder(runtime.Activity))

	project, err := eng.CreateProject(ctx, "Test Note Activity Proj")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	patentNum, err := domain.ParsePatentNumber("US10000000B2")
	if err != nil {
		t.Fatalf("ParsePatentNumber: %v", err)
	}
	// Insert patent to satisfy foreign key constraints
	p := domain.Patent{
		Number:     patentNum,
		FetchState: domain.FetchCached,
	}
	if err := eng.SavePatent(ctx, p); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}

	// 1. Save note (no prior note, should have Before == nil)
	note := domain.PatentNote{
		Project:   project.ID,
		Patent:    patentNum,
		Markdown:  "Hello notes!",
		AddedAt:   time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	_, err = eng.SavePatentNote(ctx, note)
	if err != nil {
		t.Fatalf("SavePatentNote: %v", err)
	}

	// 2. Save note again (with prior note)
	note.Markdown = "Hello notes updated!"
	_, err = eng.SavePatentNote(ctx, note)
	if err != nil {
		t.Fatalf("SavePatentNote 2: %v", err)
	}

	// 3. Delete note
	if err := eng.DeletePatentNote(ctx, project.ID, patentNum); err != nil {
		t.Fatalf("DeletePatentNote: %v", err)
	}

	// Close runtime to flush activity logs
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close runtime: %v", err)
	}

	// Read activity log files
	files, err := os.ReadDir(logsDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var activityPath string
	for _, f := range files {
		if filepath.HasPrefix(f.Name(), "activity-") {
			activityPath = filepath.Join(logsDir, f.Name())
			break
		}
	}
	if activityPath == "" {
		t.Fatal("No activity log file found")
	}

	content, err := os.ReadFile(activityPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	type rawRecord struct {
		Action   string             `json:"action"`
		EntityID string             `json:"entity_id"`
		Before   *domain.PatentNote `json:"before"`
		After    *domain.PatentNote `json:"after"`
	}

	var notesRecords []rawRecord
	for _, line := range bytes.Split(content, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var rec rawRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if rec.Action == observability.ActionNotesSave || rec.Action == observability.ActionNotesDelete {
			notesRecords = append(notesRecords, rec)
		}
	}

	// We expect exactly 3 note-related records:
	// - 1st notes.save: Before == nil, After == "Hello notes!"
	// - 2nd notes.save: Before == "Hello notes!", After == "Hello notes updated!"
	// - 3rd notes.delete: Before == "Hello notes updated!", After == nil
	if len(notesRecords) != 3 {
		t.Fatalf("Expected exactly 3 note activity records, got %d", len(notesRecords))
	}

	// Verification of 1st save
	if notesRecords[0].Action != observability.ActionNotesSave {
		t.Errorf("1st rec action = %s, want notes.save", notesRecords[0].Action)
	}
	if notesRecords[0].Before != nil {
		t.Errorf("1st rec Before = %v, want nil", notesRecords[0].Before)
	}
	if notesRecords[0].After == nil || notesRecords[0].After.Markdown != "Hello notes!" {
		t.Errorf("1st rec After markdown = %v, want 'Hello notes!'", notesRecords[0].After)
	}

	// Verification of 2nd save
	if notesRecords[1].Action != observability.ActionNotesSave {
		t.Errorf("2nd rec action = %s, want notes.save", notesRecords[1].Action)
	}
	if notesRecords[1].Before == nil || notesRecords[1].Before.Markdown != "Hello notes!" {
		t.Errorf("2nd rec Before markdown = %v, want 'Hello notes!'", notesRecords[1].Before)
	}
	if notesRecords[1].After == nil || notesRecords[1].After.Markdown != "Hello notes updated!" {
		t.Errorf("2nd rec After markdown = %v, want 'Hello notes updated!'", notesRecords[1].After)
	}

	// Verification of delete
	if notesRecords[2].Action != observability.ActionNotesDelete {
		t.Errorf("3rd rec action = %s, want notes.delete", notesRecords[2].Action)
	}
	if notesRecords[2].Before == nil || notesRecords[2].Before.Markdown != "Hello notes updated!" {
		t.Errorf("3rd rec Before markdown = %v, want 'Hello notes updated!'", notesRecords[2].Before)
	}
	if notesRecords[2].After != nil {
		t.Errorf("3rd rec After = %v, want nil", notesRecords[2].After)
	}
}

