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
	"os"
	"strings"
	"sync"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/engine"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/store"
	appversion "patentmine/internal/version"
)

const slowRPCMethod = 150 * time.Millisecond
const slowTagSortRPC = 150 * time.Millisecond

// ErrBadParams marks a request whose params failed to decode.
var ErrBadParams = errors.New("rpc: bad params")

// handlerFunc processes one request's params and returns a result payload.
type handlerFunc func(ctx context.Context, params json.RawMessage) (any, error)

// Server dispatches proto requests to engine operations and forwards engine
// events to every connected client.
type Server struct {
	engine          *engine.Engine
	handlers        map[proto.Method]handlerFunc
	usptoConfigured bool
	activityLogsDir string
	clientMetrics   sync.Map // component string → proto.MetricsSnapshot
}

// Option customizes server behavior outside the engine dispatch table.
type Option func(*Server)

// WithActivityLogsDir lets RPC clients read the daemon's raw and grouped
// activity feeds from the same log directory where daemon mutations are written.
func WithActivityLogsDir(dir string) Option {
	return func(s *Server) { s.activityLogsDir = dir }
}

// NewServer wires the dispatch table for an engine.
func NewServer(eng *engine.Engine, usptoConfigured bool, opts ...Option) *Server {
	s := &Server{engine: eng, usptoConfigured: usptoConfigured}
	for _, opt := range opts {
		opt(s)
	}
	s.handlers = map[proto.Method]handlerFunc{
		proto.MethodPing:                      s.ping,
		proto.MethodPatentGet:                 s.patentGet,
		proto.MethodPatentAssigneeStats:       s.patentAssigneeStats,
		proto.MethodPatentClassificationStats: s.patentClassificationStats,
		proto.MethodPatentInventorStats:       s.patentInventorStats,
		proto.MethodPatentList:                s.patentList,
		proto.MethodPatentTableColumns:        s.patentTableColumns,
		proto.MethodPatentDelete:              s.patentDelete,
		proto.MethodPatentDeleteBulk:          s.patentDeleteBulk,
		proto.MethodProjectList:               s.projectList,
		proto.MethodProjectCreate:             s.projectCreate,
		proto.MethodMembershipAdd:             s.membershipAdd,
		proto.MethodReviewState:               s.reviewState,
		proto.MethodTagPatent:                 s.tagPatent,
		proto.MethodUntagPatent:               s.untagPatent,
		proto.MethodCrawlFamily:               s.crawlFamily,
		proto.MethodCrawlConfig:               s.crawlConfig,
		proto.MethodCrawlCancel:               s.crawlCancel,
		proto.MethodImportFile:                s.importFile,
		proto.MethodRelations:                 s.relations,
		proto.MethodFamilyGraph:               s.familyGraph,
		proto.MethodIDSExport:                 s.idsExport,
		proto.MethodIDSPDFExport:              s.idsPDFExport,
		proto.MethodIDSPDFPreview:             s.idsPDFPreview,
		proto.MethodProjectUpdate:             s.projectUpdate,
		proto.MethodIDSEntryGet:               s.idsEntryGet,
		proto.MethodIDSEntrySave:              s.idsEntrySave,
		proto.MethodIDSEntryDelete:            s.idsEntryDelete,
		proto.MethodPatentNoteGet:             s.patentNoteGet,
		proto.MethodPatentNoteSave:            s.patentNoteSave,
		proto.MethodPatentNoteDelete:          s.patentNoteDelete,
		proto.MethodPatentNoteList:            s.patentNoteList,
		proto.MethodPatentNoteExport:          s.patentNoteExport,
		proto.MethodMetricsGet:                s.metricsGet,
		proto.MethodMetricsPush:               s.metricsPush,
		proto.MethodActivityRaw:               s.activityRaw,
		proto.MethodHistoryFeed:               s.historyFeed,
		proto.MethodTagCreate:                 s.tagCreate,
		proto.MethodTagList:                   s.tagList,
		proto.MethodTagDelete:                 s.tagDelete,
		proto.MethodTagPatentStrict:           s.tagPatentStrict,
		proto.MethodUntagPatentStrict:         s.untagPatentStrict,
		proto.MethodPatentTagList:             s.patentTagList,
		proto.MethodClassificationGet:         s.classificationGet,
		proto.MethodClassificationList:        s.classificationList,
		proto.MethodClassificationSave:        s.classificationSave,
		proto.MethodClassificationDelete:      s.classificationDelete,
		proto.MethodClassificationLookup:      s.classificationLookup,
		proto.MethodClassificationListByCodes: s.classificationListByCodes,
		proto.MethodPatentClassificationList:  s.patentClassificationList,
		proto.MethodTableViewList:             s.tableViewList,
		proto.MethodTableViewGet:              s.tableViewGet,
		proto.MethodTableViewSave:             s.tableViewSave,
		proto.MethodTableViewDelete:           s.tableViewDelete,
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

func (s *Server) engineMetrics() interface {
	ObserveDuration(string, time.Duration, bool)
	IncCounter(string, int64)
	SetGauge(string, int64)
} {
	return s.engineMetricsRef()
}

func (s *Server) engineMetricsRef() interface {
	ObserveDuration(string, time.Duration, bool)
	IncCounter(string, int64)
	SetGauge(string, int64)
} {
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
	return proto.PingResult{Pong: true, Version: appversion.String(), USPTOConfigured: s.usptoConfigured}, nil
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
	result := proto.PatentResult{Patent: patent}
	// State and tags are project-scoped; populate them only when the caller
	// named a project, leaving them empty for a project-independent lookup.
	if p.Project != "" {
		state, ok, err := s.engine.ReviewStateOf(ctx, p.Project, p.Number)
		if err != nil {
			return nil, err
		}
		if ok {
			result.ReviewState = state
		}
		tags, err := s.engine.PatentTags(ctx, p.Project, p.Number)
		if err != nil {
			return nil, err
		}
		result.Tags = tags
		entry, ok, err := s.engine.IDSEntryOf(ctx, p.Project, p.Number)
		if err != nil {
			return nil, err
		}
		if ok {
			result.IDSEntry = &entry
		}
		note, ok, err := s.engine.PatentNoteOf(ctx, p.Project, p.Number)
		if err != nil {
			return nil, err
		}
		if ok {
			result.PatentNote = &note
		}
	}
	return result, nil
}

func (s *Server) patentInventorStats(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentGetParams](raw)
	if err != nil {
		return nil, err
	}
	stats, err := s.engine.PatentInventorStats(ctx, p.Number, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.PatentInventorStatsResult{Stats: stats}, nil
}

func (s *Server) patentAssigneeStats(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentAssigneeStatsParams](raw)
	if err != nil {
		return nil, err
	}
	stats, err := s.engine.PatentAssigneeStats(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.PatentAssigneeStatsResult{Stats: stats}, nil
}

func (s *Server) patentClassificationStats(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentClassificationStatsParams](raw)
	if err != nil {
		return nil, err
	}
	stats, err := s.engine.PatentClassificationStats(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.PatentClassificationStatsResult{Stats: stats}, nil
}

func (s *Server) patentList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentListParams](raw)
	if err != nil {
		return nil, err
	}
	if !domain.PatentTableAllowsSort(p.Project, p.SortColumn) {
		s.Logger().Warn("unsupported patent list sort",
			slog.String("project", string(p.Project)),
			slog.String("sort_column", string(p.SortColumn)))
		if metrics := s.engineMetrics(); metrics != nil {
			metrics.IncCounter("patent_list.sort_unsupported_total", 1)
		}
		return nil, fmt.Errorf("%w: unsupported patent list sort %q", ErrBadParams, p.SortColumn)
	}
	if p.SortColumn != "" {
		if metrics := s.engineMetrics(); metrics != nil {
			metrics.IncCounter("patent_list.sort_request_total", 1)
		}
	}
	var tagSortStart time.Time
	if p.SortColumn == domain.SortByTags {
		tagSortStart = time.Now()
		if metrics := s.engineMetrics(); metrics != nil {
			metrics.IncCounter("patent_list.sort_tags_total", 1)
		}
		defer func() {
			d := time.Since(tagSortStart)
			if metrics := s.engineMetrics(); metrics != nil {
				metrics.ObserveDuration("rpc.method.patent.list.sort_tags", d, err != nil)
				metrics.SetGauge("rpc.method.patent.list.sort_tags.limit", int64(p.Limit))
				metrics.SetGauge("rpc.method.patent.list.sort_tags.offset", int64(p.Offset))
			}
			if d >= slowTagSortRPC {
				s.Logger().Warn("slow patent list tag sort",
					slog.String("project", string(p.Project)),
					slog.Int("limit", p.Limit),
					slog.Int("offset", p.Offset),
					slog.Int64("duration_ms", d.Milliseconds()),
					slog.Bool("failed", err != nil))
			}
		}()
	}
	patents, total, err := s.engine.ListPatents(ctx, store.PatentQuery{
		Numbers:            p.Numbers,
		Project:            p.Project,
		Filter:             p.Filter,
		ReviewState:        p.ReviewState,
		IDSStatus:          p.IDSStatus,
		Search:             p.Search,
		Classification:     p.Classification,
		ClassificationCode: p.ClassificationCode,
		Inventor:           p.Inventor,
		Assignee:           p.Assignee,
		Limit:              p.Limit,
		Offset:             p.Offset,
		SortColumn:         p.SortColumn,
		SortAscending:      p.SortAscending,
	})
	if err != nil {
		return nil, err
	}
	return proto.PatentListResult{Patents: patents, Total: total}, nil
}

