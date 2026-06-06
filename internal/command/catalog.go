package command

// Command IDs. Every action in the system is named here exactly once; no other
// file spells a command identifier as a bare string.
const (
	// Navigation within a list or scrollable pane.
	NavDown      ID = "nav.down"
	NavUp        ID = "nav.up"
	NavPageDown  ID = "nav.page-down"
	NavPageUp    ID = "nav.page-up"
	NavTop       ID = "nav.top"
	NavBottom    ID = "nav.bottom"
	ReselectLast ID = "nav.reselect-last"

	// Moving between panes and overlays.
	OpenDetail              ID = "view.detail"
	OpenCitations           ID = "view.citations"
	OpenCitedBy             ID = "view.cited-by"
	OpenParents             ID = "view.parents"
	OpenChildren            ID = "view.children"
	OpenFamilyGraph         ID = "view.family"
	OpenIDS                 ID = "view.ids"
	OpenProjects            ID = "view.projects"
	OpenAssignees           ID = "view.assignees"
	OpenAssigneesProject    ID = "view.assignees-project"
	OpenAllAssigneesHistory ID = "view.all-assignees-history"
	PatentExpirationDate    ID = "patent.expiration-date"
	OpenClassificationStats ID = "view.classification-stats"
	OpenInventors           ID = "view.inventors"
	OpenInventorsDirect     ID = "view.inventors.direct"
	Back                    ID = "view.back"
	CloseOverlay            ID = "view.close-overlay"
	Refresh                 ID = "view.refresh"
	OpenSearch              ID = "search.open"
	OpenCommand             ID = "command.open"
	JumpMode                ID = "view.jump"
	SelectVisual            ID = "select.visual"
	SelectClear             ID = "select.clear"
	SelectAll               ID = "select.all"
	HighlightFamily         ID = "highlight.family"
	HighlightCitations      ID = "highlight.citations"
	RelationFilterCollapse  ID = "relation_filter.collapse"
	RelationFilterExpand    ID = "relation_filter.expand"
	ColNext                 ID = "col.next"
	ColPrev                 ID = "col.prev"
	SortApply               ID = "col.sort-apply"
	OpenBrowser             ID = "view.browser"
	OpenBrowserUSPTO        ID = "view.browser-uspto"
	OpenBrowserUSPTOGrant   ID = "view.browser-uspto-grant"
	OpenBrowserUSPTOPGPub   ID = "view.browser-uspto-pgpub"
	OpenBrowserGoogle       ID = "view.browser-google"
	OpenMetrics             ID = "view.metrics"
	OpenPatentNote          ID = "view.patent-note"
	FamilyDepthMore         ID = "view.family-depth-more"
	FamilyDepthLess         ID = "view.family-depth-less"
	FamilyExportMermaid     ID = "view.family-export-mermaid"

	// Application-wide actions.
	Quit ID = "app.quit"
	Help ID = "app.help"

	// Engine reads, backing both pane data loads and web API routes.
	PatentList        ID = "patent.list"
	PatentGet         ID = "patent.get"
	PatentRelations   ID = "patent.relations"
	PatentFamilyGraph ID = "patent.family_graph"
	ProjectList       ID = "project.list"

	// ExportIDS builds an Information Disclosure Statement for a project.
	ExportIDS ID = "ids.export"

	// Patent review-state changes. All four map to one proto method; the
	// target state is the difference the dispatcher supplies.
	MarkActive              ID = "patent.mark-active"
	MarkUnderReview         ID = "patent.mark-under-review"
	MarkIgnored             ID = "patent.mark-ignored"
	MarkDeleted             ID = "patent.mark-deleted"
	AddToProject            ID = "patent.add-to-project"
	AddUSPTO                ID = "patent.add-uspto"
	AddGoogle               ID = "patent.add-google"
	AddRelated              ID = "patent.add-related" // add the citations/parents/children of the current selection that are still stubs or lack membership in the active project
	FetchUSPTO              ID = "patent.fetch-uspto"
	FetchUSPTOPGPub         ID = "patent.fetch-uspto-pgpub"
	FetchUSPTOGrant         ID = "patent.fetch-uspto-grant"
	FetchUSPTOAssignments   ID = "patent.fetch-uspto-assignments"
	FetchAssignmentsProject ID = "patent.fetch-uspto-assignments-project"

	// Tagging. Both act on the selected patent within the active project.
	Tag   ID = "patent.tag"
	Untag ID = "patent.untag"

	// Tag taxonomy and patent tag assignments.
	TagTaxonomyAdd    ID = "tag.add"
	TagTaxonomyList   ID = "tag.list"
	TagTaxonomyDelete ID = "tag.delete"
	TagStrict         ID = "tag.patent.add"
	UntagStrict       ID = "tag.patent.delete"
	TagPatentList     ID = "tag.patent.list"
	TagPatentManage   ID = "tag.patent"

	// Classification definitions.
	ClassificationTaxonomyList ID = "class.list"
	ClassificationLookup       ID = "class.lookup"
	OpenPatentClassifications  ID = "patent.classifications"

	// PatentDelete permanently removes a patent from the database.
	PatentDelete ID = "patent.delete"

	// ClearPatentCache clears the parsed XML bodies cache.
	ClearPatentCache ID = "patent.clear-cache"

	// Crawling.
	CrawlFamily       ID = "crawl.family"
	CrawlDepthMax     ID = "crawl.depth.max"
	CrawlCitations    ID = "crawl.citations"
	CrawlCitedBy      ID = "crawl.citedby"
	CrawlAll          ID = "crawl.all"
	CrawlCancel       ID = "crawl.cancel"
	LookupPatent      ID = "patent.lookup"
	Import            ID = "patent.import"
	AddFile           ID = "patent.add-file"      // bulk-add patent numbers listed in a plain-text file into the active project
	ExportAdded       ID = "project.export-added" // write the active project's manually-added patents to a plain-text list file
	AddOfficeAction   ID = "officeaction.add"     // import an Office Action document (with examiner metadata) into the active project
	ListOfficeActions ID = "officeaction.list"    // open the project's office-action table; select one to drill into the detail/notes editor
	AddDocument       ID = "matter.document.add"  // import a supporting document (reference, response, …) into the active matter
	OpenDocuments     ID = "matter.document.open" // open the matter's document list (view / rename / delete)
	SetMatterType     ID = "project.matter-type"  // set the project's prosecution stage (provisional / nonprovisional / …)
	DraftResponse     ID = "officeaction.respond" // draft an office-action response, copying from the matter's documents
	LogComm           ID = "matter.comm.log"      // record a communications-log entry (call/email/interview) for the matter
	OpenComms         ID = "matter.comm.open"     // open the matter's communications log
	FlagConflict      ID = "conflict.flag"        // flag a record as conflicting (material prior art) in the active matter
	ListConflicts     ID = "conflict.list"        // open the matter's conflicts list (resolve / waive / delete)
	LogTime           ID = "time.log"             // record a manual time entry for the matter
	ValidateTime      ID = "time.validate"        // review + validate auto-captured time before billing
	ShowTime          ID = "time.show"            // show the matter's time + AI-usage summary
	ShowDeadlines     ID = "deadline.show"        // open the cross-matter deadlines docket
	TrackRenewals     ID = "deadline.track"       // track a patent's U.S. maintenance-fee deadlines
	UntrackRenewals   ID = "deadline.untrack"     // stop tracking a patent's renewals
	SourceMode        ID = "source.mode"
	SourceCompare     ID = "source.compare" // review & choose between USPTO/Google data when diffs exist (default USPTO)
	SourceBibs        ID = "source.bibs"    // read-only all-fields side-by-side of every source's bibliographic data

	// Projects.
	ProjectCreate      ID = "project.create"
	ProjectActivate    ID = "project.activate"
	ProjectClearActive ID = "project.clear-active"
	ProjectIDSHeader   ID = "project.ids-header"
	IDSExportPDF       ID = "ids.export-pdf"

	// Filtering.
	Filter   ID = "view.filter"
	FindOpen ID = "find.open"
	FindNext ID = "find.next"
	FindPrev ID = "find.prev"

	// IDS entry editing.
	IDSEditField   ID = "ids.edit-field"
	IDSToggleFull  ID = "ids.toggle-full"
	IDSCycleStatus ID = "ids.cycle-status"
	IDSDelete      ID = "ids.delete"

	// Full text viewer.
	OpenFullText      ID = "view.fulltext"
	AIAnalyze         ID = "view.ai-menu"
	SettingsAI        ID = "view.settings-ai"
	CopyYank          ID = "edit.copy"
	CopyYankMeta      ID = "edit.copy-meta"
	CopyAll           ID = "edit.copy-all"
	FullTextStageNext ID = "fulltext.stage-next"
	FullTextStagePrev ID = "fulltext.stage-prev"
	FullTextCollapse  ID = "fulltext.collapse-matches"
	FullTextQuickList ID = "fulltext.quicklist"
	NoteAdd           ID = "edit.note-add"
	NoteOpen          ID = "edit.note-open"

	// History navigation and visual popup.
	HistoryBack    ID = "history.back"
	HistoryForward ID = "history.forward"
	OpenHistory    ID = "view.history"
	OpenActivity   ID = "view.activity"

	// All-notes pane.
	OpenAllNotes    ID = "view.all-notes"
	NotesSortToggle ID = "notes.sort-toggle"
	NotesExportMD   ID = "notes.export-md"

	// Orphans pane: patents in DB with no project membership.
	OpenOrphans ID = "patent.orphan-list"
)

