package api

import (
	"context"
	"net/http"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
)

func (s *Server) handleTableViewList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	params := proto.TableViewListParams{
		Owner:     q.Get("owner"),
		TableType: domain.TableType(q.Get("table_type")),
	}
	var res proto.TableViewListResult
	s.call(w, r, proto.MethodTableViewList, params, &res)
}

func (s *Server) handleTableViewGet(w http.ResponseWriter, r *http.Request) {
	params := proto.TableViewGetParams{
		Owner: r.URL.Query().Get("owner"),
		ID:    r.PathValue("id"),
	}
	var res proto.TableViewResult
	if !s.callResult(w, r, proto.MethodTableViewGet, params, &res) {
		return
	}
	s.observeTableView(r, observability.ActionTableViewSelect, res.View)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleTableViewSave(w http.ResponseWriter, r *http.Request) {
	var view domain.SavedTableView
	if !decodeBody(w, r, &view) {
		return
	}
	if view.Owner == "" {
		view.Owner = r.URL.Query().Get("owner")
	}
	var res proto.TableViewResult
	if !s.callResult(w, r, proto.MethodTableViewSave, proto.TableViewSaveParams{View: view}, &res) {
		return
	}
	s.observeTableView(r, observability.ActionTableViewSave, res.View)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleTableViewDelete(w http.ResponseWriter, r *http.Request) {
	params := proto.TableViewGetParams{
		Owner: r.URL.Query().Get("owner"),
		ID:    r.PathValue("id"),
	}
	var existing proto.TableViewResult
	if !s.callResult(w, r, proto.MethodTableViewGet, params, &existing) {
		return
	}
	var res proto.Empty
	if !s.callResult(w, r, proto.MethodTableViewDelete, params, &res) {
		return
	}
	s.observeTableView(r, observability.ActionTableViewDelete, existing.View)
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) callResult(w http.ResponseWriter, r *http.Request, method proto.Method, params, result any) bool {
	ctx, cancel := context.WithTimeout(r.Context(), apiCallTimeout)
	defer cancel()
	if err := s.client.Call(ctx, method, params, result); err != nil {
		writeError(w, err)
		return false
	}
	return true
}
