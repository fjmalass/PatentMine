package keymap

import (
	"fmt"

	"patentmine/internal/command"
	"patentmine/internal/text"
)

// Hint is one entry in the header hint bar: a label displayed alongside the
// key shortcuts for one or more commands. A multi-command Hint groups related
// commands under a single label (e.g. "a/i/r/s/x project actions").
type Hint struct {
	Commands []command.ID
	Label    text.Key
}

// HintCatalog maps each UI scope to its ordered list of header hints. It is
// built once at startup and injected, matching the command.Registry pattern.
type HintCatalog struct {
	byScope map[command.Scope][]Hint
}

// For returns the hints for scope, or nil when scope has none.
func (c *HintCatalog) For(scope command.Scope) []Hint {
	return c.byScope[scope]
}

// NewHintCatalog builds a HintCatalog and validates that every command ID in
// every hint is registered in reg. A missing ID causes a startup failure — the
// same safety as command.NewRegistry and validateWiring.
func NewHintCatalog(reg *command.Registry, raw map[command.Scope][]Hint) (*HintCatalog, error) {
	for scope, hints := range raw {
		for i, h := range hints {
			for _, id := range h.Commands {
				if _, ok := reg.Lookup(id); !ok {
					return nil, fmt.Errorf("keymap hint: scope %q hint %d references unregistered command %q", scope, i, id)
				}
			}
		}
	}
	return &HintCatalog{byScope: raw}, nil
}

// DefaultHints returns the shipped per-scope hint definitions. Each entry
// corresponds to one group of keys-and-label shown in the header bar.
func DefaultHints() map[command.Scope][]Hint {
	return map[command.Scope][]Hint{
		command.ScopeCatalog: {
			{Commands: []command.ID{command.OpenSearch}, Label: text.HintCommands},
			{Commands: []command.ID{command.OpenCommand}, Label: text.HintCommand},
			{Commands: []command.ID{command.OpenDetail}, Label: text.HintDetail},
			{Commands: []command.ID{command.OpenIDS}, Label: text.HintIDS},
			{Commands: []command.ID{command.OpenBrowser}, Label: text.HintBrowse},
			{Commands: []command.ID{command.OpenCitations}, Label: text.HintCitations},
			{Commands: []command.ID{command.OpenCitedBy}, Label: text.HintCitedBy},
			{Commands: []command.ID{command.OpenParents}, Label: text.HintParents},
			{Commands: []command.ID{command.OpenFamilyGraph}, Label: text.HintFamily},
			{Commands: []command.ID{command.OpenProjects}, Label: text.HintProjects},
			{Commands: []command.ID{command.CrawlFamily}, Label: text.HintCrawl},
			{Commands: []command.ID{command.LookupPatent}, Label: text.HintLookup},
			{Commands: []command.ID{command.AddToProject, command.MarkStored, command.MarkUnderReview, command.MarkIgnored, command.MarkDeleted}, Label: text.HintProjectActions},
			{Commands: []command.ID{command.Back}, Label: text.HintBack},
		},
		command.ScopeDetail: {
			{Commands: []command.ID{command.OpenSearch}, Label: text.HintCommands},
			{Commands: []command.ID{command.OpenCommand}, Label: text.HintCommand},
			{Commands: []command.ID{command.OpenFullText}, Label: text.HintFullText},
			{Commands: []command.ID{command.JumpMode}, Label: text.HintJump},
			{Commands: []command.ID{command.OpenIDS}, Label: text.HintIDS},
			{Commands: []command.ID{command.OpenBrowser}, Label: text.HintBrowse},
			{Commands: []command.ID{command.OpenCitations}, Label: text.HintCitations},
			{Commands: []command.ID{command.OpenCitedBy}, Label: text.HintCitedBy},
			{Commands: []command.ID{command.OpenParents}, Label: text.HintParents},
			{Commands: []command.ID{command.OpenFamilyGraph}, Label: text.HintFamily},
			{Commands: []command.ID{command.OpenProjects}, Label: text.HintProjects},
			{Commands: []command.ID{command.CrawlFamily}, Label: text.HintCrawl},
			{Commands: []command.ID{command.LookupPatent}, Label: text.HintLookup},
			{Commands: []command.ID{command.AddToProject, command.MarkStored, command.MarkUnderReview, command.MarkIgnored, command.MarkDeleted}, Label: text.HintProjectActions},
			{Commands: []command.ID{command.Back}, Label: text.HintBack},
		},
		command.ScopeCitations: {
			{Commands: []command.ID{command.OpenSearch}, Label: text.HintCommands},
			{Commands: []command.ID{command.OpenCommand}, Label: text.HintCommand},
			{Commands: []command.ID{command.OpenDetail}, Label: text.HintDetail},
			{Commands: []command.ID{command.OpenIDS}, Label: text.HintIDS},
			{Commands: []command.ID{command.OpenBrowser}, Label: text.HintBrowse},
			{Commands: []command.ID{command.OpenProjects}, Label: text.HintProjects},
			{Commands: []command.ID{command.OpenFamilyGraph}, Label: text.HintFamily},
			{Commands: []command.ID{command.CrawlFamily}, Label: text.HintCrawl},
			{Commands: []command.ID{command.AddToProject, command.MarkStored, command.MarkUnderReview, command.MarkIgnored, command.MarkDeleted}, Label: text.HintProjectActions},
			{Commands: []command.ID{command.Back}, Label: text.HintBack},
		},
		command.ScopeFamily: {
			{Commands: []command.ID{command.OpenSearch}, Label: text.HintCommands},
			{Commands: []command.ID{command.OpenCommand}, Label: text.HintCommand},
			{Commands: []command.ID{command.OpenDetail}, Label: text.HintDetail},
			{Commands: []command.ID{command.SelectVisual, command.SelectAll}, Label: text.HintSelect},
			{Commands: []command.ID{command.FamilyDepthLess, command.FamilyDepthMore}, Label: text.HintMove},
			{Commands: []command.ID{command.OpenParents}, Label: text.HintParents},
			{Commands: []command.ID{command.OpenProjects}, Label: text.HintProjects},
			{Commands: []command.ID{command.CrawlFamily}, Label: text.HintCrawl},
			{Commands: []command.ID{command.Back}, Label: text.HintBack},
		},
		command.ScopeIDS: {
			{Commands: []command.ID{command.OpenCommand}, Label: text.HintCommand},
			{Commands: []command.ID{command.IDSEditField}, Label: text.HintDetail},
			{Commands: []command.ID{command.IDSToggleFull}, Label: text.HintDetail},
			{Commands: []command.ID{command.IDSCycleStatus}, Label: text.HintProjectActions},
			{Commands: []command.ID{command.Back}, Label: text.HintBack},
		},
		command.ScopeProjects: {
			{Commands: []command.ID{command.OpenSearch}, Label: text.HintCommands},
			{Commands: []command.ID{command.OpenCommand}, Label: text.HintCommand},
			{Commands: []command.ID{command.ProjectActivate}, Label: text.HintSelectProject},
			{Commands: []command.ID{command.ProjectClearActive}, Label: text.HintClearActive},
			{Commands: []command.ID{command.ProjectCreate}, Label: text.HintNewProject},
			{Commands: []command.ID{command.ExportIDS}, Label: text.HintExportIDS},
			{Commands: []command.ID{command.Back}, Label: text.HintBack},
		},
		command.ScopeOverlay: {
			{Commands: []command.ID{command.OpenSearch}, Label: text.HintCommands},
			{Commands: []command.ID{command.OpenCommand}, Label: text.HintCommand},
		},
	}
}
