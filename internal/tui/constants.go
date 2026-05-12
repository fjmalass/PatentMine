package tui

import "patentmine/internal/domain"

const (
	commandSearch             = "search"
	commandOpen               = "open"
	commandAdd                = "add"
	commandImport             = "import"
	commandRefresh            = "refresh"
	commandRefreshRefsDetails = "refresh-refs-details"
	commandCitedBy            = "citedby"
	commandClassification     = "cpc"
	commandText               = "text"
	commandRefs               = "refs"
	commandNotes              = "notes"
	commandSummarize          = "summarize"
	commandCompare            = "compare"
	commandRef                = "ref"
	commandReview             = "review"
	commandIgnored            = "ignored"
	commandUnderReview        = "under-review"
	commandHelp               = "help"
	commandHelpShort          = "h"
	commandBrowser            = "browser"
	commandWeb                = "web"
	commandProject            = "project"
	commandSort               = "sort"
	commandFilter             = "filter"
	commandClassFilter        = "classfilter"
	commandInventorFilter     = "inventorfilter"
	commandStatusFilter       = "statusfilter"
	commandFamily             = "family"
	commandPurge              = "purge"
	commandCompact            = "compact"
	commandNote               = "note"
	commandVersion            = "version"
	commandExit               = "exit"
	commandFamilyPull         = "pull"

	// :project subcommands
	projectSubID            = "id"
	projectSubList          = "list"
	projectSubCreate        = "create"
	projectSubAdd           = "add"
	projectSubSwitch        = "switch"
	projectSubStatus        = "status"
	projectSubSummaryStatus = "summary-status"
	projectSubSummary       = "summary"
	projectSubComment       = "comment"
	projectSubDelete        = "delete"
	projectSubEvent         = "event"
	projectSubEvents        = "events"
	projectSubInvoice       = "invoice"
	projectSubInvoices      = "invoices"
	projectSubIDS           = "ids"
	projectSubExport        = "export"

	// :project ids subcommands
	idsSubAdd  = "add"
	idsSubMeta = "meta"

	// :project export subcommands
	exportSubIDS    = "ids"
	exportSubStatus = "status"
	exportSubState  = "state"

	// :filter subcommands
	filterSubStatus   = "status"
	filterSubClass    = "class"
	filterSubInventor = "inventor"
	filterSubClear    = "clear"

	// :family subcommands
	familySubParent = "parent"
	familySubChild  = "child"
	familySubRemove = "remove"
	familySubView   = "view"

	// activity actions
	activityPatentAdd      = "patent.add"
	activityPatentStatus   = "patent.status"
	activityPatentImport   = "patent.import"
	activityCitationStatus = "citation.status"
	activityCitationStore  = "citation.store"
	activityNoteAdd        = "note.add"
	activityRefAdd         = "ref.add"
	activityIDSAdd         = "ids.add"
	activityIDSStatus      = "ids.status"

	// export state aliases
	exportStateAll         = "all"
	exportStateNone        = "none"
	exportStateStored      = "stored"
	exportStateIgnored     = "ignored"
	exportStateUnderReview = "under-review"

	// invoice/event argument keywords
	argDate      = "date"
	argDue       = "due"
	argRef       = "ref"
	argNote      = "note"
	argCurrency  = "currency"
	argDirection = "direction"
	argFirm      = "firm"
	argStatus    = "status"

	refActionAdd    = "add"
	refActionExport = "export"

	refreshTargetAll       = "all"
	refreshTargetCitations = "citations"
	refreshTargetCitedBy   = "citedby"
	refreshArgDetails      = "details"

	importActionAdded     = "added"
	importActionImported  = "imported"
	importActionOpened    = "opened"
	importActionRefreshed = "refreshed"

	keyEnter             = "enter"
	keyEsc               = "esc"
	keyCtrlC             = "ctrl+c"
	keyCtrlF             = "ctrl+f"
	keyCtrlD             = "ctrl+d"
	keyBack              = "q"
	keyQuit              = "Q"
	keyCommand           = ":"
	keySearch            = "/"
	keyOpen              = "o"
	keyDelete            = "D"
	keyVimDown           = "j"
	keyVimUp             = "k"
	keyArrowDown         = "down"
	keyArrowUp           = "up"
	keyGoto              = "g"
	keyBottom            = "G"
	keyCites             = "c"
	keyCitedBy           = "b"
	keyStatus            = "s"
	keySort              = "."
	keyColLeft           = "h"
	keyColRight          = "l"
	keyClassification    = "L"
	keyText              = "t"
	keyNotes             = "n"
	keyRefs              = "ctrl+r"
	keyAI                = "a"
	keyWeb               = "w"
	keyJump              = "f"
	keyFamily            = "F"
	keyProject           = "P"
	keyFirstClaim        = "1"
	keyHelp              = "?"
	keyYes               = "y"
	keyNo                = "n"
	keyNew               = "n"
	keyIgnore            = "i"
	keyUnreview          = "r"
	keyRefreshAll        = "R"
	keyEvents            = "e"
	keyInvoices          = "i"
	keyIDS               = "ctrl+i"
	keyMarkPaid          = "p"
	keyProjectInfo       = "I"
	keyAddToIDS          = "A"
	keyNoteEdit          = "N"
	keyEditAppStatus     = "s"
	keyEditSummary       = "m"
	keyEditComment       = "c"
	keyEditProjectStatus = "S"

	defaultPDFDir = "pdfs"

	jumpFallbackLabels = "asdfghjklqwertyuiopzxcvbnm"

	jumpLabelAssignee         = "a"
	jumpLabelInventors        = "i"
	jumpLabelApplication      = "A"
	jumpLabelPublication      = "p"
	jumpLabelGrant            = "g"
	jumpLabelClassification   = "k"
	jumpLabelExpiration       = "x"
	jumpLabelFirstClaim       = "1"
	jumpLabelAbstract         = "m"
	jumpLabelStoredLocal      = "l"
	jumpLabelUpdated          = "u"
	jumpLabelSource           = "h"
	jumpLabelCitation         = "c"
	jumpLabelCitedBy          = "b"
	jumpLabelCitationCount    = "c"
	jumpLabelCitedByCount     = "b"
	jumpLabelFamilyParents    = "P"
	jumpLabelFamilyChildren   = "C"
	jumpLabelNotes            = "n"
	jumpLabelLatestAssignment = "L"

	inventorJumpNumberLabels = "123456789"

	statusFilterNone = "none" // no status restriction — passes "none" to storage layer

	EmptyFilter     = ""
	EmptySortColumn = ""
	EmptySortOrder  = ""
	EmptyPrompt     = ""
	EmptyMessage    = ""
	EmptyError      = ""
	EmptyCount      = ""

	ColorTheme        = "39"  // Blue
	ColorAccent       = "205" // Pink/Magenta
	ColorAccentFamily = "213" // Bright Pink (family tree root)
	ColorSubtle       = "245" // Light Gray
	ColorHighlight    = "236" // Dark Gray (Selection)
	ColorSelection    = "61"  // Deep Blue/Purple (Multi-selection)
	ColorAltRow       = "233" // Near Black (Alternating)
	ColorSurface      = "235" // Very Dark Gray (Surface)
	ColorError        = "9"   // Red
	ColorSuccess      = "10"  // Green
	ColorWarning      = "222" // Warm Yellow
	ColorYellow       = "11"  // Bright Yellow
	ColorDisabled     = "244" // Gray
	ColorDim          = "240" // Muted Gray
	ColorDepth        = "75"  // Cyan-blue (family tree depth labels)

	ColorFamilyContinuation = "33"  // Dodger Blue
	ColorFamilyCIP          = "171" // Medium Orchid
	ColorFamilyDivisional   = "214" // Orange
	ColorFamilyPCT          = "81"  // SteelBlue/Cyan
)

