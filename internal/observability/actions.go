package observability

// Action* constants enumerate every semantic mutation/navigation event the app
// emits via Recorder.Record. Engine, API, and TUI all reference these instead
// of raw strings so a typo becomes a compile error and the full set is
// discoverable from one place.
//
// Adding a new action:
//   1. Add the constant here and append it to AllActions.
//   2. Classify it in internal/tui/overlay/history_actions.go (render in the
//      overlay, or explicitly hide). TestEveryActionClassifiedByHistory fails
//      until that is done.
const (
	ActionCrawlStart         = "crawl.start"
	ActionCrawlCancel        = "crawl.cancel"
	ActionImportFile         = "import.file"
	ActionPatentSave         = "patent.save"
	ActionPatentDelete       = "patent.delete"
	ActionPatentRestore      = "patent.restore"
	ActionProjectCreate      = "project.create"
	ActionProjectSwitch      = "project.switch"
	ActionMembershipAdd      = "membership.add"
	ActionMembershipSetState = "membership.set_state"
	ActionTagCreate          = "tag.create"
	ActionTagDelete          = "tag.delete"
	ActionPatentTagAssign    = "patent.tag_assign"
	ActionPatentTagRemove    = "patent.tag_remove"
	ActionFilterApply        = "filter.apply"
	ActionUIFocus            = "ui.focus"
	ActionNotesExport        = "notes.export"
	ActionIDSEntrySave       = "ids.entry.save"
	ActionIDSEntryDelete     = "ids.entry.delete"
	ActionIDSExportPDF       = "ids.export.pdf"
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
	ActionUIFocus,
	ActionNotesExport,
	ActionIDSEntrySave,
	ActionIDSEntryDelete,
	ActionIDSExportPDF,
}
