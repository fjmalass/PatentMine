package tui

import "patentmine/internal/domain"

const (
	commandSearch          = "search"
	commandOpen            = "open"
	commandAdd             = "add"
	commandImport          = "import"
	commandRefresh         = "refresh"
	commandRefreshDetails  = "refresh-details"
	commandCitedBy         = "citedby"
	commandClassification  = "cpc"
	commandText            = "text"
	commandRefs            = "refs"
	commandNotes           = "notes"
	commandSummarize       = "summarize"
	commandCompare         = "compare"
	commandRef             = "ref"
	commandReview          = "review"
	commandIgnored         = "ignored"
	commandUnderReview     = "under-review"
	commandHelp            = "help"
	commandHelpShort       = "h"
	commandBrowser         = "browser"
	commandWeb             = "web"
	commandProject         = "project"
	commandSort            = "sort"
	commandClass           = "class"

	refActionAdd    = "add"
	refActionExport = "export"

	refreshTargetAll       = "all"
	refreshTargetCitations = "citations"
	refreshTargetCitedBy   = "citedby"

	importActionAdded     = "added"
	importActionImported  = "imported"
	importActionOpened    = "opened"
	importActionRefreshed = "refreshed"

	keyEnter          = "enter"
	keyEsc            = "esc"
	keyCtrlC          = "ctrl+c"
	keyCtrlF          = "ctrl+f"
	keyCtrlD          = "ctrl+d"
	keyQuit           = "q"
	keyCommand        = ":"
	keySearch         = "/"
	keyOpen           = "o"
	keyDelete         = "D"
	keyVimDown        = "j"
	keyVimUp          = "k"
	keyArrowDown      = "down"
	keyArrowUp        = "up"
	keyGoto           = "g"
	keyCites          = "c"
	keyCitedBy        = "b"
	keyClassification = "p"
	keyText           = "t"
	keyNotes          = "n"
	keyRefs           = "r"
	keyAI             = "a"
	keyWeb            = "w"
	keyJump           = "f"
	keyProject        = "P"
	keyHelp           = "?"
	keyYes            = "y"
	keyNo             = "n"
	keyNew            = "n"
	keyIgnore         = "i"
	keyUnreview       = "u"

	defaultPDFDir = "pdfs"

	jumpFallbackLabels = "asdfghjklqwertyuiopzxcvbnm"

	jumpLabelAssignee       = "a"
	jumpLabelInventors      = "i"
	jumpLabelPublication    = "p"
	jumpLabelGrant          = "g"
	jumpLabelClassification = "k"
	jumpLabelExpiration     = "x"
	jumpLabelStoredLocal    = "l"
	jumpLabelSource         = "h"
	jumpLabelCitation       = "c"
	jumpLabelCitedBy        = "b"
	jumpLabelCitationCount  = "c"
	jumpLabelCitedByCount   = "b"

	inventorJumpNumberLabels = "123456789"

	EmptyFilter = ""
	EmptySortColumn = ""
	EmptySortOrder = ""

	ColorTheme     = "39"  // Blue
	ColorAccent    = "205" // Pink/Magenta
	ColorSubtle    = "245" // Light Gray
	ColorHighlight = "236" // Dark Gray (Selection)
	ColorAltRow    = "233" // Near Black (Alternating)
	ColorSurface   = "235" // Very Dark Gray (Surface)
	ColorError     = "9"   // Red
	ColorSuccess   = "10"  // Green
	ColorWarning   = "222" // Warm Yellow
	ColorYellow    = "11"  // Bright Yellow
	ColorDisabled  = "244" // Gray
	ColorDim       = "240" // Muted Gray
)

const (
	DefaultDBPath  = "db/patentmine.db"
	DefaultLogPath = "logs/patentmine.log"
	DefaultDBDir   = "db"
	DefaultLogDir  = "logs"
)

var StatusColors = map[string]string{
	domain.CitationStatusIgnored:     ColorSubtle,
	domain.CitationStatusUnderReview: "222", // Keep yellow for review
	domain.CitationStatusStored:      ColorTheme,
}
