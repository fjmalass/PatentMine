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
	trWidthPct  = 78
	trHeightPct = 66
	trMinWidth  = 70
	trMinHeight = 14
)

type timeReviewLoadedMsg struct {
	items []domain.TimeEntry
	err   error
}

// activityCycle is the order the review overlay cycles a time entry's activity
// through when correcting it.
var activityCycle = []domain.TimeActivity{
	domain.TimeReading, domain.TimeWriting, domain.TimeAI, domain.TimeCall, domain.TimeAdmin,
}

// TimeReview is the attorney's review queue for auto-captured time: every
// unvalidated entry, which can be corrected (activity, duration, note) and then
// validated (or deleted) before it counts toward billing. Edits are local until
// the entry is validated, mirroring legal timekeeping where captured time is a
// draft until a human signs off.
type TimeReview struct {
	client  *rpc.Client
	theme   render.Theme
	project domain.ProjectID

	items   []domain.TimeEntry
	cursor  int
	loading bool
	loadErr string

	editingNote bool
	noteBuf     string
	msg         string
}

func NewTimeReview(client *rpc.Client, theme render.Theme, project domain.ProjectID) *TimeReview {
	return &TimeReview{client: client, theme: theme, project: project, loading: true}
}

func (o *TimeReview) Title() string { return "Review Time — validate before billing" }

func (o *TimeReview) Handles() []command.ID { return nil }

func (o *TimeReview) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *TimeReview) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, trWidthPct, trHeightPct, trMinWidth, trMinHeight)
}

func (o *TimeReview) Init() tea.Cmd { return o.loadCmd() }

func (o *TimeReview) loadCmd() tea.Cmd {
	client, project := o.client, o.project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.TimeListResult
		err := client.Call(ctx, proto.MethodTimeUnvalidated, proto.TimeListParams{Project: project}, &res)
		return timeReviewLoadedMsg{items: res.Entries, err: err}
	}
}

func (o *TimeReview) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if m, ok := msg.(timeReviewLoadedMsg); ok {
		o.loading = false
		if m.err != nil {
			o.loadErr = m.err.Error()
			return o, nil
		}
		o.items = m.items
		o.loadErr = ""
		if o.cursor >= len(o.items) {
			o.cursor = max(len(o.items)-1, 0)
		}
	}
	return o, nil
}

func (o *TimeReview) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if o.loading {
		return o, nil, true
	}
	if o.editingNote {
		return o.handleNoteKey(msg)
	}
	switch msg.Type {
	case tea.KeyEsc:
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case tea.KeyUp:
		o.move(-1)
	case tea.KeyDown:
		o.move(1)
	case tea.KeyRunes, tea.KeySpace:
		return o.handleRune(msg.String())
	}
	return o, nil, true
}

func (o *TimeReview) handleRune(s string) (Overlay, tea.Cmd, bool) {
	e := o.sel()
	switch s {
	case "k":
		o.move(-1)
	case "j":
		o.move(1)
	case "a": // cycle activity
		if e != nil {
			e.Activity = nextActivity(e.Activity)
		}
	case "+", "=": // +5 min
		if e != nil {
			e.Seconds += 300
		}
	case "-", "_": // -5 min, floor 60s
		if e != nil {
			e.Seconds = max(60, e.Seconds-300)
		}
	case "n": // edit note
		if e != nil {
			o.editingNote = true
			o.noteBuf = e.Note
		}
	case "v": // validate selected (commits local edits)
		return o, o.validateSelected(), true
	case "V": // validate all as-shown
		return o, o.validateAll(), true
	case "d":
		return o, o.deleteSelected(), true
	case "q":
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	return o, nil, true
}

func (o *TimeReview) handleNoteKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyEsc:
		o.editingNote = false
	case tea.KeyEnter:
		if e := o.sel(); e != nil {
			e.Note = strings.TrimSpace(o.noteBuf)
		}
		o.editingNote = false
	case tea.KeyBackspace:
		if len(o.noteBuf) > 0 {
			r := []rune(o.noteBuf)
			o.noteBuf = string(r[:len(r)-1])
		}
	case tea.KeyRunes, tea.KeySpace:
		o.noteBuf += msg.String()
	}
	return o, nil, true
}