const (
	noteDetailSnippetCount = 10
	noteTextareaHeight     = 8
	noteTextareaCharLimit  = 4000
	noteTextareaMinWidth   = 20

	idsNoteMaxLen   = 25
	idsNoteTruncLen = 22
)

const (
	DefaultDBPath       = "db/patentmine.db"
	DefaultLogPath      = "logs/patentmine.log"
	DefaultActivityPath = "logs/activity.jsonl"
	DefaultDBDir        = "db"
	DefaultLogDir       = "logs"

	DefaultProjectID     = "default"
	SettingLastProjectID = "last_project_id"
)

var StatusColors = map[string]string{
	domain.CitationStatusIgnored:     ColorSubtle,
	domain.CitationStatusUnderReview: "222",
	domain.CitationStatusStored:      ColorTheme,
	domain.CitationStatusCached:      ColorDim,
}

var SummaryStatusColors = map[string]string{
	domain.ProjectSummaryStatusWorkInProgress:   ColorWarning,
	domain.ProjectSummaryStatusProvisionalFiled: "81",  // Cyan-ish
	domain.ProjectSummaryStatusApplicationFiled: "147", // Lavender
	domain.ProjectSummaryStatusPublished:        ColorTheme,
	domain.ProjectSummaryStatusGranted:          ColorSuccess,
}