// listScopes are the scopes that behave as scrollable lists.
var listScopes = []Scope{ScopeCatalog, ScopeCitations, ScopeFamily, ScopeProjects, ScopeDetail, ScopeIDS, ScopeFullText, ScopeNotes, ScopeOrphans}

// patentScopes are the scopes where a patent is selected and can be acted on.
var patentScopes = []Scope{ScopeCatalog, ScopeDetail, ScopeCitations, ScopeFamily}

// projectScopes are the scopes where project-focused actions are relevant.
var projectScopes = []Scope{ScopeCatalog, ScopeDetail, ScopeCitations, ScopeFamily, ScopeProjects}

// Default returns the registry of every command the system supports. It is the
// single source of truth: build it once at startup and inject it. The command
// IDs above are declared here once; the per-domain registration lists live in
// catalog_<domain>.go (mirroring internal/proto and internal/rpc) and are
// assembled below.
func Default() (*Registry, error) {
	var cmds []Command
	cmds = append(cmds, viewCommands()...)
	cmds = append(cmds, patentCommands()...)
	cmds = append(cmds, tagCommands()...)
	cmds = append(cmds, classificationCommands()...)
	cmds = append(cmds, crawlCommands()...)
	cmds = append(cmds, projectCommands()...)
	cmds = append(cmds, draftingCommands()...)
	return NewRegistry(cmds...)
}
