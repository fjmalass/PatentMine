// Package engine is the daemon core. It owns the store, the crawl worker
// pool, and the event bus, and exposes the operations that frontends invoke
// (directly in tests, or over RPC in the running system).
package engine

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/store"
)

// crawlWorkers is the number of concurrent crawl jobs.
const crawlWorkers = 4

// changeDebounce is the coalescing window for EventDBChanged. A burst of writes
// — most acutely a family crawl saving thousands of rows — collapses to roughly
// one event per window, so every connected client reloads a handful of times
// instead of thousands.
const changeDebounce = 100 * time.Millisecond

// CrawlFactory builds the Job that crawls a patent family. It is injected so
// the engine does not depend on the crawl package directly: a stub factory
// works for tests, the real crawler is wired in at daemon startup. force makes
// the crawl bypass the local file cache and re-fetch from the web. Source, when
// non-empty, constrains the crawl to that provider and disables fallback.
type CrawlFactory func(root domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool, source domain.Source) Job

// SourceModeController owns the runtime crawl source policy.
type SourceModeController interface {
	SetSourceMode(mode string) error
	SourceMode() string
}

// USPTOSearcher searches the USPTO database for candidates matching the number.
type USPTOSearcher func(ctx context.Context, number domain.PatentNumber) ([]domain.USPTOCandidate, error)

// USPTOAssignmentFetcher returns recorded assignments for one application
// number. Wired by the daemon so engine does not import the crawl package
// for HTTP work.
type USPTOAssignmentFetcher func(ctx context.Context, apiKey, applicationNumber string) ([]domain.USPTOAssignment, error)

// FileImporter loads a patent record from a local file into the store. Like
// CrawlFactory it is injected, so the engine never imports the crawl package.
type FileImporter interface {
	ImportFile(ctx context.Context, path string) error
}

// CPCLookup fetches classification descriptions from an external service.
type CPCLookup func(ctx context.Context, code string) (string, error)

// Option customizes an Engine.
type Option func(*Engine)

// Engine is the daemon core. Its methods are the single set of operations the
// system supports; RPC handlers and embedded callers both go through them.
type Engine struct {
	repo             store.Repository
	bus              *Bus
	pool             *workerPool
	crawl            CrawlFactory
	sourceModes      SourceModeController
	fileImporter     FileImporter
	cpcLookup        CPCLookup
	usptoSearcher    USPTOSearcher
	usptoAssignments USPTOAssignmentFetcher
	logger           *slog.Logger
	activities       *observability.Recorder
	metrics          *observability.Metrics
	crawlMaxDepth    int
	crawlWorkerCount int
	idsExportDir     string
	usptoAPIKey      string
	patentsDir       string

	// changeMu guards the EventDBChanged debounce state below.
	changeMu      sync.Mutex
	changeTimer   *time.Timer
	changePending bool
}

// WithFileImporter wires the file-import backend used by ImportFile.
func WithFileImporter(fi FileImporter) Option {
	return func(e *Engine) { e.fileImporter = fi }
}

// WithCPCLookup wires the CPC lookup function used by LookupClassification.
func WithCPCLookup(l CPCLookup) Option {
	return func(e *Engine) { e.cpcLookup = l }
}

// WithLogger records structured logs for engine operations.
func WithLogger(logger *slog.Logger) Option {
	return func(e *Engine) {
		if logger != nil {
			e.logger = logger
		}
	}
}

// WithActivityRecorder enables semantic activity journaling.
func WithActivityRecorder(rec *observability.Recorder) Option {
	return func(e *Engine) {
		e.activities = rec
	}
}

// WithMetrics enables in-process timings and counters.
func WithMetrics(metrics *observability.Metrics) Option {
	return func(e *Engine) {
		e.metrics = metrics
	}
}

// WithSourceModeController wires runtime source mode control and records the
// startup mode in metrics/logging once the engine is assembled.
func WithSourceModeController(controller SourceModeController) Option {
	return func(e *Engine) {
		e.sourceModes = controller
		e.observeSourceMode(controller.SourceMode())
		e.log(context.Background(), slog.LevelInfo, "source mode configured", slog.String("mode", controller.SourceMode()))
	}
}

// WithCrawlMaxDepth records the daemon's default family-crawl depth.
func WithCrawlMaxDepth(maxDepth int) Option {
	return func(e *Engine) {
		e.crawlMaxDepth = maxDepth
	}
}

// WithCrawlWorkers sets the number of concurrent crawl goroutines.
// Values ≤ 0 are ignored; the default is crawlWorkers (4).
func WithCrawlWorkers(n int) Option {
	return func(e *Engine) {
		if n > 0 {
			e.crawlWorkerCount = n
		}
	}
}

