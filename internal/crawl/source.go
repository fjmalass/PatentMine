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

// Granular USPTO-specific resolution errors (for better diagnostics and partial
// data handling — e.g. "we have the application but no grant XML yet").
var (
	// ErrUSPTOApplicationNotFound is returned when the File Wrapper search
	// returns no usable wrapper for the input (common when feeding a grant
	// number into an application-oriented API).
	ErrUSPTOApplicationNotFound = fmt.Errorf("crawl/uspto: no application data found for number (grant number may not map cleanly or wrapper missing): %w", ErrNotAvailable)

	// ErrUSPTOGrantDocumentNotFound indicates we resolved an application
	// record but the grant document metadata is absent. The patent can still
	// be shown/used with whatever data we have; later enrichment is possible.
	ErrUSPTOGrantDocumentNotFound = errors.New("crawl/uspto: grant document meta not present (only application or pre-grant data available)")
)

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
	sourceMode domain.SourceMode
	modeMu     sync.RWMutex
	metrics    *observability.Metrics
	logger     *slog.Logger
}

// NewRegistry builds a Registry from sources, consulted in the given order.
func NewRegistry(sources ...Source) *Registry {
	return &Registry{sources: sources, sourceMode: SourceModeCompare}
}

// WithGoogleComparison enables best-effort Google fetches after a successful
// USPTO fetch. The USPTO result remains authoritative; Google only produces
// source_diff rows.
func (r *Registry) WithGoogleComparison(enabled bool) *Registry {
	if enabled {
		_ = r.SetSourceMode(string(SourceModeCompare))
	} else {
		_ = r.SetSourceMode(string(SourceModeUSPTOFirst))
	}
	return r
}

// WithSourceMode sets the initial runtime provider mode. Invalid values leave
// the default in place; config validation should catch them before startup.
func (r *Registry) WithSourceMode(mode string) *Registry {
	_ = r.SetSourceMode(mode)
	return r
}

// Re-exports of the canonical source-mode names so crawl callers do not
// need to import the domain package just to name a mode. The values are
// domain.SourceMode (a typed string) rather than bare strings, so a typo
// is a compile error.
const (
	SourceModeCompare    = domain.SourceModeCompare
	SourceModeUSPTOFirst = domain.SourceModeUSPTOFirst
	SourceModeUSPTOOnly  = domain.SourceModeUSPTOOnly
	SourceModeGoogleOnly = domain.SourceModeGoogleOnly
)

// NormalizeSourceMode validates a user/config supplied source mode.
// An empty value resolves to compare.
func NormalizeSourceMode(mode string) (domain.SourceMode, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", string(SourceModeCompare):
		return SourceModeCompare, nil
	case string(SourceModeUSPTOFirst):
		return SourceModeUSPTOFirst, nil
	case string(SourceModeUSPTOOnly):
		return SourceModeUSPTOOnly, nil
	case string(SourceModeGoogleOnly):
		return SourceModeGoogleOnly, nil
	default:
		return "", fmt.Errorf("crawl: unknown source mode %q (want %s, %s, %s, or %s)",
			mode, SourceModeCompare, SourceModeUSPTOFirst, SourceModeUSPTOOnly, SourceModeGoogleOnly)
	}
}

// SetSourceMode changes normal provider behavior at runtime.
func (r *Registry) SetSourceMode(mode string) error {
	normalized, err := NormalizeSourceMode(mode)
	if err != nil {
		return err
	}
	r.modeMu.Lock()
	r.sourceMode = normalized
	r.modeMu.Unlock()
	return nil
}

