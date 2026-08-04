package overlay

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/tui/render"
)

const (
	dlWidthPct  = 80
	dlHeightPct = 64
	dlMinWidth  = 70
	dlMinHeight = 14
)

type deadlinesLoadedMsg struct {
	items []domain.Deadline
	err   error
}

// Deadlines is the cross-matter docket: every pending deadline — office-action
// responses and patent maintenance fees alike — soonest due first, with an
// overdue / due-soon cue. p marks one done (paid/filed), x dismisses it; an
// office-action response deadline (synthesized) is closed by responding, not here.
type Deadlines struct {
	client *rpc.Client
	theme  render.Theme

	items        []domain.Deadline
	filtered     []domain.Deadline
	showRenewals bool
	cursor       int
	loading      bool
	loadErr      string
	msg          string
	jump         JumpNavigator
}

func NewDeadlines(client *rpc.Client, theme render.Theme) *Deadlines {
	return &Deadlines{client: client, theme: theme, loading: true}
}

func (o *Deadlines) Title() string { return "Deadlines" }

func (o *Deadlines) Handles() []command.ID { return nil }

func (o *Deadlines) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *Deadlines) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, dlWidthPct, dlHeightPct, dlMinWidth, dlMinHeight)
}

func (o *Deadlines) Init() tea.Cmd { return o.loadCmd() }

func (o *Deadlines) loadCmd() tea.Cmd {
	client := o.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.DeadlineListResult
		err := client.Call(ctx, proto.MethodDeadlineList, struct{}{}, &res)
		return deadlinesLoadedMsg{items: res.Deadlines, err: err}
	}
}

func (o *Deadlines) filterItems() {
	if !o.showRenewals {
		o.filtered = o.items
	} else {
		o.filtered = nil
		for _, d := range o.items {
			if d.Kind == domain.DeadlineMaintenanceFee || d.Kind == domain.DeadlineAnnuity {
				o.filtered = append(o.filtered, d)
			}
		}
	}
	if o.cursor >= len(o.filtered) {
		o.cursor = max(len(o.filtered)-1, 0)
	}
}

func (o *Deadlines) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if m, ok := msg.(deadlinesLoadedMsg); ok {
		o.loading = false
		if m.err != nil {
			o.loadErr = m.err.Error()
			return o, nil
		}
		o.items = m.items
		o.loadErr = ""
		o.filterItems()
	}
	return o, nil
}

func (o *Deadlines) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if o.loading {
		return o, nil, true
	}
	if newCursor, handled := o.jump.HandleKey(msg, o.cursor, len(o.filtered)); handled {
		o.cursor = newCursor
		return o, nil, true
	}
	switch msg.Type {
	case tea.KeyEsc:
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case tea.KeyTab:
		o.showRenewals = !o.showRenewals
		o.filterItems()
		return o, nil, true
	case tea.KeyUp:
		o.move(-1)
		return o, nil, true
	case tea.KeyDown:
		o.move(1)
		return o, nil, true
	case tea.KeyRunes:
		switch msg.String() {
		case ";":
			o.jump.Active = true
			o.jump.PendingCount = 0
			o.jump.PendingG = false
			return o, nil, true
		case "k":
			o.move(-1)
		case "j":
			o.move(1)
		case "p":
			return o, o.setStatus(domain.DeadlineDone), true
		case "x":
			return o, o.setStatus(domain.DeadlineDismissed), true
		case "q":
			return o, func() tea.Msg { return CloseOverlayMsg{} }, true
		}
		return o, nil, true
	}
	return o, nil, true
}

func (o *Deadlines) move(delta int) {
	if len(o.filtered) == 0 {
		return
	}
	o.cursor = max(0, min(o.cursor+delta, len(o.filtered)-1))
}

// setStatus marks the selected deadline done/dismissed. Synthesized office-action
// response deadlines ("oa:<id>") are not stored rows — they clear when the
// office action is responded to — so they cannot be set here.
func (o *Deadlines) setStatus(status domain.DeadlineStatus) tea.Cmd {
	if o.cursor < 0 || o.cursor >= len(o.filtered) {
		return nil
	}
	d := o.filtered[o.cursor]
	if strings.HasPrefix(d.ID, "oa:") {
		o.msg = "respond to the office action to clear its deadline"
		return nil
	}
	client, id := o.client, d.ID
	reload := o.loadCmd()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.DeadlineListResult
		if err := client.Call(ctx, proto.MethodDeadlineSetStatus,
			proto.DeadlineStatusParams{ID: id, Status: status}, &res); err != nil {
			return deadlinesLoadedMsg{err: err}
		}
		return reload()
	}
}

func (o *Deadlines) View(maxW, maxH int) string {
	if o.loading {
		return o.theme.Dim.Render("loading deadlines…")
	}
	if o.loadErr != "" {
		return o.theme.Error.Render("error: " + o.loadErr)
	}

	var tabLine string
	if !o.showRenewals {
		tabLine = o.theme.Selected.Render(" All Deadlines ") + "  " + o.theme.Dim.Render(" Renewals Only ")
	} else {
		tabLine = o.theme.Dim.Render(" All Deadlines ") + "  " + o.theme.Selected.Render(" Renewals Only ")
	}

	var b strings.Builder
	b.WriteString(tabLine + "\n\n")

	if len(o.filtered) == 0 {
		b.WriteString(o.theme.Dim.Render(render.Truncate(
			"No pending deadlines matching filters. Track patent renewals with :renewal.track <number>.", maxW)))
		b.WriteByte('\n')
	} else {
		bodyRows := max(maxH-4, 1)
		for i := 0; i < len(o.filtered) && i < bodyRows; i++ {
			d := o.filtered[i]
			lineNum := i + 1
			gutter := o.jump.GutterPrefix(lineNum)
			row := fmt.Sprintf("%s%-16s %-13s %s", gutter, deadlineDueLabel(d), d.Kind.Label(), d.Title)
			cell := render.Pad(render.Truncate(row, maxW), maxW)
			if i == o.cursor {
				b.WriteString(o.theme.Selected.Render(cell))
			} else {
				b.WriteString(o.theme.Row.Render(cell))
			}
			b.WriteByte('\n')
		}
	}
	var hint string
	if o.jump.Active {
		hint = o.jump.HintSuffix(o.cursor, -1, false)
	} else {
		hint = "tab toggle filter · ↑/↓ move · p mark done · x dismiss · esc close · [;] jump mode"
	}
	if o.msg != "" {
		hint = o.msg + " · " + hint
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(hint, maxW)))
	return b.String()
}

// deadlineDueLabel renders the urgency cue for a deadline's due date.
func deadlineDueLabel(d domain.Deadline) string {
	if d.DueDate.IsZero() {
		return "—"
	}
	days := d.DaysUntilDue()
	date := d.DueDate.Format(domain.DateLayout)
	switch {
	case days < 0:
		return "OVERDUE " + date
	case days == 0:
		return "TODAY " + date
	case days <= 30:
		return fmt.Sprintf("%dd %s", days, date)
	default:
		return date
	}
}