func (s *Server) patentTableColumns(_ context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentTableColumnsParams](raw)
	if err != nil {
		return nil, err
	}
	return proto.PatentTableColumnsResult{Columns: domain.PatentTableColumns(p.Project)}, nil
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

func (s *Server) patentDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentDeleteParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeletePatent(ctx, p.Number); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) patentDeleteBulk(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentDeleteBulkParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeletePatents(ctx, p.Patents); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) membershipAdd(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MembershipParams](raw)
	if err != nil {
		return nil, err
	}
	fetchStarted, err := s.engine.AddToProject(ctx, p.Project, p.Patent)
	if err != nil {
		return nil, err
	}
	return proto.MembershipAddResult{FetchStarted: fetchStarted}, nil
}

func (s *Server) reviewState(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ReviewStateParams](raw)
	if err != nil {
		return nil, err
	}
	state, err := domain.ParseReviewState(p.State)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadParams, err)
	}
	if err := s.engine.SetReviewState(ctx, p.Project, p.Patents, state); err != nil {
		return nil, err
	}
	records := make([]domain.PatentNumber, 0, len(p.Patents))
	for _, patent := range p.Patents {
		record, err := s.engine.Patent(ctx, patent)
		if err != nil {
			return nil, err
		}
		records = append(records, record.Number)
	}
	return proto.ReviewStateResult{Patents: records}, nil
}

