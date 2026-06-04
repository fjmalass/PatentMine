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
		buf:    newVimBuffer("hello"),
	}

	_, _, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	if !handled || !o.buf.vimMode {
		t.Fatalf("ctrl+] should enable vim mode, handled=%t vimMode=%t", handled, o.buf.vimMode)
	}

	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	if !handled || !o.buf.vimInsert {
		t.Fatalf("i should enter vim insert mode, handled=%t vimInsert=%t", handled, o.buf.vimInsert)
	}

	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("!")})
	if !handled {
		t.Fatal("inserted rune should be handled")
	}
	if !o.buf.dirty {
		t.Fatal("editing should mark note dirty")
	}
	if o.buf.Value() != "!hello" {
		t.Fatalf("value = %q, want %q", o.buf.Value(), "!hello")
	}
}
