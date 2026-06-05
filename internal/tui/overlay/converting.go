package overlay

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/command"
	"patentmine/internal/tui/render"
)

// Converting shows a transient spinner while the daemon copies a document and
// extracts embedded text from it.
type Converting struct {
	theme   render.Theme
	title   string
	message string
	spinner spinner.Model
}

func NewConverting(theme render.Theme, title, message string) *Converting {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return &Converting{theme: theme, title: title, message: message, spinner: s}
}

func (o *Converting) Title() string                              { return o.title }
func (o *Converting) Init() tea.Cmd                              { return o.spinner.Tick }
func (o *Converting) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }
func (o *Converting) Handles() []command.ID                      { return nil }

func (o *Converting) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		o.spinner, cmd = o.spinner.Update(m)
		return o, cmd
	}
	return o, nil
}

func (o *Converting) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, 50, 20, 38, 6)
}

func (o *Converting) View(w, _ int) string {
	var b strings.Builder
	b.WriteByte('\n')
	b.WriteString("  ")
	b.WriteString(o.spinner.View())
	b.WriteByte(' ')
	b.WriteString(render.Truncate(o.message, w-6))
	b.WriteString("\n\n")
	b.WriteString("  ")
	b.WriteString(o.theme.Dim.Render(render.Truncate("Parsing embedded text; OCR is not implemented", w-4)))
	return b.String()
}
