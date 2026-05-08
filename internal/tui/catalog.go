package tui

import (
	"fmt"
	"strings"

	"patentmine/internal/domain"
)

type HelpEntry struct {
	Usage       string
	Description TextKey
}

var commandHelpEntries = []HelpEntry{
	{Usage: "/term", Description: TextHelpFilterPatents},
	{Usage: keyCommand + commandAdd + " US11611785B2", Description: TextHelpAddPatent},
	{Usage: keyCommand + commandImport + " <google-patents-url>", Description: TextHelpImportPatent},
	{Usage: keyCommand + commandRefresh + " " + refreshTargetCitedBy, Description: TextHelpRefreshCitedBy},
	{Usage: keyCommand + commandRefresh + " " + refreshTargetCitations, Description: TextHelpRefreshCitations},
	{Usage: keyCommand + commandRefreshDetails, Description: TextHelpRefreshDetails},
	{Usage: keyCommand + commandOpen + " US11611785B2", Description: TextHelpOpenPatent},
	{Usage: keyCommand + domain.RelationCites, Description: TextHelpShowCites},
	{Usage: keyCommand + commandCitedBy, Description: TextHelpShowCitedBy},
	{Usage: keyCommand + commandClassification, Description: TextHelpShowClassification},
	{Usage: keyCommand + commandText, Description: TextHelpShowText},
	{Usage: keyCommand + commandRefs, Description: TextHelpShowRefs},
	{Usage: keyCommand + commandNotes, Description: TextHelpShowNotes},
	{Usage: keyCommand + commandSummarize, Description: TextHelpSummarize},
	{Usage: keyCommand + commandCompare + " US11611785B2", Description: TextHelpCompare},
	{Usage: keyCommand + commandRef + " " + refActionAdd, Description: TextHelpRefAdd},
	{Usage: keyCommand + commandRef + " " + refActionExport, Description: TextHelpRefExport},
	{Usage: keyCommand + commandIgnored, Description: TextHelpReviewIgnored},
	{Usage: keyCommand + commandUnderReview, Description: TextHelpReviewUnderReview},
	{Usage: keyCommand + commandBrowser, Description: TextHelpOpenBrowser},
	{Usage: keyCommand + commandHelp, Description: TextHelpShowHelp},
}

var shortcutHelp = []HelpEntry{
	{Usage: keyDown + "/" + keyUp + " or arrow keys", Description: TextHelpMoveList},
	{Usage: keyEnter + " or " + keyOpen, Description: TextHelpOpenSelected},
	{Usage: keyDelete, Description: TextHelpDeletePatent},
	{Usage: keyCites, Description: TextHelpJumpCitations},
	{Usage: keyCitedBy, Description: TextHelpJumpCitedBy},
	{Usage: keyClassification, Description: TextHelpJumpClassification},
	{Usage: keyText, Description: TextHelpJumpText},
	{Usage: keyNotes, Description: TextHelpJumpNotes},
	{Usage: keyRefs, Description: TextHelpJumpRefs},
	{Usage: keyAI, Description: TextHelpJumpAI},
	{Usage: keyWeb, Description: TextHelpJumpWeb},
	{Usage: keyHelp, Description: TextHelpShortcutShowHelp},
	{Usage: keyQuit, Description: TextHelpBackOrQuit},
}

var helpExamples = []string{
	":add US11611785B2",
	":refresh citedby",
	":import https://patents.google.com/patent/US11611785B2/en?oq=US11611785B2+",
}

func RenderHelp(text TextCatalog) string {
	var b strings.Builder
	b.WriteString(text.T(TextHelpCommands) + "\n\n")
	writeHelpEntries(&b, commandHelpEntries, text)
	b.WriteString("\n" + text.T(TextHelpShortcuts) + "\n\n")
	writeHelpEntries(&b, shortcutHelp, text)
	b.WriteString("\n" + text.T(TextHelpExamples) + "\n\n")
	for _, example := range helpExamples {
		b.WriteString(example + "\n")
	}
	return b.String()
}

func RenderContextHelp(text TextCatalog, mode viewMode) string {
	var b strings.Builder
	b.WriteString(text.T(TextHelpPopupTitle) + " · " + screenTitleForMode(mode) + "\n\n")
	b.WriteString(text.T(TextHelpScreen) + "\n\n")
	writeHelpEntries(&b, contextHelpEntries(mode), text)
	b.WriteString("\n" + text.T(TextHelpGlobal) + "\n\n")
	writeHelpEntries(&b, globalHelpEntries(), text)
	return b.String()
}