func (s *Server) tagPatent(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TagParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.TagPatent(ctx, p.Project, p.Patents, p.Name); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) untagPatent(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TagParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.UntagPatent(ctx, p.Project, p.Patents, p.Name); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) tagCreate(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TagCreateParams](raw)
	if err != nil {
		return nil, err
	}
	tag, err := s.engine.CreateTaxonomyTag(ctx, p.Project, p.Name)
	if err != nil {
		return nil, err
	}
	return tag, nil
}

func (s *Server) tagList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TagListParams](raw)
	if err != nil {
		return nil, err
	}
	tags, err := s.engine.ListTaxonomyTags(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.TagListResult{Tags: tags}, nil
}

func (s *Server) tagDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TagDeleteParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeleteTaxonomyTag(ctx, p.Project, p.Name); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) tagPatentStrict(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TagParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.TagPatentStrict(ctx, p.Project, p.Patents, p.Name); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) untagPatentStrict(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TagParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.UntagPatentStrict(ctx, p.Project, p.Patents, p.Name); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) patentTagList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentTagListParams](raw)
	if err != nil {
		return nil, err
	}
	tags, err := s.engine.PatentTags(ctx, p.Project, p.Patent)
	if err != nil {
		return nil, err
	}
	return proto.PatentTagListResult{Tags: tags}, nil
}

