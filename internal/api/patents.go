package api

import (
	"net/http"
	"strconv"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
)

// handleRelations lists a patent's family-graph edges of one kind.
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
	params := proto.RelationsParams{
		Number:         number,
		Kind:           kind,
		Project:        domain.ProjectID(q.Get("project")),
		Filter:         q.Get("filter"),
		ReviewState:    domain.ReviewState(firstNonEmpty(q.Get("review_state"), q.Get("state"))),
		IDSStatus:      q.Get("ids_status"),
		Search:         q.Get("search"),
		Classification: firstNonEmpty(q.Get("classification"), q.Get("class")),
		Limit:          limit,
		Offset:         offset,
		SortColumn:     domain.SortColumn(q.Get("sort_column")),
		SortAscending:  sortAscending,
	}
	s.observeTableQuery(r, tableQueryTelemetryFromURL(domain.TableCitations, q))
	var res proto.RelationsResult
	s.call(w, r, proto.MethodRelations, params, &res)
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

// handleOrphanList returns patents that have no membership in any project.
// Paged via ?limit=&offset=. The payload mirrors PatentListResult so existing
// table renderers can consume it.
func (s *Server) handleOrphanList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	var res proto.OrphanListResult
	s.call(w, r, proto.MethodOrphanList, proto.OrphanListParams{Limit: limit, Offset: offset}, &res)
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
		IDSStatus:          q.Get("ids_status"),
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
	s.observeTableQuery(r, tableQueryTelemetryFromURL(domain.TablePatents, q))
	var res proto.PatentListResult
	s.call(w, r, proto.MethodPatentList, params, &res)
}

// handleAllAssigneesHistory returns all assignees history.
func (s *Server) handleAllAssigneesHistory(w http.ResponseWriter, r *http.Request) {
	var res proto.AllAssigneesHistoryResult
	s.call(w, r, proto.MethodAllAssigneesHistory, proto.AllAssigneesHistoryParams{Project: domain.ProjectID(r.URL.Query().Get("project"))}, &res)
}

// handlePatentTableColumns returns patent table columns schema.
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
			attrs := map[string]any{"source": "http", "path": r.URL.Path}
			if durationMS > 0 {
				attrs["duration_ms"] = durationMS
			}
			_ = s.activity.Record(r.Context(), observability.Record{
				Action:   observability.ActionUIFocus,
				Entity:   "patent",
				EntityID: number.String(),
				Status:   "observed",
				Attributes: attrs,
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

// handleInventorStats returns database statistics for the inventors of one patent.
func (s *Server) handleInventorStats(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	params := proto.PatentGetParams{
		Number:  number,
		Project: domain.ProjectID(r.URL.Query().Get("project")),
	}
	var res proto.PatentInventorStatsResult
	s.call(w, r, proto.MethodPatentInventorStats, params, &res)
}

// handleSourceBibsList returns each source's full bibliographic snapshot for the
// all-fields side-by-side comparison view.
func (s *Server) handleSourceBibsList(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	var res proto.SourceBibsListResult
	s.call(w, r, proto.MethodSourceBibsList, proto.SourceBibsListParams{Number: number}, &res)
}

// handlePatentDelete permanently removes one patent.
func (s *Server) handlePatentDelete(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	var res proto.Empty
	s.call(w, r, proto.MethodPatentDelete, proto.PatentDeleteParams{Number: number}, &res)
}

// handlePatentClearCache clears parsed body cache for one or more patents.
func (s *Server) handlePatentClearCache(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	var body struct {
		Patents []domain.PatentNumber `json:"patents"`
	}
	if r.ContentLength > 0 {
		if !decodeBody(w, r, &body) {
			return
		}
	}
	patents := body.Patents
	if len(patents) == 0 {
		patents = []domain.PatentNumber{number}
	}
	var res proto.PatentClearCacheResult
	s.call(w, r, proto.MethodPatentClearCache, proto.PatentClearCacheParams{Patents: patents}, &res)
}

// handlePatentTagAssignLoose assigns a tag using tag.assign (creates taxonomy if needed).
func (s *Server) handlePatentTagAssignLoose(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	var body struct {
		Project domain.ProjectID `json:"project"`
		Name    string           `json:"name"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Project == "" {
		body.Project = domain.ProjectID(r.URL.Query().Get("project"))
	}
	params := proto.TagParams{
		Project: body.Project,
		Patents: []domain.PatentNumber{number},
		Name:    body.Name,
	}
	var res proto.Empty
	s.call(w, r, proto.MethodTagPatent, params, &res)
}

// handlePatentTagRemoveLoose removes a tag via tag.remove.
func (s *Server) handlePatentTagRemoveLoose(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	project := domain.ProjectID(firstNonEmpty(r.URL.Query().Get("project")))
	params := proto.TagParams{
		Project: project,
		Patents: []domain.PatentNumber{number},
		Name:    r.PathValue("name"),
	}
	var res proto.Empty
	s.call(w, r, proto.MethodUntagPatent, params, &res)
}

// handlePatentDeleteBulk permanently removes the patents named in the body.
func (s *Server) handlePatentDeleteBulk(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Patents []domain.PatentNumber `json:"patents"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var res proto.Empty
	s.call(w, r, proto.MethodPatentDeleteBulk, proto.PatentDeleteBulkParams{Patents: body.Patents}, &res)
}

// handleSourceDiffsList lists all source comparison discrepancies generated for a patent.
func (s *Server) handleSourceDiffsList(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	var res proto.SourceDiffsListResult
	s.call(w, r, proto.MethodSourceDiffsList, proto.SourceDiffsListParams{Number: number}, &res)
}

// handleSourceResolveDiffs resolves source comparison discrepancies for a patent.
func (s *Server) handleSourceResolveDiffs(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	var body struct {
		Diffs []domain.SourceDiff `json:"diffs"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	params := proto.SourceResolveDiffsParams{
		Number: number,
		Diffs:  body.Diffs,
	}
	var res proto.SourceResolveDiffsResult
	s.call(w, r, proto.MethodSourceResolveDiffs, params, &res)
}
