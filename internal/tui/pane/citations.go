package pane

import (
	"log/slog"
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

// citationsLoadedMsg delivers a finished patent.relations result.
type citationsLoadedMsg struct {
	requestID uint64
	offset    int
	total     int
	patents   []domain.PatentRow
	err       error
}

// citationsAllPagesLoadedMsg carries the full patent number list for select-all.
type citationsAllPagesLoadedMsg struct {
	requestID uint64
	numbers   []domain.PatentNumber
	err       error
}

// citationsClassDescsMsg delivers a batch of cached classification descriptions.
type citationsClassDescsMsg struct {
	requestID uint64
	descs     map[string]string
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

	activeProject  *domain.Project
	columns        []domain.PatentTableColumn
	columnsProject domain.ProjectID

	patents            []domain.PatentRow
	page               render.Paginator
	loadedBase         int
	loading            bool
	loadErr            string
	loadID             uint64
	columnsLoadID      uint64
	visualMode         bool
	visualAnchor       int
	allSelectedNumbers []domain.PatentNumber
	selectAllLoadID    uint64
	lastActive         domain.PatentNumber
	savedVisual        []domain.PatentNumber
	savedVisualAnchor  int
	savedVisualCursor  int
	gvHighlight        map[domain.PatentNumber]bool
	activeSort         domain.SortColumn
	sortAscending      bool
	filter             PatentFilter
	find               findBar
	focusedColIdx      int
	lastWidth          int
	logger             *slog.Logger
	classDescs         map[string]string
	classDescsID       uint64
	searchSeq          uint64
}

// WithLogger attaches a logger so the pane can persist RPC errors.
func (c *Citations) WithLogger(l *slog.Logger) *Citations { c.logger = l; return c }

func (c *Citations) log() *slog.Logger {
	if c.logger != nil {
		return c.logger
	}
	return slog.Default()
}

// NewCitations builds a family-edge pane for one patent and relation kind.
func NewCitations(client *rpc.Client, theme render.Theme, root domain.PatentNumber, kind domain.RelationKind) *Citations {
	c := &Citations{
		client:         client,
		theme:          theme,
		root:           root,
		kind:           kind,
		columns:        domain.PatentTableColumns(""),
		columnsProject: "",
		page:           render.NewPaginator(defaultPageSize),
		loading:        true,
		activeSort:     domain.SortByNumber,
		sortAscending:  true,
		focusedColIdx:  -1,
	}
	c.find.scopes = []SearchScope{
		{"all", "All Columns"},
		{"number", "Number"},
		{"title", "Title"},
		{"inventor", "Inventor"},
		{"class", "Class"},
		{"assignee", "Assignee"},
		{"tags", "Tags"},
	}
	c.handlers = map[command.ID]cmdHandler{
		command.NavDown:      func(inv Invocation) tea.Cmd { return c.move(func() { c.page.MoveDown(inv.Repeat) }) },
		command.NavUp:        func(inv Invocation) tea.Cmd { return c.move(func() { c.page.MoveUp(inv.Repeat) }) },
		command.NavPageDown:  func(Invocation) tea.Cmd { return c.move(c.page.PageDown) },
		command.NavPageUp:    func(Invocation) tea.Cmd { return c.move(c.page.PageUp) },
		command.NavTop:       func(inv Invocation) tea.Cmd { return c.move(func() { c.page.NavTop(inv.Repeat) }) },
		command.NavBottom:    func(inv Invocation) tea.Cmd { return c.move(func() { c.page.NavBottom(inv.Repeat) }) },
		command.ReselectLast: func(Invocation) tea.Cmd { return c.reselectLast() },
		command.Refresh:      func(Invocation) tea.Cmd { c.loading = true; c.clearVisual(); return c.load() },
		command.SelectVisual: func(Invocation) tea.Cmd { return c.toggleVisual() },
		command.SelectAll:    func(Invocation) tea.Cmd { return c.selectAllVisual() },
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
		command.LookupPatent:   func(Invocation) tea.Cmd { return c.crawlSelectedLookup() },
		command.ColNext:        func(Invocation) tea.Cmd { return c.focusNext() },
		command.ColPrev:        func(Invocation) tea.Cmd { return c.focusPrev() },
		command.SortApply:      func(Invocation) tea.Cmd { return c.applySort() },
		command.Filter:         c.applyFilter,
		command.FindOpen:       func(Invocation) tea.Cmd { c.find.open(c.filter.Search); return nil },
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
func (c *Citations) Init() tea.Cmd { return tea.Batch(c.loadColumns(), c.load()) }

func (c *Citations) loadColumns() tea.Cmd {
	requestID := nextAsyncID()
	c.columnsLoadID = requestID
	var project domain.ProjectID
	if c.activeProject != nil {
		project = c.activeProject.ID
	}
	return LoadPatentTableColumnsCmd(c.client, project, requestID)
}

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
				Filter:        c.filter.Expression,
				Search:        c.filter.Search,
				SearchScope:   c.find.activeScopeKey(),
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

func (c *Citations) applyFilter(inv Invocation) tea.Cmd {
	msg, err := c.filter.parse(inv.Args, c.activeProject != nil)
	if err != nil {
		return func() tea.Msg { return StatusMsg{Key: text.StatusUsage, Args: []any{err.Error()}, Error: true} }
	}
	c.loading = true
	c.page.Top()
	return tea.Batch(
		c.load(),
		func() tea.Msg { return StatusMsg{Key: text.StatusFilter, Args: []any{c.theme.Info.Render(msg)}} },
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
		c.applyFindInput(search)
		c.loading = true
		c.page.Top()
		c.searchSeq++
		seq := c.searchSeq
		return c, func() tea.Msg {
			time.Sleep(150 * time.Millisecond)
			return findDebounceMsg{seq: seq, query: search}
		}, true
	case "confirm":
		c.applyFindInput(search)
		return c, func() tea.Msg { return SearchAppliedMsg{Query: search} }, true
	case "cancel":
		c.applyFindInput(search)
		c.loading = true
		c.page.Top()
		c.searchSeq++ // invalidate any pending debounces
		return c, c.load(), true
	default:
		return c, nil, true
	}
}

// ApplyClassificationFilter implements ClassificationFilterTarget. It sets the
// pane's classification filter terms and reloads.
func (c *Citations) ApplyClassificationFilter(codes []string) tea.Cmd {
	if err := c.filter.replaceClassifications(codes); err != nil {
		return func() tea.Msg { return StatusMsg{Key: text.StatusUsage, Args: []any{err.Error()}, Error: true} }
	}
	c.filter.Search = ""
	c.loading = true
	c.page.Top()
	return c.load()
}

// applyFindInput routes the find-bar input into filter fields. Input prefixed
// with "class:" replaces the class terms in the boolean filter; anything else
// becomes plain search text.
func (c *Citations) applyFindInput(input string) {
	if cls, ok := parseClassFindInput(input); ok {
		if cls == "" || c.filter.replaceClassifications([]string{cls}) == nil {
			c.filter.Search = ""
			return
		}
	}
	c.filter.Search = input
	_ = c.filter.replaceClassifications(nil)
}

// focusNext moves the visual focus to the next column.
func (c *Citations) focusNext() tea.Cmd {
	c.focusedColIdx = moveSortableColumn(c.currentCols(), c.focusedColIdx, 1)
	return nil
}

// focusPrev moves the visual focus to the previous column.
func (c *Citations) focusPrev() tea.Cmd {
	c.focusedColIdx = moveSortableColumn(c.currentCols(), c.focusedColIdx, -1)
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
	schema := c.columns
	if len(schema) == 0 || c.columnsProject != projectID {
		schema = domain.PatentTableColumns(projectID)
	}
	return patentTableColumns(max(c.lastWidth, 80), schema)
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
	var projectID domain.ProjectID
	if c.activeProject != nil {
		projectID = c.activeProject.ID
	}
	numbers := c.Selections()
	if len(numbers) < 2 {
		n, ok := c.Selection()
		if !ok {
			return status(text.StatusNoPatentSelected, true)
		}
		return CrawlCmd(c.client, projectID, n, crawlDepth(profile), profile, false)
	}
	return MultiCrawlCmd(c.client, projectID, numbers, crawlDepth(profile), profile, false)
}

// crawlSelectedLookup is the explicit re-fetch ("L") path. It always forces
// a fresh web fetch. This is what makes "press L on a stub" actually pull
// data instead of repeating the same weak depth-0 path that :add used.
func (c *Citations) crawlSelectedLookup() tea.Cmd {
	var projectID domain.ProjectID
	if c.activeProject != nil {
		projectID = c.activeProject.ID
	}
	numbers := c.Selections()
	if len(numbers) < 2 {
		n, ok := c.Selection()
		if !ok {
			return status(text.StatusNoPatentSelected, true)
		}
		return CrawlCmd(c.client, projectID, n, lookupDepth, "", true)
	}
	return MultiCrawlCmd(c.client, projectID, numbers, lookupDepth, "", true)
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
	case findDebounceMsg:
		if m.seq == c.searchSeq {
			return c, c.load()
		}
		return c, nil
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
	case ReviewStateChangedMsg:
		if c.activeProject == nil || c.activeProject.ID != m.Project {
			return c, nil
		}
		applied := 0
		for i := range c.patents {
			matched := false
			for _, pat := range m.Patents {
				if patentRowMatchesNumber(c.patents[i], pat) {
					matched = true
					break
				}
			}
			if matched {
				c.patents[i].ReviewState = m.State
				applied++
			}
		}
		if len(m.Patents) > 0 && applied == 0 && !c.loading {
			c.loading = true
			return c, c.load()
		}
	case IDSEntryChangedMsg:
		if c.activeProject == nil || c.activeProject.ID != m.Project {
			return c, nil
		}
		updated := false
		for i := range c.patents {
			if patentRowMatchesNumber(c.patents[i], m.Patent) {
				c.patents[i].IDSEntry = m.Entry
				updated = true
				break
			}
		}
		if !updated && len(c.patents) > 0 {
			c.loading = true
			return c, c.load()
		}
	case IDSEntriesChangedMsg:
		if c.activeProject == nil {
			return c, nil
		}
		applied, relevant := applyIDSEntriesToPatentRows(c.patents, c.activeProject.ID, m.Entries)
		if relevant > applied && len(c.patents) > 0 {
			c.loading = true
			return c, c.load()
		}
	case patentTableColumnsLoadedMsg:
		if m.requestID != c.columnsLoadID {
			return c, nil
		}
		if m.err == nil && len(m.columns) > 0 {
			c.columns = m.columns
			c.columnsProject = m.project
			c.focusedColIdx = clampFocusedSortableColumn(c.currentCols(), c.focusedColIdx)
		}
	case citationsLoadedMsg:
		if m.requestID != c.loadID {
			return c, nil
		}
		c.loading = false
		if m.err != nil {
			c.loadErr = m.err.Error()
			c.log().Error("relations load failed", slog.String("root", c.root.String()), slog.String("kind", string(c.kind)), slog.String("error", m.err.Error()))
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
	case citationsAllPagesLoadedMsg:
		if m.requestID != c.selectAllLoadID {
			return c, nil
		}
		if m.err != nil {
			c.loadErr = m.err.Error()
			return c, nil
		}
		c.allSelectedNumbers = m.numbers
		return c, nil
	case citationsClassDescsMsg:
		if m.requestID == c.classDescsID && m.err == nil {
			c.classDescs = m.descs
		}
	}
	return c, nil
}

// loadClassDescs fetches cached classification descriptions for the codes
// referenced by the currently visible rows.
func (c *Citations) loadClassDescs() tea.Cmd {
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
			return citationsClassDescsMsg{requestID: requestID, err: err}
		}
		descs := make(map[string]string, len(res.Classifications))
		for _, cl := range res.Classifications {
			if cl.Description != "" {
				descs[strings.ToLower(cl.Code)] = cl.Description
			}
		}
		return citationsClassDescsMsg{requestID: requestID, descs: descs}
	}
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
	total := c.page.Total()
	if total == 0 {
		return nil
	}
	if c.visualMode {
		c.saveVisual()
	}
	c.visualMode = true
	c.visualAnchor = 0
	c.page.Bottom()
	return c.loadAllPages(total)
}

func (c *Citations) loadAllPages(total int) tea.Cmd {
	requestID := nextAsyncID()
	c.selectAllLoadID = requestID
	client, root, kind := c.client, c.root, c.kind
	var project domain.ProjectID
	if c.activeProject != nil {
		project = c.activeProject.ID
	}
	filter := c.filter.Expression
	search := c.filter.Search
	scope := c.find.activeScopeKey()
	sort := c.activeSort
	asc := c.sortAscending
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.RelationsResult
		err := client.Call(ctx, proto.MethodRelations,
			proto.RelationsParams{
				Number:        root,
				Kind:          kind,
				Project:       project,
				Filter:        filter,
				Search:        search,
				SearchScope:   scope,
				Limit:         total,
				Offset:        0,
				SortColumn:    sort,
				SortAscending: asc,
			}, &res)
		nums := make([]domain.PatentNumber, len(res.Patents))
		for i, p := range res.Patents {
			nums[i] = p.Number
		}
		return citationsAllPagesLoadedMsg{requestID: requestID, numbers: nums, err: err}
	}
}

