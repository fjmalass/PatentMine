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

// handleIDSPDFPreview returns a dry-run summary of an IDS export without
// writing any files.
func (s *Server) handleIDSPDFPreview(w http.ResponseWriter, r *http.Request) {
	params := proto.IDSPDFExportParams{Project: domain.ProjectID(r.PathValue("id"))}
	if r.ContentLength > 0 {
		if !decodeBody(w, r, &params) {
			return
		}
		params.Project = domain.ProjectID(r.PathValue("id"))
	}
	var res proto.IDSPDFPreviewResult
	s.call(w, r, proto.MethodIDSPDFPreview, params, &res)
}

// handleIDSBulkSetStatus applies one IDS status to every patent in the body.
func (s *Server) handleIDSBulkSetStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Patents       []domain.PatentNumber `json:"patents"`
		Status        domain.IDSEntryStatus `json:"status"`
		DefaultInFull bool                  `json:"default_in_full"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	params := proto.IDSEntryBulkSetStatusParams{
		Project:       domain.ProjectID(r.PathValue("id")),
		Patents:       body.Patents,
		Status:        body.Status,
		DefaultInFull: body.DefaultInFull,
	}
	var res proto.IDSEntriesResult
	s.call(w, r, proto.MethodIDSEntryBulkSetStatus, params, &res)
}

// handleIDSEntryGet returns one project/patent IDS entry.
func (s *Server) handleIDSEntryGet(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	params := proto.IDSEntryParams{Project: domain.ProjectID(r.PathValue("id")), Patent: number}
	var res proto.IDSEntryResult
	s.call(w, r, proto.MethodIDSEntryGet, params, &res)
}

// handleIDSEntrySave inserts or updates one project/patent IDS entry. The path
// owns the project and patent; the body carries the remaining fields.
func (s *Server) handleIDSEntrySave(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	var entry domain.IDSEntry
	if !decodeBody(w, r, &entry) {
		return
	}
	entry.Project = domain.ProjectID(r.PathValue("id"))
	entry.Patent = number
	var res proto.IDSEntryResult
	s.call(w, r, proto.MethodIDSEntrySave, proto.IDSEntrySaveParams{Entry: entry}, &res)
}

// handleIDSEntryDelete removes one project/patent IDS entry.
func (s *Server) handleIDSEntryDelete(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	params := proto.IDSEntryParams{Project: domain.ProjectID(r.PathValue("id")), Patent: number}
	var res proto.Empty
	s.call(w, r, proto.MethodIDSEntryDelete, params, &res)
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

// handleAddRelated grants membership to family-graph neighbors of a patent.
func (s *Server) handleAddRelated(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Patent domain.PatentNumber `json:"patent"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	params := proto.AddRelatedParams{
		Project: domain.ProjectID(r.PathValue("id")),
		Patent:  body.Patent,
	}
	var res proto.AddRelatedResult
	s.call(w, r, proto.MethodAddRelated, params, &res)
}

// handleAddMember adds a patent to a project.
func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Patent       domain.PatentNumber `json:"patent"`
		Source       domain.Source       `json:"source"`
		ConfirmMerge bool                `json:"confirm_merge"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	params := proto.MembershipParams{
		Project:      domain.ProjectID(r.PathValue("id")),
		Patent:       body.Patent,
		Source:       body.Source,
		ConfirmMerge: body.ConfirmMerge,
	}
	var res proto.MembershipAddResult
	s.call(w, r, proto.MethodMembershipAdd, params, &res)
}
