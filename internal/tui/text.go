package tui

type TextKey string

const (
	TextHelpCommands   TextKey = "help.commands"
	TextHelpShortcuts  TextKey = "help.shortcuts"
	TextHelpExamples   TextKey = "help.examples"
	TextHelpScreen     TextKey = "help.screen"
	TextHelpGlobal     TextKey = "help.global"
	TextHelpPopupTitle TextKey = "help.popup.title"

	TextDetailAssignee          TextKey = "detail.assignee"
	TextDetailInventor          TextKey = "detail.inventor"
	TextDetailInventors         TextKey = "detail.inventors"
	TextDetailPublication       TextKey = "detail.publication"
	TextDetailGrant             TextKey = "detail.grant"
	TextDetailClassification    TextKey = "detail.classification"
	TextDetailExpiration        TextKey = "detail.expiration"
	TextDetailStatus            TextKey = "detail.status"
	TextDetailStoredLocal       TextKey = "detail.stored_local"
	TextDetailSource            TextKey = "detail.source"
	TextDetailCitationCount     TextKey = "detail.citation_count"
	TextDetailCitedByCount      TextKey = "detail.cited_by_count"
	TextValueUnknown            TextKey = "value.unknown"
	TextValueEmpty              TextKey = "value.empty"
	TextValuePageStatus         TextKey = "value.page_status"
	TextValueOpenHint           TextKey = "value.open_hint"
	TextValueReviewOpenHint     TextKey = "value.review_open_hint"
	TextValueClassificationHint TextKey = "value.classification_hint"
	TextNavDefault              TextKey = "nav.default"
	TextNavJump                 TextKey = "nav.jump"
	TextDetailOpenHint          TextKey = "detail.open_hint"
	TextListEmpty               TextKey = "list.empty"
	TextListFilter              TextKey = "list.filter"
	TextMessageFilteredBy       TextKey = "message.filtered_by"
	TextMessageRefreshCitedBy   TextKey = "message.refresh_cited_by"
	TextMessageRefreshCitations TextKey = "message.refresh_citations"
	TextMessageRefreshAll       TextKey = "message.refresh_all"
	TextMessageBrowserOpened    TextKey = "message.browser_opened"
	TextMessageBrowserNoPatent  TextKey = "message.browser_no_patent"
	TextMessageBrowserUsage     TextKey = "message.browser_usage"
	TextMessageBrowserEmpty     TextKey = "message.browser_empty"
	TextMessageReviewUsage      TextKey = "message.review_usage"

	TextHelpFilterPatents        TextKey = "help.command.filter_patents"
	TextHelpAddPatent            TextKey = "help.command.add_patent"
	TextHelpImportPatent         TextKey = "help.command.import_patent"
	TextHelpRefreshCitedBy       TextKey = "help.command.refresh_cited_by"
	TextHelpRefreshCitations     TextKey = "help.command.refresh_citations"
	TextHelpRefreshDetails       TextKey = "help.command.refresh_details"
	TextHelpOpenPatent           TextKey = "help.command.open_patent"
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
	TextHelpMoveList             TextKey = "help.shortcut.move_list"
	TextHelpOpenSelected         TextKey = "help.shortcut.open_selected"
	TextHelpJumpViews            TextKey = "help.shortcut.jump_views"
	TextHelpShortcutShowHelp     TextKey = "help.shortcut.show_help"
	TextHelpBackOrQuit           TextKey = "help.shortcut.back_or_quit"
	TextHelpJumpCitations        TextKey = "help.jump.citations"
	TextHelpJumpCitedBy          TextKey = "help.jump.cited_by"
	TextHelpJumpClassification   TextKey = "help.jump.cpc"
	TextHelpJumpText             TextKey = "help.jump.text"
	TextHelpJumpNotes            TextKey = "help.jump.notes"
	TextHelpJumpRefs             TextKey = "help.jump.refs"
	TextHelpJumpAI               TextKey = "help.jump.ai"
	TextHelpJumpWeb              TextKey = "help.jump.web"
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
	TextMessagePreviewLoaded     TextKey = "message.preview_loaded"
	TextMessageStoredPatent      TextKey = "message.stored_patent"
	TextMessageSkippedPatent     TextKey = "message.skipped_patent"
	TextMessageIgnoredPatent     TextKey = "message.ignored_patent"
	TextMessageUnderReviewPatent TextKey = "message.under_review_patent"
	TextMessageDeletedPatent     TextKey = "message.deleted_patent"

	TextDeleteConfirmPrompt TextKey = "delete.confirm_prompt"
	TextHelpDeletePatent    TextKey = "help.shortcut.delete_patent"
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

		TextDetailAssignee:          "Assignee",
		TextDetailInventor:          "Inventor",
		TextDetailInventors:         "Inventors",
		TextDetailPublication:       "Publication",
		TextDetailGrant:             "Grant",
		TextDetailClassification:    "Classification",
		TextDetailExpiration:        "Expiration",
		TextDetailStatus:            "Status",
		TextDetailStoredLocal:       "Stored",
		TextDetailSource:            "Source",
		TextDetailCitationCount:     "Citations",
		TextDetailCitedByCount:      "Cited by",
		TextValueUnknown:            "unknown",
		TextValueEmpty:              "Empty",
		TextValuePageStatus:         "Page %d/%d - items %d-%d of %d",
		TextValueOpenHint:           "%s opens/previews - %s stores imported - %s ignores - %s marks under review - %s/%s page",
		TextValueReviewOpenHint:     "%s opens/previews - %s stores - %s ignores - %s marks under review - %s browser - %s/%s page",
		TextValueClassificationHint: "%s opens detail - %s/%s page",
		TextNavDefault:              "keys: %s/%s move, %s open/filter, %s jump, %s command, %s search, %s help, %s back, %s quit",
		TextNavJump:                 "jump: press a hint key to move focus, esc cancels",
		TextDetailOpenHint:          "Enter filters patents by the selected detail value",
		TextListEmpty:               "No patents found.",
		TextListFilter:              "filter",
		TextMessageFilteredBy:       "filtered by %s: %s",
		TextMessageRefreshCitedBy:   "cited-by refreshed: %d records (was %d)",
		TextMessageRefreshCitations: "citations refreshed: %d records (was %d)",
		TextMessageRefreshAll:       "refresh complete: citations %d (was %d), cited-by %d (was %d)",
		TextMessageBrowserOpened:    "opened browser: %s",
		TextMessageBrowserNoPatent:  "no patent selected for browser",
		TextMessageBrowserUsage:     "usage: :browser [patent-number-or-url]",
		TextMessageBrowserEmpty:     "empty patent number",
		TextMessageReviewUsage:      "usage: :review ignored or :review unreviewed",

		TextHelpFilterPatents:        "Filter the patent list.",
		TextHelpAddPatent:            "Build the Google Patents URL and import that patent.",
		TextHelpImportPatent:         "Import a specific Google Patents URL.",
		TextHelpRefreshCitedBy:       "Re-fetch the current patent page and refresh cited-by records.",
		TextHelpRefreshCitations:     "Re-fetch the current patent page and refresh cited references.",
		TextHelpRefreshDetails:       "Refresh details for visible citation rows so title, inventors, and expiration are shown.",
		TextHelpOpenPatent:           "Open a patent already stored in the database.",
		TextHelpShowCites:            "Show patents cited by the current patent.",
		TextHelpShowCitedBy:          "Show patents citing the current patent.",
		TextHelpShowClassification:   "Show patent classifications (CPC/USPC).",
		TextHelpShowText:             "Show imported abstract, claims, and description text.",
		TextHelpShowRefs:             "Show the Markdown reference list.",
		TextHelpShowNotes:            "Show notes for the current patent.",
		TextHelpSummarize:            "Create a deterministic local summary artifact.",
		TextHelpCompare:              "Compare the current patent with another stored patent.",
		TextHelpRefAdd:               "Add the current patent to the reference list.",
		TextHelpRefExport:            "Show the Markdown reference list.",
		TextHelpReviewIgnored:        "List all ignored citation references with the date they were labeled.",
		TextHelpReviewUnderReview:    "List all citation references under review with the date they were labeled.",
		TextHelpOpenBrowser:          "Open the selected/current patent in the system browser.",
		TextHelpShowHelp:             "Show this help screen.",
		TextHelpMoveList:             "Move in the list.",
		TextHelpOpenSelected:         "Open the selected list item.",
		TextHelpDeletePatent:         "Delete the selected patent (mark as ignored and delete PDF).",
		TextHelpJumpViews:            "Jump to citations, cited-by, classifications, text, notes, refs, AI, or browser.",
		TextHelpShortcutShowHelp:     "Show this help screen.",
		TextHelpBackOrQuit:           "Go back to the list, or quit from the list.",
		TextHelpJumpCitations:        "Jump to citations",
		TextHelpJumpCitedBy:          "Jump to cited-by",
		TextHelpJumpClassification:   "Jump to classifications",
		TextHelpJumpText:             "Jump to full text",
		TextHelpJumpNotes:            "Jump to research notes",
		TextHelpJumpRefs:             "Jump to references",
		TextHelpJumpAI:               "Jump to AI artifacts",
		TextHelpJumpWeb:              "Open in system browser",
		TextCitationsEmpty:           "No citation records.",
		TextCitationsOpenFailed:      "patent is not stored and could not be imported",
		TextPreviewTitle:             "Reference preview",
		TextPreviewStorePrompt:       "Store this patent? %s stores, %s ignores, %s marks under review, %s skips, %s goes back",
		TextPreviewNoAbstract:        "No abstract parsed.",
		TextCitationCreated:          "created",
		TextCitationRefreshed:        "refreshed",
		TextCitationLabeled:          "labeled",
		TextCitationNeverRefreshed:   "never refreshed",
		TextReviewQueueEmpty:         "No references currently under review.",
		TextCitationUnderReview:      "under review",
		TextCitationStored:           "stored",
		TextCitationIgnored:          "ignored",
		TextMessagePreviewLoaded:     "preview loaded: %s",
		TextMessageStoredPatent:      "stored patent: %s",
		TextMessageSkippedPatent:     "skipped patent: %s",
		TextMessageIgnoredPatent:     "ignored patent: %s",
		TextMessageUnderReviewPatent: "marked under review: %s",
		TextMessageDeletedPatent:     "deleted patent: %s",

		TextDeleteConfirmPrompt: "Are you sure you want to delete patent %s and its PDF? (y/n)",
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
