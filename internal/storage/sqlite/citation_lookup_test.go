package sqlite

import (
	"context"
	"testing"
	"time"

	"patentmine/internal/citationlookup"
	"patentmine/internal/domain"
)

func TestCitationGraphCacheMaterializedWithBundle(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	bundle := domain.PatentBundle{
		Patent: domain.Patent{
			Number:            "US8164048B2",
			Title:             "Citation graph source",
			ImportSource:      "uspto",
			ExpectedCitations: 1,
			ExpectedCitedBy:   1,
		},
		Citations: []domain.CitationEdge{
			{TargetPatent: "US1", RelationType: domain.RelationCites},
			{TargetPatent: "US2", RelationType: domain.RelationCitedBy},
		},
	}
	if err := repo.UpsertPatentBundle(ctx, "default", bundle); err != nil {
		t.Fatal(err)
	}

	graph, ok, err := repo.GetCitationGraph(ctx, "default", "US8164048B2")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected graph cache row")
	}
	if graph.Cache.Status != domain.CitationLookupStatusFresh || graph.Cache.ProvenanceSource != "uspto" {
		t.Fatalf("unexpected cache metadata: %+v", graph.Cache)
	}
	if len(graph.Cited) != 1 || len(graph.Citing) != 1 {
		t.Fatalf("expected cited and citing edges, got cited=%d citing=%d", len(graph.Cited), len(graph.Citing))
	}
	if graph.Cache.ExpectedCitations != 1 || graph.Cache.ExpectedCitedBy != 1 {
		t.Fatalf("expected totals persisted in cache, got %+v", graph.Cache)
	}
}

func TestCitationGraphLookupMatchesUSKindCodeAlias(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	if err := repo.UpsertPatentBundle(ctx, "default", domain.PatentBundle{
		Patent: domain.Patent{Number: "US8164048", Title: "Citation graph source", ImportSource: "uspto"},
		Citations: []domain.CitationEdge{{
			SourcePatent: "US8164048",
			TargetPatent: "US1",
			RelationType: domain.RelationCites,
		}},
	}); err != nil {
		t.Fatal(err)
	}

	graph, ok, err := repo.GetCitationGraph(ctx, "default", "US8164048B2")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || graph.Cache.PatentNumber != "US8164048" || len(graph.Cited) != 1 {
		t.Fatalf("expected kind-code alias lookup to resolve canonical graph, ok=%v graph=%+v", ok, graph)
	}
}

func TestCitationRefreshQueueDedupesActiveJobs(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	when := time.Date(2026, 5, 20, 20, 0, 0, 0, time.UTC)

	job1, created, err := repo.EnqueueCitationRefresh(ctx, "default", "US8164048B2", citationlookup.EnqueueOptions{NextAttemptAt: when, Priority: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !created || job1.ID == 0 {
		t.Fatalf("expected new job, got created=%v job=%+v", created, job1)
	}

	job2, created, err := repo.EnqueueCitationRefresh(ctx, "default", "US8164048B2", citationlookup.EnqueueOptions{NextAttemptAt: when, Priority: 1})
	if err != nil {
		t.Fatal(err)
	}
	if created || job2.ID != job1.ID || job2.TraceCount != 2 {
		t.Fatalf("expected active job dedupe and trace coalescing, created=%v job1=%+v job2=%+v", created, job1, job2)
	}
}

func TestCitationRefreshQueueClaimRescheduleComplete(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	due := time.Date(2026, 5, 20, 20, 0, 0, 0, time.UTC)
	job, _, err := repo.EnqueueCitationRefresh(ctx, "default", "US8164048B2", citationlookup.EnqueueOptions{NextAttemptAt: due})
	if err != nil {
		t.Fatal(err)
	}

	claimed, ok, err := repo.ClaimNextCitationRefreshJob(ctx, due.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !ok || claimed.ID != job.ID || claimed.Status != domain.CitationRefreshJobRunning {
		t.Fatalf("expected claimed running job, ok=%v job=%+v", ok, claimed)
	}

	next := due.Add(time.Minute)
	if err := repo.RescheduleCitationRefreshJob(ctx, claimed.ID, next, "HTTP 429"); err != nil {
		t.Fatal(err)
	}
	rescheduled, err := repo.GetCitationRefreshJob(ctx, claimed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rescheduled.Status != domain.CitationRefreshJobQueued || rescheduled.RetryCount != 1 || rescheduled.LastError != "HTTP 429" {
		t.Fatalf("expected queued retry, got %+v", rescheduled)
	}

	claimed, ok, err = repo.ClaimNextCitationRefreshJob(ctx, next.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !ok || claimed.RetryCount != 1 {
		t.Fatalf("expected retry claim, ok=%v job=%+v", ok, claimed)
	}
	if err := repo.CompleteCitationRefreshJob(ctx, claimed.ID); err != nil {
		t.Fatal(err)
	}
	done, err := repo.GetCitationRefreshJob(ctx, claimed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != domain.CitationRefreshJobSucceeded || done.CompletedAt.IsZero() {
		t.Fatalf("expected succeeded job, got %+v", done)
	}
}

func TestCitationLookupStatsReportsQueueAndSafeguards(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)
	now := time.Now().UTC()
	job, _, err := repo.EnqueueCitationRefresh(ctx, "default", "US8164048B2", citationlookup.EnqueueOptions{NextAttemptAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.EnqueueCitationRefresh(ctx, "default", "US8164048B2", citationlookup.EnqueueOptions{NextAttemptAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.RescheduleCitationRefreshJob(ctx, job.ID, now.Add(time.Minute), "HTTP 429"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertPatentBundle(ctx, "default", domain.PatentBundle{
		Patent: domain.Patent{Number: "US8164048B2", ImportSource: "uspto"},
	}); err != nil {
		t.Fatal(err)
	}

	stats, err := repo.CitationLookupStats(ctx, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if stats.QueuedJobs != 1 || stats.CoalescedTraceCount != 1 || stats.Upstream429Count != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if stats.OldestQueuedAge <= 0 {
		t.Fatalf("expected queued age, got %+v", stats)
	}
}
