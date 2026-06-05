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
	conflictWidthPct  = 78
	conflictHeightPct = 64
	conflictMinWidth  = 70
	conflictMinHeight = 12
)

type conflictListLoadedMsg struct {
	items []domain.Conflict
	err   error
}

// ConflictList is the matter's conflicts list: records flagged as conflicting
// (material prior art, interfering applications), newest first. r resolves, w
// waives, o re-opens, and d deletes the selected edge; new ones are added with
// :flag.conflict. The patent list shows a ⚠ badge for records with an open one.
type ConflictList struct {
	client  *rpc.Client
	theme   render.Theme
	project domain.ProjectID

	items   []domain.Conflict
	cursor  int
	loading bool
	loadErr string

	confirmDelete bool
	msg           string
}

func NewConflictList(client *rpc.Client, theme render.Theme, project domain.ProjectID) *ConflictList {
	return &ConflictList{client: client, theme: theme, project: project, loading: true}
}

func (o *ConflictList) Title() string { return "Conflicts" }

func (o *ConflictList) Handles() []command.ID { return nil }

func (o *ConflictList) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *ConflictList) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, conflictWidthPct, conflictHeightPct, conflictMinWidth, conflictMinHeight)
}

func (o *ConflictList) Init() tea.Cmd { return o.loadCmd() }

func (o *ConflictList) loadCmd() tea.Cmd {
	client, project := o.client, o.project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.ConflictListResult
		err := client.Call(ctx, proto.MethodConflictList, proto.ConflictListParams{Project: project}, &res)
		return conflictListLoadedMsg{items: res.Conflicts, err: err}
	}
}

func (o *ConflictList) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if m, ok := msg.(conflictListLoadedMsg); ok {
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

func (o *ConflictList) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
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
		case "r":
			return o, o.setStatus(domain.ConflictResolved), true
		case "w":
			return o, o.setStatus(domain.ConflictWaived), true
		case "o":
			return o, o.setStatus(domain.ConflictOpen), true
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

func (o *ConflictList) moveCursor(delta int) {
	if len(o.items) == 0 {
		return
	}
	o.cursor = max(0, min(o.cursor+delta, len(o.items)-1))
}

func (o *ConflictList) selected() (domain.Conflict, bool) {
	if o.cursor < 0 || o.cursor >= len(o.items) {
		return domain.Conflict{}, false
	}
	return o.items[o.cursor], true
}

func (o *ConflictList) viewSelected() tea.Cmd {
	c, ok := o.selected()
	if !ok {
		return nil
	}
	title := c.Record.DisplayString() + " — " + c.Status.Label()
	body := c.Reason
	if strings.TrimSpace(body) == "" {
		body = "(no reason recorded)"
	}
	if !c.Against.IsZero() {
		body = "Conflicts with: " + c.Against.DisplayString() + "\n\n" + body
	}
	return func() tea.Msg { return OpenDocumentTextMsg{Title: title, Text: body} }
}

// setStatus resolves/waives/re-opens the selected conflict, then reloads.
func (o *ConflictList) setStatus(status domain.ConflictStatus) tea.Cmd {
	c, ok := o.selected()
	if !ok {
		return nil
	}
	if c.Status == status {
		return nil
	}
	client, id := o.client, c.ID
	reload := o.loadCmd()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.ConflictResult
		if err := client.Call(ctx, proto.MethodConflictResolve,
			proto.ConflictResolveParams{ID: id, Status: status}, &res); err != nil {
			return conflictListLoadedMsg{err: err}
		}
		return reload()
	}
}

func (o *ConflictList) deleteSelected() tea.Cmd {
	c, ok := o.selected()
	if !ok {
		return nil
	}
	client, id := o.client, c.ID
	reload := o.loadCmd()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.ConflictResult
		if err := client.Call(ctx, proto.MethodConflictDelete,
			proto.ConflictIDParams{ID: id}, &res); err != nil {
			return conflictListLoadedMsg{err: err}
		}
		return reload()
	}
}

func (o *ConflictList) View(maxW, maxH int) string {
	if o.loading {
		return o.theme.Dim.Render("loading conflicts…")
	}
	if o.loadErr != "" {
		return o.theme.Error.Render("error: " + o.loadErr)
	}
	if len(o.items) == 0 {
		return o.theme.Dim.Render(render.Truncate("No conflicts yet. Flag one with :flag.conflict <patent> [reason]", maxW))
	}

	var b strings.Builder
	bodyRows := max(maxH-2, 1)
	for i := 0; i < len(o.items) && i < bodyRows; i++ {
		c := o.items[i]
		date := "—"
		if !c.FlaggedAt.IsZero() {
			date = c.FlaggedAt.Format(domain.DateLayout)
		}
		row := fmt.Sprintf("%-10s %-11s %-16s %s", date, c.Status.Label(), c.Record.DisplayString(), c.Reason)
		cell := render.Pad(render.Truncate(row, maxW), maxW)
		if i == o.cursor {
			b.WriteString(o.theme.Selected.Render(cell))
		} else {
			b.WriteString(o.theme.Row.Render(cell))
		}
		b.WriteByte('\n')
	}

	footer := "↑/↓ move · enter view · r resolve · w waive · o reopen · d delete · :flag.conflict adds · esc close"
	switch {
	case o.confirmDelete:
		footer = "delete this conflict? y to confirm, any key to cancel"
	case o.msg != "":
		footer = o.msg + " · " + footer
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(footer, maxW)))
	return b.String()
}
