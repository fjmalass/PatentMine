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
	lastActive        domain.PatentNumber
	savedVisual       []domain.PatentNumber
	savedVisualAnchor int
	savedVisualCursor int
	gvHighlight       map[domain.PatentNumber]bool
	activeSort    domain.SortColumn
	sortAscending bool
	filter        PatentFilter
	find          findBar
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
		command.NavDown:         func(inv Invocation) tea.Cmd { return c.move(func() { c.page.MoveDown(inv.Repeat) }) },
		command.NavUp:           func(inv Invocation) tea.Cmd { return c.move(func() { c.page.MoveUp(inv.Repeat) }) },
		command.NavPageDown:     func(Invocation) tea.Cmd { return c.move(c.page.PageDown) },
		command.NavPageUp:       func(Invocation) tea.Cmd { return c.move(c.page.PageUp) },
		command.NavTop:          func(Invocation) tea.Cmd { return c.move(c.page.Top) },
		command.NavBottom:       func(Invocation) tea.Cmd { return c.move(c.page.Bottom) },
		command.ReselectLast:    func(Invocation) tea.Cmd { return c.reselectLast() },
		command.Refresh:         func(Invocation) tea.Cmd { c.loading = true; c.clearVisual(); return c.load() },
		command.SelectVisual:    func(Invocation) tea.Cmd { return c.toggleVisual() },
		command.SelectAll:       func(Invocation) tea.Cmd { return c.selectAllVisual() },
		command.SelectClear: func(Invocation) tea.Cmd {
			if c.visualMode {
				c.saveVisual()
				c.clearVisual()
			}
			return nil
		},
		command.CrawlFamily:    func(Invocation) tea.Cmd { return c.crawlSelected(domain.CrawlProfileFamily) },
		command.CrawlCitations: func(Invocation) tea.Cmd { return c.crawlSelected(domain.CrawlProfileCitations) },
		command.CrawlCitedBy:   func(Invocation) tea.Cmd { return c.crawlSelected(domain.CrawlProfileCitedBy) },
		command.CrawlAll:       func(Invocation) tea.Cmd { return c.crawlSelected(domain.CrawlProfileAll) },
		command.LookupPatent:   func(Invocation) tea.Cmd { return c.crawlSelected("") },
		command.ColNext:         func(Invocation) tea.Cmd { return c.focusNext() },
		command.ColPrev:         func(Invocation) tea.Cmd { return c.focusPrev() },
		command.SortApply:       func(Invocation) tea.Cmd { return c.applySort() },
		command.Filter:          c.applyFilter,
		command.FindOpen:        func(Invocation) tea.Cmd { c.find.open(c.filter.Search); return nil },
	}
	return c
}

// Context implements Pane.
func (c *Citations) Scope() command.Scope { return command.ScopeCitations }

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
				Number:         root,
				Kind:           kind,
				Project:        project,
				ReviewState:    c.filter.ReviewState,
				Search:         c.filter.Search,
				Classification: c.filter.Classification,
				Limit:          limit,
				Offset:         offset,
				SortColumn:     c.activeSort,
				SortAscending:  c.sortAscending,
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

func (c *Citations) applyFilter(inv Invocation) tea.Cmd {
	msg, err := c.filter.parse(inv.Args)
	if err != nil {
		return func() tea.Msg { return StatusMsg{Key: text.StatusUsage, Args: []any{err.Error()}, Error: true} }
	}
	c.loading = true
	c.page.Top()
	return tea.Batch(
		c.load(),
		func() tea.Msg { return StatusMsg{Key: text.StatusFilter, Args: []any{msg}} },
	)
}

