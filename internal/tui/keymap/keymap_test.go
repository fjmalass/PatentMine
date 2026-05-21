package keymap

import (
	"testing"

	"patentmine/internal/command"
	"patentmine/internal/keys"
)

func TestStackResolvesAcrossLayers(t *testing.T) {
	base := NewLayer("base", false).Bind("Q", command.Quit)
	catalog := NewLayer("catalog", false).Bind("j", command.NavDown)
	stack := NewStack(base, catalog)

	if id, ok := stack.Resolve("j"); !ok || id != command.NavDown {
		t.Fatalf("resolve j = %q ok=%v, want nav.down", id, ok)
	}
	// A binding only in the base layer still resolves through the upper layer.
	if id, ok := stack.Resolve("Q"); !ok || id != command.Quit {
		t.Fatalf("resolve Q = %q ok=%v, want app.quit", id, ok)
	}
}

func TestHigherLayerOverridesLower(t *testing.T) {
	base := NewLayer("base", false).Bind("j", command.NavDown)
	top := NewLayer("top", false).Bind("j", command.NavUp)
	stack := NewStack(base, top)

	if id, _ := stack.Resolve("j"); id != command.NavUp {
		t.Fatalf("resolve j = %q, want the higher layer's nav.up", id)
	}
}

func TestBlockingOverlayMasksLowerLayers(t *testing.T) {
	catalog := NewLayer("catalog", false).Bind("j", command.NavDown).Bind("f", command.IngestFamily)
	overlay := NewLayer("overlay", true).Bind("esc", command.CloseOverlay)
	stack := NewStack(catalog)
	stack.Push(overlay)

	// The overlay's own binding resolves.
	if id, ok := stack.Resolve("esc"); !ok || id != command.CloseOverlay {
		t.Fatalf("resolve esc = %q ok=%v, want view.close-overlay", id, ok)
	}
	// A binding from the masked catalog layer must NOT fall through.
	if _, ok := stack.Resolve("f"); ok {
		t.Fatal("blocking overlay should mask the catalog layer's 'f' binding")
	}

	// Popping the overlay restores the catalog bindings.
	stack.Pop()
	if id, ok := stack.Resolve("f"); !ok || id != command.IngestFamily {
		t.Fatalf("after pop, resolve f = %q ok=%v, want ingest.family", id, ok)
	}
}

func TestMatchClassifiesSequences(t *testing.T) {
	layer := NewLayer("catalog", false).Bind("j", command.NavDown).Bind("g g", command.NavTop)
	stack := NewStack(layer)

	if m := stack.Match("j"); m != keys.Complete {
		t.Errorf("Match(j) = %v, want Complete", m)
	}
	if m := stack.Match("g"); m != keys.Prefix {
		t.Errorf("Match(g) = %v, want Prefix", m)
	}
	if m := stack.Match("g g"); m != keys.Complete {
		t.Errorf("Match(g g) = %v, want Complete", m)
	}
	if m := stack.Match("z"); m != keys.NoMatch {
		t.Errorf("Match(z) = %v, want NoMatch", m)
	}
}

func TestDefaultKeymapDrivesTheReader(t *testing.T) {
	km := Default()
	stack := km.StackFor(command.ScopeCatalog)

	// Drive a Vim-style "3j" through the Reader against the real keymap.
	var r keys.Reader
	if _, ok := r.Feed("3", stack.Match); ok {
		t.Fatal("count digit should not complete")
	}
	chord, ok := r.Feed("j", stack.Match)
	if !ok {
		t.Fatal("3j should complete a chord against the default catalog keymap")
	}
	if chord.Repeat() != 3 {
		t.Fatalf("repeat = %d, want 3", chord.Repeat())
	}
	id, ok := stack.Resolve(chord.Sequence())
	if !ok || id != command.NavDown {
		t.Fatalf("resolve %q = %q ok=%v, want nav.down", chord.Sequence(), id, ok)
	}

	// "g g" is a multi-key motion in the default keymap.
	r.Reset()
	r.Feed("g", stack.Match)
	ggChord, ok := r.Feed("g", stack.Match)
	if !ok {
		t.Fatal("g g should complete against the default keymap")
	}
	if id, _ := stack.Resolve(ggChord.Sequence()); id != command.NavTop {
		t.Fatalf("g g resolved to %q, want nav.top", id)
	}
}

func TestDefaultOverlayStackExcludesPaneBindings(t *testing.T) {
	// While an overlay is focused the App composes base + overlay only — the
	// pane layer is left out, so a pane binding like 'f' is simply inactive.
	km := Default()
	stack := NewStack(km.Base())
	stack.Push(km.Scope(command.ScopeOverlay))

	if _, ok := stack.Resolve("f"); ok {
		t.Fatal("the catalog 'f' binding must be inactive while an overlay is focused")
	}
	// The overlay's own binding and the global bindings both still resolve.
	if id, _ := stack.Resolve("esc"); id != command.CloseOverlay {
		t.Fatalf("esc in overlay resolved to %q, want view.close-overlay", id)
	}
	if id, _ := stack.Resolve("Q"); id != command.Quit {
		t.Fatalf("global Q resolved to %q while overlay focused, want app.quit", id)
	}
}

func TestShortcutsCollectBaseAndContextBindings(t *testing.T) {
	km := Default()
	shortcuts := km.Shortcuts(command.ScopeProjects, command.ProjectActivate)
	if len(shortcuts) != 3 || shortcuts[0] != "enter" || shortcuts[1] != "l" || shortcuts[2] != "right" {
		t.Fatalf("Shortcuts(ProjectActivate) = %v, want [enter l right]", shortcuts)
	}
}
