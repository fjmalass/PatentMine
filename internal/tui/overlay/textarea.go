package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

// TextArea is a multiline text-entry overlay for longer free-text fields. The
// editing itself lives in the shared vimBuffer; TextArea adds only the submit
// (TextSubmitMsg) wiring and its caption chrome.
type TextArea struct {
	theme   render.Theme
	catalog *text.Catalog
	purpose Purpose
	title   text.Key
	caption text.Key
	buf     *vimBuffer
}

func NewTextArea(theme render.Theme, catalog *text.Catalog, purpose Purpose, title, caption text.Key, initial string) *TextArea {
	b := newVimBuffer(initial)
	b.cursorToEnd()
	return &TextArea{theme: theme, catalog: catalog, purpose: purpose, title: title, caption: caption, buf: b}
}

func (t *TextArea) Title() string { return t.catalog.T(t.title) }

func (t *TextArea) Handles() []command.ID { return []command.ID{command.CloseOverlay} }

func (t *TextArea) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if id == command.CloseOverlay {
		return t, func() tea.Msg { return CloseOverlayMsg{} }
	}
	return t, nil
}

func (t *TextArea) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, patentNoteOverlayWidthPct, patentNoteOverlayHeightPct, patentNoteMinWidth, patentNoteMinHeight)
}

func (t *TextArea) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch _, intent := t.buf.handleKey(msg); intent {
	case intentSave:
		return t, t.submit(), true
	case intentClose:
		return t, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	return t, nil, true
}

func (t *TextArea) submit() tea.Cmd {
	value := t.buf.Value()
	return func() tea.Msg { return TextSubmitMsg{Purpose: t.purpose, Value: value} }
}

func (t *TextArea) View(maxW, maxH int) string {
	bodyRows := max(maxH-6, 3)
	rows := t.buf.view(maxW, bodyRows, t.theme.Title.Render("█"))

	var b strings.Builder
	b.WriteString(t.theme.Row.Render(render.Truncate(t.catalog.T(t.caption), maxW)))
	b.WriteString("\n\n")
	for i, row := range rows {
		if row.selected {
			b.WriteString(t.theme.Selected.Render(render.Pad(render.Truncate(row.text, maxW), maxW)))
		} else {
			b.WriteString(t.theme.Row.Render(render.Truncate(row.text, maxW)))
		}
		if i < len(rows)-1 {
			b.WriteByte('\n')
		}
	}
	if len(rows) == 0 {
		b.WriteString(t.theme.Selected.Render(render.Pad("  1 "+t.theme.Title.Render("█"), maxW)))
	}
	b.WriteString("\n\n")
	if mode := t.buf.modeLabel(); mode != "" {
		b.WriteString(t.theme.Dim.Render(render.Truncate(fmt.Sprintf("-- %s -- · ctrl+s save · esc close · ctrl+] off", mode), maxW)))
	} else {
		b.WriteString(t.theme.Dim.Render(render.Truncate("multiline editor · enter newline · ctrl+s save · esc close · ctrl+] vim", maxW)))
	}
	return b.String()
}
