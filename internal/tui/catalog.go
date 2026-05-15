package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

var allHelpSections []HelpSection

func buildAllHelpSections() []HelpSection {
	// Seed with hardcoded command-based entries and special shortcuts not in KeyMap.
	// We use a slice of structs to maintain section order.
	type sectionSeed struct {
		Title   string
		Entries []HelpEntry
	}
	seeds := []sectionSeed{
		{
			Title: "Navigation",
			Entries: []HelpEntry{
				{Key: "↓/↑", Description: TextHelpMoveList},
				{Key: "esc", Description: TextHelpBackOrQuit},
				{Key: "ctrl+f / ctrl+d", Description: TextHelpJumpViews},
			},
		},
		{
			Title: "Patents",
			Entries: []HelpEntry{
				{Command: ":add US11611785B2", Description: TextHelpAddPatent},
				{Command: ":import <url>", Description: TextHelpImportPatent},
				{Command: ":open US11611785B2", Description: TextHelpOpenPatent},
				{Command: ":date app|pub|grant <YYYY-MM-DD>", Description: TextHelpPatentDate},
				{Command: ":num app|pub|grant <number>", Description: TextHelpPatentNum},
				{Command: ":compare <num>", Description: TextHelpCompare},
				{Command: ":summarize", Description: TextHelpSummarize},
				{Command: "/term", Description: TextHelpSearchHint},
			},
		},
		{
			Title: "Refresh & Citations",
			Entries: []HelpEntry{
				{Command: ":refresh citations [details]", Description: TextHelpRefreshCitations},
				{Command: ":refresh citedby [details]", Description: TextHelpRefreshCitedBy},
				{Command: ":refresh-refs-details", Description: TextHelpRefreshDetails},
				{Key: "ctrl+r (in citation view)", Description: TextHelpCitationRefreshSelected},
				{Key: keyRefreshAll + " (in citation view)", Description: TextHelpCitationRefreshAll},
				{Key: "s (in citation view)", Description: TextHelpReviewStateCycle},
				{Command: ":ignored", Description: TextHelpReviewIgnored},
				{Command: ":under-review", Description: TextHelpReviewUnderReview},
			},
		},
		{
			Title: "Views",
			Entries: []HelpEntry{
				{Key: "ctrl+r (in list/detail)", Description: TextHelpJumpRefs},
			},
		},
		{
			Title: "Filters & Sort",
			Entries: []HelpEntry{
				{Command: ":filter review_state stored|ignored|under-review|cached|none", Description: TextHelpReviewStateFilter},
				{Command: ":filter class <cpc> [&& <cpc2>||| <cpc2>]", Description: TextHelpClass},
				{Command: ":filter inventor <name>", Description: TextHelpInventorFilter},
				{Command: ":filter clear", Description: TextHelpFilterClear},
				{Command: ":country list|<CODE>", Description: TextHelpCountryFilter},
				{Command: ":sort <col>[,<col2>] [asc|desc]", Description: TextHelpSort},
			},
		},
		{
			Title: "Family",
			Entries: []HelpEntry{
				{Command: ":family parent|child <num> [type]", Description: TextHelpFamilyAdd},
				{Command: ":family remove <num>", Description: TextHelpFamilyRemove},
				{Command: ":family pull", Description: TextHelpFamilyPull},
				{Key: "D (in family view)", Description: TextHelpFamilyRemoveEdge},
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
				{Command: ":help", Description: TextHelpShowHelp},
				{Command: ":version", Description: TextHelpShowVersion},
				{Command: ":browser", Description: TextHelpOpenBrowser},
				{Command: ":note <text>", Description: TextHelpNote},
				{Command: ":purge ignored", Description: TextHelpPurge},
				{Command: ":compact", Description: TextHelpCompact},
				{Command: ":exit", Description: TextHelpExit},
			},
		},
	}

	// Collect all registered keys from GlobalKeys and all ModeKeys.
	type kbGroup struct {
		Help    TextKey
		Section string
	}
	groups := make(map[kbGroup][]string)
	addKeys := func(km KeyMap) {
		for _, kb := range km {
			if kb.Help == "" || kb.Section == "" {
				continue
			}
			gk := kbGroup{kb.Help, kb.Section}
			// Avoid duplicate keys for the same help in the same section.
			found := false
			for _, k := range groups[gk] {
				if k == kb.Key {
					found = true
					break
				}
			}
			if !found {
				groups[gk] = append(groups[gk], kb.Key)
			}
		}
	}
	addKeys(GlobalKeys)
	for _, km := range ModeKeys {
		addKeys(km)
	}

	// Build the final sections.
	var sections []HelpSection
	for _, seed := range seeds {
		section := HelpSection{Title: seed.Title}
		// Map to track entries by description for merging.
		entryByDesc := make(map[TextKey]*HelpEntry)

		for i := range seed.Entries {
			e := &seed.Entries[i]
			section.Entries = append(section.Entries, *e)
			entryByDesc[e.Description] = &section.Entries[len(section.Entries)-1]
		}

		// Find groups for this section.
		var sectionGK []kbGroup
		for gk := range groups {
			if gk.Section == seed.Title {
				sectionGK = append(sectionGK, gk)
			}
		}
		// Sort for deterministic output.
		sort.Slice(sectionGK, func(i, j int) bool {
			return string(sectionGK[i].Help) < string(sectionGK[j].Help)
		})

		for _, gk := range sectionGK {
			ks := groups[gk]
			sort.Strings(ks)
			keyStr := strings.Join(ks, "/")

			if e, ok := entryByDesc[gk.Help]; ok {
				// Merge keys.
				if e.Key != "" {
					e.Key = keyStr + " / " + e.Key
				} else {
					e.Key = keyStr
				}
			} else {
				// Add new entry.
				newEntry := HelpEntry{
					Key:         keyStr,
					Description: gk.Help,
				}
				section.Entries = append(section.Entries, newEntry)
				entryByDesc[gk.Help] = &section.Entries[len(section.Entries)-1]
			}
			// Mark as used.
			delete(groups, gk)
		}
		sections = append(sections, section)
	}

	// Add any remaining sections that were in KeyMap but not in seeds.
	remainingSections := make(map[string]bool)
	for gk := range groups {
		remainingSections[gk.Section] = true
	}
	var sortedRemaining []string
	for s := range remainingSections {
		sortedRemaining = append(sortedRemaining, s)
	}
	sort.Strings(sortedRemaining)

	for _, sTitle := range sortedRemaining {
		section := HelpSection{Title: sTitle}
		var sectionGK []kbGroup
		for gk := range groups {
			if gk.Section == sTitle {
				sectionGK = append(sectionGK, gk)
			}
		}
		sort.Slice(sectionGK, func(i, j int) bool {
			return string(sectionGK[i].Help) < string(sectionGK[j].Help)
		})
		for _, gk := range sectionGK {
			ks := groups[gk]
			sort.Strings(ks)
			section.Entries = append(section.Entries, HelpEntry{
				Key:         strings.Join(ks, "/"),
				Description: gk.Help,
			})
		}
		sections = append(sections, section)
	}

	return sections
}

