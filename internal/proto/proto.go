// Package proto defines the client<->daemon wire contract: a JSON-RPC 2.0
// dialect carried as line-delimited JSON. Method and event names are typed
// constants so no caller spells them as a bare string.
package proto

import (
	"encoding/json"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
)

// Version is the JSON-RPC protocol version string.
const Version = "2.0"

// Method names a request a client may send to the daemon.
type Method string

const (
	MethodPing                      Method = "ping"
	MethodPatentGet                 Method = "patent.get"
	MethodPatentAssigneeStats       Method = "patent.assignee_stats"
	MethodPatentClassificationStats Method = "patent.classification_stats"
	MethodPatentInventorStats       Method = "patent.inventor_stats"
	MethodPatentList                Method = "patent.list"
	MethodPatentTableColumns        Method = "patent.table_columns"
	MethodPatentDelete              Method = "patent.delete"
	MethodPatentDeleteBulk          Method = "patent.delete_bulk"
	MethodProjectList               Method = "project.list"
	MethodProjectCreate             Method = "project.create"
	MethodMembershipAdd             Method = "membership.add"
	MethodReviewState               Method = "review_state.set"
	MethodTagPatent                 Method = "tag.assign"
	MethodUntagPatent               Method = "tag.remove"
	MethodCrawlFamily               Method = "crawl.family"
	MethodCrawlConfig               Method = "crawl.config"
	MethodSourceModeGet             Method = "source_mode.get"
	MethodSourceModeSet             Method = "source_mode.set"
	MethodCrawlCancel               Method = "crawl.cancel"
	MethodImportFile                Method = "import.file"
	MethodRelations                 Method = "patent.relations"
	MethodFamilyGraph               Method = "patent.family_graph"
	MethodIDSExport                 Method = "ids.export"
	MethodIDSPDFExport              Method = "ids.export.pdf"
	MethodIDSPDFPreview             Method = "ids.export.pdf.preview"
	MethodProjectUpdate             Method = "project.update"
	MethodIDSEntryGet               Method = "ids.entry.get"
	MethodIDSEntrySave              Method = "ids.entry.save"
	MethodIDSEntryDelete            Method = "ids.entry.delete"
	MethodIDSEntryBulkSetStatus     Method = "ids.entry.bulk_set_status"
	MethodPatentNoteGet             Method = "patent.note.get"
	MethodPatentNoteSave            Method = "patent.note.save"
	MethodPatentNoteDelete          Method = "patent.note.delete"
	MethodPatentNoteList            Method = "patent.note.list"
	MethodPatentNoteExport          Method = "patent.note.export"
	MethodMetricsGet                Method = "metrics.get"
	MethodMetricsPush               Method = "metrics.push"
	MethodActivityRaw               Method = "activity.raw"
	MethodHistoryFeed               Method = "history.feed"
	MethodTagCreate                 Method = "tag.create"
	MethodTagList                   Method = "tag.list"
	MethodTagDelete                 Method = "tag.delete"
	MethodTagPatentStrict           Method = "patent.tag.add"
	MethodUntagPatentStrict         Method = "patent.tag.delete"
	MethodPatentTagList             Method = "patent.tag.list"
	MethodClassificationGet         Method = "classification.get"
	MethodClassificationList        Method = "classification.list"
	MethodClassificationSave        Method = "classification.save"
	MethodClassificationDelete      Method = "classification.delete"
	MethodClassificationLookup      Method = "classification.lookup"
	MethodClassificationListByCodes Method = "classification.list_by_codes"
	MethodPatentClassificationList  Method = "patent.classification.list"
	MethodTableViewList             Method = "table_view.list"
	MethodTableViewGet              Method = "table_view.get"
	MethodTableViewSave             Method = "table_view.save"
	MethodTableViewDelete           Method = "table_view.delete"
	MethodUSPTOFetchXML             Method = "uspto.fetch_xml"
	MethodUSPTOGrantBody            Method = "uspto.grant_body"

	// Source comparison reconciliation (Option A): persist overlay choices.
	MethodSourceResolveDiffs Method = "source.resolve_diffs"
	MethodSourceDiffsList    Method = "source.diffs.list"
)

