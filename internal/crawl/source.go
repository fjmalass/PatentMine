// Package crawl pulls patents and family-graph edges from the web. A Source
// fetches one patent from one provider; the Crawler walks the family graph
// across sources and writes the result to the store.
package crawl

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
)

const slowSourceFetch = 300 * time.Millisecond

// ErrNotAvailable means a Source has no record for the requested patent, so
// the Crawler should try the next source.
var ErrNotAvailable = errors.New("crawl: patent not available from this source")

// Result is one patent fetched from a Source: the record's bibliographic data,
// its life-stage documents (application, publication, grant), and the
// family-graph edges discovered alongside it.
type Result struct {
	Patent    domain.Patent
	Documents []domain.Document
	Relations []domain.Relation
}

// Source fetches a single patent from one provider.
type Source interface {
	// Name reports which provider this is.
	Name() domain.Source
	// Fetch retrieves one patent; it returns ErrNotAvailable when the provider
	// has no record for the number.
	Fetch(ctx context.Context, number domain.PatentNumber) (Result, error)
}

// Registry holds an ordered set of sources. Fetch tries them in turn so a
// preferred provider can be consulted before a fallback.
type Registry struct {
	sources []Source
	metrics *observability.Metrics
	logger  *slog.Logger
}

// NewRegistry builds a Registry from sources, consulted in the given order.
func NewRegistry(sources ...Source) *Registry {
	return &Registry{sources: sources}
}

// WithMetrics attaches a phase-1 in-process metrics recorder.
func (r *Registry) WithMetrics(metrics *observability.Metrics) *Registry {
	r.metrics = metrics
	for _, source := range r.sources {
		if setter, ok := source.(interface{ WithMetrics(*observability.Metrics) }); ok {
			setter.WithMetrics(metrics)
		}
	}
	return r
}

// WithLogger attaches a logger for slow fetch warnings.
func (r *Registry) WithLogger(logger *slog.Logger) *Registry {
	r.logger = logger
	for _, source := range r.sources {
		if setter, ok := source.(interface{ WithLogger(*slog.Logger) }); ok {
			setter.WithLogger(logger)
		}
	}
	return r
}

// Fetch returns the first successful result. A source reporting ErrNotAvailable
// is skipped; a hard error is remembered but the next source is still tried.
func (r *Registry) Fetch(ctx context.Context, number domain.PatentNumber) (Result, error) {
	return r.FetchExcluding(ctx, number)
}

// FetchExcluding is Fetch with the named sources skipped. A force fetch passes
// SourceFile so the crawl bypasses the local file cache and re-pulls from the web.
func (r *Registry) FetchExcluding(ctx context.Context, number domain.PatentNumber, exclude ...domain.Source) (Result, error) {
	start := time.Now()
	var failed bool
	defer func() {
		if r.metrics != nil {
			r.metrics.ObserveDuration("crawl.registry.fetch", time.Since(start), failed)
		}
	}()
	lastErr := error(ErrNotAvailable)
	for _, s := range r.sources {
		if slices.Contains(exclude, s.Name()) {
			continue
		}
		sourceStart := time.Now()
		res, err := s.Fetch(ctx, number)
		if r.metrics != nil {
			d := time.Since(sourceStart)
			r.metrics.ObserveDuration("crawl.source."+string(s.Name())+".fetch", d, err != nil)
			if r.logger != nil && d >= slowSourceFetch {
				r.logger.Warn("slow crawl source fetch",
					slog.String("source", string(s.Name())),
					slog.String("number", number.String()),
					slog.Int64("duration_ms", d.Milliseconds()),
					slog.Bool("failed", err != nil))
			}
		}
		if err == nil {
			return res, nil
		}
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		if !errors.Is(err, ErrNotAvailable) {
			lastErr = err
			failed = true
		}
	}
	failed = true
	return Result{}, lastErr
}
