package api

import (
	"net/http"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
)

// handleNoteList lists every project-scoped patent note. Query: sort_by=date|patent.
func (s *Server) handleNoteList(w http.ResponseWriter, r *http.Request) {
	sortBy := proto.NoteSortBy(firstNonEmpty(r.URL.Query().Get("sort_by"), string(proto.NoteSortByDate)))
	params := proto.PatentNoteListParams{
		Project: domain.ProjectID(r.PathValue("id")),
		SortBy:  sortBy,
	}
	var res proto.PatentNoteListResult
	s.call(w, r, proto.MethodPatentNoteList, params, &res)
}

// handleNoteGet returns one project-scoped patent note.
func (s *Server) handleNoteGet(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	params := proto.PatentNoteParams{Project: domain.ProjectID(r.PathValue("id")), Patent: number}
	var res proto.PatentNoteResult
	s.call(w, r, proto.MethodPatentNoteGet, params, &res)
}

// handleNoteSave inserts or updates one project-scoped patent note. The path
// owns the project and patent; the body carries the markdown.
func (s *Server) handleNoteSave(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	var note domain.PatentNote
	if !decodeBody(w, r, &note) {
		return
	}
	note.Project = domain.ProjectID(r.PathValue("id"))
	note.Patent = number
	var res proto.PatentNoteResult
	s.call(w, r, proto.MethodPatentNoteSave, proto.PatentNoteSaveParams{Note: note}, &res)
}

// handleNoteDelete removes one project-scoped patent note.
func (s *Server) handleNoteDelete(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	params := proto.PatentNoteParams{Project: domain.ProjectID(r.PathValue("id")), Patent: number}
	var res proto.Empty
	s.call(w, r, proto.MethodPatentNoteDelete, params, &res)
}
