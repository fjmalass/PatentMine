package api

import (
	"net/http"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
)

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
