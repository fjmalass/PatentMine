package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
)

// handleAddedExport returns the plain-text list of a project's manually-added
// patents (the :export.added format), suitable for re-import via
// handleAddedImport.
func (s *Server) handleAddedExport(w http.ResponseWriter, r *http.Request) {
	projectID := domain.ProjectID(r.PathValue("id"))
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var res proto.AddedExportResult
	if err := s.client.Call(ctx, proto.MethodAddedExport,
		proto.AddedExportParams{Project: projectID}, &res); err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Patent-Count", strconv.Itoa(res.Count))
	_, _ = w.Write([]byte(res.Content))
}

// handleAddedImport bulk-adds the patents listed in the request body into a
// project with manual provenance. The body may be the raw list text
// (Content-Type text/plain) or a JSON object {"content": "...", "path": "..."}.
func (s *Server) handleAddedImport(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, err)
		return
	}
	params := proto.AddedImportParams{Project: domain.ProjectID(r.PathValue("id"))}
	if trimmed := strings.TrimSpace(string(raw)); strings.HasPrefix(trimmed, "{") {
		var body struct {
			Content string `json:"content"`
			Path    string `json:"path"`
		}
		if jsonErr := json.Unmarshal(raw, &body); jsonErr != nil {
			badRequest(w, "invalid JSON body: "+jsonErr.Error())
			return
		}
		params.Content = body.Content
		params.Path = body.Path
	} else {
		params.Content = string(raw)
	}
	var res proto.AddedImportResult
	s.call(w, r, proto.MethodAddedImport, params, &res)
}
