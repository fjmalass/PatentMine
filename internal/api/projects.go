package api

import (
	"context"
	"net/http"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
)

// handleNotesExport exports all notes for a project as markdown.
// Query params: sort_by=date|patent (default: date)
// Responds with text/markdown content.
func (s *Server) handleNotesExport(w http.ResponseWriter, r *http.Request) {
	projectID := domain.ProjectID(r.PathValue("id"))
	sortBy := proto.NoteSortBy(firstNonEmpty(r.URL.Query().Get("sort_by"), string(proto.NoteSortByDate)))
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var res proto.PatentNoteExportResult
	if err := s.client.Call(ctx, proto.MethodPatentNoteExport,
		proto.PatentNoteExportParams{Project: projectID, SortBy: sortBy}, &res); err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write([]byte(res.Content))
}

// handleIDS builds the Information Disclosure Statement for a project.
func (s *Server) handleIDS(w http.ResponseWriter, r *http.Request) {
	var res proto.IDSResult
	s.call(w, r, proto.MethodIDSExport,
		proto.IDSExportParams{Project: domain.ProjectID(r.PathValue("id"))}, &res)
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
