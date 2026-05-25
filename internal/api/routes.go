package api

import (
	"encoding/json"
	"net/http"

	"patentmine/internal/proto"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// routes registers every HTTP endpoint. Each maps to a command in the registry,
// so the web API and the TUI act on the same catalogue of operations.
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /metrics", s.handleMetricsProm)
	s.mux.HandleFunc("GET /metricsz", s.handleMetrics)
	s.mux.HandleFunc("GET /activity", s.handleActivityList)
	s.mux.HandleFunc("POST /activity", s.handleActivityRecord)
	s.mux.HandleFunc("GET /activity/replay_history", s.handleReplayHistory)
	s.mux.HandleFunc("GET /commands", s.handleCommands)
	s.mux.HandleFunc("GET /patents", s.handlePatentList)
	s.mux.HandleFunc("GET /assignees", s.handleAssigneeStats)
	s.mux.HandleFunc("GET /assignees/stats", s.handleAssigneeStats)
	s.mux.HandleFunc("GET /classifications/stats", s.handleClassificationStats)
	s.mux.HandleFunc("GET /patents/columns", s.handlePatentTableColumns)
	s.mux.HandleFunc("GET /patents/{number}", s.handlePatentGet)
	s.mux.HandleFunc("GET /patents/{number}/relations", s.handleRelations)
	s.mux.HandleFunc("GET /patents/{number}/family", s.handleFamilyGraph)
	s.mux.HandleFunc("PUT /patents/{number}/review_state", s.handleReviewState)
	s.mux.HandleFunc("GET /projects", s.handleProjectList)
	s.mux.HandleFunc("POST /projects", s.handleProjectCreate)
	s.mux.HandleFunc("POST /projects/{id}/patents", s.handleAddMember)
	s.mux.HandleFunc("GET /projects/{id}/ids", s.handleIDS)
	s.mux.HandleFunc("POST /projects/{id}/ids/pdf", s.handleIDSPDF)
	s.mux.HandleFunc("PUT /projects/{id}", s.handleProjectUpdate)
	s.mux.HandleFunc("GET /projects/{id}/notes/export", s.handleNotesExport)
	s.mux.HandleFunc("GET /crawl/config", s.handleCrawlConfig)
	s.mux.HandleFunc("POST /crawl", s.handleCrawl)

	// Tag Taxonomy endpoints
	s.mux.HandleFunc("POST /projects/{id}/tags", s.handleTagCreate)
	s.mux.HandleFunc("GET /projects/{id}/tags", s.handleTagList)
	s.mux.HandleFunc("DELETE /projects/{id}/tags/{name}", s.handleTagDelete)
	s.mux.HandleFunc("POST /projects/{id}/patents/{number}/tags", s.handlePatentTagAdd)
	s.mux.HandleFunc("DELETE /projects/{id}/patents/{number}/tags/{name}", s.handlePatentTagDelete)
	s.mux.HandleFunc("GET /projects/{id}/patents/{number}/tags", s.handlePatentTagList)

	// Patent Classification endpoints
	s.mux.HandleFunc("GET /classifications", s.handleClassificationList)
	s.mux.HandleFunc("GET /classifications/{system}/{code}", s.handleClassificationGet)
	s.mux.HandleFunc("POST /classifications", s.handleClassificationSave)
	s.mux.HandleFunc("DELETE /classifications/{system}/{code}", s.handleClassificationDelete)
	s.mux.HandleFunc("POST /classifications/lookup", s.handleClassificationLookup)
	s.mux.HandleFunc("POST /classifications/by_codes", s.handleClassificationListByCodes)
	s.mux.HandleFunc("GET /projects/{id}/patents/{number}/classifications", s.handlePatentClassificationList)
}

// handleHealth pings the daemon.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	var res proto.PingResult
	s.call(w, r, proto.MethodPing, nil, &res)
}

// handleCommands returns the command registry.
func (s *Server) handleCommands(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.registry.All())
}

// decodeBody decodes a JSON request body, writing a 400 on failure.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		badRequest(w, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}


