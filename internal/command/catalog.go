package command

import "patentmine/internal/proto"

// Command IDs. Every action in the system is named here exactly once; no other
// file spells a command identifier as a bare string.
const (
	// Navigation within a list or scrollable pane.
	NavDown     ID = "nav.down"
	NavUp       ID = "nav.up"
	NavPageDown ID = "nav.page-down"
	NavPageUp   ID = "nav.page-up"
	NavTop      ID = "nav.top"
	NavBottom   ID = "nav.bottom"

	// Moving between panes and overlays.
	OpenDetail    ID = "view.detail"
	OpenCitations ID = "view.citations"
	OpenCitedBy   ID = "view.cited-by"
	OpenProjects  ID = "view.projects"
	Back          ID = "view.back"
	CloseOverlay  ID = "view.close-overlay"
	Refresh       ID = "view.refresh"
	OpenSearch    ID = "search.open"
	OpenCommand   ID = "command.open"
	JumpMode      ID = "view.jump"
	SelectVisual  ID = "select.visual"
	SelectClear   ID = "select.clear"
	ColNext       ID = "col.next"
	ColPrev       ID = "col.prev"
	SortApply     ID = "col.sort-apply"

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
	TagAdd    ID = "patent.tag"
	TagRemove ID = "patent.untag"

	// PatentDelete permanently removes a patent from the database.
	PatentDelete ID = "patent.delete"

	// Ingestion.
	IngestFamily    ID = "ingest.family"
	IngestCitations ID = "ingest.citations"
	IngestCitedBy   ID = "ingest.citedby"
	IngestAll       ID = "ingest.all"
	IngestCancel    ID = "ingest.cancel"
	FetchPatent     ID = "patent.fetch"
	Import          ID = "patent.import"

	// Projects.
	ProjectCreate      ID = "project.create"
	ProjectActivate    ID = "project.activate"
	ProjectClearActive ID = "project.clear-active"

	// Filtering.
	Filter   ID = "view.filter"
	FindOpen ID = "find.open"
)

// listScopes are the contexts that behave as scrollable lists.
var listScopes = []Context{ContextCatalog, ContextCitations, ContextProjects, ContextDetail}

// patentScopes are the contexts where a patent is selected and can be acted on.
var patentScopes = []Context{ContextCatalog, ContextDetail, ContextCitations}

