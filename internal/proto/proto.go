// Package proto defines the client<->daemon wire contract: a JSON-RPC 2.0
// dialect carried as line-delimited JSON. Method and event names are typed
// constants so no caller spells them as a bare string.
package proto

import (
	"encoding/json"
	"time"

	"patentmine/internal/domain"
)

// Version is the JSON-RPC protocol version string.
const Version = "2.0"

// Method names a request a client may send to the daemon.
type Method string

const (
	MethodPing            Method = "ping"
	MethodPatentGet       Method = "patent.get"
	MethodPatentList      Method = "patent.list"
	MethodPatentDelete    Method = "patent.delete"
	MethodProjectList     Method = "project.list"
	MethodProjectCreate   Method = "project.create"
	MethodMembershipAdd   Method = "membership.add"
	MethodReviewState Method = "review_state.set"
	MethodTagAssign       Method = "tag.assign"
	MethodTagRemove       Method = "tag.remove"
	MethodIngestFamily    Method = "ingest.family"
	MethodIngestCancel    Method = "ingest.cancel"
	MethodImportFile      Method = "import.file"
	MethodRelations       Method = "patent.relations"
	MethodIDSExport       Method = "ids.export"
	MethodMetricsGet      Method = "metrics.get"
)

// EventKind names a server->client push (a JSON-RPC notification).
type EventKind string

