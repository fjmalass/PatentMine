package pane

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

// column widths for the catalog table.
const (
	colNumber   = 16
	colInventor = 18
	colExpires  = 10
	colTags     = 14
	colState    = 13
)

// sortCols is the ordered set of sort columns, left-to-right.
var sortCols = []domain.SortColumn{
	domain.SortByNumber,
	domain.SortByTitle,
	domain.SortByInventor,
	domain.SortByExpires,
}

// catCol is one catalog column descriptor.
type catCol struct {
	label   string
	sortKey domain.SortColumn // zero = not sortable
	width   int
}

// catalogLoadedMsg delivers a finished patent.list result.
type catalogLoadedMsg struct {
	requestID uint64
	offset    int
	total     int
	patents   []domain.PatentRow
	err       error
}

// Catalog is the main patent list pane.
type Catalog struct {
	client   *rpc.Client
	theme    render.Theme
	handlers map[command.ID]cmdHandler

	activeProject *domain.Project

	patents       []domain.PatentRow
	page          render.Paginator
	loadedBase    int
	loading       bool
	loadErr       string
	loadID        uint64
	visualMode    bool
	visualAnchor  int
	sortColIdx    int
	sortAscending bool
}

// NewCatalog builds an empty catalog pane bound to a daemon client.
func NewCatalog(client *rpc.Client, theme render.Theme) *Catalog {
	c := &Catalog{
		client:        client,
		theme:         theme,
		page:          render.NewPaginator(10),
		loading:       true,
		sortAscending: true,
	}
	c.handlers = map[command.ID]cmdHandler{
		command.NavDown:      func(r int) tea.Cmd { return c.move(func() { c.page.MoveDown(r) }) },
		command.NavUp:        func(r int) tea.Cmd { return c.move(func() { c.page.MoveUp(r) }) },
		command.NavPageDown:  func(int) tea.Cmd { return c.move(c.page.PageDown) },
		command.NavPageUp:    func(int) tea.Cmd { return c.move(c.page.PageUp) },
		command.NavTop:       func(int) tea.Cmd { return c.move(c.page.Top) },
		command.NavBottom:    func(int) tea.Cmd { return c.move(c.page.Bottom) },
		command.Refresh:      func(int) tea.Cmd { c.loading = true; return c.load() },
		command.SelectVisual: func(int) tea.Cmd { return c.toggleVisual() },
		command.SelectClear:  func(int) tea.Cmd { c.clearVisual(); return nil },
		command.IngestFamily: func(int) tea.Cmd { return c.ingestSelected(ingestFamilyDepth) },
		command.FetchPatent:  func(int) tea.Cmd { return c.ingestSelected(ingestPatentDepth) },
		command.ColNext:      func(int) tea.Cmd { return c.sortNext() },
		command.ColPrev:      func(int) tea.Cmd { return c.sortPrev() },
	}
	return c
}

// Context implements Pane.
func (c *Catalog) Context() command.Context { return command.ContextCatalog }

// Title implements Pane.
func (c *Catalog) Title() string { return "Patents" }

// Init implements Pane.
func (c *Catalog) Init() tea.Cmd { return c.load() }

// load fetches the patent list from the daemon.
func (c *Catalog) load() tea.Cmd {
	client := c.client
	requestID := nextAsyncID()
	c.loadID = requestID
	offset := c.page.Offset()
	limit := c.page.PageSize()
	sortCol := sortCols[c.sortColIdx]
	sortAsc := c.sortAscending
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.PatentListResult
		var project domain.ProjectID
		if c.activeProject != nil {
			project = c.activeProject.ID
		}
		err := client.Call(ctx, proto.MethodPatentList,
			proto.PatentListParams{
				Project:       project,
				Limit:         limit,
				Offset:        offset,
				SortColumn:    sortCol,
				SortAscending: sortAsc,
			}, &res)
		return catalogLoadedMsg{
			requestID: requestID,
			offset:    offset,
			total:     res.Total,
			patents:   res.Patents,
			err:       err,
		}
	}
}

// sortNext moves the active sort column right and reloads.
func (c *Catalog) sortNext() tea.Cmd {
	if c.sortColIdx >= len(sortCols)-1 {
		return nil
	}
	c.sortColIdx++
	c.sortAscending = true
	c.loading = true
	return c.load()
}

