package citationlookup

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"patentmine/internal/domain"
)

type WorkerStore interface {
	ClaimNextCitationRefreshJob(context.Context, time.Time) (domain.CitationRefreshJob, bool, error)
	UpsertCitationGraphRefresh(context.Context, domain.ProjectID, domain.PatentBundle, RefreshMetadata) error
	CompleteCitationRefreshJob(context.Context, int64) error
	RescheduleCitationRefreshJob(context.Context, int64, time.Time, string) error
	FailCitationRefreshJob(context.Context, int64, string) error
}

type Importer interface {
	ImportPatent(context.Context, domain.PatentNumber) (domain.PatentBundle, error)
}

type Limiter interface {
	Acquire(context.Context) (func(), error)
}

type RefreshMetadata struct {
	RequestedPatent     string
	ProvenanceSource    string
	ProvenanceVersion   string
	FreshFor            time.Duration
	PartialData         bool
	ForwardPageCap      int
	ForwardPagesFetched int
	ForwardPageCapHit   bool
}

type Worker struct {
	store       WorkerStore
	importer    Importer
	limiter     Limiter
	breaker     *CircuitBreaker
	metrics     *Metrics
	now         func() time.Time
	maxRetries  int
	baseBackoff time.Duration
	maxBackoff  time.Duration
	rand        *rand.Rand
}

func NewWorker(store WorkerStore, importer Importer, limiter Limiter) *Worker {
	return &Worker{
		store:       store,
		importer:    importer,
		limiter:     limiter,
		now:         time.Now,
		maxRetries:  6,
		baseBackoff: 30 * time.Second,
		maxBackoff:  30 * time.Minute,
		rand:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (w *Worker) WithCircuitBreaker(breaker *CircuitBreaker) *Worker {
	w.breaker = breaker
	return w
}

func (w *Worker) WithMetrics(metrics *Metrics) *Worker {
	w.metrics = metrics
	return w
}

func (w *Worker) WorkOnce(ctx context.Context) (bool, error) {
	job, ok, err := w.store.ClaimNextCitationRefreshJob(ctx, w.now())
	if err != nil || !ok {
		return ok, err
	}
	if w.breaker != nil {
		if openUntil, ok := w.breaker.Allow(w.now()); !ok {
			message := w.breaker.OpenError(openUntil)
			return true, w.store.RescheduleCitationRefreshJob(ctx, job.ID, openUntil, message.Error())
		}
	}
	if w.limiter != nil {
		release, err := w.limiter.Acquire(ctx)
		if err != nil {
			_ = w.store.RescheduleCitationRefreshJob(ctx, job.ID, w.now().Add(w.baseBackoff), err.Error())
			return true, err
		}
		defer release()
	}

	if w.metrics != nil {
		w.metrics.upstreamRequests.Add(1)
	}
	bundle, err := w.importer.ImportPatent(ctx, job.PatentNumber)
	if err != nil {
		var up UpstreamError
		if errors.As(err, &up) && up.Transient {
			if w.metrics != nil {
				if up.StatusCode == http.StatusTooManyRequests {
					w.metrics.upstream429.Add(1)
				}
				w.metrics.refreshFailures.Add(1)
			}
			if w.breaker != nil {
				w.breaker.RecordFailure(w.now(), up)
			}
			return true, w.reschedule(ctx, job, up)
		}
		if job.RetryCount < w.maxRetries && isRetryableContextError(err) {
			if w.metrics != nil {
				w.metrics.timeouts.Add(1)
				w.metrics.refreshFailures.Add(1)
			}
			if w.breaker != nil {
				w.breaker.RecordFailure(w.now(), err)
			}
			return true, w.reschedule(ctx, job, UpstreamError{Err: err, Transient: true})
		}
		if w.metrics != nil {
			w.metrics.refreshFailures.Add(1)
		}
		_ = w.store.FailCitationRefreshJob(ctx, job.ID, err.Error())
		return true, nil
	}
	if bundle.Patent.Number == "" {
		bundle.Patent.Number = job.PatentNumber
	}

	meta := RefreshMetadata{
		RequestedPatent:   string(job.PatentNumber),
		ProvenanceSource:  "uspto-odp",
		ProvenanceVersion: "unknown",
		FreshFor:          24 * time.Hour,
	}
	if err := w.store.UpsertCitationGraphRefresh(ctx, job.ProjectID, bundle, meta); err != nil {
		return true, err
	}
	if w.breaker != nil {
		w.breaker.RecordSuccess(w.now())
	}
	return true, w.store.CompleteCitationRefreshJob(ctx, job.ID)
}

func (w *Worker) reschedule(ctx context.Context, job domain.CitationRefreshJob, up UpstreamError) error {
	if job.RetryCount >= w.maxRetries {
		return w.store.FailCitationRefreshJob(ctx, job.ID, up.Error())
	}
	delay := up.RetryAfter
	if delay <= 0 {
		delay = w.backoff(job.RetryCount)
	}
	return w.store.RescheduleCitationRefreshJob(ctx, job.ID, w.now().Add(delay), up.Error())
}

func (w *Worker) backoff(retryCount int) time.Duration {
	delay := w.baseBackoff
	for i := 0; i < retryCount; i++ {
		delay *= 2
		if delay >= w.maxBackoff {
			delay = w.maxBackoff
			break
		}
	}
	jitter := time.Duration(w.rand.Int63n(int64(delay / 2)))
	return delay + jitter
}

type UpstreamError struct {
	Err        error
	StatusCode int
	RetryAfter time.Duration
	Transient  bool
}

func (e UpstreamError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("upstream HTTP %d", e.StatusCode)
}

func (e UpstreamError) Unwrap() error {
	return e.Err
}

func UpstreamHTTPError(status int, retryAfterHeader string) UpstreamError {
	return UpstreamError{
		StatusCode: status,
		RetryAfter: parseRetryAfter(retryAfterHeader),
		Transient:  status == http.StatusTooManyRequests || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout,
	}
}

func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil {
		return time.Until(at)
	}
	return 0
}

func isRetryableContextError(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}
