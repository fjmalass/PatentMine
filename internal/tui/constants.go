package tui

import (
	"patentmine/internal/domain"
	"patentmine/internal/storage"
)

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
	commandDate               = "date"
	commandNum                = "num"
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
	commandIDS                = "ids"
	commandVersion            = "version"
	commandExit               = "exit"
	commandFamilyPull         = "pull"
	commandTag                = "tag"

	// :tag subcommands
	tagSubAdd    = "add"
	tagSubList   = "list"
	tagSubDelete = "delete"
	tagSubRename = "rename"
	tagSubColor  = "color"
	tagSubFilter = "filter"
	tagSubHelp   = "help"

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

	// :ids edit subcommands
	idsEditSubNote     = "note"
	idsEditSubKind     = "kind"
	idsEditSubCountry  = "country"
	idsEditSubPassages = "passages"
	idsEditSubFull     = "full"

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
	keyTag               = "T"
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
	keyIDS               = "I"
	keyMarkPaid          = "p"
	keyProjectInfo       = "i"
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
	jumpLabelImportSource     = "v"
	jumpLabelTags             = "t"

	inventorJumpNumberLabels = "123456789"

	statusFilterNone = storage.StatusFilterNone // no status restriction

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
	ColorBlack        = "0"   // Black
	ColorWhite        = "255" // White
	ColorLavender     = "147" // Lavender
	ColorOrange       = "208" // Orange
	ColorLime         = "118" // Lime Green
	ColorCyan         = "81"  // Cyan
	ColorGold         = "214" // Gold

	// Mode Theme Colors
	ColorThemeList            = "39"  // Blue
	ColorThemeDetail          = "51"  // Cyan
	ColorThemeCitations       = "214" // Orange
	ColorThemeCitedBy         = "40"  // Green
	ColorThemeClassifications = "170" // Pink/Magenta
	ColorThemeNotes           = "220" // Yellow
	ColorThemeReferences      = "27"  // Dark Blue
	ColorThemeAI              = "141" // Purple/Violet
	ColorThemeHelp            = "245" // Light Gray
	ColorThemePreview         = "81"  // SteelBlue
	ColorThemeReview          = "202" // Orange/Red
	ColorThemeDelete          = "196" // Red
	ColorThemeFamily          = "213" // Purple
	ColorThemeIDS             = "75"  // Light Blue
	ColorThemeTags            = "118" // Lime Green
	ColorThemeText            = "250" // White/Light Gray

	// Overlay dimensions
	OverlayDefaultRatio     = 0.8
	OverlayExpandedRatio    = 0.85
	OverlayFallbackWidth    = 76
	OverlayMinWidth         = 44
	OverlayAbsoluteMinWidth = 20

	// Activity actions
	ActivityPatentImport   = "patent.import"
	ActivityPatentDelete   = "patent.delete"
	ActivityPatentStatus   = "patent.status"
	ActivityPatentDate     = "patent.date"
	ActivityPatentNumber   = "patent.number"
	ActivityPatentRefresh  = "patent.refresh"
	ActivityNoteAdd        = "note.add"
	ActivityIDSAdd         = "ids.add"
	ActivityIDSRemove      = "ids.remove"
	ActivityIDSStatus      = "ids.status"
	ActivityCitationStore  = "citation.store"
	ActivityCitationStatus = "citation.status"
	ActivityRefAdd         = "ref.add"
	ActivityBulkPrefix     = "bulk."
	ActivityFamilyRefresh  = "family.refresh"

	// Import sources
	ImportSourceUSPTO  = "uspto"
	ImportSourceGoogle = "google"

	// Family relation types
	FamilyRelationContinuation = "continuation"
	FamilyRelationDivisional   = "divisional"
	FamilyRelationCIP          = "cip"
	FamilyRelationPCT          = "pct"

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

	FamilyNodeStatusIconUnloadedCircle = "◌"
	FamilyNodeStatusIconUnloadedDots   = "…"
	FamilyNodeStatusIconUnloadedBlock  = "⊘"
	FamilyNodeStatusIconUnloadedOpen   = "○"
	FamilyNodeStatusIconUnloadedWave   = "⌁"
	FamilyNodeStatusIconUnloaded       = FamilyNodeStatusIconUnloadedCircle
	FamilyNodeStatusLabelUnloaded      = FamilyNodeStatusIconUnloaded + " unloaded"
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
	domain.CitationStatusUnderReview: ColorWarning,
	domain.CitationStatusStored:      ColorTheme,
	domain.CitationStatusCached:      ColorDim,
}

var SummaryStatusColors = map[string]string{
	domain.ProjectSummaryStatusWorkInProgress:   ColorWarning,
	domain.ProjectSummaryStatusProvisionalFiled: ColorCyan,
	domain.ProjectSummaryStatusApplicationFiled: ColorLavender,
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
	domain.InvoiceStatusDisputed:    ColorOrange,
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
	domain.EventTypeProvisionalFiled:  ColorCyan,
	domain.EventTypeApplicationFiled:  ColorLavender,
	domain.EventTypePublication:       ColorTheme,   // Blue
	domain.EventTypeOANonFinal:        ColorWarning, // Yellow
	domain.EventTypeOAFinal:           ColorOrange,  // Orange
	domain.EventTypeResponseFiled:     ColorLime,    // Green
	domain.EventTypeRCEFiled:          ColorGold,    // Gold
	domain.EventTypeNoticeOfAllowance: ColorSuccess, // Green
	domain.EventTypeIssueFee:          ColorSuccess,
	domain.EventTypeGranted:           ColorSuccess,
	domain.EventTypeMaintenance3:      ColorWarning,
	domain.EventTypeMaintenance7:      ColorWarning,
	domain.EventTypeMaintenance11:     ColorWarning,
	domain.EventTypeContinuationFiled: ColorCyan,
	domain.EventTypeDivisionalFiled:   ColorCyan,
	domain.EventTypeCIPFiled:          ColorCyan,
	domain.EventTypeAppealFiled:       ColorOrange,
	domain.EventTypePTABDecision:      ColorLavender,
	domain.EventTypeIPRFiled:          ColorError,
	domain.EventTypeReexamRequested:   ColorError,
	domain.EventTypeExtensionFiled:    ColorSubtle,
	domain.EventTypeAbandonment:       ColorError,
	domain.EventTypeRevivalFiled:      ColorWarning,
}
