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
