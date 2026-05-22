package overlay

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/proto"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

func TestMetricsOverlayDefaultsToTimings(t *testing.T) {
	o := &MetricsOverlay{
		theme:   render.NewTheme(),
		catalog: text.English(),
		current: proto.MetricsSnapshot{
			Timestamp: time.Unix(100, 0).UTC(),
			Timings: map[string]proto.TimingMetric{
				"rpc.method.patent.list": {Count: 4, Errors: 1, AvgMillis: 22, MaxMillis: 55, LastMillis: 14, TotalNanos: int64(88 * time.Millisecond)},
			},
			Counters: map[string]int64{"engine.worker.submit_total": 17},
			Gauges:   map[string]int64{"engine.worker.queue_depth": 3},
		},
		history: []proto.MetricsSnapshot{
			{
				Timestamp: time.Unix(98, 0).UTC(),
				Timings: map[string]proto.TimingMetric{
					"rpc.method.patent.list": {Count: 3, AvgMillis: 20, MaxMillis: 40, LastMillis: 20, TotalNanos: int64(60 * time.Millisecond)},
				},
				Counters: map[string]int64{"engine.worker.submit_total": 15},
				Gauges:   map[string]int64{"engine.worker.queue_depth": 1},
			},
		},
	}
	o.history = append(o.history, o.current)

	view := o.View(120, 16)
	if !strings.Contains(view, "Timings") {
		t.Fatalf("expected timings tab, view:\n%s", view)
	}
	if !strings.Contains(view, "RPC") {
		t.Fatalf("expected RPC section, view:\n%s", view)
	}
	if !strings.Contains(view, "rpc.method.patent.list") {
		t.Fatalf("expected timing metric name, view:\n%s", view)
	}
	if strings.Contains(view, "engine.worker.submit_total") {
		t.Fatalf("expected counters to stay hidden on default timings tab, view:\n%s", view)
	}

	w, h := o.OverlaySize(100, 50)
	if w != 90 || h != 45 {
		t.Fatalf("expected 90%% overlay size, got %dx%d", w, h)
	}
}

func TestMetricsOverlayTabSwitchAndStaleUpdate(t *testing.T) {
	o := &MetricsOverlay{
		theme:   render.NewTheme(),
		catalog: text.English(),
		current: proto.MetricsSnapshot{
			Timestamp: time.Unix(200, 0).UTC(),
			Timings:   map[string]proto.TimingMetric{"rpc.method.ping": {Count: 1, AvgMillis: 1, LastMillis: 1}},
			Counters:  map[string]int64{"crawl.crawler.fetch_total": 9},
			Gauges:    map[string]int64{"crawl.crawler.pending": 2},
		},
		history: []proto.MetricsSnapshot{{
			Timestamp: time.Unix(200, 0).UTC(),
			Timings:   map[string]proto.TimingMetric{"rpc.method.ping": {Count: 1, AvgMillis: 1, LastMillis: 1}},
			Counters:  map[string]int64{"crawl.crawler.fetch_total": 9},
			Gauges:    map[string]int64{"crawl.crawler.pending": 2},
		}},
		requestID: 3,
	}

	_, _, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	if !handled {
		t.Fatal("expected tab key to be handled")
	}
	if o.tab != metricsTabCounters {
		t.Fatalf("expected counters tab, got %d", o.tab)
	}
	view := o.View(120, 16)
	if !strings.Contains(view, "crawl.crawler.fetch_total") {
		t.Fatalf("expected counter metric on counters tab, view:\n%s", view)
	}

	_, _ = o.Update(metricsLoadedMsg{requestID: 2, snapshot: proto.MetricsSnapshot{Counters: map[string]int64{"ignored": 99}}})
	if _, ok := o.current.Counters["ignored"]; ok {
		t.Fatal("stale metrics response should be ignored")
	}

	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if !handled {
		t.Fatal("expected h to be handled for previous tab")
	}
	if o.tab != metricsTabTimings {
		t.Fatalf("expected timings tab after moving back, got %d", o.tab)
	}
}