// HandleKey implements pane.KeyHandler: intercepts raw keys while the find bar
// is active so typed characters go into the search input before chord resolution.
func (c *Citations) HandleKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool) {
	if !c.find.active {
		return c, nil, false
	}
	search, action := c.find.handleKey(msg)
	switch action {
	case "reload":
		c.filter.Search = search
		c.loading = true
		c.page.Top()
		return c, c.load(), true
	case "confirm":
		c.filter.Search = search
		return c, nil, true
	case "cancel":
		c.filter.Search = search
		c.loading = true
		c.page.Top()
		return c, c.load(), true
	default:
		return c, nil, true
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
func (c *Citations) Command(id command.ID, inv Invocation) (Pane, tea.Cmd) {
	if handler, ok := c.handlers[id]; ok {
		return c, handler(inv)
	}
	return c, nil
}

// Handles implements Pane.
func (c *Citations) Handles() []command.ID { return handlerIDs(c.handlers) }

// crawlSelected enqueues a crawl or lookup for the selected neighbour patent(s).
func (c *Citations) crawlSelected(profile domain.CrawlProfile) tea.Cmd {
	numbers := c.Selections()
	if len(numbers) < 2 {
		n, ok := c.Selection()
		if !ok {
			return status(text.StatusNoPatentSelected, true)
		}
		return CrawlCmd(c.client, n, crawlDepth(profile), profile, false)
	}
	return MultiCrawlCmd(c.client, numbers, crawlDepth(profile), profile, false)
}

// move runs a cursor motion and reloads the page when the visible window
// scrolled to a new offset.
func (c *Citations) move(motion func()) tea.Cmd {
	before := c.page.Offset()
	motion()
	if pn, ok := c.Selection(); ok {
		c.lastActive = pn
	}
	c.gvHighlight = nil
	if c.page.Offset() != before {
		c.loading = true
		return c.load()
	}
	return nil
}

// saveVisual captures the current visual selection so gv can restore it.
func (c *Citations) saveVisual() {
	c.savedVisual = c.Selections()
	c.savedVisualAnchor = c.visualAnchor
	c.savedVisualCursor = c.page.Cursor()
}

// SaveVisualSelection implements pane.VisualSelectionSaver.
func (c *Citations) SaveVisualSelection() {
	if c.visualMode {
		c.saveVisual()
		c.clearVisual()
	}
}

// reselectLast re-enters visual mode over the last saved selection by patent
// number (gv behaves like Vim's gv). Falls back to the last active patent if
// no visual selection was saved.
func (c *Citations) reselectLast() tea.Cmd {
	targets := c.savedVisual
	if len(targets) == 0 && !c.lastActive.IsZero() {
		targets = []domain.PatentNumber{c.lastActive}
	}
	if len(targets) == 0 {
		return status(text.StatusNoPatentSelected, false)
	}

	// Build index by patent number (sort-agnostic).
	idx := make(map[domain.PatentNumber]int, len(c.patents))
	for i, p := range c.patents {
		idx[p.Number] = c.loadedBase + i
	}

	highlights := make(map[domain.PatentNumber]bool, len(targets))
	first := -1
	last := -1
	for _, t := range targets {
		highlights[t] = true
		if pos, ok := idx[t]; ok {
			if first == -1 || pos < first {
				first = pos
			}
			if pos > last {
				last = pos
			}
		}
	}
	if first == -1 {
		return status(text.StatusNoPatentSelected, false)
	}

	c.clearVisual()
	c.visualMode = true
	c.visualAnchor = first
	c.gvHighlight = highlights
	c.page.ScrollTo(last)
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
		c.saveVisual()
		c.clearVisual()
		return nil
	}
	c.visualMode, c.visualAnchor = true, c.page.Cursor()
	return nil
}

func (c *Citations) selectAllVisual() tea.Cmd {
	if c.page.Total() == 0 {
		return nil
	}
	if c.visualMode {
		c.saveVisual()
	}
	c.visualMode = true
	c.visualAnchor = 0
	return c.move(c.page.Bottom)
}

func (c *Citations) clearVisual() {
	c.visualMode = false
	c.visualAnchor = 0
	c.gvHighlight = nil
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
	case c.loading && len(c.patents) == 0:
		return c.theme.Dim.Render("loading family edges…")
	case c.loadErr != "":
		return c.theme.Error.Render("error: " + c.loadErr)
	case c.page.Total() == 0:
		return c.theme.Dim.Render("no " + relationLabel(c.kind) + " edges recorded")
	}

	findRows := 0
	if c.find.active {
		findRows = 1
	}
	c.page.SetPageSize(max(h-headerRows-findRows, 1))
	var projectID domain.ProjectID
	if c.activeProject != nil {
		projectID = c.activeProject.ID
	}
	cols := c.currentCols()

	var b strings.Builder
	filterSummary := ""
	if c.filter.IsActive() && !c.find.active {
		filterSummary = "filters: " + strings.Join(c.filter.Labels(), " · ")
	}
	b.WriteString(renderTableStatusLine(c.theme, w, c.page.Cursor(), c.page.Total(), filterSummary))
	b.WriteByte('\n')
	b.WriteString(renderTableHeader(c.theme, cols, c.activeSort, c.sortAscending, c.focusedColIdx))

	for i, p := range c.patents {
		absolute := c.loadedBase + i
		line := renderStyledTableRow(c.theme, p, cols, projectID, absolute)
		rowStyle := tableRowStyle(c.theme, absolute)
		b.WriteByte('\n')
		switch {
		case c.gvHighlight != nil && c.gvHighlight[p.Number]:
			b.WriteString(c.theme.Visual.Render(render.Pad(line, w)))
		case c.visualMode && c.inVisualRange(absolute):
			b.WriteString(c.theme.Visual.Render(render.Pad(line, w)))
		case absolute == c.page.Cursor():
			b.WriteString(c.theme.Selected.Render(render.Pad(line, w)))
		default:
			b.WriteString(rowStyle(render.Pad(line, w)))
		}
	}
	if c.find.active {
		b.WriteByte('\n')
		b.WriteString(c.find.view(w, c.theme, c.page.Total()))
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
