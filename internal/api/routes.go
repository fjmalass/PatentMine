package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
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

// routes registers every HTTP endpoint. Each maps to a command in the registry
// (named in the trailing comment), so the web API and the TUI act on the same
// catalogue of operations.
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.HandleFunc("GET /metrics", s.handleMetricsProm)
	s.mux.HandleFunc("GET /metricsz", s.handleMetrics)
	s.mux.HandleFunc("GET /activity", s.handleActivityList)
	s.mux.HandleFunc("POST /activity", s.handleActivityRecord)
	s.mux.HandleFunc("GET /activity/replay_history", s.handleReplayHistory)
	s.mux.HandleFunc("GET /commands", s.handleCommands)
	s.mux.HandleFunc("GET /patents", s.handlePatentList) // command.PatentList
	s.mux.HandleFunc("GET /assignees", s.handleAssigneeStats)
	s.mux.HandleFunc("GET /assignees/stats", s.handleAssigneeStats)
	s.mux.HandleFunc("GET /classifications/stats", s.handleClassificationStats)
	s.mux.HandleFunc("GET /patents/columns", s.handlePatentTableColumns)
	s.mux.HandleFunc("GET /patents/{number}", s.handlePatentGet)           // command.PatentGet
	s.mux.HandleFunc("GET /patents/{number}/relations", s.handleRelations) // command.PatentRelations
	s.mux.HandleFunc("GET /patents/{number}/family", s.handleFamilyGraph)
	s.mux.HandleFunc("PUT /patents/{number}/review_state", s.handleReviewState) // command.Mark*
	s.mux.HandleFunc("GET /projects", s.handleProjectList)                      // command.ProjectList
	s.mux.HandleFunc("POST /projects", s.handleProjectCreate)                   // command.ProjectCreate
	s.mux.HandleFunc("POST /projects/{id}/patents", s.handleAddMember)          // command.AddToProject
	s.mux.HandleFunc("GET /projects/{id}/ids", s.handleIDS)                     // command.ExportIDS
	s.mux.HandleFunc("GET /crawl/config", s.handleCrawlConfig)
	s.mux.HandleFunc("POST /crawl", s.handleCrawl) // command.CrawlFamily

	// Fixed Tag Taxonomy endpoints
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

// handleActivityList returns replay/review activity records from the shared JSONL journal.
func (s *Server) handleActivityList(w http.ResponseWriter, r *http.Request) {
	if s.activityLogsDir == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "activity logging is not configured"})
		return
	}
	records, err := observability.ReadActivityRecords(s.activityLogsDir, parseActivityQuery(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": records})
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

// handleCrawlConfig returns daemon-owned crawl defaults.
func (s *Server) handleCrawlConfig(w http.ResponseWriter, r *http.Request) {
	var res proto.CrawlConfigResult
	s.call(w, r, proto.MethodCrawlConfig, nil, &res)
}

// handleMetrics returns the daemon's current in-memory timing/counter snapshot.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	var res proto.MetricsResult
	s.call(w, r, proto.MethodMetricsGet, nil, &res)
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

// handleRelations lists a patent's family-graph edges of one kind (the "kind"
// query parameter; defaults to citations).
func (s *Server) handleRelations(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	q := r.URL.Query()
	kind := domain.RelationKind(q.Get("kind"))
	if kind == "" {
		kind = domain.RelationCites
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	sortAscending, _ := strconv.ParseBool(q.Get("sort_ascending"))
	var res proto.RelationsResult
	s.call(w, r, proto.MethodRelations,
		proto.RelationsParams{
			Number:         number,
			Kind:           kind,
			Project:        domain.ProjectID(q.Get("project")),
			Filter:         q.Get("filter"),
			ReviewState:    domain.ReviewState(firstNonEmpty(q.Get("review_state"), q.Get("state"))),
			Search:         q.Get("search"),
			Classification: firstNonEmpty(q.Get("classification"), q.Get("class")),
			Limit:          limit,
			Offset:         offset,
			SortColumn:     domain.SortColumn(q.Get("sort_column")),
			SortAscending:  sortAscending,
		}, &res)
}

// handleFamilyGraph returns a bounded parent/child family DAG around one patent.
func (s *Server) handleFamilyGraph(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	q := r.URL.Query()
	depth, _ := strconv.Atoi(q.Get("depth"))
	maxNodes, _ := strconv.Atoi(firstNonEmpty(q.Get("max_nodes"), q.Get("limit")))
	var res proto.FamilyGraphResult
	s.call(w, r, proto.MethodFamilyGraph,
		proto.FamilyGraphParams{
			Root:      number,
			Depth:     depth,
			MaxNodes:  maxNodes,
			Countries: q["country"],
		}, &res)
}

// handleIDS builds the Information Disclosure Statement for a project.
func (s *Server) handleIDS(w http.ResponseWriter, r *http.Request) {
	var res proto.IDSResult
	s.call(w, r, proto.MethodIDSExport,
		proto.IDSExportParams{Project: domain.ProjectID(r.PathValue("id"))}, &res)
}

// handleHealth pings the daemon.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	var res proto.PingResult
	s.call(w, r, proto.MethodPing, nil, &res)
}

// handleCommands returns the command registry — the shared source of truth for
// every frontend's actions.
func (s *Server) handleCommands(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.registry.All())
}

// handlePatentList lists patents, paginated via query parameters.
func (s *Server) handlePatentList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	sortAscending, _ := strconv.ParseBool(q.Get("sort_ascending"))
	params := proto.PatentListParams{
		Project:            domain.ProjectID(q.Get("project")),
		Filter:             q.Get("filter"),
		ReviewState:        domain.ReviewState(firstNonEmpty(q.Get("review_state"), q.Get("state"))),
		Search:             q.Get("search"),
		Classification:     firstNonEmpty(q.Get("classification"), q.Get("class")),
		ClassificationCode: q.Get("classification_code"),
		Inventor:           q.Get("inventor"),
		Assignee:           firstNonEmpty(q.Get("assignee"), q.Get("owner")),
		Limit:              limit,
		Offset:             offset,
		SortColumn:         domain.SortColumn(q.Get("sort_column")),
		SortAscending:      sortAscending,
	}
	if s.activity != nil && (params.Filter != "" || params.ReviewState != "" || params.Search != "" || params.Classification != "" || params.ClassificationCode != "" || params.Inventor != "" || params.Assignee != "") {
		_ = s.activity.Record(r.Context(), observability.Record{
			Action:   "filter.apply",
			Entity:   "filter",
			EntityID: r.URL.RawQuery,
			Status:   "requested",
			Metadata: map[string]any{"source": "http", "path": r.URL.Path, "query": r.URL.RawQuery},
		})
	}
	var res proto.PatentListResult
	s.call(w, r, proto.MethodPatentList, params, &res)
}

