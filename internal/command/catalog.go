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
	OpenDetail          ID = "view.detail"
	OpenCitations       ID = "view.citations"
	OpenCitedBy         ID = "view.cited-by"
	OpenIDS             ID = "view.ids"
	OpenProjects        ID = "view.projects"
	OpenInventors       ID = "view.inventors"
	OpenInventorsDirect ID = "view.inventors.direct"
	Back                ID = "view.back"
	CloseOverlay        ID = "view.close-overlay"
	Refresh             ID = "view.refresh"
	OpenSearch          ID = "search.open"
	OpenCommand         ID = "command.open"
	JumpMode            ID = "view.jump"
	SelectVisual        ID = "select.visual"
	SelectClear         ID = "select.clear"
	SelectAll           ID = "select.all"
	ColNext             ID = "col.next"
	ColPrev             ID = "col.prev"
	SortApply           ID = "col.sort-apply"
	OpenBrowser         ID = "view.browser"
	OpenMetrics         ID = "view.metrics"
	OpenPatentNote      ID = "view.patent-note"

	// Application-wide actions.
	Quit ID = "app.quit"
	Help ID = "app.help"

	// Engine reads, backing both pane data loads and web API routes.
	PatentList      ID = "patent.list"
	PatentGet       ID = "patent.get"
	PatentRelations ID = "patent.relations"
	ProjectList     ID = "project.list"

	// ExportIDS builds an Information Disclosure Statement for a project.
	ExportIDS ID = "ids.export"

	// Patent review-state changes. All four map to one proto method; the
	// target state is the difference the dispatcher supplies.
	MarkStored      ID = "patent.mark-stored"
	MarkUnderReview ID = "patent.mark-under-review"
	MarkIgnored     ID = "patent.mark-ignored"
	MarkDeleted     ID = "patent.mark-deleted"
	AddToProject    ID = "patent.add-to-project"

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
	ClassTaxonomyList ID = "class.list"
	ClassLookup       ID = "class.lookup"

	// PatentDelete permanently removes a patent from the database.
	PatentDelete ID = "patent.delete"

	// Crawling.
	CrawlFamily    ID = "crawl.family"
	CrawlCitations ID = "crawl.citations"
	CrawlCitedBy   ID = "crawl.citedby"
	CrawlAll       ID = "crawl.all"
	CrawlCancel    ID = "crawl.cancel"
	LookupPatent   ID = "patent.lookup"
	Import         ID = "patent.import"

	// Projects.
	ProjectCreate      ID = "project.create"
	ProjectActivate    ID = "project.activate"
	ProjectClearActive ID = "project.clear-active"

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
)

// listScopes are the scopes that behave as scrollable lists.
var listScopes = []Scope{ScopeCatalog, ScopeCitations, ScopeProjects, ScopeDetail, ScopeIDS, ScopeFullText}

// patentScopes are the scopes where a patent is selected and can be acted on.
var patentScopes = []Scope{ScopeCatalog, ScopeDetail, ScopeCitations}