// USPTOGrantBodyParams identifies the (patent, kind) whose parsed body should
// be returned. Kind defaults to grant; the daemon falls back to pgpub when no
// grant body has been ingested yet.
type USPTOGrantBodyParams struct {
	Number domain.PatentNumber `json:"number"`
	Kind   USPTOXMLKind        `json:"kind,omitempty"`
}

// USPTOGrantBodyResult carries the body parsed from XML, when one has been
// ingested. Present is false when nothing has been parsed for this patent.
type USPTOGrantBodyResult struct {
	Present bool                   `json:"present"`
	Body    domain.USPTOGrantBody  `json:"body,omitempty"`
}

// USPTOXMLKind selects which XML document to fetch for a USPTO application.
type USPTOXMLKind string

const (
	USPTOXMLKindPGPub USPTOXMLKind = "pgpub"
	USPTOXMLKindGrant USPTOXMLKind = "grant"
)

// USPTOFetchXMLParams identifies which XML to download for a patent.
type USPTOFetchXMLParams struct {
	Number domain.PatentNumber `json:"number"`
	Kind   USPTOXMLKind        `json:"kind"`
}

// USPTOFetchXMLResult reports the local file written and how many bytes the
// upstream returned (after unzip, when applicable). Cached is true when the
// XML was served from the local patents dir without re-hitting the API.
// DownloadCount is the total number of times this (application, kind) has
// been requested, including the call that produced this result.
type USPTOFetchXMLResult struct {
	LocalPath     string `json:"local_path"`
	Bytes         int64  `json:"bytes"`
	Cached        bool   `json:"cached"`
	DownloadCount int64  `json:"download_count"`
}

// SourceResolveDiffsParams carries the patent and the (updated) diffs from the
// comparison overlay. The engine will apply ChosenValue to patent fields and
// stamp reconciled metadata on the diffs (Option A).
type SourceResolveDiffsParams struct {
	Number domain.PatentNumber `json:"number"`
	Diffs  []domain.SourceDiff `json:"diffs"`
}

// SourceResolveDiffsResult reports the outcome of reconciliation.
type SourceResolveDiffsResult struct {
	Resolved int    `json:"resolved"`
	Message  string `json:"message,omitempty"`
}

// SourceDiffsListParams / Result for loading diffs for the comparison overlay.
type SourceDiffsListParams struct {
	Number domain.PatentNumber `json:"number"`
}

type SourceDiffsListResult struct {
	Diffs []domain.SourceDiff `json:"diffs"`
}

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
	Pong             bool   `json:"pong"`
	Version          string `json:"version"`
	USPTOConfigured  bool   `json:"uspto_configured"`
	BackupConfigured bool   `json:"backup_configured"`
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
	Patent           domain.Patent            `json:"patent"`
	ReviewState      domain.ReviewState       `json:"review_state,omitempty"`
	Tags             []domain.Tag             `json:"tags,omitempty"`
	IDSEntry         *domain.IDSEntry         `json:"ids_entry,omitempty"`
	PatentNote       *domain.PatentNote       `json:"patent_note,omitempty"`
	USPTOApplication *domain.USPTOApplication `json:"uspto_application,omitempty"`
}

// PatentListParams selects and paginates a patent listing.
type PatentListParams struct {
	Numbers            []domain.PatentNumber `json:"numbers,omitempty"`
	Project            domain.ProjectID      `json:"project,omitempty"`
	Filter             string                `json:"filter,omitempty"`
	ReviewState        domain.ReviewState    `json:"review_state,omitempty"`
	IDSStatus          string                `json:"ids_status,omitempty"`
	Search             string                `json:"search,omitempty"`
	Classification     string                `json:"classification,omitempty"`
	ClassificationCode string                `json:"classification_code,omitempty"`
	Inventor           string                `json:"inventor,omitempty"`
	Assignee           string                `json:"assignee,omitempty"`
	Limit              int                   `json:"limit,omitempty"`
	Offset             int                   `json:"offset,omitempty"`
	SortColumn         domain.SortColumn     `json:"sort_column,omitempty"`
	SortAscending      bool                  `json:"sort_ascending,omitempty"`
}

