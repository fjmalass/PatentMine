package api

import (
	"net/http"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
)

// handleClassificationStats returns classification stats for a project.
func (s *Server) handleClassificationStats(w http.ResponseWriter, r *http.Request) {
	var res proto.PatentClassificationStatsResult
	s.call(w, r, proto.MethodPatentClassificationStats, proto.PatentClassificationStatsParams{Project: domain.ProjectID(r.URL.Query().Get("project"))}, &res)
}

func (s *Server) handleClassificationList(w http.ResponseWriter, r *http.Request) {
	var res proto.ClassificationListResult
	s.call(w, r, proto.MethodClassificationList, nil, &res)
}

func (s *Server) handleClassificationGet(w http.ResponseWriter, r *http.Request) {
	params := proto.ClassificationGetParams{
		System: r.PathValue("system"),
		Code:   r.PathValue("code"),
	}
	var res domain.Classification
	s.call(w, r, proto.MethodClassificationGet, params, &res)
}

func (s *Server) handleClassificationSave(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Classification domain.Classification `json:"classification"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var res proto.Empty
	s.call(w, r, proto.MethodClassificationSave, proto.ClassificationParams{Classification: body.Classification}, &res)
}

func (s *Server) handleClassificationDelete(w http.ResponseWriter, r *http.Request) {
	params := proto.ClassificationDeleteParams{
		System: r.PathValue("system"),
		Code:   r.PathValue("code"),
	}
	var res proto.Empty
	s.call(w, r, proto.MethodClassificationDelete, params, &res)
}

func (s *Server) handleClassificationLookup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var res domain.Classification
	s.call(w, r, proto.MethodClassificationLookup, proto.ClassificationLookupParams{Code: body.Code}, &res)
}

func (s *Server) handleClassificationListByCodes(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Codes []string `json:"codes"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	var res proto.ClassificationListResult
	s.call(w, r, proto.MethodClassificationListByCodes, proto.ClassificationListByCodesParams{Codes: body.Codes}, &res)
}

func (s *Server) handlePatentClassificationList(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	params := proto.PatentClassificationListParams{
		Project: domain.ProjectID(r.PathValue("id")),
		Patent:  number,
	}
	var res proto.ClassificationListResult
	s.call(w, r, proto.MethodPatentClassificationList, params, &res)
}
