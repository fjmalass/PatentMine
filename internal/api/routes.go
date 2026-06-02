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
	s.mux.HandleFunc("POST /metrics", s.handleMetricsPush)
	s.mux.HandleFunc("GET /activity", s.handleActivityList)
	s.mux.HandleFunc("GET /activity/raw", s.handleActivityRaw)
	s.mux.HandleFunc("POST /activity", s.handleActivityRecord)
	s.mux.HandleFunc("GET /history", s.handleHistoryList)
	s.mux.HandleFunc("GET /table_views", s.handleTableViewList)
	s.mux.HandleFunc("POST /table_views", s.handleTableViewSave)
	s.mux.HandleFunc("GET /table_views/{id}", s.handleTableViewGet)
	s.mux.HandleFunc("DELETE /table_views/{id}", s.handleTableViewDelete)
	s.mux.HandleFunc("GET /activity/replay_history", s.handleReplayHistory)
	s.mux.HandleFunc("GET /commands", s.handleCommands)
	s.mux.HandleFunc("GET /events", s.handleEvents)
	s.mux.HandleFunc("GET /session", s.handleSessionGet)
	s.mux.HandleFunc("PUT /session", s.handleSessionPut)
	s.mux.HandleFunc("PATCH /session", s.handleSessionPatch)
	s.mux.HandleFunc("POST /filters/validate", s.handleFilterValidate)
	s.mux.HandleFunc("GET /ai/config", s.handleAIConfig)
	s.mux.HandleFunc("POST /patents/{number}/ai/analyze", s.handleAIAnalyze)
	s.mux.HandleFunc("GET /patents", s.handlePatentList)
	s.mux.HandleFunc("GET /patents/orphans", s.handleOrphanList)
	s.mux.HandleFunc("GET /assignees", s.handleAssigneeStats)
	s.mux.HandleFunc("GET /assignees/stats", s.handleAssigneeStats)
	s.mux.HandleFunc("GET /classifications/stats", s.handleClassificationStats)
	s.mux.HandleFunc("GET /patents/columns", s.handlePatentTableColumns)
	s.mux.HandleFunc("GET /patents/{number}", s.handlePatentGet)
	s.mux.HandleFunc("DELETE /patents/{number}", s.handlePatentDelete)
	s.mux.HandleFunc("POST /patents/{number}/clear_cache", s.handlePatentClearCache)
	s.mux.HandleFunc("POST /patents/{number}/tags", s.handlePatentTagAssignLoose)
	s.mux.HandleFunc("DELETE /patents/{number}/tags/{name}", s.handlePatentTagRemoveLoose)
	s.mux.HandleFunc("GET /patents/{number}/relations", s.handleRelations)
	s.mux.HandleFunc("GET /patents/{number}/family", s.handleFamilyGraph)
	s.mux.HandleFunc("PUT /patents/{number}/review_state", s.handleReviewState)
	s.mux.HandleFunc("GET /patents/{number}/diffs", s.handleSourceDiffsList)
	s.mux.HandleFunc("POST /patents/{number}/diffs/resolve", s.handleSourceResolveDiffs)
	s.mux.HandleFunc("GET /patents/{number}/inventors", s.handleInventorStats)
	s.mux.HandleFunc("GET /patents/{number}/bibs", s.handleSourceBibsList)
	s.mux.HandleFunc("DELETE /patents", s.handlePatentDeleteBulk)

	// USPTO data fetch and computation (per patent).
	s.mux.HandleFunc("POST /patents/{number}/uspto/fetch", s.handleUSPTOFetchXML)
	s.mux.HandleFunc("GET /patents/{number}/uspto/grant_body", s.handleUSPTOGrantBody)
	s.mux.HandleFunc("POST /patents/{number}/uspto/assignments", s.handleUSPTOFetchAssignments)
	s.mux.HandleFunc("GET /patents/{number}/uspto/assignments", s.handleUSPTOAssignmentList)
	s.mux.HandleFunc("GET /patents/{number}/assignees", s.handleAssigneeHistory)
	s.mux.HandleFunc("POST /assignments/fetch", s.handleFetchAssignmentsBatch)
	s.mux.HandleFunc("GET /patents/{number}/expiration", s.handleExpirationCalculate)
	s.mux.HandleFunc("GET /projects", s.handleProjectList)
	s.mux.HandleFunc("POST /projects", s.handleProjectCreate)
	s.mux.HandleFunc("POST /projects/{id}/patents", s.handleAddMember)
	s.mux.HandleFunc("POST /projects/{id}/patents/related", s.handleAddRelated)
	s.mux.HandleFunc("GET /projects/{id}/ids", s.handleIDS)
	s.mux.HandleFunc("POST /projects/{id}/ids/pdf", s.handleIDSPDF)
	s.mux.HandleFunc("POST /projects/{id}/ids/pdf/preview", s.handleIDSPDFPreview)
	s.mux.HandleFunc("POST /projects/{id}/ids/status", s.handleIDSBulkSetStatus)
	s.mux.HandleFunc("GET /projects/{id}/ids/entries/{number}", s.handleIDSEntryGet)
	s.mux.HandleFunc("PUT /projects/{id}/ids/entries/{number}", s.handleIDSEntrySave)
	s.mux.HandleFunc("DELETE /projects/{id}/ids/entries/{number}", s.handleIDSEntryDelete)
	s.mux.HandleFunc("PUT /projects/{id}", s.handleProjectUpdate)
	s.mux.HandleFunc("GET /projects/{id}/added/export", s.handleAddedExport)
	s.mux.HandleFunc("POST /projects/{id}/added", s.handleAddedImport)
	s.mux.HandleFunc("GET /projects/{id}/assignees", s.handleProjectAssignees)
	s.mux.HandleFunc("POST /projects/{id}/assignees/fetch", s.handleProjectAssigneesFetch)
	s.mux.HandleFunc("GET /projects/{id}/notes", s.handleNoteList)
	s.mux.HandleFunc("GET /projects/{id}/notes/export", s.handleNotesExport)
	s.mux.HandleFunc("GET /projects/{id}/patents/{number}/note", s.handleNoteGet)
	s.mux.HandleFunc("PUT /projects/{id}/patents/{number}/note", s.handleNoteSave)
	s.mux.HandleFunc("DELETE /projects/{id}/patents/{number}/note", s.handleNoteDelete)
	s.mux.HandleFunc("GET /crawl/config", s.handleCrawlConfig)
	s.mux.HandleFunc("GET /source_mode", s.handleSourceModeGet)
	s.mux.HandleFunc("PUT /source_mode", s.handleSourceModeSet)
	s.mux.HandleFunc("GET /config/source_mode", s.handleSourceModeGet)
	s.mux.HandleFunc("PUT /config/source_mode", s.handleSourceModeSet)
	s.mux.HandleFunc("POST /crawl", s.handleCrawl)
	s.mux.HandleFunc("POST /crawl/cancel", s.handleCrawlCancel)
	s.mux.HandleFunc("POST /import", s.handleImportFile)

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

	s.registerWebUI()
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
