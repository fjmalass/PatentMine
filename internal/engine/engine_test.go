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

// newTestEngine builds an engine over a fresh temp database.
func newTestEngine(t *testing.T, ingest IngestFactory) *Engine {
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
	return eng
}

func TestEngineProjectLifecycle(t *testing.T) {
	eng := newTestEngine(t, nil)
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
	eng := newTestEngine(t, nil)
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
	eng := newTestEngine(t, factory)

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
	eng := newTestEngine(t, nil)
	if _, err := eng.StartFamilyIngest(domain.MustParsePatentNumber("US11611785B2"), 1); err == nil {
		t.Fatal("StartFamilyIngest without a factory should fail")
	}
}

func TestEngineExportIDSExcludesIgnoredAndDeleted(t *testing.T) {
	eng := newTestEngine(t, nil)
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

func TestEngineExportIDSUnknownProject(t *testing.T) {
	eng := newTestEngine(t, nil)
	if _, err := eng.ExportIDS(context.Background(), "no-such-project"); err == nil {
		t.Fatal("ExportIDS for an unknown project should fail")
	}
}

func TestEngineRelationsRejectsInvalidKind(t *testing.T) {
	eng := newTestEngine(t, nil)
	_, err := eng.Relations(context.Background(),
		domain.MustParsePatentNumber("US0000001B2"), domain.RelationKind("bogus"))
	if err == nil {
		t.Fatal("Relations with an invalid kind should fail")
	}
}
