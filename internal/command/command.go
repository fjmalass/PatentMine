// Package command is the frontend-agnostic action catalog. Every action the
// system can perform is one Command declaration. Keymaps (TUI), HTTP routes
// (web API), and the help screen are all generated from these declarations,
// so an action is defined once and every frontend stays in step.
//
// A Command carries no behavior and no display strings: it is structure only.
// A frontend's dispatcher interprets the Command.ID; human labels live in the
// text catalog, keyed by ID. The Registry is built once at startup and injected
// — there is deliberately no package-level mutable state.
package command

import (
	"fmt"
	"maps"
	"slices"
	"strings"

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

// Scope names a UI situation — which pane or overlay is active. Commands are
// scoped to scopes so a keymap can offer the right actions and the help
// screen can group them.
type Scope string

const (
	ScopeCatalog   Scope = "catalog"   // the main patent list
	ScopeDetail    Scope = "detail"    // one patent's detail view
	ScopeCitations Scope = "citations" // a citations / cited-by list
	ScopeFamily    Scope = "family"    // a bounded family DAG view
	ScopeIDS       Scope = "ids"       // one patent's IDS entry editor
	ScopeProjects  Scope = "projects"  // the project list
	ScopeFullText  Scope = "fulltext"  // full claims text viewer
	ScopeOverlay   Scope = "overlay"   // a modal overlay is focused
)

// Command is the declaration of one action.
type Command struct {
	// ID uniquely identifies the command.
	ID ID
	// Name is the canonical typed-command form, such as "open.projects".
	// Empty means the command is not exposed in the command palette/prompt.
	Name string
	// Aliases are additional typed forms accepted by the command prompt.
	Aliases []string
	// Usage is a concise invocation example shown in help/palette footers. It
	// is command syntax, not prose, so it stays a literal rather than a key.
	Usage string
	// Kind says whether the command hits the daemon or only the local view.
	Kind Kind
	// Method is the proto method a KindEngine command maps to; empty for
	// KindView commands.
	Method proto.Method
	// Scopes lists the scopes in which the command is offered. An empty
	// Scopes means the command is global (valid in every scope).
	Scopes []Scope
}

// Global reports whether the command applies in every scope.
func (c Command) Global() bool {
	return len(c.Scopes) == 0
}

// AvailableIn reports whether the command is offered in scope.
func (c Command) AvailableIn(scope Scope) bool {
	if c.Global() {
		return true
	}
	return slices.Contains(c.Scopes, scope)
}

// Registry is an immutable, injected catalog of commands. Build it once with
// NewRegistry and pass it to frontends; never store one in a package global.
type Registry struct {
	byID    map[ID]Command
	byName  map[string]ID
	ordered []Command
}

// NewRegistry builds a Registry, rejecting duplicate IDs and malformed
// commands so a wiring mistake fails loudly at startup.
func NewRegistry(commands ...Command) (*Registry, error) {
	r := &Registry{
		byID:    make(map[ID]Command, len(commands)),
		byName:  make(map[string]ID, len(commands)),
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
		if err := validateTypedNames(r.byName, c); err != nil {
			return nil, err
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

// InScope returns the commands offered in scope, in registration order.
func (r *Registry) InScope(scope Scope) []Command {
	var out []Command
	for _, c := range r.ordered {
		if c.AvailableIn(scope) {
			out = append(out, c)
		}
	}
	return out
}

// TypedInScope returns commands that are offered in scope and exposed in the
// command palette/prompt.
func (r *Registry) TypedInScope(scope Scope) []Command {
	var out []Command
	for _, c := range r.InScope(scope) {
		if c.Name != "" {
			out = append(out, c)
		}
	}
	return out
}

// LookupName resolves a canonical or alias typed command.
func (r *Registry) LookupName(name string) (Command, bool) {
	id, ok := r.byName[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return Command{}, false
	}
	return r.byID[id], true
}

// Len reports how many commands the registry holds.
func (r *Registry) Len() int {
	return len(r.ordered)
}

func validateTypedNames(index map[string]ID, c Command) error {
	seen := maps.Clone(index)
	register := func(name string) error {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			return nil
		}
		if prior, dup := seen[key]; dup {
			return fmt.Errorf("command: typed name %q for %q conflicts with %q", key, c.ID, prior)
		}
		seen[key] = c.ID
		index[key] = c.ID
		return nil
	}
	if err := register(c.Name); err != nil {
		return err
	}
	for _, alias := range c.Aliases {
		if err := register(alias); err != nil {
			return err
		}
	}
	return nil
}
