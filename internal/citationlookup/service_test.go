package citationlookup

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"patentmine/internal/domain"
)

func TestLookupReturnsFreshCacheWithoutEnqueue(t *testing.T) {
	store := &fakeGraphStore{
		graph: domain.CitationGraph{
			Cache: domain.CitationGraphCacheRecord{
				Status:           domain.CitationLookupStatusFresh,
				ProvenanceSource: "uspto-odp",
				FreshUntil:       time.Date(2026, 5, 21, 0, 0, 0, 0, time.UTC),
			},
		},
		ok: true,
	}
	svc := NewService(store)
	svc.now = func() time.Time { return time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC) }

	resp, err := svc.Lookup(context.Background(), LookupRequest{ProjectID: "tenant-a", PatentNumber: "8164048"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.HTTPStatus != 200 || resp.Status != domain.CitationLookupStatusFresh {
		t.Fatalf("expected fresh 200 response, got %+v", resp)
	}
	if store.enqueueCalls != 0 {
		t.Fatalf("fresh cache should not enqueue refresh, got %d call(s)", store.enqueueCalls)
	}
	if resp.PatentNumber != "US8164048" {
		t.Fatalf("expected normalized patent number, got %q", resp.PatentNumber)
	}
}

func TestLookupReturnsStaleWhileRevalidate(t *testing.T) {
	store := &fakeGraphStore{
		graph: domain.CitationGraph{
			Cache: domain.CitationGraphCacheRecord{
				Status:           domain.CitationLookupStatusFresh,
				ProvenanceSource: "uspto-odp",
				FreshUntil:       time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC),
			},
		},
		ok:  true,
		job: domain.CitationRefreshJob{ID: 42},
	}
	svc := NewService(store)
	svc.now = func() time.Time { return time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC) }

	resp, err := svc.Lookup(context.Background(), LookupRequest{ProjectID: "tenant-a", PatentNumber: "US8164048B2"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.HTTPStatus != 200 || resp.Status != domain.CitationLookupStatusStale || !resp.RefreshPending || resp.JobID != 42 {
		t.Fatalf("expected stale response with queued job, got %+v", resp)
	}
	if store.enqueueCalls != 1 {
		t.Fatalf("expected one enqueue call, got %d", store.enqueueCalls)
	}
}

func TestLookupReturnsPendingWhenMissing(t *testing.T) {
	store := &fakeGraphStore{job: domain.CitationRefreshJob{ID: 7}}
	svc := NewService(store)

	resp, err := svc.Lookup(context.Background(), LookupRequest{ProjectID: "tenant-a", PatentNumber: "8164048"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.HTTPStatus != 202 || resp.Status != domain.CitationLookupStatusPending || !resp.RefreshPending || resp.JobID != 7 {
		t.Fatalf("expected pending 202 response, got %+v", resp)
	}
}

func TestWorkerReschedulesTransientUpstreamFailure(t *testing.T) {
	store := &fakeWorkerStore{job: domain.CitationRefreshJob{ID: 3, ProjectID: "tenant-a", PatentNumber: "US8164048"}}
	worker := NewWorker(store, fakeImporter{err: UpstreamHTTPError(429, "60")}, NoopLimiter{})
	worker.now = func() time.Time { return time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC) }

	worked, err := worker.WorkOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !worked || store.rescheduledID != 3 {
		t.Fatalf("expected job to be rescheduled, worked=%v store=%+v", worked, store)
	}
	if got := store.nextAttempt.Sub(worker.now()); got != 60*time.Second {
		t.Fatalf("expected Retry-After delay, got %s", got)
	}
}

func TestWorkerCompletesSuccessfulRefresh(t *testing.T) {
	store := &fakeWorkerStore{job: domain.CitationRefreshJob{ID: 4, ProjectID: "tenant-a", PatentNumber: "US8164048"}}
	worker := NewWorker(store, fakeImporter{bundle: domain.PatentBundle{Patent: domain.Patent{Number: "US8164048"}}}, NoopLimiter{})

	worked, err := worker.WorkOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !worked || store.upserted.Patent.Number != "US8164048" || store.completedID != 4 {
		t.Fatalf("expected refresh upsert and completion, worked=%v store=%+v", worked, store)
	}
}

func TestWorkerCircuitBreakerReschedulesWithoutImporterCall(t *testing.T) {
	now := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)
	store := &fakeWorkerStore{job: domain.CitationRefreshJob{ID: 5, ProjectID: "tenant-a", PatentNumber: "US8164048"}}
	breaker := NewCircuitBreaker(1, time.Minute)
	breaker.RecordFailure(now, UpstreamHTTPError(429, ""))
	importer := &countingImporter{}
	worker := NewWorker(store, importer, NoopLimiter{}).WithCircuitBreaker(breaker)
	worker.now = func() time.Time { return now }

	worked, err := worker.WorkOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !worked || store.rescheduledID != 5 || importer.calls != 0 {
		t.Fatalf("expected open circuit to reschedule before import, worked=%v calls=%d store=%+v", worked, importer.calls, store)
	}
}

func TestHTTPHandlerLookupRequiresTenantAndReturnsPending(t *testing.T) {
	store := &fakeGraphStore{job: domain.CitationRefreshJob{ID: 9}}
	handler := HTTPHandler{Service: NewService(store)}

	req := httptest.NewRequest(http.MethodGet, "/lookup?patent=8164048", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected tenant identity enforcement, got status %d", resp.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/lookup?patent=8164048", nil)
	req.Header.Set("X-Tenant-ID", "tenant-a")
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected pending 202, got status %d body=%s", resp.Code, resp.Body.String())
	}
	var body LookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.PatentNumber != "US8164048" || body.JobID != 9 || !body.RefreshPending {
		t.Fatalf("unexpected lookup body: %+v", body)
	}
}

