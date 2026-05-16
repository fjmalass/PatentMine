package tui

import (
	"patentmine/internal/domain"
	"patentmine/internal/storage"
)

const (
	defaultPDFDir = "pdfs"

	reviewStateFilterNone = storage.ReviewStateFilterNone // no review state restriction

	EmptyFilter     = ""
	EmptySortColumn = ""
	EmptySortOrder  = ""
	EmptyPrompt     = ""
	EmptyMessage    = ""
	EmptyError      = ""
	EmptyCount      = ""

	filterLabelState          = "state: "
	filterLabelSearch         = "search: "
	filterLabelClassification = "classification: "
	filterLabelCountry        = "country: "
	filterLabelTag            = "tag: "
	filterLabelTitle          = "title: "

	// UI separators
	sepBullet            = " · "  // inline separator used in titles and headers
	sepFilterBar         = "  · " // status-bar prefix before filter labels
	sepBreadcrumb        = " › "
	sepBreadcrumbEllipsis = "… › "
	sepRuleChar          = "─"

	// Date format layouts (Go reference time)
	dateFmtDate     = "2006-01-02"
	dateFmtDateTime = "2006-01-02 15:04"
	dateFmtYear     = "2006"

	// Mode indicator labels
	visualModeLabel = " VISUAL "

	// Input char limits
	commandInputCharLimit = 512
	dateInputCharLimit    = 10

	// Breadcrumb limit
	maxBreadcrumbs = 3

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
	ActivityPatentImport        = "patent.import"
	ActivityPatentDelete        = "patent.delete"
	ActivityPatentReviewState   = "patent.review_state"
	ActivityPatentDate          = "patent.date"
	ActivityPatentNumber        = "patent.number"
	ActivityPatentRefresh       = "patent.refresh"
	ActivityNoteAdd             = "note.add"
	ActivityIDSAdd              = "ids.add"
	ActivityIDSRemove           = "ids.remove"
	ActivityIDSStatus           = "ids.status"
	ActivityCitationStore       = "citation.store"
	ActivityCitationReviewState = "citation.review_state"
	ActivityRefAdd              = "ref.add"
	ActivityBulkPrefix          = "bulk."
	ActivityFamilyRefresh       = "family.refresh"
	ActivityTagApply            = "tag.apply"
	ActivityTagRemove           = "tag.remove"
	ActivityTagDelete           = "tag.delete"
	ActivityFilterCountry       = "filter.country"

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

// keySpec pairs a normal-mode keyboard shortcut with its jump-mode hint label.
// key  = letter pressed in normal mode to trigger the action
// jump = letter shown as a hint when jump mode (f) is active
// Use ks*.key in switch statements; ks*.jump when assigning to detailField or listColumn.
type keySpec struct {
	key  string
	jump string
}

// Features that have both a normal-mode key binding and a jump-mode hint label.
var (
	ksNotes     = keySpec{key: keyNotes, jump: keyNotes}
	ksCitations = keySpec{key: keyCites, jump: keyCites}
	ksCitedBy   = keySpec{key: keyCitedBy, jump: keyCitedBy}
	ksTags      = keySpec{key: keyTag, jump: keyTag}
)

const (
	noteDetailSnippetCount = 10
	noteTextareaHeight     = 8
	noteTextareaCharLimit  = 4000
	noteTextareaMinWidth   = 20

	idsNoteMaxLen   = 25
	idsNoteTruncLen = 22

	IDSIconInFull = "⊛" // icon shown when an IDS entry is cited in its entirety

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

var ReviewStateColors = map[string]string{
	domain.ReviewStateIgnored:     ColorSubtle,
	domain.ReviewStateUnderReview: ColorWarning,
	domain.ReviewStateStored:      ColorTheme,
	domain.ReviewStateCached:      ColorDim,
}

var ReviewStateLabels = map[string]string{
	domain.ReviewStateStored:      "stored",
	domain.ReviewStateUnderReview: "under review",
	domain.ReviewStateIgnored:     "ignored",
	domain.ReviewStateCached:      "cached",
	reviewStateFilterNone:         "all",
}

type classificationDetailRow int

const (
	classDetailRowTotal classificationDetailRow = iota
	classDetailRowStored
	classDetailRowUnderReview
	classDetailRowIgnored
	classDetailRowCached
	classDetailRowCount // sentinel — number of rows
)

var classDetailRowReviewState = map[classificationDetailRow]string{
	classDetailRowTotal:       reviewStateFilterNone,
	classDetailRowStored:      domain.ReviewStateStored,
	classDetailRowUnderReview: domain.ReviewStateUnderReview,
	classDetailRowIgnored:     domain.ReviewStateIgnored,
	classDetailRowCached:      domain.ReviewStateCached,
}

var classDetailRowLabel = map[classificationDetailRow]string{
	classDetailRowTotal:       "Total",
	classDetailRowStored:      "Stored",
	classDetailRowUnderReview: "Under Review",
	classDetailRowIgnored:     "Ignored",
	classDetailRowCached:      "Cached",
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
