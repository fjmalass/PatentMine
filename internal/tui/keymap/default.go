package keymap

import (
	"maps"

	"patentmine/internal/command"
)

// Keymaps holds the base layer plus one layer per UI context. A frontend
// composes a Stack from these; Default() builds the shipped bindings, but the
// same shape could be loaded from a user config file.
type Keymaps struct {
	base     *Layer
	contexts map[command.Context]*Layer
}

// Base returns the always-active global layer.
func (k *Keymaps) Base() *Layer {
	return k.base
}

// Context returns the layer for c, or nil when c has no dedicated layer.
func (k *Keymaps) Context(c command.Context) *Layer {
	return k.contexts[c]
}

// ContextLayers returns every context-scoped layer keyed by its context. The
// wiring check walks this to verify every bound key reaches a handler.
func (k *Keymaps) ContextLayers() map[command.Context]*Layer {
	out := make(map[command.Context]*Layer, len(k.contexts))
	maps.Copy(out, k.contexts)
	return out
}

// StackFor builds the resolution stack for a context: the base layer plus the
// context's own layer.
func (k *Keymaps) StackFor(c command.Context) *Stack {
	s := NewStack(k.base)
	if layer := k.contexts[c]; layer != nil {
		s.Push(layer)
	}
	return s
}

// listMotions are the navigation bindings shared by every scrollable context.
func listMotions() map[string]command.ID {
	return map[string]command.ID{
		"j":      command.NavDown,
		"down":   command.NavDown,
		"k":      command.NavUp,
		"up":     command.NavUp,
		"ctrl+d": command.NavPageDown,
		"pgdown": command.NavPageDown,
		"ctrl+u": command.NavPageUp,
		"pgup":   command.NavPageUp,
		"g g":    command.NavTop,
		"G":      command.NavBottom,
	}
}

// patentActions are the bindings shared by contexts where a patent is selected.
func patentActions() map[string]command.ID {
	return map[string]command.ID{
		"s": command.MarkStored,
		"r": command.MarkUnderReview,
		"i": command.MarkIgnored,
		"x": command.MarkDeleted,
		"a": command.AddToProject,
		"f": command.IngestFamily,
	}
}

// Default returns the shipped keymap.
func Default() *Keymaps {
	base := NewLayer("global", false).BindAll(map[string]command.ID{
		"ctrl+c": command.Quit,
		"Q":      command.Quit,
		"?":      command.Help,
		"q":      command.Back,
		":":      command.OpenCommand,
	})

	catalog := NewLayer("catalog", false).
		BindAll(listMotions()).
		BindAll(patentActions()).
		BindAll(map[string]command.ID{
			"enter":  command.OpenDetail,
			"l":      command.OpenDetail,
			"c":      command.OpenCitations,
			"b":      command.OpenCitedBy,
			"p":      command.OpenProjects,
			"/":      command.OpenSearch,
			"ctrl+r": command.Refresh,
		})

	detail := NewLayer("detail", false).
		BindAll(listMotions()).
		BindAll(patentActions()).
		BindAll(map[string]command.ID{
			"c":      command.OpenCitations,
			"b":      command.OpenCitedBy,
			"p":      command.OpenProjects,
			"/":      command.OpenSearch,
			"ctrl+r": command.Refresh,
		})

	citations := NewLayer("citations", false).
		BindAll(listMotions()).
		BindAll(patentActions()).
		BindAll(map[string]command.ID{
			"enter":  command.OpenDetail,
			"l":      command.OpenDetail,
			"p":      command.OpenProjects,
			"/":      command.OpenSearch,
			"ctrl+r": command.Refresh,
		})

	projects := NewLayer("projects", false).
		BindAll(listMotions()).
		BindAll(map[string]command.ID{
			"enter":  command.ProjectActivate,
			"l":      command.ProjectActivate,
			"u":      command.ProjectClearActive,
			"n":      command.ProjectCreate,
			"I":      command.ExportIDS,
			"/":      command.OpenSearch,
			"ctrl+r": command.Refresh,
		})

	// The overlay layer is composed directly on top of the global base while an
	// overlay is focused — the App leaves the pane layer out entirely, so the
	// pane's bindings are inactive while only the global ones remain.
	overlay := NewLayer("overlay", false).BindAll(map[string]command.ID{
		"esc":    command.CloseOverlay,
		"q":      command.CloseOverlay,
		"j":      command.NavDown,
		"down":   command.NavDown,
		"k":      command.NavUp,
		"up":     command.NavUp,
		"ctrl+d": command.NavPageDown,
		"ctrl+u": command.NavPageUp,
	})

	return &Keymaps{
		base: base,
		contexts: map[command.Context]*Layer{
			command.ContextCatalog:   catalog,
			command.ContextDetail:    detail,
			command.ContextCitations: citations,
			command.ContextProjects:  projects,
			command.ContextOverlay:   overlay,
		},
	}
}
