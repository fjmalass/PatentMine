// Package engine is the daemon core. It owns the store, the crawl worker
// pool, and the event bus, and exposes the operations that frontends invoke
// (directly in tests, or over RPC in the running system).
package engine

import (
	"context"
	"io"
	"log/slog"
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
// the crawl bypass the local file cache and re-fetch from the web.
type CrawlFactory func(root domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool) Job

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
	fileImporter     FileImporter
	cpcLookup        CPCLookup
	logger           *slog.Logger
	activities       *observability.Recorder
	metrics          *observability.Metrics
	crawlMaxDepth    int
	crawlWorkerCount int

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