// projectScopes are the contexts where project-focused actions are relevant.
var projectScopes = []Context{ContextCatalog, ContextDetail, ContextCitations, ContextProjects}

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

		// --- panes and overlays (view) ---
		Command{ID: OpenDetail, Name: "open.detail", Aliases: []string{"detail"}, Usage: ":open.detail", Kind: KindView, Scopes: []Context{ContextCatalog, ContextCitations}},
		Command{ID: OpenCitations, Name: "open.citations", Aliases: []string{"citations"}, Usage: ":open.citations", Kind: KindView, Scopes: []Context{ContextCatalog, ContextDetail}},
		Command{ID: OpenCitedBy, Name: "open.citedby", Aliases: []string{"citedby"}, Usage: ":open.citedby", Kind: KindView, Scopes: []Context{ContextCatalog, ContextDetail}},
		Command{ID: OpenProjects, Name: "open.projects", Aliases: []string{"projects"}, Usage: ":open.projects", Kind: KindView, Scopes: []Context{ContextCatalog, ContextDetail, ContextCitations}},
		Command{ID: Back, Kind: KindView},
		Command{ID: CloseOverlay, Kind: KindView, Scopes: []Context{ContextOverlay}},
		Command{ID: Refresh, Name: "refresh", Aliases: []string{"reload"}, Usage: ":refresh", Kind: KindView, Scopes: []Context{ContextCatalog, ContextDetail, ContextCitations, ContextProjects}},
		Command{ID: OpenSearch, Kind: KindView, Scopes: []Context{ContextCatalog, ContextDetail, ContextCitations, ContextProjects}},
		Command{ID: OpenCommand, Kind: KindView},
		Command{ID: JumpMode, Name: "jump", Aliases: []string{"jump-to-field"}, Usage: ":jump", Kind: KindView, Scopes: []Context{ContextDetail}},
		Command{ID: SelectVisual, Kind: KindView, Scopes: []Context{ContextCatalog, ContextCitations}},
		Command{ID: SelectClear, Kind: KindView, Scopes: []Context{ContextCatalog, ContextCitations}},
		Command{ID: ColNext, Kind: KindView, Scopes: []Context{ContextCatalog, ContextCitations}},
		Command{ID: ColPrev, Kind: KindView, Scopes: []Context{ContextCatalog, ContextCitations}},
		Command{ID: SortApply, Kind: KindView, Scopes: []Context{ContextCatalog, ContextCitations}},
		Command{ID: ProjectActivate, Name: "project.use", Aliases: []string{"use-project"}, Usage: ":project.use [PROJECT]", Kind: KindView, Scopes: projectScopes},
		Command{ID: ProjectClearActive, Name: "project.clear", Aliases: []string{"clear-project"}, Usage: ":project.clear", Kind: KindView, Scopes: projectScopes},

		// --- filtering (view) ---
		Command{ID: Filter,   Name: "filter", Aliases: []string{"f"}, Usage: ":filter <type> <value>", Kind: KindView, Scopes: []Context{ContextCatalog, ContextCitations}},
		Command{ID: FindOpen, Name: "find", Aliases: []string{"/"}, Usage: ":find", Kind: KindView, Scopes: []Context{ContextCatalog, ContextCitations}},

		// --- application-wide (view) ---
		Command{ID: Quit, Name: "quit", Aliases: []string{"exit"}, Usage: ":quit", Kind: KindView},
		Command{ID: Help, Name: "help", Aliases: []string{"?"}, Usage: ":help", Kind: KindView},

		// --- engine reads ---
		Command{ID: PatentList, Kind: KindEngine, Method: proto.MethodPatentList},
		Command{ID: PatentGet, Kind: KindEngine, Method: proto.MethodPatentGet},
		Command{ID: PatentRelations, Kind: KindEngine, Method: proto.MethodRelations},
		Command{ID: ProjectList, Kind: KindEngine, Method: proto.MethodProjectList},
		Command{ID: ExportIDS, Kind: KindEngine, Method: proto.MethodIDSExport, Scopes: []Context{ContextProjects}},

		// --- patent review state (engine) ---
		Command{ID: MarkStored, Name: "review_state.stored", Aliases: []string{"stored"}, Usage: ":review_state.stored", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
		Command{ID: MarkUnderReview, Name: "review_state.under_review", Aliases: []string{"under-review"}, Usage: ":review_state.under_review", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
		Command{ID: MarkIgnored, Name: "review_state.ignored", Aliases: []string{"ignored"}, Usage: ":review_state.ignored", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
		Command{ID: MarkDeleted, Name: "review_state.deleted", Aliases: []string{"deleted"}, Usage: ":review_state.deleted", Kind: KindEngine, Method: proto.MethodReviewState, Scopes: patentScopes},
		Command{ID: AddToProject, Name: "add", Aliases: []string{"add-to-project"}, Usage: ":add [PATENT]", Kind: KindEngine, Method: proto.MethodMembershipAdd, Scopes: patentScopes},
		Command{ID: PatentDelete, Name: "delete", Aliases: []string{"delete-patent"}, Usage: ":delete", Kind: KindEngine, Method: proto.MethodPatentDelete, Scopes: patentScopes},
		Command{ID: TagAdd, Name: "tag", Aliases: []string{"tag-patent"}, Usage: ":tag <name>", Kind: KindEngine, Method: proto.MethodTagAssign, Scopes: patentScopes},
		Command{ID: TagRemove, Name: "untag", Aliases: []string{"untag-patent"}, Usage: ":untag <name>", Kind: KindEngine, Method: proto.MethodTagRemove, Scopes: patentScopes},

		// --- ingestion (engine) ---
		Command{ID: IngestFamily, Name: "ingest.family", Aliases: []string{"family"}, Usage: ":ingest.family", Kind: KindEngine, Method: proto.MethodIngestFamily, Scopes: patentScopes},
		Command{ID: IngestCitations, Name: "ingest.citations", Aliases: []string{"ingest-citations"}, Usage: ":ingest.citations", Kind: KindEngine, Method: proto.MethodIngestFamily, Scopes: patentScopes},
		Command{ID: IngestCitedBy, Name: "ingest.citedby", Aliases: []string{"ingest-citedby"}, Usage: ":ingest.citedby", Kind: KindEngine, Method: proto.MethodIngestFamily, Scopes: patentScopes},
		Command{ID: IngestAll, Name: "ingest.all", Aliases: []string{"ingest", "recursion"}, Usage: ":ingest.all", Kind: KindEngine, Method: proto.MethodIngestFamily, Scopes: patentScopes},
		Command{ID: FetchPatent, Name: "fetch", Aliases: []string{"fetch-patent"}, Usage: ":fetch", Kind: KindEngine, Method: proto.MethodIngestFamily, Scopes: patentScopes},
		Command{ID: Import, Name: "import", Aliases: []string{"import-patent"}, Usage: ":import <number|file> [force]", Kind: KindEngine, Method: proto.MethodIngestFamily},
		Command{ID: IngestCancel, Kind: KindEngine, Method: proto.MethodIngestCancel},

		// --- projects (engine) ---
		Command{ID: ProjectCreate, Name: "project.create", Aliases: []string{"create-project"}, Usage: ":project.create [NAME]", Kind: KindEngine, Method: proto.MethodProjectCreate, Scopes: []Context{ContextProjects}},
	)
}
