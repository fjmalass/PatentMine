package overlay

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/tui/render"
)

// ExportNotesConfirm shows the proposed export path and any existing files in
// the same directory before the write happens. [y] confirms, anything else
// cancels. Despite the name it backs any export-to-file confirmation; the
// title is supplied by the caller.
type ExportNotesConfirm struct {
	theme         render.Theme
	title         string
	path          string
	existingFiles []string
}

// NewExportConfirm builds the overlay with a caller-supplied title.
func NewExportConfirm(theme render.Theme, title, path string, existingFiles []string) *ExportNotesConfirm {
	return &ExportNotesConfirm{theme: theme, title: title, path: path, existingFiles: existingFiles}
}

// NewExportNotesConfirm builds the overlay for the notes export.
func NewExportNotesConfirm(theme render.Theme, path string, existingFiles []string) *ExportNotesConfirm {
	return NewExportConfirm(theme, "Export Notes", path, existingFiles)
}

// Title implements Overlay.
func (e *ExportNotesConfirm) Title() string { return e.title }

// Command implements Overlay.
func (e *ExportNotesConfirm) Command(command.ID, int) (Overlay, tea.Cmd) { return e, nil }

// Handles implements Overlay.
func (e *ExportNotesConfirm) Handles() []command.ID { return nil }

// HandleKey implements KeyHandler.
func (e *ExportNotesConfirm) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if strings.ToLower(msg.String()) == "y" {
		return e, func() tea.Msg { return ConfirmAcceptMsg{} }, true
	}
	return e, func() tea.Msg { return ConfirmRejectMsg{} }, true
}

// View implements Overlay.
func (e *ExportNotesConfirm) View(maxW, _ int) string {
	var b strings.Builder

	b.WriteString(e.theme.Dim.Render("File:"))
	b.WriteByte('\n')
	b.WriteString(e.theme.Row.Render(render.Truncate("  "+e.path, maxW)))
	b.WriteString("\n\n")

	if len(e.existingFiles) > 0 {
		b.WriteString(e.theme.Dim.Render("Existing files in directory:"))
		b.WriteByte('\n')
		for _, f := range e.existingFiles {
			b.WriteString(e.theme.Dim.Render(render.Truncate("  "+f, maxW)))
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}

	b.WriteString(e.theme.HelpKey.Render("[y]"))
	b.WriteString(e.theme.Row.Render(" Export    "))
	b.WriteString(e.theme.HelpKey.Render("[n/esc]"))
	b.WriteString(e.theme.Row.Render(" Cancel"))
	return b.String()
}
