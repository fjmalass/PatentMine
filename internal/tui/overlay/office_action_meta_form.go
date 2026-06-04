package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/tui/render"
)

// OfficeActionMetaForm captures the office-action metadata at import time —
// crucially the examiner name, which the path-only import dropped — before the
// daemon copies the file in. Tab/Shift+Tab (or ↑/↓) move between fields, Enter
// imports, Esc cancels. The response deadline is computed from the mail date and
// type by the engine, so it is not entered here.
type OfficeActionMetaForm struct {
	theme   render.Theme
	project domain.ProjectID
	source  string // the picked file path
	values  [5]string
	focus   int
}

const (
	oamExaminer = iota
	oamMailDate
	oamType
	oamArtUnit
	oamAppNumber
)

var oamLabels = [5]string{
	"Examiner",
	"Mail Date (YYYY-MM-DD)",
	"Type (non_final/final/restriction/advisory/notice_of_allowance)",
	"Art Unit",
	"Application Number",
}

// NewOfficeActionMetaForm builds the form for the picked source file, pre-filled
// from the active project where the office action inherits the same examiner, art
// unit, and application number.
func NewOfficeActionMetaForm(theme render.Theme, project domain.Project, source string) *OfficeActionMetaForm {
	o := &OfficeActionMetaForm{theme: theme, project: project.ID, source: source}
	o.values[oamExaminer] = project.LatestExaminer()
	o.values[oamType] = string(domain.OANonFinal)
	o.values[oamArtUnit] = project.ArtUnit
	o.values[oamAppNumber] = project.ApplicationNumber
	return o
}

func (o *OfficeActionMetaForm) Title() string { return "Import Office Action" }

func (o *OfficeActionMetaForm) Handles() []command.ID { return nil }

func (o *OfficeActionMetaForm) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *OfficeActionMetaForm) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyEsc:
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
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

func (o *OfficeActionMetaForm) submit() tea.Cmd {
	params := proto.OfficeActionImportParams{
		Project:           o.project,
		SourcePath:        o.source,
		Examiner:          strings.TrimSpace(o.values[oamExaminer]),
		MailDate:          strings.TrimSpace(o.values[oamMailDate]),
		Type:              domain.OAType(strings.TrimSpace(o.values[oamType])),
		ArtUnit:           strings.TrimSpace(o.values[oamArtUnit]),
		ApplicationNumber: strings.TrimSpace(o.values[oamAppNumber]),
	}
	return func() tea.Msg { return OfficeActionImportSubmitMsg{Params: params} }
}

func (o *OfficeActionMetaForm) View(maxW, _ int) string {
	var b strings.Builder
	b.WriteString(o.theme.Dim.Render(render.Truncate("file: "+o.source, maxW)))
	b.WriteString("\n\n")
	for i, label := range oamLabels {
		marker := "  "
		if i == o.focus {
			marker = "▸ "
		}
		line := fmt.Sprintf("%s%-22s %s", marker, shortLabel(label)+":", o.values[i])
		if i == o.focus {
			b.WriteString(o.theme.Title.Render(render.Truncate(line, maxW)))
		} else {
			b.WriteString(o.theme.Row.Render(render.Truncate(line, maxW)))
		}
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	hint := "[tab] field · [enter] import · [esc] cancel · [ctrl+u] clear · deadline auto-set from mail date"
	b.WriteString(o.theme.Dim.Render(render.Truncate(hint, maxW)))
	return b.String()
}

// shortLabel trims the parenthetical help off a field label for the aligned
// left column; the full label (with the help) is shown in the type field's hint.
func shortLabel(label string) string {
	if i := strings.IndexByte(label, '('); i > 0 {
		return strings.TrimSpace(label[:i])
	}
	return label
}