func (c *Citations) clearVisual() {
	c.visualMode = false
	c.visualAnchor = 0
	c.allSelectedNumbers = nil
	c.gvHighlight = nil
}

func (c *Citations) inVisualRange(absolute int) bool {
	lo := min(c.visualAnchor, c.page.Cursor())
	hi := max(c.visualAnchor, c.page.Cursor())
	return absolute >= lo && absolute <= hi
}

// Selections implements MultiSelector.
func (c *Citations) Selections() []domain.PatentNumber {
	if !c.visualMode {
		return nil
	}
	if len(c.allSelectedNumbers) > 0 {
		return c.allSelectedNumbers
	}
	if len(c.patents) == 0 {
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

// ActivityFocus implements ActivityFocusProvider.
func (c *Citations) ActivityFocus() (ActivityFocus, bool) {
	cur := c.page.Cursor() - c.loadedBase
	if cur < 0 || cur >= len(c.patents) {
		return ActivityFocus{}, false
	}
	focus := patentRowActivity("citations", c.Title(), c.patents[cur], activeProjectID(c.activeProject), c.filter)
	focus.Attributes["root"] = c.root.String()
	focus.Attributes["relation"] = c.kind
	return focus, true
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
		if c.filter.IsActive() {
			var b strings.Builder
			filterHint := c.theme.Info.Render("filters: " + strings.Join(c.filter.Labels(), " · "))
			b.WriteString(renderTableStatusLine(c.theme, w, 0, 0, filterHint))
			b.WriteByte('\n')
			b.WriteString(c.theme.Warn.Render("no results for " + strings.Join(c.filter.Labels(), " · ") + " — :filter clear to see all"))
			return b.String()
		}
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
		filterSummary = c.theme.Info.Render("filters: " + strings.Join(c.filter.Labels(), " · "))
	}
	b.WriteString(renderTableStatusLine(c.theme, w, c.page.Cursor(), c.page.Total(), filterSummary))
	b.WriteByte('\n')
	b.WriteString(renderTableHeader(c.theme, cols, c.activeSort, c.sortAscending, c.focusedColIdx))
	for i, p := range c.patents {
		absolute := c.loadedBase + i
		isSelectedRow := absolute == c.page.Cursor()
		hl := HighlightNone
		if c.gvHighlight != nil && c.gvHighlight[p.Number] {
			hl = HighlightGotoVisual
		}
		line := renderStyledTableRow(c.theme, p, cols, projectID, absolute, c.focusedColIdx, isSelectedRow, hl, c.classDescs)
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

func (c *Citations) PatentNumber() domain.PatentNumber { return c.root }
func (c *Citations) Kind() domain.RelationKind         { return c.kind }