// WithIDSExportDir sets the base directory the IDS PDF exporter writes into.
// Empty leaves it unset; ExportIDSPDF will then return an error when called.
func WithIDSExportDir(dir string) Option {
	return func(e *Engine) { e.idsExportDir = dir }
}

// WithUSPTOSearcher wires the USPTO candidate searcher.
func WithUSPTOSearcher(s USPTOSearcher) Option {
	return func(e *Engine) { e.usptoSearcher = s }
}

// WithUSPTOAssignmentFetcher wires the Patent Assignment Search backend used
// by FetchUSPTOAssignments. Nil leaves the engine with no assignment fetch
// capability; the RPC returns an error in that case.
func WithUSPTOAssignmentFetcher(f USPTOAssignmentFetcher) Option {
	return func(e *Engine) { e.usptoAssignments = f }
}

// WithUSPTOAPIKey supplies the API key used when downloading USPTO XML files.
func WithUSPTOAPIKey(key string) Option {
	return func(e *Engine) { e.usptoAPIKey = key }
}

// WithPatentsDir names the on-disk directory XML downloads are written into.
func WithPatentsDir(dir string) Option {
	return func(e *Engine) { e.patentsDir = dir }
}

// New builds an Engine. The pool's jobs are children of ctx, so cancelling ctx
// stops all background work. crawl may be nil if no crawl will be started.
func New(ctx context.Context, repo store.Repository, crawl CrawlFactory, opts ...Option) *Engine {
	bus := NewBus()
	eng := &Engine{
		repo:             repo,
		bus:              bus,
		crawl:            crawl,
		logger:           slog.New(slog.NewJSONHandler(io.Discard, nil)),
		crawlWorkerCount: crawlWorkers,
	}
	for _, opt := range opts {
		opt(eng)
	}
	eng.pool = newWorkerPool(ctx, eng.crawlWorkerCount, bus, nil)
	eng.pool.metrics = eng.metrics
	eng.bus.metrics = eng.metrics
	return eng
}

// Close stops the worker pool and the event bus. The store is not closed here:
// its lifetime is owned by whoever opened it.
func (e *Engine) Close() {
	e.pool.shutdown()
	e.bus.Close()
	e.changeMu.Lock()
	if e.changeTimer != nil {
		e.changeTimer.Stop()
		e.changeTimer = nil
	}
	e.changeMu.Unlock()
}

// Subscribe registers an event subscriber; the returned function unsubscribes.
func (e *Engine) Subscribe() (<-chan proto.Event, func()) {
	return e.bus.Subscribe()
}

// announceChange notifies subscribers that stored data changed. Calls are
// debounced over changeDebounce: the first call in an idle period publishes
// immediately (leading edge), and any calls during the window collapse into a
// single trailing publish when it closes. A crawl that writes thousands of
// rows therefore triggers a few client reloads, not thousands.
func (e *Engine) announceChange() {
	e.changeMu.Lock()
	defer e.changeMu.Unlock()
	if e.changeTimer != nil {
		e.changePending = true
		return
	}
	e.publishChange()
	e.changeTimer = time.AfterFunc(changeDebounce, e.onChangeWindowClosed)
}

// onChangeWindowClosed fires when a debounce window ends. It emits one trailing
// event when writes arrived during the window and reopens the window; otherwise
// it clears the timer so the next change publishes on the leading edge.
func (e *Engine) onChangeWindowClosed() {
	e.changeMu.Lock()
	defer e.changeMu.Unlock()
	if e.changePending {
		e.changePending = false
		e.publishChange()
		e.changeTimer.Reset(changeDebounce)
		return
	}
	e.changeTimer = nil
}

func (e *Engine) publishChange() {
	e.bus.Publish(proto.NewEvent(proto.EventDBChanged, proto.Empty{}))
}

func (e *Engine) existingPatent(ctx context.Context, n domain.PatentNumber) (domain.Patent, bool) {
	p, err := e.repo.Patent(ctx, n)
	if err == nil {
		return p, true
	}
	return domain.Patent{}, false
}

