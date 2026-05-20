package text

import "maps"

// Named keys for strings that are not command titles. Command titles and help
// lines are keyed by ID through CmdTitle/CmdHelp and need no constant here.
const (
	// Status-line messages.
	StatusWelcome              Key = "status.welcome"
	StatusActiveProject        Key = "status.active_project"
	StatusActiveProjectSaveErr Key = "status.active_project_save_err"
	StatusClearedProject       Key = "status.cleared_project"
	StatusProjectNotFound      Key = "status.project_not_found"
	StatusNoPatentSelected     Key = "status.no_patent_selected"
	StatusNoActiveProject      Key = "status.no_active_project"
	StatusDaemonUnavailable    Key = "status.daemon_unavailable"
	StatusNoProjectSelection   Key = "status.no_project_selection"
	StatusNoProjectSelected    Key = "status.no_project_selected"
	StatusUnknownCommand       Key = "status.unknown_command"
	StatusCommandNotHere       Key = "status.command_not_here"
	StatusUsage                Key = "status.usage"
	StatusUnhandledCommand     Key = "status.unhandled_command"
	StatusInvalidPatentNumber  Key = "status.invalid_patent_number"
	StatusDaemonClosed         Key = "status.daemon_closed"
	StatusIngestProgress       Key = "status.ingest_progress"
	StatusIngestFailed         Key = "status.ingest_failed"
	StatusIngestComplete       Key = "status.ingest_complete"
	StatusIngestStarted        Key = "status.ingest_started"
	StatusIngestStartFailed    Key = "status.ingest_start_failed"
	StatusAddFailed            Key = "status.add_failed"
	StatusAdded                Key = "status.added"
	StatusAddedNoIngest        Key = "status.added_no_ingest"
	StatusSetStateFailed       Key = "status.set_state_failed"
	StatusSetState             Key = "status.set_state"
	StatusExportFailed         Key = "status.export_failed"
	StatusExportDone           Key = "status.export_done"
	StatusProjectCreateFailed  Key = "status.project_create_failed"
	StatusProjectCreated       Key = "status.project_created"
	StatusProjectNameEmpty     Key = "status.project_name_empty"
	StatusImportFailed         Key = "status.import_failed"
	StatusImported             Key = "status.imported"
	StatusTagFailed            Key = "status.tag_failed"
	StatusTagged               Key = "status.tagged"
	StatusUntagFailed          Key = "status.untag_failed"
	StatusUntagged             Key = "status.untagged"
	StatusDeleteFailed         Key = "status.delete_failed"
	StatusDeleted              Key = "status.deleted"
	StatusBatchDeleted         Key = "status.batch_deleted"
	StatusBatchSetState        Key = "status.batch_set_state"
	StatusBatchAdded           Key = "status.batch_added"
	StatusFilter               Key = "status.filter"
	StatusBrowserOpenFailed    Key = "status.browser_open_failed"
	StatusBrowserOpened        Key = "status.browser_opened"
	StatusTagTaxonomyAddFailed    Key = "status.tag_taxonomy_add_failed"
	StatusTagTaxonomyAdded        Key = "status.tag_taxonomy_added"
	StatusTagTaxonomyDeleteFailed Key = "status.tag_taxonomy_delete_failed"
	StatusTagTaxonomyDeleted      Key = "status.tag_taxonomy_deleted"
	StatusTagTaxonomyListFailed   Key = "status.tag_taxonomy_list_failed"
	StatusTagPatentAddFailed      Key = "status.tag_patent_add_failed"
	StatusTagPatentAdded          Key = "status.tag_patent_added"
	StatusTagPatentDeleteFailed   Key = "status.tag_patent_delete_failed"
	StatusTagPatentDeleted        Key = "status.tag_patent_deleted"
	StatusTagPatentListFailed     Key = "status.tag_patent_list_failed"

	// Header / footer navigation hints.
	HintCommands        Key = "hint.commands"
	HintCommand         Key = "hint.command"
	HintDetail          Key = "hint.detail"
	HintCitations       Key = "hint.citations"
	HintCitedBy         Key = "hint.cited_by"
	HintIDS             Key = "hint.ids"
	HintProjects        Key = "hint.projects"
	HintProjectActions  Key = "hint.project_actions"
	HintBack            Key = "hint.back"
	HintNewProject      Key = "hint.new_project"
	HintSelectProject   Key = "hint.select_project"
	HintSelect          Key = "hint.select"
	HintClearActive     Key = "hint.clear_active"
	HintExportIDS       Key = "hint.export_ids"
	HintIngest          Key = "hint.ingest"
	HintFetch           Key = "hint.fetch"
	HintBrowse          Key = "hint.browse"
	HintJump            Key = "hint.jump"
	HintHelp            Key = "hint.help"
	HintQuit            Key = "hint.quit"
	HintMove            Key = "hint.move"
	HintSlashCommands   Key = "hint.slash_commands"
	SplashCreateHint    Key = "splash.create_hint"
	SplashCreateKeyHint Key = "splash.create_key_hint"

	// Overlays.
	OverlayHelpTitle       Key = "overlay.help.title"
	OverlayCommandsTitle   Key = "overlay.commands.title"
	OverlayCommandTitle    Key = "overlay.command.title"
	PromptFilterHint       Key = "overlay.prompt.filter_hint"
	PromptDirectHint       Key = "overlay.prompt.direct_hint"
	PromptNoMatch          Key = "overlay.prompt.no_match"
	PromptRunHint          Key = "overlay.prompt.run_hint"
	HelpSectionGlobal      Key = "help.section.global"
	HelpSectionCatalog     Key = "help.section.catalog"
	HelpSectionDetail      Key = "help.section.detail"
	HelpSectionCitations   Key = "help.section.citations"
	HelpSectionIDS         Key = "help.section.ids"
	HelpSectionProjects    Key = "help.section.projects"
	HelpSectionOverlay     Key = "help.section.overlay"
	NewProjectTitle        Key = "overlay.new_project.title"
	NewProjectCaption      Key = "overlay.new_project.caption"
	EditIDSKindTitle       Key = "overlay.ids.kind.title"
	EditIDSKindCaption     Key = "overlay.ids.kind.caption"
	EditIDSCountryTitle    Key = "overlay.ids.country.title"
	EditIDSCountryCaption  Key = "overlay.ids.country.caption"
	EditIDSPassagesTitle   Key = "overlay.ids.passages.title"
	EditIDSPassagesCaption Key = "overlay.ids.passages.caption"
	EditIDSNotesTitle      Key = "overlay.ids.notes.title"
	EditIDSNotesCaption    Key = "overlay.ids.notes.caption"
	TextInputHint          Key = "overlay.text_input.hint"
)

