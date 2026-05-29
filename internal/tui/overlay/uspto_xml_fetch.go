package overlay

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/tui/render"
)

// USPTOXMLFetchingOverlay is the modal spinner shown while a USPTO XML
// document is being downloaded (or located on disk). It dismisses itself when
// the App receives a USPTOXMLFetchedMsg and dispatches a CloseOverlayMsg.
type USPTOXMLFetchingOverlay struct {
	theme   render.Theme
	patent  domain.PatentNumber
	kind    proto.USPTOXMLKind
	spinner spinner.Model
}

// NewUSPTOXMLFetchingOverlay builds the spinner overlay.
func NewUSPTOXMLFetchingOverlay(theme render.Theme, patent domain.PatentNumber, kind proto.USPTOXMLKind) *USPTOXMLFetchingOverlay {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return &USPTOXMLFetchingOverlay{theme: theme, patent: patent, kind: kind, spinner: s}
}

// Title implements Overlay.
func (o *USPTOXMLFetchingOverlay) Title() string {
	return "Fetching " + kindLabel(o.kind) + " XML"
}

// Init starts the spinner ticker.
func (o *USPTOXMLFetchingOverlay) Init() tea.Cmd { return o.spinner.Tick }

// Command implements Overlay.
func (o *USPTOXMLFetchingOverlay) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

// Handles implements Overlay.
func (o *USPTOXMLFetchingOverlay) Handles() []command.ID { return nil }

// Update keeps the spinner ticking. Ctrl+C aborts the app the same way every
// other overlay does; the fetch itself cannot be cancelled from here.
func (o *USPTOXMLFetchingOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		o.spinner, cmd = o.spinner.Update(m)
		return o, cmd
	}
	return o, nil
}

// OverlaySize keeps the box small: this is a transient spinner, not a layout.
func (o *USPTOXMLFetchingOverlay) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, 50, 20, 36, 6)
}

// View renders the spinner + status line.
func (o *USPTOXMLFetchingOverlay) View(w, _ int) string {
	var b strings.Builder
	b.WriteByte('\n')
	b.WriteString("  ")
	b.WriteString(o.spinner.View())
	b.WriteByte(' ')
	b.WriteString(render.Truncate("Fetching "+kindLabel(o.kind)+" XML for "+o.patent.String()+"…", w-6))
	b.WriteString("\n\n")
	b.WriteString("  ")
	b.WriteString(o.theme.Dim.Render(render.Truncate("(cached copies on disk are served without re-downloading)", w-4)))
	return b.String()
}

func kindLabel(k proto.USPTOXMLKind) string {
	switch k {
	case proto.USPTOXMLKindPGPub:
		return "PGPub"
	case proto.USPTOXMLKindGrant:
		return "Grant"
	}
	return string(k)
}