func (s *Server) handleAssigneeStats(w http.ResponseWriter, r *http.Request) {
	var res proto.PatentAssigneeStatsResult
	s.call(w, r, proto.MethodPatentAssigneeStats, proto.PatentAssigneeStatsParams{Project: domain.ProjectID(r.URL.Query().Get("project"))}, &res)
}

func (s *Server) handleClassificationStats(w http.ResponseWriter, r *http.Request) {
	var res proto.PatentClassificationStatsResult
	s.call(w, r, proto.MethodPatentClassificationStats, proto.PatentClassificationStatsParams{Project: domain.ProjectID(r.URL.Query().Get("project"))}, &res)
}

func (s *Server) handlePatentTableColumns(w http.ResponseWriter, r *http.Request) {
	var res proto.PatentTableColumnsResult
	s.call(w, r, proto.MethodPatentTableColumns,
		proto.PatentTableColumnsParams{Project: domain.ProjectID(r.URL.Query().Get("project"))}, &res)
}

// handlePatentGet returns one patent.
func (s *Server) handlePatentGet(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	if s.activity != nil {
		durationMS, _ := strconv.ParseInt(r.URL.Query().Get("duration_ms"), 10, 64)
		if durationMS <= 0 || time.Duration(durationMS)*time.Millisecond >= s.activityMinDuration {
			metadata := map[string]any{"source": "http", "path": r.URL.Path}
			if durationMS > 0 {
				metadata["duration_ms"] = durationMS
			}
			_ = s.activity.Record(r.Context(), observability.Record{
				Action:   "ui.focus",
				Entity:   "patent",
				EntityID: number.String(),
				Status:   "observed",
				Metadata: metadata,
			})
		}
	}
	var res proto.PatentResult
	s.call(w, r, proto.MethodPatentGet, proto.PatentGetParams{Number: number}, &res)
}

