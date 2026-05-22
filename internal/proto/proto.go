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
	MethodPing                     Method = "ping"
	MethodPatentGet                Method = "patent.get"
	MethodPatentInventorStats      Method = "patent.inventor_stats"
	MethodPatentList               Method = "patent.list"
	MethodPatentTableColumns       Method = "patent.table_columns"
	MethodPatentDelete             Method = "patent.delete"
	MethodPatentDeleteBulk         Method = "patent.delete_bulk"
	MethodProjectList              Method = "project.list"
	MethodProjectCreate            Method = "project.create"
	MethodMembershipAdd            Method = "membership.add"
	MethodReviewState              Method = "review_state.set"
	MethodTagPatent                Method = "tag.assign"
	MethodUntagPatent              Method = "tag.remove"
	MethodCrawlFamily              Method = "crawl.family"
	MethodCrawlConfig              Method = "crawl.config"
	MethodCrawlCancel              Method = "crawl.cancel"
	MethodImportFile               Method = "import.file"
	MethodRelations                Method = "patent.relations"
	MethodFamilyGraph              Method = "patent.family_graph"
	MethodIDSExport                Method = "ids.export"
	MethodIDSEntryGet              Method = "ids.entry.get"
	MethodIDSEntrySave             Method = "ids.entry.save"
	MethodIDSEntryDelete           Method = "ids.entry.delete"
	MethodPatentNoteGet            Method = "patent.note.get"
	MethodPatentNoteSave           Method = "patent.note.save"
	MethodPatentNoteDelete         Method = "patent.note.delete"
	MethodMetricsGet               Method = "metrics.get"
	MethodTagCreate                Method = "tag.create"
	MethodTagList                  Method = "tag.list"
	MethodTagDelete                Method = "tag.delete"
	MethodTagPatentStrict          Method = "patent.tag.add"
	MethodUntagPatentStrict        Method = "patent.tag.delete"
	MethodPatentTagList            Method = "patent.tag.list"
	MethodClassificationGet        Method = "classification.get"
	MethodClassificationList       Method = "classification.list"
	MethodClassificationSave       Method = "classification.save"
	MethodClassificationDelete     Method = "classification.delete"
	MethodClassificationLookup     Method = "classification.lookup"
	MethodPatentClassificationList Method = "patent.classification.list"
)

// EventKind names a server->client push (a JSON-RPC notification).
type EventKind string

const (
	EventCrawlProgress EventKind = "crawl.progress"
	EventCrawlDone     EventKind = "crawl.done"
	EventDBChanged     EventKind = "db.changed"
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
	Pong            bool   `json:"pong"`
	Version         string `json:"version"`
	USPTOConfigured bool   `json:"uspto_configured"`
}

// PatentDeleteParams identifies the patent to permanently remove.
type PatentDeleteParams struct {
	Number domain.PatentNumber `json:"number"`
}

// PatentDeleteBulkParams identifies the patents to permanently remove.
type PatentDeleteBulkParams struct {
	Patents []domain.PatentNumber `json:"patents"`
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
	IDSEntry    *domain.IDSEntry   `json:"ids_entry,omitempty"`
	PatentNote  *domain.PatentNote `json:"patent_note,omitempty"`
}

// PatentListParams selects and paginates a patent listing.
type PatentListParams struct {
	Project        domain.ProjectID   `json:"project,omitempty"`
	ReviewState    domain.ReviewState `json:"review_state,omitempty"`
	Search         string             `json:"search,omitempty"`
	Classification string             `json:"classification,omitempty"`
	Inventor       string             `json:"inventor,omitempty"`
	Limit          int                `json:"limit,omitempty"`
	Offset         int                `json:"offset,omitempty"`
	SortColumn     domain.SortColumn  `json:"sort_column,omitempty"`
	SortAscending  bool               `json:"sort_ascending,omitempty"`
}

// PatentListResult carries one page of patents plus the unpaged total.
type PatentListResult struct {
	Patents []domain.PatentRow `json:"patents"`
	Total   int                `json:"total"`
}