func (s *Server) crawlFamily(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.CrawlFamilyParams](raw)
	if err != nil {
		return nil, err
	}
	id, err := s.engine.StartFamilyCrawl(ctx, p.Root, p.Depth, p.Profile, p.Force)
	if err != nil {
		return nil, err
	}
	return proto.CrawlStartResult{JobID: string(id)}, nil
}

func (s *Server) crawlConfig(_ context.Context, _ json.RawMessage) (any, error) {
	return s.engine.CrawlConfig(), nil
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

func (s *Server) crawlCancel(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.CrawlCancelParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.CancelCrawl(ctx, engine.JobID(p.JobID)); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) relations(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.RelationsParams](raw)
	if err != nil {
		return nil, err
	}
	if !domain.PatentTableAllowsSort(p.Project, p.SortColumn) {
		s.Logger().Warn("unsupported patent relations sort",
			slog.String("project", string(p.Project)),
			slog.String("sort_column", string(p.SortColumn)),
			slog.String("kind", string(p.Kind)))
		if metrics := s.engineMetrics(); metrics != nil {
			metrics.IncCounter("patent_relations.sort_unsupported_total", 1)
		}
		return nil, fmt.Errorf("%w: unsupported patent relations sort %q", ErrBadParams, p.SortColumn)
	}
	var tagSortStart time.Time
	if p.SortColumn == domain.SortByTags {
		tagSortStart = time.Now()
		if metrics := s.engineMetrics(); metrics != nil {
			metrics.IncCounter("patent_relations.sort_tags_total", 1)
		}
		defer func() {
			d := time.Since(tagSortStart)
			if metrics := s.engineMetrics(); metrics != nil {
				metrics.ObserveDuration("rpc.method.patent.relations.sort_tags", d, err != nil)
				metrics.SetGauge("rpc.method.patent.relations.sort_tags.limit", int64(p.Limit))
				metrics.SetGauge("rpc.method.patent.relations.sort_tags.offset", int64(p.Offset))
			}
			if d >= slowTagSortRPC {
				s.Logger().Warn("slow patent relations tag sort",
					slog.String("project", string(p.Project)),
					slog.String("kind", string(p.Kind)),
					slog.Int("limit", p.Limit),
					slog.Int("offset", p.Offset),
					slog.Int64("duration_ms", d.Milliseconds()),
					slog.Bool("failed", err != nil))
			}
		}()
	}
	q := store.PatentQuery{
		Relation:       p.Number,
		RelationKind:   p.Kind,
		Project:        p.Project,
		Filter:         p.Filter,
		ReviewState:    p.ReviewState,
		IDSStatus:      p.IDSStatus,
		Search:         p.Search,
		Classification: p.Classification,
		Inventor:       p.Inventor,
		Limit:          p.Limit,
		Offset:         p.Offset,
		SortColumn:     p.SortColumn,
		SortAscending:  p.SortAscending,
	}
	patents, total, err := s.engine.Relations(ctx, q)
	if err != nil {
		return nil, err
	}
	return proto.RelationsResult{Patents: patents, Total: total}, nil
}

func (s *Server) familyGraph(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.FamilyGraphParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.FamilyGraph(ctx, p)
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

func (s *Server) idsPDFExport(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.IDSPDFExportParams](raw)
	if err != nil {
		return nil, err
	}
	res, err := s.engine.ExportIDSPDF(ctx, p.Project, engine.IDSPDFOptions{
		CumulativeCount: p.CumulativeCount,
		FeeAmount:       p.FeeAmount,
		DepositAccount:  p.DepositAccount,
		SignerName:      p.SignerName,
		SignerSignature: p.SignerSignature,
		SignerRegNumber: p.SignerRegNumber,
	})
	if err != nil {
		return nil, err
	}
	return proto.IDSPDFExportResult{
		Dir:       res.Dir,
		Files:     res.Files,
		FeeTier:   res.FeeTier,
		PageCount: res.PageCount,
	}, nil
}