var SummaryStatusLabels = map[string]string{
	domain.ProjectSummaryStatusWorkInProgress:   "WIP",
	domain.ProjectSummaryStatusProvisionalFiled: "Provisional",
	domain.ProjectSummaryStatusApplicationFiled: "Filed",
	domain.ProjectSummaryStatusPublished:        "Published",
	domain.ProjectSummaryStatusGranted:          "Granted",
}

var EventTypeLabels = map[string]string{
	domain.EventTypeProvisionalFiled:  "Provisional Filed",
	domain.EventTypeApplicationFiled:  "Application Filed",
	domain.EventTypePublication:       "Publication",
	domain.EventTypeOANonFinal:        "OA Non-Final",
	domain.EventTypeOAFinal:           "OA Final",
	domain.EventTypeResponseFiled:     "Response Filed",
	domain.EventTypeRCEFiled:          "RCE Filed",
	domain.EventTypeNoticeOfAllowance: "Notice of Allowance",
	domain.EventTypeIssueFee:          "Issue Fee Paid",
	domain.EventTypeGranted:           "Patent Granted",
	domain.EventTypeMaintenance3:      "Maintenance Due (3.5yr)",
	domain.EventTypeMaintenance7:      "Maintenance Due (7.5yr)",
	domain.EventTypeMaintenance11:     "Maintenance Due (11.5yr)",
	domain.EventTypeContinuationFiled: "Continuation Filed",
	domain.EventTypeDivisionalFiled:   "Divisional Filed",
	domain.EventTypeCIPFiled:          "CIP Filed",
	domain.EventTypeAppealFiled:       "Appeal Filed",
	domain.EventTypePTABDecision:      "PTAB Decision",
	domain.EventTypeIPRFiled:          "IPR Filed",
	domain.EventTypeReexamRequested:   "Reexam Requested",
	domain.EventTypeExtensionFiled:    "Extension Filed",
	domain.EventTypeAbandonment:       "Abandonment",
	domain.EventTypeRevivalFiled:      "Revival Filed",
}

var InvoiceStatusColors = map[string]string{
	domain.InvoiceStatusOutstanding: ColorWarning,
	domain.InvoiceStatusPaid:        ColorSuccess,
	domain.InvoiceStatusOverdue:     ColorError,
	domain.InvoiceStatusDisputed:    "208", // Orange
}

var InvoiceStatusLabels = map[string]string{
	domain.InvoiceStatusOutstanding: "Outstanding",
	domain.InvoiceStatusPaid:        "Paid",
	domain.InvoiceStatusOverdue:     "Overdue",
	domain.InvoiceStatusDisputed:    "Disputed",
}

var InvoiceDirectionLabels = map[string]string{
	domain.InvoiceDirectionToFirm:   "→ Firm",
	domain.InvoiceDirectionFromFirm: "← Firm",
}

var EventTypeColors = map[string]string{
	domain.EventTypeProvisionalFiled:  "81",         // Cyan
	domain.EventTypeApplicationFiled:  "147",        // Lavender
	domain.EventTypePublication:       ColorTheme,   // Blue
	domain.EventTypeOANonFinal:        ColorWarning, // Yellow
	domain.EventTypeOAFinal:           "208",        // Orange
	domain.EventTypeResponseFiled:     "118",        // Green
	domain.EventTypeRCEFiled:          "214",        // Gold
	domain.EventTypeNoticeOfAllowance: ColorSuccess, // Green
	domain.EventTypeIssueFee:          ColorSuccess,
	domain.EventTypeGranted:           ColorSuccess,
	domain.EventTypeMaintenance3:      ColorWarning,
	domain.EventTypeMaintenance7:      ColorWarning,
	domain.EventTypeMaintenance11:     ColorWarning,
	domain.EventTypeContinuationFiled: "81",
	domain.EventTypeDivisionalFiled:   "81",
	domain.EventTypeCIPFiled:          "81",
	domain.EventTypeAppealFiled:       "208",
	domain.EventTypePTABDecision:      "147",
	domain.EventTypeIPRFiled:          ColorError,
	domain.EventTypeReexamRequested:   ColorError,
	domain.EventTypeExtensionFiled:    ColorSubtle,
	domain.EventTypeAbandonment:       ColorError,
	domain.EventTypeRevivalFiled:      ColorWarning,
}