// PatentInventorStatsResult carries database statistics for multiple inventors.
type PatentInventorStatsResult struct {
	Stats []domain.InventorStats `json:"stats"`
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

// MembershipAddResult reports the outcome of adding a patent to a project.
type MembershipAddResult struct {
	FetchStarted bool `json:"fetch_started"`
}

// ReviewStateParams sets one or more memberships' states.
type ReviewStateParams struct {
	Project domain.ProjectID      `json:"project"`
	Patents []domain.PatentNumber `json:"patents"`
	State   string                `json:"state"`
}

// TagParams names a tag to assign to, or remove from, one or more patents within a
// project. On assign an unknown name creates the tag.
type TagParams struct {
	Project domain.ProjectID      `json:"project"`
	Patents []domain.PatentNumber `json:"patents"`
	Name    string                `json:"name"`
}

// TagCreateParams registers a new tag in the project's taxonomy.
type TagCreateParams struct {
	Project domain.ProjectID `json:"project"`
	Name    string           `json:"name"`
}

// TagDeleteParams removes a tag from the project's taxonomy.
type TagDeleteParams struct {
	Project domain.ProjectID `json:"project"`
	Name    string           `json:"name"`
}

// TagListParams lists all taxonomy tags in the project.
type TagListParams struct {
	Project domain.ProjectID `json:"project"`
}

// TagListResult carries the list of project taxonomy tags.
type TagListResult struct {
	Tags []domain.Tag `json:"tags"`
}

// PatentTagListParams lists tags assigned to a patent.
type PatentTagListParams struct {
	Project domain.ProjectID    `json:"project"`
	Patent  domain.PatentNumber `json:"patent"`
}

// PatentTagListResult carries the list of tags assigned to a patent.
type PatentTagListResult struct {
	Tags []domain.Tag `json:"tags"`
}

// ClassificationGetParams selects a single classification definition.
type ClassificationGetParams struct {
	System string `json:"system"`
	Code   string `json:"code"`
}

// ClassificationListResult carries all classification definitions.
type ClassificationListResult struct {
	Classifications []domain.Classification `json:"classifications"`
}

// ClassificationParams contains a classification definition to save.
type ClassificationParams struct {
	Classification domain.Classification `json:"classification"`
}

// ClassificationDeleteParams identifies the classification to delete.
type ClassificationDeleteParams struct {
	System string `json:"system"`
	Code   string `json:"code"`
}

// ClassificationLookupParams contains the CPC code to lookup.
type ClassificationLookupParams struct {
	Code string `json:"code"`
}

// PatentClassificationListParams lists classifications for a patent.
type PatentClassificationListParams struct {
	Project domain.ProjectID    `json:"project"`
	Patent  domain.PatentNumber `json:"patent"`
}

// CrawlFamilyParams starts a family-graph crawl rooted at one patent. Depth 0
// fetches only the root; a negative depth uses the configured family depth.
// Force bypasses the local file cache and re-fetches from the web.
type CrawlFamilyParams struct {
	Root    domain.PatentNumber `json:"root"`
	Depth   int                 `json:"depth"`
	Profile domain.CrawlProfile `json:"profile,omitempty"`
	Force   bool                `json:"force,omitempty"`
}

// CrawlStartResult returns the id of an enqueued job.
type CrawlStartResult struct {
	JobID string `json:"job_id"`
}

// CrawlConfigResult reports daemon-owned crawl defaults.
type CrawlConfigResult struct {
	MaxDepth int `json:"max_depth"`
}

// ImportFileParams loads a patent record from a local fixture file.
type ImportFileParams struct {
	Path string `json:"path"`
}

// CrawlCancelParams cancels a running job.
type CrawlCancelParams struct {
	JobID string `json:"job_id"`
}

// RelationsParams selects family-graph edges of one kind from one patent.
type RelationsParams struct {
	Number         domain.PatentNumber `json:"number"`
	Kind           domain.RelationKind `json:"kind"`
	Project        domain.ProjectID    `json:"project,omitempty"`
	ReviewState    domain.ReviewState  `json:"review_state,omitempty"`
	Search         string              `json:"search,omitempty"`
	Classification string              `json:"classification,omitempty"`
	Inventor       string              `json:"inventor,omitempty"`
	Limit          int                 `json:"limit,omitempty"`
	Offset         int                 `json:"offset,omitempty"`
	SortColumn     domain.SortColumn   `json:"sort_column,omitempty"`
	SortAscending  bool                `json:"sort_ascending,omitempty"`
}

// RelationsResult carries the requested family-graph edges.
type RelationsResult struct {
	Patents []domain.PatentRow `json:"patents"`
	Total   int                `json:"total"`
}

// FamilyGraphParams selects a bounded family-only DAG around one patent.
type FamilyGraphParams struct {
	Root      domain.PatentNumber `json:"root"`
	Depth     int                 `json:"depth,omitempty"`
	MaxNodes  int                 `json:"max_nodes,omitempty"`
	Project   domain.ProjectID    `json:"project,omitempty"`
	Countries []string            `json:"countries,omitempty"`
}

// FamilyGraphNode is one patent in a family DAG result.
type FamilyGraphNode struct {
	Patent   domain.PatentRow      `json:"patent"`
	Depth    int                   `json:"depth"`
	Parents  []domain.PatentNumber `json:"parents,omitempty"`
	Children []domain.PatentNumber `json:"children,omitempty"`
}

// FamilyGraphEdge is one canonical parent->child edge in a family DAG.
type FamilyGraphEdge struct {
	Parent       domain.PatentNumber `json:"parent"`
	Child        domain.PatentNumber `json:"child"`
	Inconsistent bool                `json:"inconsistent,omitempty"`
}

// FamilyGraphResult carries a bounded parent/child graph plus traversal metadata.
type FamilyGraphResult struct {
	Root               domain.PatentNumber `json:"root"`
	Depth              int                 `json:"depth"`
	MaxNodes           int                 `json:"max_nodes"`
	Countries          []string            `json:"countries,omitempty"`
	Nodes              []FamilyGraphNode   `json:"nodes"`
	Edges              []FamilyGraphEdge   `json:"edges"`
	Truncated          bool                `json:"truncated,omitempty"`
	HiddenByCountry    int                 `json:"hidden_by_country,omitempty"`
	InconsistencyCount int                 `json:"inconsistency_count,omitempty"`
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

// IDSEntryParams identifies one project/patent IDS entry.
type IDSEntryParams struct {
	Project domain.ProjectID    `json:"project"`
	Patent  domain.PatentNumber `json:"patent"`
}

// IDSEntrySaveParams carries the IDS entry to insert or update.
type IDSEntrySaveParams struct {
	Entry domain.IDSEntry `json:"entry"`
}

// IDSEntryResult carries one curated IDS entry.
type IDSEntryResult struct {
	Entry domain.IDSEntry `json:"entry"`
}

// PatentNoteParams identifies one project-scoped patent note.
type PatentNoteParams struct {
	Project domain.ProjectID    `json:"project"`
	Patent  domain.PatentNumber `json:"patent"`
}

// PatentNoteSaveParams carries the patent note to insert or update.
type PatentNoteSaveParams struct {
	Note domain.PatentNote `json:"note"`
}

// PatentNoteResult carries one project-scoped patent note.
type PatentNoteResult struct {
	Note domain.PatentNote `json:"note"`
}

// MetricsResult carries the daemon's current in-memory timing/counter snapshot.
type MetricsResult struct {
	Metrics MetricsSnapshot `json:"metrics"`
}

// MetricsSnapshot is a transport-safe view of the daemon's metrics.
type MetricsSnapshot struct {
	Timestamp time.Time               `json:"timestamp"`
	Timings   map[string]TimingMetric `json:"timings"`
	Counters  map[string]int64        `json:"counters"`
	Gauges    map[string]int64        `json:"gauges"`
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

// CrawlProgress reports incremental progress of a crawl job.
type CrawlProgress struct {
	JobID           string `json:"job_id"`
	CrawledCount    int    `json:"crawled_count"`
	DiscoveredCount int    `json:"discovered_count"`
	PendingCount    int    `json:"pending_count"`
	CitationsCount  int    `json:"citations_count,omitempty"`
	CitedByCount    int    `json:"cited_by_count,omitempty"`
	ParentsCount    int    `json:"parents_count,omitempty"`
	ChildrenCount   int    `json:"children_count,omitempty"`
	Message         string `json:"message"`
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
