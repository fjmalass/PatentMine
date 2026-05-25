package api

import (
	"net/http"
	"strconv"
	"time"

	"patentmine/internal/observability"
)

// handleActivityList returns replay/review activity records from the shared JSONL journal.
func (s *Server) handleActivityList(w http.ResponseWriter, r *http.Request) {
	s.handleActivityRaw(w, r)
}

// handleActivityRaw returns raw activity records from the shared JSONL journal.
func (s *Server) handleActivityRaw(w http.ResponseWriter, r *http.Request) {
	if s.activityLogsDir == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "activity logging is not configured"})
		return
	}
	start := time.Now()
	query := parseActivityQuery(r)
	records, err := observability.ReadActivityRecords(s.activityLogsDir, query)
	s.observeRawActivityQuery(start, len(records), err)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": records, "limit": effectiveActivityLimit(query.Limit), "returned": len(records)})
}

// handleActivityRecord lets an HTTP/browser UI submit a user activity event.
// Events with duration_ms below the configured threshold are acknowledged but skipped.
func (s *Server) handleActivityRecord(w http.ResponseWriter, r *http.Request) {
	if s.activity == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "activity logging is not configured"})
		return
	}
	var body struct {
		Action     string         `json:"action"`
		Entity     string         `json:"entity"`
		EntityID   string         `json:"entity_id"`
		Status     string         `json:"status"`
		DurationMS int64          `json:"duration_ms"`
		Before     any            `json:"before"`
		After      any            `json:"after"`
		Metadata   map[string]any `json:"metadata"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Action == "" || body.Entity == "" {
		badRequest(w, "activity action and entity are required")
		return
	}
	if body.DurationMS > 0 && time.Duration(body.DurationMS)*time.Millisecond < s.activityMinDuration {
		writeJSON(w, http.StatusOK, map[string]any{"recorded": false, "skipped": true})
		return
	}
	if body.Status == "" {
		body.Status = "observed"
	}
	metadata := body.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	if body.DurationMS > 0 {
		metadata["duration_ms"] = body.DurationMS
	}
	rec := observability.Record{
		Action:   body.Action,
		Entity:   body.Entity,
		EntityID: body.EntityID,
		Status:   body.Status,
		Before:   body.Before,
		After:    body.After,
		Metadata: metadata,
	}
	if err := s.activity.Record(r.Context(), rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recorded": true})
}

// handleReplayHistory returns the capped log of activity rows opened from replay UI.
func (s *Server) handleReplayHistory(w http.ResponseWriter, r *http.Request) {
	if s.activityLogsDir == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "activity logging is not configured"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	history, err := observability.ReadReplayHistory(s.activityLogsDir, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": history, "cap": 10000})
}

func effectiveActivityLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	return limit
}

func (s *Server) observeRawActivityQuery(start time.Time, returned int, err error) {
	if s.activityMetrics == nil {
		return
	}
	s.activityMetrics.ObserveDuration("api.activity.raw.query", time.Since(start), err != nil)
	s.activityMetrics.IncCounter("api.activity.raw.query_total", 1)
	s.activityMetrics.IncCounter("api.activity.raw.records_returned_total", int64(returned))
	if err != nil {
		s.activityMetrics.IncCounter("api.activity.raw.query_error_total", 1)
	}
}
