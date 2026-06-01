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
		Root  domain.PatentNumber `json:"root"`
		Depth int                 `json:"depth"`
		Force bool                `json:"force"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var res proto.CrawlStartResult
	s.call(w, r, proto.MethodCrawlFamily,
		proto.CrawlFamilyParams{Root: body.Root, Depth: body.Depth, Force: body.Force}, &res)
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
