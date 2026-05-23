package overlay

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/tui/render"
)

// ErrorOverlay displays a modal error message to the user that is dismissed on any keypress.
type ErrorOverlay struct {
	theme   render.Theme
	message string
}

// NewErrorOverlay builds a new ErrorOverlay showing the given message.
func NewErrorOverlay(theme render.Theme, message string) *ErrorOverlay {
	return &ErrorOverlay{theme: theme, message: message}
}

// Title implements Overlay.
func (e *ErrorOverlay) Title() string { return "Error" }

// Command implements Overlay: ErrorOverlay consumes all input via HandleKey.
func (e *ErrorOverlay) Command(command.ID, int) (Overlay, tea.Cmd) { return e, nil }

// Handles implements Overlay.
func (e *ErrorOverlay) Handles() []command.ID { return nil }

// HandleKey implements KeyHandler: any key closes the overlay.
func (e *ErrorOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	return e, func() tea.Msg { return CloseOverlayMsg{} }, true
}

// View implements Overlay.
func (e *ErrorOverlay) View(maxW, _ int) string {
	var b strings.Builder
	b.WriteString(e.theme.Error.Render(render.Truncate(e.message, maxW)))
	b.WriteString("\n\n")
	b.WriteString(e.theme.HelpKey.Render("[any key]"))
	b.WriteString(e.theme.Row.Render(" Close"))
	return b.String()
}