// SourceMode returns the current normal provider behavior.
func (r *Registry) SourceMode() string {
	r.modeMu.RLock()
	defer r.modeMu.RUnlock()
	return string(r.sourceMode)
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
	mode := domain.SourceMode(r.SourceMode())

	type fetchAttempt struct {
		source string
		err    error
		dur    time.Duration
	}
	var attempts []fetchAttempt
	lastErr := error(ErrNotAvailable)

	for _, s := range r.sources {
		// Honor source-mode exclusions: uspto-only blocks Google,
		// google-only blocks USPTO. File source is always considered
		// (it is the local cache, not a network provider).
		switch s.Name() {
		case domain.SourceGoogle:
			if mode == SourceModeUSPTOOnly {
				continue
			}
		case domain.SourceUSPTO:
			if mode == SourceModeGoogleOnly {
				continue
			}
		}
		if slices.Contains(exclude, s.Name()) {
			continue
		}
		sourceStart := time.Now()
		res, err := s.Fetch(ctx, number)
		d := time.Since(sourceStart)

		attempts = append(attempts, fetchAttempt{
			source: string(s.Name()),
			err:    err,
			dur:    d,
		})

		if r.metrics != nil {
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
			if mode == SourceModeCompare {
				if s.Name() == domain.SourceUSPTO {
					res = r.compareWithGoogle(ctx, number, res, exclude...)
				} else {
					// Best-effort secondary USPTO enrichment when Google (or another source)
					// succeeded first in compare mode. This populates the uspto_application
					// row (and related data) needed for later grant XML fetches, even if
					// the primary bibliographic data came from Google.
					res = r.enrichWithUSPTO(ctx, number, res, exclude...)
				}
			}
			return res, nil
		}
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		if errors.Is(err, ErrNotAvailable) {
			if r.metrics != nil {
				r.metrics.IncCounter("crawl.source."+string(s.Name())+".not_available_total", 1)
			}
		} else if err != nil {
			if r.metrics != nil {
				r.metrics.IncCounter("crawl.source."+string(s.Name())+".hard_error_total", 1)
			}
			lastErr = err
			failed = true
		}
	}
	failed = true

	// Build richer error with per-source details (for better logging, telemetry,
	// and user-facing messages per the approved plan).
	if len(attempts) > 0 {
		var details []string
		for _, a := range attempts {
			if a.err != nil {
				details = append(details, fmt.Sprintf("%s: %v (%.2fs)", a.source, a.err, a.dur.Seconds()))
			}
		}
		if len(details) > 0 {
			if r.metrics != nil {
				r.metrics.IncCounter("crawl.registry.all_sources_failed_total", 1)
			}
			return Result{}, fmt.Errorf("all sources failed for %s: %w; details: [%s]", number, lastErr, strings.Join(details, "; "))
		}
	}

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
		compStart := time.Now()
		google, err := s.Fetch(ctx, number)
		compDur := time.Since(compStart)

		if r.metrics != nil {
			r.metrics.ObserveDuration("crawl.source.google.comparison_fetch", compDur, err != nil)
			r.metrics.IncCounter("crawl.source.google.comparison_attempted_total", 1)
			if err != nil {
				r.metrics.IncCounter("crawl.source.google.comparison_failed_total", 1)
			}
		}

		if err != nil {
			if r.logger != nil && !errors.Is(err, ErrNotAvailable) && ctx.Err() == nil {
				r.logger.Warn("google comparison fetch failed",
					slog.String("number", number.String()),
					slog.String("error", err.Error()),
					slog.Duration("duration", compDur))
			}
			return uspto
		}

		if r.metrics != nil {
			r.metrics.IncCounter("crawl.source.google.comparison_diffs_generated_total", int64(len(comparePatentFields(uspto.Patent, google.Patent))))
		}

		uspto.SourceSnapshots = append(uspto.SourceSnapshots, google.SourceSnapshots...)
		uspto.SourceDiffs = append(uspto.SourceDiffs, comparePatentFields(uspto.Patent, google.Patent)...)
		return uspto
	}
	return uspto
}

