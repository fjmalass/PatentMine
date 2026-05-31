package overlay

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// IDSHeader edits the USPTO IDS header fields written into PTO/SB/08a and
// PTO/SB/08c. Tab/Shift+Tab move between fields, Enter saves, Esc cancels.
//
// Inventors are entered as a comma-separated list (first name = First Named
// Inventor). The examiner field captures the *current* assignee; on save the
// new name is appended to the project's examiner history with a timestamp of
// now, so the prior examiner is preserved for audit.
type IDSHeader struct {
	theme  render.Theme
	source domain.Project
	values [7]string
	focus  int
}

const (
	ihFieldName = iota
	ihFieldApp
	ihFieldFiling
	ihFieldInventors
	ihFieldArtUnit
	ihFieldExaminer
	ihFieldDocket
)

var ihLabels = [7]string{
	"Project Name",
	"Application Number",
	"Filing Date (YYYY-MM-DD)",
	"Inventors (comma-separated)",
	"Art Unit",
	"Current Examiner",
	"Attorney Docket Number",
}

// NewIDSHeader builds the overlay pre-populated from the given project.
func NewIDSHeader(theme render.Theme, project domain.Project) *IDSHeader {
	o := &IDSHeader{theme: theme, source: project}
	o.values[ihFieldName] = project.Name
	o.values[ihFieldApp] = project.ApplicationNumber
	if !project.FilingDate.IsZero() {
		o.values[ihFieldFiling] = project.FilingDate.Format(domain.DateLayout)
	}
	o.values[ihFieldInventors] = project.JoinInventors()
	o.values[ihFieldArtUnit] = project.ArtUnit
	o.values[ihFieldExaminer] = project.LatestExaminer()
	o.values[ihFieldDocket] = project.AttorneyDocketNumber
	return o
}

// Title implements Overlay.
func (o *IDSHeader) Title() string { return "Edit IDS Header" }

// Command implements Overlay.
func (o *IDSHeader) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

// Handles implements Overlay.
func (o *IDSHeader) Handles() []command.ID { return nil }

// HandleKey implements KeyHandler.
func (o *IDSHeader) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyEsc:
		return o, func() tea.Msg { return PromptCloseMsg{} }, true
	case tea.KeyEnter:
		return o, o.submit(), true
	case tea.KeyTab, tea.KeyDown:
		o.focus = (o.focus + 1) % len(o.values)
		return o, nil, true
	case tea.KeyShiftTab, tea.KeyUp:
		o.focus = (o.focus - 1 + len(o.values)) % len(o.values)
		return o, nil, true
	case tea.KeyBackspace:
		if len(o.values[o.focus]) > 0 {
			r := []rune(o.values[o.focus])
			o.values[o.focus] = string(r[:len(r)-1])
		}
		return o, nil, true
	case tea.KeyCtrlU:
		o.values[o.focus] = ""
		return o, nil, true
	case tea.KeyRunes, tea.KeySpace:
		o.values[o.focus] += msg.String()
		return o, nil, true
	}
	return o, nil, true
}

func (o *IDSHeader) submit() tea.Cmd {
	updated := o.source
	updated.Name = strings.TrimSpace(o.values[ihFieldName])
	updated.ApplicationNumber = strings.TrimSpace(o.values[ihFieldApp])
	updated.Inventors = domain.ParseInventors(o.values[ihFieldInventors])
	updated.ArtUnit = strings.TrimSpace(o.values[ihFieldArtUnit])
	updated.AttorneyDocketNumber = strings.TrimSpace(o.values[ihFieldDocket])
	if v := strings.TrimSpace(o.values[ihFieldFiling]); v != "" {
		if t, err := time.Parse(domain.DateLayout, v); err == nil {
			updated.FilingDate = t
		}
	} else {
		updated.FilingDate = time.Time{}
	}
	// Examiner: only append a new history entry when the typed name differs
	// from the project's current latest examiner. Same value = no-op.
	current := o.source.LatestExaminer()
	typed := strings.TrimSpace(o.values[ihFieldExaminer])
	if typed != "" && typed != current {
		updated.Examiners = append(updated.Examiners, domain.ProjectExaminer{
			Name:       typed,
			RecordedAt: time.Now().UTC(),
		})
	}
	return func() tea.Msg { return IDSHeaderSubmitMsg{Project: updated} }
}

// View implements Overlay.
func (o *IDSHeader) View(maxW, _ int) string {
	var b strings.Builder
	for i, label := range ihLabels {
		marker := "  "
		if i == o.focus {
			marker = "▸ "
		}
		line := fmt.Sprintf("%s%-30s %s", marker, label+":", o.values[i])
		if i == o.focus {
			b.WriteString(o.theme.Title.Render(render.Truncate(line, maxW)))
		} else {
			b.WriteString(o.theme.Row.Render(render.Truncate(line, maxW)))
		}
		b.WriteByte('\n')
	}
	if n := len(o.source.Examiners); n > 1 {
		b.WriteByte('\n')
		b.WriteString(o.theme.Dim.Render(render.Truncate(
			fmt.Sprintf("Examiner history (%d entries, oldest first):", n), maxW)))
		b.WriteByte('\n')
		for _, ex := range o.source.Examiners {
			line := fmt.Sprintf("  · %s  —  %s", ex.RecordedAt.Format(domain.DateLayout), ex.Name)
			b.WriteString(o.theme.Dim.Render(render.Truncate(line, maxW)))
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')
	hint := "[tab]/[shift+tab] field  ·  [enter] save  ·  [esc] cancel  ·  [ctrl+u] clear field"
	b.WriteString(o.theme.Dim.Render(render.Truncate(hint, maxW)))
	return b.String()
}
