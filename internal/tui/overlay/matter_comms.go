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
	commWidthPct  = 76
	commHeightPct = 64
	commMinWidth  = 68
	commMinHeight = 12
)

type matterCommsLoadedMsg struct {
	items []domain.MatterEvent
	err   error
}

// MatterComms is the matter's communications log: emails, phone calls, examiner
// interviews, filings, and notes, newest first. Enter views the full comment, d
// deletes an entry; new entries are added with :log.comm.
type MatterComms struct {
	client  *rpc.Client
	theme   render.Theme
	project domain.ProjectID

	items   []domain.MatterEvent
	cursor  int
	loading bool
	loadErr string

	confirmDelete bool
	msg           string
}

func NewMatterComms(client *rpc.Client, theme render.Theme, project domain.ProjectID) *MatterComms {
	return &MatterComms{client: client, theme: theme, project: project, loading: true}
}

func (o *MatterComms) Title() string { return "Communications Log" }

func (o *MatterComms) Handles() []command.ID { return nil }

func (o *MatterComms) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *MatterComms) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, commWidthPct, commHeightPct, commMinWidth, commMinHeight)
}

func (o *MatterComms) Init() tea.Cmd { return o.loadCmd() }

func (o *MatterComms) loadCmd() tea.Cmd {
	client, project := o.client, o.project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.MatterEventListResult
		err := client.Call(ctx, proto.MethodMatterEventList, proto.MatterEventListParams{Project: project}, &res)
		return matterCommsLoadedMsg{items: res.Events, err: err}
	}
}

func (o *MatterComms) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if m, ok := msg.(matterCommsLoadedMsg); ok {
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

func (o *MatterComms) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if o.loading {
		return o, nil, true
	}
	if o.confirmDelete {
		switch msg.String() {
		case "y", "Y":
			o.confirmDelete = false
			return o, o.deleteSelected(), true
		default:
			o.confirmDelete = false
			o.msg = "delete cancelled"
			return o, nil, true
		}
	}
	switch msg.Type {
	case tea.KeyEsc:
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case tea.KeyUp:
		o.moveCursor(-1)
		return o, nil, true
	case tea.KeyDown:
		o.moveCursor(1)
		return o, nil, true
	case tea.KeyEnter:
		return o, o.viewSelected(), true
	case tea.KeyRunes:
		switch msg.String() {
		case "k":
			o.moveCursor(-1)
		case "j":
			o.moveCursor(1)
		case "d":
			if len(o.items) > 0 {
				o.confirmDelete = true
				o.msg = ""
			}
		case "q":
			return o, func() tea.Msg { return CloseOverlayMsg{} }, true
		}
		return o, nil, true
	}
	return o, nil, true
}

func (o *MatterComms) moveCursor(delta int) {
	if len(o.items) == 0 {
		return
	}
	o.cursor = max(0, min(o.cursor+delta, len(o.items)-1))
}

func (o *MatterComms) selected() (domain.MatterEvent, bool) {
	if o.cursor < 0 || o.cursor >= len(o.items) {
		return domain.MatterEvent{}, false
	}
	return o.items[o.cursor], true
}

func (o *MatterComms) viewSelected() tea.Cmd {
	ev, ok := o.selected()
	if !ok {
		return nil
	}
	title := ev.Kind.Label()
	if ev.Summary != "" {
		title += " — " + ev.Summary
	}
	body := ev.Comment
	if strings.TrimSpace(body) == "" {
		body = "(no comment recorded)"
	}
	return func() tea.Msg { return OpenDocumentTextMsg{Title: title, Text: body} }
}

func (o *MatterComms) deleteSelected() tea.Cmd {
	ev, ok := o.selected()
	if !ok {
		return nil
	}
	client, id := o.client, ev.ID
	reload := o.loadCmd()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.MatterEventResult
		if err := client.Call(ctx, proto.MethodMatterEventDelete,
			proto.MatterEventIDParams{ID: id}, &res); err != nil {
			return matterCommsLoadedMsg{err: err}
		}
		return reload()
	}
}

func (o *MatterComms) View(maxW, maxH int) string {
	if o.loading {
		return o.theme.Dim.Render("loading communications…")
	}
	if o.loadErr != "" {
		return o.theme.Error.Render("error: " + o.loadErr)
	}
	if len(o.items) == 0 {
		return o.theme.Dim.Render(render.Truncate("No communications yet. Add one with :log.comm", maxW))
	}

	var b strings.Builder
	bodyRows := max(maxH-2, 1)
	for i := 0; i < len(o.items) && i < bodyRows; i++ {
		ev := o.items[i]
		date := "—"
		if !ev.OccurredAt.IsZero() {
			date = ev.OccurredAt.Format(domain.DateLayout)
		}
		summary := ev.Summary
		if summary == "" {
			summary = ev.Comment
		}
		row := fmt.Sprintf("%-10s %-11s %-12s %s", date, ev.Kind.Label(), ev.Party, summary)
		cell := render.Pad(render.Truncate(row, maxW), maxW)
		if i == o.cursor {
			b.WriteString(o.theme.Selected.Render(cell))
		} else {
			b.WriteString(o.theme.Row.Render(cell))
		}
		b.WriteByte('\n')
	}

	footer := "↑/↓ move · enter view comment · d delete · :log.comm adds · esc close"
	switch {
	case o.confirmDelete:
		footer = "delete this entry? y to confirm, any key to cancel"
	case o.msg != "":
		footer = o.msg + " · " + footer
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(footer, maxW)))
	return b.String()
}
