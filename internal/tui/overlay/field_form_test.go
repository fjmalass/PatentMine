package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/tui/render"
)

// newTestForm builds a 3-field form: two free-text fields and one choice field
// that toggles between "x" and "y".
func newTestForm() fieldForm {
	return newFieldForm([]fieldFormField{
		{Label: "A", Kind: fieldText},
		{Label: "B", Kind: fieldText},
		{Label: "C", Kind: fieldChoice, Cycle: func(cur, _ string) string {
			if cur == "x" {
				return "y"
			}
			return "x"
		}},
	})
}

func TestFieldFormViewModeIgnoresTyping(t *testing.T) {
	f := newTestForm()
	f.SetValue(0, "hi")
	if a, _ := f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")}); a != fieldFormNone {
		t.Fatalf("typing in view mode should be a no-op, got action %v", a)
	}
	if f.Value(0) != "hi" {
		t.Fatalf("view-mode typing must not edit, got %q", f.Value(0))
	}
	if f.Editing() {
		t.Fatal("a stray key must not enter edit mode")
	}
}

func TestFieldFormNavigation(t *testing.T) {
	f := newTestForm()
	f.HandleKey(tea.KeyMsg{Type: tea.KeyDown})
	if f.Focus() != 1 {
		t.Fatalf("down → 1, got %d", f.Focus())
	}
	f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if f.Focus() != 2 {
		t.Fatalf("j → 2, got %d", f.Focus())
	}
	f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if f.Focus() != 0 {
		t.Fatalf("j should wrap to 0, got %d", f.Focus())
	}
	f.HandleKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	if f.Focus() != 2 {
		t.Fatalf("shift-tab should wrap to 2, got %d", f.Focus())
	}
}

func TestFieldFormEditInsertAtCursor(t *testing.T) {
	f := newTestForm()
	f.SetValue(0, "ac")
	f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !f.Editing() {
		t.Fatal("enter should start editing")
	}
	// Cursor starts at the end (2); move left to sit between 'a' and 'c'.
	f.HandleKey(tea.KeyMsg{Type: tea.KeyLeft})
	f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if f.Value(0) != "abc" {
		t.Fatalf("expected insert at cursor → abc, got %q", f.Value(0))
	}
	f.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if f.Value(0) != "ac" {
		t.Fatalf("expected backspace at cursor → ac, got %q", f.Value(0))
	}
	f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if f.Editing() {
		t.Fatal("enter should commit and leave edit mode")
	}
}

func TestFieldFormEscDiscardsEdit(t *testing.T) {
	f := newTestForm()
	f.SetValue(0, "orig")
	f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	f.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	if f.Value(0) != "origX" {
		t.Fatalf("expected origX while editing, got %q", f.Value(0))
	}
	if a, _ := f.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}); a != fieldFormNone {
		t.Fatalf("esc while editing should not cancel the form, got %v", a)
	}
	if f.Editing() {
		t.Fatal("esc should leave edit mode")
	}
	if f.Value(0) != "orig" {
		t.Fatalf("esc should restore the pre-edit value, got %q", f.Value(0))
	}
}

func TestFieldFormSubmitAndCancelActions(t *testing.T) {
	f := newTestForm()
	if a, _ := f.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlS}); a != fieldFormSubmit {
		t.Fatalf("ctrl+s in view mode should submit, got %v", a)
	}
	if a, _ := f.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}); a != fieldFormCancel {
		t.Fatalf("esc in view mode should cancel, got %v", a)
	}
}

func TestFieldFormChoiceCycle(t *testing.T) {
	f := newTestForm()
	f.focus = 2
	f.SetValue(2, "x")
	f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	f.HandleKey(tea.KeyMsg{Type: tea.KeySpace})
	if f.Value(2) != "y" {
		t.Fatalf("space should cycle the choice x→y, got %q", f.Value(2))
	}
	// Backspace must not corrupt a choice field.
	f.HandleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if f.Value(2) != "y" {
		t.Fatalf("backspace must not change a choice field, got %q", f.Value(2))
	}
}

func TestFieldFormRenderShowsValue(t *testing.T) {
	f := newTestForm()
	f.SetValue(0, "hello")
	f.HandleKey(tea.KeyMsg{Type: tea.KeyEnter}) // edit field 0 with cursor at end
	out := f.RenderFields(render.NewTheme(), 80, 10)
	if !strings.Contains(out, "hello") {
		t.Fatalf("render should contain the field value, got %q", out)
	}
}