func groupBindings(keys KeyMap) []HelpEntry {
	groups := make(map[TextKey][]string)
	var ordered []TextKey
	for _, k := range sortedKeyMapKeys(keys) {
		kb := keys[k]
		if kb.Help == "" || kb.Section == "" {
			continue
		}
		if _, ok := groups[kb.Help]; !ok {
			ordered = append(ordered, kb.Help)
		}
		groups[kb.Help] = append(groups[kb.Help], kb.Key)
	}

	var entries []HelpEntry
	for _, h := range ordered {
		ks := groups[h]
		sort.Strings(ks)
		entries = append(entries, HelpEntry{
			Key:         strings.Join(ks, "/"),
			Description: h,
		})
	}
	return entries
}

func sortedKeyMapKeys(m KeyMap) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func ensureHelpBuilt() {
	if len(allHelpSections) == 0 {
		allHelpSections = buildAllHelpSections()
	}
}

var helpExamples = []string{
	":add US11611785B2",
	":refresh citedby",
	":version",
	":import https://patents.google.com/patent/US11611785B2/en",
	":project event provisional-filed date 2024-03-01",
	":project invoice 5000 firm ACME date 2024-04-01 due 2024-05-01",
}

// commandsSection builds a compact "Commands" section listing each unique
// root command (e.g. :project, :filter) derived from all help sections.
// This gives users a fast overview of what commands exist without reading
// every detail section.
func commandsSection() HelpSection {
	ensureHelpBuilt()
	seen := map[string]bool{}
	var entries []HelpEntry
	for _, section := range allHelpSections {
		for _, e := range section.Entries {
			if e.Command == "" {
				continue
			}
			cmd := strings.TrimPrefix(e.Command, ":")
			parts := strings.Fields(cmd)
			if len(parts) == 0 {
				continue
			}
			root := parts[0]
			if seen[root] {
				continue
			}
			seen[root] = true
			entries = append(entries, HelpEntry{
				Command:     ":" + root,
				Description: e.Description,
			})
		}
	}
	return HelpSection{Title: "Commands", Entries: entries}
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
// A "Commands" overview section is always prepended first.
func FilterHelpSections(q string, text TextCatalog) []HelpSection {
	ensureHelpBuilt()
	cmds := commandsSection()
	if q == "" {
		return append([]HelpSection{cmds}, allHelpSections...)
	}
	var out []HelpSection
	// Filter the commands overview section too.
	var cmdEntries []HelpEntry
	for _, e := range cmds.Entries {
		if matchHelpQuery(q, e, text) {
			cmdEntries = append(cmdEntries, e)
		}
	}
	if len(cmdEntries) > 0 {
		out = append(out, HelpSection{Title: cmds.Title, Entries: cmdEntries})
	}
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
	ensureHelpBuilt()
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

	spec := mustModeSpec(viewHelpPopup)
	accentColor := spec.themeColor
	if accentColor == "" {
		accentColor = ColorThemeList
	}

	titleStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(accentColor)).
		Foreground(lipgloss.Color(ColorBlack)).
		Bold(true).
		Padding(0, 1)

	b.WriteString(titleStyle.Render(text.T(TextHelpPopupTitle)+" · "+screenTitleForMode(mode)) + "\n")

	b.WriteString("\n")
	b.WriteString(text.T(TextHelpScreen) + "\n\n")
	writeHelpEntries(&b, contextHelpEntries(mode), text)
	b.WriteString("\n" + text.T(TextHelpGlobal) + "\n\n")
	writeHelpEntries(&b, globalHelpEntries(), text)
	return b.String()
}

