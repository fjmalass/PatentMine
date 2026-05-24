package text

import "maps"

// Named keys for strings that are not command titles. Command titles and help
// lines are keyed by ID through CmdTitle/CmdHelp and need no constant here.
const (
	// Status-line messages.
	StatusWelcome                 Key = "status.welcome"
	StatusActiveProject           Key = "status.active_project"
	StatusActiveProjectSaveErr    Key = "status.active_project_save_err"
	StatusClearedProject          Key = "status.cleared_project"
	StatusProjectNotFound         Key = "status.project_not_found"
	StatusNoPatentSelected        Key = "status.no_patent_selected"
	StatusNoActiveProject         Key = "status.no_active_project"
	StatusDaemonUnavailable       Key = "status.daemon_unavailable"
	StatusNoProjectSelection      Key = "status.no_project_selection"
	StatusNoProjectSelected       Key = "status.no_project_selected"
	StatusUnknownCommand          Key = "status.unknown_command"
	StatusCommandNotHere          Key = "status.command_not_here"
	StatusUsage                   Key = "status.usage"
	StatusUnhandledCommand        Key = "status.unhandled_command"
	StatusInvalidPatentNumber     Key = "status.invalid_patent_number"
	StatusDaemonClosed            Key = "status.daemon_closed"
	StatusAIAnalysisStarted       Key = "status.ai_analysis_started"
	StatusAIAnalysisFailed        Key = "status.ai_analysis_failed"
	StatusAIAnalysisComplete      Key = "status.ai_analysis_complete"
	StatusCrawlProgress           Key = "status.ingest_progress"
	StatusCrawlFailed             Key = "status.ingest_failed"
	StatusCrawlComplete           Key = "status.ingest_complete"
	StatusCrawlStarted            Key = "status.ingest_started"
	StatusCrawlStartFailed        Key = "status.ingest_start_failed"
	StatusCrawlDepthMax           Key = "status.ingest_depth_max"
	StatusAddFailed               Key = "status.add_failed"
	StatusAdded                   Key = "status.added"
	StatusAddedNoCrawl            Key = "status.added_no_ingest"
	StatusSetStateFailed          Key = "status.set_state_failed"
	StatusSetState                Key = "status.set_state"
	StatusNotesAdded              Key = "status.notes_added"
	StatusNotesFlushed            Key = "status.notes_flushed"
	StatusNotesSavedToPatentNote  Key = "status.notes_saved_to_patent_note"
	StatusCopiedToClipboard       Key = "status.copied_to_clipboard"
	StatusClipboardFailed         Key = "status.clipboard_failed"
	StatusExportFailed            Key = "status.export_failed"
	StatusExportDone              Key = "status.export_done"
	StatusNotesExportDone         Key = "status.notes_export_done"
	StatusNotesExportFailed       Key = "status.notes_export_failed"
	StatusNotesLoadFailed         Key = "status.notes_load_failed"
	StatusNotesSorted             Key = "status.notes_sorted"
	StatusProjectCreateFailed     Key = "status.project_create_failed"
	StatusProjectCreated          Key = "status.project_created"
	StatusProjectNameEmpty        Key = "status.project_name_empty"
	StatusImportFailed            Key = "status.import_failed"
	StatusImported                Key = "status.imported"
	StatusTagFailed               Key = "status.tag_failed"
	StatusTagged                  Key = "status.tagged"
	StatusUntagFailed             Key = "status.untag_failed"
	StatusUntagged                Key = "status.untagged"
	StatusDeleteFailed            Key = "status.delete_failed"
	StatusDeleted                 Key = "status.deleted"
	StatusBatchDeleted            Key = "status.batch_deleted"
	StatusBatchSetState           Key = "status.batch_set_state"
	StatusBatchAdded              Key = "status.batch_added"
	StatusFilter                  Key = "status.filter"
	StatusBrowserOpenFailed       Key = "status.browser_open_failed"
	StatusBrowserOpened           Key = "status.browser_opened"
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

	// History navigation.
	StatusHistoryEmpty             Key = "status.history_empty"
	StatusHistoryAtEnd             Key = "status.history_at_end"
	StatusHistoryPatentUnavailable Key = "status.history_patent_unavailable"

	// Classification definitions.
	StatusClassificationLookupFailed  Key = "status.classification_lookup_failed"
	StatusClassificationLookupSuccess Key = "status.classification_lookup_success"
	StatusClassificationListFailed    Key = "status.classification_list_failed"
	StatusClassificationDeleteFailed  Key = "status.classification_delete_failed"
	StatusClassificationDeleted       Key = "status.classification_deleted"

	// Header / footer navigation hints.
	HintCommands        Key = "hint.commands"
	HintCommand         Key = "hint.command"
	HintDetail          Key = "hint.detail"
	HintCitations       Key = "hint.citations"
	HintCitedBy         Key = "hint.cited_by"
	HintParents         Key = "hint.parents"
	HintChildren        Key = "hint.children"
	HintFamily          Key = "hint.family"
	HintIDS             Key = "hint.ids"
	HintProjects        Key = "hint.projects"
	HintProjectActions  Key = "hint.project_actions"
	HintBack            Key = "hint.back"
	HintNewProject      Key = "hint.new_project"
	HintSelectProject   Key = "hint.select_project"
	HintSelect          Key = "hint.select"
	HintClearActive     Key = "hint.clear_active"
	HintExportIDS       Key = "hint.export_ids"
	HintCrawl           Key = "hint.ingest"
	HintLookup          Key = "hint.fetch"
	HintClassifications Key = "hint.classifications"
	HintBrowse          Key = "hint.browse"
	HintFullText        Key = "hint.fulltext"
	HintJump            Key = "hint.jump"
	HintHelp            Key = "hint.help"
	HintQuit            Key = "hint.quit"
	HintMove            Key = "hint.move"
	HintSlashCommands   Key = "hint.slash_commands"
	HintHistory         Key = "hint.history"
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
	HelpSectionFamily      Key = "help.section.family"
	HelpSectionIDS         Key = "help.section.ids"
	HelpSectionProjects    Key = "help.section.projects"
	HelpSectionOverlay     Key = "help.section.overlay"
	HelpSectionAvailable   Key = "help.section.available"
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
	EditPatentNoteTitle    Key = "overlay.patent_note.title"
	EditPatentNoteCaption  Key = "overlay.patent_note.caption"
	TextInputHint          Key = "overlay.text_input.hint"
)

