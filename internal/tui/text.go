package tui

type TextKey string

const (
	TextHelpCommands   TextKey = "help.commands"
	TextHelpShortcuts  TextKey = "help.shortcuts"
	TextHelpExamples   TextKey = "help.examples"
	TextHelpScreen     TextKey = "help.screen"
	TextHelpGlobal     TextKey = "help.global"
	TextHelpPopupTitle TextKey = "help.popup.title"

	TextDetailAssignee           TextKey = "detail.assignee"
	TextDetailInventor           TextKey = "detail.inventor"
	TextDetailInventors          TextKey = "detail.inventors"
	TextDetailPublication        TextKey = "detail.publication"
	TextDetailGrant              TextKey = "detail.grant"
	TextDetailClassification     TextKey = "detail.classification"
	TextDetailCountry            TextKey = "detail.country"
	TextDetailExpiration         TextKey = "detail.expiration"
	TextDetailStatus             TextKey = "detail.status"
	TextDetailIDS                TextKey = "detail.ids"
	TextDetailLatestAssignment   TextKey = "detail.latest_assignment"
	TextDetailStoredLocal        TextKey = "detail.stored_local"
	TextDetailUpdated            TextKey = "detail.updated"
	TextDetailSource             TextKey = "detail.source"
	TextDetailCitationCount      TextKey = "detail.citation_count"
	TextDetailCitedByCount       TextKey = "detail.cited_by_count"
	TextDetailFamilyParents      TextKey = "detail.family_parents"
	TextDetailFamilyChildren     TextKey = "detail.family_children"
	TextDetailNotes              TextKey = "detail.notes"
	TextDetailAbstract           TextKey = "detail.abstract"
	TextDetailImportSource       TextKey = "detail.import_source"
	TextDetailApplication        TextKey = "detail.application"
	TextDetailPublicationLong    TextKey = "detail.publication_long"
	TextDetailGrantLong          TextKey = "detail.grant_long"
	TextDetailFirstClaim         TextKey = "detail.first_claim"
	TextValueUnknown             TextKey = "value.unknown"
	TextValuePending             TextKey = "value.pending"
	TextValueEmpty               TextKey = "value.empty"
	TextValuePageStatus          TextKey = "value.page_status"
	TextValueOpenHint            TextKey = "value.open_hint"
	TextValueReviewOpenHint      TextKey = "value.review_open_hint"
	TextValueClassificationHint  TextKey = "value.classification_hint"
	TextValueExpirationManual    TextKey = "value.expiration.manual"
	TextValueExpirationImported  TextKey = "value.expiration.imported"
	TextValueExpirationEstimated TextKey = "value.expiration.estimated"
	TextValueSearchLabel         TextKey = "value.search_label"
	TextValueProjectTag          TextKey = "value.project_tag"
	TextValueBreadcrumbFormat    TextKey = "value.breadcrumb_format"
	TextValueFilterStatusTag     TextKey = "value.filter.status_tag"
	TextValueFilterGeneralTag    TextKey = "value.filter.general_tag"
	TextValueFilterSortTag       TextKey = "value.filter.sort_tag"
	TextValueFilterRefsTag       TextKey = "value.filter.refs_tag"
	TextValueFilterClassTag      TextKey = "value.filter.class_tag"
	TextValueProjectSummaryLead  TextKey = "value.project_summary_lead"
	TextNavDefault               TextKey = "nav.default"
	TextNavList                  TextKey = "nav.list"
	TextNavJump                  TextKey = "nav.jump"
	TextDetailOpenHint           TextKey = "detail.open_hint"
	TextListEmpty                TextKey = "list.empty"
	TextListFilter               TextKey = "list.filter"
	TextMessageFilteredBy        TextKey = "message.filtered_by"
	TextMessageRefreshCitedBy    TextKey = "message.refresh_cited_by"
	TextMessageRefreshCitations  TextKey = "message.refresh_citations"
	TextMessageRefreshAll        TextKey = "message.refresh_all"
	TextMessageBrowserOpened     TextKey = "message.browser_opened"
	TextMessageBrowserNoPatent   TextKey = "message.browser_no_patent"
	TextMessageBrowserUsage      TextKey = "message.browser_usage"
	TextMessageBrowserEmpty      TextKey = "message.browser_empty"
	TextMessageReviewUsage       TextKey = "message.review_usage"

	TextHelpFilterPatents        TextKey = "help.command.filter_patents"
	TextHelpAddPatent            TextKey = "help.command.add_patent"
	TextHelpImportPatent         TextKey = "help.command.import_patent"
	TextHelpRefreshCitedBy       TextKey = "help.command.refresh_cited_by"
	TextHelpRefreshCitations     TextKey = "help.command.refresh_citations"
	TextHelpRefreshDetails       TextKey = "help.command.refresh_details"
	TextHelpOpenPatent           TextKey = "help.command.open_patent"
	TextHelpPatentDate           TextKey = "help.command.patent_date"
	TextHelpPatentNum            TextKey = "help.command.patent_num"
	TextHelpShowCites            TextKey = "help.command.show_cites"
	TextHelpShowCitedBy          TextKey = "help.command.show_cited_by"
	TextHelpShowClassification   TextKey = "help.command.show_cpc"
	TextHelpShowText             TextKey = "help.command.show_text"
	TextHelpShowRefs             TextKey = "help.command.show_refs"
	TextHelpShowNotes            TextKey = "help.command.show_notes"
	TextHelpSummarize            TextKey = "help.command.summarize"
	TextHelpCompare              TextKey = "help.command.compare"
	TextHelpRefAdd               TextKey = "help.command.ref_add"
	TextHelpRefExport            TextKey = "help.command.ref_export"
	TextHelpReviewIgnored        TextKey = "help.command.review_ignored"
	TextHelpReviewUnderReview    TextKey = "help.command.review_under_review"
	TextHelpOpenBrowser          TextKey = "help.command.open_browser"
	TextHelpShowHelp             TextKey = "help.command.show_help"
	TextHelpShowVersion          TextKey = "help.command.show_version"
	TextHelpMoveList             TextKey = "help.shortcut.move_list"
	TextHelpOpenSelected         TextKey = "help.shortcut.open_selected"
	TextHelpJumpViews            TextKey = "help.shortcut.jump_views"
	TextHelpShortcutShowHelp     TextKey = "help.shortcut.show_help"
	TextHelpBackOrQuit           TextKey = "help.shortcut.back_or_quit"
	TextHelpQuitApp              TextKey = "help.shortcut.quit_app"
	TextHelpMoveColumns          TextKey = "help.shortcut.move_columns"
	TextHelpJumpCitations        TextKey = "help.jump.citations"
	TextHelpJumpCitedBy          TextKey = "help.jump.cited_by"
	TextHelpJumpClassification   TextKey = "help.jump.cpc"
	TextHelpJumpText             TextKey = "help.jump.text"
	TextHelpJumpNotes            TextKey = "help.jump.notes"
	TextHelpJumpRefs             TextKey = "help.jump.refs"
	TextHelpJumpAI               TextKey = "help.jump.ai"
	TextHelpJumpWeb              TextKey = "help.jump.web"
	TextHelpJumpProject          TextKey = "help.jump.project"
	TextHelpProjectID            TextKey = "help.project.id"
	TextHelpProjectList          TextKey = "help.project.list"
	TextHelpProjectCreate        TextKey = "help.project.create"
	TextHelpProjectSwitch        TextKey = "help.project.switch"
	TextHelpProjectAdd           TextKey = "help.project.add"
	TextHelpProjectStatus        TextKey = "help.project.status"
	TextHelpProjectComment       TextKey = "help.project.comment"
	TextHelpProjectSummaryStatus TextKey = "help.project.summary_status"
	TextHelpProjectSummary       TextKey = "help.project.summary"
	TextHelpProjectDelete        TextKey = "help.project.delete"
	TextHelpProjectEvent         TextKey = "help.project.event"
	TextHelpProjectEvents        TextKey = "help.project.events"
	TextHelpProjectInvoice       TextKey = "help.project.invoice"
	TextHelpProjectInvoices      TextKey = "help.project.invoices"
	TextHelpProjectIDSAdd        TextKey = "help.project.ids_add"
	TextHelpProjectIDS           TextKey = "help.project.ids"
	TextHelpExportIDS            TextKey = "help.export_ids"
	TextHelpExportStatus         TextKey = "help.export_status"
	TextHelpExportState          TextKey = "help.export_state"
	TextHelpProjectInfo          TextKey = "help.project.info"
	TextHelpFilterClear          TextKey = "help.filter.clear"
	TextHelpSort                 TextKey = "help.sort"
	TextHelpClass                TextKey = "help.class"
	TextHelpInventorFilter       TextKey = "help.inventorfilter"
	TextHelpStatusFilter         TextKey = "help.filterstatus"
	TextHelpFamilyAdd            TextKey = "help.family.add"
	TextHelpFamilyRemove         TextKey = "help.family.remove"
	TextHelpFamilyView           TextKey = "help.family.view"
	TextHelpJumpFamily           TextKey = "help.jump.family"
	TextCitationsEmpty           TextKey = "citations.empty"
	TextCitationsOpenFailed      TextKey = "citations.open_failed"
	TextPreviewTitle             TextKey = "preview.title"
	TextPreviewStorePrompt       TextKey = "preview.store_prompt"
	TextPreviewNoAbstract        TextKey = "preview.no_abstract"
	TextCitationCreated          TextKey = "citation.created"
	TextCitationRefreshed        TextKey = "citation.refreshed"
	TextCitationLabeled          TextKey = "citation.labeled"
	TextCitationNeverRefreshed   TextKey = "citation.never_refreshed"
	TextReviewQueueEmpty         TextKey = "review_queue.empty"
	TextCitationUnderReview      TextKey = "citation.status.under_review"
	TextCitationStored           TextKey = "citation.status.stored"
	TextCitationIgnored          TextKey = "citation.status.ignored"
	TextCitationCached           TextKey = "citation.status.cached"
	TextMessagePreviewLoaded     TextKey = "message.preview_loaded"
	TextMessageStoredPatent      TextKey = "message.stored_patent"
	TextMessageSkippedPatent     TextKey = "message.skipped_patent"
	TextMessageIgnoredPatent     TextKey = "message.ignored_patent"
	TextMessageUnderReviewPatent TextKey = "message.under_review_patent"
	TextMessageDeletedPatent     TextKey = "message.deleted_patent"

	TextDeleteConfirmPrompt TextKey = "delete.confirm_prompt"
	TextHelpDeletePatent    TextKey = "help.shortcut.delete_patent"

	// Family view keys
	TextHelpFamilyParent     TextKey = "help.family.parent"
	TextHelpFamilyChild      TextKey = "help.family.child"
	TextHelpFamilyRemoveEdge TextKey = "help.family.remove_edge"
	TextHelpFamilyPull       TextKey = "help.family.pull"
	TextHelpFamilyAddChild   TextKey = "help.family.add_child"

	// List view
	TextHelpCyclePatentStatus TextKey = "help.cycle_patent_status"

	// Citation/review view
	TextHelpStatusCycle             TextKey = "help.status_cycle"
	TextHelpCitationRefreshSelected TextKey = "help.citation.refresh_selected"
	TextHelpCitationRefreshAll      TextKey = "help.citation.refresh_all"

	// Project info overlay
	TextHelpProjectInfoKeys TextKey = "help.project_info.keys"

	// Project events/invoices/IDS overlays
	TextHelpDeleteSelected TextKey = "help.shortcut.delete_selected"
	TextHelpMarkPaid       TextKey = "help.invoice.mark_paid"

	// Splash / project selector
	TextHelpSplashSelect   TextKey = "help.splash.select"
	TextHelpSplashNew      TextKey = "help.splash.new"
	TextHelpSplashEvents   TextKey = "help.splash.events"
	TextHelpSplashInvoices TextKey = "help.splash.invoices"
	TextHelpSplashIDS      TextKey = "help.splash.ids"

	// General commands
	TextHelpPurge      TextKey = "help.purge"
	TextHelpCompact    TextKey = "help.compact"
	TextHelpNote       TextKey = "help.note"
	TextHelpExit       TextKey = "help.exit"
	TextHelpSearchHint TextKey = "help.search_hint"
)

