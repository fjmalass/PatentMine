package sqlite

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"patentmine/internal/citationlookup"
	"patentmine/internal/config"
	"patentmine/internal/domain"
)

func TestLiveHybridCitationRefreshPipelineViaDotEnv(t *testing.T) {
	if os.Getenv("PATENTMINE_LIVE_USPTO") != "1" {
		t.Skip("set PATENTMINE_LIVE_USPTO=1 to run the live USPTO ODP pipeline smoke test")
	}
	root, err := findLiveRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatal(err)
		}
	})

	cfg := config.Load()
	if cfg.USPTO.APIKey == "" {
		t.Fatal("USPTO API key was not loaded from .env or environment")
	}

	ctx := context.Background()
	repo := newTestRepo(t)
	metrics := citationlookup.NewMetrics()
	job, created, err := repo.EnqueueCitationRefresh(ctx, "default", "US8164048B2", citationlookup.EnqueueOptions{
		RequestedPatent: "US8164048B2",
		Priority:        10,
		DesiredDepth:    domain.CitationRefreshDepthFull,
		TraceID:         "live-pipeline-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created || job.ID == 0 {
		t.Fatalf("expected durable refresh job, created=%v job=%+v", created, job)
	}

	worker := citationlookup.NewWorker(repo, citationlookup.USPTOImporter{APIKey: cfg.USPTO.APIKey}, citationlookup.NoopLimiter{}).WithMetrics(metrics)
	worked, err := worker.WorkOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !worked {
		t.Fatal("expected worker to process the queued live refresh job")
	}

	done, err := repo.GetCitationRefreshJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != domain.CitationRefreshJobSucceeded {
		t.Fatalf("expected succeeded refresh job, got %+v", done)
	}

	graph, ok, err := repo.GetCitationGraph(ctx, "default", "US8164048B2")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected graph cache to be readable by original kind-code patent number")
	}
	if graph.Patent.Number == "" || graph.Patent.Title == "" {
		t.Fatalf("expected live patent metadata in graph, got %+v", graph.Patent)
	}
	if len(graph.Cited)+len(graph.Citing) == 0 {
		t.Fatalf("expected materialized citation graph, got cited=%d citing=%d", len(graph.Cited), len(graph.Citing))
	}
	if graph.Cache.Status != domain.CitationLookupStatusFresh || graph.Cache.ProvenanceSource == "" || graph.Cache.RefreshedAt.IsZero() {
		t.Fatalf("expected fresh cache metadata with provenance, got %+v", graph.Cache)
	}

	service := citationlookup.NewService(repo).WithMetrics(metrics)
	handler := citationlookup.HTTPHandler{Service: service, Store: repo}
	req := httptest.NewRequest(http.MethodGet, "/lookup?project_id=default&patent=US8164048B2", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected cache-first HTTP 200 after refresh, got %d body=%s", resp.Code, resp.Body.String())
	}
	var lookup citationlookup.LookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&lookup); err != nil {
		t.Fatal(err)
	}
	if lookup.Status != domain.CitationLookupStatusFresh || lookup.RefreshPending {
		t.Fatalf("expected fresh non-pending lookup response, got %+v", lookup)
	}
	if len(lookup.Graph.Cited)+len(lookup.Graph.Citing) == 0 {
		t.Fatalf("expected HTTP lookup graph data, got %+v", lookup.Graph)
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/lookup/status", nil)
	statusResp := httptest.NewRecorder()
	handler.ServeHTTP(statusResp, statusReq)
	if statusResp.Code != http.StatusOK {
		t.Fatalf("expected status HTTP 200, got %d body=%s", statusResp.Code, statusResp.Body.String())
	}
	var stats domain.CitationLookupStats
	if err := json.NewDecoder(statusResp.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	if stats.SucceededJobs < 1 || stats.FreshCacheHits < 1 || stats.UpstreamRequests < 1 {
		t.Fatalf("expected status metrics for live pipeline, got %+v", stats)
	}
}

func findLiveRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
