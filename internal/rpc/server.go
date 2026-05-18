// Package rpc carries the proto contract over a unix-domain socket: a Server
// hosted by the daemon and a Client used by every thin frontend.
package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/engine"
	"patentmine/internal/proto"
	"patentmine/internal/store"
	appversion "patentmine/internal/version"
)

const slowRPCMethod = 150 * time.Millisecond

// ErrBadParams marks a request whose params failed to decode.
var ErrBadParams = errors.New("rpc: bad params")

// handlerFunc processes one request's params and returns a result payload.
type handlerFunc func(ctx context.Context, params json.RawMessage) (any, error)

// Server dispatches proto requests to engine operations and forwards engine
// events to every connected client.
type Server struct {
	engine   *engine.Engine
	handlers map[proto.Method]handlerFunc
}

// NewServer wires the dispatch table for an engine.
func NewServer(eng *engine.Engine) *Server {
	s := &Server{engine: eng}
	s.handlers = map[proto.Method]handlerFunc{
		proto.MethodPing:            s.ping,
		proto.MethodPatentGet:       s.patentGet,
		proto.MethodPatentList:      s.patentList,
		proto.MethodProjectList:     s.projectList,
		proto.MethodProjectCreate:   s.projectCreate,
		proto.MethodMembershipAdd:   s.membershipAdd,
		proto.MethodMembershipState: s.membershipState,
		proto.MethodIngestFamily:    s.ingestFamily,
		proto.MethodIngestCancel:    s.ingestCancel,
		proto.MethodImportFile:      s.importFile,
		proto.MethodRelations:       s.relations,
		proto.MethodIDSExport:       s.idsExport,
		proto.MethodMetricsGet:      s.metricsGet,
	}
	return s
}

// Serve accepts connections on ln until ctx is cancelled.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("rpc: accept: %w", err)
		}
		go s.serveConn(ctx, conn)
	}
}

// serveConn handles one client: a goroutine forwards engine events while the
// main loop reads and dispatches requests.
func (s *Server) serveConn(ctx context.Context, nc net.Conn) {
	conn := proto.NewConn(nc)
	defer func() { _ = conn.Close() }()

	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	events, unsub := s.engine.Subscribe()
	defer unsub()
	go func() {
		for {
			select {
			case <-connCtx.Done():
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				if err := conn.WriteMessage(ev); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	for {
		raw, err := conn.ReadMessage()
		if err != nil {
			cancel()
			return
		}
		var req proto.Request
		if err := json.Unmarshal(raw, &req); err != nil {
			_ = conn.WriteMessage(errorReply(0, proto.CodeParse, "malformed request"))
			continue
		}
		// Each request runs in its own goroutine; the Conn serializes writes,
		// so a slow handler never blocks other requests on the connection.
		go s.handle(connCtx, conn, req)
	}
}

func (s *Server) handle(ctx context.Context, conn *proto.Conn, req proto.Request) {
	start := time.Now()
	h, ok := s.handlers[req.Method]
	if !ok {
		s.observeRPC(req.Method, start, true)
		_ = conn.WriteMessage(errorReply(req.ID, proto.CodeNoMethod,
			fmt.Sprintf("unknown method %q", req.Method)))
		return
	}
	result, err := h(ctx, req.Params)
	if err != nil {
		s.observeRPC(req.Method, start, true)
		_ = conn.WriteMessage(errorReply(req.ID, codeFor(err), err.Error()))
		return
	}
	payload, err := json.Marshal(result)
	if err != nil {
		s.observeRPC(req.Method, start, true)
		_ = conn.WriteMessage(errorReply(req.ID, proto.CodeInternal, err.Error()))
		return
	}
	s.observeRPC(req.Method, start, false)
	_ = conn.WriteMessage(proto.Reply{JSONRPC: proto.Version, ID: req.ID, Result: payload})
}

func (s *Server) observeRPC(method proto.Method, start time.Time, failed bool) {
	if s.engine == nil {
		return
	}
	if metrics := s.engineMetrics(); metrics != nil {
		d := time.Since(start)
		metrics.ObserveDuration("rpc.method."+string(method), d, failed)
		metrics.IncCounter("rpc.method."+string(method)+".total", 1)
		if failed {
			metrics.IncCounter("rpc.method."+string(method)+".error_total", 1)
		}
		if d >= slowRPCMethod {
			s.engine.Logger().Warn("slow rpc method",
				slog.String("method", string(method)),
				slog.Int64("duration_ms", d.Milliseconds()),
				slog.Bool("failed", failed))
		}
	}
}

func (s *Server) engineMetrics() interface{ ObserveDuration(string, time.Duration, bool); IncCounter(string, int64) } {
	return s.engineMetricsRef()
}

func (s *Server) engineMetricsRef() interface{ ObserveDuration(string, time.Duration, bool); IncCounter(string, int64) } {
	if s.engine == nil {
		return nil
	}
	return s.engine.Metrics()
}

func (s *Server) Logger() *slog.Logger {
	if s.engine == nil {
		return slog.Default()
	}
	return s.engine.Logger()
}

// errorReply builds a JSON-RPC error response.
func errorReply(id uint64, code int, message string) proto.Reply {
	return proto.Reply{
		JSONRPC: proto.Version,
		ID:      id,
		Error:   &proto.Error{Code: code, Message: message},
	}
}

// codeFor maps an engine/store error to a JSON-RPC error code.
func codeFor(err error) int {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return proto.CodeNotFound
	case errors.Is(err, ErrBadParams):
		return proto.CodeBadParams
	case errors.Is(err, engine.ErrQueueFull):
		return proto.CodeBusy
	default:
		return proto.CodeInternal
	}
}

// decodeParams unmarshals request params into T, tagging failures as bad params.
func decodeParams[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(raw) == 0 {
		return v, nil
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, fmt.Errorf("%w: %v", ErrBadParams, err)
	}
	return v, nil
}

// --- handlers ---

func (s *Server) ping(context.Context, json.RawMessage) (any, error) {
	return proto.PingResult{Pong: true, Version: appversion.String()}, nil
}

func (s *Server) patentGet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentGetParams](raw)
	if err != nil {
		return nil, err
	}
	patent, err := s.engine.Patent(ctx, p.Number)
	if err != nil {
		return nil, err
	}
	return proto.PatentResult{Patent: patent}, nil
}

