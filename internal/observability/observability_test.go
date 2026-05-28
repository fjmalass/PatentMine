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

func TestAdaptiveSizeAndConfigurablePruning(t *testing.T) {
	logsDir := t.TempDir()

	// Set LogRetainDays package variable dynamically
	originalDays := LogRetainDays
	originalSize := LogMaxSizeBytes
	t.Cleanup(func() {
		LogRetainDays = originalDays
		LogMaxSizeBytes = originalSize
	})

	LogRetainDays = 3
	LogMaxSizeBytes = 200 // Very small limit so it triggers size pruning

	now := time.Now()

	// Create some mock activity files
	dates := []string{
		now.AddDate(0, 0, -5).Format(dateLayout), // older than 3 days cutoff
		now.AddDate(0, 0, -2).Format(dateLayout), // within 3 days
		now.AddDate(0, 0, -1).Format(dateLayout), // within 3 days
		now.Format(dateLayout),                   // today
	}

	payload := "some dummy content that exceeds size limit\n" // ~40 bytes

	for _, d := range dates {
		path := filepath.Join(logsDir, "activity-"+d+".jsonl")
		if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", d, err)
		}
	}

	metrics := NewMetrics()

	// Trigger pruning
	pruneOldLogs(logsDir, now, nil, metrics)

	// Verify metrics for the first run (should delete dates[0], leaving 3 surviving files)
	snap := metrics.Snapshot()
	if snap.Counters["observability.pruning.total"] != 1 {
		t.Errorf("expected run counter to be 1, got %d", snap.Counters["observability.pruning.total"])
	}
	if snap.Counters["observability.pruning.deleted_files_total"] != 1 {
		t.Errorf("expected deleted files count to be 1, got %d", snap.Counters["observability.pruning.deleted_files_total"])
	}
	if snap.Gauges["observability.pruning.final_files"] != 3 {
		t.Errorf("expected final files gauge to be 3, got %d", snap.Gauges["observability.pruning.final_files"])
	}

	// Older than 3 days (dates[0]) should be deleted because of time-based pruning.
	if _, err := os.Stat(filepath.Join(logsDir, "activity-"+dates[0]+".jsonl")); !os.IsNotExist(err) {
		t.Errorf("expected oldest time-pruned file to be deleted, stat error: %v", err)
	}

	// But dates[1], dates[2], dates[3] are each ~40 bytes. Total is ~120 bytes.
	// Since LogMaxSizeBytes is 200, they fit.
	// Let's now reduce LogMaxSizeBytes to 50 bytes.
	// Now only one file can be kept! Since dates[3] is the newest, it should survive, and the older ones should be pruned oldest-first.
	LogMaxSizeBytes = 50

	// Write more files to trigger size pruning
	for _, d := range dates[1:] {
		path := filepath.Join(logsDir, "activity-"+d+".jsonl")
		// Write large payloads
		largePayload := strings.Repeat("A", 40) + "\n" // 41 bytes
		if err := os.WriteFile(path, []byte(largePayload), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", d, err)
		}
	}

	metrics2 := NewMetrics()

	// Trigger pruning again
	pruneOldLogs(logsDir, now, nil, metrics2)

	// Verify metrics for the second run (should delete 2 files due to size limit)
	snap2 := metrics2.Snapshot()
	if snap2.Counters["observability.pruning.total"] != 1 {
		t.Errorf("expected second run counter to be 1, got %d", snap2.Counters["observability.pruning.total"])
	}
	if snap2.Counters["observability.pruning.deleted_files_total"] != 2 {
		t.Errorf("expected second run deleted files to be 2, got %d", snap2.Counters["observability.pruning.deleted_files_total"])
	}
	if snap2.Gauges["observability.pruning.final_files"] != 1 {
		t.Errorf("expected second run final files to be 1, got %d", snap2.Gauges["observability.pruning.final_files"])
	}

	// The newest file (today) should survive, others should be pruned.
	todayPath := filepath.Join(logsDir, "activity-"+dates[3]+".jsonl")
	if _, err := os.Stat(todayPath); err != nil {
		t.Errorf("expected newest file to survive, error: %v", err)
	}

	// Older files like dates[1] and dates[2] should be pruned oldest-first to stay under 50 bytes.
	if _, err := os.Stat(filepath.Join(logsDir, "activity-"+dates[1]+".jsonl")); !os.IsNotExist(err) {
		t.Errorf("expected dates[1] to be size-pruned")
	}
	if _, err := os.Stat(filepath.Join(logsDir, "activity-"+dates[2]+".jsonl")); !os.IsNotExist(err) {
		t.Errorf("expected dates[2] to be size-pruned")
	}
}
