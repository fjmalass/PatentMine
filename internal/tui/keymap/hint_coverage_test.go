package keymap

import (
	"sort"
	"strings"
	"testing"

	"patentmine/internal/command"
)

// TestHintCoverage fails when a key-bound command is missing from the hint
// footer for the scope it lives in. The point is to catch the brittle case
// where a new chord is added in keymap/default.go and somebody forgets to
// extend hint.go — the footer is the only on-screen surface that reveals the
// chord to a user who has not read the docs.
//
// Pure-navigation and structural edits (motions, scrolling, selection) are
// excluded via hintCoverageExempt below; they would clutter the footer if
// listed, and users discover them through cursor motion anyway.
func TestHintCoverage(t *testing.T) {
	keymaps := Default()
	catalog, err := NewHintCatalog(stubRegistry(), DefaultHints())
	if err != nil {
		t.Fatalf("NewHintCatalog: %v", err)
	}

	type gap struct {
		scope command.Scope
		id    command.ID
	}
	var gaps []gap
	for scope, layer := range keymaps.ScopeLayers() {
		if layer == nil {
			continue
		}
		hinted := hintedCommands(catalog.For(scope))
		seen := map[command.ID]bool{}
		for _, id := range layer.Bindings() {
			if seen[id] {
				continue
			}
			seen[id] = true
			if hintCoverageExempt[id] {
				continue
			}
			if hinted[id] {
				continue
			}
			gaps = append(gaps, gap{scope: scope, id: id})
		}
	}

	if len(gaps) == 0 {
		return
	}
	sort.Slice(gaps, func(i, j int) bool {
		if gaps[i].scope != gaps[j].scope {
			return gaps[i].scope < gaps[j].scope
		}
		return gaps[i].id < gaps[j].id
	})
	var b strings.Builder
	b.WriteString("commands bound to keys but missing from the hint footer:\n")
	for _, g := range gaps {
		b.WriteString("  ")
		b.WriteString(string(g.scope))
		b.WriteString(" → ")
		b.WriteString(string(g.id))
		b.WriteByte('\n')
	}
	b.WriteString("fix: add a Hint entry for the command in keymap/hint.go " +
		"for the listed scope, or add the command id to hintCoverageExempt " +
		"when it is intentionally not surfaced in the footer.")
	t.Fatal(b.String())
}

func hintedCommands(hints []Hint) map[command.ID]bool {
	out := map[command.ID]bool{}
	for _, h := range hints {
		for _, id := range h.Commands {
			out[id] = true
		}
	}
	return out
}

// hintCoverageExempt lists command IDs that are deliberately not exposed in
// the hint footer. Most are universal motions a user already discovers from
// cursor behaviour, or are edit-mode keys that only make sense inside a
// focused control.
var hintCoverageExempt = map[command.ID]bool{
	// List navigation — shown on every list, footer would be noise.
	command.NavDown: true, command.NavUp: true,
	command.NavPageDown: true, command.NavPageUp: true,
	command.NavTop: true, command.NavBottom: true,

	// Column nav + sort apply — discoverable from header chrome.
	command.ColNext: true, command.ColPrev: true, command.SortApply: true,
	command.FindOpen: true,

	// Selection model — visible from the selection chrome on the list.
	command.SelectVisual: true, command.SelectClear: true, command.SelectAll: true,
	command.ReselectLast: true,

	// Refresh + general overlay close + back — bound everywhere; surfaced once
	// in the global help.
	command.Refresh: true, command.Back: true, command.CloseOverlay: true,

	// Global app-wide keys live in the base layer; the per-scope footer is for
	// scope-specific actions, so do not duplicate them.
	command.OpenSearch: true, command.OpenCommand: true,
	command.HistoryBack: true, command.HistoryForward: true,
	command.OpenHistory: true, command.OpenActivity: true,
	command.OpenAllNotes:   true, // shown selectively in scopes that need it.
	command.OpenMetrics:    true,
	command.OpenPatentNote: true, // shown selectively in scopes that need it.
	command.Help:           true, command.Quit: true, command.SettingsAI: true,

	// Catalog-only highlights and fold/unfold — pane chrome reveals them.
	command.HighlightFamily: true, command.HighlightCitations: true,
	command.RelationFilterCollapse: true, command.RelationFilterExpand: true,

	// Detail-pane jump + AI — surfaced in the detail header.
	command.JumpMode: true, command.AIAnalyze: true,
	command.OpenFullText:    true,
	command.FetchUSPTOGrant: true,

	// IDS pane editing keys — the edit chrome marks them inline.
	command.IDSEditField: true, command.IDSToggleFull: true,
	command.IDSCycleStatus: true, command.IDSDelete: true,

	// Notes pane editing — header line tells the user.
	command.NotesSortToggle: true,

	// Raw XML viewer yank (very useful in a text viewer but we don't want
	// to force a hint footer entry for the uspto-xml scope yet).
	command.CopyYank:     true,
	command.CopyYankMeta: true,

	// Family controls — surfaced as +/− chips in the graph header.
	command.FamilyDepthMore: true, command.FamilyDepthLess: true,
	command.FamilyExportMermaid: true,

	// Quick scope-jump keys handled by their target scope's footer.
	command.OpenIDS:                   true,
	command.OpenProjects:              true,
	command.OpenFamilyGraph:           true,
	command.OpenBrowser:               true,
	command.OpenCitations:             true,
	command.OpenCitedBy:               true,
	command.OpenParents:               true,
	command.OpenChildren:              true,
	command.OpenDetail:                true,
	command.OpenInventors:             true,
	command.OpenInventorsDirect:       true,
	command.OpenAssignees:             true,
	command.OpenClassificationStats:   true,
	command.OpenPatentClassifications: true,

	// Patent actions — shown collectively under HintProjectActions.
	command.MarkActive: true, command.MarkUnderReview: true,
	command.MarkIgnored: true, command.MarkDeleted: true,
	command.AddToProject: true, command.PatentDelete: true,
	command.TagPatentManage: true,
	command.CrawlFamily:     true,
	command.LookupPatent:    true,

	// Source comparison is a conditional/advanced feature (only relevant after
	// a compare-mode fetch that produced source_diff rows). The conditional
	// hint inside the detail pane + the working command (g c / :source-compare)
	// provide the discoverability; we don't want it in the global header footer.
	command.SourceCompare: true,
}

// stubRegistry returns a command.Registry that contains every shipped command
// so NewHintCatalog can verify hint references. It panics on registration
// failure since that would mean the catalog itself is broken.
func stubRegistry() *command.Registry {
	r, err := command.Default()
	if err != nil {
		panic(err)
	}
	return r
}
