package pane

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

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
	activeSort    domain.SortColumn
	sortAscending bool
	focusedColIdx int
	lastWidth     int
}

// NewCatalog builds an empty catalog pane bound to a daemon client.
func NewCatalog(client *rpc.Client, theme render.Theme) *Catalog {
	c := &Catalog{
		client:        client,
		theme:         theme,
		page:          render.NewPaginator(defaultPageSize),
		loading:       true,
		activeSort:    domain.SortByReviewState,
		sortAscending: true,
		focusedColIdx: -1,
	}
	c.handlers = map[command.ID]cmdHandler{
		command.NavDown:      func(r int) tea.Cmd { return c.move(func() { c.page.MoveDown(r) }) },
		command.NavUp:        func(r int) tea.Cmd { return c.move(func() { c.page.MoveUp(r) }) },
		command.NavPageDown:  func(int) tea.Cmd { return c.move(c.page.PageDown) },
		command.NavPageUp:    func(int) tea.Cmd { return c.move(c.page.PageUp) },
		command.NavTop:       func(int) tea.Cmd { return c.move(c.page.Top) },
		command.NavBottom:    func(int) tea.Cmd { return c.move(c.page.Bottom) },
		command.Refresh:      func(int) tea.Cmd { c.loading = true; c.clearVisual(); return c.load() },
		command.SelectVisual: func(int) tea.Cmd { return c.toggleVisual() },
		command.SelectClear:  func(int) tea.Cmd { c.clearVisual(); return nil },
		command.IngestFamily: func(int) tea.Cmd { return c.ingestSelected(domain.CrawlProfileFamily) },
		command.IngestCitations: func(int) tea.Cmd { return c.ingestSelected(domain.CrawlProfileCitations) },
		command.IngestCitedBy:   func(int) tea.Cmd { return c.ingestSelected(domain.CrawlProfileCitedBy) },
		command.IngestAll:       func(int) tea.Cmd { return c.ingestSelected(domain.CrawlProfileAll) },
		command.FetchPatent:  func(int) tea.Cmd { return c.ingestSelected("") },
		command.ColNext:      func(int) tea.Cmd { return c.focusNext() },
		command.ColPrev:      func(int) tea.Cmd { return c.focusPrev() },
		command.SortApply:    func(int) tea.Cmd { return c.applySort() },
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
				SortColumn:    c.activeSort,
				SortAscending: c.sortAscending,
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

// focusNext moves the visual focus to the next column.
func (c *Catalog) focusNext() tea.Cmd {
	cols := c.currentCols()
	if c.focusedColIdx < 0 {
		c.focusedColIdx = 0
	} else {
		c.focusedColIdx = (c.focusedColIdx + 1) % len(cols)
	}
	return nil
}

// focusPrev moves the visual focus to the previous column.
func (c *Catalog) focusPrev() tea.Cmd {
	cols := c.currentCols()
	if c.focusedColIdx < 0 {
		c.focusedColIdx = len(cols) - 1
	} else {
		c.focusedColIdx = (c.focusedColIdx - 1 + len(cols)) % len(cols)
	}
	return nil
}

// applySort applies sorting to the currently focused column.
func (c *Catalog) applySort() tea.Cmd {
	if c.focusedColIdx < 0 {
		return nil
	}
	cols := c.currentCols()
	col := cols[c.focusedColIdx]
	if col.sortKey == "" {
		return nil // column not sortable
	}

	if c.activeSort == col.sortKey {
		c.sortAscending = !c.sortAscending
	} else {
		c.activeSort = col.sortKey
		c.sortAscending = true
	}
	c.loading = true
	c.clearVisual()
	return c.load()
}

func (c *Catalog) currentCols() []tableCol {
	var projectID domain.ProjectID
	if c.activeProject != nil {
		projectID = c.activeProject.ID
	}
	return patentTableColumns(max(c.lastWidth, 80), projectID)
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
func (c *Catalog) ingestSelected(profile domain.CrawlProfile) tea.Cmd {
	number, ok := c.Selection()
	if !ok {
		return status(text.StatusNoPatentSelected, true)
	}
	depth := ingestFamilyDepth
	if profile == "" {
		depth = ingestPatentDepth
	}
	return IngestCmd(c.client, number, depth, profile, false)
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
		pageSize := max(m.Height-headerRows, 1)
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

// View implements Pane.
func (c *Catalog) View(w, h int) string {
	c.lastWidth = w
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

	c.page.SetPageSize(max(h-headerRows, 1))
	var projectID domain.ProjectID
	if c.activeProject != nil {
		projectID = c.activeProject.ID
	}
	cols := c.currentCols()

	var b strings.Builder
	b.WriteString(renderTableHeader(c.theme, cols, c.activeSort, c.sortAscending, c.focusedColIdx))

	for i, p := range c.patents {
		absolute := c.loadedBase + i
		line := renderTableRow(p, cols, projectID)
		b.WriteByte('\n')
		switch {
		case c.visualMode && c.inVisualRange(absolute):
			b.WriteString(c.theme.Visual.Render(render.Pad(line, w)))
		case absolute == c.page.Cursor():
			b.WriteString(c.theme.Selected.Render(render.Pad(line, w)))
		default:
			b.WriteString(c.theme.Row.Render(line))
		}
	}
	return b.String()
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
