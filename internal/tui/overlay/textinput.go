package overlay

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

// TextInput is a single-line text-entry overlay. One type serves every
// free-text prompt — project name today, notes or rename later — so a new
// text field never means a new overlay type.
type TextInput struct {
	theme   render.Theme
	catalog *text.Catalog
	purpose Purpose
	title   text.Key
	caption text.Key
	value   string
	cursor  int // rune index into value

	vimMode   bool
	vimInsert bool
	undos     []struct{ value string; cursor int }
}

// NewTextInput builds a text-entry overlay. title and caption are catalog keys;
// the submitted value is delivered as a TextSubmitMsg tagged with purpose.
func NewTextInput(theme render.Theme, catalog *text.Catalog, purpose Purpose, title, caption text.Key) *TextInput {
	return &TextInput{theme: theme, catalog: catalog, purpose: purpose, title: title, caption: caption}
}

// Title implements Overlay.
func (t *TextInput) Title() string { return t.catalog.T(t.title) }

// Command implements Overlay: a text field services no resolved commands —
// every key press is consumed by HandleKey as literal input.
func (t *TextInput) Command(command.ID, int) (Overlay, tea.Cmd) { return t, nil }

// Handles implements Overlay.
func (t *TextInput) Handles() []command.ID { return nil }

// HandleKey implements KeyHandler: the overlay consumes every key as text so
// no key press leaks to the keymap while a field is focused.
func (t *TextInput) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if isVimToggleKey(msg) {
		t.vimMode = !t.vimMode
		if t.vimMode {
			t.vimInsert = false
			t.saveTextUndo()
		}
		return t, nil, true
	}
	if t.vimMode {
		return t.handleVimKey(msg)
	}
	switch msg.Type {
	case tea.KeyEsc:
		return t, func() tea.Msg { return PromptCloseMsg{} }, true
	case tea.KeyEnter:
		value := strings.TrimSpace(t.value)
		if value == "" {
			return t, nil, true // stay open until the field has content
		}
		return t, func() tea.Msg { return TextSubmitMsg{Purpose: t.purpose, Value: value} }, true
	case tea.KeyBackspace:
		if t.cursor > 0 {
			runes := []rune(t.value)
			t.value = string(append(runes[:t.cursor-1], runes[t.cursor:]...))
			t.cursor--
		}
		return t, nil, true
	case tea.KeyLeft:
		if t.cursor > 0 {
			t.cursor--
		}
		return t, nil, true
	case tea.KeyRight:
		if t.cursor < len([]rune(t.value)) {
			t.cursor++
		}
		return t, nil, true
	case tea.KeyRunes, tea.KeySpace:
		runes := []rune(t.value)
		ins := []rune(msg.String())
		merged := make([]rune, 0, len(runes)+len(ins))
		merged = append(merged, runes[:t.cursor]...)
		merged = append(merged, ins...)
		merged = append(merged, runes[t.cursor:]...)
		t.value = string(merged)
		t.cursor += len(ins)
		return t, nil, true
	}
	return t, nil, true
}

func (t *TextInput) handleVimKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if t.vimInsert {
		switch msg.Type {
		case tea.KeyEsc:
			t.vimInsert = false
			return t, nil, true
		case tea.KeyEnter:
			value := strings.TrimSpace(t.value)
			if value == "" {
				return t, nil, true
			}
			return t, func() tea.Msg { return TextSubmitMsg{Purpose: t.purpose, Value: value} }, true
		case tea.KeyBackspace:
			if t.cursor > 0 {
				runes := []rune(t.value)
				t.value = string(append(runes[:t.cursor-1], runes[t.cursor:]...))
				t.cursor--
			}
			return t, nil, true
		case tea.KeyLeft:
			if t.cursor > 0 {
				t.cursor--
			}
			return t, nil, true
		case tea.KeyRight:
			if t.cursor < len([]rune(t.value)) {
				t.cursor++
			}
			return t, nil, true
		case tea.KeyRunes, tea.KeySpace:
			runes := []rune(t.value)
			ins := []rune(msg.String())
			merged := make([]rune, 0, len(runes)+len(ins))
			merged = append(merged, runes[:t.cursor]...)
			merged = append(merged, ins...)
			merged = append(merged, runes[t.cursor:]...)
			t.value = string(merged)
			t.cursor += len(ins)
			return t, nil, true
		}
		return t, nil, true
	}
	switch msg.Type {
	case tea.KeyEsc:
		return t, nil, true
	case tea.KeyEnter:
		value := strings.TrimSpace(t.value)
		if value == "" {
			return t, nil, true
		}
		return t, func() tea.Msg { return TextSubmitMsg{Purpose: t.purpose, Value: value} }, true
	case tea.KeyLeft:
		if t.cursor > 0 {
			t.cursor--
		}
		return t, nil, true
	case tea.KeyRight:
		if t.cursor < len([]rune(t.value)) {
			t.cursor++
		}
		return t, nil, true
	case tea.KeyRunes, tea.KeySpace:
		switch msg.String() {
		case "h":
			if t.cursor > 0 {
				t.cursor--
			}
		case "l":
			if t.cursor < len([]rune(t.value)) {
				t.cursor++
			}
		case "i":
			t.vimInsert = true
			t.saveTextUndo()
		case "a":
			if t.cursor < len([]rune(t.value)) {
				t.cursor++
			}
			t.vimInsert = true
			t.saveTextUndo()
		case "I":
			t.cursor = 0
			t.vimInsert = true
			t.saveTextUndo()
		case "A":
			t.cursor = len([]rune(t.value))
			t.vimInsert = true
			t.saveTextUndo()
		case "x":
			t.saveTextUndo()
			if t.cursor < len([]rune(t.value)) {
				runes := []rune(t.value)
				t.value = string(append(runes[:t.cursor], runes[t.cursor+1:]...))
			}
		case "X":
			t.saveTextUndo()
			if t.cursor > 0 {
				runes := []rune(t.value)
				t.value = string(append(runes[:t.cursor-1], runes[t.cursor:]...))
				t.cursor--
			}
		case "d":
			if t.value != "" {
				t.saveTextUndo()
				t.value = ""
				t.cursor = 0
			}
		case "D":
			t.saveTextUndo()
			runes := []rune(t.value)
			if t.cursor < len(runes) {
				t.value = string(runes[:t.cursor])
			}
		case "0":
			t.cursor = 0
		case "$":
			t.cursor = len([]rune(t.value))
		case "w":
			t.moveTextWordForward()
		case "b":
			t.moveTextWordBackward()
		case "u":
			t.undoText()
		}
		return t, nil, true
	}
	return t, nil, true
}

