package tui

import (
	"fmt"
	"strings"
)

type HelpEntry struct {
	Key         string  // keyboard shortcut(s), e.g. "r", "j/k", "F"
	Command     string  // colon command syntax, e.g. ":refresh citations [details]"
	Description TextKey // text key for description
}

type HelpSection struct {
	Title   string
	Entries []HelpEntry
}

var allHelpSections = []HelpSection{
	{
		Title: "Navigation",
		Entries: []HelpEntry{
			{Key: "j/k / ↓/↑", Description: TextHelpMoveList},
			{Key: "enter / o", Description: TextHelpOpenSelected},
			{Key: "esc / q", Description: TextHelpBackOrQuit},
			{Key: keyQuit, Description: TextHelpQuitApp},
			{Key: "?", Description: TextHelpShortcutShowHelp},
			{Key: "ctrl+f / ctrl+d", Description: TextHelpJumpViews},
		},
	},
	{
		Title: "Patents",
		Entries: []HelpEntry{
			{Command: ":add US11611785B2", Description: TextHelpAddPatent},
			{Command: ":import <url>", Description: TextHelpImportPatent},
			{Command: ":open US11611785B2", Description: TextHelpOpenPatent},
			{Key: keyStatus, Description: TextHelpCyclePatentStatus},
			{Key: keyDelete, Description: TextHelpDeletePatent},
			{Key: keySearch, Command: "/term", Description: TextHelpFilterPatents},
			{Key: keyWeb, Description: TextHelpJumpWeb},
		},
	},
	{
		Title: "Refresh & Citations",
		Entries: []HelpEntry{
			{Key: "ctrl+r", Command: ":refresh citations [details]", Description: TextHelpRefreshCitations},
			{Key: keyRefreshAll, Command: ":refresh citedby [details]", Description: TextHelpRefreshCitedBy},
			{Command: ":refresh-refs-details", Description: TextHelpRefreshDetails},
			{Key: "ctrl+r (in citation view)", Description: TextHelpCitationRefreshSelected},
			{Key: keyRefreshAll + " (in citation view)", Description: TextHelpCitationRefreshAll},
			{Key: keyCites, Description: TextHelpJumpCitations},
			{Key: keyCitedBy, Description: TextHelpJumpCitedBy},
			{Key: keyYes, Description: TextHelpRefAdd},
			{Key: keyIgnore, Description: TextHelpReviewIgnored},
			{Key: keyUnreview, Description: TextHelpReviewUnderReview},
			{Key: keyStatus + " (in citation view)", Description: TextHelpStatusCycle},
			{Command: ":ignored", Description: TextHelpReviewIgnored},
			{Command: ":under-review", Description: TextHelpReviewUnderReview},
		},
	},
	{
		Title: "Views",
		Entries: []HelpEntry{
			{Key: keyClassification, Description: TextHelpJumpClassification},
			{Key: keyText, Description: TextHelpJumpText},
			{Key: keyNotes, Description: TextHelpJumpNotes},
			{Key: keyRefs + " (in list/detail)", Description: TextHelpJumpRefs},
			{Key: keyAI, Description: TextHelpJumpAI},
			{Key: keyFamily, Description: TextHelpFamilyView},
			{Key: keyProject, Description: TextHelpJumpProject},
			{Key: keyProjectInfo, Description: TextHelpProjectInfo},
		},
	},
	{
		Title: "Filters & Sort",
		Entries: []HelpEntry{
			{Command: ":filter status stored|ignored|under-review|cached|none", Description: TextHelpStatusFilter},
			{Command: ":filter class <cpc> [&& <cpc2>||| <cpc2>]", Description: TextHelpClass},
			{Command: ":filter inventor <name>", Description: TextHelpInventorFilter},
			{Command: ":filter clear", Description: TextHelpFilterClear},
			{Command: ":sort <col>[,<col2>] [asc|desc]", Description: TextHelpSort},
		},
	},
	{
		Title: "Family",
		Entries: []HelpEntry{
			{Key: keyFamily, Command: ":family parent|child <num> [type]", Description: TextHelpFamilyAdd},
			{Command: ":family remove <num>", Description: TextHelpFamilyRemove},
			{Command: ":family pull", Description: TextHelpFamilyPull},
			{Key: "h", Description: TextHelpFamilyParent},
			{Key: "l (in family view)", Description: TextHelpFamilyChild},
			{Key: keyDelete + " (in family view)", Description: TextHelpFamilyRemoveEdge},
			{Key: "+ (in family view)", Description: TextHelpFamilyAddChild},
		},
	},
	{
		Title: "Project",
		Entries: []HelpEntry{
			{Command: ":project id", Description: TextHelpProjectID},
			{Command: ":project list", Description: TextHelpProjectList},
			{Command: ":project create <id> [name]", Description: TextHelpProjectCreate},
			{Command: ":project switch <id>", Description: TextHelpProjectSwitch},
			{Command: ":project add <id>", Description: TextHelpProjectAdd},
			{Command: ":project status <active|archived>", Description: TextHelpProjectStatus},
			{Command: ":project summary-status <stage>", Description: TextHelpProjectSummaryStatus},
			{Command: ":project summary <text>", Description: TextHelpProjectSummary},
			{Command: ":project comment <text>", Description: TextHelpProjectComment},
			{Command: ":project delete <id>", Description: TextHelpProjectDelete},
		},
	},
	{
		Title: "Prosecution & Invoices",
		Entries: []HelpEntry{
			{Command: ":project event <type> [date YYYY-MM-DD] [due YYYY-MM-DD] [ref <ref>] [note <text>]", Description: TextHelpProjectEvent},
			{Command: ":project events", Description: TextHelpProjectEvents},
			{Command: ":project invoice <amount> [currency USD] [direction to-firm|from-firm] ...", Description: TextHelpProjectInvoice},
			{Command: ":project invoices", Description: TextHelpProjectInvoices},
			{Command: ":project ids add <num> [note <text>]", Description: TextHelpProjectIDSAdd},
			{Command: ":project ids", Description: TextHelpProjectIDS},
			{Command: ":project export ids [file]", Description: TextHelpExportIDS},
			{Command: ":project export status [file]", Description: TextHelpExportStatus},
			{Command: ":project export state [stored|ignored|under-review|all] [file]", Description: TextHelpExportState},
		},
	},
	{
		Title: "General",
		Entries: []HelpEntry{
			{Key: keyProjectInfo, Description: TextHelpProjectInfo},
			{Command: ":help", Description: TextHelpShowHelp},
			{Command: ":version", Description: TextHelpShowVersion},
			{Command: ":browser", Description: TextHelpOpenBrowser},
			{Command: ":purge ignored", Description: TextHelpPurge},
			{Command: ":compact", Description: TextHelpCompact},
		},
	},
}

