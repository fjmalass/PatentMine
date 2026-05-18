package overlay

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/tui/render"
)

// ConfirmAcceptMsg signals that the user accepted the confirmation prompt.
type ConfirmAcceptMsg struct{}

// ConfirmRejectMsg signals that the user rejected or dismissed the confirmation.
type ConfirmRejectMsg struct{}

// Confirm is a yes/no modal that blocks until the user makes a choice.
type Confirm struct {
	theme   render.Theme
	message string
}

// NewConfirm builds a confirmation overlay showing message.
func NewConfirm(theme render.Theme, message string) *Confirm {
	return &Confirm{theme: theme, message: message}
}

// Title implements Overlay.
func (c *Confirm) Title() string { return "Confirm" }

// Command implements Overlay: Confirm consumes all input via HandleKey.
func (c *Confirm) Command(command.ID, int) (Overlay, tea.Cmd) { return c, nil }

// Handles implements Overlay.
func (c *Confirm) Handles() []command.ID { return nil }

// HandleKey implements KeyHandler: y accepts, anything else rejects.
func (c *Confirm) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if strings.ToLower(msg.String()) == "y" {
		return c, func() tea.Msg { return ConfirmAcceptMsg{} }, true
	}
	return c, func() tea.Msg { return ConfirmRejectMsg{} }, true
}

// View implements Overlay.
func (c *Confirm) View(maxW, _ int) string {
	var b strings.Builder
	b.WriteString(c.theme.Row.Render(render.Truncate(c.message, maxW)))
	b.WriteString("\n\n")
	b.WriteString(c.theme.HelpKey.Render("[y]"))
	b.WriteString(c.theme.Row.Render(" Yes    "))
	b.WriteString(c.theme.HelpKey.Render("[n/esc]"))
	b.WriteString(c.theme.Row.Render(" No"))
	return b.String()
}
