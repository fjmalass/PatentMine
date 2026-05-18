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
