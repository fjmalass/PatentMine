package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func semicolon() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(";")} }

// TestJumpNavigatorEmptyListDoesNotPanic guards the regression where a movement
// key in jump mode over an empty list divided by zero (numFields == 0).
func TestJumpNavigatorEmptyListDoesNotPanic(t *testing.T) {
	var j JumpNavigator
	if _, handled := j.HandleKey(semicolon(), 0, 0); !handled || !j.Active {
		t.Fatalf("; should activate jump mode (handled=%v active=%v)", handled, j.Active)
	}
	// j / k / G / enter over an empty list must be consumed without panicking.
	for _, key := range []string{"j", "k", "G", "enter"} {
		j.Active = true
		focus, handled := j.HandleKey(runes(key), 0, 0)
		if !handled || focus != 0 {
			t.Fatalf("key %q on empty list: focus=%d handled=%v, want 0/true", key, focus, handled)
		}
		if j.Active {
			t.Fatalf("key %q should have exited jump mode on an empty list", key)
		}
	}
}

func TestJumpNavigatorJumpToLineAndMove(t *testing.T) {
	var j JumpNavigator
	// ";" then "3" then enter → focus index 2 (1-based line 3).
	j.HandleKey(semicolon(), 0, 5)
	j.HandleKey(runes("3"), 0, 5)
	focus, handled := j.HandleKey(runes("enter"), 0, 5)
	if !handled || focus != 2 {
		t.Fatalf("jump to line 3 → focus=%d handled=%v, want 2/true", focus, handled)
	}

	// ";" then "j" moves down one (wrapping via modulo).
	j.HandleKey(semicolon(), 4, 5)
	focus, _ = j.HandleKey(runes("j"), 4, 5)
	if focus != 0 {
		t.Fatalf("j from last index should wrap to 0, got %d", focus)
	}

	// G jumps to the last field.
	j.HandleKey(semicolon(), 0, 5)
	focus, _ = j.HandleKey(runes("G"), 0, 5)
	if focus != 4 {
		t.Fatalf("G → last index, want 4 got %d", focus)
	}
}
