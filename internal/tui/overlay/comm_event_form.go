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

// CommEventForm captures one communications-log entry — an email, a phone call,
// an examiner interview, a filing, or a note — recording who it was with and
// what happened. Tab/↑↓ move between fields, Enter saves, Esc cancels.
type CommEventForm struct {
	theme   render.Theme
	project domain.ProjectID
	values  [5]string
	focus   int
}

const (
	cefKind = iota
	cefParty
	cefDate
	cefSummary
	cefComment
)

var cefLabels = [5]string{
	"Kind (email/phone/interview/filed/deadline/note)",
	"With (examiner / client / …)",
	"Date (YYYY-MM-DD, blank = today)",
	"Summary",
	"What happened (comment)",
}

// NewCommEventForm builds the entry form for a project's communications log.
func NewCommEventForm(theme render.Theme, project domain.ProjectID) *CommEventForm {
	o := &CommEventForm{theme: theme, project: project}
	o.values[cefKind] = string(domain.MatterEventPhone)
	return o
}

func (o *CommEventForm) Title() string { return "Log Communication" }

func (o *CommEventForm) Handles() []command.ID { return nil }

func (o *CommEventForm) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *CommEventForm) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
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

func (o *CommEventForm) submit() tea.Cmd {
	params := proto.MatterEventAddParams{
		Project:    o.project,
		Kind:       domain.MatterEventKind(strings.TrimSpace(o.values[cefKind])),
		Party:      strings.TrimSpace(o.values[cefParty]),
		OccurredAt: strings.TrimSpace(o.values[cefDate]),
		Summary:    strings.TrimSpace(o.values[cefSummary]),
		Comment:    strings.TrimSpace(o.values[cefComment]),
	}
	return func() tea.Msg { return MatterEventSubmitMsg{Params: params} }
}

func (o *CommEventForm) View(maxW, _ int) string {
	var b strings.Builder
	for i, label := range cefLabels {
		marker := "  "
		if i == o.focus {
			marker = "▸ "
		}
		line := fmt.Sprintf("%s%-20s %s", marker, shortLabel(label)+":", o.values[i])
		if i == o.focus {
			b.WriteString(o.theme.Title.Render(render.Truncate(line, maxW)))
		} else {
			b.WriteString(o.theme.Row.Render(render.Truncate(line, maxW)))
		}
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(o.theme.Dim.Render(render.Truncate(
		"[tab] field · [enter] save · [esc] cancel · [ctrl+u] clear", maxW)))
	return b.String()
}
