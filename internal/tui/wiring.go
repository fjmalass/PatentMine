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

	// dispatchable reports whether command id reaches a handler in ctx: either
	// the App table, or the pane/overlay that owns that context.
	dispatchable := func(id command.ID, ctx command.Context) bool {
		if _, ok := appHandlers[id]; ok {
			return true
		}
		if ctx == command.ContextOverlay {
			return slices.Contains(overlayHandled, id)
		}
		return slices.Contains(paneHandled[ctx], id)
	}

	var errs []error

	// Every key binding must resolve to a registered, dispatchable command.
	checkLayer := func(ctx command.Context, layer *keymap.Layer, global bool) {
		if layer == nil {
			return
		}
		for seq, id := range layer.Bindings() {
			if _, ok := reg.Lookup(id); !ok {
				errs = append(errs, fmt.Errorf("keymap: %q binds %q to unregistered command %q", ctx, seq, id))
				continue
			}
			if global {
				// A global key is active in every context, so only the App
				// table can service it.
				if _, ok := appHandlers[id]; !ok {
					errs = append(errs, fmt.Errorf("keymap: global key %q binds %q, which no App handler services", seq, id))
				}
				continue
			}
			if !dispatchable(id, ctx) {
				errs = append(errs, fmt.Errorf("keymap: %q key %q binds %q, which no handler services there", ctx, seq, id))
			}
		}
	}
	checkLayer("global", keymaps.Base(), true)
	for ctx, layer := range keymaps.ContextLayers() {
		checkLayer(ctx, layer, false)
	}

	// Every typed command must be dispatchable in each context it is offered in,
	// so the command prompt and palette can never invoke a dead command.
	for _, c := range reg.All() {
		if c.Name == "" {
			continue
		}
		for _, ctx := range typedCheckContexts(c) {
			if !dispatchable(c.ID, ctx) {
				errs = append(errs, fmt.Errorf("command: typed command %q is offered in %q but no handler services it there", c.Name, ctx))
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

// paneContexts are the contexts backed by a focusable pane.
var paneContexts = []command.Context{
	command.ContextCatalog, command.ContextDetail,
	command.ContextCitations, command.ContextProjects,
}

// typedCheckContexts returns the contexts in which a typed command must be
// dispatchable. A global command is checked against every pane context (where
// dispatchable resolves it through the App table); a scoped command against its
// own pane scopes.
func typedCheckContexts(c command.Command) []command.Context {
	if c.Global() {
		return paneContexts
	}
	var out []command.Context
	for _, ctx := range c.Scopes {
		if ctx == command.ContextOverlay {
			continue // overlay-scoped typed commands run in their source context
		}
		out = append(out, ctx)
	}
	return out
}

// paneHandlerSets builds a sample of every pane and records the command IDs it
// services, keyed by the pane's context.
func paneHandlerSets() map[command.Context][]command.ID {
	theme := render.NewTheme()
	panes := []pane.Pane{
		pane.NewCatalog(nil, theme),
		pane.NewDetail(nil, theme, domain.PatentNumber{}),
		pane.NewCitations(nil, theme, domain.PatentNumber{}, domain.RelationCites),
		pane.NewProjects(nil, theme),
	}
	out := make(map[command.Context][]command.ID, len(panes))
	for _, p := range panes {
		out[p.Context()] = p.Handles()
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
		overlay.NewPrompt(reg, keymaps, theme, catalog, command.ContextCatalog, overlay.PromptPalette),
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