func (t *TextInput) moveTextWordForward() {
	runes := []rune(t.value)
	if t.cursor >= len(runes) {
		return
	}
	wordType := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }(runes[t.cursor])
	end := t.cursor
	for end < len(runes) && (unicode.IsLetter(runes[end]) || unicode.IsDigit(runes[end]) || runes[end] == '_') == wordType {
		end++
	}
	for end < len(runes) && (runes[end] == ' ' || runes[end] == '\t') {
		end++
	}
	if end <= len(runes) {
		t.cursor = end
	}
}

func (t *TextInput) moveTextWordBackward() {
	if t.cursor == 0 {
		return
	}
	runes := []rune(t.value)
	pos := t.cursor - 1
	for pos > 0 && (runes[pos] == ' ' || runes[pos] == '\t') {
		pos--
	}
	if pos == 0 && (runes[0] == ' ' || runes[0] == '\t') {
		t.cursor = 0
		return
	}
	wordType := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }(runes[pos])
	for pos > 0 && (unicode.IsLetter(runes[pos]) || unicode.IsDigit(runes[pos]) || runes[pos] == '_') == wordType {
		pos--
	}
	if pos == 0 && (unicode.IsLetter(runes[0]) || unicode.IsDigit(runes[0]) || runes[0] == '_') == wordType {
		t.cursor = 0
		return
	}
	if (unicode.IsLetter(runes[pos]) || unicode.IsDigit(runes[pos]) || runes[pos] == '_') != wordType {
		pos++
	}
	t.cursor = pos
}

func (t *TextInput) saveTextUndo() {
	t.undos = append(t.undos, struct{ value string; cursor int }{t.value, t.cursor})
	if len(t.undos) > 100 {
		t.undos = t.undos[1:]
	}
}

func (t *TextInput) undoText() {
	if len(t.undos) == 0 {
		return
	}
	e := t.undos[len(t.undos)-1]
	t.undos = t.undos[:len(t.undos)-1]
	t.value = e.value
	t.cursor = e.cursor
}

// View implements Overlay.
func (t *TextInput) View(maxW, _ int) string {
	var b strings.Builder
	b.WriteString(t.theme.Row.Render(render.Truncate(t.catalog.T(t.caption), maxW)))
	b.WriteString("\n\n")
	runes := []rune(t.value)
	before := string(runes[:t.cursor])
	after := string(runes[t.cursor:])
	input := "> " + before + t.theme.Title.Render("█") + after
	b.WriteString(render.Truncate(input, maxW))
	b.WriteString("\n\n")
	if t.vimMode {
		mode := "NORMAL"
		if t.vimInsert {
			mode = "INSERT"
		}
		b.WriteString(t.theme.Dim.Render(render.Truncate(fmt.Sprintf("-- %s -- · enter confirms · esc cancels · ctrl+] off", mode), maxW)))
	} else {
		hint := t.catalog.T(text.TextInputHint) + " · ctrl+] vim"
		b.WriteString(t.theme.Dim.Render(render.Truncate(hint, maxW)))
	}
	return b.String()
}
