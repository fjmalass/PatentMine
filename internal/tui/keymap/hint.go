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

// HintCatalog maps each UI context to its ordered list of header hints. It is
// built once at startup and injected, matching the command.Registry pattern.
type HintCatalog struct {
	byContext map[command.Context][]Hint
}

// For returns the hints for ctx, or nil when ctx has none.
func (c *HintCatalog) For(ctx command.Context) []Hint {
	return c.byContext[ctx]
}

// NewHintCatalog builds a HintCatalog and validates that every command ID in
// every hint is registered in reg. A missing ID causes a startup failure — the
// same safety as command.NewRegistry and validateWiring.
func NewHintCatalog(reg *command.Registry, raw map[command.Context][]Hint) (*HintCatalog, error) {
	for ctx, hints := range raw {
		for i, h := range hints {
			for _, id := range h.Commands {
				if _, ok := reg.Lookup(id); !ok {
					return nil, fmt.Errorf("keymap hint: context %q hint %d references unregistered command %q", ctx, i, id)
				}
			}
		}
	}
	return &HintCatalog{byContext: raw}, nil
}

// DefaultHints returns the shipped per-context hint definitions. Each entry
// corresponds to one group of keys-and-label shown in the header bar.
func DefaultHints() map[command.Context][]Hint {
	return map[command.Context][]Hint{
		command.ContextCatalog: {
			{Commands: []command.ID{command.OpenSearch}, Label: text.HintCommands},
			{Commands: []command.ID{command.OpenCommand}, Label: text.HintCommand},
			{Commands: []command.ID{command.OpenDetail}, Label: text.HintDetail},
			{Commands: []command.ID{command.OpenIDS}, Label: text.HintIDS},
			{Commands: []command.ID{command.OpenBrowser}, Label: text.HintBrowse},
			{Commands: []command.ID{command.OpenCitations}, Label: text.HintCitations},
			{Commands: []command.ID{command.OpenCitedBy}, Label: text.HintCitedBy},
			{Commands: []command.ID{command.OpenProjects}, Label: text.HintProjects},
			{Commands: []command.ID{command.IngestFamily}, Label: text.HintIngest},
			{Commands: []command.ID{command.FetchPatent}, Label: text.HintFetch},
			{Commands: []command.ID{command.AddToProject, command.MarkStored, command.MarkUnderReview, command.MarkIgnored, command.MarkDeleted}, Label: text.HintProjectActions},
			{Commands: []command.ID{command.Back}, Label: text.HintBack},
		},
		command.ContextDetail: {
			{Commands: []command.ID{command.OpenSearch}, Label: text.HintCommands},
			{Commands: []command.ID{command.OpenCommand}, Label: text.HintCommand},
			{Commands: []command.ID{command.JumpMode}, Label: text.HintJump},
			{Commands: []command.ID{command.OpenIDS}, Label: text.HintIDS},
			{Commands: []command.ID{command.OpenBrowser}, Label: text.HintBrowse},
			{Commands: []command.ID{command.OpenCitations}, Label: text.HintCitations},
			{Commands: []command.ID{command.OpenCitedBy}, Label: text.HintCitedBy},
			{Commands: []command.ID{command.OpenProjects}, Label: text.HintProjects},
			{Commands: []command.ID{command.IngestFamily}, Label: text.HintIngest},
			{Commands: []command.ID{command.FetchPatent}, Label: text.HintFetch},
			{Commands: []command.ID{command.AddToProject, command.MarkStored, command.MarkUnderReview, command.MarkIgnored, command.MarkDeleted}, Label: text.HintProjectActions},
			{Commands: []command.ID{command.Back}, Label: text.HintBack},
		},
		command.ContextCitations: {
			{Commands: []command.ID{command.OpenSearch}, Label: text.HintCommands},
			{Commands: []command.ID{command.OpenCommand}, Label: text.HintCommand},
			{Commands: []command.ID{command.OpenDetail}, Label: text.HintDetail},
			{Commands: []command.ID{command.OpenIDS}, Label: text.HintIDS},
			{Commands: []command.ID{command.OpenBrowser}, Label: text.HintBrowse},
			{Commands: []command.ID{command.OpenProjects}, Label: text.HintProjects},
			{Commands: []command.ID{command.IngestFamily}, Label: text.HintIngest},
			{Commands: []command.ID{command.AddToProject, command.MarkStored, command.MarkUnderReview, command.MarkIgnored, command.MarkDeleted}, Label: text.HintProjectActions},
			{Commands: []command.ID{command.Back}, Label: text.HintBack},
		},
		command.ContextIDS: {
			{Commands: []command.ID{command.OpenCommand}, Label: text.HintCommand},
			{Commands: []command.ID{command.IDSEditField}, Label: text.HintDetail},
			{Commands: []command.ID{command.IDSToggleFull}, Label: text.HintFetch},
			{Commands: []command.ID{command.IDSCycleStatus}, Label: text.HintProjectActions},
			{Commands: []command.ID{command.Back}, Label: text.HintBack},
		},
		command.ContextProjects: {
			{Commands: []command.ID{command.OpenSearch}, Label: text.HintCommands},
			{Commands: []command.ID{command.OpenCommand}, Label: text.HintCommand},
			{Commands: []command.ID{command.ProjectActivate}, Label: text.HintSelectProject},
			{Commands: []command.ID{command.ProjectClearActive}, Label: text.HintClearActive},
			{Commands: []command.ID{command.ProjectCreate}, Label: text.HintNewProject},
			{Commands: []command.ID{command.ExportIDS}, Label: text.HintExportIDS},
			{Commands: []command.ID{command.Back}, Label: text.HintBack},
		},
		command.ContextOverlay: {
			{Commands: []command.ID{command.OpenSearch}, Label: text.HintCommands},
			{Commands: []command.ID{command.OpenCommand}, Label: text.HintCommand},
		},
	}
}
