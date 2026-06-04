package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// OfficeActionDeleteConfirmMsg is dispatched when the user chooses how to delete.
type OfficeActionDeleteConfirmMsg struct {
	OA          domain.OfficeAction
	DeleteFiles bool
}

// OfficeActionDeleteConfirm presents options to delete only the db record or also delete cached files.
type OfficeActionDeleteConfirm struct {
	theme render.Theme
	oa    domain.OfficeAction
}

// NewOfficeActionDeleteConfirm builds a confirmation overlay for deleting an office action.
func NewOfficeActionDeleteConfirm(theme render.Theme, oa domain.OfficeAction) *OfficeActionDeleteConfirm {
	return &OfficeActionDeleteConfirm{theme: theme, oa: oa}
}

// Title implements Overlay.
func (c *OfficeActionDeleteConfirm) Title() string { return "Delete Office Action" }

// Command implements Overlay.
func (c *OfficeActionDeleteConfirm) Command(command.ID, int) (Overlay, tea.Cmd) { return c, nil }

// Handles implements Overlay.
func (c *OfficeActionDeleteConfirm) Handles() []command.ID { return nil }

// HandleKey implements KeyHandler.
func (c *OfficeActionDeleteConfirm) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	key := msg.String()
	switch key {
	case "1", "y":
		oa := c.oa
		return c, func() tea.Msg { return OfficeActionDeleteConfirmMsg{OA: oa, DeleteFiles: false} }, true
	case "2", "Y":
		oa := c.oa
		return c, func() tea.Msg { return OfficeActionDeleteConfirmMsg{OA: oa, DeleteFiles: true} }, true
	case "q", "esc", "n", "N":
		return c, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	return c, nil, true
}

func (c *OfficeActionDeleteConfirm) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, 80, 80, 76, 12)
}

// View implements Overlay.
func (c *OfficeActionDeleteConfirm) View(maxW, _ int) string {
	var b strings.Builder
	prompt := fmt.Sprintf("How do you want to delete the office action mailed %s?", c.oa.MailDate.Format(domain.DateLayout))
	for _, line := range wrapText(prompt, maxW) {
		b.WriteString(c.theme.Row.Render(line))
		b.WriteByte('\n')
	}
	b.WriteString("\n")

	b.WriteString("  ")
	b.WriteString(c.theme.HelpKey.Render("[1/y]"))
	b.WriteString(" ")
	b.WriteString(c.theme.Row.Render("Delete database entry only (keep files on disk)"))
	b.WriteByte('\n')

	b.WriteString("  ")
	b.WriteString(c.theme.HelpKey.Render("[2/Y]"))
	b.WriteString(" ")
	b.WriteString(c.theme.Row.Render("Delete entry and all cached files on disk"))
	b.WriteString("\n\n")

	b.WriteString(c.theme.HelpKey.Render("[q/esc/n]"))
	b.WriteString(c.theme.Row.Render(" Cancel"))
	return b.String()
}
