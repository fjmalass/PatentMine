// Package observability tests. These checks verify that the observability
// package creates dated JSONL files and tags records with the expected date and
// component fields.
package observability

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenCreatesDatedLogAndActivityFiles(t *testing.T) {
	logsDir := filepath.Join(t.TempDir(), "logs")
	obs, err := Open(logsDir, "daemon", "test-version")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = obs.Close() })

	obs.Logger.Info("daemon started")
	if err := obs.Activity.Record(context.Background(), Record{
		Action:   "project.create",
		Entity:   "project",
		EntityID: "p-1",
		Status:   "committed",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	date := time.Now().In(time.Local).Format(dateLayout)
	logPath := filepath.Join(logsDir, "log-"+date+".jsonl")
	activityPath := filepath.Join(logsDir, "activity-"+date+".jsonl")
	for _, path := range []string{logPath, activityPath} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("Stat(%s): %v", path, err)
		}
	}
	logBody, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile log: %v", err)
	}
	if !strings.Contains(string(logBody), `"component":"daemon"`) ||
		!strings.Contains(string(logBody), `"version":"test-version"`) ||
		!strings.Contains(string(logBody), `"date":"`+date+`"`) {
		t.Fatalf("log body missing component/date tags: %s", logBody)
	}
	activityBody, err := os.ReadFile(activityPath)
	if err != nil {
		t.Fatalf("ReadFile activity: %v", err)
	}
	if !strings.Contains(string(activityBody), `"action":"project.create"`) ||
		!strings.Contains(string(activityBody), `"date":"`+date+`"`) {
		t.Fatalf("activity body missing action/date tags: %s", activityBody)
	}
}

func TestPrometheusTextRendersDerivedMetrics(t *testing.T) {
	snap := Snapshot{
		Timestamp: time.Now(),
		Timings: map[string]TimingSummary{
			"rpc.method.ping": {Count: 2, TotalNanos: 20, MinNanos: 5, MaxNanos: 15, LastNanos: 15},
		},
		Counters: map[string]int64{"engine.bus.drop_total": 3},
		Gauges:   map[string]int64{"engine.bus.subscribers": 1},
	}
	body := PrometheusText(snap)
	for _, want := range []string{"rpc_method_ping_count 2", "rpc_method_ping_avg_nanos 10", "engine_bus_drop_total 3", "engine_bus_subscribers 1"} {
		if !strings.Contains(body, want) {
			t.Fatalf("prometheus body missing %q: %s", want, body)
		}
	}
}

func TestReadActivityRecordsHandlesCorruptedLines(t *testing.T) {
	logsDir := t.TempDir()
	date := time.Now().In(time.Local).Format(dateLayout)
	activityPath := filepath.Join(logsDir, "activity-"+date+".jsonl")

	// Write a mix of valid, blank, and corrupted (half-written) JSON entries
	lines := []string{
		`{"id":"1","timestamp":"2026-05-23T20:00:00Z","action":"filter.apply","entity":"filter","entity_id":"cpc:G06F","status":"requested"}`,
		``, // empty line
		`{"id":"2","timestamp":"2026-05-23T20:01:00Z","action":"project.switch"`, // Corrupted (unexpected end of JSON input)
		`{"id":"3","timestamp":"2026-05-23T20:02:00Z","action":"ui.focus","entity":"patent","entity_id":"US10000000B2","status":"observed"}`,
	}
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(activityPath, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Read the records
	records, err := ReadActivityRecords(logsDir, ActivityQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ReadActivityRecords failed: %v", err)
	}

	// It should skip the empty and corrupted lines, returning only the 2 valid records
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d: %+v", len(records), records)
	}

	// Order is newest-first (descending timestamp/log lines order)
	if records[0].ID != "3" || records[1].ID != "1" {
		t.Errorf("unexpected record order or content: %+v", records)
	}
}

