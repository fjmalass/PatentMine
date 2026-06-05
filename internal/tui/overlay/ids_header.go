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

const (
	ihFieldName = iota
	ihFieldApp
	ihFieldFiling
	ihFieldInventors
	ihFieldArtUnit
	ihFieldExaminer
	ihFieldDocket
)

// ihFields are the labeled IDS-header fields. Inventors are entered as a
// comma-separated list (first name = First Named Inventor); the examiner field
// captures the current assignee (appended to history on save).
var ihFields = []fieldFormField{
	{Label: "Project Name", Kind: fieldText},
	{Label: "Application Number", Kind: fieldText},
	{Label: "Filing Date (YYYY-MM-DD)", Kind: fieldText},
	{Label: "Inventors (comma-separated)", Kind: fieldText},
	{Label: "Art Unit", Kind: fieldText},
	{Label: "Current Examiner", Kind: fieldText},
	{Label: "Attorney Docket Number", Kind: fieldText},
}

// IDSHeader edits the USPTO IDS header fields written into PTO/SB/08a and
// PTO/SB/08c. It uses the shared view/edit field model (Enter to edit a field,
// Ctrl+S to save, Esc to cancel).
//
// Inventors are entered as a comma-separated list (first name = First Named
// Inventor). The examiner field captures the *current* assignee; on save the
// new name is appended to the project's examiner history with a timestamp of
// now, so the prior examiner is preserved for audit.
type IDSHeader struct {
	theme  render.Theme
	source domain.Project
	form   fieldForm
}

// NewIDSHeader builds the overlay pre-populated from the given project.
func NewIDSHeader(theme render.Theme, project domain.Project) *IDSHeader {
	o := &IDSHeader{theme: theme, source: project, form: newFieldForm(ihFields)}
	o.form.SetValue(ihFieldName, project.Name)
	o.form.SetValue(ihFieldApp, project.ApplicationNumber)
	if !project.FilingDate.IsZero() {
		o.form.SetValue(ihFieldFiling, project.FilingDate.Format(domain.DateLayout))
	}
	o.form.SetValue(ihFieldInventors, project.JoinInventors())
	o.form.SetValue(ihFieldArtUnit, project.ArtUnit)
	o.form.SetValue(ihFieldExaminer, project.LatestExaminer())
	o.form.SetValue(ihFieldDocket, project.AttorneyDocketNumber)
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
	switch action, _ := o.form.HandleKey(msg); action {
	case fieldFormSubmit:
		return o, o.submit(), true
	case fieldFormCancel:
		return o, func() tea.Msg { return PromptCloseMsg{} }, true
	default:
		return o, nil, true
	}
}

func (o *IDSHeader) submit() tea.Cmd {
	updated := o.source
	updated.Name = strings.TrimSpace(o.form.Value(ihFieldName))
	updated.ApplicationNumber = strings.TrimSpace(o.form.Value(ihFieldApp))
	updated.Inventors = domain.ParseInventors(o.form.Value(ihFieldInventors))
	updated.ArtUnit = strings.TrimSpace(o.form.Value(ihFieldArtUnit))
	updated.AttorneyDocketNumber = strings.TrimSpace(o.form.Value(ihFieldDocket))
	if v := strings.TrimSpace(o.form.Value(ihFieldFiling)); v != "" {
		if t, err := time.Parse(domain.DateLayout, v); err == nil {
			updated.FilingDate = t
		}
	} else {
		updated.FilingDate = time.Time{}
	}
	// Examiner: only append a new history entry when the typed name differs
	// from the project's current latest examiner. Same value = no-op.
	current := o.source.LatestExaminer()
	typed := strings.TrimSpace(o.form.Value(ihFieldExaminer))
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
	b.WriteString(o.form.RenderFields(o.theme, maxW, 30))
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
	b.WriteString(o.theme.Dim.Render(render.Truncate(o.form.Hint(), maxW)))
	return b.String()
}