// cmdStrings is the title/help text for every command, keyed by command ID.
// English() expands it into CmdTitle/CmdHelp catalog entries.
var cmdStrings = map[string][2]string{
	"nav.down":                   {"Down", "Move the cursor down one row."},
	"nav.up":                     {"Up", "Move the cursor up one row."},
	"nav.page-down":              {"Page down", "Scroll down one page."},
	"nav.page-up":                {"Page up", "Scroll up one page."},
	"nav.top":                    {"Top", "Jump to the first row."},
	"nav.bottom":                 {"Bottom", "Jump to the last row."},
	"nav.reselect-last":          {"Reselect last", "Jump back to the last active patent."},
	"view.detail":                {"Open detail", "Open the selected patent's detail view."},
	"view.ai-menu":               {"AI Curation Menu", "Open the AI curation and analysis popup menu."},
	"view.settings-ai":           {"AI & Search Settings", "Open the AI engines and search crawls configuration settings."},
	"view.assignees":             {"Open assignee stats", "Open the statistics popup for all assignees, preselecting the current patent's assignee when available."},
	"view.classification-stats":  {"Open classification stats", "Open the statistics popup for all classifications, preselecting the current patent's codes when available."},
	"view.inventors":             {"Open inventors stats", "Open the statistics popup for the patent's inventors when cursor is on the inventors line."},
	"view.inventors.direct":      {"Open inventors stats direct", "Open the statistics popup for the patent's inventors directly."},
	"view.browser":               {"Open browser", "Open the selected patent's page in the browser, or a typed patent number when given."},
	"view.metrics":               {"Metrics", "Open the daemon metrics dashboard overlay."},
	"view.activity":              {"Activity replay", "Open the activity journal for review and replay."},
	"view.patent-note":           {"Open patent note", "Open the project-scoped markdown note editor for the selected patent."},
	"view.citations":             {"Open citations", "Show patents the selected patent cites."},
	"view.cited-by":              {"Open cited by", "Show patents that cite the selected patent."},
	"view.parents":               {"Open parents", "Show parent patents of the selected patent."},
	"view.children":              {"Open children", "Show child patents of the selected patent."},
	"view.family":                {"Open family graph", "Show a bounded BFS parent/child family graph for the selected patent."},
	"view.family-depth-more":     {"Increase family depth", "Increase the family graph BFS depth and reload the graph."},
	"view.family-depth-less":     {"Decrease family depth", "Decrease the family graph BFS depth and reload the graph."},
	"view.family-export-mermaid": {"Copy Mermaid graph", "Copy a simplified Mermaid diagram for the current family graph to the clipboard."},
	"crawl.depth.max":            {"Show crawl depth max", "Show the daemon's default maximum family-crawl depth."},
	"view.ids":                   {"Open IDS", "Open the selected patent's IDS entry editor."},
	"view.projects":              {"Open projects", "Open the project list."},
	"view.back":                  {"Back", "Return to the previous pane."},
	"view.close-overlay":         {"Close", "Close the focused overlay."},
	"view.refresh":               {"Refresh", "Reload the current pane from the daemon."},
	"search.open":                {"Command palette", "Open the command palette."},
	"command.open":               {"Command prompt", "Open the command prompt."},
	"view.jump":                  {"Jump to field", "Jump straight to a labelled field in the detail view."},
	"app.quit":                   {"Quit", "Quit the application."},
	"app.help":                   {"Help", "Show the help screen."},
	"patent.list":                {"List patents", "List stored patents."},
	"patent.get":                 {"Get patent", "Fetch one patent's record."},
	"patent.relations":           {"Patent relations", "List a patent's family-graph edges."},
	"patent.family_graph":        {"Patent family graph", "Load a bounded parent/child family DAG around one patent."},
	"project.list":               {"List projects", "List all projects."},
	"ids.export":                 {"Export IDS", "Build the Information Disclosure Statement for the selected project."},
	"patent.mark-stored":         {"Mark stored", "Set the selected patent to the stored review state."},
	"patent.mark-under-review":   {"Mark under review", "Set the selected patent to under review."},
	"patent.mark-ignored":        {"Mark ignored", "Set the selected patent to ignored."},
	"patent.mark-deleted":        {"Mark deleted", "Soft-delete the selected patent from the project."},
	"patent.delete":              {"Delete patent", "Permanently remove the selected patent from the database."},
	"select.visual":              {"Visual select", "Toggle visual mode at the cursor to begin range selection."},
	"select.clear":               {"Clear selection", "Exit visual mode and clear the selection range."},
	"select.all":                 {"Select all", "Select every patent in the current list."},
	"highlight.family":           {"Highlight family", "Toggle inline highlight of parent/child rows of the selected patent."},
	"highlight.citations":        {"Highlight citations", "Toggle inline highlight of citations and cited-by rows of the selected patent."},
	"relation_filter.collapse":   {"Collapse relations", "Collapse catalog view to show only highlighted relation rows."},
	"relation_filter.expand":     {"Expand relations", "Expand catalog view to show all patents."},
	"col.next":                   {"Next column", "Move the visual focus to the next column."},
	"col.prev":                   {"Prev column", "Move the visual focus to the previous column."},
	"col.sort-apply":             {"Apply sort", "Apply sorting to the currently focused column."},
	"patent.add-to-project":      {"Add to project", "Add the selected patent to the active project."},
	"patent.tag":                 {"Tag patent", "Tag the selected patent within the active project; an unknown name creates the tag."},
	"patent.untag":               {"Untag patent", "Remove a tag from the selected patent within the active project."},
	"crawl.family":               {"Crawl family", "Recursively crawl the selected patent's family graph (parents and children)."},
	"crawl.citations":            {"Crawl citations", "Crawl patents the selected patent cites."},
	"crawl.citedby":              {"Crawl cited-by", "Crawl patents that cite the selected patent."},
	"crawl.all":                  {"Crawl all", "Crawl the full family graph including citations and cited-by."},
	"patent.lookup":              {"Lookup patent", "Fetch the selected patent's record from the web."},
	"patent.import":              {"Import patent", "Fetch a patent by number (add 'force' to bypass the cache) or load a fixture file by path."},
	"crawl.cancel":               {"Cancel crawl", "Cancel a running crawl job."},
	"project.create":             {"Create project", "Create a new project."},
	"project.activate":           {"Use project", "Make the selected project the active project for patent actions."},
	"project.clear-active":       {"Clear active project", "Clear the active project filter and target."},
	"view.filter":                {"Filter", "Apply a boolean filter to the current list (e.g. :filter tag:prior_art and not state:under_review, :filter class:S04*, :filter inventor:\"Ada Lovelace\" and assignee:Acme* and search:\"widget sensor\")."},
	"find.open":                  {"Find", "Open the inline find bar; type to search, n/N to navigate, Enter to keep, Esc to cancel."},
	"ids.edit-field":             {"Edit IDS field", "Edit the selected IDS field."},
	"ids.toggle-full":            {"Toggle IDS full", "Toggle whether the full document is cited on the IDS."},
	"ids.cycle-status":           {"Cycle IDS status", "Cycle the IDS entry status through pending, submitted, and accepted."},
	"ids.delete":                 {"Delete IDS entry", "Remove the current patent from the curated IDS."},
	"view.fulltext":              {"Full text", "Open the full claims text viewer for the selected patent."},
	"edit.copy":                  {"Copy/yank", "Copy the selection to the clipboard with its locator and capture timestamp."},
	"edit.copy-meta":             {"Copy with patent info", "Copy the selection to the clipboard with locator, timestamp, and patent metadata."},
	"edit.note-add":              {"Add to notes", "Add the selected passage and its locator to the session notes buffer."},
	"edit.note-open":             {"Open notes", "Show the accumulated notes buffer for this patent."},
	"tag.add":                    {"Add taxonomy tag", "Register a new tag in the project's taxonomy."},
	"tag.list":                   {"List taxonomy tags", "List all tags in the project's taxonomy."},
	"tag.delete":                 {"Delete taxonomy tag", "Remove a tag from the project's taxonomy."},
	"tag.patent.add":             {"Assign patent tag", "Assign a taxonomy tag to the selected patent."},
	"tag.patent.delete":          {"Remove patent tag", "Remove a tag assignment from the selected patent."},
	"tag.patent.list":            {"List patent tags", "List tags assigned to the selected patent."},
	"tag.patent":                 {"Manage patent tags", "Open the interactive tag selector popup for the selected patent(s)."},

	"class.list":             {"List classifications", "List all cached patent classification definitions."},
	"class.lookup":           {"Lookup classification", "Lookup classification code details from the EPO Linked Open Data SPARQL Endpoint."},
	"patent.classifications": {"Show patent classifications", "Open the selected patent's classification codes with their cached descriptions; press L inside to fetch any missing ones."},
	"history.back":           {"History back", "Go back to the previously viewed patent details."},
	"history.forward":        {"History forward", "Go forward in the history of viewed patent details."},
	"view.history":           {"History", "Open the interactive history popup overlay."},
	"view.all-notes":         {"All notes", "Open the all-notes pane listing every project note with sort and export."},
	"notes.sort-toggle":      {"Sort notes", "Toggle note list sort between date (most recent) and patent number."},
	"notes.export-md":        {"Export notes", "Export all project notes to a Markdown file in your home directory."},
}

