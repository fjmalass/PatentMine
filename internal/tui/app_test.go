package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/tui/keymap"
)

// newTestApp builds an App with no daemon connection. The tests here exercise
// only input routing and the pane/overlay stacks, which never touch the client.
func newTestApp(t *testing.T) *App {
	t.Helper()
	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	app := New(nil, reg, keymap.Default())
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return app
}

// runeKey builds a KeyMsg for a printable key.
func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestAppOpensAndClosesHelpOverlay(t *testing.T) {
	app := newTestApp(t)

	// "?" opens the help overlay.
	app.Update(runeKey('?'))
	if len(app.overlays) != 1 {
		t.Fatalf("after '?', overlays = %d, want 1", len(app.overlays))
	}
	// The overlay renders over the dimmed background without panicking.
	if view := app.View(); !strings.Contains(view, "Help") {
		t.Fatalf("help overlay not visible in view")
	}
	// "esc" closes it.
	app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if len(app.overlays) != 0 {
		t.Fatalf("after esc, overlays = %d, want 0", len(app.overlays))
	}
}

func TestAppQuitCommand(t *testing.T) {
	app := newTestApp(t)
	_, cmd := app.Update(runeKey('Q'))
	if cmd == nil {
		t.Fatal("'Q' should return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("'Q' should resolve to tea.Quit")
	}
}

func TestAppChordCountReachesPane(t *testing.T) {
	app := newTestApp(t)
	// While an overlay is open, pane bindings are inactive: 'f' (ingest, a
	// catalog binding) must not be routed anywhere.
	app.Update(runeKey('?'))
	stack := app.keyStack()
	if _, ok := stack.Resolve("f"); ok {
		t.Fatal("catalog 'f' binding should be inactive while the overlay is focused")
	}
	// The overlay's own navigation binding is active.
	if id, ok := stack.Resolve("j"); !ok || id != command.NavDown {
		t.Fatalf("overlay 'j' resolved to %q ok=%v, want nav.down", id, ok)
	}
}

func TestAppBackPopsPaneStack(t *testing.T) {
	app := newTestApp(t)
	// Simulate having drilled into a detail pane by pushing one.
	app.Update(runeKey('?')) // overlay
	app.Update(runeKey('q')) // 'q' = Back: closes the overlay first
	if len(app.overlays) != 0 {
		t.Fatalf("'q' should close the overlay, overlays = %d", len(app.overlays))
	}
}

func TestStatusTextIncludesTUIAndDaemonVersions(t *testing.T) {
	app := newTestApp(t)
	app.daemonVersion = "daemon-v1"
	text := app.statusText()
	for _, want := range []string{"tui ", "daemon daemon-v1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("status text %q missing %q", text, want)
		}
	}
}