// sortPrev moves the active sort column left and reloads.
func (c *Catalog) sortPrev() tea.Cmd {
	if c.sortColIdx <= 0 {
		return nil
	}
	c.sortColIdx--
	c.sortAscending = true
	c.loading = true
	return c.load()
}

// Command implements Pane.
func (c *Catalog) Command(id command.ID, repeat int) (Pane, tea.Cmd) {
	if handler, ok := c.handlers[id]; ok {
		return c, handler(repeat)
	}
	return c, nil
}

// Handles implements Pane.
func (c *Catalog) Handles() []command.ID { return handlerIDs(c.handlers) }

// ingestSelected enqueues an ingest for the highlighted patent.
func (c *Catalog) ingestSelected(depth int) tea.Cmd {
	number, ok := c.Selection()
	if !ok {
		return status(text.StatusNoPatentSelected, true)
	}
	return IngestCmd(c.client, number, depth, false)
}

// move runs a cursor motion and reloads the page when the visible window
// scrolled to a new offset.
func (c *Catalog) move(motion func()) tea.Cmd {
	before := c.page.Offset()
	motion()
	if c.page.Offset() != before {
		c.loading = true
		return c.load()
	}
	return nil
}

// Update implements Pane.
func (c *Catalog) Update(msg tea.Msg) (Pane, tea.Cmd) {
	switch m := msg.(type) {
	case ResizeMsg:
		pageSize := max(m.Height-1, 1)
		if pageSize != c.page.PageSize() {
			before := c.page.Offset()
			c.page.SetPageSize(pageSize)
			if before != c.page.Offset() || len(c.patents) != c.page.PageSize() {
				c.loading = true
				return c, c.load()
			}
		}
	case ProjectChangedMsg:
		changed := !sameProject(c.activeProject, m.Project)
		c.activeProject = cloneProject(m.Project)
		if changed {
			c.page.Top()
			c.loadedBase = 0
			c.loading = true
			c.clearVisual()
			return c, c.load()
		}
	case catalogLoadedMsg:
		if m.requestID != c.loadID {
			return c, nil
		}
		c.loading = false
		if m.err != nil {
			c.loadErr = m.err.Error()
			return c, nil
		}
		c.loadErr = ""
		c.patents = m.patents
		c.loadedBase = m.offset
		c.page.SetTotal(m.total)
		c.clearVisual()
		if c.page.Offset() != m.offset {
			c.loading = true
			return c, c.load()
		}
	}
	return c, nil
}

func (c *Catalog) toggleVisual() tea.Cmd {
	if c.visualMode {
		c.clearVisual()
		return nil
	}
	c.visualMode, c.visualAnchor = true, c.page.Cursor()
	return nil
}

func (c *Catalog) clearVisual() {
	c.visualMode = false
	c.visualAnchor = 0
}

func (c *Catalog) inVisualRange(absolute int) bool {
	lo := min(c.visualAnchor, c.page.Cursor())
	hi := max(c.visualAnchor, c.page.Cursor())
	return absolute >= lo && absolute <= hi
}

// Selections implements MultiSelector.
func (c *Catalog) Selections() []domain.PatentNumber {
	if !c.visualMode || len(c.patents) == 0 {
		return nil
	}
	lo := min(c.visualAnchor, c.page.Cursor())
	hi := max(c.visualAnchor, c.page.Cursor())
	lo = max(lo, c.loadedBase)
	hi = min(hi, c.loadedBase+len(c.patents)-1)
	if lo > hi {
		return nil
	}
	out := make([]domain.PatentNumber, 0, hi-lo+1)
	for abs := lo; abs <= hi; abs++ {
		out = append(out, c.patents[abs-c.loadedBase].Number)
	}
	return out
}

// Selection implements Pane.
func (c *Catalog) Selection() (domain.PatentNumber, bool) {
	cur := c.page.Cursor() - c.loadedBase
	if cur < 0 || cur >= len(c.patents) {
		return domain.PatentNumber{}, false
	}
	return c.patents[cur].Number, true
}