func contextHelpEntries(mode viewMode) []HelpEntry {
	switch mode {
	case viewList:
		return []HelpEntry{
			{Usage: keyDown + "/" + keyUp + " or arrow keys", Description: TextHelpMoveList},
			{Usage: keyEnter + " or " + keyOpen, Description: TextHelpOpenSelected},
			{Usage: keySearch + "term", Description: TextHelpFilterPatents},
			{Usage: keyCites, Description: TextHelpJumpCitations},
			{Usage: keyCitedBy, Description: TextHelpJumpCitedBy},
			{Usage: keyClassification, Description: TextHelpJumpClassification},
			{Usage: keyHelp, Description: TextHelpShortcutShowHelp},
			{Usage: keyQuit, Description: TextHelpBackOrQuit},
		}
	case viewDetail:
		return []HelpEntry{
			{Usage: keyDown + "/" + keyUp + " or arrow keys", Description: TextHelpMoveList},
			{Usage: keyEnter + " or " + keyOpen, Description: TextHelpOpenSelected},
			{Usage: keyCites, Description: TextHelpJumpCitations},
			{Usage: keyCitedBy, Description: TextHelpJumpCitedBy},
			{Usage: keyClassification, Description: TextHelpJumpClassification},
			{Usage: keyHelp, Description: TextHelpShortcutShowHelp},
			{Usage: keyQuit, Description: TextHelpBackOrQuit},
		}
	case viewCites, viewCitedBy:
		return []HelpEntry{
			{Usage: keyDown + "/" + keyUp + " or arrow keys", Description: TextHelpMoveList},
			{Usage: "10" + keyDown + "/" + "10" + keyUp, Description: TextHelpMoveList},
			{Usage: "10" + keyGoto, Description: TextHelpMoveList},
			{Usage: keyEnter + " or " + keyOpen, Description: TextHelpOpenSelected},
			{Usage: keyYes, Description: TextHelpRefAdd},
			{Usage: keyIgnore, Description: TextHelpReviewIgnored},
			{Usage: keyUnreview, Description: TextHelpReviewUnderReview},
			{Usage: keyCtrlF + "/" + keyCtrlD, Description: TextHelpJumpViews},
			{Usage: keyHelp, Description: TextHelpShortcutShowHelp},
			{Usage: keyQuit, Description: TextHelpBackOrQuit},
		}
	case viewReview:
		return []HelpEntry{
			{Usage: keyDown + "/" + keyUp + " or arrow keys", Description: TextHelpMoveList},
			{Usage: "10" + keyDown + "/" + "10" + keyUp, Description: TextHelpMoveList},
			{Usage: "10" + keyGoto, Description: TextHelpMoveList},
			{Usage: keyEnter + " or " + keyOpen, Description: TextHelpOpenSelected},
			{Usage: keyYes, Description: TextHelpRefAdd},
			{Usage: keyIgnore, Description: TextHelpReviewIgnored},
			{Usage: keyUnreview, Description: TextHelpReviewUnderReview},
			{Usage: keyWeb, Description: TextHelpOpenBrowser},
			{Usage: keyCtrlF + "/" + keyCtrlD, Description: TextHelpJumpViews},
			{Usage: keyHelp, Description: TextHelpShortcutShowHelp},
			{Usage: keyQuit, Description: TextHelpBackOrQuit},
		}
	case viewClassifications:
		return []HelpEntry{
			{Usage: keyDown + "/" + keyUp + " or arrow keys", Description: TextHelpMoveList},
			{Usage: "10" + keyDown + "/" + "10" + keyUp, Description: TextHelpMoveList},
			{Usage: "10" + keyGoto, Description: TextHelpMoveList},
			{Usage: keyEnter + " or " + keyOpen, Description: TextHelpOpenSelected},
			{Usage: keyCtrlF + "/" + keyCtrlD, Description: TextHelpJumpViews},
			{Usage: keyHelp, Description: TextHelpShortcutShowHelp},
			{Usage: keyQuit, Description: TextHelpBackOrQuit},
		}
	case viewPreview:
		return []HelpEntry{
			{Usage: keyYes, Description: TextHelpRefAdd},
			{Usage: keyIgnore, Description: TextHelpReviewIgnored},
			{Usage: keyUnreview, Description: TextHelpReviewUnderReview},
			{Usage: keyNo + "/" + keyEsc, Description: TextHelpBackOrQuit},
			{Usage: keyHelp, Description: TextHelpShortcutShowHelp},
		}
	case viewInventors:
		return []HelpEntry{
			{Usage: keyDown + "/" + keyUp + " or arrow keys", Description: TextHelpMoveList},
			{Usage: keyEnter + " or " + keyOpen, Description: TextHelpOpenSelected},
			{Usage: keyHelp, Description: TextHelpShortcutShowHelp},
			{Usage: keyQuit, Description: TextHelpBackOrQuit},
		}
	case viewText, viewRefs, viewNotes, viewAI:
		return []HelpEntry{
			{Usage: keyHelp, Description: TextHelpShortcutShowHelp},
			{Usage: keyQuit, Description: TextHelpBackOrQuit},
		}
	default:
		return []HelpEntry{
			{Usage: keyHelp, Description: TextHelpShortcutShowHelp},
			{Usage: keyQuit, Description: TextHelpBackOrQuit},
		}
	}
}

func globalHelpEntries() []HelpEntry {
	return []HelpEntry{
		{Usage: keyCommand + commandHelp, Description: TextHelpShowHelp},
		{Usage: keyCommand + commandRefresh + " " + refreshTargetCitedBy, Description: TextHelpRefreshCitedBy},
		{Usage: keyCommand + commandRefresh + " " + refreshTargetCitations, Description: TextHelpRefreshCitations},
		{Usage: keyCommand + commandRefreshDetails, Description: TextHelpRefreshDetails},
		{Usage: keyCommand + commandBrowser, Description: TextHelpOpenBrowser},
	}
}

func writeHelpEntries(b *strings.Builder, entries []HelpEntry, text TextCatalog) {
	width := 0
	for _, entry := range entries {
		if len(entry.Usage) > width {
			width = len(entry.Usage)
		}
	}
	for _, entry := range entries {
		b.WriteString(fmt.Sprintf("%-*s  %s\n", width, entry.Usage, text.T(entry.Description)))
	}
}