func TestHTTPHandlerStatusReturnsJobAndStats(t *testing.T) {
	store := &fakeStatusStore{
		job:   domain.CitationRefreshJob{ID: 12, Status: domain.CitationRefreshJobQueued},
		stats: domain.CitationLookupStats{QueuedJobs: 1, CoalescedTraceCount: 2},
	}
	handler := HTTPHandler{Store: store, AuthorizedToken: "secret"}

	req := httptest.NewRequest(http.MethodGet, "/lookup/status?job_id=12", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"Status":"queued"`) {
		t.Fatalf("expected job status response, got status %d body=%s", resp.Code, resp.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/lookup/status", nil)
	req.Header.Set("X-API-Key", "secret")
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"QueuedJobs":1`) {
		t.Fatalf("expected stats response, got status %d body=%s", resp.Code, resp.Body.String())
	}
}

type fakeGraphStore struct {
	graph        domain.CitationGraph
	ok           bool
	job          domain.CitationRefreshJob
	enqueueCalls int
	err          error
}

func (f *fakeGraphStore) GetCitationGraph(context.Context, domain.ProjectID, domain.PatentNumber) (domain.CitationGraph, bool, error) {
	return f.graph, f.ok, f.err
}

func (f *fakeGraphStore) EnqueueCitationRefresh(context.Context, domain.ProjectID, domain.PatentNumber, EnqueueOptions) (domain.CitationRefreshJob, bool, error) {
	f.enqueueCalls++
	if f.err != nil {
		return domain.CitationRefreshJob{}, false, f.err
	}
	return f.job, f.enqueueCalls == 1, nil
}

type fakeImporter struct {
	bundle domain.PatentBundle
	err    error
}

func (f fakeImporter) ImportPatent(context.Context, domain.PatentNumber) (domain.PatentBundle, error) {
	if f.err != nil {
		return domain.PatentBundle{}, f.err
	}
	return f.bundle, nil
}

type countingImporter struct {
	calls int
}

func (f *countingImporter) ImportPatent(context.Context, domain.PatentNumber) (domain.PatentBundle, error) {
	f.calls++
	return domain.PatentBundle{Patent: domain.Patent{Number: "US8164048"}}, nil
}

type fakeWorkerStore struct {
	job           domain.CitationRefreshJob
	claimed       bool
	upserted      domain.PatentBundle
	completedID   int64
	rescheduledID int64
	nextAttempt   time.Time
	failedID      int64
}

func (f *fakeWorkerStore) ClaimNextCitationRefreshJob(context.Context, time.Time) (domain.CitationRefreshJob, bool, error) {
	if f.claimed {
		return domain.CitationRefreshJob{}, false, nil
	}
	f.claimed = true
	return f.job, true, nil
}

func (f *fakeWorkerStore) UpsertCitationGraphRefresh(_ context.Context, _ domain.ProjectID, bundle domain.PatentBundle, _ RefreshMetadata) error {
	if bundle.Patent.Number == "" {
		return errors.New("missing patent number")
	}
	f.upserted = bundle
	return nil
}

func (f *fakeWorkerStore) CompleteCitationRefreshJob(_ context.Context, id int64) error {
	f.completedID = id
	return nil
}

func (f *fakeWorkerStore) RescheduleCitationRefreshJob(_ context.Context, id int64, next time.Time, _ string) error {
	f.rescheduledID = id
	f.nextAttempt = next
	return nil
}

func (f *fakeWorkerStore) FailCitationRefreshJob(_ context.Context, id int64, _ string) error {
	f.failedID = id
	return nil
}

type fakeStatusStore struct {
	job   domain.CitationRefreshJob
	stats domain.CitationLookupStats
}

func (f *fakeStatusStore) GetCitationRefreshJob(context.Context, int64) (domain.CitationRefreshJob, error) {
	return f.job, nil
}

func (f *fakeStatusStore) CitationLookupStats(context.Context, time.Time) (domain.CitationLookupStats, error) {
	return f.stats, nil
}
