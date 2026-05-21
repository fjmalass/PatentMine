package keymap

import (
	"maps"

	"patentmine/internal/command"
)

// Keymaps holds the base layer plus one layer per UI scope. A frontend
// composes a Stack from these; Default() builds the shipped bindings, but the
// same shape could be loaded from a user config file.
type Keymaps struct {
	base     *Layer
	scopes map[command.Scope]*Layer
}

// Base returns the always-active global layer.
func (k *Keymaps) Base() *Layer {
	return k.base
}

// Scope returns the layer for s, or nil when s has no dedicated layer.
func (k *Keymaps) Scope(s command.Scope) *Layer {
	return k.scopes[s]
}

// ScopeLayers returns every scope-scoped layer keyed by its scope. The
// wiring check walks this to verify every bound key reaches a handler.
func (k *Keymaps) ScopeLayers() map[command.Scope]*Layer {
	out := make(map[command.Scope]*Layer, len(k.scopes))
	maps.Copy(out, k.scopes)
	return out
}

// StackFor builds the resolution stack for a scope: the base layer plus the
// scope's own layer.
func (k *Keymaps) StackFor(s command.Scope) *Stack {
	st := NewStack(k.base)
	if layer := k.scopes[s]; layer != nil {
		st.Push(layer)
	}
	return st
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

// patentActions are the bindings shared by scopes where a patent is selected.
func patentActions() map[string]command.ID {
	return map[string]command.ID{
		"s": command.MarkStored,
		"r": command.MarkUnderReview,
		"i": command.MarkIgnored,
		"x": command.MarkDeleted,
		"D": command.PatentDelete,
		"a": command.AddToProject,
		"f": command.CrawlFamily,
		"L": command.LookupPatent,
	}
}

// viewActions are the bindings shared by scopes that display a patent and
// its related views (detail, citations, cited-by, IDS, browser, etc.).
func viewActions() map[string]command.ID {
	return map[string]command.ID{
		"I":      command.OpenIDS,
		"w":      command.OpenBrowser,
		"c":      command.OpenCitations,
		"b":      command.OpenCitedBy,
		"p":      command.OpenProjects,
		"ctrl+r": command.Refresh,
	}
}

// Default returns the shipped keymap.
func Default() *Keymaps {
	base := NewLayer("global", false).BindAll(map[string]command.ID{
		"ctrl+c": command.Quit,
		"Q":      command.Quit,
		"?":      command.Help,
		"h":      command.Back,
		"left":   command.Back,
		"esc":    command.Back,
		":":      command.OpenCommand,
	})

	catalog := NewLayer("catalog", false).
		BindAll(listMotions()).
		BindAll(patentActions()).
		BindAll(viewActions()).
		BindAll(map[string]command.ID{
			"enter":  command.OpenDetail,
			"l":      command.OpenDetail,
			"right":  command.ColNext,
			"left":   command.ColPrev,
			".":      command.SortApply,
			"/":      command.FindOpen,
			"n":      command.NavDown,
			"N":      command.NavUp,
			"g v":    command.ReselectLast,
			"v":      command.SelectVisual,
			"esc":    command.SelectClear,
			"g a":    command.SelectAll,
			"ctrl+a": command.SelectAll,
		})

	detail := NewLayer("detail", false).
		BindAll(listMotions()).
		BindAll(patentActions()).
		BindAll(viewActions()).
		BindAll(map[string]command.ID{
			"/":  command.OpenSearch,
			";":  command.JumpMode,
			"t":  command.OpenFullText,
		})

	citations := NewLayer("citations", false).
		BindAll(listMotions()).
		BindAll(patentActions()).
		BindAll(viewActions()).
		BindAll(map[string]command.ID{
			"enter":  command.OpenDetail,
			"l":      command.OpenDetail,
			"right":  command.ColNext,
			"left":   command.ColPrev,
			".":      command.SortApply,
			"/":      command.FindOpen,
			"n":      command.NavDown,
			"N":      command.NavUp,
			"g v":    command.ReselectLast,
			"v":      command.SelectVisual,
			"g a":    command.SelectAll,
			"ctrl+a": command.SelectAll,
		})

	ids := NewLayer("ids", false).
		BindAll(listMotions()).
		BindAll(map[string]command.ID{
			"enter":  command.IDSEditField,
			"e":      command.IDSEditField,
			"f":      command.IDSToggleFull,
			"s":      command.IDSCycleStatus,
			"D":      command.IDSDelete,
			"p":      command.OpenProjects,
			"ctrl+r": command.Refresh,
		})

	projects := NewLayer("projects", false).
		BindAll(listMotions()).
		BindAll(map[string]command.ID{
			"enter":  command.ProjectActivate,
			"l":      command.ProjectActivate,
			"right":  command.ProjectActivate,
			"u":      command.ProjectClearActive,
			"n":      command.ProjectCreate,
			"I":      command.ExportIDS,
			"/":      command.OpenSearch,
			"ctrl+r": command.Refresh,
		})

	// The overlay layer is composed directly on top of the global base while an
	// overlay is focused — the App leaves the pane layer out entirely, so the
	// pane's bindings are inactive while only the global ones remain.
	fullText := NewLayer("fulltext", false).
		BindAll(listMotions()).
		BindAll(map[string]command.ID{
			";":  command.JumpMode,
			"V":  command.SelectVisual,
			"y":  command.CopyYank,
			"Y":  command.CopyYankMeta,
			"n":  command.NoteAdd,
			"N":  command.NoteOpen,
			"w":  command.OpenBrowser,
			"/":  command.OpenSearch,
			"h":  command.Back,
			"esc": command.Back,
		})

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
		scopes: map[command.Scope]*Layer{
			command.ScopeCatalog:   catalog,
			command.ScopeDetail:    detail,
			command.ScopeCitations: citations,
			command.ScopeIDS:       ids,
			command.ScopeProjects:  projects,
			command.ScopeFullText:  fullText,
			command.ScopeOverlay:   overlay,
		},
	}
}
