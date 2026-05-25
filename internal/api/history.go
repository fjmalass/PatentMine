package api

import (
	"net/http"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
)

// handleHistoryList returns the grouped replay/history feed. raw_limit controls
// how many raw activity rows are scanned before grouping.
func (s *Server) handleHistoryList(w http.ResponseWriter, r *http.Request) {
	if s.activityLogsDir == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "activity logging is not configured"})
		return
	}
	start := time.Now()
	s.observeTableQuery(r, tableQueryTelemetryFromURL(domain.TableIDSActivityHistory, r.URL.Query()))
	feed, err := observability.ReadHistoryFeed(s.activityLogsDir, parseHistoryQuery(r))
	s.observeHistoryQuery(start, feed, err)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, feed)
}

func (s *Server) observeHistoryQuery(start time.Time, feed observability.HistoryFeed, err error) {
	if s.activityMetrics == nil {
		return
	}
	s.activityMetrics.ObserveDuration("api.history.query", time.Since(start), err != nil)
	s.activityMetrics.IncCounter("api.history.query_total", 1)
	s.activityMetrics.IncCounter("api.history.raw_scanned_total", int64(feed.RawScanned))
	s.activityMetrics.IncCounter("api.history.records_returned_total", int64(feed.Returned))
	s.activityMetrics.IncCounter("api.history.records_suppressed_total", int64(feed.Suppressed))
	if feed.RawScanned >= feed.RawLimit {
		s.activityMetrics.IncCounter("api.history.raw_limit_hit_total", 1)
	}
	if err != nil {
		s.activityMetrics.IncCounter("api.history.query_error_total", 1)
	}
}
