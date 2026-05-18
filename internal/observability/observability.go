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
	"sync"
	"time"
)

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

	logFile      *os.File
	activityFile *os.File
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

// Open creates dated log and activity files under logsDir.
func Open(logsDir, component string) (*Runtime, error) {
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return nil, fmt.Errorf("observability: create logs dir %q: %w", logsDir, err)
	}
	date := localDate(time.Now())
	logFile, err := os.OpenFile(filepath.Join(logsDir, "log-"+date+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("observability: open log file: %w", err)
	}
	activityFile, err := os.OpenFile(filepath.Join(logsDir, "activity-"+date+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("observability: open activity file: %w", err)
	}
	handler := slog.NewJSONHandler(logFile, nil)
	logger := slog.New(handler).With(
		slog.String("service", "patentmine"),
		slog.String("component", component),
		slog.String("date", date),
	)
	return &Runtime{
		Logger:       logger,
		Activity:     &Recorder{component: component, w: activityFile},
		logFile:      logFile,
		activityFile: activityFile,
	}, nil
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
		rec.ID = fmt.Sprintf("%s-%d", rec.Date, rec.Timestamp.UnixNano())
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

func localDate(t time.Time) string {
	return t.In(time.Local).Format(dateLayout)
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
