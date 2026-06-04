package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// Compile-time proof both overlays are usable, key-consuming overlays.
var (
	_ Overlay    = (*OfficeActionList)(nil)
	_ KeyHandler = (*OfficeActionList)(nil)
	_ Overlay    = (*OfficeActionEditor)(nil)
	_ KeyHandler = (*OfficeActionEditor)(nil)
)

func TestOfficeActionEditorCtrlWSwitchesFocus(t *testing.T) {
	oa := domain.OfficeAction{ID: "oa-1", ExtractedText: "Claims 1-3 rejected.", Notes: "traverse"}
	o := NewOfficeActionEditor(nil, render.NewTheme(), oa)
	if !o.focusNotes {
		t.Fatal("editor should start focused on the notes pane")
	}
	// ctrl-w h → examiner (left)
	o.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlW})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if o.focusNotes {
		t.Fatal("ctrl-w h should focus the examiner (left) pane")
	}
	// ctrl-w l → notes (right)
	o.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlW})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if !o.focusNotes {
		t.Fatal("ctrl-w l should focus the notes (right) pane")
	}
}

func TestOfficeActionEditorExaminerReadOnly(t *testing.T) {
	oa := domain.OfficeAction{ID: "oa-1", ExtractedText: "original", Notes: ""}
	o := NewOfficeActionEditor(nil, render.NewTheme(), oa)
	// Focus the examiner pane and try to edit it.
	o.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlW})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")}) // try insert
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Z")})
	if o.examiner.Value() != "original" {
		t.Fatalf("examiner pane must be read-only, got %q", o.examiner.Value())
	}
}

func TestOfficeActionEditorEscCloses(t *testing.T) {
	o := NewOfficeActionEditor(nil, render.NewTheme(), domain.OfficeAction{ID: "oa-1"})
	_, cmd, consumed := o.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !consumed || cmd == nil {
		t.Fatal("esc should be consumed and emit a command")
	}
	if _, ok := cmd().(CloseOverlayMsg); !ok {
		t.Fatalf("esc in normal mode should close, got %T", cmd())
	}
}

func TestOfficeActionListEnterOpens(t *testing.T) {
	o := NewOfficeActionList(nil, render.NewTheme(), "p1")
	o.loading = false
	o.items = []domain.OfficeAction{{ID: "oa-A"}, {ID: "oa-B"}}
	o.cursor = 1

	_, cmd, consumed := o.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !consumed || cmd == nil {
		t.Fatal("enter should be consumed and emit a command")
	}
	msg, ok := cmd().(OpenOfficeActionMsg)
	if !ok {
		t.Fatalf("enter should emit OpenOfficeActionMsg, got %T", cmd())
	}
	if msg.OA.ID != "oa-B" {
		t.Fatalf("enter should open the cursor row, got %q", msg.OA.ID)
	}
}
