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
	oaListWidthPct  = 70
	oaListHeightPct = 60
	oaListMinWidth  = 64
	oaListMinHeight = 12
)

type officeActionListLoadedMsg struct {
	items []domain.OfficeAction
	err   error
}

// OpenOfficeActionMsg asks the app to open the split editor for one office
// action chosen in the list.
type OpenOfficeActionMsg struct {
	OA domain.OfficeAction
}

// OfficeActionList is the table of a project's office actions; enter opens the
// selected one in the split text/notes editor.
type OfficeActionList struct {
	client  *rpc.Client
	theme   render.Theme
	project domain.ProjectID

	items   []domain.OfficeAction
	cursor  int
	loading bool
	loadErr string
	jump    JumpNavigator
}

func NewOfficeActionList(client *rpc.Client, theme render.Theme, project domain.ProjectID) *OfficeActionList {
	return &OfficeActionList{client: client, theme: theme, project: project, loading: true}
}

func (o *OfficeActionList) Title() string { return "Office Actions" }

func (o *OfficeActionList) Handles() []command.ID { return nil }

func (o *OfficeActionList) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *OfficeActionList) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, oaListWidthPct, oaListHeightPct, oaListMinWidth, oaListMinHeight)
}

func (o *OfficeActionList) Init() tea.Cmd {
	client, project := o.client, o.project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.OfficeActionListResult
		err := client.Call(ctx, proto.MethodOfficeActionList, proto.OfficeActionListParams{Project: project}, &res)
		return officeActionListLoadedMsg{items: res.OfficeActions, err: err}
	}
}

func (o *OfficeActionList) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if m, ok := msg.(officeActionListLoadedMsg); ok {
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

func (o *OfficeActionList) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
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
			return o, nil, true
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

func (o *OfficeActionList) moveCursor(delta int) {
	if len(o.items) == 0 {
		return
	}
	o.cursor = max(0, min(o.cursor+delta, len(o.items)-1))
}

func (o *OfficeActionList) openSelected() tea.Cmd {
	if o.cursor < 0 || o.cursor >= len(o.items) {
		return nil
	}
	oa := o.items[o.cursor]
	return func() tea.Msg { return OpenOfficeActionMsg{OA: oa} }
}

func (o *OfficeActionList) View(maxW, maxH int) string {
	if o.loading {
		return o.theme.Dim.Render("loading office actions…")
	}
	if o.loadErr != "" {
		return o.theme.Error.Render("error: " + o.loadErr)
	}
	if len(o.items) == 0 {
		return o.theme.Dim.Render(render.Truncate("No office actions yet. Add one with :officeaction.add", maxW))
	}

	var b strings.Builder
	// col truncates then pads s to a fixed display width so the columns line up.
	col := func(s string, w int) string { return render.Pad(render.Truncate(s, w), w) }
	body := func(name, typ, mailed, added, examiner string) string {
		return col(name, 22) + " " + col(typ, 14) + " " + col(mailed, 12) + " " + col(added, 12) + " " + examiner
	}

	// Header, aligned to the row prefix (gutter " N " + notes mark + space = 5).
	header := "     " + body("Name", "Type", "Mailed", "Added", "Examiner")
	b.WriteString(o.theme.Header.Render(render.Pad(render.Truncate(header, maxW), maxW)))
	b.WriteByte('\n')

	bodyRows := max(maxH-3, 1)
	for i := 0; i < len(o.items) && i < bodyRows; i++ {
		oa := o.items[i]
		mailed := "—"
		if !oa.MailDate.IsZero() {
			mailed = oa.MailDate.Format(domain.DateLayout)
		}
		added := "—"
		if !oa.ImportedAt.IsZero() {
			added = oa.ImportedAt.Format(domain.DateLayout)
		}
		typ := string(oa.Type)
		if typ == "" {
			typ = "—"
		}
		name := oa.Name
		if strings.TrimSpace(name) == "" {
			name = "(unnamed)"
		}
		notesMark := " "
		if strings.TrimSpace(oa.Notes) != "" {
			notesMark = "✎"
		}
		lineNum := i + 1
		gutter := o.jump.GutterPrefix(lineNum)
		row := fmt.Sprintf("%s%s %s", gutter, notesMark, body(name, typ, mailed, added, oa.Examiner))
		cell := render.Pad(render.Truncate(row, maxW), maxW)
		if i == o.cursor {
			b.WriteString(o.theme.Selected.Render(cell))
		} else {
			b.WriteString(o.theme.Row.Render(cell))
		}
		b.WriteByte('\n')
	}
	var hint string
	if o.jump.Active {
		hint = o.jump.HintSuffix(o.cursor, -1, false)
	} else {
		hint = "↑/↓ or j/k move · enter open · esc close · [;] jump mode"
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(hint, maxW)))
	return b.String()
}
