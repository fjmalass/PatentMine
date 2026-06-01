package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
)

// handleAssigneeHistory returns one patent's record.id-keyed ownership timeline
// (at-grant owner + recorded assignments), oldest first, with current owner(s)
// flagged. Read-only; derived by the last assignment fetch.
func (s *Server) handleAssigneeHistory(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	var res proto.AssigneeHistoryResult
	s.call(w, r, proto.MethodAssigneeHistory, proto.AssigneeHistoryParams{Number: number}, &res)
}

// handleProjectAssignees returns a project's assignee rollup: current owners
// (deduped, live patents only) vs. every assignee ever, plus live/expired/
// not-fetched coverage.
func (s *Server) handleProjectAssignees(w http.ResponseWriter, r *http.Request) {
	var res domain.ProjectAssigneeRollup
	s.call(w, r, proto.MethodProjectAssignees,
		proto.ProjectAssigneesParams{Project: domain.ProjectID(r.PathValue("id"))}, &res)
}

// handleFetchAssignmentsBatch fetches recorded assignments for an explicit list
// of patents. Body: JSON {"numbers": ["US…", …], "include_expired": false}.
// Expired patents are skipped unless include_expired is set.
func (s *Server) handleFetchAssignmentsBatch(w http.ResponseWriter, r *http.Request) {
	params, ok := decodeBatchBody(w, r)
	if !ok {
		return
	}
	var res proto.USPTOFetchAssignmentsBatchResult
	s.call(w, r, proto.MethodUSPTOFetchAssignmentsBatch, params, &res)
}

// handleProjectAssigneesFetch batch-fetches assignments for a project's curated
// (manually-added) members. Body (optional): JSON {"include_expired": false}.
func (s *Server) handleProjectAssigneesFetch(w http.ResponseWriter, r *http.Request) {
	params := proto.USPTOFetchAssignmentsBatchParams{Project: domain.ProjectID(r.PathValue("id"))}
	if raw, _ := io.ReadAll(r.Body); len(strings.TrimSpace(string(raw))) > 0 {
		var body struct {
			IncludeExpired bool `json:"include_expired"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			badRequest(w, "invalid JSON body: "+err.Error())
			return
		}
		params.IncludeExpired = body.IncludeExpired
	}
	var res proto.USPTOFetchAssignmentsBatchResult
	s.call(w, r, proto.MethodUSPTOFetchAssignmentsBatch, params, &res)
}

// decodeBatchBody parses {"numbers":[…],"include_expired":bool}, validating each
// patent number. Returns ok=false (after writing the error) on malformed input.
func decodeBatchBody(w http.ResponseWriter, r *http.Request) (proto.USPTOFetchAssignmentsBatchParams, bool) {
	var params proto.USPTOFetchAssignmentsBatchParams
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, err)
		return params, false
	}
	var body struct {
		Numbers        []string `json:"numbers"`
		IncludeExpired bool     `json:"include_expired"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		badRequest(w, "invalid JSON body: "+err.Error())
		return params, false
	}
	if len(body.Numbers) == 0 {
		badRequest(w, "numbers must be a non-empty array of patent numbers")
		return params, false
	}
	for _, s := range body.Numbers {
		n, perr := domain.ParsePatentNumber(s)
		if perr != nil {
			badRequest(w, "invalid patent number "+s+": "+perr.Error())
			return params, false
		}
		params.Numbers = append(params.Numbers, n)
	}
	params.IncludeExpired = body.IncludeExpired
	return params, true
}
