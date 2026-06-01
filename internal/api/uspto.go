package api

import (
	"net/http"
	"strconv"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
)

// uspToXMLKind reads the ?kind= query param, defaulting to grant.
func uspToXMLKind(r *http.Request) proto.USPTOXMLKind {
	if kind := proto.USPTOXMLKind(r.URL.Query().Get("kind")); kind != "" {
		return kind
	}
	return proto.USPTOXMLKindGrant
}

// handleUSPTOFetchXML downloads the grant or pgpub XML for a patent. Query:
// kind=grant|pgpub (default grant).
func (s *Server) handleUSPTOFetchXML(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	params := proto.USPTOFetchXMLParams{Number: number, Kind: uspToXMLKind(r)}
	var res proto.USPTOFetchXMLResult
	s.call(w, r, proto.MethodUSPTOFetchXML, params, &res)
}

// handleUSPTOGrantBody returns the parsed grant/pgpub body for a patent. Query:
// kind=grant|pgpub (default grant, with daemon fallback to pgpub).
func (s *Server) handleUSPTOGrantBody(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	params := proto.USPTOGrantBodyParams{Number: number, Kind: uspToXMLKind(r)}
	var res proto.USPTOGrantBodyResult
	s.call(w, r, proto.MethodUSPTOGrantBody, params, &res)
}

// handleUSPTOFetchAssignments fetches and stores a patent's recorded assignment
// chain from the USPTO Patent Assignment Search API.
func (s *Server) handleUSPTOFetchAssignments(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	var res proto.USPTOFetchAssignmentsResult
	s.call(w, r, proto.MethodUSPTOFetchAssignments, proto.USPTOFetchAssignmentsParams{Number: number}, &res)
}

// handleUSPTOAssignmentList returns a patent's persisted assignment chain.
func (s *Server) handleUSPTOAssignmentList(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	var res proto.USPTOAssignmentListResult
	s.call(w, r, proto.MethodUSPTOAssignmentList, proto.USPTOAssignmentListParams{Number: number}, &res)
}

// handleExpirationCalculate computes (and persists) a patent's statutory U.S.
// expiration date. Query: project, refresh=true forces re-fetch of metadata.
func (s *Server) handleExpirationCalculate(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	refresh, _ := strconv.ParseBool(r.URL.Query().Get("refresh"))
	params := proto.USPTOExpirationCalculateParams{
		Number:    number,
		ProjectID: r.URL.Query().Get("project"),
		Refresh:   refresh,
	}
	var res proto.USPTOExpirationCalculateResult
	s.call(w, r, proto.MethodUSPTOExpirationCalculate, params, &res)
}
