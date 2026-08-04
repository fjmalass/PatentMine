package overlay

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/tui/render"
)

const (
	docsListWidthPct  = 70
	docsListHeightPct = 60
	docsListMinWidth  = 64
	docsListMinHeight = 12
)

type docsListLoadedMsg struct {
	items []proto.DocSummary
	err   error
}

// OpenDocMsg asks the app to open one daemon-provided project doc.
type OpenDocMsg struct {
	ID string
}

type DocsList struct {
	client  *rpc.Client
	theme   render.Theme
	items   []proto.DocSummary
	cursor  int
	loading bool
	loadErr string
	jump    JumpNavigator
}

func NewDocsList(client *rpc.Client, theme render.Theme) *DocsList {
	return &DocsList{client: client, theme: theme, loading: true}
}

func (o *DocsList) Title() string { return "Docs" }

func (o *DocsList) Handles() []command.ID { return nil }

func (o *DocsList) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *DocsList) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, docsListWidthPct, docsListHeightPct, docsListMinWidth, docsListMinHeight)
}

func (o *DocsList) Init() tea.Cmd {
	client := o.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var res proto.DocsListResult
		err := client.Call(ctx, proto.MethodDocsList, nil, &res)
		return docsListLoadedMsg{items: res.Docs, err: err}
	}
}

func (o *DocsList) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if m, ok := msg.(docsListLoadedMsg); ok {
		o.loading = false
		if m.err != nil {
			o.loadErr = m.err.Error()
			return o, nil
		}
		o.items = m.items
		if o.cursor >= len(o.items) {
			o.cursor = max(len(o.items)-1, 0)
		}
	}
	return o, nil
}

func (o *DocsList) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if o.loading {
		return o, nil, true
	}
	if newCursor, handled := o.jump.HandleKey(msg, o.cursor, len(o.items)); handled {
		o.cursor = newCursor
		return o, nil, true
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
		return o, o.openSelected(), true
	case tea.KeyRunes:
		switch msg.String() {
		case ";":
			o.jump.Active = true
			o.jump.PendingCount = 0
			o.jump.PendingG = false
		case "k":
			o.moveCursor(-1)
		case "j":
			o.moveCursor(1)
		case "q":
			return o, func() tea.Msg { return CloseOverlayMsg{} }, true
		}
		return o, nil, true
	}
	return o, nil, true
}

func (o *DocsList) moveCursor(delta int) {
	if len(o.items) == 0 {
		return
	}
	o.cursor = max(0, min(o.cursor+delta, len(o.items)-1))
}

func (o *DocsList) openSelected() tea.Cmd {
	if o.cursor < 0 || o.cursor >= len(o.items) {
		return nil
	}
	id := o.items[o.cursor].ID
	return func() tea.Msg { return OpenDocMsg{ID: id} }
}

func (o *DocsList) View(maxW, maxH int) string {
	if o.loading {
		return o.theme.Dim.Render("loading docs...")
	}
	if o.loadErr != "" {
		return o.theme.Error.Render("error: " + o.loadErr)
	}
	if len(o.items) == 0 {
		return o.theme.Dim.Render(render.Truncate("No docs found. Expected README.md, CHANGELOG.md, or docs/*.md.", maxW))
	}

	var b strings.Builder
	header := "     " + render.Pad("ID", 28) + " Title"
	b.WriteString(o.theme.Header.Render(render.Pad(render.Truncate(header, maxW), maxW)))
	b.WriteByte('\n')
	bodyRows := max(maxH-3, 1)
	for i := 0; i < len(o.items) && i < bodyRows; i++ {
		item := o.items[i]
		row := fmt.Sprintf("%s%s %s", o.jump.GutterPrefix(i+1), render.Pad(render.Truncate(item.ID, 28), 28), item.Title)
		cell := render.Pad(render.Truncate(row, maxW), maxW)
		if i == o.cursor {
			b.WriteString(o.theme.Selected.Render(cell))
		} else {
			b.WriteString(o.theme.Row.Render(cell))
		}
		b.WriteByte('\n')
	}
	hint := "up/down or j/k move | enter open | esc close | [;] jump mode"
	if o.jump.Active {
		hint = o.jump.HintSuffix(o.cursor, -1, false)
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(hint, maxW)))
	return b.String()
}