func (o *TimeReview) move(delta int) {
	if len(o.items) == 0 {
		return
	}
	o.cursor = max(0, min(o.cursor+delta, len(o.items)-1))
}

func (o *TimeReview) sel() *domain.TimeEntry {
	if o.cursor < 0 || o.cursor >= len(o.items) {
		return nil
	}
	return &o.items[o.cursor]
}

func (o *TimeReview) validateSelected() tea.Cmd {
	e := o.sel()
	if e == nil {
		return nil
	}
	return o.updateCmd(*e, true)
}

func (o *TimeReview) validateAll() tea.Cmd {
	if len(o.items) == 0 {
		return nil
	}
	client := o.client
	entries := append([]domain.TimeEntry(nil), o.items...)
	reload := o.loadCmd()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		for _, e := range entries {
			var res proto.TimeEntryResult
			if err := client.Call(ctx, proto.MethodTimeUpdate, proto.TimeUpdateParams{
				ID: e.ID, Seconds: e.Seconds, Activity: e.Activity, Note: e.Note, Validated: true,
			}, &res); err != nil {
				return timeReviewLoadedMsg{err: err}
			}
		}
		return reload()
	}
}

func (o *TimeReview) updateCmd(e domain.TimeEntry, validated bool) tea.Cmd {
	client := o.client
	reload := o.loadCmd()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.TimeEntryResult
		if err := client.Call(ctx, proto.MethodTimeUpdate, proto.TimeUpdateParams{
			ID: e.ID, Seconds: e.Seconds, Activity: e.Activity, Note: e.Note, Validated: validated,
		}, &res); err != nil {
			return timeReviewLoadedMsg{err: err}
		}
		return reload()
	}
}

func (o *TimeReview) deleteSelected() tea.Cmd {
	e := o.sel()
	if e == nil {
		return nil
	}
	client, id := o.client, e.ID
	reload := o.loadCmd()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.TimeEntryResult
		if err := client.Call(ctx, proto.MethodTimeDelete, proto.TimeIDParams{ID: id}, &res); err != nil {
			return timeReviewLoadedMsg{err: err}
		}
		return reload()
	}
}

func nextActivity(a domain.TimeActivity) domain.TimeActivity {
	for i, x := range activityCycle {
		if x == a {
			return activityCycle[(i+1)%len(activityCycle)]
		}
	}
	return activityCycle[0]
}

func (o *TimeReview) View(maxW, maxH int) string {
	if o.loading {
		return o.theme.Dim.Render("loading time entries…")
	}
	if o.loadErr != "" {
		return o.theme.Error.Render("error: " + o.loadErr)
	}
	if len(o.items) == 0 {
		return o.theme.Dim.Render(render.Truncate("All time is validated. Nothing to review.", maxW))
	}

	var b strings.Builder
	bodyRows := max(maxH-2, 1)
	for i := 0; i < len(o.items) && i < bodyRows; i++ {
		e := o.items[i]
		note := e.Note
		if o.editingNote && i == o.cursor {
			note = o.noteBuf + "▏"
		}
		row := fmt.Sprintf("%-9s %7s  %-6s %s", e.Activity.Label(), reviewDuration(e.Seconds), string(e.Source), note)
		cell := render.Pad(render.Truncate(row, maxW), maxW)
		if i == o.cursor {
			b.WriteString(o.theme.Selected.Render(cell))
		} else {
			b.WriteString(o.theme.Row.Render(cell))
		}
		b.WriteByte('\n')
	}

	footer := "a activity · +/- 5m · n note · v validate · V all · d delete · esc close"
	if o.editingNote {
		footer = "note: type · enter save · esc cancel"
	} else if o.msg != "" {
		footer = o.msg + " · " + footer
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(footer, maxW)))
	return b.String()
}

func reviewDuration(secs int) string {
	m := secs / 60
	s := secs % 60
	return fmt.Sprintf("%d:%02d", m, s)
}
