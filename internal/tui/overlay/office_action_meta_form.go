package overlay

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/tui/render"
)

var oaTypes = []domain.OAType{
	domain.OANonFinal,
	domain.OAFinal,
	domain.OARestriction,
	domain.OAAdvisory,
	domain.OANoticeOfAllowance,
}

// Field indices into the office-action meta form.
const (
	oaFieldName = iota
	oaFieldType
	oaFieldMailDate
	oaFieldAppNumber
)

// oaMetaFields are the labeled fields shared by the import and edit forms. The
// Type field cycles through oaTypes (see cycleOAType); the rest are free text.
var oaMetaFields = []fieldFormField{
	{Label: "Action Name", Kind: fieldText},
	{Label: "Type", Kind: fieldChoice, Cycle: cycleOAType},
	{Label: "Mail Date", Kind: fieldText},
	{Label: "Application Number", Kind: fieldText},
}

// OfficeActionMetaForm captures the office-action metadata at import time or edit time —
// crucially the examiner name, which the path-only import dropped — before the
// daemon copies the file in. It uses the shared view/edit field model: arrive on
// a field in view mode, Enter to edit (a cursor shows where input lands), Enter
// to validate, Ctrl+S to save, Esc to cancel. The response deadline is computed
// from the mail date and type by the engine, so it is not entered here.
type OfficeActionMetaForm struct {
	theme            render.Theme
	project          domain.ProjectID
	oaID             string // the office action ID if edit
	isEdit           bool
	form             fieldForm
	originalExaminer string
	originalArtUnit  string
}

// NewOfficeActionMetaForm builds the form for creating/loading an office action, pre-filled
// from the active project.
func NewOfficeActionMetaForm(theme render.Theme, project domain.Project, source string) *OfficeActionMetaForm {
	o := &OfficeActionMetaForm{
		theme:            theme,
		project:          project.ID,
		isEdit:           false,
		form:             newFieldForm(oaMetaFields),
		originalExaminer: project.LatestExaminer(),
		originalArtUnit:  project.ArtUnit,
	}
	if source != "" {
		base := filepath.Base(source)
		ext := filepath.Ext(base)
		o.form.SetValue(oaFieldName, strings.TrimSuffix(base, ext))
	}
	o.form.SetValue(oaFieldType, string(domain.OANonFinal))
	o.form.SetValue(oaFieldAppNumber, project.ApplicationNumber)
	return o
}

// NewOfficeActionEditForm builds the form for editing an existing office action.
func NewOfficeActionEditForm(theme render.Theme, project domain.ProjectID, oa domain.OfficeAction) *OfficeActionMetaForm {
	o := &OfficeActionMetaForm{
		theme:            theme,
		project:          project,
		oaID:             oa.ID,
		isEdit:           true,
		form:             newFieldForm(oaMetaFields),
		originalExaminer: oa.Examiner,
		originalArtUnit:  oa.ArtUnit,
	}
	o.form.SetValue(oaFieldName, oa.Name)
	o.form.SetValue(oaFieldType, string(oa.Type))
	o.form.SetValue(oaFieldMailDate, oa.MailDate.Format(domain.DateLayout))
	o.form.SetValue(oaFieldAppNumber, oa.ApplicationNumber)
	return o
}

func (o *OfficeActionMetaForm) Title() string {
	if o.isEdit {
		return "Edit Office Action Details"
	}
	return "Load Office Action"
}

func (o *OfficeActionMetaForm) Handles() []command.ID { return nil }

func (o *OfficeActionMetaForm) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *OfficeActionMetaForm) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch action, _ := o.form.HandleKey(msg); action {
	case fieldFormSubmit:
		return o, o.submit(), true
	case fieldFormCancel:
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	default:
		return o, nil, true
	}
}

// cycleOAType advances the office-action type. A first-letter key sets a type
// directly (f→final, r→restriction, a→advisory, o/l→notice of allowance, n
// toggles non-final ↔ notice of allowance); space or any other key advances to
// the next type in canonical order.
func cycleOAType(current, key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "f":
		return string(domain.OAFinal)
	case "r":
		return string(domain.OARestriction)
	case "a":
		return string(domain.OAAdvisory)
	case "o", "l":
		return string(domain.OANoticeOfAllowance)
	case "n":
		if domain.OAType(current) == domain.OANonFinal {
			return string(domain.OANoticeOfAllowance)
		}
		return string(domain.OANonFinal)
	}
	idx := -1
	for i, t := range oaTypes {
		if t == domain.OAType(current) {
			idx = i
			break
		}
	}
	return string(oaTypes[(idx+1)%len(oaTypes)])
}

func (o *OfficeActionMetaForm) submit() tea.Cmd {
	if o.isEdit {
		params := proto.OfficeActionUpdateParams{
			ID:                o.oaID,
			Name:              strings.TrimSpace(o.form.Value(oaFieldName)),
			Examiner:          o.originalExaminer,
			MailDate:          strings.TrimSpace(o.form.Value(oaFieldMailDate)),
			Type:              domain.OAType(strings.TrimSpace(o.form.Value(oaFieldType))),
			ArtUnit:           o.originalArtUnit,
			ApplicationNumber: strings.TrimSpace(o.form.Value(oaFieldAppNumber)),
		}
		return func() tea.Msg { return OfficeActionEditSubmitMsg{Params: params} }
	}

	params := proto.OfficeActionImportParams{
		Project:           o.project,
		Name:              strings.TrimSpace(o.form.Value(oaFieldName)),
		SourcePath:        "",
		Examiner:          o.originalExaminer,
		MailDate:          strings.TrimSpace(o.form.Value(oaFieldMailDate)),
		Type:              domain.OAType(strings.TrimSpace(o.form.Value(oaFieldType))),
		ArtUnit:           o.originalArtUnit,
		ApplicationNumber: strings.TrimSpace(o.form.Value(oaFieldAppNumber)),
	}
	return func() tea.Msg { return OfficeActionImportSubmitMsg{Params: params} }
}

func (o *OfficeActionMetaForm) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, 80, 80, 76, 15)
}

func (o *OfficeActionMetaForm) View(maxW, _ int) string {
	var b strings.Builder
	b.WriteString(o.form.RenderFields(o.theme, maxW, 22))
	b.WriteByte('\n')

	hint := o.form.Hint()
	if o.form.Focus() == oaFieldType && o.form.Editing() {
		hint += " · f/r/a/o/n set type directly"
	} else if !o.form.Editing() {
		hint += " · deadline auto-set from mail date"
	}
	for _, line := range wrapText(hint, maxW) {
		b.WriteString(o.theme.Dim.Render(line))
		b.WriteByte('\n')
	}
	return b.String()
}