func (s *Server) projectUpdate(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ProjectUpdateParams](raw)
	if err != nil {
		return nil, err
	}
	saved, err := s.engine.UpdateProject(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.ProjectResult{Project: saved}, nil
}

func (s *Server) idsPDFPreview(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.IDSPDFExportParams](raw)
	if err != nil {
		return nil, err
	}
	prev, err := s.engine.PreviewIDSPDF(ctx, p.Project, engine.IDSPDFOptions{
		CumulativeCount: p.CumulativeCount,
	})
	if err != nil {
		return nil, err
	}
	return proto.IDSPDFPreviewResult{
		BaseDir:         prev.BaseDir,
		USCount:         prev.USCount,
		ForeignCount:    prev.ForeignCount,
		Sheets:          prev.Sheets,
		FeeTier:         prev.FeeTier,
		CumulativeCount: prev.CumulativeCount,
		ExistingDirs:    prev.ExistingDirs,
		MissingFields:   prev.MissingFields,
	}, nil
}

func (s *Server) idsEntryGet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.IDSEntryParams](raw)
	if err != nil {
		return nil, err
	}
	entry, ok, err := s.engine.IDSEntryOf(ctx, p.Project, p.Patent)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, store.ErrNotFound
	}
	return proto.IDSEntryResult{Entry: entry}, nil
}

func (s *Server) idsEntrySave(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.IDSEntrySaveParams](raw)
	if err != nil {
		return nil, err
	}
	entry, err := s.engine.SaveIDSEntry(ctx, p.Entry)
	if err != nil {
		return nil, err
	}
	return proto.IDSEntryResult{Entry: entry}, nil
}

func (s *Server) idsEntryDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.IDSEntryParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeleteIDSEntry(ctx, p.Project, p.Patent); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) patentNoteGet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentNoteParams](raw)
	if err != nil {
		return nil, err
	}
	note, ok, err := s.engine.PatentNoteOf(ctx, p.Project, p.Patent)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, store.ErrNotFound
	}
	return proto.PatentNoteResult{Note: note}, nil
}

func (s *Server) patentNoteSave(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentNoteSaveParams](raw)
	if err != nil {
		return nil, err
	}
	note, err := s.engine.SavePatentNote(ctx, p.Note)
	if err != nil {
		return nil, err
	}
	return proto.PatentNoteResult{Note: note}, nil
}

func (s *Server) patentNoteDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentNoteParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeletePatentNote(ctx, p.Project, p.Patent); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) patentNoteList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentNoteListParams](raw)
	if err != nil {
		return nil, err
	}
	sortByDate := p.SortBy != proto.NoteSortByPatent
	notes, err := s.engine.ListPatentNotes(ctx, p.Project, sortByDate)
	if err != nil {
		return nil, err
	}
	return proto.PatentNoteListResult{Notes: notes}, nil
}

func (s *Server) patentNoteExport(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentNoteExportParams](raw)
	if err != nil {
		return nil, err
	}
	t0 := time.Now()
	sortByDate := p.SortBy != proto.NoteSortByPatent
	notes, err := s.engine.ListPatentNotes(ctx, p.Project, sortByDate)
	if err != nil {
		return nil, fmt.Errorf("list notes: %w", err)
	}

	md := buildNotesMarkdown(notes, string(p.Project), sortByDate)
	bytes := len(md)

	if p.OutputPath != "" {
		if err := os.WriteFile(p.OutputPath, []byte(md), 0o644); err != nil {
			return nil, fmt.Errorf("write export file: %w", err)
		}
		s.engineLogger().Info("notes exported",
			slog.Int("count", len(notes)),
			slog.Int("bytes", bytes),
			slog.String("path", p.OutputPath),
			slog.Int64("duration_ms", time.Since(t0).Milliseconds()))
		return proto.PatentNoteExportResult{Path: p.OutputPath, Count: len(notes), Bytes: bytes}, nil
	}
	s.engineLogger().Info("notes export rendered",
		slog.Int("count", len(notes)),
		slog.Int("bytes", bytes),
		slog.Int64("duration_ms", time.Since(t0).Milliseconds()))
	return proto.PatentNoteExportResult{Count: len(notes), Bytes: bytes, Content: md}, nil
}

