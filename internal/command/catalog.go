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
	ProjectCreate      ID = "project.create"
	ProjectActivate    ID = "project.activate"
	ProjectClearActive ID = "project.clear-active"
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
		Command{ID: NavDown, Title: "Down", Help: "Move the cursor down one row.", Kind: KindView, Scopes: listScopes},
		Command{ID: NavUp, Title: "Up", Help: "Move the cursor up one row.", Kind: KindView, Scopes: listScopes},
		Command{ID: NavPageDown, Title: "Page down", Help: "Scroll down one page.", Kind: KindView, Scopes: listScopes},
		Command{ID: NavPageUp, Title: "Page up", Help: "Scroll up one page.", Kind: KindView, Scopes: listScopes},
		Command{ID: NavTop, Title: "Top", Help: "Jump to the first row.", Kind: KindView, Scopes: listScopes},
		Command{ID: NavBottom, Title: "Bottom", Help: "Jump to the last row.", Kind: KindView, Scopes: listScopes},

		// --- panes and overlays (view) ---
		Command{ID: OpenDetail, Name: "open.detail", Aliases: []string{"detail"}, Usage: ":open.detail", Title: "Open detail", Help: "Open the selected patent's detail view.", Kind: KindView, Scopes: []Context{ContextCatalog, ContextCitations}},
		Command{ID: OpenCitations, Name: "open.citations", Aliases: []string{"citations"}, Usage: ":open.citations", Title: "Open citations", Help: "Show patents the selected patent cites.", Kind: KindView, Scopes: []Context{ContextCatalog, ContextDetail}},
		Command{ID: OpenCitedBy, Name: "open.citedby", Aliases: []string{"citedby"}, Usage: ":open.citedby", Title: "Open cited by", Help: "Show patents that cite the selected patent.", Kind: KindView, Scopes: []Context{ContextCatalog, ContextDetail}},
		Command{ID: OpenProjects, Name: "open.projects", Aliases: []string{"projects"}, Usage: ":open.projects", Title: "Open projects", Help: "Open the project list.", Kind: KindView, Scopes: []Context{ContextCatalog, ContextDetail, ContextCitations}},
		Command{ID: Back, Title: "Back", Help: "Return to the previous pane.", Kind: KindView},
		Command{ID: CloseOverlay, Title: "Close", Help: "Close the focused overlay.", Kind: KindView, Scopes: []Context{ContextOverlay}},
		Command{ID: Refresh, Name: "refresh", Aliases: []string{"reload"}, Usage: ":refresh", Title: "Refresh", Help: "Reload the current pane from the daemon.", Kind: KindView, Scopes: []Context{ContextCatalog, ContextDetail, ContextCitations, ContextProjects}},
		Command{ID: OpenSearch, Title: "Command palette", Help: "Open the command palette.", Kind: KindView, Scopes: []Context{ContextCatalog, ContextDetail, ContextCitations, ContextProjects}},
		Command{ID: OpenCommand, Title: "Command prompt", Help: "Open the command prompt.", Kind: KindView},
		Command{ID: ProjectActivate, Name: "project.use", Aliases: []string{"use-project"}, Usage: ":project.use [PROJECT]", Title: "Use project", Help: "Make the selected project the active project for patent actions.", Kind: KindView, Scopes: projectScopes},
		Command{ID: ProjectClearActive, Name: "project.clear", Aliases: []string{"clear-project"}, Usage: ":project.clear", Title: "Clear active project", Help: "Clear the active project filter and target.", Kind: KindView, Scopes: projectScopes},

		// --- application-wide (view) ---
		Command{ID: Quit, Name: "quit", Aliases: []string{"exit"}, Usage: ":quit", Title: "Quit", Help: "Quit the application.", Kind: KindView},
		Command{ID: Help, Name: "help", Aliases: []string{"?"}, Usage: ":help", Title: "Help", Help: "Show the help screen.", Kind: KindView},

		// --- engine reads ---
		Command{ID: PatentList, Title: "List patents", Help: "List stored patents.", Kind: KindEngine, Method: proto.MethodPatentList},
		Command{ID: PatentGet, Title: "Get patent", Help: "Fetch one patent's record.", Kind: KindEngine, Method: proto.MethodPatentGet},
		Command{ID: PatentRelations, Title: "Patent relations", Help: "List a patent's family-graph edges.", Kind: KindEngine, Method: proto.MethodRelations},
		Command{ID: ProjectList, Title: "List projects", Help: "List all projects.", Kind: KindEngine, Method: proto.MethodProjectList},
		Command{ID: ExportIDS, Title: "Export IDS", Help: "Build the Information Disclosure Statement for the selected project.", Kind: KindEngine, Method: proto.MethodIDSExport, Scopes: []Context{ContextProjects}},

		// --- patent membership state (engine) ---
		Command{ID: MarkStored, Name: "state.stored", Aliases: []string{"stored"}, Usage: ":state.stored", Title: "Mark stored", Help: "Set the selected patent to the stored state.", Kind: KindEngine, Method: proto.MethodMembershipState, Scopes: patentScopes},
		Command{ID: MarkUnderReview, Name: "state.under_review", Aliases: []string{"under-review"}, Usage: ":state.under_review", Title: "Mark under review", Help: "Set the selected patent to under review.", Kind: KindEngine, Method: proto.MethodMembershipState, Scopes: patentScopes},
		Command{ID: MarkIgnored, Name: "state.ignored", Aliases: []string{"ignored"}, Usage: ":state.ignored", Title: "Mark ignored", Help: "Set the selected patent to ignored.", Kind: KindEngine, Method: proto.MethodMembershipState, Scopes: patentScopes},
		Command{ID: MarkDeleted, Name: "state.deleted", Aliases: []string{"deleted"}, Usage: ":state.deleted", Title: "Mark deleted", Help: "Soft-delete the selected patent from the project.", Kind: KindEngine, Method: proto.MethodMembershipState, Scopes: patentScopes},
		Command{ID: AddToProject, Name: "add", Aliases: []string{"add-to-project"}, Usage: ":add [PATENT]", Title: "Add to project", Help: "Add the selected patent to the active project.", Kind: KindEngine, Method: proto.MethodMembershipAdd, Scopes: patentScopes},

		// --- ingestion (engine) ---
		Command{ID: IngestFamily, Name: "ingest.family", Aliases: []string{"ingest"}, Usage: ":ingest.family", Title: "Ingest family", Help: "Crawl the selected patent's family graph.", Kind: KindEngine, Method: proto.MethodIngestFamily, Scopes: patentScopes},
		Command{ID: IngestCancel, Title: "Cancel ingest", Help: "Cancel a running ingest job.", Kind: KindEngine, Method: proto.MethodIngestCancel},

		// --- projects (engine) ---
		Command{ID: ProjectCreate, Name: "project.create", Aliases: []string{"create-project"}, Usage: ":project.create", Title: "Create project", Help: "Create a new project.", Kind: KindEngine, Method: proto.MethodProjectCreate, Scopes: []Context{ContextProjects}},
	)
}
