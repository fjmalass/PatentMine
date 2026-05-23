package pane

import (
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

// catalogClassDescsMsg delivers a batch of cached classification descriptions.
type catalogClassDescsMsg struct {
	requestID uint64
	descs     map[string]string
	err       error
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
	columns       []domain.PatentTableColumn
	columnsProject domain.ProjectID

	patents       []domain.PatentRow
	page          render.Paginator
	loadedBase    int
	loading       bool
	loadErr       string
	loadID        uint64
	columnsLoadID uint64
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
	focusedColIdx  int
	lastWidth      int
	logger         *slog.Logger
	cachedCols     []tableCol
	cachedColWidth int
	cachedColProj  domain.ProjectID
	classDescs     map[string]string
	classDescsID   uint64
}

// WithLogger attaches a logger so the pane can persist RPC errors.
func (c *Catalog) WithLogger(l *slog.Logger) *Catalog { c.logger = l; return c }

func (c *Catalog) log() *slog.Logger {
	if c.logger != nil {
		return c.logger
	}
	return slog.Default()
}

// NewCatalog builds an empty catalog pane bound to a daemon client.
func NewCatalog(client *rpc.Client, theme render.Theme) *Catalog {
	c := &Catalog{
		client:        client,
		theme:         theme,
		columns:       domain.PatentTableColumns(""),
		columnsProject: "",
		page:          render.NewPaginator(defaultPageSize),
		loading:       true,
		activeSort:    domain.SortByReviewState,
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
func (c *Catalog) Scope() command.Scope { return command.ScopeCatalog }

// Title implements Pane.
func (c *Catalog) Title() string { return "Patents" }

// Init implements Pane.
func (c *Catalog) Init() tea.Cmd { return tea.Batch(c.loadColumns(), c.load()) }

func (c *Catalog) loadColumns() tea.Cmd {
	requestID := nextAsyncID()
	c.columnsLoadID = requestID
	var project domain.ProjectID
	if c.activeProject != nil {
		project = c.activeProject.ID
	}
	return LoadPatentTableColumnsCmd(c.client, project, requestID)
}

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
				Project:        project,
				ReviewState:    c.filter.ReviewState,
				Search:         c.filter.Search,
				Classification: c.filter.Classification,
				Inventor:       c.filter.Inventor,
				Limit:          limit,
				Offset:         offset,
				SortColumn:     c.activeSort,
				SortAscending:  c.sortAscending,
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

func (c *Catalog) applyFilter(inv Invocation) tea.Cmd {
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
func (c *Catalog) HandleKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool) {
	if !c.find.active {
		return c, nil, false
	}
	search, action := c.find.handleKey(msg)
	switch action {
	case "reload":
		c.applyFindInput(search)
		c.loading = true
		c.page.Top()
		return c, c.load(), true
	case "confirm":
		c.applyFindInput(search)
		return c, nil, true
	case "cancel":
		c.applyFindInput(search)
		c.loading = true
		c.page.Top()
		return c, c.load(), true
	default:
		return c, nil, true
	}
}

// applyFindInput routes the find-bar input into filter fields. Input prefixed
// with "class:" populates Classification (clearing Search); anything else
// populates Search (clearing Classification).
func (c *Catalog) applyFindInput(input string) {
	if cls, ok := parseClassFindInput(input); ok {
		c.filter.Classification = cls
		c.filter.Search = ""
		return
	}
	c.filter.Search = input
	c.filter.Classification = ""
}

// focusNext moves the visual focus to the next column.
func (c *Catalog) focusNext() tea.Cmd {
	c.focusedColIdx = moveSortableColumn(c.currentCols(), c.focusedColIdx, 1)
	return nil
}

// focusPrev moves the visual focus to the previous column.
func (c *Catalog) focusPrev() tea.Cmd {
	c.focusedColIdx = moveSortableColumn(c.currentCols(), c.focusedColIdx, -1)
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
	w := max(c.lastWidth, 80)
	if c.cachedCols != nil && c.cachedColWidth == w && c.cachedColProj == projectID {
		return c.cachedCols
	}
	schema := c.columns
	if len(schema) == 0 || c.columnsProject != projectID {
		schema = domain.PatentTableColumns(projectID)
	}
	c.cachedCols = patentTableColumns(w, schema)
	c.cachedColWidth = w
	c.cachedColProj = projectID
	return c.cachedCols
}

// Command implements Pane.
func (c *Catalog) Command(id command.ID, inv Invocation) (Pane, tea.Cmd) {
	if handler, ok := c.handlers[id]; ok {
		return c, handler(inv)
	}
	return c, nil
}

// Handles implements Pane.
func (c *Catalog) Handles() []command.ID { return handlerIDs(c.handlers) }

// crawlSelected enqueues a crawl or lookup for the selected patent(s).
func (c *Catalog) crawlSelected(profile domain.CrawlProfile) tea.Cmd {
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
func (c *Catalog) move(motion func()) tea.Cmd {
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
func (c *Catalog) saveVisual() {
	c.savedVisual = c.Selections()
	c.savedVisualAnchor = c.visualAnchor
	c.savedVisualCursor = c.page.Cursor()
}

// SaveVisualSelection implements pane.VisualSelectionSaver.
func (c *Catalog) SaveVisualSelection() {
	if c.visualMode {
		c.saveVisual()
		c.clearVisual()
	}
}

// reselectLast re-enters visual mode over the last saved selection by patent
// number (gv behaves like Vim's gv). Falls back to the last active patent if
// no visual selection was saved.
func (c *Catalog) reselectLast() tea.Cmd {
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
func (c *Catalog) Update(msg tea.Msg) (Pane, tea.Cmd) {
	switch m := msg.(type) {
	case ResizeMsg:
		pageSize := max(m.Height-headerRows, 1)
		c.focusedColIdx = clampFocusedSortableColumn(c.currentCols(), c.focusedColIdx)
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
			return c, tea.Batch(c.loadColumns(), c.load())
		}
	case patentTableColumnsLoadedMsg:
		if m.requestID != c.columnsLoadID {
			return c, nil
		}
		if m.err == nil && len(m.columns) > 0 {
			c.columns = m.columns
			c.columnsProject = m.project
			c.cachedCols = nil // schema changed; invalidate col cache
			c.focusedColIdx = clampFocusedSortableColumn(c.currentCols(), c.focusedColIdx)
		}
	case catalogLoadedMsg:
		if m.requestID != c.loadID {
			return c, nil
		}
		c.loading = false
		if m.err != nil {
			c.loadErr = m.err.Error()
			c.log().Error("patent list load failed", slog.String("error", m.err.Error()))
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
		return c, c.loadClassDescs()
	case catalogClassDescsMsg:
		if m.requestID == c.classDescsID && m.err == nil {
			c.classDescs = m.descs
		}
	}
	return c, nil
}

// loadClassDescs fetches cached classification descriptions for the codes
// referenced by the currently visible rows. Misses are silently skipped — the
// row renderer falls back to the bare code list.
func (c *Catalog) loadClassDescs() tea.Cmd {
	if len(c.patents) == 0 {
		c.classDescs = nil
		return nil
	}
	seen := make(map[string]struct{})
	codes := make([]string, 0, len(c.patents)*2)
	for _, p := range c.patents {
		for _, code := range p.Classifications {
			if _, dup := seen[code]; dup {
				continue
			}
			seen[code] = struct{}{}
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		c.classDescs = nil
		return nil
	}
	client := c.client
	requestID := nextAsyncID()
	c.classDescsID = requestID
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.ClassificationListResult
		if err := client.Call(ctx, proto.MethodClassificationListByCodes, proto.ClassificationListByCodesParams{Codes: codes}, &res); err != nil {
			return catalogClassDescsMsg{requestID: requestID, err: err}
		}
		descs := make(map[string]string, len(res.Classifications))
		for _, c := range res.Classifications {
			if c.Description != "" {
				descs[strings.ToLower(c.Code)] = c.Description
			}
		}
		return catalogClassDescsMsg{requestID: requestID, descs: descs}
	}
}

// ApplyClassificationFilter implements ClassificationFilterTarget. It sets the
// pane's classification filter to the given codes (space-separated; the
// existing list query parser already accepts that format) and reloads.
func (c *Catalog) ApplyClassificationFilter(codes []string) tea.Cmd {
	c.filter.Classification = strings.Join(codes, " ")
	c.filter.Search = ""
	c.loading = true
	c.page.Top()
	return c.load()
}

func (c *Catalog) toggleVisual() tea.Cmd {
	if c.visualMode {
		c.saveVisual()
		c.clearVisual()
		return nil
	}
	c.visualMode, c.visualAnchor = true, c.page.Cursor()
	return nil
}

func (c *Catalog) selectAllVisual() tea.Cmd {
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

func (c *Catalog) clearVisual() {
	c.visualMode = false
	c.visualAnchor = 0
	c.gvHighlight = nil
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
	case c.loading && len(c.patents) == 0:
		return c.theme.Dim.Render("loading patents…")
	case c.loadErr != "":
		return c.theme.Error.Render("error: " + c.loadErr)
	case c.page.Total() == 0:
		if c.activeProject != nil {
			return c.theme.Dim.Render("no patents in active project " + c.activeProject.Name)
		}
		return c.theme.Dim.Render("no patents yet — select a number and press f to crawl its family")
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
		isSelectedRow := absolute == c.page.Cursor()
		line := renderStyledTableRow(c.theme, p, cols, projectID, absolute, c.focusedColIdx, isSelectedRow, c.classDescs)
		rowStyle := tableRowStyle(c.theme, absolute)
		b.WriteByte('\n')
		switch {
		case c.gvHighlight != nil && c.gvHighlight[p.Number]:
			b.WriteString(c.theme.Visual.Render(render.Pad(line, w)))
		case c.visualMode && c.inVisualRange(absolute):
			b.WriteString(c.theme.Visual.Render(render.Pad(line, w)))
		case isSelectedRow:
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
