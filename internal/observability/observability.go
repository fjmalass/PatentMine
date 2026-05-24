// Package observability owns structured process logs and semantic activity
// journaling. It writes dated JSONL files, keeps activity records replayable for
// future undo/redo tooling, and exposes context-carried trace fields so real
// OpenTelemetry spans can be attached later without changing callers.
package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var recordSeq atomic.Uint64

// logLevel reads LOG_LEVEL from the environment and returns the matching slog
// level. Unset or unrecognised values default to Info.
func logLevel() slog.Level {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL"))) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

const dateLayout = "2006-01-02"

type ctxKey string

const (
	traceIDKey ctxKey = "trace_id"
	spanIDKey  ctxKey = "span_id"
)

// Runtime owns structured log and activity sinks for one process.
type Runtime struct {
	Logger   *slog.Logger
	Activity *Recorder
	Metrics  *Metrics
	Version  string
	LogsDir  string

	logFile      *os.File
	activityFile *os.File
}

// Snapshot is the current in-memory metrics view.
type Snapshot struct {
	Timestamp time.Time                `json:"timestamp"`
	Timings   map[string]TimingSummary `json:"timings"`
	Counters  map[string]int64         `json:"counters"`
	Gauges    map[string]int64         `json:"gauges"`
}

// TimingSummary is one duration aggregate.
type TimingSummary struct {
	Count      int64 `json:"count"`
	Errors     int64 `json:"errors"`
	TotalNanos int64 `json:"total_nanos"`
	MinNanos   int64 `json:"min_nanos"`
	MaxNanos   int64 `json:"max_nanos"`
	LastNanos  int64 `json:"last_nanos"`
}

// AvgNanos returns the arithmetic mean duration.
func (t TimingSummary) AvgNanos() int64 {
	if t.Count == 0 {
		return 0
	}
	return t.TotalNanos / t.Count
}

// PrometheusText renders a snapshot as Prometheus exposition text.
func PrometheusText(s Snapshot) string {
	var b strings.Builder
	writeMetric := func(typeName, name string, value int64) {
		fmt.Fprintf(&b, "# TYPE %s %s\n%s %d\n", name, typeName, name, value)
	}
	names := sortedKeys(s.Timings)
	for _, name := range names {
		metric := s.Timings[name]
		base := sanitizePromName(name)
		writeMetric("counter", base+"_count", metric.Count)
		writeMetric("counter", base+"_errors", metric.Errors)
		writeMetric("counter", base+"_total_nanos", metric.TotalNanos)
		writeMetric("gauge", base+"_avg_nanos", metric.AvgNanos())
		writeMetric("gauge", base+"_min_nanos", metric.MinNanos)
		writeMetric("gauge", base+"_max_nanos", metric.MaxNanos)
		writeMetric("gauge", base+"_last_nanos", metric.LastNanos)
	}
	for _, name := range sortedKeys(s.Counters) {
		writeMetric("counter", sanitizePromName(name), s.Counters[name])
	}
	for _, name := range sortedKeys(s.Gauges) {
		writeMetric("gauge", sanitizePromName(name), s.Gauges[name])
	}
	return b.String()
}

// Metrics is a lightweight in-process recorder for counters, gauges, and
// boundary durations. It is intentionally simple: phase 1 needs immediate
// timing visibility without a collector dependency.
type Metrics struct {
	mu       sync.RWMutex
	timings  map[string]*TimingSummary
	counters map[string]int64
	gauges   map[string]int64
}

// Record is one semantic activity event. It is written as JSONL so future
// replay, undo/redo, or export tooling can stream it efficiently.
type Record struct {
	ID        string         `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Date      string         `json:"date"`
	Component string         `json:"component"`
	Action    string         `json:"action"`
	Entity    string         `json:"entity"`
	EntityID  string         `json:"entity_id,omitempty"`
	Status    string         `json:"status"`
	TraceID   string         `json:"trace_id,omitempty"`
	SpanID    string         `json:"span_id,omitempty"`
	Before    any            `json:"before,omitempty"`
	After     any            `json:"after,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// Recorder appends JSONL activity events to a file.
type Recorder struct {
	mu        sync.Mutex
	component string
	w         io.Writer
}

// logRetainDays is how many days of log/activity files to keep on startup.
const logRetainDays = 14

// pruneOldLogs deletes log-*.jsonl and activity-*.jsonl files in logsDir
// whose date suffix is older than logRetainDays days.
func pruneOldLogs(logsDir string, now time.Time) {
	cutoff := now.In(time.Local).AddDate(0, 0, -logRetainDays)
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		var prefix string
		switch {
		case strings.HasPrefix(name, "log-") && strings.HasSuffix(name, ".jsonl"):
			prefix = "log-"
		case strings.HasPrefix(name, "activity-") && strings.HasSuffix(name, ".jsonl"):
			prefix = "activity-"
		default:
			continue
		}
		dateStr := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".jsonl")
		t, err := time.ParseInLocation(dateLayout, dateStr, time.Local)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			_ = os.Remove(filepath.Join(logsDir, name))
		}
	}
}

