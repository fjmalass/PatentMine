package overlay

import (
	"strings"

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
	b.WriteString(t.theme.Dim.Render(render.Truncate(t.catalog.T(text.TextInputHint), maxW)))
	return b.String()
}