// projectScopes are the scopes where project-focused actions are relevant.
var projectScopes = []Scope{ScopeCatalog, ScopeDetail, ScopeCitations, ScopeProjects}

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
		Command{ID: ReselectLast, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},

		// --- panes and overlays (view) ---
		Command{ID: OpenDetail, Name: "open.detail", Aliases: []string{"detail"}, Usage: ":open.detail", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
		Command{ID: OpenBrowser, Name: "browse", Aliases: []string{"open.browser", "web"}, Usage: ":browse [PATENT ...]", Kind: KindView, Scopes: patentScopes},
		Command{ID: OpenMetrics, Name: "metrics", Aliases: []string{"open.metrics", "observability"}, Usage: ":metrics", Kind: KindView},
		Command{ID: OpenCitations, Name: "open.citations", Aliases: []string{"citations"}, Usage: ":open.citations", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail}},
		Command{ID: OpenCitedBy, Name: "open.citedby", Aliases: []string{"citedby"}, Usage: ":open.citedby", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail}},
		Command{ID: OpenProjects, Name: "open.projects", Aliases: []string{"projects"}, Usage: ":open.projects", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeCitations, ScopeIDS}},
		Command{ID: OpenPatentNote, Name: "open.note", Aliases: []string{"patent-note", "project-note"}, Usage: ":open.note", Kind: KindView, Scopes: patentScopes},
		Command{ID: OpenIDS, Name: "open.ids", Aliases: []string{"ids"}, Usage: ":open.ids", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeCitations}},
		Command{ID: OpenInventors, Name: "open.inventors", Aliases: []string{"inventors"}, Usage: ":open.inventors", Kind: KindView, Scopes: []Scope{ScopeDetail}},
		Command{ID: OpenInventorsDirect, Name: "open.inventors.direct", Aliases: []string{"inventors-direct"}, Usage: ":open.inventors.direct", Kind: KindView, Scopes: []Scope{ScopeDetail}},
		Command{ID: Back, Kind: KindView},
		Command{ID: CloseOverlay, Kind: KindView, Scopes: []Scope{ScopeOverlay}},
		Command{ID: Refresh, Name: "refresh", Aliases: []string{"reload"}, Usage: ":refresh", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeCitations, ScopeProjects, ScopeIDS}},
		Command{ID: OpenSearch, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeDetail, ScopeCitations, ScopeProjects}},
		Command{ID: OpenCommand, Kind: KindView},
		Command{ID: JumpMode, Name: "jump", Aliases: []string{"jump-to-field"}, Usage: ":jump", Kind: KindView, Scopes: []Scope{ScopeDetail}},
		Command{ID: SelectVisual, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
		Command{ID: SelectClear, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
		Command{ID: SelectAll, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
		Command{ID: ColNext, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
		Command{ID: ColPrev, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
		Command{ID: SortApply, Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
		Command{ID: ProjectActivate, Name: "project.use", Aliases: []string{"use-project"}, Usage: ":project.use [PROJECT]", Kind: KindView, Scopes: projectScopes},
		Command{ID: ProjectClearActive, Name: "project.clear", Aliases: []string{"clear-project"}, Usage: ":project.clear", Kind: KindView, Scopes: projectScopes},

		// --- filtering (view) ---
		Command{ID: Filter, Name: "filter", Aliases: []string{"f", "filter.clear"}, Usage: ":filter <type> <value>", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
		Command{ID: FindOpen, Name: "find", Aliases: []string{"/"}, Usage: ":find", Kind: KindView, Scopes: []Scope{ScopeCatalog, ScopeCitations}},
		Command{ID: IDSEditField, Kind: KindView, Scopes: []Scope{ScopeIDS}},
		Command{ID: IDSToggleFull, Kind: KindView, Scopes: []Scope{ScopeIDS}},
		Command{ID: IDSCycleStatus, Kind: KindView, Scopes: []Scope{ScopeIDS}},
		Command{ID: IDSDelete, Kind: KindView, Scopes: []Scope{ScopeIDS}},

		// --- application-wide (view) ---
		Command{ID: Quit, Name: "quit", Aliases: []string{"exit"}, Usage: ":quit", Kind: KindView},
		Command{ID: Help, Name: "help", Aliases: []string{"?"}, Usage: ":help", Kind: KindView},

		// --- engine reads ---
		Command{ID: PatentList, Kind: KindEngine, Method: proto.MethodPatentList},
		Command{ID: PatentGet, Kind: KindEngine, Method: proto.MethodPatentGet},
		Command{ID: PatentRelations, Kind: KindEngine, Method: proto.MethodRelations},
		Command{ID: ProjectList, Kind: KindEngine, Method: proto.MethodProjectList},
		Command{ID: ExportIDS, Kind: KindEngine, Method: proto.MethodIDSExport, Scopes: []Scope{ScopeProjects}},

		// --- patent review state (engine) ---
		Command{ID: MarkStored, Name: "review_state.stored", Aliases: []string{"stored"}, Usage: ":review_state.stored", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
		Command{ID: MarkUnderReview, Name: "review_state.under_review", Aliases: []string{"under-review"}, Usage: ":review_state.under_review", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
		Command{ID: MarkIgnored, Name: "review_state.ignored", Aliases: []string{"ignored"}, Usage: ":review_state.ignored", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
		Command{ID: MarkDeleted, Name: "review_state.deleted", Aliases: []string{"deleted"}, Usage: ":review_state.deleted", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
		Command{ID: AddToProject, Name: "add", Aliases: []string{"add-to-project"}, Usage: ":add [PATENT]", Kind: KindEngine, Method: proto.MethodMembershipAdd, Scopes: patentScopes},
		Command{ID: PatentDelete, Name: "delete", Aliases: []string{"delete-patent"}, Usage: ":delete", Kind: KindEngine, Method: proto.MethodPatentDelete, Scopes: patentScopes},
		Command{ID: Tag, Name: "tag", Aliases: []string{"tag-patent"}, Usage: ":tag <name>", Kind: KindEngine, Method: proto.MethodTagPatent, Scopes: patentScopes},
		Command{ID: Untag, Name: "untag", Aliases: []string{"untag-patent"}, Usage: ":untag <name>", Kind: KindEngine, Method: proto.MethodUntagPatent, Scopes: patentScopes},
		Command{ID: TagTaxonomyAdd, Name: "tag.add", Aliases: []string{"create-tag"}, Usage: ":tag.add <name>", Kind: KindEngine, Method: proto.MethodTagCreate, Scopes: projectScopes},
		Command{ID: TagTaxonomyList, Name: "tag.list", Aliases: []string{"list-tags"}, Usage: ":tag.list", Kind: KindEngine, Method: proto.MethodTagList, Scopes: projectScopes},
		Command{ID: TagTaxonomyDelete, Name: "tag.delete", Aliases: []string{"delete-tag"}, Usage: ":tag.delete <name>", Kind: KindEngine, Method: proto.MethodTagDelete, Scopes: projectScopes},
		Command{ID: TagStrict, Name: "tag.patent.add", Aliases: []string{"patent-tag"}, Usage: ":tag.patent.add <name>", Kind: KindEngine, Method: proto.MethodTagPatentStrict, Scopes: patentScopes},
		Command{ID: UntagStrict, Name: "tag.patent.delete", Aliases: []string{"patent-untag"}, Usage: ":tag.patent.delete <name>", Kind: KindEngine, Method: proto.MethodUntagPatentStrict, Scopes: patentScopes},
		Command{ID: TagPatentList, Name: "tag.patent.list", Aliases: []string{"patent-tags"}, Usage: ":tag.patent.list", Kind: KindEngine, Method: proto.MethodPatentTagList, Scopes: patentScopes},
		Command{ID: TagPatentManage, Name: "tag.patent", Aliases: []string{"tag-manage"}, Usage: ":tag.patent", Kind: KindEngine, Method: proto.MethodPatentTagList, Scopes: patentScopes},

		// --- classification definitions ---
		Command{ID: ClassTaxonomyList, Name: "class.list", Aliases: []string{"classifications"}, Usage: ":class.list", Kind: KindEngine, Method: proto.MethodClassificationList},
		Command{ID: ClassLookup, Name: "class.lookup", Aliases: []string{"lookup-class"}, Usage: ":class.lookup <code>", Kind: KindEngine, Method: proto.MethodClassificationLookup},

		// --- crawling (engine) ---
		Command{ID: CrawlFamily, Name: "crawl.family", Aliases: []string{"family"}, Usage: ":crawl.family", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
		Command{ID: CrawlCitations, Name: "crawl.citations", Aliases: []string{"crawl-citations"}, Usage: ":crawl.citations", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
		Command{ID: CrawlCitedBy, Name: "crawl.citedby", Aliases: []string{"crawl-citedby"}, Usage: ":crawl.citedby", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
		Command{ID: CrawlAll, Name: "crawl.all", Aliases: []string{"crawl", "recursion"}, Usage: ":crawl.all", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
		Command{ID: LookupPatent, Name: "lookup", Aliases: []string{"lookup-patent"}, Usage: ":lookup", Kind: KindEngine, Method: proto.MethodCrawlFamily, Scopes: patentScopes},
		Command{ID: Import, Name: "import", Aliases: []string{"import-patent"}, Usage: ":import <number|file> [force]", Kind: KindEngine, Method: proto.MethodCrawlFamily},
		Command{ID: CrawlCancel, Kind: KindEngine, Method: proto.MethodCrawlCancel},

		// --- full text viewer (view) ---
		Command{ID: OpenFullText, Name: "open.fulltext", Aliases: []string{"fulltext", "claims", "all-claims"}, Usage: ":open.fulltext", Kind: KindView, Scopes: []Scope{ScopeDetail}},
		Command{ID: AIAnalyze, Name: "ai.analyze", Aliases: []string{"ai", "analyze"}, Usage: ":ai.analyze", Kind: KindView, Scopes: []Scope{ScopeDetail}},
		Command{ID: SettingsAI, Name: "settings.ai", Aliases: []string{"settings", "config"}, Usage: ":settings.ai", Kind: KindView},
		Command{ID: CopyYank, Name: "copy", Aliases: []string{"yank", "clipboard"}, Usage: ":copy", Kind: KindView, Scopes: []Scope{ScopeFullText}},
		Command{ID: CopyYankMeta, Name: "copy.meta", Aliases: []string{"yank-meta", "copy-with-patent"}, Usage: ":copy.meta", Kind: KindView, Scopes: []Scope{ScopeFullText}},
		Command{ID: NoteAdd, Name: "note.add", Aliases: []string{"add-note", "note"}, Usage: ":note.add", Kind: KindView, Scopes: []Scope{ScopeFullText}},
		Command{ID: NoteOpen, Name: "note.open", Aliases: []string{"notes", "show-notes"}, Usage: ":note.open", Kind: KindView, Scopes: []Scope{ScopeFullText}},

		// --- projects (engine) ---
		Command{ID: ProjectCreate, Name: "project.create", Aliases: []string{"create-project"}, Usage: ":project.create [NAME]", Kind: KindEngine, Method: proto.MethodProjectCreate, Scopes: []Scope{ScopeProjects}},
	)
}
