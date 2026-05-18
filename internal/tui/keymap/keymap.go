// Package keymap maps key sequences to commands as a stack of layers. An
// overlay pushes a blocking layer that overrides — and masks — the layers
// beneath it, while the layers beneath keep rendering as a dimmed background.
// The keymap is built from data and injected; there is no global keymap state.
package keymap

import (
	"maps"
	"slices"
	"strings"

	"patentmine/internal/command"
	"patentmine/internal/keys"
)

// Layer is one named set of key-sequence bindings. A Blocking layer (used by
// overlays) stops resolution from falling through to the layers below it.
type Layer struct {
	Name     string
	Blocking bool
	bindings map[string]command.ID
}

// NewLayer creates an empty layer.
func NewLayer(name string, blocking bool) *Layer {
	return &Layer{Name: name, Blocking: blocking, bindings: make(map[string]command.ID)}
}

// Bind maps a canonical key sequence (e.g. "g g", "ctrl+d") to a command. It
// returns the layer so binds can be chained.
func (l *Layer) Bind(sequence string, id command.ID) *Layer {
	l.bindings[sequence] = id
	return l
}

// BindAll applies a set of bindings at once.
func (l *Layer) BindAll(bindings map[string]command.ID) *Layer {
	maps.Copy(l.bindings, bindings)
	return l
}

// Bindings returns a copy of the layer's sequence-to-command map.
func (l *Layer) Bindings() map[string]command.ID {
	return maps.Clone(l.bindings)
}

// Stack is the ordered set of active layers. The last pushed layer has the
// highest priority. Resolution and matching honour Blocking layers.
type Stack struct {
	layers []*Layer
}

// NewStack creates a stack from base layers, lowest priority first.
func NewStack(base ...*Layer) *Stack {
	return &Stack{layers: append([]*Layer(nil), base...)}
}

// Push adds a layer at the top (highest priority).
func (s *Stack) Push(l *Layer) {
	s.layers = append(s.layers, l)
}

// Pop removes the top layer, if any.
func (s *Stack) Pop() {
	if n := len(s.layers); n > 0 {
		s.layers = s.layers[:n-1]
	}
}

// Depth reports how many layers are stacked.
func (s *Stack) Depth() int {
	return len(s.layers)
}

// active returns the participating layers, top priority first: every layer
// from the top down to and including the first Blocking layer (all layers when
// none blocks).
func (s *Stack) active() []*Layer {
	var out []*Layer
	for i := len(s.layers) - 1; i >= 0; i-- {
		out = append(out, s.layers[i])
		if s.layers[i].Blocking {
			break
		}
	}
	return out
}

// Resolve returns the command bound to a complete key sequence.
func (s *Stack) Resolve(sequence string) (command.ID, bool) {
	for _, l := range s.active() {
		if id, ok := l.bindings[sequence]; ok {
			return id, true
		}
	}
	return "", false
}

// Match classifies a candidate sequence for the input Reader. It is the Lookup
// the keys.Reader expects.
func (s *Stack) Match(sequence string) keys.Match {
	var complete, prefix bool
	for _, l := range s.active() {
		for bound := range l.bindings {
			switch {
			case bound == sequence:
				complete = true
			case strings.HasPrefix(bound, sequence+" "):
				prefix = true
			}
		}
	}
	switch {
	case complete && prefix:
		return keys.CompleteAndPrefix
	case complete:
		return keys.Complete
	case prefix:
		return keys.Prefix
	default:
		return keys.NoMatch
	}
}

// Shortcuts returns the active key sequences that invoke id in ctx.
func (k *Keymaps) Shortcuts(ctx command.Context, id command.ID) []string {
	seen := map[string]bool{}
	var out []string
	collect := func(layer *Layer) {
		if layer == nil {
			return
		}
		for seq, bound := range layer.bindings {
			if bound != id || seen[seq] {
				continue
			}
			seen[seq] = true
			out = append(out, seq)
		}
	}
	collect(k.base)
	collect(k.contexts[ctx])
	slices.Sort(out)
	return out
}