const (
	EventIngestProgress EventKind = "ingest.progress"
	EventIngestDone     EventKind = "ingest.done"
	EventDBChanged      EventKind = "db.changed"
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

// PingResult confirms the daemon is alive.
type PingResult struct {
	Pong    bool   `json:"pong"`
	Version string `json:"version"`
}

// PatentDeleteParams identifies the patent to permanently remove.
type PatentDeleteParams struct {
	Number domain.PatentNumber `json:"number"`
}

// PatentGetParams selects a single patent. Project, when set, scopes the
// project-relative fields of the result — review state and tags — to that
// project; the patent record itself is project-independent.
type PatentGetParams struct {
	Number  domain.PatentNumber `json:"number"`
	Project domain.ProjectID    `json:"project,omitempty"`
}

// PatentResult carries one patent. State and Tags are populated only when the
// request named a project the patent is a member of, and are empty otherwise:
// they describe the (patent, project) pair, not the patent.
type PatentResult struct {
	Patent      domain.Patent      `json:"patent"`
	ReviewState domain.ReviewState `json:"review_state,omitempty"`
	Tags        []domain.Tag       `json:"tags,omitempty"`
}

// PatentListParams selects and paginates a patent listing.
type PatentListParams struct {
	Project       domain.ProjectID   `json:"project,omitempty"`
	ReviewState   domain.ReviewState `json:"review_state,omitempty"`
	Search        string             `json:"search,omitempty"`
	Limit         int                `json:"limit,omitempty"`
	Offset        int                `json:"offset,omitempty"`
	SortColumn    domain.SortColumn  `json:"sort_column,omitempty"`
	SortAscending bool               `json:"sort_ascending,omitempty"`
}

// PatentListResult carries one page of patents plus the unpaged total.
type PatentListResult struct {
	Patents []domain.PatentRow `json:"patents"`
	Total   int                `json:"total"`
}

// ProjectListResult carries every project.
type ProjectListResult struct {
	Projects []domain.Project `json:"projects"`
}

// ProjectCreateParams names a new project.
type ProjectCreateParams struct {
	Name string `json:"name"`
}

// ProjectResult carries one project.
type ProjectResult struct {
	Project domain.Project `json:"project"`
}

// MembershipParams identifies a (project, patent) pair.
type MembershipParams struct {
	Project domain.ProjectID    `json:"project"`
	Patent  domain.PatentNumber `json:"patent"`
}

// ReviewStateParams sets a membership's state.
type ReviewStateParams struct {
	Project domain.ProjectID    `json:"project"`
	Patent  domain.PatentNumber `json:"patent"`
	State   string              `json:"state"`
}

// TagParams names a tag to assign to, or remove from, a patent within a
// project. On assign an unknown name creates the tag.
type TagParams struct {
	Project domain.ProjectID    `json:"project"`
	Patent  domain.PatentNumber `json:"patent"`
	Name    string              `json:"name"`
}

// IngestFamilyParams starts a family-graph crawl rooted at one patent. Depth 0
// fetches only the root; a negative depth uses the configured family depth.
// Force bypasses the local file cache and re-fetches from the web.
type IngestFamilyParams struct {
	Root    domain.PatentNumber `json:"root"`
	Depth   int                 `json:"depth"`
	Profile domain.CrawlProfile `json:"profile,omitempty"`
	Force   bool                `json:"force,omitempty"`
}

// IngestStartResult returns the id of an enqueued job.
type IngestStartResult struct {
	JobID string `json:"job_id"`
}

// ImportFileParams loads a patent record from a local fixture file.
type ImportFileParams struct {
	Path string `json:"path"`
}

// IngestCancelParams cancels a running job.
type IngestCancelParams struct {
	JobID string `json:"job_id"`
}

// RelationsParams selects family-graph edges of one kind from one patent.
type RelationsParams struct {
	Number        domain.PatentNumber `json:"number"`
	Kind          domain.RelationKind `json:"kind"`
	Project       domain.ProjectID    `json:"project,omitempty"`
	ReviewState   domain.ReviewState  `json:"review_state,omitempty"`
	Search        string              `json:"search,omitempty"`
	Limit         int                 `json:"limit,omitempty"`
	Offset        int                 `json:"offset,omitempty"`
	SortColumn    domain.SortColumn   `json:"sort_column,omitempty"`
	SortAscending bool                `json:"sort_ascending,omitempty"`
}

// RelationsResult carries the requested family-graph edges.
type RelationsResult struct {
	Patents []domain.PatentRow `json:"patents"`
	Total   int                `json:"total"`
}

// IDSExportParams selects the project to build an Information Disclosure
// Statement for.
type IDSExportParams struct {
	Project domain.ProjectID `json:"project"`
}

// IDSResult carries a generated Information Disclosure Statement.
type IDSResult struct {
	IDS domain.IDS `json:"ids"`
}

// MetricsResult carries the daemon's current in-memory timing/counter snapshot.
type MetricsResult struct {
	Metrics MetricsSnapshot `json:"metrics"`
}

// MetricsSnapshot is a transport-safe view of the daemon's metrics.
type MetricsSnapshot struct {
	Timestamp time.Time                 `json:"timestamp"`
	Timings   map[string]TimingMetric   `json:"timings"`
	Counters  map[string]int64          `json:"counters"`
	Gauges    map[string]int64          `json:"gauges"`
}

// TimingMetric aggregates one named operation's observed durations.
type TimingMetric struct {
	Count      int64 `json:"count"`
	Errors     int64 `json:"errors"`
	TotalNanos int64 `json:"total_nanos"`
	AvgNanos   int64 `json:"avg_nanos"`
	AvgMillis  int64 `json:"avg_millis"`
	MinNanos   int64 `json:"min_nanos"`
	MinMillis  int64 `json:"min_millis"`
	MaxNanos   int64 `json:"max_nanos"`
	MaxMillis  int64 `json:"max_millis"`
	LastNanos  int64 `json:"last_nanos"`
	LastMillis int64 `json:"last_millis"`
}

// Empty is the result of operations with nothing to return.
type Empty struct{}

// --- Event payloads ---

// IngestProgress reports incremental progress of an ingest job.
type IngestProgress struct {
	JobID           string `json:"job_id"`
	IngestedCount   int    `json:"ingested_count"`
	DiscoveredCount int    `json:"discovered_count"`
	PendingCount    int    `json:"pending_count"`
	CitationsCount  int    `json:"citations_count,omitempty"`
	CitedByCount    int    `json:"cited_by_count,omitempty"`
	ParentsCount    int    `json:"parents_count,omitempty"`
	ChildrenCount   int    `json:"children_count,omitempty"`
	Message         string `json:"message"`
}

// IngestDone reports that an ingest job finished, with an error if it failed.
type IngestDone struct {
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
