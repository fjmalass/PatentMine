package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

func TestPatentNoteEditorCtrlBracketVimEditDoesNotRecurse(t *testing.T) {
	o := &PatentNoteEditor{
		theme:  render.NewTheme(),
		number: domain.MustParsePatentNumber("US10000000B2"),
		value:  "hello",
	}

	_, _, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	if !handled || !o.vimMode {
		t.Fatalf("ctrl+] should enable vim mode, handled=%t vimMode=%t", handled, o.vimMode)
	}

	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if !handled || !o.vimInsert {
		t.Fatalf("i should enter vim insert mode, handled=%t vimInsert=%t", handled, o.vimInsert)
	}

	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	if !handled {
		t.Fatal("inserted rune should be handled")
	}
	if !o.dirty {
		t.Fatal("editing should mark note dirty")
	}
	if o.value != "!hello" {
		t.Fatalf("value = %q, want %q", o.value, "!hello")
	}
}
