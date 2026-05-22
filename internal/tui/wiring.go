package tui

import (
	"errors"
	"fmt"
	"slices"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/text"
	"patentmine/internal/tui/keymap"
	"patentmine/internal/tui/overlay"
	"patentmine/internal/tui/pane"
	"patentmine/internal/tui/render"
)

// validateWiring fails the boot when the keymap, command registry, handlers,
// and text catalog are not consistent. It is the structural guarantee behind
// the input model: a bound key or typed command can never resolve to a command
// that no handler services, and every command has display strings.
func validateWiring(reg *command.Registry, keymaps *keymap.Keymaps, catalog *text.Catalog) error {
	if reg == nil || keymaps == nil || catalog == nil {
		return errors.New("tui: New requires a registry, keymap, and text catalog")
	}
	paneHandled := paneHandlerSets()
	overlayHandled := overlayHandlerSet(reg, keymaps, catalog)

	// dispatchable reports whether command id reaches a handler in scope: either
	// the App table, or the pane/overlay that owns that scope.
	dispatchable := func(id command.ID, scope command.Scope) bool {
		if _, ok := appHandlers[id]; ok {
			return true
		}
		if scope == command.ScopeOverlay {
			return slices.Contains(overlayHandled, id)
		}
		return slices.Contains(paneHandled[scope], id)
	}

	var errs []error

	// Every key binding must resolve to a registered, dispatchable command.
	checkLayer := func(scope command.Scope, layer *keymap.Layer, global bool) {
		if layer == nil {
			return
		}
		for seq, id := range layer.Bindings() {
			if _, ok := reg.Lookup(id); !ok {
				errs = append(errs, fmt.Errorf("keymap: %q binds %q to unregistered command %q", scope, seq, id))
				continue
			}
			if global {
				// A global key is active in every scope, so only the App
				// table can service it.
				if _, ok := appHandlers[id]; !ok {
					errs = append(errs, fmt.Errorf("keymap: global key %q binds %q, which no App handler services", seq, id))
				}
				continue
			}
			if !dispatchable(id, scope) {
				errs = append(errs, fmt.Errorf("keymap: %q key %q binds %q, which no handler services there", scope, seq, id))
			}
		}
	}
	checkLayer("global", keymaps.Base(), true)
	for scope, layer := range keymaps.ScopeLayers() {
		checkLayer(scope, layer, false)
	}

	// Every typed command must be dispatchable in each scope it is offered in,
	// so the command prompt and palette can never invoke a dead command.
	for _, c := range reg.All() {
		if c.Name == "" {
			continue
		}
		for _, scope := range typedCheckScope(c) {
			if !dispatchable(c.ID, scope) {
				errs = append(errs, fmt.Errorf("command: typed command %q is offered in %q but no handler services it there", c.Name, scope))
			}
		}
	}

	// Every command must have title and help strings in the catalog.
	for _, c := range reg.All() {
		if !catalog.Has(text.CmdTitle(string(c.ID))) {
			errs = append(errs, fmt.Errorf("text: catalog has no title for command %q", c.ID))
		}
		if !catalog.Has(text.CmdHelp(string(c.ID))) {
			errs = append(errs, fmt.Errorf("text: catalog has no help for command %q", c.ID))
		}
	}

	return errors.Join(errs...)
}

// paneScopes are the scopes backed by a focusable pane.
var paneScopes = []command.Scope{
	command.ScopeCatalog, command.ScopeDetail,
	command.ScopeCitations, command.ScopeFamily, command.ScopeIDS, command.ScopeProjects,
	command.ScopeFullText,
}

// typedCheckScope returns the scopes in which a typed command must be
// dispatchable. A global command is checked against every pane scope (where
// dispatchable resolves it through the App table); a scoped command against its
// own pane scopes.
func typedCheckScope(c command.Command) []command.Scope {
	if c.Global() {
		return paneScopes
	}
	var out []command.Scope
	for _, sc := range c.Scopes {
		if sc == command.ScopeOverlay {
			continue // overlay-scoped typed commands run in their source scope
		}
		out = append(out, sc)
	}
	return out
}

// paneHandlerSets builds a sample of every pane and records the command IDs it
// services, keyed by the pane's context.
func paneHandlerSets() map[command.Scope][]command.ID {
	theme := render.NewTheme()
	panes := []pane.Pane{
		pane.NewCatalog(nil, theme),
		pane.NewDetail(nil, theme, domain.PatentNumber{}, "", nil),
		pane.NewCitations(nil, theme, domain.PatentNumber{}, domain.RelationCites),
		pane.NewFamilyGraph(nil, theme, domain.PatentNumber{}, 0, nil),
		pane.NewIDSDetail(nil, theme, domain.PatentNumber{}, ""),
		pane.NewProjects(nil, theme, "", ""),
		pane.NewFullText(nil, theme, domain.PatentNumber{}, "", nil),
	}
	out := make(map[command.Scope][]command.ID, len(panes))
	for _, p := range panes {
		out[p.Scope()] = p.Handles()
	}
	return out
}

// overlayHandlerSet returns the union of command IDs serviced by any overlay.
// KeyHandler overlays consume input before the keymap, so the overlay keymap
// layer only governs passive overlays — a union is the right test here.
func overlayHandlerSet(reg *command.Registry, keymaps *keymap.Keymaps, catalog *text.Catalog) []command.ID {
	theme := render.NewTheme()
	overlays := []overlay.Overlay{
		overlay.NewHelp(reg, keymaps, theme, catalog),
		overlay.NewPrompt(reg, keymaps, theme, catalog, command.ScopeCatalog, overlay.PromptPalette),
		overlay.NewTextInput(theme, catalog, overlay.PurposeCreateProject, text.NewProjectTitle, text.NewProjectCaption),
	}
	var out []command.ID
	for _, ov := range overlays {
		for _, id := range ov.Handles() {
			if !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	return out
}