// englishNamed is the English text for every named key.
var englishNamed = map[Key]string{
	StatusWelcome:                 "select a project to begin — press ? for help",
	StatusActiveProject:           "active project: %s",
	StatusActiveProjectSaveErr:    "active project: %s (save failed: %s)",
	StatusClearedProject:          "cleared active project",
	StatusProjectNotFound:         "project not found: %s",
	StatusNoPatentSelected:        "no patent selected",
	StatusNoActiveProject:         "select an active project first",
	StatusDaemonUnavailable:       "daemon connection unavailable",
	StatusNoProjectSelection:      "focused pane has no project selection",
	StatusNoProjectSelected:       "no project selected",
	StatusUnknownCommand:          "unknown command: %s",
	StatusCommandNotHere:          "%s is not available here",
	StatusUsage:                   "usage: %s",
	StatusUnhandledCommand:        "unhandled command: %s",
	StatusInvalidPatentNumber:     "invalid patent number: %s",
	StatusDaemonClosed:            "daemon connection closed",
	StatusAIAnalysisStarted:       "AI analysis starting on patent %s via %s...",
	StatusAIAnalysisFailed:        "AI curation failed: %s",
	StatusAIAnalysisComplete:      "AI analysis complete for %s. Report saved in session notes.",
	StatusCrawlProgress:           "crawl %s — depth %d/%d, crawled %d, discovered %d: %s",
	StatusCrawlFailed:             "crawl %s failed: %s",
	StatusCrawlComplete:           "crawl %s complete",
	StatusCrawlStarted:            "crawl started for %s (%s)",
	StatusCrawlStartFailed:        "crawl failed: %s",
	StatusCrawlDepthMax:           "crawl family depth max: %d",
	StatusAddFailed:               "add to project failed: %s",
	StatusAdded:                   "added %s to %s",
	StatusAddedNoCrawl:            "added %s — press L to lookup",
	StatusSetStateFailed:          "set state failed: %s",
	StatusSetState:                "set %s to %s review state in %s",
	StatusExportFailed:            "IDS export failed: %s",
	StatusExportDone:              "IDS for %q: %d disclosed reference(s)",
	StatusNotesExportDone:         "exported %d note(s) to %s",
	StatusNotesExportFailed:       "notes export failed: %s",
	StatusNotesLoadFailed:         "load notes failed: %s",
	StatusNotesSorted:             "sorted by %s",
	StatusProjectCreateFailed:     "create project failed: %s",
	StatusProjectCreated:          "created project %s",
	StatusProjectNameEmpty:        "project name cannot be empty",
	StatusImportFailed:            "import failed: %s",
	StatusImported:                "imported %s",
	StatusTagFailed:               "tag failed: %s",
	StatusTagged:                  "tagged %s as %q in %s",
	StatusUntagFailed:             "untag failed: %s",
	StatusUntagged:                "removed tag %q from %s",
	StatusDeleteFailed:            "delete failed: %s",
	StatusDeleted:                 "deleted %s",
	StatusBatchDeleted:            "deleted %d patents",
	StatusBatchSetState:           "set %d patents to %s in %s",
	StatusBatchAdded:              "added %d patents to %s",
	StatusFilter:                  "%s",
	StatusBrowserOpenFailed:       "open browser failed: %s",
	StatusBrowserOpened:           "opened %d patent page(s) in browser",
	StatusNotesAdded:              "added %s to notes buffer",
	StatusNotesFlushed:            "flushed notes for %s to IDS: %s",
	StatusNotesSavedToPatentNote:  "saved notes for %s to patent note",
	StatusCopiedToClipboard:       "copied to clipboard: %d bytes",
	StatusClipboardFailed:         "clipboard: %s",
	StatusTagTaxonomyAddFailed:    "add tag to taxonomy failed: %s",
	StatusTagTaxonomyAdded:        "added taxonomy tag %q to %s",
	StatusTagTaxonomyDeleteFailed: "delete tag from taxonomy failed: %s",
	StatusTagTaxonomyDeleted:      "deleted taxonomy tag %q from %s",
	StatusTagTaxonomyListFailed:   "list taxonomy tags failed: %s",
	StatusTagPatentAddFailed:      "assign tag to patent failed: %s",
	StatusTagPatentAdded:          "assigned tag %q to patent %s",
	StatusTagPatentDeleteFailed:   "remove tag from patent failed: %s",
	StatusTagPatentListFailed:     "list patent tags failed: %s",

	StatusHistoryEmpty:             "history is empty",
	StatusHistoryAtEnd:             "reached the end of history",
	StatusHistoryPatentUnavailable: "patent %s is no longer available in the database",

	StatusClassificationLookupFailed:  "classification lookup failed: %s",
	StatusClassificationLookupSuccess: "crawled classification %s: %s",
	StatusClassificationListFailed:    "list classifications failed: %s",
	StatusClassificationDeleteFailed:  "delete classification failed: %s",
	StatusClassificationDeleted:       "deleted classification %s",

	HintCommands:        "commands",
	HintCommand:         "command",
	HintDetail:          "detail",
	HintCitations:       "citations",
	HintCitedBy:         "cited by",
	HintParents:         "parents",
	HintChildren:        "children",
	HintFamily:          "family graph",
	HintIDS:             "IDS",
	HintProjects:        "projects",
	HintProjectActions:  "project actions",
	HintBack:            "back",
	HintNewProject:      "new project",
	HintSelectProject:   "select project",
	HintSelect:          "select",
	HintClearActive:     "clear active",
	HintExportIDS:       "export IDS",
	HintFullText:        "full text",
	HintCrawl:           "crawl family",
	HintLookup:          "lookup patent",
	HintClassifications: "classifications",
	HintBrowse:          "open browser",
	HintJump:            "jump",
	HintHelp:            "help",
	HintQuit:            "quit",
	HintMove:            "move",
	HintSlashCommands:   "/ commands",
	HintHistory:         "history",
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
	HelpSectionFamily:      "Family Graph",
	HelpSectionIDS:         "IDS",
	HelpSectionProjects:    "Projects",
	HelpSectionOverlay:     "Overlay",
	HelpSectionAvailable:   "Available keys",
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
	EditPatentNoteTitle:    "Patent notes",
	EditPatentNoteCaption:  "Edit the project-scoped markdown note for this patent.",
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
