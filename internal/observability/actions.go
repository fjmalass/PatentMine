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
	ActionUICommand             = "ui.command"
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
	ActionDraftCreate           = "draft.create"
	ActionDraftSave             = "draft.save"
	ActionDraftDelete           = "draft.delete"
	ActionDraftExportDocx       = "draft.export.docx"
	ActionDraftSectionAI        = "draft.section.ai"
	ActionOfficeActionGet       = "office_action.get"
	ActionOfficeActionImport    = "office_action.import"
	ActionOfficeActionList      = "office_action.list"
	ActionOfficeActionSaveNotes = "office_action.save_notes"
	ActionOfficeActionUpdate    = "office_action.update"
	ActionOfficeActionDelete    = "office_action.delete"

	// Prosecution-matter workspace (matter-scoped documents and project stage).
	ActionMatterDocumentImport     = "matter_document.import"
	ActionMatterDocumentRename     = "matter_document.rename"
	ActionMatterDocumentDelete     = "matter_document.delete"
	ActionMatterDocumentExtract    = "matter_document.extract" // deterministic text extraction
	ActionMatterDocumentTag        = "matter_document.tag"
	ActionMatterDocumentUntag      = "matter_document.untag"
	ActionMatterDocumentAssignOA   = "matter_document.assign_office_action"
	ActionMatterDocumentUnassignOA = "matter_document.unassign_office_action"
	ActionProjectSetMatterType     = "project.set_matter_type"
	ActionMatterEventAdd           = "matter_event.add"
	ActionMatterEventDelete        = "matter_event.delete"
	ActionTimeLog                  = "time_entry.log"
	ActionTimeValidate             = "time_entry.validate"
	ActionTimeDelete               = "time_entry.delete"
	ActionAIUsage                  = "ai_usage.record" // an AI call recorded for billing
	ActionRenewalsTrack            = "deadline.track_renewals"
	ActionRenewalsUntrack          = "deadline.untrack_renewals"
	ActionDeadlineStatus           = "deadline.set_status"
	ActionDeadlineRemind           = "deadline.remind"
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
	ActionUICommand,
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
	ActionOfficeActionGet,
	ActionOfficeActionImport,
	ActionOfficeActionList,
	ActionOfficeActionSaveNotes,
	ActionOfficeActionUpdate,
	ActionOfficeActionDelete,
	ActionMatterDocumentImport,
	ActionMatterDocumentRename,
	ActionMatterDocumentDelete,
	ActionMatterDocumentExtract,
	ActionMatterDocumentTag,
	ActionMatterDocumentUntag,
	ActionMatterDocumentAssignOA,
	ActionMatterDocumentUnassignOA,
	ActionProjectSetMatterType,
	ActionMatterEventAdd,
	ActionMatterEventDelete,
	ActionTimeLog,
	ActionTimeValidate,
	ActionTimeDelete,
	ActionAIUsage,
	ActionRenewalsTrack,
	ActionRenewalsUntrack,
	ActionDeadlineStatus,
	ActionDeadlineRemind,
}
