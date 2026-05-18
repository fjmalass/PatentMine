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

	// Patent membership-state changes. All four map to one proto method; the
	// target state is the difference the dispatcher supplies.
	MarkStored      ID = "patent.mark-stored"
	MarkUnderReview ID = "patent.mark-under-review"
	MarkIgnored     ID = "patent.mark-ignored"
	MarkDeleted     ID = "patent.mark-deleted"
	AddToProject    ID = "patent.add-to-project"

	// Ingestion.
	IngestFamily ID = "ingest.family"
	IngestCancel ID = "ingest.cancel"

	// Projects.
	ProjectCreate ID = "project.create"
)

// listScopes are the contexts that behave as scrollable lists.
var listScopes = []Context{ContextCatalog, ContextCitations, ContextProjects, ContextDetail}

// patentScopes are the contexts where a patent is selected and can be acted on.
var patentScopes = []Context{ContextCatalog, ContextDetail, ContextCitations}

// Default returns the registry of every command the system supports. It is the
// single source of truth: build it once at startup and inject it.
func Default() (*Registry, error) {
	return NewRegistry(
		// --- navigation (view) ---
		Command{ID: NavDown, Title: "Down", Help: "Move the cursor down one row.", Kind: KindView, Scopes: listScopes},
		Command{ID: NavUp, Title: "Up", Help: "Move the cursor up one row.", Kind: KindView, Scopes: listScopes},
		Command{ID: NavPageDown, Title: "Page down", Help: "Scroll down one page.", Kind: KindView, Scopes: listScopes},
		Command{ID: NavPageUp, Title: "Page up", Help: "Scroll up one page.", Kind: KindView, Scopes: listScopes},
		Command{ID: NavTop, Title: "Top", Help: "Jump to the first row.", Kind: KindView, Scopes: listScopes},
		Command{ID: NavBottom, Title: "Bottom", Help: "Jump to the last row.", Kind: KindView, Scopes: listScopes},

		// --- panes and overlays (view) ---
		Command{ID: OpenDetail, Title: "Open detail", Help: "Open the selected patent's detail view.", Kind: KindView, Scopes: []Context{ContextCatalog, ContextCitations}},
		Command{ID: OpenCitations, Title: "Citations", Help: "Show patents the selected patent cites.", Kind: KindView, Scopes: []Context{ContextCatalog, ContextDetail}},
		Command{ID: OpenCitedBy, Title: "Cited by", Help: "Show patents that cite the selected patent.", Kind: KindView, Scopes: []Context{ContextCatalog, ContextDetail}},
		Command{ID: OpenProjects, Title: "Projects", Help: "Open the project list.", Kind: KindView, Scopes: []Context{ContextCatalog, ContextDetail}},
		Command{ID: Back, Title: "Back", Help: "Return to the previous pane.", Kind: KindView},
		Command{ID: CloseOverlay, Title: "Close", Help: "Close the focused overlay.", Kind: KindView, Scopes: []Context{ContextOverlay}},
		Command{ID: Refresh, Title: "Refresh", Help: "Reload the current pane from the daemon.", Kind: KindView, Scopes: []Context{ContextCatalog, ContextCitations, ContextProjects}},
		Command{ID: OpenSearch, Title: "Search", Help: "Open the search prompt.", Kind: KindView, Scopes: []Context{ContextCatalog, ContextCitations, ContextProjects}},

		// --- application-wide (view) ---
		Command{ID: Quit, Title: "Quit", Help: "Quit the application.", Kind: KindView},
		Command{ID: Help, Title: "Help", Help: "Show the help screen.", Kind: KindView},

		// --- engine reads ---
		Command{ID: PatentList, Title: "List patents", Help: "List stored patents.", Kind: KindEngine, Method: proto.MethodPatentList},
		Command{ID: PatentGet, Title: "Get patent", Help: "Fetch one patent's record.", Kind: KindEngine, Method: proto.MethodPatentGet},
		Command{ID: PatentRelations, Title: "Patent relations", Help: "List a patent's family-graph edges.", Kind: KindEngine, Method: proto.MethodRelations},
		Command{ID: ProjectList, Title: "List projects", Help: "List all projects.", Kind: KindEngine, Method: proto.MethodProjectList},
		Command{ID: ExportIDS, Title: "Export IDS", Help: "Build the Information Disclosure Statement for the selected project.", Kind: KindEngine, Method: proto.MethodIDSExport, Scopes: []Context{ContextProjects}},

		// --- patent membership state (engine) ---
		Command{ID: MarkStored, Title: "Mark stored", Help: "Set the selected patent to the stored state.", Kind: KindEngine, Method: proto.MethodMembershipState, Scopes: patentScopes},
		Command{ID: MarkUnderReview, Title: "Mark under review", Help: "Set the selected patent to under review.", Kind: KindEngine, Method: proto.MethodMembershipState, Scopes: patentScopes},
		Command{ID: MarkIgnored, Title: "Mark ignored", Help: "Set the selected patent to ignored.", Kind: KindEngine, Method: proto.MethodMembershipState, Scopes: patentScopes},
		Command{ID: MarkDeleted, Title: "Mark deleted", Help: "Soft-delete the selected patent from the project.", Kind: KindEngine, Method: proto.MethodMembershipState, Scopes: patentScopes},
		Command{ID: AddToProject, Title: "Add to project", Help: "Add the selected patent to the active project.", Kind: KindEngine, Method: proto.MethodMembershipAdd, Scopes: patentScopes},

		// --- ingestion (engine) ---
		Command{ID: IngestFamily, Title: "Ingest family", Help: "Crawl the selected patent's family graph.", Kind: KindEngine, Method: proto.MethodIngestFamily, Scopes: patentScopes},
		Command{ID: IngestCancel, Title: "Cancel ingest", Help: "Cancel a running ingest job.", Kind: KindEngine, Method: proto.MethodIngestCancel},

		// --- projects (engine) ---
		Command{ID: ProjectCreate, Title: "New project", Help: "Create a new project.", Kind: KindEngine, Method: proto.MethodProjectCreate, Scopes: []Context{ContextProjects}},
	)
}
