package pane

import (
	"fmt"
	"log/slog"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

// orphansLoadedMsg delivers a finished patent.orphan_list result.
type orphansLoadedMsg struct {
	requestID uint64
	rows      []domain.PatentRow
	total     int
	err       error
}

// Orphans lists every patent in the database that is not associated with any
// project. The point is to surface citation stubs that crawl created but no
// project owns, so the user can either delete them or add them to a project.
type Orphans struct {
	client   *rpc.Client
	theme    render.Theme
	handlers map[command.ID]cmdHandler

	rows    []domain.PatentRow
	total   int
	page    render.Paginator
	loading bool
	loadErr string
	loadID  uint64
	logger  *slog.Logger
}

// NewOrphans builds the orphan-patents pane.
func NewOrphans(client *rpc.Client, theme render.Theme) *Orphans {
	o := &Orphans{
		client:  client,
		theme:   theme,
		page:    render.NewPaginator(20),
		loading: true,
	}
	o.handlers = map[command.ID]cmdHandler{
		command.NavDown:     func(inv Invocation) tea.Cmd { o.page.MoveDown(inv.Repeat); return nil },
		command.NavUp:       func(inv Invocation) tea.Cmd { o.page.MoveUp(inv.Repeat); return nil },
		command.NavPageDown: func(Invocation) tea.Cmd { o.page.PageDown(); return nil },
		command.NavPageUp:   func(Invocation) tea.Cmd { o.page.PageUp(); return nil },
		command.NavTop:      func(inv Invocation) tea.Cmd { o.page.NavTop(inv.Repeat); return nil },
		command.NavBottom:   func(inv Invocation) tea.Cmd { o.page.NavBottom(inv.Repeat); return nil },
		command.Refresh:     func(Invocation) tea.Cmd { o.loading = true; return o.load() },
	}
	return o
}

// WithLogger attaches a logger.
func (o *Orphans) WithLogger(l *slog.Logger) *Orphans { o.logger = l; return o }

func (o *Orphans) log() *slog.Logger {
	if o.logger != nil {
		return o.logger
	}
	return slog.Default()
}

func (o *Orphans) Scope() command.Scope { return command.ScopeOrphans }

func (o *Orphans) Title() string {
	if o.total == 0 {
		return "Orphan patents"
	}
	return fmt.Sprintf("Orphan patents (%d)", o.total)
}

func (o *Orphans) Init() tea.Cmd { return o.load() }

func (o *Orphans) load() tea.Cmd {
	client := o.client
	if client == nil {
		return nil
	}
	requestID := nextAsyncID()
	o.loadID = requestID
	offset := o.page.Cursor() / 100 * 100
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.OrphanListResult
		err := client.Call(ctx, proto.MethodOrphanList,
			proto.OrphanListParams{Limit: 200, Offset: offset}, &res)
		return orphansLoadedMsg{requestID: requestID, rows: res.Patents, total: res.Total, err: err}
	}
}

func (o *Orphans) Command(id command.ID, inv Invocation) (Pane, tea.Cmd) {
	if handler, ok := o.handlers[id]; ok {
		return o, handler(inv)
	}
	return o, nil
}

func (o *Orphans) Handles() []command.ID { return handlerIDs(o.handlers) }

func (o *Orphans) Update(msg tea.Msg) (Pane, tea.Cmd) {
	if m, ok := msg.(orphansLoadedMsg); ok {
		if m.requestID != o.loadID {
			return o, nil
		}
		o.loading = false
		if m.err != nil {
			o.loadErr = m.err.Error()
			o.log().Error("orphan list load failed", slog.String("error", m.err.Error()))
			return o, status(text.StatusOrphanLoadFailed, true, m.err.Error())
		}
		o.loadErr = ""
		o.rows = m.rows
		o.total = m.total
		o.page.SetTotal(len(o.rows))
		return o, nil
	}
	return o, nil
}

func (o *Orphans) Selection() (domain.PatentNumber, bool) {
	cur := o.page.Cursor()
	if cur < 0 || cur >= len(o.rows) {
		return domain.PatentNumber{}, false
	}
	return o.rows[cur].Number, true
}

func (o *Orphans) View(w, h int) string {
	switch {
	case o.loading && len(o.rows) == 0:
		return o.theme.Dim.Render("loading orphan patents…")
	case o.loadErr != "":
		return o.theme.Error.Render("error: " + o.loadErr)
	case len(o.rows) == 0:
		return o.theme.Dim.Render("no orphan patents — every patent in the database belongs to a project")
	}
	o.page.SetPageSize(max(h-headerRows, 1))

	var b strings.Builder
	b.WriteString(renderTableStatusLine(o.theme, w, o.page.Cursor(), o.page.Total(),
		fmt.Sprintf("total:%d", o.total)))
	b.WriteByte('\n')
	b.WriteString(o.theme.Header.Render(orphanRow("#", "NUMBER", "FETCH", "TITLE", w)))
	start, end := o.page.Window()
	for i := start; i < end; i++ {
		row := o.rows[i]
		line := orphanRow(formatViewIndex(i), row.Number.String(), string(row.FetchState), truncTitle(row.Title), w)
		b.WriteByte('\n')
		if i == o.page.Cursor() {
			b.WriteString(o.theme.Selected.Render(render.Pad(line, w)))
		} else {
			b.WriteString(tableRowStyle(o.theme, i)(render.Pad(line, w)))
		}
	}
	return b.String()
}

func orphanRow(idx, number, fetch, title string, w int) string {
	const idxW, numW, fetchW = 4, 18, 8
	titleW := w - idxW - numW - fetchW - 6
	if titleW < 10 {
		titleW = 10
	}
	return fmt.Sprintf("%-*s  %-*s  %-*s  %-*s",
		idxW, idx, numW, render.Truncate(number, numW), fetchW, render.Truncate(fetch, fetchW), titleW, render.Truncate(title, titleW))
}

func truncTitle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "—"
	}
	return s
}
