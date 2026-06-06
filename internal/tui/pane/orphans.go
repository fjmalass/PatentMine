package pane

import (
	"fmt"
	"log/slog"
	"slices"
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

	allRows  []domain.PatentRow
	rows     []domain.PatentRow
	total    int
	page     render.Paginator
	loading  bool
	loadErr  string
	loadID   uint64
	logger   *slog.Logger

	searchActive  bool
	searchQuery   string
	searchScope   int
	focusedColIdx int
	sortAscending bool
	activeSort    string
}

// NewOrphans builds the orphan-patents pane.
func NewOrphans(client *rpc.Client, theme render.Theme) *Orphans {
	o := &Orphans{
		client:        client,
		theme:         theme,
		page:          render.NewPaginator(20),
		loading:       true,
		focusedColIdx: -1,
		sortAscending: true,
		activeSort:    "number",
		searchScope:   0,
	}
	o.handlers = map[command.ID]cmdHandler{
		command.NavDown:     func(inv Invocation) tea.Cmd { o.page.MoveDown(inv.Repeat); return nil },
		command.NavUp:       func(inv Invocation) tea.Cmd { o.page.MoveUp(inv.Repeat); return nil },
		command.NavPageDown: func(Invocation) tea.Cmd { o.page.PageDown(); return nil },
		command.NavPageUp:   func(Invocation) tea.Cmd { o.page.PageUp(); return nil },
		command.NavTop:      func(inv Invocation) tea.Cmd { o.page.NavTop(inv.Repeat); return nil },
		command.NavBottom:   func(inv Invocation) tea.Cmd { o.page.NavBottom(inv.Repeat); return nil },
		command.Refresh:     func(Invocation) tea.Cmd { o.loading = true; return o.load() },
		command.ColNext:     func(Invocation) tea.Cmd { return o.focusNext() },
		command.ColPrev:     func(Invocation) tea.Cmd { return o.focusPrev() },
		command.SortApply:   func(Invocation) tea.Cmd { return o.applySort() },
		command.FindOpen: func(Invocation) tea.Cmd {
			o.searchActive = true
			o.searchQuery = ""
			o.applyFilter()
			return nil
		},
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

func (o *Orphans) focusNext() tea.Cmd {
	o.focusedColIdx = render.MoveSortableColumn(o.currentCols(80), o.focusedColIdx, 1)
	return nil
}

func (o *Orphans) focusPrev() tea.Cmd {
	o.focusedColIdx = render.MoveSortableColumn(o.currentCols(80), o.focusedColIdx, -1)
	return nil
}

func (o *Orphans) applySort() tea.Cmd {
	if o.focusedColIdx < 0 {
		return nil
	}
	cols := o.currentCols(80)
	col := cols[o.focusedColIdx]
	if col.SortKey == "" {
		return nil
	}
	if o.activeSort == col.SortKey {
		o.sortAscending = !o.sortAscending
	} else {
		o.activeSort = col.SortKey
		o.sortAscending = true
	}
	o.applyFilter()
	return nil
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
		o.allRows = m.rows
		o.total = m.total
		o.applyFilter()
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

var orphansSearchScopes = []struct {
	Key   string
	Label string
}{
	{"all", "All Columns"},
	{"number", "Patent Number"},
	{"fetch", "Fetch State"},
	{"title", "Title"},
}

func (o *Orphans) applyFilter() {
	if o.searchQuery == "" {
		o.rows = o.allRows
	} else {
		o.rows = nil
		q := strings.ToLower(o.searchQuery)
		scope := orphansSearchScopes[o.searchScope].Key
		for _, row := range o.allRows {
			match := false
			switch scope {
			case "all":
				match = strings.Contains(strings.ToLower(row.Number.String()), q) ||
					strings.Contains(strings.ToLower(string(row.FetchState)), q) ||
					strings.Contains(strings.ToLower(row.Title), q)
			case "number":
				match = strings.Contains(strings.ToLower(row.Number.String()), q)
			case "fetch":
				match = strings.Contains(strings.ToLower(string(row.FetchState)), q)
			case "title":
				match = strings.Contains(strings.ToLower(row.Title), q)
			}
			if match {
				o.rows = append(o.rows, row)
			}
		}
	}
	o.sortRows()
	o.page.SetTotal(len(o.rows))
	if o.page.Cursor() >= len(o.rows) {
		o.page.NavTop(0)
	}
}

func (o *Orphans) sortRows() {
	if o.activeSort == "" {
		return
	}
	slices.SortFunc(o.rows, func(i, j domain.PatentRow) int {
		var cmp int
		switch o.activeSort {
		case "number":
			cmp = strings.Compare(i.Number.String(), j.Number.String())
		case "fetch_state":
			cmp = strings.Compare(string(i.FetchState), string(j.FetchState))
		case "title":
			cmp = strings.Compare(i.Title, j.Title)
		default:
			cmp = strings.Compare(i.Number.String(), j.Number.String())
		}
		if !o.sortAscending {
			cmp = -cmp
		}
		return cmp
	})
}

func (o *Orphans) currentCols(w int) []render.TableColumn {
	titleWidth := max(10, w-18-12-6)
	return []render.TableColumn{
		{Key: "number", Label: "NUMBER", SortKey: "number", Width: 18},
		{Key: "fetch", Label: "FETCH", SortKey: "fetch_state", Width: 12},
		{Key: "title", Label: "TITLE", SortKey: "title", Width: titleWidth},
	}
}

func (o *Orphans) HandleKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool) {
	if o.searchActive {
		switch msg.Type {
		case tea.KeyTab:
			o.searchScope = (o.searchScope + 1) % len(orphansSearchScopes)
			o.applyFilter()
			return o, nil, true
		case tea.KeyEsc:
			o.searchActive = false
			o.searchQuery = ""
			o.applyFilter()
			return o, nil, true
		case tea.KeyEnter:
			o.searchActive = false
			return o, nil, true
		case tea.KeyBackspace, tea.KeyDelete:
			if len(o.searchQuery) > 0 {
				runes := []rune(o.searchQuery)
				o.searchQuery = string(runes[:len(runes)-1])
				o.applyFilter()
			}
			return o, nil, true
		case tea.KeyCtrlW:
			o.searchQuery = ""
			o.applyFilter()
			return o, nil, true
		case tea.KeyRunes, tea.KeySpace:
			o.searchQuery += msg.String()
			o.applyFilter()
			return o, nil, true
		}
		return o, nil, true
	}
	return o, nil, false
}

// View implements Pane.
func (o *Orphans) View(w, h int) string {
	switch {
	case o.loading && len(o.allRows) == 0:
		return o.theme.Dim.Render("loading orphan patents…")
	case o.loadErr != "":
		return o.theme.Error.Render("error: " + o.loadErr)
	case len(o.allRows) == 0:
		return o.theme.Dim.Render("no orphan patents — every patent in the database belongs to a project")
	}
	o.page.SetPageSize(max(h-headerRows-2, 1))

	var b strings.Builder
	statusText := fmt.Sprintf("total:%d  sort:%s", o.total, o.activeSort)
	if o.sortAscending {
		statusText += " (asc)"
	} else {
		statusText += " (desc)"
	}
	b.WriteString(renderTableStatusLine(o.theme, w, o.page.Cursor(), o.page.Total(), statusText))
	b.WriteByte('\n')

	cols := o.currentCols(w)
	start, end := o.page.Window()

	tableStr := render.RenderTable(render.TableParams{
		Theme:         o.theme,
		Columns:       cols,
		RowCount:      end - start,
		FocusedColIdx: o.focusedColIdx,
		ActiveSort:    o.activeSort,
		SortAscending: o.sortAscending,
		FocusActive:   true,
		IsRowCursor: func(rowIdx int) bool {
			return start+rowIdx == o.page.Cursor()
		},
	}, w, func(rowIdx, colIdx int) string {
		absIdx := start + rowIdx
		if absIdx < 0 || absIdx >= len(o.rows) {
			return ""
		}
		row := o.rows[absIdx]
		switch cols[colIdx].Key {
		case "number":
			return row.Number.String()
		case "fetch":
			return string(row.FetchState)
		case "title":
			return truncTitle(row.Title)
		default:
			return ""
		}
	})
	b.WriteString(tableStr)

	b.WriteByte('\n')
	if o.searchActive {
		searchLine := "/ " + o.searchQuery + "▋  [Scope: " + orphansSearchScopes[o.searchScope].Label + " (Tab to cycle)]"
		b.WriteString(o.theme.Selected.Render(render.Pad(searchLine, w)))
	} else {
		b.WriteString(o.theme.Dim.Render(render.Pad("  [←/→] Focus Col  [.] Sort  [/] Search  [esc] back", w)))
	}
	return b.String()
}

func truncTitle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "—"
	}
	return s
}
