package overlay

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/tui/render"
)

const (
	tvWidthPct  = 80
	tvHeightPct = 80
	tvMinWidth  = 70
	tvMinHeight = 18
)

// TextViewer is a read-only, vim-navigable viewer over a document's extracted
// text. It reuses the editor's vimBuffer in read-only mode (motions and search
// work, edits do not), so reading an imported reference feels like the office
// action's examiner pane. Esc closes.
type TextViewer struct {
	theme render.Theme
	title string
	buf   *vimBuffer
}

func NewTextViewer(theme render.Theme, title, text string) *TextViewer {
	buf := newVimBuffer(text)
	buf.vimMode = true
	buf.readOnly = true
	return &TextViewer{theme: theme, title: title, buf: buf}
}

func (o *TextViewer) Title() string { return o.title }

func (o *TextViewer) Handles() []command.ID { return nil }

func (o *TextViewer) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *TextViewer) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, tvWidthPct, tvHeightPct, tvMinWidth, tvMinHeight)
}

func (o *TextViewer) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if msg.Type == tea.KeyEsc {
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	o.buf.handleKey(msg)
	return o, nil, true
}

func (o *TextViewer) View(maxW, maxH int) string {
	bodyRows := max(maxH-2, 1)
	rows := o.buf.view(maxW, bodyRows, o.theme.Title.Render("█"))

	var b strings.Builder
	for i := 0; i < bodyRows; i++ {
		if i < len(rows) {
			b.WriteString(o.theme.Row.Render(render.Pad(render.Truncate(rows[i].text, maxW), maxW)))
		} else {
			b.WriteString(render.Pad("", maxW))
		}
		b.WriteByte('\n')
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(
		"j/k or ↑/↓ scroll · / search · esc close", maxW)))
	return b.String()
}
