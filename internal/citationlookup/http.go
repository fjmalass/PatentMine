package citationlookup

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"patentmine/internal/domain"
)

type StatusStore interface {
	GetCitationRefreshJob(context.Context, int64) (domain.CitationRefreshJob, error)
	CitationLookupStats(context.Context, time.Time) (domain.CitationLookupStats, error)
}

type HTTPHandler struct {
	Service         *Service
	Store           StatusStore
	TenantHeader    string
	TraceHeader     string
	AuthorizedToken string
	MaxPatentLength int
}

func (h HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/lookup":
		h.handleLookup(w, r)
	case "/lookup/status":
		h.handleStatus(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h HTTPHandler) handleLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.authorize(r); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if h.Service == nil {
		http.Error(w, "lookup service is not configured", http.StatusInternalServerError)
		return
	}
	patent := strings.TrimSpace(r.URL.Query().Get("patent"))
	maxLen := h.MaxPatentLength
	if maxLen <= 0 {
		maxLen = 64
	}
	if patent == "" || len(patent) > maxLen {
		http.Error(w, "patent query is required and must be reasonably bounded", http.StatusBadRequest)
		return
	}
	tenantID := strings.TrimSpace(r.URL.Query().Get("project_id"))
	if tenantID == "" {
		tenantID = strings.TrimSpace(r.Header.Get(firstNonEmpty(h.TenantHeader, "X-Tenant-ID")))
	}
	if tenantID == "" {
		http.Error(w, "tenant identity is required", http.StatusUnauthorized)
		return
	}
	resp, err := h.Service.Lookup(r.Context(), LookupRequest{
		ProjectID:    domain.ProjectID(tenantID),
		TenantID:     tenantID,
		TraceID:      h.traceID(r),
		PatentNumber: patent,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if resp.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(resp.RetryAfter.Seconds())))
	}
	writeJSON(w, resp.HTTPStatus, resp)
}

func (h HTTPHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := h.authorize(r); err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	if h.Store == nil {
		http.Error(w, "status store is not configured", http.StatusInternalServerError)
		return
	}
	jobID := strings.TrimSpace(r.URL.Query().Get("job_id"))
	if jobID != "" {
		id, err := strconv.ParseInt(jobID, 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "job_id must be a positive integer", http.StatusBadRequest)
			return
		}
		job, err := h.Store.GetCitationRefreshJob(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, job)
		return
	}
	stats, err := h.Store.CitationLookupStats(r.Context(), time.Now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if h.Service != nil {
		stats = MergeStats(stats, h.Service.Stats())
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h HTTPHandler) authorize(r *http.Request) error {
	if h.AuthorizedToken == "" {
		return nil
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.EqualFold(auth, "Bearer "+h.AuthorizedToken) || r.Header.Get("X-API-Key") == h.AuthorizedToken {
		return nil
	}
	return errors.New("unauthorized")
}

func (h HTTPHandler) traceID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get(firstNonEmpty(h.TraceHeader, "X-Request-ID")))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