func (e *Engine) existingMembership(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (domain.Membership, bool) {
	m, err := e.repo.Membership(ctx, project, patent)
	if err == nil {
		return m, true
	}
	return domain.Membership{}, false
}

// USPTOApplication returns the saved USPTO application details, or ErrNotFound.
func (e *Engine) USPTOApplication(ctx context.Context, n domain.PatentNumber) (domain.USPTOApplication, error) {
	return e.repo.USPTOApplication(ctx, n)
}

func (e *Engine) log(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	logger := observability.WithContextAttrs(ctx, e.logger)
	args := make([]any, 0, len(attrs))
	for _, attr := range attrs {
		args = append(args, attr)
	}
	logger.Log(ctx, level, msg, args...)
}

func (e *Engine) recordActivity(ctx context.Context, rec observability.Record) {
	if e.activities == nil {
		return
	}
	if err := e.activities.Record(ctx, rec); err != nil {
		e.log(ctx, slog.LevelWarn, "activity record failed", slog.String("action", rec.Action), slog.String("error", err.Error()))
	}
}

func (e *Engine) incCounter(name string, delta int64) {
	if e.metrics == nil {
		return
	}
	e.metrics.IncCounter(name, delta)
}

func (e *Engine) observeDuration(name string, start time.Time, errp *error) {
	if e.metrics == nil {
		return
	}
	failed := errp != nil && *errp != nil
	e.metrics.ObserveDuration(name, time.Since(start), failed)
}

// ResolveSourceDiffs applies user reconciliation choices from the source
// comparison overlay (Option A). It updates the core patent fields from the
// ChosenValue of each diff (title, abstract, inventors, dates, first_claim,
// classifications, assignee), saves the patent, then marks the corresponding
// SourceDiff rows as reconciled (ReconciledAt/By/Choice + final Chosen) for
// provenance. This makes the overlay choices durable while keeping the
// patent table as the single "current" view and source_* tables for audit.
func (e *Engine) ResolveSourceDiffs(ctx context.Context, patentNum domain.PatentNumber, diffs []domain.SourceDiff) error {
	var err error
	defer e.observeDuration("engine.source.resolve_diffs", time.Now(), &err)

	if patentNum.IsZero() {
		return fmt.Errorf("engine: resolve diffs: zero patent number")
	}
	if len(diffs) == 0 {
		return nil
	}

	p, err := e.repo.Patent(ctx, patentNum)
	if err != nil {
		return fmt.Errorf("engine: resolve diffs: load patent: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	reconciledBy := "user" // future: derive from ctx/identity when auth exists

	// Apply chosen values back to the patent (reverse of comparePatentFields
	// stringification in crawl/source.go).
	for _, d := range diffs {
		val := strings.TrimSpace(d.ChosenValue)
		if val == "" {
			continue
		}

		applied := false
		for _, f := range domain.ReconciliableFields {
			if f.Path == d.FieldPath {
				f.Set(&p, val) // Set already takes *Patent (as does Get)
				applied = true
				break
			}
		}
		if !applied {
			// Unknown or future field; ignore for now (safe).
			if e.logger != nil {
				e.log(ctx, slog.LevelDebug, "resolve diffs: unknown field path",
					slog.String("field", d.FieldPath))
			}
		}
	}

	// Persist the updated patent (this becomes the new "current" reconciled view).
	if err = e.repo.SavePatent(ctx, p); err != nil {
		return fmt.Errorf("engine: resolve diffs: save patent: %w", err)
	}

	// Mark the diffs as reconciled and persist the choice metadata.
	// We reuse the NodeBatch path (saveSourceData handles ON CONFLICT update
	// of the reconciled_* columns and Chosen*).
	nowForDiffs := now
	for i := range diffs {
		diffs[i].ReconciledAt = nowForDiffs
		diffs[i].ReconciledBy = reconciledBy
		diffs[i].ReconciledChoice = diffs[i].ChosenSource
		// Ensure ChosenValue/ChosenSource are the final user decision.
		// (They were already set by the overlay before calling resolve.)
	}
	batch := store.NodeBatch{SourceDiffs: diffs}
	if err = e.repo.SaveNode(ctx, batch); err != nil {
		return fmt.Errorf("engine: resolve diffs: save reconciled diffs: %w", err)
	}

	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionSourceResolveDiffs,
		Entity:   "patent",
		EntityID: patentNum.String(),
		Status:   "committed",
		Attributes: map[string]any{
			"diff_count": len(diffs),
			"by":         reconciledBy,
		},
	})

	if e.metrics != nil {
		e.metrics.IncCounter("engine.source.resolve_diffs_total", 1)
		e.metrics.IncCounter("engine.source.resolve_diffs.diffs_resolved_total", int64(len(diffs)))
	}

	e.announceChange()

	e.log(ctx, slog.LevelInfo, "source diffs resolved",
		slog.String("patent", patentNum.String()),
		slog.Int("diff_count", len(diffs)),
		slog.String("reconciled_by", reconciledBy))

	return nil
}

// ListSourceDiffs is a thin pass-through for TUI / RPC (the real query lives in store).
func (e *Engine) ListSourceDiffs(ctx context.Context, n domain.PatentNumber) ([]domain.SourceDiff, error) {
	diffs, err := e.repo.ListSourceDiffs(ctx, n)
	if e.metrics != nil {
		e.metrics.IncCounter("engine.source.diffs.list_total", 1)
		if err != nil {
			e.metrics.IncCounter("engine.source.diffs.list_error_total", 1)
		} else {
			e.metrics.IncCounter("engine.source.diffs.list.diffs_returned_total", int64(len(diffs)))
		}
	}
	return diffs, err
}