// cmdStrings is the title/help text for every command, keyed by command ID.
// English() expands it into CmdTitle/CmdHelp catalog entries.
var cmdStrings = map[string][2]string{
	"nav.down":                 {"Down", "Move the cursor down one row."},
	"nav.up":                   {"Up", "Move the cursor up one row."},
	"nav.page-down":            {"Page down", "Scroll down one page."},
	"nav.page-up":              {"Page up", "Scroll up one page."},
	"nav.top":                  {"Top", "Jump to the first row."},
	"nav.bottom":               {"Bottom", "Jump to the last row."},
	"view.detail":              {"Open detail", "Open the selected patent's detail view."},
	"view.browser":             {"Open browser", "Open the selected patent's page in the browser, or a typed patent number when given."},
	"view.citations":           {"Open citations", "Show patents the selected patent cites."},
	"view.cited-by":            {"Open cited by", "Show patents that cite the selected patent."},
	"view.ids":                 {"Open IDS", "Open the selected patent's IDS entry editor."},
	"view.projects":            {"Open projects", "Open the project list."},
	"view.back":                {"Back", "Return to the previous pane."},
	"view.close-overlay":       {"Close", "Close the focused overlay."},
	"view.refresh":             {"Refresh", "Reload the current pane from the daemon."},
	"search.open":              {"Command palette", "Open the command palette."},
	"command.open":             {"Command prompt", "Open the command prompt."},
	"view.jump":                {"Jump to field", "Jump straight to a labelled field in the detail view."},
	"app.quit":                 {"Quit", "Quit the application."},
	"app.help":                 {"Help", "Show the help screen."},
	"patent.list":              {"List patents", "List stored patents."},
	"patent.get":               {"Get patent", "Fetch one patent's record."},
	"patent.relations":         {"Patent relations", "List a patent's family-graph edges."},
	"project.list":             {"List projects", "List all projects."},
	"ids.export":               {"Export IDS", "Build the Information Disclosure Statement for the selected project."},
	"patent.mark-stored":       {"Mark stored", "Set the selected patent to the stored review state."},
	"patent.mark-under-review": {"Mark under review", "Set the selected patent to under review."},
	"patent.mark-ignored":      {"Mark ignored", "Set the selected patent to ignored."},
	"patent.mark-deleted":      {"Mark deleted", "Soft-delete the selected patent from the project."},
	"patent.delete":            {"Delete patent", "Permanently remove the selected patent from the database."},
	"select.visual":            {"Visual select", "Toggle visual mode at the cursor to begin range selection."},
	"select.clear":             {"Clear selection", "Exit visual mode and clear the selection range."},
	"col.next":                 {"Next column", "Move the visual focus to the next column."},
	"col.prev":                 {"Prev column", "Move the visual focus to the previous column."},
	"col.sort-apply":           {"Apply sort", "Apply sorting to the currently focused column."},
	"patent.add-to-project":    {"Add to project", "Add the selected patent to the active project."},
	"patent.tag":               {"Tag patent", "Tag the selected patent within the active project; an unknown name creates the tag."},
	"patent.untag":             {"Untag patent", "Remove a tag from the selected patent within the active project."},
	"ingest.family":            {"Ingest family", "Crawl the selected patent's family graph (parents and children)."},
	"ingest.citations":         {"Ingest citations", "Crawl patents the selected patent cites."},
	"ingest.citedby":           {"Ingest cited-by", "Crawl patents that cite the selected patent."},
	"ingest.all":               {"Ingest all", "Crawl the full family graph including citations and cited-by."},
	"patent.fetch":             {"Fetch patent", "Fetch the selected patent's record from the web."},
	"patent.import":            {"Import patent", "Fetch a patent by number (add 'force' to bypass the cache) or load a fixture file by path."},
	"ingest.cancel":            {"Cancel ingest", "Cancel a running ingest job."},
	"project.create":           {"Create project", "Create a new project."},
	"project.activate":         {"Use project", "Make the selected project the active project for patent actions."},
	"project.clear-active":     {"Clear active project", "Clear the active project filter and target."},
	"view.filter":              {"Filter", "Apply a filter to the current list (e.g. :filter state cached)."},
	"find.open":                {"Find", "Open the inline find bar; type to search, n/N to navigate, Enter to keep, Esc to cancel."},
	"ids.edit-field":           {"Edit IDS field", "Edit the selected IDS field."},
	"ids.toggle-full":          {"Toggle IDS full", "Toggle whether the full document is cited on the IDS."},
	"ids.cycle-status":         {"Cycle IDS status", "Cycle the IDS entry status through pending, submitted, and accepted."},
	"ids.delete":               {"Delete IDS entry", "Remove the current patent from the curated IDS."},
	"tag.add":                  {"Add taxonomy tag", "Register a new tag in the project's taxonomy."},
	"tag.list":                 {"List taxonomy tags", "List all tags in the project's taxonomy."},
	"tag.delete":               {"Delete taxonomy tag", "Remove a tag from the project's taxonomy."},
	"tag.patent.add":           {"Assign patent tag", "Assign a taxonomy tag to the selected patent."},
	"tag.patent.delete":        {"Remove patent tag", "Remove a tag assignment from the selected patent."},
	"tag.patent.list":          {"List patent tags", "List tags assigned to the selected patent."},
	"tag.patent":               {"Manage patent tags", "Open the interactive tag selector popup for the selected patent(s)."},
}

