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
	"sync"
	"time"

	"patentmine/internal/engine"
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
	engine           *engine.Engine
	handlers         map[proto.Method]handlerFunc
	usptoConfigured  bool
	backupConfigured bool
	activityLogsDir  string
	clientMetrics    sync.Map // component string → proto.MetricsSnapshot
}

// Option customizes server behavior outside the engine dispatch table.
type Option func(*Server)

// WithActivityLogsDir lets RPC clients read the daemon's raw and grouped
// activity feeds from the same log directory where daemon mutations are written.
func WithActivityLogsDir(dir string) Option {
	return func(s *Server) { s.activityLogsDir = dir }
}

// WithBackupConfigured reports backup configuration to thin clients and API
// health checks. It intentionally means configured, not live-verified.
func WithBackupConfigured(configured bool) Option {
	return func(s *Server) { s.backupConfigured = configured }
}

// NewServer wires the dispatch table for an engine.
func NewServer(eng *engine.Engine, usptoConfigured bool, opts ...Option) *Server {
	s := &Server{engine: eng, usptoConfigured: usptoConfigured}
	for _, opt := range opts {
		opt(s)
	}
	s.handlers = map[proto.Method]handlerFunc{
		proto.MethodPing:                         s.ping,
		proto.MethodPatentGet:                    s.patentGet,
		proto.MethodAllAssigneesHistory:          s.allAssigneesHistory,
		proto.MethodPatentClassificationStats:    s.patentClassificationStats,
		proto.MethodPatentInventorStats:          s.patentInventorStats,
		proto.MethodPatentList:                   s.patentList,
		proto.MethodPatentTableColumns:           s.patentTableColumns,
		proto.MethodPatentDelete:                 s.patentDelete,
		proto.MethodPatentDeleteBulk:             s.patentDeleteBulk,
		proto.MethodPatentClearCache:             s.patentClearCache,
		proto.MethodProjectList:                  s.projectList,
		proto.MethodProjectCreate:                s.projectCreate,
		proto.MethodMembershipAdd:                s.membershipAdd,
		proto.MethodAddRelated:                   s.addRelated,
		proto.MethodOrphanList:                   s.orphanList,
		proto.MethodReviewState:                  s.reviewState,
		proto.MethodTagPatent:                    s.tagPatent,
		proto.MethodUntagPatent:                  s.untagPatent,
		proto.MethodCrawlFamily:                  s.crawlFamily,
		proto.MethodCrawlConfig:                  s.crawlConfig,
		proto.MethodSourceModeGet:                s.sourceModeGet,
		proto.MethodSourceModeSet:                s.sourceModeSet,
		proto.MethodCrawlCancel:                  s.crawlCancel,
		proto.MethodImportFile:                   s.importFile,
		proto.MethodRelations:                    s.relations,
		proto.MethodFamilyGraph:                  s.familyGraph,
		proto.MethodFamilyGraphExport:            s.familyGraphExport,
		proto.MethodIDSExport:                    s.idsExport,
		proto.MethodIDSPDFExport:                 s.idsPDFExport,
		proto.MethodIDSPDFPreview:                s.idsPDFPreview,
		proto.MethodProjectUpdate:                s.projectUpdate,
		proto.MethodIDSEntryGet:                  s.idsEntryGet,
		proto.MethodIDSEntrySave:                 s.idsEntrySave,
		proto.MethodIDSEntryDelete:               s.idsEntryDelete,
		proto.MethodIDSEntryBulkSetStatus:        s.idsEntryBulkSetStatus,
		proto.MethodPatentNoteGet:                s.patentNoteGet,
		proto.MethodPatentNoteSave:               s.patentNoteSave,
		proto.MethodPatentNoteDelete:             s.patentNoteDelete,
		proto.MethodPatentNoteList:               s.patentNoteList,
		proto.MethodPatentNoteExport:             s.patentNoteExport,
		proto.MethodAddedExport:                  s.addedExport,
		proto.MethodAddedImport:                  s.addedImport,
		proto.MethodMetricsGet:                   s.metricsGet,
		proto.MethodMetricsPush:                  s.metricsPush,
		proto.MethodActivityRaw:                  s.activityRaw,
		proto.MethodHistoryFeed:                  s.historyFeed,
		proto.MethodTagCreate:                    s.tagCreate,
		proto.MethodTagList:                      s.tagList,
		proto.MethodTagDelete:                    s.tagDelete,
		proto.MethodTagPatentStrict:              s.tagPatentStrict,
		proto.MethodUntagPatentStrict:            s.untagPatentStrict,
		proto.MethodPatentTagList:                s.patentTagList,
		proto.MethodClassificationGet:            s.classificationGet,
		proto.MethodClassificationList:           s.classificationList,
		proto.MethodClassificationSave:           s.classificationSave,
		proto.MethodClassificationDelete:         s.classificationDelete,
		proto.MethodClassificationLookup:         s.classificationLookup,
		proto.MethodClassificationListByCodes:    s.classificationListByCodes,
		proto.MethodPatentClassificationList:     s.patentClassificationList,
		proto.MethodTableViewList:                s.tableViewList,
		proto.MethodTableViewGet:                 s.tableViewGet,
		proto.MethodTableViewSave:                s.tableViewSave,
		proto.MethodTableViewDelete:              s.tableViewDelete,
		proto.MethodUSPTOFetchXML:                s.usptoFetchXML,
		proto.MethodUSPTOGrantBody:               s.usptoGrantBody,
		proto.MethodFullTextSearch:               s.fullTextSearch,
		proto.MethodUSPTOLookup:                  s.usptoLookup,
		proto.MethodUSPTOFetchAssignments:        s.usptoFetchAssignments,
		proto.MethodUSPTOFetchAssignmentsBatch:   s.usptoFetchAssignmentsBatch,
		proto.MethodUSPTOAssignmentList:          s.usptoAssignmentList,
		proto.MethodAssigneeHistory:              s.assigneeHistory,
		proto.MethodProjectAssignees:             s.projectAssignees,
		proto.MethodUSPTOExpirationCalculate:     s.usptoExpirationCalculate,
		proto.MethodSourceResolveDiffs:           s.sourceResolveDiffs,
		proto.MethodSourceDiffsList:              s.sourceDiffsList,
		proto.MethodSourceBibsList:               s.sourceBibsList,
		proto.MethodDraftCreate:                  s.draftCreate,
		proto.MethodDraftGet:                     s.draftGet,
		proto.MethodDraftList:                    s.draftList,
		proto.MethodDraftSave:                    s.draftSave,
		proto.MethodDraftDelete:                  s.draftDelete,
		proto.MethodDraftExportDocx:              s.draftExportDocx,
		proto.MethodDraftSectionAI:               s.draftSectionAI,
		proto.MethodDraftSnapshot:                s.draftSnapshot,
		proto.MethodDraftRevisionList:            s.draftRevisionList,
		proto.MethodDraftRevisionGet:             s.draftRevisionGet,
		proto.MethodDraftRestore:                 s.draftRestore,
		proto.MethodProvisionalCoverSheetGet:     s.provisionalCoverSheetGet,
		proto.MethodProvisionalCoverSheetSave:    s.provisionalCoverSheetSave,
		proto.MethodProvisionalCoverSheetPreview: s.provisionalCoverSheetPreview,
		proto.MethodProvisionalCoverSheetApprove: s.provisionalCoverSheetApprove,
		proto.MethodOfficeActionImport:           s.officeActionImport,
		proto.MethodOfficeActionList:             s.officeActionList,
		proto.MethodOfficeActionGet:              s.officeActionGet,
		proto.MethodOfficeActionSaveNotes:        s.officeActionSaveNotes,
		proto.MethodOfficeActionUpdate:           s.officeActionUpdate,
		proto.MethodOfficeActionDelete:           s.officeActionDelete,
		proto.MethodOfficeActionOpen:             s.officeActionOpen,
		proto.MethodOfficeActionTag:              s.officeActionTag,
		proto.MethodOfficeActionUntag:            s.officeActionUntag,
		proto.MethodOfficeActionAssignPatents:    s.officeActionAssignPatents,
		proto.MethodOfficeActionReleasePatents:   s.officeActionReleasePatents,
		proto.MethodOfficeActionReviewStatus:     s.officeActionReviewStatus,
		proto.MethodOfficeActionPatents:          s.officeActionPatents,
		proto.MethodOfficeActionCopyPatents:      s.officeActionCopyPatents,
		proto.MethodPatentOfficeActions:          s.patentOfficeActions,
		proto.MethodMatterDocumentImport:         s.matterDocumentImport,
		proto.MethodMatterDocumentList:           s.matterDocumentList,
		proto.MethodMatterDocumentRename:         s.matterDocumentRename,
		proto.MethodMatterDocumentSetMeta:        s.matterDocumentSetMeta,
		proto.MethodMatterDocumentDelete:         s.matterDocumentDelete,
		proto.MethodMatterDocumentExtract:        s.matterDocumentExtract,
		proto.MethodMatterDocumentTag:            s.matterDocumentTag,
		proto.MethodMatterDocumentUntag:          s.matterDocumentUntag,
		proto.MethodMatterDocumentAssign:         s.matterDocumentAssign,
		proto.MethodMatterDocumentUnassign:       s.matterDocumentUnassign,
		proto.MethodMatterDocumentOpen:           s.matterDocumentOpen,
		proto.MethodMatterEventAdd:               s.matterEventAdd,
		proto.MethodMatterEventList:              s.matterEventList,
		proto.MethodMatterEventDelete:            s.matterEventDelete,
		proto.MethodProjectSetMatterType:         s.projectSetMatterType,
		proto.MethodConflictFlag:                 s.conflictFlag,
		proto.MethodConflictResolve:              s.conflictResolve,
		proto.MethodConflictList:                 s.conflictList,
		proto.MethodConflictDelete:               s.conflictDelete,
		proto.MethodTimeLog:                      s.timeLog,
		proto.MethodTimeList:                     s.timeList,
		proto.MethodTimeUnvalidated:              s.timeUnvalidated,
		proto.MethodTimeUpdate:                   s.timeUpdate,
		proto.MethodTimeDelete:                   s.timeDelete,
		proto.MethodTimeSummary:                  s.timeSummary,
		proto.MethodDeadlineList:                 s.deadlineList,
		proto.MethodTrackRenewals:                s.trackRenewals,
		proto.MethodUntrackRenewals:              s.untrackRenewals,
		proto.MethodDeadlineSetStatus:            s.deadlineSetStatus,
		proto.MethodDeadlineRemind:               s.deadlineRemind,
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
	return proto.PingResult{
		Pong:             true,
		Version:          appversion.String(),
		USPTOConfigured:  s.usptoConfigured,
		BackupConfigured: s.backupConfigured,
	}, nil
}
