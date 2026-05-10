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

type HelpSection struct {
	Title   string
	Entries []HelpEntry
}

var commandHelpSections = []HelpSection{
	{
		Title: "Patents",
		Entries: []HelpEntry{
			{Usage: "/term", Description: TextHelpFilterPatents},
			{Usage: keyCommand + commandAdd + " US11611785B2", Description: TextHelpAddPatent},
			{Usage: keyCommand + commandImport + " <url>", Description: TextHelpImportPatent},
			{Usage: keyCommand + commandOpen + " US11611785B2", Description: TextHelpOpenPatent},
			{Usage: keyCommand + commandRefresh + " [" + refreshTargetCitedBy + "|" + refreshTargetCitations + "|all]", Description: TextHelpRefreshCitations},
			{Usage: keyCommand + commandRefreshRefsDetails, Description: TextHelpRefreshDetails},
			{Usage: keyCommand + commandClassFilter + " <cpc> [&& <cpc2> | || <cpc2>]", Description: TextHelpClass},
			{Usage: keyCommand + commandInventorFilter + " <name>", Description: TextHelpInventorFilter},
			{Usage: keyCommand + commandStatusFilter + " <stored|ignored|under-review|none>", Description: TextHelpStatusFilter},
			{Usage: keyCommand + commandFamily + " parent|child <number> [type]", Description: TextHelpFamilyAdd},
			{Usage: keyCommand + commandFamily + " remove <number>", Description: TextHelpFamilyRemove},
			{Usage: keyFamily + " (from any patent view)", Description: TextHelpFamilyView},
			{Usage: keyCommand + commandSort + " <col>[,<col2>] [asc|desc]", Description: TextHelpSort},
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
			{Usage: keyCommand + commandBrowser, Description: TextHelpOpenBrowser},
		},
	},
	{
		Title: "Review",
		Entries: []HelpEntry{
			{Usage: keyCommand + commandIgnored, Description: TextHelpReviewIgnored},
			{Usage: keyCommand + commandUnderReview, Description: TextHelpReviewUnderReview},
		},
	},
	{
		Title: "Project",
		Entries: []HelpEntry{
			{Usage: keyCommand + commandProject + " id", Description: TextHelpProjectID},
			{Usage: keyCommand + commandProject + " list", Description: TextHelpProjectList},
			{Usage: keyCommand + commandProject + " create <id> [name]", Description: TextHelpProjectCreate},
			{Usage: keyCommand + commandProject + " switch <id>", Description: TextHelpProjectSwitch},
			{Usage: keyCommand + commandProject + " add <id>", Description: TextHelpProjectAdd},
			{Usage: keyCommand + commandProject + " status <active|archived>", Description: TextHelpProjectStatus},
			{Usage: keyCommand + commandProject + " summary-status <stage>", Description: TextHelpProjectSummaryStatus},
			{Usage: keyCommand + commandProject + " summary <text>", Description: TextHelpProjectSummary},
			{Usage: keyCommand + commandProject + " comment <text>", Description: TextHelpProjectComment},
			{Usage: keyCommand + commandProject + " delete <id>", Description: TextHelpProjectDelete},
		},
	},
	{
		Title: "Prosecution & Invoices",
		Entries: []HelpEntry{
			{Usage: keyCommand + commandProject + " event <type> [date YYYY-MM-DD] [due YYYY-MM-DD] [ref <ref>] [note <text>]", Description: TextHelpProjectEvent},
			{Usage: keyCommand + commandProject + " events", Description: TextHelpProjectEvents},
			{Usage: keyCommand + commandProject + " invoice <amount> [currency USD] [direction to-firm|from-firm] [date YYYY-MM-DD] [due YYYY-MM-DD] [firm <name>] [ref <n>] [note <text>]", Description: TextHelpProjectInvoice},
			{Usage: keyCommand + commandProject + " invoices", Description: TextHelpProjectInvoices},
			{Usage: keyCommand + commandProject + " ids add <patent-number> [note <text>]", Description: TextHelpProjectIDSAdd},
			{Usage: keyCommand + commandProject + " ids", Description: TextHelpProjectIDS},
			{Usage: keyCommand + commandProject + " export ids [filename]", Description: TextHelpExportIDS},
			{Usage: keyCommand + commandProject + " export status [filename]", Description: TextHelpExportStatus},
			{Usage: keyCommand + commandProject + " export state [stored|ignored|under-review|all|none] [file]", Description: TextHelpExportState},
			{Usage: keyProjectInfo + " (from any patent view)", Description: TextHelpProjectInfo},
		},
	},
	{
		Title: "General",
		Entries: []HelpEntry{
			{Usage: keyCommand + commandHelp, Description: TextHelpShowHelp},
		},
	},
}

var shortcutHelp = []HelpEntry{
	{Usage: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
	{Usage: keyEnter + " or " + keyOpen, Description: TextHelpOpenSelected},
	{Usage: keyDelete, Description: TextHelpDeletePatent},
	{Usage: keyProject, Description: TextHelpJumpProject},
	{Usage: keyProjectInfo, Description: TextHelpProjectInfo},
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
	":import https://patents.google.com/patent/US11611785B2/en",
	":project event provisional-filed date 2024-03-01",
	":project invoice 5000 firm ACME date 2024-04-01 due 2024-05-01",
}

func RenderHelp(text TextCatalog) string {
	var b strings.Builder
	for _, section := range commandHelpSections {
		b.WriteString(section.Title + "\n\n")
		writeHelpEntries(&b, section.Entries, text)
		b.WriteString("\n")
	}
	b.WriteString(text.T(TextHelpShortcuts) + "\n\n")
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
			{Usage: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
			{Usage: keyEnter + " or " + keyOpen, Description: TextHelpOpenSelected},
			{Usage: keySearch + "term", Description: TextHelpFilterPatents},
			{Usage: keyCites, Description: TextHelpJumpCitations},
			{Usage: keyCitedBy, Description: TextHelpJumpCitedBy},
			{Usage: keyClassification, Description: TextHelpJumpClassification},
			{Usage: keyProjectInfo, Description: TextHelpProjectInfo},
			{Usage: keyHelp, Description: TextHelpShortcutShowHelp},
			{Usage: keyQuit, Description: TextHelpBackOrQuit},
		}
	case viewDetail:
		return []HelpEntry{
			{Usage: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
			{Usage: keyEnter + " or " + keyOpen, Description: TextHelpOpenSelected},
			{Usage: keyCites, Description: TextHelpJumpCitations},
			{Usage: keyCitedBy, Description: TextHelpJumpCitedBy},
			{Usage: keyClassification, Description: TextHelpJumpClassification},
			{Usage: keyProjectInfo, Description: TextHelpProjectInfo},
			{Usage: keyHelp, Description: TextHelpShortcutShowHelp},
			{Usage: keyQuit, Description: TextHelpBackOrQuit},
		}
	case viewCites, viewCitedBy:
		return []HelpEntry{
			{Usage: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
			{Usage: "10" + keyVimDown + "/" + "10" + keyVimUp, Description: TextHelpMoveList},
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
			{Usage: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
			{Usage: "10" + keyVimDown + "/" + "10" + keyVimUp, Description: TextHelpMoveList},
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
			{Usage: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
			{Usage: "10" + keyVimDown + "/" + "10" + keyVimUp, Description: TextHelpMoveList},
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
			{Usage: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
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
		{Usage: keyCommand + commandRefreshRefsDetails, Description: TextHelpRefreshDetails},
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
