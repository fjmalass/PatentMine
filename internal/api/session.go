package api

import (
	"encoding/json"
	"net/http"
	"sync"

	"patentmine/internal/domain"
)

// SessionState is shared UI context for web clients (and optional cross-tab sync).
type SessionState struct {
	ActiveProject domain.ProjectID    `json:"active_project,omitempty"`
	FocusedPatent domain.PatentNumber `json:"focused_patent,omitempty"`
	View          string              `json:"view,omitempty"`
	Filter        string              `json:"filter,omitempty"`
	TableViewID   string              `json:"table_view_id,omitempty"`
}

type sessionStore struct {
	mu    sync.RWMutex
	state SessionState
}

func (s *sessionStore) get() SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *sessionStore) set(st SessionState) SessionState {
	s.mu.Lock()
	s.state = st
	s.mu.Unlock()
	return st
}

func (s *sessionStore) patch(body SessionState) SessionState {
	s.mu.Lock()
	if body.ActiveProject != "" {
		s.state.ActiveProject = body.ActiveProject
	}
	if !body.FocusedPatent.IsZero() {
		s.state.FocusedPatent = body.FocusedPatent
	}
	if body.View != "" {
		s.state.View = body.View
	}
	if body.Filter != "" {
		s.state.Filter = body.Filter
	}
	if body.TableViewID != "" {
		s.state.TableViewID = body.TableViewID
	}
	out := s.state
	s.mu.Unlock()
	return out
}

func (s *Server) handleSessionGet(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.session.get())
}

func (s *Server) handleSessionPut(w http.ResponseWriter, r *http.Request) {
	var body SessionState
	if !decodeBody(w, r, &body) {
		return
	}
	st := s.session.set(body)
	if s.events != nil {
		data, _ := json.Marshal(map[string]any{"method": "session.changed", "params": st})
		s.events.broadcast(data)
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleSessionPatch(w http.ResponseWriter, r *http.Request) {
	var body SessionState
	if !decodeBody(w, r, &body) {
		return
	}
	st := s.session.patch(body)
	if s.events != nil {
		data, _ := json.Marshal(map[string]any{"method": "session.changed", "params": st})
		s.events.broadcast(data)
	}
	writeJSON(w, http.StatusOK, st)
}
