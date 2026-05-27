package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// IDSSetStatusMsg is dispatched when the user picks a status in the IDS status
// picker. The App turns it into a bulk save against the selected patents.
type IDSSetStatusMsg struct {
	Status  domain.IDSEntryStatus
	Patents []domain.PatentNumber
}

// IDSStatusPicker is the popup shown when the user invokes the IDS shortcut
// over more than one selected patent. It lists the four valid IDS entry
// statuses and saves the chosen one across the selection.
type IDSStatusPicker struct {
	theme   render.Theme
	patents []domain.PatentNumber
}

var idsStatusChoices = []struct {
	key    string
	status domain.IDSEntryStatus
	label  string
}{
	{"1", domain.IDSEntryPending, "Pending"},
	{"2", domain.IDSEntrySubmitted, "Submitted"},
	{"3", domain.IDSEntryAccepted, "Accepted"},
	{"4", domain.IDSEntryIgnored, "Ignored"},
}

// NewIDSStatusPicker builds the picker for the given selection.
func NewIDSStatusPicker(theme render.Theme, patents []domain.PatentNumber) *IDSStatusPicker {
	return &IDSStatusPicker{theme: theme, patents: append([]domain.PatentNumber(nil), patents...)}
}

// Title implements Overlay.
func (p *IDSStatusPicker) Title() string {
	return fmt.Sprintf("Set IDS status (%d patents)", len(p.patents))
}

// Command implements Overlay.
func (p *IDSStatusPicker) Command(command.ID, int) (Overlay, tea.Cmd) { return p, nil }

// Handles implements Overlay.
func (p *IDSStatusPicker) Handles() []command.ID { return nil }

// HandleKey implements KeyHandler.
func (p *IDSStatusPicker) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	key := strings.ToLower(msg.String())
	if key == "q" || key == "esc" {
		return p, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	for _, c := range idsStatusChoices {
		if key == c.key {
			status := c.status
			patents := append([]domain.PatentNumber(nil), p.patents...)
			return p, func() tea.Msg {
				return IDSSetStatusMsg{Status: status, Patents: patents}
			}, true
		}
	}
	return p, nil, true
}

// View implements Overlay.
func (p *IDSStatusPicker) View(maxW, _ int) string {
	var b strings.Builder
	b.WriteString(p.theme.Dim.Render(render.Truncate(
		fmt.Sprintf("Apply IDS status to %d selected patents. New entries default to in-full.", len(p.patents)),
		maxW)))
	b.WriteString("\n\n")
	for _, c := range idsStatusChoices {
		b.WriteString("  ")
		b.WriteString(p.theme.HelpKey.Render("[" + c.key + "]"))
		b.WriteString(" ")
		b.WriteString(p.theme.Row.Render(p.theme.IDSEntryStatusGlyph(c.status) + " " + c.label))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(p.theme.HelpKey.Render("[q/esc]"))
	b.WriteString(p.theme.Row.Render(" Cancel"))
	return b.String()
}