var helpExamples = []string{
	":add US11611785B2",
	":refresh citedby",
	":version",
	":import https://patents.google.com/patent/US11611785B2/en",
	":project event provisional-filed date 2024-03-01",
	":project invoice 5000 firm ACME date 2024-04-01 due 2024-05-01",
}

// matchHelpQuery returns true if the entry matches the query.
// Query ending in "*" is a prefix match; otherwise substring.
func matchHelpQuery(q string, e HelpEntry, text TextCatalog) bool {
	if q == "" {
		return true
	}
	q = strings.ToLower(q)
	target := strings.ToLower(e.Key + " " + e.Command + " " + text.T(e.Description))
	if strings.HasSuffix(q, "*") {
		return strings.Contains(target, strings.TrimSuffix(q, "*"))
	}
	return strings.Contains(target, q)
}

// FilterHelpSections returns sections with only entries matching q.
// Sections with no matching entries are omitted.
func FilterHelpSections(q string, text TextCatalog) []HelpSection {
	if q == "" {
		return allHelpSections
	}
	var out []HelpSection
	for _, section := range allHelpSections {
		var entries []HelpEntry
		for _, e := range section.Entries {
			if matchHelpQuery(q, e, text) {
				entries = append(entries, e)
			}
		}
		if len(entries) > 0 {
			out = append(out, HelpSection{Title: section.Title, Entries: entries})
		}
	}
	return out
}

