package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func TestPlainModeJKInserts(t *testing.T) {
	// In plain (non-vim) mode, j and k are literal text, not motion — so a word
	// like "junk" types correctly. (The old behaviour moved the cursor.)
	b := newVimBuffer("")
	for _, ch := range []string{"j", "u", "n", "k"} {
		b.handleKey(runes(ch))
	}
	if b.Value() != "junk" {
		t.Fatalf("plain-mode typing = %q, want %q", b.Value(), "junk")
	}
}

func TestVimNormalJKNavigates(t *testing.T) {
	b := newVimBuffer("line1\nline2")
	b.vimMode = true // opens in NORMAL
	b.line, b.column = 0, 0
	if _, _ = b.handleKey(runes("j")); b.line != 1 {
		t.Fatalf("vim normal j should move down, line=%d", b.line)
	}
	if _, _ = b.handleKey(runes("k")); b.line != 0 {
		t.Fatalf("vim normal k should move up, line=%d", b.line)
	}
	// j/k must not have inserted any text.
	if b.Value() != "line1\nline2" {
		t.Fatalf("navigation mutated text: %q", b.Value())
	}
}

func TestReadOnlyBlocksEditsAllowsMotion(t *testing.T) {
	b := newVimBuffer("alpha\nbeta")
	b.vimMode = true
	b.readOnly = true
	b.line, b.column = 0, 0

	// Motions work.
	b.handleKey(runes("j"))
	if b.line != 1 {
		t.Fatalf("read-only j should move, line=%d", b.line)
	}
	// Edit commands are inert: x, dd, and entering insert + typing.
	b.handleKey(runes("x"))
	b.handleKey(runes("d"))
	b.handleKey(runes("d"))
	b.handleKey(runes("i"))
	if b.vimInsert {
		t.Fatal("read-only buffer must not enter insert mode")
	}
	b.handleKey(runes("Z"))
	if b.Value() != "alpha\nbeta" {
		t.Fatalf("read-only buffer was mutated: %q", b.Value())
	}
	if b.dirty {
		t.Fatal("read-only buffer should never be dirty")
	}
}

func TestYankPasteWithinBuffer(t *testing.T) {
	b := newVimBuffer("one\ntwo\nthree")
	b.vimMode = true
	b.line, b.column = 0, 0

	// yy on line 0, then p pastes a copy of "one" below it.
	b.handleKey(runes("y"))
	b.handleKey(runes("y"))
	b.handleKey(runes("p"))
	if want := "one\none\ntwo\nthree"; b.Value() != want {
		t.Fatalf("yy/p = %q, want %q", b.Value(), want)
	}
	if b.line != 1 {
		t.Fatalf("after paste cursor line = %d, want 1 (first pasted line)", b.line)
	}
}

func TestYankFromReadOnlyPasteIntoEditableViaSharedRegister(t *testing.T) {
	reg := &yankRegister{}
	src := newVimBuffer("REJECTION: claim 1 is anticipated by Smith.\nmore")
	src.vimMode = true
	src.readOnly = true
	src.yank = reg

	dst := newVimBuffer("REMARKS\n")
	dst.vimMode = true
	dst.yank = reg
	dst.line = 0

	// Yank the first line from the read-only source…
	src.line = 0
	src.handleKey(runes("y"))
	src.handleKey(runes("y"))
	if reg.text != "REJECTION: claim 1 is anticipated by Smith.\n" {
		t.Fatalf("shared register after yank = %q", reg.text)
	}

	// …and paste it into the editable response buffer (below the REMARKS heading;
	// the trailing empty line of the seed remains last).
	dst.handleKey(runes("p"))
	if want := "REMARKS\nREJECTION: claim 1 is anticipated by Smith.\n"; dst.Value() != want {
		t.Fatalf("cross-pane paste = %q, want %q", dst.Value(), want)
	}
	if !dst.dirty {
		t.Fatal("paste should mark the destination dirty")
	}
}

func TestCountYank(t *testing.T) {
	b := newVimBuffer("a\nb\nc\nd")
	b.vimMode = true
	b.line = 0
	// 2yy yanks two lines.
	b.handleKey(runes("2"))
	b.handleKey(runes("y"))
	b.handleKey(runes("y"))
	if b.reg().text != "a\nb\n" {
		t.Fatalf("2yy register = %q, want %q", b.reg().text, "a\nb\n")
	}
}