func (s *Server) patentList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentListParams](raw)
	if err != nil {
		return nil, err
	}
	patents, total, err := s.engine.ListPatents(ctx, store.PatentQuery{
		Project: domain.ProjectID(p.Project),
		State:   domain.MembershipState(p.State),
		Search:  p.Search,
		Limit:   p.Limit,
		Offset:  p.Offset,
	})
	if err != nil {
		return nil, err
	}
	return proto.PatentListResult{Patents: patents, Total: total}, nil
}

func (s *Server) projectList(ctx context.Context, _ json.RawMessage) (any, error) {
	projects, err := s.engine.Projects(ctx)
	if err != nil {
		return nil, err
	}
	return proto.ProjectListResult{Projects: projects}, nil
}

func (s *Server) projectCreate(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ProjectCreateParams](raw)
	if err != nil {
		return nil, err
	}
	project, err := s.engine.CreateProject(ctx, p.Name)
	if err != nil {
		return nil, err
	}
	return proto.ProjectResult{Project: project}, nil
}

func (s *Server) membershipAdd(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MembershipParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.AddToProject(ctx, p.Project, p.Patent); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) membershipState(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MembershipStateParams](raw)
	if err != nil {
		return nil, err
	}
	state, err := domain.ParseMembershipState(p.State)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadParams, err)
	}
	if err := s.engine.SetMembershipState(ctx, p.Project, p.Patent, state); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) ingestFamily(_ context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.IngestFamilyParams](raw)
	if err != nil {
		return nil, err
	}
	id, err := s.engine.StartFamilyIngest(p.Root, p.Depth, p.Force)
	if err != nil {
		return nil, err
	}
	return proto.IngestStartResult{JobID: string(id)}, nil
}

func (s *Server) importFile(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ImportFileParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.ImportFile(ctx, p.Path); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) ingestCancel(_ context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.IngestCancelParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.CancelIngest(engine.JobID(p.JobID)); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) relations(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.RelationsParams](raw)
	if err != nil {
		return nil, err
	}
	kind, err := domain.ParseRelationKind(p.Kind)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadParams, err)
	}
	rels, err := s.engine.Relations(ctx, p.Number, kind)
	if err != nil {
		return nil, err
	}
	return proto.RelationsResult{Relations: rels}, nil
}

func (s *Server) idsExport(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.IDSExportParams](raw)
	if err != nil {
		return nil, err
	}
	ids, err := s.engine.ExportIDS(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.IDSResult{IDS: ids}, nil
}

func (s *Server) metricsGet(context.Context, json.RawMessage) (any, error) {
	return proto.MetricsResult{Metrics: s.engine.MetricsSnapshot()}, nil
}
