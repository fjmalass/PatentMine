package engine

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
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

func TestEngineEnforcesMembershipTransitions(t *testing.T) {
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
	if err := eng.AddToProject(ctx, project.ID, patent.Number); err != nil {
		t.Fatalf("AddToProject: %v", err)
	}

	// stored -> deleted is allowed.
	if err := eng.SetMembershipState(ctx, project.ID, patent.Number, domain.MembershipDeleted); err != nil {
		t.Fatalf("stored->deleted: %v", err)
	}
	// deleted -> ignored is rejected by the domain transition rules.
	if err := eng.SetMembershipState(ctx, project.ID, patent.Number, domain.MembershipIgnored); err == nil {
		t.Fatal("deleted->ignored should be rejected")
	}
	// deleted -> stored (undelete) is allowed.
	if err := eng.SetMembershipState(ctx, project.ID, patent.Number, domain.MembershipStored); err != nil {
		t.Fatalf("deleted->stored: %v", err)
	}
}

func TestEngineIngestEmitsProgressAndDone(t *testing.T) {
	factory := func(root domain.PatentNumber, _ int) Job {
		return JobFunc(func(_ context.Context, id JobID, emit func(proto.Event)) error {
			emit(proto.NewEvent(proto.EventIngestProgress, proto.IngestProgress{
				JobID: string(id), Found: 1, Message: "crawled " + root.String(),
			}))
			return nil
		})
	}
	eng, _ := newTestEngine(t, factory)

	events, unsub := eng.Subscribe()
	defer unsub()

	jobID, err := eng.StartFamilyIngest(domain.MustParsePatentNumber("US11611785B2"), 1)
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

func TestEngineIngestWithoutFactoryFails(t *testing.T) {
	eng, _ := newTestEngine(t, nil)
	if _, err := eng.StartFamilyIngest(domain.MustParsePatentNumber("US11611785B2"), 1); err == nil {
		t.Fatal("StartFamilyIngest without a factory should fail")
	}
}

func TestEngineExportIDSExcludesIgnoredAndDeleted(t *testing.T) {
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
		if err := eng.AddToProject(ctx, project.ID, number); err != nil {
			t.Fatalf("AddToProject %s: %v", n, err)
		}
	}
	// Ignored patents are not disclosed on the IDS.
	if err := eng.SetMembershipState(ctx, project.ID,
		domain.MustParsePatentNumber("US0000002B2"), domain.MembershipIgnored); err != nil {
		t.Fatalf("SetMembershipState: %v", err)
	}

	ids, err := eng.ExportIDS(ctx, project.ID)
	if err != nil {
		t.Fatalf("ExportIDS: %v", err)
	}
	if ids.Status != domain.IDSDraft {
		t.Fatalf("IDS status = %s, want draft", ids.Status)
	}
	if len(ids.Entries) != 2 {
		t.Fatalf("IDS has %d entries, want 2 (the ignored patent excluded)", len(ids.Entries))
	}
	for _, e := range ids.Entries {
		if e.Number.Serial == "0000002" {
			t.Fatal("ignored patent must not appear on the IDS")
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
	if err := eng.AddToProject(ctx, project.ID, granted); err != nil {
		t.Fatalf("AddToProject granted: %v", err)
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
	if err := eng.AddToProject(ctx, project.ID, pending); err != nil {
		t.Fatalf("AddToProject pending: %v", err)
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
	if err := eng.AddToProject(ctx, project.ID, application); err != nil {
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
	if err := eng.SetMembershipState(ctx, project.ID, application, domain.MembershipUnderReview); err != nil {
		t.Fatalf("SetMembershipState by application number: %v", err)
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
	_, err := eng.Relations(context.Background(),
		domain.MustParsePatentNumber("US0000001B2"), domain.RelationKind("bogus"))
	if err == nil {
		t.Fatal("Relations with an invalid kind should fail")
	}
}
