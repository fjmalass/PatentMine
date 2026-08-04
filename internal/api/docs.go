package api

import (
	"net/http"
	"strings"

	"patentmine/internal/proto"
)

func (s *Server) handleDocsList(w http.ResponseWriter, r *http.Request) {
	var res proto.DocsListResult
	s.call(w, r, proto.MethodDocsList, nil, &res)
}

func (s *Server) handleDocsGet(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.PathValue("id"), "/")
	var res proto.DocsGetResult
	s.call(w, r, proto.MethodDocsGet, proto.DocsGetParams{ID: id, Mode: r.URL.Query().Get("mode")}, &res)
}