// englishNamed is the English text for every named key.
var englishNamed = map[Key]string{
	StatusWelcome:              "select a project to begin — press ? for help",
	StatusActiveProject:        "active project: %s",
	StatusActiveProjectSaveErr: "active project: %s (save failed: %s)",
	StatusClearedProject:       "cleared active project",
	StatusProjectNotFound:      "project not found: %s",
	StatusNoPatentSelected:     "no patent selected",
	StatusNoActiveProject:      "select an active project first",
	StatusDaemonUnavailable:    "daemon connection unavailable",
	StatusNoProjectSelection:   "focused pane has no project selection",
	StatusNoProjectSelected:    "no project selected",
	StatusUnknownCommand:       "unknown command: %s",
	StatusCommandNotHere:       "%s is not available here",
	StatusUsage:                "usage: %s",
	StatusUnhandledCommand:     "unhandled command: %s",
	StatusInvalidPatentNumber:  "invalid patent number: %s",
	StatusDaemonClosed:         "daemon connection closed",
	StatusIngestProgress:       "ingest %s — ingested %d, discovered %d: %s",
	StatusIngestFailed:         "ingest %s failed: %s",
	StatusIngestComplete:       "ingest %s complete",
	StatusIngestStarted:        "ingest started for %s (%s)",
	StatusIngestStartFailed:    "ingest failed: %s",
	StatusAddFailed:            "add to project failed: %s",
	StatusAdded:                "added %s to %s",
	StatusAddedNoIngest:        "added %s — press F to fetch",
	StatusSetStateFailed:       "set state failed: %s",
	StatusSetState:             "set %s to %s review state in %s",
	StatusExportFailed:         "IDS export failed: %s",
	StatusExportDone:           "IDS for %q: %d disclosed reference(s)",
	StatusProjectCreateFailed:  "create project failed: %s",
	StatusProjectCreated:       "created project %s",
	StatusProjectNameEmpty:     "project name cannot be empty",
	StatusImportFailed:         "import failed: %s",
	StatusImported:             "imported %s",
	StatusTagFailed:            "tag failed: %s",
	StatusTagged:               "tagged %s as %q in %s",
	StatusUntagFailed:          "untag failed: %s",
	StatusUntagged:             "removed tag %q from %s",
	StatusDeleteFailed:         "delete failed: %s",
	StatusDeleted:              "deleted %s",
	StatusBatchDeleted:         "deleted %d patents",
	StatusBatchSetState:        "set %d patents to %s in %s",
	StatusBatchAdded:           "added %d patents to %s",
	StatusFilter:               "%s",
	StatusBrowserOpenFailed:    "open browser failed: %s",
	StatusBrowserOpened:        "opened %d patent page(s) in browser",
	StatusTagTaxonomyAddFailed:    "add tag to taxonomy failed: %s",
	StatusTagTaxonomyAdded:        "added taxonomy tag %q to %s",
	StatusTagTaxonomyDeleteFailed: "delete tag from taxonomy failed: %s",
	StatusTagTaxonomyDeleted:      "deleted taxonomy tag %q from %s",
	StatusTagTaxonomyListFailed:   "list taxonomy tags failed: %s",
	StatusTagPatentAddFailed:      "assign tag to patent failed: %s",
	StatusTagPatentAdded:          "assigned tag %q to patent %s",
	StatusTagPatentDeleteFailed:   "remove tag from patent failed: %s",
	StatusTagPatentDeleted:        "removed tag %q from patent %s",
	StatusTagPatentListFailed:     "list patent tags failed: %s",

	HintCommands:        "commands",
	HintCommand:         "command",
	HintDetail:          "detail",
	HintCitations:       "citations",
	HintCitedBy:         "cited by",
	HintIDS:             "IDS",
	HintProjects:        "projects",
	HintProjectActions:  "project actions",
	HintBack:            "back",
	HintNewProject:      "new project",
	HintSelectProject:   "select project",
	HintSelect:          "select",
	HintClearActive:     "clear active",
	HintExportIDS:       "export IDS",
	HintIngest:          "ingest family",
	HintFetch:           "fetch patent",
	HintBrowse:          "open browser",
	HintJump:            "jump",
	HintHelp:            "help",
	HintQuit:            "quit",
	HintMove:            "move",
	HintSlashCommands:   "/ commands",
	SplashCreateHint:    "Create one with %s.",
	SplashCreateKeyHint: "Create one with %s or %s.",

	OverlayHelpTitle:       "Help — key bindings",
	OverlayCommandsTitle:   "Commands",
	OverlayCommandTitle:    "Command",
	PromptFilterHint:       "Filter commands",
	PromptDirectHint:       "Type a dot command",
	PromptNoMatch:          "no matching commands",
	PromptRunHint:          "enter runs the selected command · esc closes",
	HelpSectionGlobal:      "Global",
	HelpSectionCatalog:     "Catalog",
	HelpSectionDetail:      "Detail",
	HelpSectionCitations:   "Citations",
	HelpSectionIDS:         "IDS",
	HelpSectionProjects:    "Projects",
	HelpSectionOverlay:     "Overlay",
	NewProjectTitle:        "New project",
	NewProjectCaption:      "Enter a name for the new project.",
	EditIDSKindTitle:       "IDS kind code",
	EditIDSKindCaption:     "Enter the patent kind code, such as B2 or A1.",
	EditIDSCountryTitle:    "IDS country code",
	EditIDSCountryCaption:  "Enter the patent country code, such as US, EP, or WO.",
	EditIDSPassagesTitle:   "IDS relevant passages",
	EditIDSPassagesCaption: "Enter the cited passages, or leave blank when citing the full document.",
	EditIDSNotesTitle:      "IDS notes",
	EditIDSNotesCaption:    "Enter any IDS note for this patent.",
	TextInputHint:          "enter confirms · esc cancels",
}

// English returns the shipped en catalog: every command's title and help plus
// every named string.
func English() *Catalog {
	entries := make(map[Key]string, len(englishNamed)+2*len(cmdStrings))
	maps.Copy(entries, englishNamed)
	for id, pair := range cmdStrings {
		entries[CmdTitle(id)] = pair[0]
		entries[CmdHelp(id)] = pair[1]
	}
	return New("en", entries)
}
