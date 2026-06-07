// Package proto defines the client<->daemon wire contract: a JSON-RPC 2.0
// dialect carried as line-delimited JSON. Method and event names are typed
// constants so no caller spells them as a bare string.
package proto

import (
	"encoding/json"

	"patentmine/internal/domain"
)

// Version is the JSON-RPC protocol version string.
const Version = "2.0"

// Method names a request a client may send to the daemon.
type Method string

const (
	MethodPing                       Method = "ping"
	MethodPatentGet                  Method = "patent.get"
	MethodAllAssigneesHistory        Method = "patent.all_assignees_history"
	MethodPatentClassificationStats  Method = "patent.classification_stats"
	MethodPatentInventorStats        Method = "patent.inventor_stats"
	MethodPatentList                 Method = "patent.list"
	MethodPatentTableColumns         Method = "patent.table_columns"
	MethodPatentDelete               Method = "patent.delete"
	MethodPatentDeleteBulk           Method = "patent.delete_bulk"
	MethodPatentClearCache           Method = "patent.clear_cache"
	MethodProjectList                Method = "project.list"
	MethodProjectCreate              Method = "project.create"
	MethodMembershipAdd              Method = "membership.add"
	MethodAddRelated                 Method = "membership.add_related"
	MethodOrphanList                 Method = "patent.orphan_list"
	MethodReviewState                Method = "review_state.set"
	MethodTagPatent                  Method = "tag.assign"
	MethodUntagPatent                Method = "tag.remove"
	MethodCrawlFamily                Method = "crawl.family"
	MethodCrawlConfig                Method = "crawl.config"
	MethodSourceModeGet              Method = "source_mode.get"
	MethodSourceModeSet              Method = "source_mode.set"
	MethodCrawlCancel                Method = "crawl.cancel"
	MethodImportFile                 Method = "import.file"
	MethodRelations                  Method = "patent.relations"
	MethodFamilyGraph                Method = "patent.family_graph"
	MethodFamilyGraphExport          Method = "patent.family_graph.export"
	MethodIDSExport                  Method = "ids.export"
	MethodIDSPDFExport               Method = "ids.export.pdf"
	MethodIDSPDFPreview              Method = "ids.export.pdf.preview"
	MethodProjectUpdate              Method = "project.update"
	MethodIDSEntryGet                Method = "ids.entry.get"
	MethodIDSEntrySave               Method = "ids.entry.save"
	MethodIDSEntryDelete             Method = "ids.entry.delete"
	MethodIDSEntryBulkSetStatus      Method = "ids.entry.bulk_set_status"
	MethodPatentNoteGet              Method = "patent.note.get"
	MethodPatentNoteSave             Method = "patent.note.save"
	MethodPatentNoteDelete           Method = "patent.note.delete"
	MethodPatentNoteList             Method = "patent.note.list"
	MethodPatentNoteExport           Method = "patent.note.export"
	MethodAddedExport                Method = "added.export"
	MethodAddedImport                Method = "added.import"
	MethodMetricsGet                 Method = "metrics.get"
	MethodMetricsPush                Method = "metrics.push"
	MethodActivityRaw                Method = "activity.raw"
	MethodHistoryFeed                Method = "history.feed"
	MethodTagCreate                  Method = "tag.create"
	MethodTagList                    Method = "tag.list"
	MethodTagDelete                  Method = "tag.delete"
	MethodTagPatentStrict            Method = "patent.tag.add"
	MethodUntagPatentStrict          Method = "patent.tag.delete"
	MethodPatentTagList              Method = "patent.tag.list"
	MethodClassificationGet          Method = "classification.get"
	MethodClassificationList         Method = "classification.list"
	MethodClassificationSave         Method = "classification.save"
	MethodClassificationDelete       Method = "classification.delete"
	MethodClassificationLookup       Method = "classification.lookup"
	MethodClassificationListByCodes  Method = "classification.list_by_codes"
	MethodPatentClassificationList   Method = "patent.classification.list"
	MethodTableViewList              Method = "table_view.list"
	MethodTableViewGet               Method = "table_view.get"
	MethodTableViewSave              Method = "table_view.save"
	MethodTableViewDelete            Method = "table_view.delete"
	MethodUSPTOFetchXML              Method = "uspto.fetch_xml"
	MethodUSPTOGrantBody             Method = "uspto.grant_body"
	MethodFullTextSearch             Method = "fulltext.search"
	MethodFullTextCoverage           Method = "fulltext.coverage"
	MethodUSPTOLookup                Method = "uspto.lookup"
	MethodUSPTOFetchAssignments      Method = "uspto.fetch_assignments"
	MethodUSPTOFetchAssignmentsBatch Method = "uspto.fetch_assignments.batch"
	MethodUSPTOAssignmentList        Method = "uspto.assignment.list"
	MethodAssigneeHistory            Method = "assignee.history"
	MethodProjectAssignees           Method = "project.assignees"
	MethodUSPTOExpirationCalculate   Method = "uspto.expiration.calculate"

	// Drafting subsystem: provisional / non-provisional applications and
	// office-action responses, rendered to .docx.
	MethodDraftCreate                  Method = "draft.create"
	MethodDraftGet                     Method = "draft.get"
	MethodDraftList                    Method = "draft.list"
	MethodDraftSave                    Method = "draft.save"
	MethodDraftDelete                  Method = "draft.delete"
	MethodDraftExportDocx              Method = "draft.export.docx"
	MethodDraftSectionAI               Method = "draft.section.ai"
	MethodDraftSnapshot                Method = "draft.snapshot"
	MethodDraftRevisionList            Method = "draft.revision.list"
	MethodDraftRevisionGet             Method = "draft.revision.get"
	MethodDraftRestore                 Method = "draft.restore"
	MethodProvisionalCoverSheetGet     Method = "coversheet.provisional.get"
	MethodProvisionalCoverSheetSave    Method = "coversheet.provisional.save"
	MethodProvisionalCoverSheetPreview Method = "coversheet.provisional.preview"
	MethodProvisionalCoverSheetApprove Method = "coversheet.provisional.approve"
	MethodOfficeActionImport           Method = "office_action.import"
	MethodOfficeActionList             Method = "office_action.list"
	MethodOfficeActionGet              Method = "office_action.get"
	MethodOfficeActionSaveNotes        Method = "office_action.save_notes"
	MethodOfficeActionUpdate           Method = "office_action.update"
	MethodOfficeActionDelete           Method = "office_action.delete"
	MethodOfficeActionOpen             Method = "office_action.open"
	MethodOfficeActionTag              Method = "office_action.tag"
	MethodOfficeActionUntag            Method = "office_action.untag"

	// Prosecution-matter workspace: matter-scoped documents and project stage.
	MethodMatterDocumentImport   Method = "matter.document.import"
	MethodMatterDocumentList     Method = "matter.document.list"
	MethodMatterDocumentRename   Method = "matter.document.rename"
	MethodMatterDocumentSetMeta  Method = "matter.document.set_meta"
	MethodMatterDocumentDelete   Method = "matter.document.delete"
	MethodMatterDocumentExtract  Method = "matter.document.extract"
	MethodMatterDocumentTag      Method = "matter.document.tag"
	MethodMatterDocumentUntag    Method = "matter.document.untag"
	MethodMatterDocumentAssign   Method = "matter.document.assign_oa"
	MethodMatterDocumentUnassign Method = "matter.document.unassign_oa"
	MethodMatterDocumentOpen     Method = "matter.document.open"
	MethodMatterEventAdd         Method = "matter.event.add"
	MethodMatterEventList        Method = "matter.event.list"
	MethodMatterEventDelete      Method = "matter.event.delete"
	MethodProjectSetMatterType   Method = "project.set_matter_type"

	// Conflict edges: flag a record as conflicting within a matter and manage them.
	// The patent-list ⚠ badge is computed in the daemon (ListPatents sets
	// PatentRow.Conflicting), so no separate "records" RPC is needed.
	MethodConflictFlag    Method = "conflict.flag"
	MethodConflictResolve Method = "conflict.resolve"
	MethodConflictList    Method = "conflict.list"
	MethodConflictDelete  Method = "conflict.delete"

	// Time tracking + AI usage (billing).
	MethodTimeLog         Method = "time_entry.log"
	MethodTimeList        Method = "time_entry.list"
	MethodTimeUnvalidated Method = "time_entry.unvalidated"
	MethodTimeUpdate      Method = "time_entry.update"
	MethodTimeDelete      Method = "time_entry.delete"
	MethodTimeSummary     Method = "time_entry.summary"

	// Deadlines + reminders (OA responses, patent maintenance fees).
	MethodDeadlineList      Method = "deadline.list"
	MethodTrackRenewals     Method = "deadline.track_renewals"
	MethodUntrackRenewals   Method = "deadline.untrack_renewals"
	MethodDeadlineSetStatus Method = "deadline.set_status"
	MethodDeadlineRemind    Method = "deadline.remind"

	// Source comparison reconciliation (Option A): persist overlay choices.
	MethodSourceResolveDiffs Method = "source.resolve_diffs"
	MethodSourceDiffsList    Method = "source.diffs.list"
	// MethodSourceBibsList loads each source's full bibliographic snapshot for
	// the all-fields side-by-side comparison view.
	MethodSourceBibsList Method = "source.bibs.list"
)