func contextHelpEntries(mode viewMode) []HelpEntry {
	active := make(KeyMap)
	for k, v := range GlobalKeys {
		active[k] = v
	}
	for k, v := range ModeKeys[mode] {
		active[k] = v
	}
	return groupBindings(active)
}

func globalHelpEntries() []HelpEntry {
	return groupBindings(GlobalKeys)
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

func BuildHelperLine(keys KeyMap, text TextCatalog) string {
	// 1. Filter keys for those with ShowInHint == true
	var hintKeys []KeyBinding
	for _, kb := range keys {
		if kb.ShowInHint {
			hintKeys = append(hintKeys, kb)
		}
	}

	// 2. Group keys by their Label
	groups := make(map[string][]string)
	var labels []string
	for _, kb := range hintKeys {
		if _, ok := groups[kb.Label]; !ok {
			labels = append(labels, kb.Label)
		}
		// Avoid duplicate keys in same label group
		found := false
		for _, k := range groups[kb.Label] {
			if k == kb.Key {
				found = true
				break
			}
		}
		if !found {
			groups[kb.Label] = append(groups[kb.Label], kb.Key)
		}
	}

	// 3. Sort the groups in a logical order
	sort.Slice(labels, func(i, j int) bool {
		wi, wj := labelWeight(labels[i]), labelWeight(labels[j])
		if wi != wj {
			return wi < wj
		}
		return labels[i] < labels[j]
	})

	// 4. Join groups with " · "
	var parts []string
	for _, label := range labels {
		ks := groups[label]
		sort.Strings(ks)
		parts = append(parts, fmt.Sprintf("%s: %s", strings.Join(ks, "/"), label))
	}

	return strings.Join(parts, " · ")
}

func labelWeight(l string) int {
	switch l {
	case "move":
		return 1
	case "open", "select", "save", "filter", "status":
		return 2
	case "refresh", "add", "rename":
		return 3
	case "help", "back", "quit":
		return 4
	default:
		return 5
	}
}