func RenderHelp(text TextCatalog) string {
	var b strings.Builder
	for _, section := range allHelpSections {
		b.WriteString(section.Title + "\n\n")
		writeHelpEntries(&b, section.Entries, text)
		b.WriteString("\n")
	}
	b.WriteString("\nExamples\n\n")
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
			{Key: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
			{Key: keyEnter + " or " + keyOpen, Description: TextHelpOpenSelected},
			{Key: keySearch + "term", Description: TextHelpFilterPatents},
			{Key: keyStatus, Description: TextHelpCyclePatentStatus},
			{Key: keyCites, Description: TextHelpJumpCitations},
			{Key: keyCitedBy, Description: TextHelpJumpCitedBy},
			{Key: keyClassification, Description: TextHelpJumpClassification},
			{Key: keyProjectInfo, Description: TextHelpProjectInfo},
			{Key: keyHelp, Description: TextHelpShortcutShowHelp},
			{Key: keyBack, Description: TextHelpBackOrQuit},
			{Key: keyQuit, Description: TextHelpQuitApp},
		}
	case viewDetail:
		return []HelpEntry{
			{Key: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
			{Key: keyEnter + " or " + keyOpen, Description: TextHelpOpenSelected},
			{Key: keyCites, Description: TextHelpJumpCitations},
			{Key: keyCitedBy, Description: TextHelpJumpCitedBy},
			{Key: keyClassification, Description: TextHelpJumpClassification},
			{Key: keyProjectInfo, Description: TextHelpProjectInfo},
			{Key: keyHelp, Description: TextHelpShortcutShowHelp},
			{Key: keyBack, Description: TextHelpBackOrQuit},
			{Key: keyQuit, Description: TextHelpQuitApp},
		}
	case viewCites, viewCitedBy:
		return []HelpEntry{
			{Key: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
			{Key: "10" + keyVimDown + "/" + "10" + keyVimUp, Description: TextHelpMoveList},
			{Key: "10" + keyGoto, Description: TextHelpMoveList},
			{Key: keyEnter + " or " + keyOpen, Description: TextHelpOpenSelected},
			{Key: keyYes, Description: TextHelpRefAdd},
			{Key: keyIgnore, Description: TextHelpReviewIgnored},
			{Key: keyUnreview, Description: TextHelpReviewUnderReview},
			{Key: keyStatus, Description: TextHelpStatusCycle},
			{Key: keyRefs, Description: TextHelpCitationRefreshSelected},
			{Key: keyRefreshAll, Description: TextHelpCitationRefreshAll},
			{Key: keyCtrlF + "/" + keyCtrlD, Description: TextHelpJumpViews},
			{Key: keyHelp, Description: TextHelpShortcutShowHelp},
			{Key: keyBack, Description: TextHelpBackOrQuit},
			{Key: keyQuit, Description: TextHelpQuitApp},
		}
	case viewReview:
		return []HelpEntry{
			{Key: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
			{Key: "10" + keyVimDown + "/" + "10" + keyVimUp, Description: TextHelpMoveList},
			{Key: "10" + keyGoto, Description: TextHelpMoveList},
			{Key: keyEnter + " or " + keyOpen, Description: TextHelpOpenSelected},
			{Key: keyYes, Description: TextHelpRefAdd},
			{Key: keyIgnore, Description: TextHelpReviewIgnored},
			{Key: keyUnreview, Description: TextHelpReviewUnderReview},
			{Key: keyStatus, Description: TextHelpStatusCycle},
			{Key: keyWeb, Description: TextHelpOpenBrowser},
			{Key: keyCtrlF + "/" + keyCtrlD, Description: TextHelpJumpViews},
			{Key: keyHelp, Description: TextHelpShortcutShowHelp},
			{Key: keyBack, Description: TextHelpBackOrQuit},
			{Key: keyQuit, Description: TextHelpQuitApp},
		}
	case viewClassifications:
		return []HelpEntry{
			{Key: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
			{Key: "10" + keyVimDown + "/" + "10" + keyVimUp, Description: TextHelpMoveList},
			{Key: "10" + keyGoto, Description: TextHelpMoveList},
			{Key: keyEnter + " / " + keyOpen, Description: TextHelpOpenSelected},
			{Key: keyCtrlF + "/" + keyCtrlD, Description: TextHelpJumpViews},
			{Key: keyHelp, Description: TextHelpShortcutShowHelp},
			{Key: keyBack, Description: TextHelpBackOrQuit},
			{Key: keyQuit, Description: TextHelpQuitApp},
		}
	case viewPreview:
		return []HelpEntry{
			{Key: keyYes, Description: TextHelpRefAdd},
			{Key: keyIgnore, Description: TextHelpReviewIgnored},
			{Key: keyUnreview, Description: TextHelpReviewUnderReview},
			{Key: keyNo + "/" + keyEsc, Description: TextHelpBackOrQuit},
			{Key: keyHelp, Description: TextHelpShortcutShowHelp},
		}
	case viewInventors:
		return []HelpEntry{
			{Key: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
			{Key: keyEnter + " or " + keyOpen, Description: TextHelpOpenSelected},
			{Key: keyHelp, Description: TextHelpShortcutShowHelp},
			{Key: keyBack, Description: TextHelpBackOrQuit},
			{Key: keyQuit, Description: TextHelpQuitApp},
		}
	case viewText, viewRefs, viewNotes, viewAI:
		return []HelpEntry{
			{Key: keyHelp, Description: TextHelpShortcutShowHelp},
			{Key: keyBack, Description: TextHelpBackOrQuit},
			{Key: keyQuit, Description: TextHelpQuitApp},
		}
	case viewFamily:
		return []HelpEntry{
			{Key: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
			{Key: keyColLeft, Description: TextHelpFamilyParent},
			{Key: keyColRight, Description: TextHelpFamilyChild},

			{Key: keyEnter, Description: TextHelpOpenSelected},
			{Key: keyDelete, Description: TextHelpFamilyRemoveEdge},
			{Key: keyRefs, Description: TextHelpFamilyPull},
			{Key: "+", Description: TextHelpFamilyAddChild},
			{Key: keyFamily, Command: keyCommand + commandFamily + " parent|child <num> [type]", Description: TextHelpFamilyAdd},
			{Command: keyCommand + commandFamily + " remove <num>", Description: TextHelpFamilyRemove},
			{Key: keyHelp, Description: TextHelpShortcutShowHelp},
			{Key: keyBack, Description: TextHelpBackOrQuit},
			{Key: keyQuit, Description: TextHelpQuitApp},
		}
	case viewSplash:
		return []HelpEntry{
			{Key: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
			{Key: keyEnter, Description: TextHelpSplashSelect},
			{Key: keyEvents, Description: TextHelpSplashEvents},
			{Key: keyInvoices, Description: TextHelpSplashInvoices},
			{Key: keyIDS, Description: TextHelpSplashIDS},
			{Key: keyNew, Description: TextHelpSplashNew},
			{Key: keyBack, Description: TextHelpBackOrQuit},
			{Key: keyQuit, Description: TextHelpQuitApp},
		}
	case viewProjectInfo:
		return []HelpEntry{
			{Key: "s / m / c / S", Description: TextHelpProjectInfoKeys},
			{Key: keyEsc + "/" + keyBack, Description: TextHelpBackOrQuit},
			{Key: keyQuit, Description: TextHelpQuitApp},
		}
	case viewProjectEvents:
		return []HelpEntry{
			{Key: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
			{Key: keyDelete, Description: TextHelpDeleteSelected},
			{Key: keyEsc + "/" + keyBack, Description: TextHelpBackOrQuit},
			{Key: keyQuit, Description: TextHelpQuitApp},
		}
	case viewProjectInvoices:
		return []HelpEntry{
			{Key: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
			{Key: keyMarkPaid, Description: TextHelpMarkPaid},
			{Key: keyDelete, Description: TextHelpDeleteSelected},
			{Key: keyEsc + "/" + keyBack, Description: TextHelpBackOrQuit},
			{Key: keyQuit, Description: TextHelpQuitApp},
		}
	case viewProjectIDS:
		return []HelpEntry{
			{Key: keyVimDown + "/" + keyVimUp + " or arrow keys", Description: TextHelpMoveList},
			{Key: keyDelete, Description: TextHelpDeleteSelected},
			{Key: keyEsc + "/" + keyBack, Description: TextHelpBackOrQuit},
			{Key: keyQuit, Description: TextHelpQuitApp},
		}
	default:
		return []HelpEntry{
			{Key: keyHelp, Description: TextHelpShortcutShowHelp},
			{Key: keyBack, Description: TextHelpBackOrQuit},
			{Key: keyQuit, Description: TextHelpQuitApp},
		}
	}
}

func globalHelpEntries() []HelpEntry {
	return []HelpEntry{
		{Command: keyCommand + commandHelp, Description: TextHelpShowHelp},
		{Command: keyCommand + commandVersion, Description: TextHelpShowVersion},
		{Command: keyCommand + commandRefresh + " " + refreshTargetCitedBy, Description: TextHelpRefreshCitedBy},
		{Command: keyCommand + commandRefresh + " " + refreshTargetCitations, Description: TextHelpRefreshCitations},
		{Command: keyCommand + commandRefreshRefsDetails, Description: TextHelpRefreshDetails},
		{Command: keyCommand + commandBrowser, Description: TextHelpOpenBrowser},
	}
}

func writeHelpEntries(b *strings.Builder, entries []HelpEntry, text TextCatalog) {
	for _, e := range entries {
		usage := e.Key
		if e.Command != "" && e.Key != "" {
			usage = e.Key + "  " + e.Command
		} else if e.Command != "" {
			usage = e.Command
		}
		b.WriteString(fmt.Sprintf("  %-40s  %s\n", usage, text.T(e.Description)))
	}
}
