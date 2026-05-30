package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/store"
)

type mockSourceModeController struct {
	mode string
}

func (m *mockSourceModeController) SourceMode() string {
	return m.mode
}

func (m *mockSourceModeController) SetSourceMode(mode string) error {
	m.mode = mode
	return nil
}

func TestEngineSourceModesAndStateTransitions(t *testing.T) {
	modes := []string{"compare", "uspto-first", "uspto-only", "google-only"}

	for _, mode := range modes {
		t.Run("source.mode="+mode, func(t *testing.T) {
			ctx := context.Background()

			// Define a mock CrawlFactory that simulates a successful crawl.
			// It marks the root patent as cached and creates a neighbor stub.
			var activeRepo store.Repository
			factory := func(root domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool, source domain.Source) Job {
				return JobFunc(func(ctx context.Context, id JobID, emit func(proto.Event)) error {
					rootPatent := domain.Patent{
						Number:        root,
						DisplayNumber: root,
						Title:         "Mock Patent Title",
						FetchState:    domain.FetchCached,
					}
					
					// Determine expected source based on source mode
					if source != "" {
						rootPatent.Source = source
					} else {
						switch mode {
						case "google-only":
							rootPatent.Source = domain.SourceGoogle
						case "uspto-only":
							rootPatent.Source = domain.SourceUSPTO
						case "uspto-first", "compare":
							rootPatent.Source = domain.SourceUSPTO
						}
					}

					citation := domain.MustParsePatentNumber("US7654321B2")
					batch := store.NodeBatch{
						Patent: rootPatent,
						Relations: []domain.Relation{
							{From: root, To: citation, Kind: domain.RelationCites},
						},
						Stubs: []store.StubRecord{
							{Number: citation, Stage: domain.StageGrant},
						},
					}

					if err := activeRepo.SaveNode(ctx, batch); err != nil {
						return err
					}

					// Emit done event to trigger post-crawl promotion in engine
					d := proto.CrawlDone{
						JobID: string(id),
						Error: "",
					}
					payload, _ := json.Marshal(d)
					emit(proto.Event{
						Method: proto.EventCrawlDone,
						Params: payload,
					})
					return nil
				})
			}

			eng, repo := newTestEngine(t, factory)
			activeRepo = repo

			// Configure the mock source mode controller
			smCtrl := &mockSourceModeController{mode: mode}
			eng.sourceModes = smCtrl

			// Create a project
			project, err := eng.CreateProject(ctx, "Test Project")
			if err != nil {
				t.Fatalf("CreateProject: %v", err)
			}

			// Subscribe to event bus to wait for crawl completion
			events, unsub := eng.Subscribe()
			defer unsub()

			// Add the root patent to the project
			rootNumber := domain.MustParsePatentNumber("US11549750B1")
			fetchStarted, jobID, candidates, err := eng.AddToProject(ctx, project.ID, rootNumber)
			if err != nil {
				t.Fatalf("AddToProject: %v", err)
			}
			_ = candidates
			if !fetchStarted {
				t.Fatalf("expected crawl job to be started")
			}
			if jobID == "" {
				t.Fatalf("expected non-empty job ID")
			}

			// Wait for CrawlDone event
			gotDone := false
			deadline := time.After(2 * time.Second)
			for !gotDone {
				select {
				case ev := <-events:
					if ev.Method == proto.EventCrawlDone {
						var done proto.CrawlDone
						if err := json.Unmarshal(ev.Params, &done); err == nil && done.JobID == string(jobID) {
							gotDone = true
						}
					}
				case <-deadline:
					t.Fatalf("timed out waiting for CrawlDone event")
				}
			}

			// Give cleanupIfNotFound goroutine a short moment to write state change to database
			time.Sleep(100 * time.Millisecond)

			// 1. Verify the root patent is set to FetchState = cached and ReviewState = under_review
			pRoot, err := repo.Patent(ctx, rootNumber)
			if err != nil {
				t.Fatalf("failed to retrieve root patent: %v", err)
			}
			if pRoot.FetchState != domain.FetchCached {
				t.Errorf("expected root patent fetch state to be cached, got %s", pRoot.FetchState)
			}

			mRoot, err := repo.Membership(ctx, project.ID, rootNumber)
			if err != nil {
				t.Fatalf("failed to retrieve root membership: %v", err)
			}
			if mRoot.ReviewState != domain.ReviewStateUnderReview {
				t.Errorf("expected root patent review state to be under_review, got %s", mRoot.ReviewState)
			}

			// Verify source resolution based on the configured source.mode
			var expectedSource domain.Source
			switch mode {
			case "google-only":
				expectedSource = domain.SourceGoogle
			case "uspto-only", "uspto-first", "compare":
				expectedSource = domain.SourceUSPTO
			}
			if pRoot.Source != expectedSource {
				t.Errorf("expected root patent source to be %s, got %s", expectedSource, pRoot.Source)
			}

			// 2. Verify the crawled neighbor citation has FetchState = stub and ReviewState = unknown (not in project)
			citationNumber := domain.MustParsePatentNumber("US7654321B2")
			pNeighbor, err := repo.Patent(ctx, citationNumber)
			if err != nil {
				t.Fatalf("failed to retrieve neighbor patent: %v", err)
			}
			if pNeighbor.FetchState != domain.FetchStub {
				t.Errorf("expected neighbor patent fetch state to be stub, got %s", pNeighbor.FetchState)
			}

			// Neighbor was not added to the project, so membership should not exist or be unassigned/unknown
			_, err = repo.Membership(ctx, project.ID, citationNumber)
			if err == nil {
				t.Errorf("expected neighbor patent to not have project membership")
			}
		})
	}
}
