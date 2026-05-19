package overlay

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/command"
	"patentmine/internal/proto"
	"patentmine/internal/tui/render"
)

// Loading is a modal overlay that shows a throbber and progress message
// while a background daemon job (like an ingest) is running.
type Loading struct {
	theme    render.Theme
	jobID    string
	title    string
	message  string
	spinner  spinner.Model
	finished bool
}

// NewLoading builds a loading overlay for the given job.
func NewLoading(theme render.Theme, jobID, title string) *Loading {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return &Loading{
		theme:   theme,
		jobID:   jobID,
		title:   title,
		message: "Starting…",
		spinner: s,
	}
}

func (l *Loading) Title() string { return l.title }

func (l *Loading) Init() tea.Cmd {
	return l.spinner.Tick
}

func (l *Loading) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	// Loading doesn't handle commands directly; it closes on EventIngestDone.
	return l, nil
}

func (l *Loading) Handles() []command.ID { return nil }

func (l *Loading) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		if m.String() == "ctrl+c" {
			return l, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		l.spinner, cmd = l.spinner.Update(m)
		return l, cmd
	case proto.Event:
		switch m.Method {
		case proto.EventIngestProgress:
			var p proto.IngestProgress
			if err := json.Unmarshal(m.Params, &p); err == nil && p.JobID == l.jobID {
				l.message = p.Message
				if p.Fetched > 0 {
					l.message = fmt.Sprintf("%s (%d fetched)", p.Message, p.Fetched)
				}
			}
		case proto.EventIngestDone:
			var d proto.IngestDone
			if err := json.Unmarshal(m.Params, &d); err == nil && d.JobID == l.jobID {
				l.finished = true
				return l, func() tea.Msg { return CloseOverlayMsg{} }
			}
		}
	}
	return l, nil
}

func (l *Loading) View(w, h int) string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  " + l.spinner.View() + " " + render.Truncate(l.message, w-6))
	b.WriteByte('\n')
	b.WriteByte('\n')
	b.WriteString(l.theme.Dim.Render(render.Pad("JobID: "+l.jobID, w-2)))
	return b.String()
}
