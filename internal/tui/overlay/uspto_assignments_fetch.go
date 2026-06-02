package overlay

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// USPTOAssignmentsFetchingOverlay is the modal spinner shown while a USPTO
// assignments search is in flight.
type USPTOAssignmentsFetchingOverlay struct {
	theme   render.Theme
	patent  domain.PatentNumber
	spinner spinner.Model
}

func NewUSPTOAssignmentsFetchingOverlay(theme render.Theme, patent domain.PatentNumber) *USPTOAssignmentsFetchingOverlay {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return &USPTOAssignmentsFetchingOverlay{theme: theme, patent: patent, spinner: s}
}

func (o *USPTOAssignmentsFetchingOverlay) Title() string {
	return "Fetching USPTO Assignments"
}

func (o *USPTOAssignmentsFetchingOverlay) Init() tea.Cmd { return o.spinner.Tick }
func (o *USPTOAssignmentsFetchingOverlay) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }
func (o *USPTOAssignmentsFetchingOverlay) Handles() []command.ID { return nil }

func (o *USPTOAssignmentsFetchingOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		o.spinner, cmd = o.spinner.Update(m)
		return o, cmd
	}
	return o, nil
}

func (o *USPTOAssignmentsFetchingOverlay) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, 50, 20, 36, 6)
}

func (o *USPTOAssignmentsFetchingOverlay) View(w, _ int) string {
	var b strings.Builder
	b.WriteByte('\n')
	b.WriteString("  ")
	b.WriteString(o.spinner.View())
	b.WriteByte(' ')
	b.WriteString(render.Truncate("Fetching assignments for "+o.patent.String()+"…", w-6))
	b.WriteString("\n\n")
	b.WriteString("  ")
	b.WriteString(o.theme.Dim.Render(render.Truncate("(this queries the USPTO ODP and parses assignments)", w-4)))
	return b.String()
}
