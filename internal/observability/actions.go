package observability

// Action* constants enumerate every semantic mutation/navigation event the app
// emits via Recorder.Record. Engine, API, and TUI all reference these instead
// of raw strings so a typo becomes a compile error and the full set is
// discoverable from one place.
//
// Adding a new action:
//  1. Add the constant here and append it to AllActions.
//  2. Classify it in internal/tui/overlay/history_actions.go (render in the
//     overlay, or explicitly hide). TestEveryActionClassifiedByHistory fails
//     until that is done.
const (
	ActionCrawlStart            = "crawl.start"
	ActionCrawlCancel           = "crawl.cancel"
	ActionImportFile            = "import.file"
	ActionPatentSave            = "patent.save"
	ActionPatentDelete          = "patent.delete"
	ActionPatentSoftDelete      = "patent.soft_delete"
	ActionPatentRestore         = "patent.restore"
	ActionProjectCreate         = "project.create"
	ActionProjectSwitch         = "project.switch"
	ActionMembershipAdd         = "membership.add"
	ActionMembershipSetState    = "membership.set_state"
	ActionTagCreate             = "tag.create"
	ActionTagDelete             = "tag.delete"
	ActionPatentTagAssign       = "patent.tag_assign"
	ActionPatentTagRemove       = "patent.tag_remove"
	ActionFilterApply           = "filter.apply"
	ActionTableFilterApply      = "table_filter.apply"
	ActionTableViewSelect       = "table_view.select"
	ActionTableViewSave         = "table_view.save"
	ActionTableViewDelete       = "table_view.delete"
	ActionUIBrowse              = "ui.browse"
	ActionUIFocus               = "ui.focus"
	ActionUIClearCache          = "ui.clear_cache"
	ActionNotesExport           = "notes.export"
	ActionAddedExport           = "added.export"
	ActionAddedImport           = "added.import"
	ActionNotesSave             = "notes.save"
	ActionNotesDelete           = "notes.delete"
	ActionIDSEntrySave          = "ids.entry.save"
	ActionIDSEntryDelete        = "ids.entry.delete"
	ActionIDSEntryBulkSetStatus = "ids.entry.bulk_set_status"
	ActionIDSExportPDF          = "ids.export.pdf"
	ActionSourceModeSet         = "config.source_mode.set"
	// CLI management operations (for observability of backup/clean of patents XML cache, logs, etc.)
	ActionCLIPatentsList    = "cli.patents.list"
	ActionCLIPatentsArchive = "cli.patents.archive"
	ActionCLIPatentsClean   = "cli.patents.clean"

	// Resolution / crawl failure paths (for the compare-mode + grant number issues)
	ActionCrawlRootNotFound    = "crawl.root_not_found"
	ActionUSPTOCandidateSearch = "uspto.candidate.search"

	// Source comparison / reconciliation (Option A persistence of overlay choices)
	ActionSourceCompareOpen  = "source.compare.open"
	ActionSourceDiffsLoad    = "source.diffs.load"
	ActionSourceResolveDiffs = "source.resolve_diffs"

	// Drafting subsystem (provisional / non-provisional applications and
	// office-action responses rendered to .docx).
	ActionDraftCreate        = "draft.create"
	ActionDraftSave          = "draft.save"
	ActionDraftDelete        = "draft.delete"
	ActionDraftExportDocx    = "draft.export.docx"
	ActionDraftSectionAI     = "draft.section.ai"
	ActionOfficeActionImport = "office_action.import"
)

// AllActions is the canonical list of every emitted action. Tests iterate this
// to enforce that downstream consumers (history overlay, replay router) handle
// every value.
var AllActions = []string{
	ActionCrawlStart,
	ActionCrawlCancel,
	ActionImportFile,
	ActionPatentSave,
	ActionPatentDelete,
	ActionPatentSoftDelete,
	ActionPatentRestore,
	ActionProjectCreate,
	ActionProjectSwitch,
	ActionMembershipAdd,
	ActionMembershipSetState,
	ActionTagCreate,
	ActionTagDelete,
	ActionPatentTagAssign,
	ActionPatentTagRemove,
	ActionFilterApply,
	ActionTableFilterApply,
	ActionTableViewSelect,
	ActionTableViewSave,
	ActionTableViewDelete,
	ActionUIBrowse,
	ActionUIFocus,
	ActionNotesExport,
	ActionNotesSave,
	ActionNotesDelete,
	ActionIDSEntrySave,
	ActionIDSEntryDelete,
	ActionIDSEntryBulkSetStatus,
	ActionIDSExportPDF,
	ActionSourceModeSet,
	ActionCLIPatentsList,
	ActionCLIPatentsArchive,
	ActionCLIPatentsClean,
	ActionCrawlRootNotFound,
	ActionUSPTOCandidateSearch,
	ActionSourceCompareOpen,
	ActionSourceDiffsLoad,
	ActionSourceResolveDiffs,
	ActionUIClearCache,
	ActionDraftCreate,
	ActionDraftSave,
	ActionDraftDelete,
	ActionDraftExportDocx,
	ActionDraftSectionAI,
	ActionOfficeActionImport,
}
