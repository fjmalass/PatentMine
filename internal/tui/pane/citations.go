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

// citationsLoadedMsg delivers a finished patent.relations result.
type citationsLoadedMsg struct {
	requestID uint64
	offset    int
	total     int
	patents   []domain.PatentRow
	err       error
}

// Citations lists one slice of a patent's family graph — the edges of a single
// RelationKind (citations, cited-by, parents, or children) — so the same pane
// type serves every family view.
type Citations struct {
	client   *rpc.Client
	theme    render.Theme
	root     domain.PatentNumber
	kind     domain.RelationKind
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

// NewCitations builds a family-edge pane for one patent and relation kind.
func NewCitations(client *rpc.Client, theme render.Theme, root domain.PatentNumber, kind domain.RelationKind) *Citations {
	c := &Citations{
		client:        client,
		theme:         theme,
		root:          root,
		kind:          kind,
		page:          render.NewPaginator(defaultPageSize),
		loading:       true,
		activeSort:    domain.SortByNumber,
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
func (c *Citations) Context() command.Context { return command.ContextCitations }

// Title implements Pane.
func (c *Citations) Title() string {
	return relationLabel(c.kind) + " · " + c.root.String()
}

// Init implements Pane.
func (c *Citations) Init() tea.Cmd { return c.load() }

func (c *Citations) load() tea.Cmd {
	client, root, kind := c.client, c.root, c.kind
	requestID := nextAsyncID()
	c.loadID = requestID
	offset := c.page.Offset()
	limit := c.page.PageSize()
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.RelationsResult
		var project domain.ProjectID
		if c.activeProject != nil {
			project = c.activeProject.ID
		}
		err := client.Call(ctx, proto.MethodRelations,
			proto.RelationsParams{
				Number:        root,
				Kind:          kind,
				Project:       project,
				Limit:         limit,
				Offset:        offset,
				SortColumn:    c.activeSort,
				SortAscending: c.sortAscending,
			}, &res)
		return citationsLoadedMsg{
			requestID: requestID,
			offset:    offset,
			total:     res.Total,
			patents:   res.Patents,
			err:       err,
		}
	}
}

// focusNext moves the visual focus to the next column.
func (c *Citations) focusNext() tea.Cmd {
	cols := c.currentCols()
	if c.focusedColIdx < 0 {
		c.focusedColIdx = 0
	} else {
		c.focusedColIdx = (c.focusedColIdx + 1) % len(cols)
	}
	return nil
}

// focusPrev moves the visual focus to the previous column.
func (c *Citations) focusPrev() tea.Cmd {
	cols := c.currentCols()
	if c.focusedColIdx < 0 {
		c.focusedColIdx = len(cols) - 1
	} else {
		c.focusedColIdx = (c.focusedColIdx - 1 + len(cols)) % len(cols)
	}
	return nil
}

// applySort applies sorting to the currently focused column.
func (c *Citations) applySort() tea.Cmd {
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

func (c *Citations) currentCols() []tableCol {
	var projectID domain.ProjectID
	if c.activeProject != nil {
		projectID = c.activeProject.ID
	}
	return patentTableColumns(max(c.lastWidth, 80), projectID)
}

// Command implements Pane.
func (c *Citations) Command(id command.ID, repeat int) (Pane, tea.Cmd) {
	if handler, ok := c.handlers[id]; ok {
		return c, handler(repeat)
	}
	return c, nil
}

// Handles implements Pane.
func (c *Citations) Handles() []command.ID { return handlerIDs(c.handlers) }

// ingestSelected enqueues an ingest for the highlighted neighbour patent.
func (c *Citations) ingestSelected(profile domain.CrawlProfile) tea.Cmd {
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
func (c *Citations) move(motion func()) tea.Cmd {
	before := c.page.Offset()
	motion()
	if c.page.Offset() != before {
		c.loading = true
		return c.load()
	}
	return nil
}

// Update implements Pane.
func (c *Citations) Update(msg tea.Msg) (Pane, tea.Cmd) {
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
	case citationsLoadedMsg:
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

func (c *Citations) toggleVisual() tea.Cmd {
	if c.visualMode {
		c.clearVisual()
		return nil
	}
	c.visualMode, c.visualAnchor = true, c.page.Cursor()
	return nil
}

func (c *Citations) clearVisual() {
	c.visualMode = false
	c.visualAnchor = 0
}

func (c *Citations) inVisualRange(absolute int) bool {
	lo := min(c.visualAnchor, c.page.Cursor())
	hi := max(c.visualAnchor, c.page.Cursor())
	return absolute >= lo && absolute <= hi
}

// Selections implements MultiSelector.
func (c *Citations) Selections() []domain.PatentNumber {
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

// Selection implements Pane: the highlighted neighbour patent.
func (c *Citations) Selection() (domain.PatentNumber, bool) {
	cur := c.page.Cursor() - c.loadedBase
	if cur < 0 || cur >= len(c.patents) {
		return domain.PatentNumber{}, false
	}
	return c.patents[cur].Number, true
}

// View implements Pane.
func (c *Citations) View(w, h int) string {
	c.lastWidth = w
	switch {
	case c.loading:
		return c.theme.Dim.Render("loading family edges…")
	case c.loadErr != "":
		return c.theme.Error.Render("error: " + c.loadErr)
	case c.page.Total() == 0:
		return c.theme.Dim.Render("no " + relationLabel(c.kind) + " edges recorded")
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

// relationLabel is the human heading for a relation kind.
func relationLabel(k domain.RelationKind) string {
	switch k {
	case domain.RelationCites:
		return "Citations"
	case domain.RelationCitedBy:
		return "Cited by"
	case domain.RelationParent:
		return "Parents"
	case domain.RelationChild:
		return "Children"
	default:
		return "Relations"
	}
}