// buildNotesMarkdown generates the markdown document for a set of patent notes.
func buildNotesMarkdown(notes []domain.PatentNote, projectName string, sortByDate bool) string {
	var b strings.Builder
	b.WriteString("# PatentMine Notes")
	if projectName != "" {
		b.WriteString(" — " + projectName)
	}
	b.WriteString("\n\n")
	sortLabel := "date (most recent first)"
	if !sortByDate {
		sortLabel = "patent number"
	}
	fmt.Fprintf(&b, "Exported: %s  ·  Sorted by: %s\n\n",
		time.Now().Format("2006-01-02 15:04:05"), sortLabel)
	b.WriteString(strings.Repeat("─", 72) + "\n\n")
	for _, note := range notes {
		fmt.Fprintf(&b, "## %s\n\n_Updated: %s_\n\n",
			note.Patent.String(), note.UpdatedAt.Format("2006-01-02 15:04:05"))
		b.WriteString(note.Markdown)
		b.WriteString("\n\n" + strings.Repeat("─", 72) + "\n\n")
	}
	return b.String()
}

// engineLogger returns the engine's logger if available, or the default.
func (s *Server) engineLogger() *slog.Logger {
	if l := s.engine.Logger(); l != nil {
		return l
	}
	return slog.Default()
}

func (s *Server) metricsGet(context.Context, json.RawMessage) (any, error) {
	merged := s.engine.MetricsSnapshot()
	s.clientMetrics.Range(func(_, v any) bool {
		snap, ok := v.(proto.MetricsSnapshot)
		if !ok {
			return true
		}
		for k, tm := range snap.Timings {
			if existing, found := merged.Timings[k]; found {
				merged.Timings[k] = mergeTimingMetric(existing, tm)
			} else {
				merged.Timings[k] = tm
			}
		}
		for k, v := range snap.Counters {
			merged.Counters[k] += v
		}
		for k, v := range snap.Gauges {
			merged.Gauges[k] += v
		}
		return true
	})
	return proto.MetricsResult{Metrics: merged}, nil
}

func (s *Server) metricsPush(_ context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MetricsPushParams](raw)
	if err != nil {
		return nil, err
	}
	key := p.Component
	if key == "" {
		key = "client"
	}
	s.clientMetrics.Store(key, p.Snapshot)
	return proto.Empty{}, nil
}

func (s *Server) activityRaw(_ context.Context, raw json.RawMessage) (any, error) {
	if s.activityLogsDir == "" {
		return nil, errors.New("rpc: activity logging is not configured")
	}
	p, err := decodeParams[proto.ActivityRawParams](raw)
	if err != nil {
		return nil, err
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 100
	}
	records, err := observability.ReadActivityRecords(s.activityLogsDir, observability.ActivityQuery{
		Limit:     limit,
		Component: p.Component,
		Action:    p.Action,
		Entity:    p.Entity,
		Since:     p.Since,
	})
	if err != nil {
		return nil, err
	}
	return proto.ActivityRawResult{Records: records, Limit: limit, Returned: len(records)}, nil
}

func (s *Server) historyFeed(_ context.Context, raw json.RawMessage) (any, error) {
	if s.activityLogsDir == "" {
		return nil, errors.New("rpc: activity logging is not configured")
	}
	p, err := decodeParams[proto.HistoryFeedParams](raw)
	if err != nil {
		return nil, err
	}
	return observability.ReadHistoryFeed(s.activityLogsDir, observability.HistoryQuery{
		RawLimit:  p.RawLimit,
		Component: p.Component,
		Since:     p.Since,
	})
}

