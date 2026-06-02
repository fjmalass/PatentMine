package api

import (
	"net/http"

	"patentmine/internal/filterexpr"
)

// handleFilterValidate parses a filter expression and returns the canonical form.
func (s *Server) handleFilterValidate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Expression    string `json:"expression"`
		ProjectActive bool   `json:"project_active"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	expr, err := filterexpr.Parse(body.Expression)
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	if err := filterexpr.ValidateProjectScope(expr, body.ProjectActive); err != nil {
		badRequest(w, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"canonical": filterexpr.CanonicalString(expr),
	})
}