// enrichWithUSPTO is the symmetric counterpart to compareWithGoogle.
// When a non-USPTO source (typically Google in compare mode) provides the
// primary bibliographic data, we still attempt a best-effort secondary fetch
// from USPTO. This populates the critical uspto_application row (and related
// parties/events/continuities) needed for later grant XML downloads via
// FetchUSPTOXML, without failing the overall fetch.
func (r *Registry) enrichWithUSPTO(ctx context.Context, number domain.PatentNumber, primary Result, exclude ...domain.Source) Result {
	if slices.Contains(exclude, domain.SourceUSPTO) {
		return primary
	}
	for _, s := range r.sources {
		if s.Name() != domain.SourceUSPTO {
			continue
		}
		enrichStart := time.Now()
		uspto, err := s.Fetch(ctx, number)
		enrichDur := time.Since(enrichStart)

		if r.metrics != nil {
			r.metrics.ObserveDuration("crawl.source.uspto.enrichment_fetch", enrichDur, err != nil)
			r.metrics.IncCounter("crawl.source.uspto.enrichment_attempted_total", 1)
			if err != nil {
				r.metrics.IncCounter("crawl.source.uspto.enrichment_failed_total", 1)
			}
		}

		if err != nil {
			if r.logger != nil && !errors.Is(err, ErrNotAvailable) && ctx.Err() == nil {
				r.logger.Debug("best-effort USPTO enrichment failed (non-fatal)",
					slog.String("number", number.String()),
					slog.String("error", err.Error()),
					slog.Duration("duration", enrichDur))
			}
			return primary
		}

		// Merge the valuable USPTO-only data (especially the application record
		// needed for grant XML URLs).
		if uspto.USPTOApplication != nil {
			primary.USPTOApplication = uspto.USPTOApplication
		}
		primary.USPTOParties = append(primary.USPTOParties, uspto.USPTOParties...)
		primary.USPTOEvents = append(primary.USPTOEvents, uspto.USPTOEvents...)
		primary.USPTOContinuities = append(primary.USPTOContinuities, uspto.USPTOContinuities...)
		primary.USPTOForeignPriority = append(primary.USPTOForeignPriority, uspto.USPTOForeignPriority...)

		// Also bring in any source snapshots from the enrichment for auditability.
		primary.SourceSnapshots = append(primary.SourceSnapshots, uspto.SourceSnapshots...)

		// Generate diffs (Google as primary vs this USPTO enrichment).
		// Default choice = USPTO per user preference for the comparison UI.
		diffs := comparePatentFields(primary.Patent, uspto.Patent)
		for i := range diffs {
			diffs[i].ChosenValue = diffs[i].USPTOValue // default to USPTO
			diffs[i].ChosenSource = string(domain.SourceUSPTO)
		}
		primary.SourceDiffs = append(primary.SourceDiffs, diffs...)

		if r.logger != nil {
			r.logger.Info("best-effort USPTO enrichment succeeded",
				slog.String("number", number.String()),
				slog.Duration("duration", enrichDur),
				slog.Bool("had_application", primary.USPTOApplication != nil))
		}

		return primary
	}
	return primary
}

func comparePatentFields(uspto, google domain.Patent) []domain.SourceDiff {
	now := time.Now().UTC().Format(time.RFC3339)

	var diffs []domain.SourceDiff
	for _, f := range domain.ReconciliableFields {
		left := strings.TrimSpace(f.Get(&uspto))
		right := strings.TrimSpace(f.Get(&google))
		if left == right || (left == "" && right == "") {
			continue
		}
		idSeed := uspto.Number.Normalized() + ":" + f.Path + ":" + left + ":" + right
		diffs = append(diffs, domain.SourceDiff{
			ID:           "diff:" + payloadHash([]byte(idSeed))[:24],
			PatentNumber: uspto.Number,
			FieldPath:    f.Path,
			USPTOValue:   left,
			GoogleValue:  right,
			ChosenValue:  left, // default to USPTO per long-standing preference
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
