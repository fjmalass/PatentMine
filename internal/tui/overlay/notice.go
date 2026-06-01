package overlay

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/tui/render"
)

// NoticeOverlay displays a modal informational message — a title and one or
// more body lines — dismissed on any keypress. It is the neutral counterpart to
// ErrorOverlay, used to confirm an action's outcome (e.g. how many patents were
// exported and where).
type NoticeOverlay struct {
	theme render.Theme
	title string
	lines []string
}

// NewNoticeOverlay builds a NoticeOverlay with the given title and body lines.
func NewNoticeOverlay(theme render.Theme, title string, lines []string) *NoticeOverlay {
	return &NoticeOverlay{theme: theme, title: title, lines: lines}
}

// Title implements Overlay.
func (n *NoticeOverlay) Title() string { return n.title }

// Command implements Overlay: NoticeOverlay consumes all input via HandleKey.
func (n *NoticeOverlay) Command(command.ID, int) (Overlay, tea.Cmd) { return n, nil }

// Handles implements Overlay.
func (n *NoticeOverlay) Handles() []command.ID { return nil }

// HandleKey implements KeyHandler: any key closes the overlay.
func (n *NoticeOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	return n, func() tea.Msg { return CloseOverlayMsg{} }, true
}

// View implements Overlay.
func (n *NoticeOverlay) View(maxW, _ int) string {
	var b strings.Builder
	for _, line := range n.lines {
		b.WriteString(n.theme.Row.Render(render.Truncate(line, maxW)))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(n.theme.HelpKey.Render("[any key]"))
	b.WriteString(n.theme.Row.Render(" Close"))
	return b.String()
}