// handleReviewState changes a patent's review state in a project.
func (s *Server) handleReviewState(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	var body struct {
		Project string `json:"project"`
		State   string `json:"state"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	params := proto.ReviewStateParams{
		Project: domain.ProjectID(body.Project),
		Patents: []domain.PatentNumber{number},
		State:   body.State,
	}
	var res proto.Empty
	s.call(w, r, proto.MethodReviewState, params, &res)
}

// handleProjectList lists projects.
func (s *Server) handleProjectList(w http.ResponseWriter, r *http.Request) {
	var res proto.ProjectListResult
	s.call(w, r, proto.MethodProjectList, nil, &res)
}

// handleProjectCreate creates a project.
func (s *Server) handleProjectCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var res proto.ProjectResult
	s.call(w, r, proto.MethodProjectCreate, proto.ProjectCreateParams{Name: body.Name}, &res)
}

// handleAddMember adds a patent to a project.
func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Patent domain.PatentNumber `json:"patent"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	params := proto.MembershipParams{
		Project: domain.ProjectID(r.PathValue("id")),
		Patent:  body.Patent,
	}
	var res proto.Empty
	s.call(w, r, proto.MethodMembershipAdd, params, &res)
}

// handleCrawl starts a family-graph crawl.
func (s *Server) handleCrawl(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Root  domain.PatentNumber `json:"root"`
		Depth int                 `json:"depth"`
		Force bool                `json:"force"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var res proto.CrawlStartResult
	s.call(w, r, proto.MethodCrawlFamily,
		proto.CrawlFamilyParams{Root: body.Root, Depth: body.Depth, Force: body.Force}, &res)
}

// handleTagCreate creates a taxonomy tag in a project.
func (s *Server) handleTagCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	params := proto.TagCreateParams{
		Project: domain.ProjectID(r.PathValue("id")),
		Name:    body.Name,
	}
	var res domain.Tag
	s.call(w, r, proto.MethodTagCreate, params, &res)
}

// handleTagList lists taxonomy tags in a project.
func (s *Server) handleTagList(w http.ResponseWriter, r *http.Request) {
	params := proto.TagListParams{
		Project: domain.ProjectID(r.PathValue("id")),
	}
	var res proto.TagListResult
	s.call(w, r, proto.MethodTagList, params, &res)
}

// handleTagDelete deletes a taxonomy tag from a project.
func (s *Server) handleTagDelete(w http.ResponseWriter, r *http.Request) {
	params := proto.TagDeleteParams{
		Project: domain.ProjectID(r.PathValue("id")),
		Name:    r.PathValue("name"),
	}
	var res proto.Empty
	s.call(w, r, proto.MethodTagDelete, params, &res)
}

// handlePatentTagAdd assigns a tag to a patent in a project.
func (s *Server) handlePatentTagAdd(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	params := proto.TagParams{
		Project: domain.ProjectID(r.PathValue("id")),
		Patents: []domain.PatentNumber{number},
		Name:    body.Name,
	}
	var res proto.Empty
	s.call(w, r, proto.MethodTagPatentStrict, params, &res)
}

// handlePatentTagDelete removes a tag from a patent in a project.
func (s *Server) handlePatentTagDelete(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	params := proto.TagParams{
		Project: domain.ProjectID(r.PathValue("id")),
		Patents: []domain.PatentNumber{number},
		Name:    r.PathValue("name"),
	}
	var res proto.Empty
	s.call(w, r, proto.MethodUntagPatentStrict, params, &res)
}

// handlePatentTagList lists tags assigned to a patent in a project.
func (s *Server) handlePatentTagList(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	params := proto.PatentTagListParams{
		Project: domain.ProjectID(r.PathValue("id")),
		Patent:  number,
	}
	var res proto.PatentTagListResult
	s.call(w, r, proto.MethodPatentTagList, params, &res)
}

// decodeBody decodes a JSON request body, writing a 400 on failure. It reports
// whether decoding succeeded.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		badRequest(w, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func (s *Server) handleClassificationList(w http.ResponseWriter, r *http.Request) {
	var res proto.ClassificationListResult
	s.call(w, r, proto.MethodClassificationList, nil, &res)
}

func (s *Server) handleClassificationGet(w http.ResponseWriter, r *http.Request) {
	params := proto.ClassificationGetParams{
		System: r.PathValue("system"),
		Code:   r.PathValue("code"),
	}
	var res domain.Classification
	s.call(w, r, proto.MethodClassificationGet, params, &res)
}

func (s *Server) handleClassificationSave(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Classification domain.Classification `json:"classification"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var res proto.Empty
	s.call(w, r, proto.MethodClassificationSave, proto.ClassificationParams{Classification: body.Classification}, &res)
}

func (s *Server) handleClassificationDelete(w http.ResponseWriter, r *http.Request) {
	params := proto.ClassificationDeleteParams{
		System: r.PathValue("system"),
		Code:   r.PathValue("code"),
	}
	var res proto.Empty
	s.call(w, r, proto.MethodClassificationDelete, params, &res)
}

func (s *Server) handleClassificationLookup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var res domain.Classification
	s.call(w, r, proto.MethodClassificationLookup, proto.ClassificationLookupParams{Code: body.Code}, &res)
}

func (s *Server) handleClassificationListByCodes(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Codes []string `json:"codes"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var res proto.ClassificationListResult
	s.call(w, r, proto.MethodClassificationListByCodes, proto.ClassificationListByCodesParams{Codes: body.Codes}, &res)
}

func (s *Server) handlePatentClassificationList(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	params := proto.PatentClassificationListParams{
		Project: domain.ProjectID(r.PathValue("id")),
		Patent:  number,
	}
	var res proto.ClassificationListResult
	s.call(w, r, proto.MethodPatentClassificationList, params, &res)
}