// PatentListResult carries one page of patents plus the unpaged total.
type PatentListResult struct {
	Patents []domain.PatentRow `json:"patents"`
	Total   int                `json:"total"`
}

// PatentAssigneeStatsParams selects assignee analytics within an optional project scope.
type PatentAssigneeStatsParams struct {
	Project domain.ProjectID `json:"project,omitempty"`
}

// PatentAssigneeStatsResult carries database statistics for assignees.
type PatentAssigneeStatsResult struct {
	Stats []domain.AssigneeStats `json:"stats"`
}

// PatentClassificationStatsParams selects classification analytics within an optional project scope.
type PatentClassificationStatsParams struct {
	Project domain.ProjectID `json:"project,omitempty"`
}

// PatentClassificationStatsResult carries database statistics for classifications.
type PatentClassificationStatsResult struct {
	Stats []domain.ClassificationStats `json:"stats"`
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
	Source  domain.Source       `json:"source,omitempty"`
}

// MembershipAddResult reports the outcome of adding a patent to a project.
type MembershipAddResult struct {
	FetchStarted bool                    `json:"fetch_started"`
	Candidates   []domain.USPTOCandidate `json:"candidates,omitempty"`
}

// ReviewStateParams sets one or more memberships' states.
type ReviewStateParams struct {
	Project domain.ProjectID      `json:"project"`
	Patents []domain.PatentNumber `json:"patents"`
	State   string                `json:"state"`
}

