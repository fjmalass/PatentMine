// Package api is the web frontend. Like the TUI it is a thin client: it owns
// no database and no business logic, only translating HTTP requests into proto
// calls to the daemon. Its routes mirror the command registry, so the web API
// and the TUI expose the same set of actions.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"patentmine/internal/command"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
)

// apiCallTimeout bounds one daemon call made on behalf of an HTTP request.
const apiCallTimeout = 20 * time.Second

// Server is the HTTP front end. It forwards every request to the daemon.
type Server struct {
	client   *rpc.Client
	registry *command.Registry
	mux      *http.ServeMux
}

// NewServer builds the HTTP server and registers its routes.
func NewServer(client *rpc.Client, registry *command.Registry) *Server {
	s := &Server{client: client, registry: registry, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler exposes the router, mainly for tests.
func (s *Server) Handler() http.Handler { return s.mux }

// ListenAndServe serves HTTP on addr until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("api: serve: %w", err)
	}
	return nil
}

// call forwards a request to the daemon and writes the JSON result, or an
// error response with an HTTP status mapped from the proto error code.
func (s *Server) call(w http.ResponseWriter, r *http.Request, method proto.Method, params, result any) {
	ctx, cancel := context.WithTimeout(r.Context(), apiCallTimeout)
	defer cancel()
	if err := s.client.Call(ctx, method, params, result); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// writeJSON encodes v as a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError maps a daemon/proto error to an HTTP status and JSON body.
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if protoErr, ok := errors.AsType[*proto.Error](err); ok {
		switch protoErr.Code {
		case proto.CodeNotFound, proto.CodeNoMethod:
			status = http.StatusNotFound
		case proto.CodeBadParams, proto.CodeParse, proto.CodeInvalidReq:
			status = http.StatusBadRequest
		}
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// badRequest writes a 400 with a message.
func badRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": msg})
}
