package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// USPTOCandidateSelectMsg is dispatched when the user selects a candidate.
type USPTOCandidateSelectMsg struct {
	Project   domain.ProjectID
	Candidate domain.USPTOCandidate
}

// USPTOCandidatePicker is the Choice Menu Popup overlay shown when multiple
// matching wrappers are found.
type USPTOCandidatePicker struct {
	theme      render.Theme
	project    domain.ProjectID
	candidates []domain.USPTOCandidate
	cursor     int
}

// NewUSPTOCandidatePicker builds the candidate picker.
func NewUSPTOCandidatePicker(theme render.Theme, project domain.ProjectID, candidates []domain.USPTOCandidate) *USPTOCandidatePicker {
	return &USPTOCandidatePicker{
		theme:      theme,
		project:    project,
		candidates: candidates,
		cursor:     0,
	}
}

// Title implements Overlay.
func (p *USPTOCandidatePicker) Title() string {
	return fmt.Sprintf("Multiple USPTO wrappers found (%d)", len(p.candidates))
}

// Command implements Overlay.
func (p *USPTOCandidatePicker) Command(command.ID, int) (Overlay, tea.Cmd) { return p, nil }

// Handles implements Overlay.
func (p *USPTOCandidatePicker) Handles() []command.ID { return nil }

// HandleKey implements KeyHandler.
func (p *USPTOCandidatePicker) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	key := msg.String()
	switch key {
	case "q", "esc":
		return p, func() tea.Msg { return CloseOverlayMsg{} }, true
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
		} else {
			p.cursor = len(p.candidates) - 1
		}
	case "down", "j":
		if p.cursor < len(p.candidates)-1 {
			p.cursor++
		} else {
			p.cursor = 0
		}
	case "enter":
		if len(p.candidates) > 0 {
			c := p.candidates[p.cursor]
			project := p.project
			return p, func() tea.Msg {
				return USPTOCandidateSelectMsg{Project: project, Candidate: c}
			}, true
		}
	}
	return p, nil, true
}

// View implements Overlay.
func (p *USPTOCandidatePicker) View(maxW, _ int) string {
	var b strings.Builder
	b.WriteString(p.theme.Dim.Render(render.Truncate(
		"Select the correct application wrapper to load from USPTO:",
		maxW)))
	b.WriteString("\n\n")

	for i, c := range p.candidates {
		prefix := "  "
		style := p.theme.Row
		if i == p.cursor {
			prefix = "> "
			style = p.theme.Selected
		}

		b.WriteString(prefix)
		appNumStr := fmt.Sprintf("[%s]", c.ApplicationNumber)
		b.WriteString(p.theme.HelpKey.Render(render.Pad(appNumStr, 12)))
		b.WriteString(" ")

		metaStr := fmt.Sprintf("Filing: %s | Inventor: %s", firstNonEmpty(c.FilingDate, "N/A"), firstNonEmpty(c.FirstInventorName, "Unknown"))
		b.WriteString(p.theme.Dim.Render(render.Pad(metaStr, 35)))
		b.WriteString(" ")

		title := c.Title
		if title == "" {
			title = "(No Title)"
		}
		b.WriteString(style.Render(render.Truncate(title, maxW-2-12-1-35-1)))
		b.WriteByte('\n')
	}

	b.WriteByte('\n')
	b.WriteString(p.theme.HelpKey.Render("[enter]"))
	b.WriteString(p.theme.Row.Render(" Choose  "))
	b.WriteString(p.theme.HelpKey.Render("[q/esc]"))
	b.WriteString(p.theme.Row.Render(" Cancel"))
	return b.String()
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