// Open creates dated log and activity files under logsDir.
func Open(logsDir, component, buildVersion string) (*Runtime, error) {
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, fmt.Errorf("observability: create logs dir %q: %w", logsDir, err)
	}
	now := time.Now()
	pruneOldLogs(logsDir, now)
	date := localDate(now)
	logFile, err := os.OpenFile(filepath.Join(logsDir, "log-"+date+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("observability: open log file: %w", err)
	}
	activityFile, err := os.OpenFile(filepath.Join(logsDir, "activity-"+date+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("observability: open activity file: %w", err)
	}
	handler := slog.NewJSONHandler(logFile, &slog.HandlerOptions{Level: logLevel()})
	logger := slog.New(handler).With(
		slog.String("service", "patentmine"),
		slog.String("component", component),
		slog.String("date", date),
		slog.String("version", buildVersion),
	)
	return &Runtime{
		Logger:       logger,
		Activity:     &Recorder{component: component, w: activityFile},
		Metrics:      NewMetrics(),
		Version:      buildVersion,
		LogsDir:      logsDir,
		logFile:      logFile,
		activityFile: activityFile,
	}, nil
}

// NewMetrics builds an empty in-memory metrics registry.
func NewMetrics() *Metrics {
	return &Metrics{
		timings:  make(map[string]*TimingSummary),
		counters: make(map[string]int64),
		gauges:   make(map[string]int64),
	}
}

// Close flushes and closes the runtime sinks.
func (r *Runtime) Close() error {
	var err error
	if r.logFile != nil {
		err = r.logFile.Close()
	}
	if r.activityFile != nil {
		if cerr := r.activityFile.Close(); err == nil {
			err = cerr
		}
	}
	return err
}

// Record writes one semantic activity event.
func (r *Recorder) Record(ctx context.Context, rec Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now().UTC()
	}
	if rec.Date == "" {
		rec.Date = localDate(rec.Timestamp)
	}
	if rec.Component == "" {
		rec.Component = r.component
	}
	if rec.ID == "" {
		rec.ID = fmt.Sprintf("%s-%d-%d", rec.Date, rec.Timestamp.UnixNano(), recordSeq.Add(1))
	}
	if rec.TraceID == "" || rec.SpanID == "" {
		traceID, spanID := TraceIDs(ctx)
		if rec.TraceID == "" {
			rec.TraceID = traceID
		}
		if rec.SpanID == "" {
			rec.SpanID = spanID
		}
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("observability: marshal activity: %w", err)
	}
	if _, err := r.w.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("observability: write activity: %w", err)
	}
	return nil
}

// ObserveDuration records one completed operation duration.
func (m *Metrics) ObserveDuration(name string, d time.Duration, failed bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	metric := m.timings[name]
	if metric == nil {
		metric = &TimingSummary{MinNanos: d.Nanoseconds(), MaxNanos: d.Nanoseconds()}
		m.timings[name] = metric
	}
	ns := d.Nanoseconds()
	metric.Count++
	metric.LastNanos = ns
	metric.TotalNanos += ns
	if metric.Count == 1 || ns < metric.MinNanos {
		metric.MinNanos = ns
	}
	if ns > metric.MaxNanos {
		metric.MaxNanos = ns
	}
	if failed {
		metric.Errors++
	}
}

// IncCounter adds delta to a named counter.
func (m *Metrics) IncCounter(name string, delta int64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name] += delta
}

// SetGauge sets a named gauge.
func (m *Metrics) SetGauge(name string, value int64) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gauges[name] = value
}

// Snapshot returns a copy safe for JSON responses.
func (m *Metrics) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{Timestamp: time.Now().UTC(), Timings: map[string]TimingSummary{}, Counters: map[string]int64{}, Gauges: map[string]int64{}}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	timings := make(map[string]TimingSummary, len(m.timings))
	for k, v := range m.timings {
		timings[k] = *v
	}
	counters := make(map[string]int64, len(m.counters))
	for k, v := range m.counters {
		counters[k] = v
	}
	gauges := make(map[string]int64, len(m.gauges))
	for k, v := range m.gauges {
		gauges[k] = v
	}
	return Snapshot{Timestamp: time.Now().UTC(), Timings: timings, Counters: counters, Gauges: gauges}
}

func localDate(t time.Time) string {
	return t.In(time.Local).Format(dateLayout)
}

func sanitizePromName(name string) string {
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, "/", "_")
	return name
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// WithTraceIDs stores trace metadata in a context without taking an OTel dependency.
func WithTraceIDs(ctx context.Context, traceID, spanID string) context.Context {
	ctx = context.WithValue(ctx, traceIDKey, traceID)
	return context.WithValue(ctx, spanIDKey, spanID)
}

// TraceIDs returns trace metadata previously associated with ctx.
func TraceIDs(ctx context.Context) (traceID, spanID string) {
	if v, ok := ctx.Value(traceIDKey).(string); ok {
		traceID = v
	}
	if v, ok := ctx.Value(spanIDKey).(string); ok {
		spanID = v
	}
	return traceID, spanID
}

// WithContextAttrs adds future trace/span fields from ctx to the logger.
func WithContextAttrs(ctx context.Context, logger *slog.Logger) *slog.Logger {
	traceID, spanID := TraceIDs(ctx)
	if traceID == "" && spanID == "" {
		return logger
	}
	attrs := make([]any, 0, 4)
	if traceID != "" {
		attrs = append(attrs, slog.String("trace_id", traceID))
	}
	if spanID != "" {
		attrs = append(attrs, slog.String("span_id", spanID))
	}
	return logger.With(attrs...)
}