func (s *Server) tableViewList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TableViewListParams](raw)
	if err != nil {
		return nil, err
	}
	views, err := s.engine.ListTableViews(ctx, p.Owner, p.TableType)
	if err != nil {
		return nil, err
	}
	return proto.TableViewListResult{Views: views}, nil
}

func (s *Server) tableViewGet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TableViewGetParams](raw)
	if err != nil {
		return nil, err
	}
	view, err := s.engine.TableView(ctx, p.Owner, p.ID)
	if err != nil {
		return nil, err
	}
	return proto.TableViewResult{View: view}, nil
}

func (s *Server) tableViewSave(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TableViewSaveParams](raw)
	if err != nil {
		return nil, err
	}
	view, err := s.engine.SaveTableView(ctx, p.View)
	if err != nil {
		return nil, err
	}
	return proto.TableViewResult{View: view}, nil
}

func (s *Server) tableViewDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.TableViewGetParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeleteTableView(ctx, p.Owner, p.ID); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

// mergeTimingMetric combines two timing summaries for the same key.
func mergeTimingMetric(a, b proto.TimingMetric) proto.TimingMetric {
	count := a.Count + b.Count
	totalNanos := a.TotalNanos + b.TotalNanos
	errors := a.Errors + b.Errors
	minNanos := a.MinNanos
	if b.MinNanos < minNanos {
		minNanos = b.MinNanos
	}
	maxNanos := a.MaxNanos
	if b.MaxNanos > maxNanos {
		maxNanos = b.MaxNanos
	}
	lastNanos := b.LastNanos
	var avgNanos int64
	if count > 0 {
		avgNanos = totalNanos / count
	}
	return proto.TimingMetric{
		Count:      count,
		Errors:     errors,
		TotalNanos: totalNanos,
		AvgNanos:   avgNanos,
		AvgMillis:  avgNanos / int64(time.Millisecond),
		MinNanos:   minNanos,
		MinMillis:  minNanos / int64(time.Millisecond),
		MaxNanos:   maxNanos,
		MaxMillis:  maxNanos / int64(time.Millisecond),
		LastNanos:  lastNanos,
		LastMillis: lastNanos / int64(time.Millisecond),
	}
}

func (s *Server) classificationGet(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ClassificationGetParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.ClassificationDefinition(ctx, p.System, p.Code)
}

func (s *Server) classificationList(ctx context.Context, _ json.RawMessage) (any, error) {
	classifications, err := s.engine.ListClassificationDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	return proto.ClassificationListResult{Classifications: classifications}, nil
}

func (s *Server) classificationSave(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ClassificationParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.SaveClassificationDefinition(ctx, p.Classification); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) classificationDelete(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ClassificationDeleteParams](raw)
	if err != nil {
		return nil, err
	}
	if err := s.engine.DeleteClassificationDefinition(ctx, p.System, p.Code); err != nil {
		return nil, err
	}
	return proto.Empty{}, nil
}

func (s *Server) classificationLookup(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ClassificationLookupParams](raw)
	if err != nil {
		return nil, err
	}
	return s.engine.LookupClassification(ctx, p.Code)
}

func (s *Server) classificationListByCodes(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ClassificationListByCodesParams](raw)
	if err != nil {
		return nil, err
	}
	classifications, err := s.engine.ListClassificationDefinitionsByCodes(ctx, p.Codes)
	if err != nil {
		return nil, err
	}
	return proto.ClassificationListResult{Classifications: classifications}, nil
}

func (s *Server) patentClassificationList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.PatentClassificationListParams](raw)
	if err != nil {
		return nil, err
	}
	classifications, err := s.engine.ListPatentClassifications(ctx, p.Project, p.Patent)
	if err != nil {
		return nil, err
	}
	return proto.ClassificationListResult{Classifications: classifications}, nil
}