// ReviewStateResult reports the canonical patent records whose state changed.
type ReviewStateResult struct {
	Patents []domain.PatentNumber `json:"patents"`
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

// ClassificationListByCodesParams selects cached classification definitions by
// a set of raw codes. Codes are parsed to (system, code) and matched
// case-insensitively against the cache.
type ClassificationListByCodesParams struct {
	Codes []string `json:"codes"`
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

// SourceModeParams changes normal provider behavior: compare, fallback, or off.
type SourceModeParams struct {
	Mode string `json:"mode"`
}

// SourceModeResult reports normal provider behavior.
type SourceModeResult struct {
	Mode string `json:"mode"`
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
	Filter         string              `json:"filter,omitempty"`
	ReviewState    domain.ReviewState  `json:"review_state,omitempty"`
	IDSStatus      string              `json:"ids_status,omitempty"`
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

// FamilyGraphResult carries a bounded parent/child graph plus traversal attrs.
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

// IDSPDFExportParams selects the project to render an IDS PDF bundle for.
type IDSPDFExportParams struct {
	Project         domain.ProjectID `json:"project"`
	CumulativeCount int              `json:"cumulative_count,omitempty"`
	FeeAmount       string           `json:"fee_amount,omitempty"`
	DepositAccount  string           `json:"deposit_account,omitempty"`
	SignerName      string           `json:"signer_name,omitempty"`
	SignerSignature string           `json:"signer_signature,omitempty"`
	SignerRegNumber string           `json:"signer_reg_number,omitempty"`
}

// IDSPDFExportResult reports where the generated bundle was written.
type IDSPDFExportResult struct {
	Dir       string   `json:"dir"`
	Files     []string `json:"files"`
	FeeTier   int      `json:"fee_tier"`
	PageCount int      `json:"page_count"`
}

// ProjectUpdateParams updates a project's mutable fields (name + IDS header
// fields, inventor list, examiner history).
type ProjectUpdateParams struct {
	Project domain.Project `json:"project"`
}

// IDSPDFPreviewResult is the dry-run summary of an IDS export. It tells the
// caller where files would land, how many sheets the renderer would emit, the
// fee tier, any IDS header fields the project is still missing, and which
// previous exports already exist on disk for the same project.
type IDSPDFPreviewResult struct {
	BaseDir         string   `json:"base_dir"`
	USCount         int      `json:"us_count"`
	ForeignCount    int      `json:"foreign_count"`
	Sheets          int      `json:"sheets"`
	FeeTier         int      `json:"fee_tier"`
	CumulativeCount int      `json:"cumulative_count"`
	ExistingDirs    []string `json:"existing_dirs,omitempty"`
	MissingFields   []string `json:"missing_fields,omitempty"`
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

// IDSEntryBulkSetStatusParams applies a single IDSEntryStatus to every patent
// in Patents under Project. Missing entries are created with InFull=DefaultInFull.
type IDSEntryBulkSetStatusParams struct {
	Project       domain.ProjectID      `json:"project"`
	Patents       []domain.PatentNumber `json:"patents"`
	Status        domain.IDSEntryStatus `json:"status"`
	DefaultInFull bool                  `json:"default_in_full"`
}

// IDSEntriesResult carries every curated IDS entry touched by a bulk write.
type IDSEntriesResult struct {
	Entries []domain.IDSEntry `json:"entries"`
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

// NoteSortBy names how a patent note listing is ordered.
type NoteSortBy string

const (
	NoteSortByDate   NoteSortBy = "date"
	NoteSortByPatent NoteSortBy = "patent"
)

// PatentNoteListParams filters and sorts a patent note listing.
type PatentNoteListParams struct {
	Project domain.ProjectID `json:"project"`
	SortBy  NoteSortBy       `json:"sort_by"`
}

// PatentNoteListResult carries every note for a project.
type PatentNoteListResult struct {
	Notes []domain.PatentNote `json:"notes"`
}

// PatentNoteExportParams requests a markdown export of all notes for a project.
// When OutputPath is set the server writes the file and returns attrs.
// When OutputPath is empty the server returns the markdown in Content.
type PatentNoteExportParams struct {
	Project    domain.ProjectID `json:"project"`
	SortBy     NoteSortBy       `json:"sort_by"`
	OutputPath string           `json:"output_path,omitempty"`
}

// PatentNoteExportResult is returned by a successful export.
type PatentNoteExportResult struct {
	Path    string `json:"path"`    // absolute path written; empty when Content is set
	Count   int    `json:"count"`   // number of notes exported
	Bytes   int    `json:"bytes"`   // size of generated markdown in bytes
	Content string `json:"content"` // markdown body; populated only when OutputPath was empty
}

// MetricsResult carries the daemon's current in-memory timing/counter snapshot.
type MetricsResult struct {
	Metrics MetricsSnapshot `json:"metrics"`
}

// MetricsPushParams carries a client-side metrics snapshot for the daemon to merge.
type MetricsPushParams struct {
	Component string          `json:"component"`
	Snapshot  MetricsSnapshot `json:"snapshot"`
}

// ActivityRawParams selects raw activity records from the daemon journal.
type ActivityRawParams struct {
	Limit     int       `json:"limit,omitempty"`
	Component string    `json:"component,omitempty"`
	Action    string    `json:"action,omitempty"`
	Entity    string    `json:"entity,omitempty"`
	Since     time.Time `json:"since,omitempty"`
}

// ActivityRawResult carries raw activity records and accounting.
type ActivityRawResult struct {
	Records  []observability.Record `json:"records"`
	Limit    int                    `json:"limit"`
	Returned int                    `json:"returned"`
}

// HistoryFeedParams selects the raw activity window to group into history.
type HistoryFeedParams struct {
	RawLimit  int       `json:"raw_limit,omitempty"`
	Component string    `json:"component,omitempty"`
	Since     time.Time `json:"since,omitempty"`
}

// TableViewListParams selects saved views for an owner and optional table type.
type TableViewListParams struct {
	Owner     string           `json:"owner,omitempty"`
	TableType domain.TableType `json:"table_type,omitempty"`
}

// TableViewGetParams identifies one saved table view.
type TableViewGetParams struct {
	Owner string `json:"owner,omitempty"`
	ID    string `json:"id"`
}

// TableViewSaveParams carries a saved table view to insert or update.
type TableViewSaveParams struct {
	View domain.SavedTableView `json:"view"`
}

// TableViewResult carries one saved table view.
type TableViewResult struct {
	View domain.SavedTableView `json:"view"`
}

// TableViewListResult carries saved table views.
type TableViewListResult struct {
	Views []domain.SavedTableView `json:"views"`
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
	Depth           int    `json:"depth"`
	MaxDepth        int    `json:"max_depth"`
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
