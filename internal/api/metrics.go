package api

import (
	"context"
	"net/http"

	"patentmine/internal/observability"
	"patentmine/internal/proto"
)

// handleMetrics returns the daemon's current in-memory timing/counter snapshot.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var res proto.MetricsResult
	s.call(w, r, proto.MethodMetricsGet, nil, &res)
}

// handleMetricsPush merges a client-side metrics snapshot into the daemon's.
// Body: component, snapshot.
func (s *Server) handleMetricsPush(w http.ResponseWriter, r *http.Request) {
	var body proto.MetricsPushParams
	if !decodeBody(w, r, &body) {
		return
	}
	var res proto.Empty
	s.call(w, r, proto.MethodMetricsPush, body, &res)
}

// handleMetricsProm renders the current in-memory metrics snapshot in
// Prometheus exposition format.
func (s *Server) handleMetricsProm(w http.ResponseWriter, r *http.Request) {
	var res proto.MetricsResult
	ctx, cancel := context.WithTimeout(r.Context(), apiCallTimeout)
	defer cancel()
	if err := s.client.Call(ctx, proto.MethodMetricsGet, nil, &res); err != nil {
		writeError(w, err)
		return
	}
	snap := observability.Snapshot{
		Timestamp: res.Metrics.Timestamp,
		Timings:   make(map[string]observability.TimingSummary, len(res.Metrics.Timings)),
		Counters:  res.Metrics.Counters,
		Gauges:    res.Metrics.Gauges,
	}
	for name, metric := range res.Metrics.Timings {
		snap.Timings[name] = observability.TimingSummary{
			Count:      metric.Count,
			Errors:     metric.Errors,
			TotalNanos: metric.TotalNanos,
			MinNanos:   metric.MinNanos,
			MaxNanos:   metric.MaxNanos,
			LastNanos:  metric.LastNanos,
		}
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte(observability.PrometheusText(snap)))
}
