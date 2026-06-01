// Metrics, activity-feed and history payloads.
package proto

import (
	"time"

	"patentmine/internal/observability"
)

// MetricsResult carries the daemon's current in-memory timing/counter snapshot.
type MetricsResult struct {
	Metrics MetricsSnapshot `json:"metrics"`
}

// MetricsPushParams carries a client-side metrics snapshot for the daemon to merge.
type MetricsPushParams struct {
	Component string          `json:"component"`
	Snapshot  MetricsSnapshot `json:"snapshot"`
}

// ActivityRawParams selects raw activity records from the daemon journal.
type ActivityRawParams struct {
	Limit     int       `json:"limit,omitempty"`
	Component string    `json:"component,omitempty"`
	Action    string    `json:"action,omitempty"`
	Entity    string    `json:"entity,omitempty"`
	Since     time.Time `json:"since,omitempty"`
}

// ActivityRawResult carries raw activity records and accounting.
type ActivityRawResult struct {
	Records  []observability.Record `json:"records"`
	Limit    int                    `json:"limit"`
	Returned int                    `json:"returned"`
}

// HistoryFeedParams selects the raw activity window to group into history.
type HistoryFeedParams struct {
	RawLimit  int       `json:"raw_limit,omitempty"`
	Component string    `json:"component,omitempty"`
	Since     time.Time `json:"since,omitempty"`
}

// MetricsSnapshot is a transport-safe view of the daemon's metrics.
type MetricsSnapshot struct {
	Timestamp time.Time               `json:"timestamp"`
	Timings   map[string]TimingMetric `json:"timings"`
	Counters  map[string]int64        `json:"counters"`
	Gauges    map[string]int64        `json:"gauges"`
}

// TimingMetric aggregates one named operation's observed durations.
type TimingMetric struct {
	Count      int64 `json:"count"`
	Errors     int64 `json:"errors"`
	TotalNanos int64 `json:"total_nanos"`
	AvgNanos   int64 `json:"avg_nanos"`
	AvgMillis  int64 `json:"avg_millis"`
	MinNanos   int64 `json:"min_nanos"`
	MinMillis  int64 `json:"min_millis"`
	MaxNanos   int64 `json:"max_nanos"`
	MaxMillis  int64 `json:"max_millis"`
	LastNanos  int64 `json:"last_nanos"`
	LastMillis int64 `json:"last_millis"`
}