// EventKind names a server->client push (a JSON-RPC notification).
type EventKind string

const (
	EventCrawlProgress        EventKind = "crawl.progress"
	EventCrawlDone            EventKind = "crawl.done"
	EventDBChanged            EventKind = "db.changed"
	EventMembershipAutoAssign EventKind = "membership.auto_assign"
)

// JSON-RPC error codes. The negative range follows the spec; -32000 down is
// reserved for application errors.
const (
	CodeParse      = -32700
	CodeInvalidReq = -32600
	CodeNoMethod   = -32601
	CodeBadParams  = -32602
	CodeInternal   = -32603
	CodeNotFound   = -32000
	CodeBusy       = -32001
)

// Request is one JSON-RPC call from a client to the daemon.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Method  Method          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Reply is the daemon's response to a Request, matched back by ID.
type Reply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      uint64          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error is a JSON-RPC error object.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

// Event is a daemon->client notification. It has no ID: that absence is how a
// client tells an Event apart from a Reply on the shared connection.
type Event struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  EventKind       `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// --- Method parameter and result payloads ---

// Empty is the result of operations with nothing to return.
type Empty struct{}

// --- Event payloads ---

// CrawlProgress reports incremental progress of a crawl job.
type CrawlProgress struct {
	JobID           string   `json:"job_id"`
	CrawledCount    int      `json:"crawled_count"`
	DiscoveredCount int      `json:"discovered_count"`
	PendingCount    int      `json:"pending_count"`
	Depth           int      `json:"depth"`
	MaxDepth        int      `json:"max_depth"`
	CitationsCount  int      `json:"citations_count,omitempty"`
	CitedByCount    int      `json:"cited_by_count,omitempty"`
	ParentsCount    int      `json:"parents_count,omitempty"`
	ChildrenCount   int      `json:"children_count,omitempty"`
	Message         string   `json:"message"`
	RecordNumber    string   `json:"record_number,omitempty"` // Record just saved in this event, if any.
	Stubs           []string `json:"stubs,omitempty"`         // Numbers freshly stubbed in this event, if any.
	Sources         []string `json:"sources,omitempty"`       // Providers (uspto, google, ...) that contributed snapshots for RecordNumber.
	Mode            string   `json:"mode,omitempty"`          // Source-mode policy in effect (compare, uspto-first, uspto-only, google-only).
	Warnings        []string `json:"warnings,omitempty"`      // Non-fatal warnings (e.g. a source was anti-bot blocked) raised this event.
}

// MembershipAutoAssign reports the result of the engine's depth-1 auto-assign
// after :add: how many neighbor stubs received project membership.
type MembershipAutoAssign struct {
	JobID   string `json:"job_id"`
	Project string `json:"project"`
	Root    string `json:"root"`
	Added   int    `json:"added"`
}

// CrawlDone reports that a crawl job finished, with an error if it failed.
type CrawlDone struct {
	JobID string `json:"job_id"`
	Error string `json:"error,omitempty"`
}

// NewEvent builds an Event, marshaling params. A marshal failure yields an
// event with empty params rather than a hard error on a best-effort channel.
func NewEvent(kind EventKind, params any) Event {
	raw, err := json.Marshal(params)
	if err != nil {
		raw = nil
	}
	return Event{JSONRPC: Version, Method: kind, Params: raw}
}

type PatentClearCacheParams struct {
	Patents []domain.PatentNumber `json:"patents"`
}

type PatentClearCacheResult struct {
	ClearedCount int64 `json:"cleared_count"`
	BytesSaved   int64 `json:"bytes_saved"`
}
