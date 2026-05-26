// Package crawl pulls patents and family-graph edges from the web. A Source
// fetches one patent from one provider; the Crawler walks the family graph
// across sources and writes the result to the store.
package crawl

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
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
	Patent               domain.Patent
	Documents            []domain.Document
	Relations            []domain.Relation
	AuthorityIdentifiers []domain.AuthorityIdentifier
	USPTOApplication     *domain.USPTOApplication
	USPTOParties         []domain.USPTOParty
	USPTOEvents          []domain.USPTOEvent
	USPTOContinuities    []domain.USPTOContinuity
	USPTOForeignPriority []domain.USPTOForeignPriority
	SourceSnapshots      []domain.SourceSnapshot
	SourceDiffs          []domain.SourceDiff
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
	sources    []Source
	googleMode string
	modeMu     sync.RWMutex
	metrics    *observability.Metrics
	logger     *slog.Logger
}

// NewRegistry builds a Registry from sources, consulted in the given order.
func NewRegistry(sources ...Source) *Registry {
	return &Registry{sources: sources, googleMode: SourceModeCompare}
}

// WithGoogleComparison enables best-effort Google fetches after a successful
// USPTO fetch. The USPTO result remains authoritative; Google only produces
// source_diff rows.
func (r *Registry) WithGoogleComparison(enabled bool) *Registry {
	if enabled {
		_ = r.SetSourceMode(SourceModeCompare)
	} else {
		_ = r.SetSourceMode(SourceModeFallback)
	}
	return r
}

// WithSourceMode sets the initial runtime provider mode. Invalid values leave
// the default in place; config validation should catch them before startup.
func (r *Registry) WithSourceMode(mode string) *Registry {
	_ = r.SetSourceMode(mode)
	return r
}

const (
	SourceModeCompare  = "compare"
	SourceModeFallback = "fallback"
	SourceModeOff      = "off"
)

// NormalizeSourceMode validates a user/config supplied source mode.
func NormalizeSourceMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", SourceModeCompare:
		return SourceModeCompare, nil
	case SourceModeFallback:
		return SourceModeFallback, nil
	case SourceModeOff:
		return SourceModeOff, nil
	default:
		return "", fmt.Errorf("crawl: unknown source mode %q", mode)
	}
}

// SetSourceMode changes normal provider behavior at runtime.
func (r *Registry) SetSourceMode(mode string) error {
	normalized, err := NormalizeSourceMode(mode)
	if err != nil {
		return err
	}
	r.modeMu.Lock()
	r.googleMode = normalized
	r.modeMu.Unlock()
	return nil
}

// SourceMode returns the current normal provider behavior.
func (r *Registry) SourceMode() string {
	r.modeMu.RLock()
	defer r.modeMu.RUnlock()
	return r.googleMode
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

// FetchOnly fetches from exactly one provider. It is used by source-specific
// commands such as add.uspto/add.google where fallback would be misleading.
func (r *Registry) FetchOnly(ctx context.Context, number domain.PatentNumber, source domain.Source) (Result, error) {
	lastErr := error(ErrNotAvailable)
	for _, s := range r.sources {
		if s.Name() != source {
			continue
		}
		res, err := s.Fetch(ctx, number)
		if err == nil {
			return res, nil
		}
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		lastErr = err
	}
	return Result{}, lastErr
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
		if s.Name() == domain.SourceGoogle && r.SourceMode() == SourceModeOff {
			continue
		}
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
			if s.Name() == domain.SourceUSPTO && r.SourceMode() == SourceModeCompare {
				res = r.compareWithGoogle(ctx, number, res, exclude...)
			}
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

func (r *Registry) compareWithGoogle(ctx context.Context, number domain.PatentNumber, uspto Result, exclude ...domain.Source) Result {
	if slices.Contains(exclude, domain.SourceGoogle) {
		return uspto
	}
	for _, s := range r.sources {
		if s.Name() != domain.SourceGoogle {
			continue
		}
		google, err := s.Fetch(ctx, number)
		if err != nil {
			if r.logger != nil && !errors.Is(err, ErrNotAvailable) && ctx.Err() == nil {
				r.logger.Warn("google comparison fetch failed",
					slog.String("number", number.String()),
					slog.String("error", err.Error()))
			}
			return uspto
		}
		uspto.SourceSnapshots = append(uspto.SourceSnapshots, google.SourceSnapshots...)
		uspto.SourceDiffs = append(uspto.SourceDiffs, comparePatentFields(uspto.Patent, google.Patent)...)
		return uspto
	}
	return uspto
}

func comparePatentFields(uspto, google domain.Patent) []domain.SourceDiff {
	now := time.Now().UTC().Format(time.RFC3339)
	fields := []struct {
		path  string
		left  string
		right string
	}{
		{"title", uspto.Title, google.Title},
		{"abstract", uspto.Abstract, google.Abstract},
		{"assignee", uspto.Assignee, google.Assignee},
		{"inventors", inventorsString(uspto.Inventors), inventorsString(google.Inventors)},
		{"application_date", dateString(uspto.ApplicationDate), dateString(google.ApplicationDate)},
		{"publication_date", dateString(uspto.PublicationDate), dateString(google.PublicationDate)},
		{"grant_date", dateString(uspto.GrantDate), dateString(google.GrantDate)},
		{"classifications", strings.Join(uspto.Classifications, ";"), strings.Join(google.Classifications, ";")},
		{"first_claim", uspto.FirstClaim, google.FirstClaim},
	}
	var diffs []domain.SourceDiff
	for _, f := range fields {
		left, right := strings.TrimSpace(f.left), strings.TrimSpace(f.right)
		if left == right || (left == "" && right == "") {
			continue
		}
		idSeed := uspto.Number.Normalized() + ":" + f.path + ":" + left + ":" + right
		diffs = append(diffs, domain.SourceDiff{
			ID:           "diff:" + payloadHash([]byte(idSeed))[:24],
			PatentNumber: uspto.Number,
			FieldPath:    f.path,
			USPTOValue:   left,
			GoogleValue:  right,
			ChosenValue:  left,
			ChosenSource: string(domain.SourceUSPTO),
			Severity:     diffSeverity(left, right),
			RecordedAt:   now,
		})
	}
	return diffs
}

func inventorsString(in []domain.Inventor) string {
	parts := make([]string, 0, len(in))
	for _, inv := range in {
		parts = append(parts, string(inv))
	}
	return strings.Join(parts, ";")
}

func dateString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

func diffSeverity(usptoValue, googleValue string) string {
	if usptoValue == "" || googleValue == "" {
		return "info"
	}
	if len(usptoValue) > 256 || len(googleValue) > 256 {
		return "warning"
	}
	return "conflict"
}
