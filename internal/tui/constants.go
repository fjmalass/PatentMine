package tui

import "patentmine/internal/domain"

const (
	commandSearch     = "search"
	commandOpen       = "open"
	commandAdd        = "add"
	commandImport     = "import"
	commandRefresh    = "refresh"
	commandCitedBy    = "citedby"
	commandClassification        = "cpc"
	commandText       = "text"
	commandRefs       = "refs"
	commandNotes      = "notes"
	commandSummarize  = "summarize"
	commandCompare    = "compare"
	commandRef        = "ref"
	commandReview     = "review"
	commandIgnored    = "ignored"
	commandUnderReview = "under-review"
	commandHelp       = "help"
	commandHelpShort  = "h"
	commandBrowser    = "browser"
	commandWeb        = "web"

	refActionAdd    = "add"
	refActionExport = "export"

	refreshTargetAll       = "all"
	refreshTargetCitations = "citations"
	refreshTargetCitedBy   = "citedby"

	importActionAdded     = "added"
	importActionImported  = "imported"
	importActionOpened    = "opened"
	importActionRefreshed = "refreshed"

	keyEnter     = "enter"
	keyEsc       = "esc"
	keyCtrlC     = "ctrl+c"
	keyCtrlF     = "ctrl+f"
	keyCtrlD     = "ctrl+d"
	keyQuit      = "q"
	keyCommand   = ":"
	keySearch    = "/"
	keyOpen      = "o"
	keyDelete    = "D"
	keyDown      = "j"
	keyArrowDown = "down"
	keyUp        = "k"
	keyArrowUp   = "up"
	keyCites     = "c"
	keyCitedBy   = "b"
	keyClassification       = "p"
	keyText      = "t"
	keyNotes     = "n"
	keyRefs      = "r"
	keyAI        = "a"
	keyWeb       = "w"
	keyJump      = "f"
	keyHelp      = "?"
	keyYes       = "y"
	keyNo        = "n"
	keyIgnore    = "i"
	keyUnreview  = "u"

	defaultPDFDir = "pdfs"

	jumpFallbackLabels = "asdfghjklqwertyuiopzxcvbnm"

	jumpLabelAssignee      = "a"
	jumpLabelInventors     = "i"
	jumpLabelPublication   = "p"
	jumpLabelGrant         = "g"
	jumpLabelClassification           = "p"
	jumpLabelExpiration    = "x"
	jumpLabelStoredLocal   = "l"
	jumpLabelSource        = "h"
	jumpLabelCitation      = "c"
	jumpLabelCitedBy       = "b"
	jumpLabelCitationCount = "c"
	jumpLabelCitedByCount  = "b"

	inventorJumpNumberLabels = "123456789"
)

var StatusColors = map[string]string{
	domain.CitationStatusIgnored:    "244", // Gray
	domain.CitationStatusUnderReview: "222", // Yellow
	domain.CitationStatusStored:     "39",  // Blue (Theme color)
}
