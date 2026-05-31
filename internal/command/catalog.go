package command

import "patentmine/internal/proto"

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
	MarkActive      ID = "patent.mark-active"
	MarkUnderReview ID = "patent.mark-under-review"
	MarkIgnored     ID = "patent.mark-ignored"
	MarkDeleted     ID = "patent.mark-deleted"
	AddToProject    ID = "patent.add-to-project"
	AddUSPTO        ID = "patent.add-uspto"
	AddGoogle       ID = "patent.add-google"
	AddRelated      ID = "patent.add-related" // add the citations/parents/children of the current selection that are still stubs or lack membership in the active project
	FetchUSPTO            ID = "patent.fetch-uspto"
	FetchUSPTOPGPub       ID = "patent.fetch-uspto-pgpub"
	FetchUSPTOGrant       ID = "patent.fetch-uspto-grant"
	FetchUSPTOAssignments ID = "patent.fetch-uspto-assignments"

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
	CrawlFamily    ID = "crawl.family"
	CrawlDepthMax  ID = "crawl.depth.max"
	CrawlCitations ID = "crawl.citations"
	CrawlCitedBy   ID = "crawl.citedby"
	CrawlAll       ID = "crawl.all"
	CrawlCancel    ID = "crawl.cancel"
	LookupPatent   ID = "patent.lookup"
	Import         ID = "patent.import"
	SourceMode     ID = "source.mode"
	SourceCompare  ID = "source.compare" // review & choose between USPTO/Google data when diffs exist (default USPTO)

	// Projects.
	ProjectCreate      ID = "project.create"
	ProjectActivate    ID = "project.activate"
	ProjectClearActive ID = "project.clear-active"
	ProjectIDSHeader   ID = "project.ids-header"
	IDSExportPDF       ID = "ids.export-pdf"

	// Filtering.
	Filter   ID = "view.filter"
	FindOpen ID = "find.open"

	// IDS entry editing.
	IDSEditField   ID = "ids.edit-field"
	IDSToggleFull  ID = "ids.toggle-full"
	IDSCycleStatus ID = "ids.cycle-status"
	IDSDelete      ID = "ids.delete"

	// Full text viewer.
	OpenFullText ID = "view.fulltext"
	AIAnalyze    ID = "view.ai-menu"
	SettingsAI   ID = "view.settings-ai"
	CopyYank     ID = "edit.copy"
	CopyYankMeta ID = "edit.copy-meta"
	NoteAdd      ID = "edit.note-add"
	NoteOpen     ID = "edit.note-open"

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
// single source of truth: build it once at startup and inject it.
func Default() (*Registry, error) {
	return NewRegistry(
		// --- navigation (view) ---
		Command{ID: NavDown, Kind: KindView, Scopes: listScopes},
		Command{ID: NavUp, Kind: KindView, Scopes: listScopes},
		Command{ID: NavPageDown, Kind: KindView, Scopes: listScopes},
		Command{ID: NavPageUp, Kind: KindView, Scopes: listScopes},
		Command{ID: NavTop, Kind: KindView, Scopes: listScopes},
		Command{ID: NavBottom, Kind: KindView, Scopes: listScopes},
		Command{ID: ReselectLast, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations, ScopeFamily}},

		// --- panes and overlays (view) ---
		Command{ID: OpenDetail, Name: "open.detail", Aliases: []string{"detail"}, Usage: ":open.detail", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations, ScopeFamily}},
		Command{ID: OpenBrowser, Name: "browse", Aliases: []string{"open.browser", "web"}, Usage: ":browse [PATENT ...]", Kind: KindView, Scopes: patentScopes},
		Command{ID: OpenBrowserUSPTO, Name: "browse.uspto", Aliases: []string{"browse-uspto"}, Usage: ":browse.uspto [PATENT ...]", Kind: KindView, Scopes: patentScopes},
		Command{ID: OpenBrowserUSPTOGrant, Name: "browse.uspto.grant", Aliases: []string{"browse-grant", "browse.uspto.patent"}, Usage: ":browse.uspto.grant [PATENT ...]", Kind: KindView, Scopes: patentScopes},
		Command{ID: OpenBrowserUSPTOPGPub, Name: "browse.uspto.pgpub", Aliases: []string{"browse.uspto.pub", "browse-pgpub", "browse-pub"}, Usage: ":browse.uspto.pgpub [PATENT ...]", Kind: KindView, Scopes: patentScopes},
		Command{ID: OpenBrowserGoogle, Name: "browse.google", Aliases: []string{"browse-google"}, Usage: ":browse.google [PATENT ...]", Kind: KindView, Scopes: patentScopes},
		Command{ID: OpenMetrics, Name: "metrics", Aliases: []string{"open.metrics", "observability"}, Usage: ":metrics", Kind: KindView},
		Command{ID: OpenCitations, Name: "open.citations", Aliases: []string{"citations"}, Usage: ":open.citations", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeFamily}},
		Command{ID: OpenCitedBy, Name: "open.citedby", Aliases: []string{"citedby"}, Usage: ":open.citedby", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeFamily}},
		Command{ID: OpenParents, Name: "open.parents", Aliases: []string{"parents"}, Usage: ":open.parents", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeFamily}},
		Command{ID: OpenChildren, Name: "open.children", Aliases: []string{"children"}, Usage: ":open.children", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeFamily}},
		Command{ID: OpenFamilyGraph, Name: "open.family", Aliases: []string{"family-tree", "tree"}, Usage: ":open.family [depth] [COUNTRY ...]", Kind: KindView, Scopes: patentScopes},
		Command{ID: FamilyDepthMore, Name: "family.depth.more", Aliases: []string{"family-depth-more"}, Usage: ":family.depth.more", Kind: KindView, Scopes: []Scope{ScopeFamily}},
		Command{ID: FamilyDepthLess, Name: "family.depth.less", Aliases: []string{"family-depth-less"}, Usage: ":family.depth.less", Kind: KindView, Scopes: []Scope{ScopeFamily}},
		Command{ID: FamilyExportMermaid, Name: "export.family.mermaid", Aliases: []string{"export-mermaid", "copy-mermaid", "family.mermaid"}, Usage: ":export.family.mermaid", Kind: KindView, Scopes: []Scope{ScopeFamily}},
		Command{ID: OpenProjects, Name: "open.projects", Aliases: []string{"projects"}, Usage: ":open.projects", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeCitations, ScopeFamily, ScopeIDS}},
		Command{ID: OpenPatentNote, Name: "open.note", Aliases: []string{"patent-note", "project-note"}, Usage: ":open.note", Kind: KindView, Scopes: patentScopes},
		Command{ID: OpenIDS, Name: "open.ids", Aliases: []string{"ids"}, Usage: ":open.ids", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeCitations, ScopeFamily}},
		Command{ID: OpenAssignees, Name: "open.assignees", Aliases: []string{"assignees"}, Usage: ":open.assignees", Kind: KindView, Scopes: []Scope{ScopeDetail}},
		Command{ID: PatentExpirationDate, Name: "patent.expiration-date", Aliases: []string{"expiration-date", "expiration", "exp"}, Usage: ":patent.expiration-date [number] [refresh]", Kind: KindView, Scopes: patentScopes},
		Command{ID: OpenClassificationStats, Name: "open.classification-stats", Aliases: []string{"classification-stats", "class-stats"}, Usage: ":open.classification-stats", Kind: KindView, Scopes: []Scope{ScopeDetail}},
		Command{ID: OpenInventors, Name: "open.inventors", Aliases: []string{"inventors"}, Usage: ":open.inventors", Kind: KindView, Scopes: []Scope{ScopeDetail}},
		Command{ID: OpenInventorsDirect, Name: "open.inventors.direct", Aliases: []string{"inventors-direct"}, Usage: ":open.inventors.direct", Kind: KindView, Scopes: []Scope{ScopeDetail}},
		Command{ID: Back, Kind: KindView},
		Command{ID: CloseOverlay, Kind: KindView, Scopes: []Scope{ScopeOverlay}},
		Command{ID: HistoryBack, Name: "history.back", Aliases: []string{"back-history"}, Usage: ":history.back", Kind: KindView},
		Command{ID: HistoryForward, Name: "history.forward", Aliases: []string{"forward-history"}, Usage: ":history.forward", Kind: KindView},
		Command{ID: OpenHistory, Name: "open.history", Aliases: []string{"history", "visits"}, Usage: ":open.history", Kind: KindView},
		Command{ID: OpenActivity, Name: "open.activity", Aliases: []string{"activity", "replay"}, Usage: ":open.activity", Kind: KindView},
		Command{ID: OpenAllNotes, Name: "open.notes", Aliases: []string{"all-notes", "notes"}, Usage: ":open.notes", Kind: KindView},
		Command{ID: OpenOrphans, Name: "orphans", Aliases: []string{"open.orphans", "unassigned"}, Usage: ":orphans", Kind: KindView},
		Command{ID: NotesSortToggle, Kind: KindView, Scopes: []Scope{ScopeNotes}},
		Command{ID: NotesExportMD, Name: "export.notes", Aliases: []string{"export-notes", "notes.export"}, Usage: ":export.notes", Kind: KindView, Scopes: []Scope{ScopeNotes}},
		Command{ID: Refresh, Name: "refresh", Aliases: []string{"reload"}, Usage: ":refresh", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeCitations, ScopeFamily, ScopeProjects, ScopeIDS}},
		Command{ID: OpenSearch, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeCitations, ScopeFamily, ScopeProjects}},
		Command{ID: OpenCommand, Kind: KindView},
		Command{ID: JumpMode, Name: "jump", Aliases: []string{"jump-to-field"}, Usage: ":jump", Kind: KindView, Scopes: []Scope{ScopeDetail}},
		Command{ID: SelectVisual, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations, ScopeFamily}},
		Command{ID: SelectClear, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations, ScopeFamily}},
		Command{ID: SelectAll, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations, ScopeFamily}},
		Command{ID: HighlightFamily, Name: "highlight.family", Aliases: []string{"hl-family"}, Usage: ":highlight.family", Kind: KindView, Scopes: []Scope{ScopeCatalog}},
		Command{ID: HighlightCitations, Name: "highlight.citations", Aliases: []string{"hl-citations", "hl-references"}, Usage: ":highlight.citations", Kind: KindView, Scopes: []Scope{ScopeCatalog}},
		Command{ID: RelationFilterCollapse, Name: "relation-filter.collapse", Aliases: []string{"collapse", "fold", "relation_filter.collapse"}, Usage: ":relation-filter.collapse", Kind: KindView, Scopes: []Scope{ScopeCatalog}},
		Command{ID: RelationFilterExpand, Name: "relation-filter.expand", Aliases: []string{"expand", "unfold", "relation_filter.expand"}, Usage: ":relation-filter.expand", Kind: KindView, Scopes: []Scope{ScopeCatalog}},
		Command{ID: ColNext, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
		Command{ID: ColPrev, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
		Command{ID: SortApply, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
		Command{ID: ProjectActivate, Name: "project.use", Aliases: []string{"use-project"}, Usage: ":project.use [PROJECT]", Kind: KindView, Scopes: projectScopes},
		Command{ID: ProjectClearActive, Name: "project.clear", Aliases: []string{"clear-project"}, Usage: ":project.clear", Kind: KindView, Scopes: projectScopes},

		// --- filtering (view) ---
		Command{ID: Filter, Name: "filter", Aliases: []string{"f", "filter.clear"}, Usage: ":filter <expr>", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations, ScopeFamily}},
		Command{ID: FindOpen, Name: "find", Aliases: []string{"/"}, Usage: ":find", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
		Command{ID: IDSEditField, Kind: KindView, Scopes: []Scope{ScopeIDS}},
		Command{ID: IDSToggleFull, Kind: KindView, Scopes: []Scope{ScopeIDS}},
		Command{ID: IDSCycleStatus, Name: "ids.cycle-status", Aliases: []string{"ids.cycle", "cycle-ids"}, Usage: ":ids.cycle-status", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeCitations, ScopeFamily, ScopeIDS}},
		Command{ID: IDSDelete, Kind: KindView, Scopes: []Scope{ScopeIDS}},

		// --- application-wide (view) ---
		Command{ID: Quit, Name: "quit", Aliases: []string{"exit"}, Usage: ":quit", Kind: KindView},
		Command{ID: Help, Name: "help", Aliases: []string{"?"}, Usage: ":help", Kind: KindView},

		// --- engine reads ---
		Command{ID: PatentList, Kind: KindEngine, Method: proto.MethodPatentList},
		Command{ID: PatentGet, Kind: KindEngine, Method: proto.MethodPatentGet},
		Command{ID: PatentRelations, Kind: KindEngine, Method: proto.MethodRelations},
		Command{ID: PatentFamilyGraph, Kind: KindEngine, Method: proto.MethodFamilyGraph},
		Command{ID: ProjectList, Kind: KindEngine, Method: proto.MethodProjectList},
		Command{ID: ExportIDS, Kind: KindEngine, Method: proto.MethodIDSExport, Scopes: []Scope{ScopeProjects}},

		// --- patent review state (engine) ---
		Command{ID: MarkActive, Name: "review-state.active", Aliases: []string{"active", "review_state.active"}, Usage: ":review-state.active", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
		Command{ID: MarkUnderReview, Name: "review-state.under-review", Aliases: []string{"under-review", "review_state.under_review"}, Usage: ":review-state.under-review", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
		Command{ID: MarkIgnored, Name: "review-state.ignored", Aliases: []string{"ignored", "review_state.ignored"}, Usage: ":review-state.ignored", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
		Command{ID: MarkDeleted, Name: "review-state.deleted", Aliases: []string{"deleted", "review_state.deleted"}, Usage: ":review-state.deleted", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
		Command{ID: AddToProject, Name: "add", Aliases: []string{"add-to-project"}, Usage: ":add [PATENT]", Kind: KindEngine, Method: proto.MethodMembershipAdd, Scopes: patentScopes},
		Command{ID: AddUSPTO, Name: "add.uspto", Aliases: []string{"add-uspto"}, Usage: ":add.uspto [PATENT]", Kind: KindEngine, Method: proto.MethodMembershipAdd},
		Command{ID: AddGoogle, Name: "add.google", Aliases: []string{"add-google"}, Usage: ":add.google [PATENT]", Kind: KindEngine, Method: proto.MethodMembershipAdd},
		Command{ID: AddRelated, Name: "add.related", Aliases: []string{"add-stubs", "add.neighbors", "pull-refs", "add.refs", "add.related-patents"}, Usage: ":add.related", Kind: KindEngine, Method: proto.MethodAddRelated, Scopes: patentScopes},
		Command{ID: FetchUSPTO, Name: "fetch.uspto", Aliases: []string{"fetch", "fetch-uspto"}, Usage: ":fetch.uspto", Kind: KindEngine, Method: proto.MethodUSPTOFetchXML, Scopes: patentScopes},
		Command{ID: FetchUSPTOPGPub, Name: "fetch.uspto.pgpub", Aliases: []string{"fetch-pgpub"}, Usage: ":fetch.uspto.pgpub", Kind: KindEngine, Method: proto.MethodUSPTOFetchXML, Scopes: patentScopes},
		Command{ID: FetchUSPTOGrant, Name: "fetch.uspto.grant", Aliases: []string{"fetch-grant"}, Usage: ":fetch.uspto.grant", Kind: KindEngine, Method: proto.MethodUSPTOFetchXML, Scopes: patentScopes},
		Command{ID: FetchUSPTOAssignments, Name: "fetch.uspto.assignments", Aliases: []string{"fetch-assignments"}, Usage: ":fetch.uspto.assignments", Kind: KindEngine, Method: proto.MethodUSPTOFetchAssignments, Scopes: patentScopes},
		Command{ID: PatentDelete, Name: "delete", Aliases: []string{"delete-patent"}, Usage: ":delete", Kind: KindEngine, Method: proto.MethodPatentDelete, Scopes: patentScopes},
		Command{ID: ClearPatentCache, Name: "patent.clear-cache", Aliases: []string{"clear-cache", "clear.cache", "patent.clear_cache"}, Usage: ":patent.clear-cache [PATENT ...]", Kind: KindEngine, Method: proto.MethodPatentClearCache, Scopes: patentScopes},
		Command{ID: Tag, Name: "tag", Aliases: []string{"tag-patent"}, Usage: ":tag <name>", Kind: KindEngine, Method: proto.MethodTagPatent, Scopes: patentScopes},
		Command{ID: Untag, Name: "untag", Aliases: []string{"untag-patent"}, Usage: ":untag <name>", Kind: KindEngine, Method: proto.MethodUntagPatent, Scopes: patentScopes},
		Command{ID: TagTaxonomyAdd, Name: "tag.create", Aliases: []string{"create-tag", "tag.add"}, Usage: ":tag.create <name>", Kind: KindEngine, Method: proto.MethodTagCreate, Scopes: projectScopes},
		Command{ID: TagTaxonomyList, Name: "tag.list", Aliases: []string{"list-tags"}, Usage: ":tag.list", Kind: KindEngine, Method: proto.MethodTagList, Scopes: projectScopes},
		Command{ID: TagTaxonomyDelete, Name: "tag.delete", Aliases: []string{"delete-tag"}, Usage: ":tag.delete <name>", Kind: KindEngine, Method: proto.MethodTagDelete, Scopes: projectScopes},
		Command{ID: TagStrict, Name: "tag.patent.add", Aliases: []string{"patent-tag"}, Usage: ":tag.patent.add <name>", Kind: KindEngine, Method: proto.MethodTagPatentStrict, Scopes: patentScopes},
		Command{ID: UntagStrict, Name: "tag.patent.delete", Aliases: []string{"patent-untag"}, Usage: ":tag.patent.delete <name>", Kind: KindEngine, Method: proto.MethodUntagPatentStrict, Scopes: patentScopes},
		Command{ID: TagPatentList, Name: "tag.patent.list", Aliases: []string{"patent-tags"}, Usage: ":tag.patent.list", Kind: KindEngine, Method: proto.MethodPatentTagList, Scopes: patentScopes},
		Command{ID: TagPatentManage, Name: "tag.patent", Aliases: []string{"tag-manage"}, Usage: ":tag.patent", Kind: KindEngine, Method: proto.MethodPatentTagList, Scopes: patentScopes},

		// --- classification definitions ---
		Command{ID: ClassificationTaxonomyList, Name: "class.list", Aliases: []string{"classifications"}, Usage: ":class.list", Kind: KindEngine, Method: proto.MethodClassificationList},
		Command{ID: ClassificationLookup, Name: "class.lookup", Aliases: []string{"lookup-class"}, Usage: ":class.lookup <code>", Kind: KindEngine, Method: proto.MethodClassificationLookup},
		Command{ID: OpenPatentClassifications, Name: "open.classifications", Aliases: []string{"class", "patent-classifications"}, Usage: ":open.classifications", Kind: KindView, Scopes: patentScopes},

		// --- crawling (engine) ---
		Command{ID: CrawlFamily, Name: "crawl.family", Aliases: []string{"family"}, Usage: ":crawl.family", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
		Command{ID: CrawlDepthMax, Name: "crawl.depth.max", Aliases: []string{"family.depth.max", "crawl-depth-max"}, Usage: ":crawl.depth.max", Kind: KindView},
		Command{ID: CrawlCitations, Name: "crawl.citations", Aliases: []string{"crawl-citations"}, Usage: ":crawl.citations", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
		Command{ID: CrawlCitedBy, Name: "crawl.citedby", Aliases: []string{"crawl-citedby"}, Usage: ":crawl.citedby", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
		Command{ID: CrawlAll, Name: "crawl.all", Aliases: []string{"crawl", "recursion"}, Usage: ":crawl.all", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
		Command{ID: LookupPatent, Name: "lookup", Aliases: []string{"lookup-patent"}, Usage: ":lookup", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
		Command{ID: Import, Name: "import", Aliases: []string{"import-patent"}, Usage: ":import <number|file> [force]", Kind: KindEngine, Method: proto.MethodCrawlFamily},
		Command{ID: SourceMode, Name: "source.mode", Aliases: []string{"source-mode"}, Usage: ":source.mode <compare|uspto-first|uspto-only|google-only>", Kind: KindView},
		Command{ID: SourceCompare, Name: "source.compare", Aliases: []string{"compare", "compare-sources"}, Usage: ":source.compare", Kind: KindView, Scopes: []Scope{ScopeDetail}},
		Command{ID: CrawlCancel, Kind: KindEngine, Method: proto.MethodCrawlCancel},

		// --- full text viewer (view) ---
		Command{ID: OpenFullText, Name: "open.fulltext", Aliases: []string{"fulltext", "claims", "all-claims"}, Usage: ":open.fulltext", Kind: KindView, Scopes: []Scope{ScopeDetail}},
		Command{ID: AIAnalyze, Name: "ai.analyze", Aliases: []string{"ai", "analyze"}, Usage: ":ai.analyze", Kind: KindView, Scopes: []Scope{ScopeDetail}},
		Command{ID: SettingsAI, Name: "settings.ai", Aliases: []string{"settings", "config"}, Usage: ":settings.ai", Kind: KindView},
		Command{ID: CopyYank, Name: "copy", Aliases: []string{"yank", "clipboard"}, Usage: ":copy", Kind: KindView, Scopes: []Scope{ScopeFullText}},
		Command{ID: CopyYankMeta, Name: "copy.meta", Aliases: []string{"yank-meta", "copy-with-patent"}, Usage: ":copy.meta", Kind: KindView, Scopes: []Scope{ScopeFullText}},
		Command{ID: NoteAdd, Name: "note.add", Aliases: []string{"add-note", "note"}, Usage: ":note.add", Kind: KindView, Scopes: []Scope{ScopeFullText}},
		Command{ID: NoteOpen, Name: "note.open", Aliases: []string{"show-notes"}, Usage: ":note.open", Kind: KindView, Scopes: []Scope{ScopeFullText}},

		// --- projects (engine) ---
		Command{ID: ProjectCreate, Name: "project.create", Aliases: []string{"create-project"}, Usage: ":project.create [NAME]", Kind: KindEngine, Method: proto.MethodProjectCreate, Scopes: []Scope{ScopeProjects}},
		Command{ID: ProjectIDSHeader, Name: "project.ids-header", Aliases: []string{"ids-header", "project-header"}, Usage: ":project.ids-header", Kind: KindView, Scopes: projectScopes},
		Command{ID: IDSExportPDF, Name: "export.ids.pdf", Aliases: []string{"export-ids-pdf", "ids-pdf", "ids.export.pdf"}, Usage: ":export.ids.pdf", Kind: KindEngine, Method: proto.MethodIDSPDFExport, Scopes: projectScopes},
	)
}