type TextCatalog map[TextKey]string

func EnglishText() TextCatalog {
	return TextCatalog{
		TextHelpCommands:   "Commands",
		TextHelpShortcuts:  "Shortcuts",
		TextHelpExamples:   "Examples",
		TextHelpScreen:     "This Screen",
		TextHelpGlobal:     "Global",
		TextHelpPopupTitle: "Help",

		TextDetailAssignee:           "Assignee",
		TextDetailInventor:           "Inventor",
		TextDetailInventors:          "Inventors",
		TextDetailPublication:        "Publication",
		TextDetailGrant:              "Grant",
		TextDetailClassification:     "Classification",
		TextDetailCountry:            "Country",
		TextDetailExpiration:         "Expiration",
		TextDetailStatus:             "Status",
		TextDetailIDS:                "IDS",
		TextDetailLatestAssignment:   "Latest Assignment",
		TextDetailStoredLocal:        "Imported",
		TextDetailUpdated:            "Updated",
		TextDetailSource:             "Source",
		TextDetailCitationCount:      "Citations",
		TextDetailCitedByCount:       "Cited by",
		TextDetailFamilyParents:      "Parents",
		TextDetailFamilyChildren:     "Children",
		TextDetailNotes:              "Notes",
		TextDetailAbstract:           "Abstract",
		TextDetailImportSource:       "Via",
		TextDetailApplication:        "Application",
		TextDetailPublicationLong:    "Publication",
		TextDetailGrantLong:          "Grant",
		TextDetailFirstClaim:         "First Claim",
		TextValueUnknown:             "unknown",
		TextValuePending:             "pending",
		TextValueEmpty:               "Empty",
		TextValuePageStatus:          "Page %d/%d - items %d-%d of %d",
		TextValueOpenHint:            "[%s] open/preview · [%s] save · [%s] ignore · [%s] under review · [%s] refresh · [%s/%s] page · [↓↑] scroll",
		TextValueReviewOpenHint:      "[%s] open/preview · [%s] save · [%s] ignore · [%s] under review · [%s] refresh · [%s] browse · [%s/%s] page · [↓↑] scroll",
		TextValueClassificationHint:  "[%s/%s] open detail · [%s] find · [%s] next · [%s/%s] page · [↓↑] scroll",
		TextValueExpirationManual:    " (man.)",
		TextValueExpirationImported:  " (import)",
		TextValueExpirationEstimated: " (est.)",
		TextValueSearchLabel:         " search:/",
		TextValueProjectTag:          "PROJECT: %s (%s)",
		TextValueBreadcrumbFormat:    "[%d] ‹ %s",
		TextValueFilterStatusTag:     "status:",
		TextValueFilterGeneralTag:    "filter:",
		TextValueFilterSortTag:       "sort:",
		TextValueFilterRefsTag:       "refs:",
		TextValueFilterClassTag:      "class:",
		TextValueProjectSummaryLead:  "> ",
		TextNavDefault:               "keys: [%s/%s/↓↑] move · [%s] open · [%s] jump · [%s] sort · [%s] cmd · [%s] find · [%s] help · [%s] back · [%s] quit",
		TextNavList:                  "keys: [%s/%s/↓↑] rows · [%s/%s/←→] cols · [%s] open · [%s] jump · [%s] sort · [%s] cmd · [%s] find · [%s] help · [%s] back · [%s] quit",
		TextNavJump:                  "jump: press a hint key to move focus, [esc] cancels",
		TextDetailOpenHint:           "Enter filters patents by the selected detail value",
		TextListEmpty:                "No patents found.",
		TextListFilter:               "filter",
		TextMessageFilteredBy:        "filtered by %s: %s",
		TextMessageRefreshCitedBy:    "cited-by refreshed: %d records (was %d)",
		TextMessageRefreshCitations:  "citations refreshed: %d records (was %d)",
		TextMessageRefreshAll:        "refresh complete: citations %d (was %d), cited-by %d (was %d)",
		TextMessageBrowserOpened:     "opened browser: %s",
		TextMessageBrowserNoPatent:   "no patent selected for browser",
		TextMessageBrowserUsage:      "usage: :browser [patent-number-or-url]",
		TextMessageBrowserEmpty:      "empty patent number",
		TextMessageReviewUsage:       "usage: :review ignored or :review under_review",

		TextHelpFilterPatents:           "Filter the patent list.",
		TextHelpAddPatent:               "Build the Google Patents URL and import that patent.",
		TextHelpImportPatent:            "Import a specific Google Patents URL.",
		TextHelpRefreshCitedBy:          "Re-fetch the current patent page and refresh cited-by records.",
		TextHelpRefreshCitations:        "Re-fetch the current patent page and refresh cited references.",
		TextHelpRefreshDetails:          "Refresh details for visible citation rows (citations, cited-by, or review queue) so title, inventors, and expiration are shown.",
		TextHelpOpenPatent:              "Open a patent already stored in the database.",
		TextHelpPatentDate:              "Update patent date (supports YYYY/MM/DD).",
		TextHelpPatentNum:               "Update patent application/publication/grant number.",
		TextHelpShowCites:               "Show patents cited by the current patent.",
		TextHelpShowCitedBy:             "Show patents citing the current patent.",
		TextHelpShowClassification:      "Show patent classifications (CPC/USPC).",
		TextHelpShowText:                "Show imported abstract, claims, and description text.",
		TextHelpShowRefs:                "Show the Markdown reference list.",
		TextHelpShowNotes:               "Show notes for the current patent.",
		TextHelpSummarize:               "Create a deterministic local summary artifact.",
		TextHelpCompare:                 "Compare the current patent with another stored patent.",
		TextHelpRefAdd:                  "Add the current patent to the reference list.",
		TextHelpRefExport:               "Show the Markdown reference list.",
		TextHelpReviewIgnored:           "List all ignored citation references with the date they were labeled.",
		TextHelpReviewUnderReview:       "List all citation references under review with the date they were labeled.",
		TextHelpOpenBrowser:             "Open the selected/current patent in the system browser.",
		TextHelpShowHelp:                "Show this help screen.",
		TextHelpShowVersion:             "Show the current PatentMine version.",
		TextHelpMoveList:                "Move row or column focus. Supports multipliers: [10j], [4l], etc.",
		TextHelpMoveColumns:             "Move list column focus with [h/l] or left/right arrows. Supports multipliers: [4l].",
		TextHelpOpenSelected:            "Open the selected list item.",
		TextHelpDeletePatent:            "Delete the selected patent (mark as ignored and delete PDF).",
		TextHelpJumpViews:               "Jump to citations, cited-by, classifications, text, notes, refs, AI, or browser.",
		TextHelpShortcutShowHelp:        "Show this help screen.",
		TextHelpBackOrQuit:              "Go back or clear the current search/filter state.",
		TextHelpQuitApp:                 "Quit the application immediately.",
		TextHelpJumpCitations:           "Jump to citations",
		TextHelpJumpCitedBy:             "Jump to cited-by",
		TextHelpJumpClassification:      "Jump to classifications",
		TextHelpJumpText:                "Jump to full text",
		TextHelpJumpNotes:               "Jump to research notes",
		TextHelpJumpRefs:                "Jump to references",
		TextHelpJumpAI:                  "Jump to AI artifacts",
		TextHelpJumpWeb:                 "Open in system browser",
		TextHelpJumpProject:             "Jump to project selection",
		TextHelpProjectID:               "Print the current project ID and name as a status message.",
		TextHelpProjectList:             "List all projects.",
		TextHelpProjectCreate:           "Create a new project.",
		TextHelpProjectSwitch:           "Switch to a different project.",
		TextHelpProjectAdd:              "Add current patent to a project.",
		TextHelpProjectStatus:           "Set project lifecycle status (active/archived).",
		TextHelpProjectComment:          "Update internal comment on current project.",
		TextHelpProjectSummaryStatus:    "Set application stage (work-in-progress/provisional-filed/application-filed/published/granted).",
		TextHelpProjectSummary:          "Set free-text summary shown on splash and in project info.",
		TextHelpProjectDelete:           "Delete project and all associated data.",
		TextHelpProjectEvent:            "Add a prosecution event (type, date, due, ref, note).",
		TextHelpProjectEvents:           "Open prosecution history overlay for current project.",
		TextHelpProjectInvoice:          "Add invoice to/from law firm (amount, currency, direction, date, due, firm, ref, note).",
		TextHelpProjectInvoices:         "Open invoice list overlay for current project.",
		TextHelpProjectIDSAdd:           "Add a patent to the IDS (Information Disclosure Statement) for this project.",
		TextHelpProjectIDS:              "Open the IDS overlay to view/manage prior art references.",
		TextHelpExportIDS:               "Export the IDS as a Markdown file (default: <project>_IDS_<date>.md).",
		TextHelpExportStatus:            "Export project status: stage, events, invoices, and patent counts as Markdown.",
		TextHelpExportState:             "Export patent list filtered by state (stored/ignored/under-review/all/none). Default: current filter.",
		TextHelpProjectInfo:             "Open project info popup (editable: [s]=app status, [m]=summary, [c]=comment, [S]=status).",
		TextHelpFilterClear:             "Reset all filters (status, class, inventor) back to defaults.",
		TextHelpSort:                    "Sort the list by column (number, title, date, status, assignee, inventor, class, expiration, updated). Comma-separate for secondary sort: status,expiration.",
		TextHelpClass:                   "Filter by classification prefix. Supports && (AND) and || (OR): H04N && G06F or H04N || G06F. Clear with :classfilter clear.",
		TextHelpInventorFilter:          "Filter the patent list by inventor name. Clear with :inventorfilter clear.",
		TextHelpStatusFilter:            "Filter by patent status: stored (default), ignored, under-review, all.",
		TextHelpFamilyAdd:               "Declare a parent or child relationship. Types: continuation, divisional, cip, pct.",
		TextHelpFamilyRemove:            "Remove a family relationship with the specified patent.",
		TextHelpFamilyView:              "Open the patent family overlay (parents and children).",
		TextHelpJumpFamily:              "Jump to patent family view.",
		TextHelpFamilyParent:            "Move selection to the parent node.",
		TextHelpFamilyChild:             "Move selection to the first child node.",
		TextHelpFamilyRemoveEdge:        "Remove the edge between the selected node and its tree-parent.",
		TextHelpFamilyPull:              "Refresh the selected family member; on the current patent, pull and store the full family.",
		TextHelpFamilyAddChild:          "Pre-fill :family child command in the input bar.",
		TextHelpCyclePatentStatus:       "Open the status selection popup for the selected patent(s).",
		TextHelpStatusCycle:             "Cycle the status filter: stored → ignored → under-review → all.",
		TextHelpCitationRefreshSelected: "Re-fetch the selected citation from Google Patents (title, inventors, expiration).",
		TextHelpCitationRefreshAll:      "Re-fetch all citations for the current patent from Google Patents.",
		TextHelpProjectInfoKeys:         "Edit project fields inline: [s]=app status, [m]=summary, [c]=comment, [S]=project status.",
		TextHelpDeleteSelected:          "Delete or remove the selected item.",
		TextHelpMarkPaid:                "Mark the selected invoice as paid.",
		TextHelpSplashSelect:            "Open the selected project.",
		TextHelpSplashNew:               "Create a new project.",
		TextHelpSplashEvents:            "Open prosecution events for the selected project.",
		TextHelpSplashInvoices:          "Open invoice list for the selected project.",
		TextHelpSplashIDS:               "Open IDS for the selected project.",
		TextCitationsEmpty:              "No citation records.",
		TextCitationsOpenFailed:         "patent is not stored and could not be imported",
		TextPreviewTitle:                "Reference preview",
		TextPreviewStorePrompt:          "[%s] save · [%s] ignore · [%s] under review · [%s] skip · [%s] back",
		TextPreviewNoAbstract:           "No abstract parsed.",
		TextCitationCreated:             "created",
		TextCitationRefreshed:           "refreshed",
		TextCitationLabeled:             "labeled",
		TextCitationNeverRefreshed:      "never refreshed",
		TextReviewQueueEmpty:            "No references currently under review.",
		TextCitationUnderReview:         "under-review",
		TextCitationStored:              "stored",
		TextCitationIgnored:             "ignored",
		TextCitationCached:              "cached",
		TextMessagePreviewLoaded:        "patent preview loaded",

		TextMessageStoredPatent:      "stored patent: %s",
		TextMessageSkippedPatent:     "skipped patent: %s",
		TextMessageIgnoredPatent:     "ignored patent: %s",
		TextMessageUnderReviewPatent: "marked under review: %s",
		TextMessageDeletedPatent:     "deleted patent: %s",

		TextDeleteConfirmPrompt: "Are you sure you want to delete patent %s and its PDF? (y/n)",

		TextHelpPurge:      "Purge all ignored patents from the project and vacuum the database.",
		TextHelpCompact:    "Compact (vacuum) the SQLite database to reclaim space.",
		TextHelpNote:       "Log a research note for the current patent.",
		TextHelpExit:       "Quit the application.",
		TextHelpSearchHint: "[/] to search (smart-case) · [q/esc] to close",
	}
}

func (c TextCatalog) T(key TextKey) string {
	if c == nil {
		return EnglishText().T(key)
	}
	if value, ok := c[key]; ok {
		return value
	}
	return string(key)
}
