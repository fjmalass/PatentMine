// Package command is the frontend-agnostic action catalog. Every action the
// system can perform is one Command declaration. Keymaps (TUI), HTTP routes
// (web API), and the help screen are all generated from these declarations,
// so an action is defined once and every frontend stays in step.
//
// A Command carries no behavior: it is data. A frontend's dispatcher interprets
// the Command.ID. The Registry is built once at startup and injected — there
// is deliberately no package-level mutable state.
package command

import (
	"fmt"
	"slices"

	"patentmine/internal/proto"
)

// ID is a stable, typed command identifier. Every ID is a const in catalog.go;
// nothing spells one as a bare string.
type ID string

// Kind classifies how a command is carried out.
type Kind string

const (
	// KindView changes only client-local view state (scrolling, opening an
	// overlay, switching panes). It never touches the daemon.
	KindView Kind = "view"
	// KindEngine results in a request to the daemon over the proto layer.
	KindEngine Kind = "engine"
)

// Context names a UI situation — which pane or overlay is active. Commands are
// scoped to contexts so a keymap can offer the right actions and the help
// screen can group them.
type Context string

const (
	ContextCatalog   Context = "catalog"   // the main patent list
	ContextDetail    Context = "detail"    // one patent's detail view
	ContextCitations Context = "citations" // a citations / cited-by list
	ContextProjects  Context = "projects"  // the project list
	ContextOverlay   Context = "overlay"   // a modal overlay is focused
)

// Command is the declaration of one action.
type Command struct {
	// ID uniquely identifies the command.
	ID ID
	// Title is a short human label (used in help and the web API).
	Title string
	// Help is a one-line description of what the command does.
	Help string
	// Kind says whether the command hits the daemon or only the local view.
	Kind Kind
	// Method is the proto method a KindEngine command maps to; empty for
	// KindView commands.
	Method proto.Method
	// Scopes lists the contexts in which the command is offered. An empty
	// Scopes means the command is global (valid in every context).
	Scopes []Context
}

// Global reports whether the command applies in every context.
func (c Command) Global() bool {
	return len(c.Scopes) == 0
}

// AvailableIn reports whether the command is offered in ctx.
func (c Command) AvailableIn(ctx Context) bool {
	if c.Global() {
		return true
	}
	return slices.Contains(c.Scopes, ctx)
}

// Registry is an immutable, injected catalog of commands. Build it once with
// NewRegistry and pass it to frontends; never store one in a package global.
type Registry struct {
	byID    map[ID]Command
	ordered []Command
}

// NewRegistry builds a Registry, rejecting duplicate IDs and malformed
// commands so a wiring mistake fails loudly at startup.
func NewRegistry(commands ...Command) (*Registry, error) {
	r := &Registry{
		byID:    make(map[ID]Command, len(commands)),
		ordered: make([]Command, 0, len(commands)),
	}
	for _, c := range commands {
		if c.ID == "" {
			return nil, fmt.Errorf("command: a command has an empty ID")
		}
		if _, dup := r.byID[c.ID]; dup {
			return nil, fmt.Errorf("command: duplicate command ID %q", c.ID)
		}
		if c.Kind == KindEngine && c.Method == "" {
			return nil, fmt.Errorf("command: engine command %q has no method", c.ID)
		}
		if c.Kind == KindView && c.Method != "" {
			return nil, fmt.Errorf("command: view command %q must not set a method", c.ID)
		}
		r.byID[c.ID] = c
		r.ordered = append(r.ordered, c)
	}
	return r, nil
}

// Lookup returns the command for id.
func (r *Registry) Lookup(id ID) (Command, bool) {
	c, ok := r.byID[id]
	return c, ok
}

// All returns every command in registration order.
func (r *Registry) All() []Command {
	out := make([]Command, len(r.ordered))
	copy(out, r.ordered)
	return out
}

// InContext returns the commands offered in ctx, in registration order.
func (r *Registry) InContext(ctx Context) []Command {
	var out []Command
	for _, c := range r.ordered {
		if c.AvailableIn(ctx) {
			out = append(out, c)
		}
	}
	return out
}

// Len reports how many commands the registry holds.
func (r *Registry) Len() int {
	return len(r.ordered)
}