// catColumns returns the column layout, computing the title width from bodyWidth.
func (c *Catalog) catColumns(bodyWidth int) []catCol {
	// 5 one-space gaps between 6 columns
	fixedW := colNumber + colInventor + colExpires + colTags + colState + 5
	titleW := max(bodyWidth-fixedW, 12)
	return []catCol{
		{"NUMBER",         domain.SortByNumber,   colNumber},
		{"TITLE",          domain.SortByTitle,    titleW},
		{"INVENTOR",       domain.SortByInventor, colInventor},
		{"EXPIRES",        domain.SortByExpires,  colExpires},
		{"TAGS",           "",                    colTags},
		{c.stateHeading(), "",                    colState},
	}
}

// View implements Pane.
func (c *Catalog) View(w, h int) string {
	switch {
	case c.loading:
		return c.theme.Dim.Render("loading patents…")
	case c.loadErr != "":
		return c.theme.Error.Render("error: " + c.loadErr)
	case c.page.Total() == 0:
		if c.activeProject != nil {
			return c.theme.Dim.Render("no patents in active project " + c.activeProject.Name)
		}
		return c.theme.Dim.Render("no patents yet — select a number and press f to ingest its family")
	}

	c.page.SetPageSize(max(h-1, 1))
	cols := c.catColumns(w)
	activeSortKey := sortCols[c.sortColIdx]

	var b strings.Builder
	b.WriteString(c.renderCatHeader(cols, activeSortKey))

	for i, p := range c.patents {
		absolute := c.loadedBase + i
		line := c.renderCatRow(p, cols)
		b.WriteByte('\n')
		switch {
		case absolute == c.page.Cursor():
			b.WriteString(c.theme.Selected.Render(render.Pad(line, w)))
		case c.visualMode && c.inVisualRange(absolute):
			b.WriteString(c.theme.Visual.Render(render.Pad(line, w)))
		default:
			b.WriteString(c.theme.Row.Render(line))
		}
	}
	return b.String()
}

// renderCatHeader builds the column header row with sort indicators.
func (c *Catalog) renderCatHeader(cols []catCol, activeSortKey domain.SortColumn) string {
	var b strings.Builder
	for i, col := range cols {
		if i > 0 {
			b.WriteByte(' ')
		}
		label := col.label
		if col.sortKey == activeSortKey {
			if c.sortAscending {
				label += " ▴"
			} else {
				label += " ▾"
			}
			b.WriteString(c.theme.SortActive.Render(render.Pad(render.Truncate(label, col.width), col.width)))
		} else {
			b.WriteString(c.theme.Header.Render(render.Pad(render.Truncate(label, col.width), col.width)))
		}
	}
	return b.String()
}

// renderCatRow formats one patent row across all columns.
func (c *Catalog) renderCatRow(p domain.PatentRow, cols []catCol) string {
	vals := [6]string{
		numberToShowRow(p).String(),
		p.Title,
		formatInventorsShort(p.Inventors),
		formatExpires(p.ExpirationDate),
		formatTags(p.Tags),
		c.stateText(p),
	}
	var b strings.Builder
	for i, col := range cols {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(render.Pad(render.Truncate(vals[i], col.width), col.width))
	}
	return b.String()
}

func (c *Catalog) stateHeading() string {
	if c.activeProject != nil {
		return "PROJECT STATE"
	}
	return "FETCH"
}

func (c *Catalog) stateText(row domain.PatentRow) string {
	if c.activeProject != nil && row.MembershipState.Valid() {
		return string(row.MembershipState)
	}
	return string(row.FetchState)
}

func cloneProject(project *domain.Project) *domain.Project {
	if project == nil {
		return nil
	}
	copy := *project
	return &copy
}

func sameProject(a, b *domain.Project) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ID == b.ID && a.Name == b.Name && a.CreatedAt.Equal(b.CreatedAt)
}

func numberToShowRow(row domain.PatentRow) domain.PatentNumber {
	if !row.DisplayNumber.IsZero() {
		return row.DisplayNumber
	}
	return row.Number
}

// formatInventorsShort returns the first inventor's name, appending "et al."
// when there are multiple inventors.
func formatInventorsShort(inventors []string) string {
	if len(inventors) == 0 {
		return "-"
	}
	if len(inventors) == 1 {
		return inventors[0]
	}
	return inventors[0] + " et al."
}

// formatExpires formats an expiration date for display; returns "-" when zero.
func formatExpires(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02")
}

// formatTags formats a patent's tag list for display; returns "-" when empty.
func formatTags(tags []string) string {
	if len(tags) == 0 {
		return "-"
	}
	return strings.Join(tags, " ")
}
