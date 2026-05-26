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

// handleIDSPDF renders the IDS bundle (PTO/SB/08a + 08c) to disk.
// Optional JSON body fields override defaults: cumulative_count, fee_amount,
// deposit_account, signer_name, signer_signature, signer_reg_number.
func (s *Server) handleIDSPDF(w http.ResponseWriter, r *http.Request) {
	params := proto.IDSPDFExportParams{Project: domain.ProjectID(r.PathValue("id"))}
	if r.ContentLength > 0 {
		if !decodeBody(w, r, &params) {
			return
		}
		params.Project = domain.ProjectID(r.PathValue("id"))
	}
	var res proto.IDSPDFExportResult
	s.call(w, r, proto.MethodIDSPDFExport, params, &res)
}

// handleProjectUpdate persists changes to a project's mutable fields.
func (s *Server) handleProjectUpdate(w http.ResponseWriter, r *http.Request) {
	var body domain.Project
	if !decodeBody(w, r, &body) {
		return
	}
	body.ID = domain.ProjectID(r.PathValue("id"))
	var res proto.ProjectResult
	s.call(w, r, proto.MethodProjectUpdate, proto.ProjectUpdateParams{Project: body}, &res)
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
		Source domain.Source       `json:"source"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	params := proto.MembershipParams{
		Project: domain.ProjectID(r.PathValue("id")),
		Patent:  body.Patent,
		Source:  body.Source,
	}
	var res proto.Empty
	s.call(w, r, proto.MethodMembershipAdd, params, &res)
}
