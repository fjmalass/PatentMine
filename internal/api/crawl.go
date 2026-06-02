package api

import (
	"net/http"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
)

// handleCrawlConfig returns daemon-owned crawl defaults.
func (s *Server) handleCrawlConfig(w http.ResponseWriter, r *http.Request) {
	var res proto.CrawlConfigResult
	s.call(w, r, proto.MethodCrawlConfig, nil, &res)
}

// handleSourceModeGet returns the runtime provider mode.
func (s *Server) handleSourceModeGet(w http.ResponseWriter, r *http.Request) {
	var res proto.SourceModeResult
	s.call(w, r, proto.MethodSourceModeGet, nil, &res)
}

// handleSourceModeSet changes the runtime provider mode.
func (s *Server) handleSourceModeSet(w http.ResponseWriter, r *http.Request) {
	var body proto.SourceModeParams
	if !decodeBody(w, r, &body) {
		return
	}
	var res proto.SourceModeResult
	s.call(w, r, proto.MethodSourceModeSet, body, &res)
}

// handleCrawl starts a family-graph crawl.
func (s *Server) handleCrawl(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Project domain.ProjectID    `json:"project"`
		Root    domain.PatentNumber `json:"root"`
		Depth   int                 `json:"depth"`
		Profile domain.CrawlProfile `json:"profile"`
		Force   bool                `json:"force"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Profile == "" {
		body.Profile = domain.CrawlProfileFamily
	}
	var res proto.CrawlStartResult
	s.call(w, r, proto.MethodCrawlFamily, proto.CrawlFamilyParams{
		Project: body.Project,
		Root:    body.Root,
		Depth:   body.Depth,
		Profile: body.Profile,
		Force:   body.Force,
	}, &res)
}

// handleCrawlCancel cancels a running crawl job.
func (s *Server) handleCrawlCancel(w http.ResponseWriter, r *http.Request) {
	var body proto.CrawlCancelParams
	if !decodeBody(w, r, &body) {
		return
	}
	var res proto.Empty
	s.call(w, r, proto.MethodCrawlCancel, body, &res)
}

// handleImportFile loads a patent record from a local fixture file. Body: path.
func (s *Server) handleImportFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var res proto.Empty
	s.call(w, r, proto.MethodImportFile, proto.ImportFileParams{Path: body.Path}, &res)
}
