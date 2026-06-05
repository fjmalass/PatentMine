package overlay

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/tui/render"
)

const (
	cefKind = iota
	cefParty
	cefDate
	cefSummary
	cefComment
)

// cefFields are the labeled fields of the communications-log entry form.
var cefFields = []fieldFormField{
	{Label: "Kind", Kind: fieldText},
	{Label: "With", Kind: fieldText},
	{Label: "Date", Kind: fieldText},
	{Label: "Summary", Kind: fieldText},
	{Label: "What happened", Kind: fieldText},
}

// CommEventForm captures one communications-log entry — an email, a phone call,
// an examiner interview, a filing, or a note — recording who it was with and
// what happened. It uses the shared view/edit field model (Enter to edit a
// field, Ctrl+S to save, Esc to cancel).
type CommEventForm struct {
	theme   render.Theme
	project domain.ProjectID
	form    fieldForm
}

// NewCommEventForm builds the entry form for a project's communications log.
func NewCommEventForm(theme render.Theme, project domain.ProjectID) *CommEventForm {
	o := &CommEventForm{theme: theme, project: project, form: newFieldForm(cefFields)}
	o.form.SetValue(cefKind, string(domain.MatterEventPhone))
	return o
}

func (o *CommEventForm) Title() string { return "Log Communication" }

func (o *CommEventForm) Handles() []command.ID { return nil }

func (o *CommEventForm) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *CommEventForm) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch action, _ := o.form.HandleKey(msg); action {
	case fieldFormSubmit:
		return o, o.submit(), true
	case fieldFormCancel:
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	default:
		return o, nil, true
	}
}

func (o *CommEventForm) submit() tea.Cmd {
	params := proto.MatterEventAddParams{
		Project:    o.project,
		Kind:       domain.MatterEventKind(strings.TrimSpace(o.form.Value(cefKind))),
		Party:      strings.TrimSpace(o.form.Value(cefParty)),
		OccurredAt: strings.TrimSpace(o.form.Value(cefDate)),
		Summary:    strings.TrimSpace(o.form.Value(cefSummary)),
		Comment:    strings.TrimSpace(o.form.Value(cefComment)),
	}
	return func() tea.Msg { return MatterEventSubmitMsg{Params: params} }
}

func (o *CommEventForm) View(maxW, _ int) string {
	var b strings.Builder
	b.WriteString(o.form.RenderFields(o.theme, maxW, 20))
	b.WriteByte('\n')
	hint := o.form.Hint()
	if o.form.Focus() == cefKind && o.form.Editing() {
		hint += " · email/phone/interview/filed/deadline/note"
	} else if o.form.Focus() == cefDate && o.form.Editing() {
		hint += " · YYYY-MM-DD, blank = today"
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(hint, maxW)))
	return b.String()
}
