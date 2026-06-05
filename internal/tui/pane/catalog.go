package pane

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/filterexpr"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
	"patentmine/internal/tui/tableview"
)

// catalogClassDescsMsg delivers a batch of cached classification descriptions.
type catalogClassDescsMsg struct {
	requestID uint64
	descs     map[string]string
	err       error
}

type findDebounceMsg struct {
	seq   uint64
	query string
}

// catalogTableViewMsg delivers a saved table view for the patents catalog.
type catalogTableViewMsg struct {
	filter string
}

// catalogLoadedMsg delivers a finished patent.list result.
type catalogLoadedMsg struct {
	requestID uint64
	offset    int
	total     int
	patents   []domain.PatentRow
	err       error
}

// catalogAllPagesLoadedMsg carries the full list of patent numbers fetched by
// select-all so Selections() can span all pages, not just the loaded window.
type catalogAllPagesLoadedMsg struct {
	requestID uint64
	numbers   []domain.PatentNumber
	err       error
}

// anchorIndexLoadedMsg carries the absolute index of a target patent within
// the current filtered list. Used to scroll back to the anchor row when the
// user expands out of a collapsed relation view and the anchor is not on the
// first page.
type anchorIndexLoadedMsg struct {
	requestID uint64
	anchor    domain.PatentNumber
	index     int
	err       error
}

// HighlightState tracks the active relation highlighting.
type HighlightState struct {
	Group             HighlightGroup
	Anchor            domain.PatentNumber
	LoadID            uint64
	Loading           bool
	FilterToRelations bool
}

// relationHighlightLoadedMsg delivers the neighbours of the anchor patent for
// the selected highlight group.
type relationHighlightLoadedMsg struct {
	requestID    uint64
	group        HighlightGroup
	anchor       domain.PatentNumber
	forward      []domain.PatentNumber // parents or citations
	reverse      []domain.PatentNumber // children or citedby
	dispatchedAt time.Time
	forwardRPC   time.Duration
	reverseRPC   time.Duration
	rpcTotal     time.Duration
	err          error
}

type relationCacheEntry struct {
	group      HighlightGroup
	forward    []domain.PatentNumber
	reverse    []domain.PatentNumber
	lastAccess time.Time
}

// Catalog is the main patent list pane.
type Catalog struct {
	client   *rpc.Client
	theme    render.Theme
	handlers map[command.ID]cmdHandler

	activeProject  *domain.Project
	columns        []domain.PatentTableColumn
	columnsProject domain.ProjectID

	patents             []domain.PatentRow
	page                render.Paginator
	loadedBase          int
	loading             bool
	loadErr             string
	loadID              uint64
	columnsLoadID       uint64
	visualMode          bool
	visualAnchor        int
	allSelectedNumbers  []domain.PatentNumber
	selectAllLoadID     uint64
	pendingScrollAnchor domain.PatentNumber
	findAnchorLoadID    uint64
	lastActive          domain.PatentNumber
	savedVisual         []domain.PatentNumber
	savedVisualAnchor   int
	savedVisualCursor   int
	highlights          HighlightSet
	activeHighlight     HighlightState
	relationCache       map[domain.PatentNumber]relationCacheEntry
	activeSort          domain.SortColumn
	sortAscending       bool
	filter              PatentFilter
	find                findBar
	focusedColIdx       int
	lastWidth           int
	logger              *slog.Logger
	metrics             *observability.Metrics
	cachedCols          []tableCol
	cachedColWidth      int
	cachedColProj       domain.ProjectID
	classDescs          map[string]string
	classDescsID        uint64
	searchSeq           uint64
}

// WithLogger attaches a logger so the pane can persist RPC errors.
func (c *Catalog) WithLogger(l *slog.Logger) *Catalog { c.logger = l; return c }

// WithMetrics attaches the runtime metrics sink. Optional: nil is safe.
func (c *Catalog) WithMetrics(m *observability.Metrics) *Catalog { c.metrics = m; return c }

// SetPendingScrollAnchor registers a patent to scroll to and cursor-highlight on the next load.
func (c *Catalog) SetPendingScrollAnchor(number domain.PatentNumber) {
	c.pendingScrollAnchor = number
}

// PendingScrollAnchor returns the current pending scroll target, if any.
func (c *Catalog) PendingScrollAnchor() domain.PatentNumber {
	return c.pendingScrollAnchor
}

func (c *Catalog) log() *slog.Logger {
	if c.logger != nil {
		return c.logger
	}
	return slog.Default()
}

// NewCatalog builds an empty catalog pane bound to a daemon client.
func NewCatalog(client *rpc.Client, theme render.Theme) *Catalog {
	c := &Catalog{
		client:         client,
		theme:          theme,
		columns:        domain.PatentTableColumns(""),
		columnsProject: "",
		page:           render.NewPaginator(defaultPageSize),
		loading:        true,
		activeSort:     domain.SortByReviewState,
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
	if expr, err := filterexpr.Parse("fetch_state:cached"); err == nil {
		c.filter.setExpression(expr)
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
			if c.highlights.HasRelation() {
				c.clearRelationHighlight()
			}
			return nil
		},
		command.HighlightFamily:        func(Invocation) tea.Cmd { return c.toggleFamilyHighlight() },
		command.HighlightCitations:     func(Invocation) tea.Cmd { return c.toggleCitationsHighlight() },
		command.RelationFilterCollapse: func(Invocation) tea.Cmd { return c.setRelationFilter(true) },
		command.RelationFilterExpand:   func(Invocation) tea.Cmd { return c.setRelationFilter(false) },
		command.CrawlFamily:            func(Invocation) tea.Cmd { return c.crawlSelected(domain.CrawlProfileFamily) },
		command.CrawlCitations:         func(Invocation) tea.Cmd { return c.crawlSelected(domain.CrawlProfileCitations) },
		command.CrawlCitedBy:           func(Invocation) tea.Cmd { return c.crawlSelected(domain.CrawlProfileCitedBy) },
		command.CrawlAll:               func(Invocation) tea.Cmd { return c.crawlSelected(domain.CrawlProfileAll) },
		command.LookupPatent:           func(Invocation) tea.Cmd { return c.crawlSelectedLookup() },
		command.ColNext:                func(Invocation) tea.Cmd { return c.focusNext() },
		command.ColPrev:                func(Invocation) tea.Cmd { return c.focusPrev() },
		command.SortApply:              func(Invocation) tea.Cmd { return c.applySort() },
		command.Filter:                 c.applyFilter,
		command.FindOpen:               func(Invocation) tea.Cmd { c.find.open(c.filter.Search); return nil },
	}
	return c
}

// Context implements Pane.
func (c *Catalog) Scope() command.Scope { return command.ScopeCatalog }

// Title implements Pane.
func (c *Catalog) Title() string { return "Patents" }

// Init implements Pane.
func (c *Catalog) Init() tea.Cmd {
	return tea.Batch(c.loadColumns(), loadDefaultPatentsTableViewCmd(c.client), c.load())
}

func loadDefaultPatentsTableViewCmd(client *rpc.Client) tea.Cmd {
	return func() tea.Msg {
		view, ok, err := tableview.LoadDefaultPatents(context.Background(), client)
		if err != nil || !ok {
			return catalogTableViewMsg{}
		}
		return catalogTableViewMsg{filter: tableview.FilterFromView(view)}
	}
}

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
		var numbers []domain.PatentNumber
		if c.activeHighlight.FilterToRelations && !c.activeHighlight.Anchor.IsZero() {
			numbers = c.highlights.RelationNumbers()
		}
		err := client.Call(ctx, proto.MethodPatentList,
			proto.PatentListParams{
				Numbers:       numbers,
				Project:       project,
				Filter:        c.filter.Expression,
				Search:        c.filter.Search,
				SearchScope:   c.find.activeScopeKey(),
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

func (c *Catalog) applyFilter(inv Invocation) tea.Cmd {
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
		c.searchSeq++
		seq := c.searchSeq
		return c, func() tea.Msg {
			// Wait 150ms before launching the load
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

// applyFindInput routes the find-bar input into filter fields. Input prefixed
// with "class:" replaces the class terms in the boolean filter; anything else
// becomes plain search text.
func (c *Catalog) applyFindInput(input string) {
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
// a fresh web fetch (bypassing any local cache or existing stub row). This is
// the only way to turn a near-empty stub left by :add into real data.
func (c *Catalog) crawlSelectedLookup() tea.Cmd {
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
func (c *Catalog) move(motion func()) tea.Cmd {
	before := c.page.Offset()
	motion()
	if pn, ok := c.Selection(); ok {
		c.lastActive = pn
	}
	c.clearGotoVisualHighlight()
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

	first := -1
	last := -1
	c.clearVisual()
	for _, t := range targets {
		c.highlights.Set(t, HighlightGotoVisual)
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
		c.clearGotoVisualHighlight()
		return status(text.StatusNoPatentSelected, false)
	}

	c.visualMode = true
	c.visualAnchor = first
	c.page.ScrollTo(last)
	return nil
}

// Update implements Pane.
func (c *Catalog) Update(msg tea.Msg) (Pane, tea.Cmd) {
	switch m := msg.(type) {
	case catalogTableViewMsg:
		if m.filter != "" {
			if expr, err := filterexpr.Parse(m.filter); err == nil {
				// The built-in default (fetch_state:cached) is the baseline:
				// keep un-fetched stubs hidden at startup unless the saved view
				// explicitly chose a fetch_state. Without this, a saved default
				// view silently drops the cached default.
				if cached, cerr := filterexpr.Parse("fetch_state:cached"); cerr == nil &&
					filterexpr.CanonicalString(filterexpr.RemoveField(expr, filterexpr.FieldFetchState)) == filterexpr.CanonicalString(expr) {
					expr = filterexpr.JoinAnd(expr, cached)
				}
				c.filter.setExpression(expr)
				c.loading = true
				c.page.Top()
				return c, c.load()
			}
		}
		return c, nil
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
		c.log().Info("tui.catalog.ids_entries_applied",
			slog.Int("entries", len(m.Entries)),
			slog.Int("matched_rows", applied),
			slog.String("project_id", string(c.activeProject.ID)))
		if c.metrics != nil {
			c.metrics.IncCounter("tui.catalog.ids_entries_applied", int64(applied))
		}
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
		if !c.pendingScrollAnchor.IsZero() {
			anchor := c.pendingScrollAnchor
			for i, p := range c.patents {
				if p.Number == anchor {
					c.pendingScrollAnchor = domain.PatentNumber{}
					c.page.ScrollTo(m.offset + i)
					c.log().Info("tui.relation_filter.scroll_to_anchor",
						slog.String("anchor", anchor.Normalized()),
						slog.Int("index", m.offset+i),
						slog.String("source", "page"))
					c.metrics.IncCounter("tui.relation_filter.expand_scroll.page", 1)
					if c.page.Offset() != m.offset {
						c.loading = true
						return c, tea.Batch(c.load(), c.loadClassDescs())
					}
					return c, c.loadClassDescs()
				}
			}
			return c, tea.Batch(c.findAnchorIndex(anchor, m.total), c.loadClassDescs())
		}
		return c, c.loadClassDescs()
	case catalogAllPagesLoadedMsg:
		if m.requestID != c.selectAllLoadID {
			return c, nil
		}
		if m.err != nil {
			c.loadErr = m.err.Error()
			return c, nil
		}
		c.allSelectedNumbers = m.numbers
		return c, nil
	case anchorIndexLoadedMsg:
		if m.requestID != c.findAnchorLoadID {
			return c, nil
		}
		c.pendingScrollAnchor = domain.PatentNumber{}
		if m.err != nil {
			c.log().Error("tui.relation_filter.anchor_index_load",
				slog.String("anchor", m.anchor.Normalized()),
				slog.String("error", m.err.Error()))
			c.metrics.IncCounter("tui.relation_filter.expand_scroll.error", 1)
			return c, nil
		}
		if m.index < 0 {
			c.metrics.IncCounter("tui.relation_filter.expand_scroll.missing", 1)
			return c, nil
		}
		c.page.ScrollTo(m.index)
		c.log().Info("tui.relation_filter.scroll_to_anchor",
			slog.String("anchor", m.anchor.Normalized()),
			slog.Int("index", m.index),
			slog.String("source", "lookup"))
		c.metrics.IncCounter("tui.relation_filter.expand_scroll.lookup", 1)
		c.loading = true
		return c, c.load()
	case catalogClassDescsMsg:
		if m.requestID == c.classDescsID && m.err == nil {
			c.classDescs = m.descs
		}
	case relationHighlightLoadedMsg:
		// Drop stale results: a different anchor or a newer request supersedes.
		if m.requestID != c.activeHighlight.LoadID || m.anchor != c.activeHighlight.Anchor {
			c.metrics.IncCounter(fmt.Sprintf("tui.highlight.%s.stale", m.group.Key()), 1)
			return c, nil
		}
		c.activeHighlight.Loading = false
		c.metrics.ObserveDuration(fmt.Sprintf("tui.highlight.%s.fetch.forward", m.group.Key()), m.forwardRPC, m.err != nil)
		c.metrics.ObserveDuration(fmt.Sprintf("tui.highlight.%s.fetch.reverse", m.group.Key()), m.reverseRPC, m.err != nil)
		c.metrics.ObserveDuration(fmt.Sprintf("tui.highlight.%s.fetch", m.group.Key()), m.rpcTotal, m.err != nil)
		if m.err != nil {
			c.clearRelationHighlight()
			c.log().Error("tui.highlight.load",
				slog.String("group", m.group.String()),
				slog.String("anchor", m.anchor.Normalized()),
				slog.String("error", m.err.Error()))
			c.metrics.IncCounter(fmt.Sprintf("tui.highlight.%s.error", m.group.Key()), 1)
			return c, status(text.StatusUsage, true, m.err.Error())
		}

		// Save to cache (limit size to 50 entries)
		if c.relationCache == nil {
			c.relationCache = make(map[domain.PatentNumber]relationCacheEntry)
		}
		if len(c.relationCache) >= 50 {
			// Find oldest entry to evict
			var oldestKey domain.PatentNumber
			var oldestTime time.Time
			first := true
			for k, v := range c.relationCache {
				if first || v.lastAccess.Before(oldestTime) {
					oldestKey = k
					oldestTime = v.lastAccess
					first = false
				}
			}
			delete(c.relationCache, oldestKey)
		}
		c.relationCache[m.anchor] = relationCacheEntry{
			group:      m.group,
			forward:    m.forward,
			reverse:    m.reverse,
			lastAccess: time.Now(),
		}

		applyStart := time.Now()
		var fKind, rKind HighlightKind
		if m.group == HighlightGroupFamily {
			fKind = HighlightFamilyParent
			rKind = HighlightFamilyChild
		} else {
			fKind = HighlightCitationCites
			rKind = HighlightCitationCitedBy
		}

		for _, p := range m.forward {
			c.highlights.Upgrade(p, fKind)
		}
		for _, p := range m.reverse {
			c.highlights.Upgrade(p, rKind)
		}
		if m.group == HighlightGroupFamily {
			c.highlights.Upgrade(m.anchor, HighlightFamilyAnchor)
		} else {
			c.highlights.Upgrade(m.anchor, HighlightCitationAnchor)
		}
		applyDuration := time.Since(applyStart)
		totalDuration := time.Since(m.dispatchedAt)
		c.metrics.ObserveDuration(fmt.Sprintf("tui.highlight.%s.apply", m.group.Key()), applyDuration, false)
		c.metrics.ObserveDuration(fmt.Sprintf("tui.highlight.%s.total", m.group.Key()), totalDuration, false)
		c.metrics.SetGauge(fmt.Sprintf("tui.highlight.%s.last.forward", m.group.Key()), int64(len(m.forward)))
		c.metrics.SetGauge(fmt.Sprintf("tui.highlight.%s.last.reverse", m.group.Key()), int64(len(m.reverse)))
		c.log().Info("tui.highlight.toggle",
			slog.String("group", m.group.String()),
			slog.String("anchor", m.anchor.Normalized()),
			slog.Int("forward", len(m.forward)),
			slog.Int("reverse", len(m.reverse)),
			slog.Duration("rpc_forward", m.forwardRPC),
			slog.Duration("rpc_reverse", m.reverseRPC),
			slog.Duration("rpc_total", m.rpcTotal),
			slog.Duration("apply", applyDuration),
			slog.Duration("total", totalDuration),
			slog.String("action", "on"))
		c.metrics.IncCounter(fmt.Sprintf("tui.highlight.%s.on", m.group.Key()), 1)
		if c.activeHighlight.FilterToRelations {
			c.page.Top()
			c.loading = true
			return c, c.load()
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
// pane's classification filter terms and reloads.
func (c *Catalog) ApplyClassificationFilter(codes []string) tea.Cmd {
	if err := c.filter.replaceClassifications(codes); err != nil {
		return func() tea.Msg { return StatusMsg{Key: text.StatusUsage, Args: []any{err.Error()}, Error: true} }
	}
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

func (c *Catalog) loadAllPages(total int) tea.Cmd {
	requestID := nextAsyncID()
	c.selectAllLoadID = requestID
	client := c.client
	var project domain.ProjectID
	if c.activeProject != nil {
		project = c.activeProject.ID
	}
	var numbers []domain.PatentNumber
	if c.activeHighlight.FilterToRelations && !c.activeHighlight.Anchor.IsZero() {
		numbers = c.highlights.RelationNumbers()
	}
	filter := c.filter.Expression
	search := c.filter.Search
	scope := c.find.activeScopeKey()
	sort := c.activeSort
	asc := c.sortAscending
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.PatentListResult
		err := client.Call(ctx, proto.MethodPatentList,
			proto.PatentListParams{
				Numbers:       numbers,
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
		return catalogAllPagesLoadedMsg{requestID: requestID, numbers: nums, err: err}
	}
}

// findAnchorIndex fetches the full filtered list to locate the absolute index
// of anchor when it falls outside the currently loaded page.
func (c *Catalog) findAnchorIndex(anchor domain.PatentNumber, total int) tea.Cmd {
	requestID := nextAsyncID()
	c.findAnchorLoadID = requestID
	client := c.client
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
		var res proto.PatentListResult
		err := client.Call(ctx, proto.MethodPatentList,
			proto.PatentListParams{
				Project:       project,
				Filter:        filter,
				Search:        search,
				SearchScope:   scope,
				Limit:         total,
				Offset:        0,
				SortColumn:    sort,
				SortAscending: asc,
			}, &res)
		idx := -1
		for i, p := range res.Patents {
			if p.Number == anchor {
				idx = i
				break
			}
		}
		return anchorIndexLoadedMsg{requestID: requestID, anchor: anchor, index: idx, err: err}
	}
}

func (c *Catalog) clearVisual() {
	c.visualMode = false
	c.visualAnchor = 0
	c.allSelectedNumbers = nil
	c.clearGotoVisualHighlight()
}

// clearGotoVisualHighlight drops only the gv-restore overlay; the family
// highlight (a different HighlightKind in the same set) is preserved so
// toggling visual mode does not also clear the family hint.
func (c *Catalog) clearGotoVisualHighlight() {
	if c.highlights.Len() == 0 {
		return
	}
	for _, p := range c.patents {
		if c.highlights.Kind(p.Number) == HighlightGotoVisual {
			c.highlights.Set(p.Number, HighlightNone)
		}
	}
}

// clearRelationHighlight drops the relation overlay and forgets the anchor.
func (c *Catalog) clearRelationHighlight() {
	c.highlights.ClearRelation()
	c.activeHighlight = HighlightState{}
}

// toggleFamilyHighlight triggers the Family highlight group (g h).
func (c *Catalog) toggleFamilyHighlight() tea.Cmd {
	return c.toggleHighlight(HighlightGroupFamily, domain.RelationParent, domain.RelationChild)
}

// toggleCitationsHighlight triggers the Citation highlight group (g c).
func (c *Catalog) toggleCitationsHighlight() tea.Cmd {
	return c.toggleHighlight(HighlightGroupCitations, domain.RelationCites, domain.RelationCitedBy)
}

// setRelationFilter collapses or expands the view to highlighted relations.
// When expanding (active=false), the cursor is steered back to the anchor row
// after the load completes so the user lands on the patent they started from.
func (c *Catalog) setRelationFilter(active bool) tea.Cmd {
	if c.activeHighlight.Anchor.IsZero() {
		return status(text.StatusUsage, true, "No relationship highlight active to filter")
	}
	if c.activeHighlight.FilterToRelations == active {
		return nil
	}
	c.activeHighlight.FilterToRelations = active
	c.page.Top()
	if !active {
		c.pendingScrollAnchor = c.activeHighlight.Anchor
	}
	c.loading = true
	return c.load()
}

// toggleHighlight is the generic key handler for relation highlights.
func (c *Catalog) toggleHighlight(group HighlightGroup, forwardKind, reverseKind domain.RelationKind) tea.Cmd {
	sel, ok := c.Selection()
	if !ok {
		return status(text.StatusNoPatentSelected, true)
	}

	// Toggle OFF if already active for this anchor
	if c.activeHighlight.Group == group && c.activeHighlight.Anchor == sel {
		wasFiltered := c.activeHighlight.FilterToRelations
		c.clearRelationHighlight()
		c.log().Info("tui.highlight.toggle",
			slog.String("group", group.String()),
			slog.String("anchor", sel.Normalized()),
			slog.String("action", "off"))
		c.metrics.IncCounter(fmt.Sprintf("tui.highlight.%s.off", group.Key()), 1)
		if wasFiltered {
			c.page.Top()
			c.loading = true
			return c.load()
		}
		return nil
	}

	c.clearRelationHighlight()

	// Check bounded cache first
	if c.relationCache != nil {
		if cached, found := c.relationCache[sel]; found && cached.group == group {
			// Update last access time
			cached.lastAccess = time.Now()
			c.relationCache[sel] = cached

			c.activeHighlight = HighlightState{
				Group:   group,
				Anchor:  sel,
				Loading: false,
			}

			// Apply instantly
			var fKind, rKind HighlightKind
			if group == HighlightGroupFamily {
				fKind = HighlightFamilyParent
				rKind = HighlightFamilyChild
			} else {
				fKind = HighlightCitationCites
				rKind = HighlightCitationCitedBy
			}

			for _, p := range cached.forward {
				c.highlights.Upgrade(p, fKind)
			}
			for _, p := range cached.reverse {
				c.highlights.Upgrade(p, rKind)
			}
			if group == HighlightGroupFamily {
				c.highlights.Upgrade(sel, HighlightFamilyAnchor)
			} else {
				c.highlights.Upgrade(sel, HighlightCitationAnchor)
			}
			c.metrics.IncCounter(fmt.Sprintf("tui.highlight.%s.cache_hit", group.Key()), 1)
			c.metrics.IncCounter(fmt.Sprintf("tui.highlight.%s.on", group.Key()), 1)
			return nil
		}
	}

	requestID := nextAsyncID()
	c.activeHighlight = HighlightState{
		Group:   group,
		Anchor:  sel,
		Loading: true,
		LoadID:  requestID,
	}

	client := c.client
	dispatchedAt := time.Now()

	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()

		var forward proto.RelationsResult
		var reverse proto.RelationsResult

		startAll := time.Now()

		tF := time.Now()
		errF := client.Call(ctx, proto.MethodRelations,
			proto.RelationsParams{Number: sel, Kind: forwardKind}, &forward)
		forwardRPC := time.Since(tF)

		tR := time.Now()
		errR := client.Call(ctx, proto.MethodRelations,
			proto.RelationsParams{Number: sel, Kind: reverseKind}, &reverse)
		reverseRPC := time.Since(tR)

		err := errF
		if err == nil {
			err = errR
		}

		fList := make([]domain.PatentNumber, 0, len(forward.Patents))
		for _, p := range forward.Patents {
			fList = append(fList, p.Number)
		}

		rList := make([]domain.PatentNumber, 0, len(reverse.Patents))
		for _, p := range reverse.Patents {
			rList = append(rList, p.Number)
		}

		return relationHighlightLoadedMsg{
			requestID:    requestID,
			group:        group,
			anchor:       sel,
			forward:      fList,
			reverse:      rList,
			dispatchedAt: dispatchedAt,
			forwardRPC:   forwardRPC,
			reverseRPC:   reverseRPC,
			rpcTotal:     time.Since(startAll),
			err:          err,
		}
	}
}

func (c *Catalog) inVisualRange(absolute int) bool {
	lo := min(c.visualAnchor, c.page.Cursor())
	hi := max(c.visualAnchor, c.page.Cursor())
	return absolute >= lo && absolute <= hi
}

// Selections implements MultiSelector.
func (c *Catalog) Selections() []domain.PatentNumber {
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

// Selection implements Pane.
func (c *Catalog) Selection() (domain.PatentNumber, bool) {
	cur := c.page.Cursor() - c.loadedBase
	if cur < 0 || cur >= len(c.patents) {
		return domain.PatentNumber{}, false
	}
	return c.patents[cur].Number, true
}

func patentRowMatchesNumber(row domain.PatentRow, number domain.PatentNumber) bool {
	return patentNumberMatches(row.Number, row.DisplayNumber, number)
}

func patentNumberMatches(record, display, number domain.PatentNumber) bool {
	return record == number || (!display.IsZero() && display == number)
}

func applyIDSEntriesToPatentRows(rows []domain.PatentRow, project domain.ProjectID, entries []domain.IDSEntry) (applied int, relevant int) {
	for i := range entries {
		entry := entries[i]
		if entry.Project != project {
			continue
		}
		relevant++
		for j := range rows {
			if patentRowMatchesNumber(rows[j], entry.Patent) {
				copied := entry
				rows[j].IDSEntry = &copied
				applied++
				break
			}
		}
	}
	return applied, relevant
}

// ActivityFocus implements ActivityFocusProvider.
func (c *Catalog) ActivityFocus() (ActivityFocus, bool) {
	cur := c.page.Cursor() - c.loadedBase
	if cur < 0 || cur >= len(c.patents) {
		return ActivityFocus{}, false
	}
	return patentRowActivity("catalog", c.Title(), c.patents[cur], activeProjectID(c.activeProject), c.filter), true
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
		if c.filter.IsActive() {
			var b strings.Builder
			filterHint := c.theme.Info.Render("filters: " + strings.Join(c.filter.Labels(), " · "))
			b.WriteString(renderTableStatusLine(c.theme, w, 0, 0, filterHint))
			b.WriteByte('\n')
			b.WriteString(c.theme.Warn.Render("no results for " + strings.Join(c.filter.Labels(), " · ") + " — :filter clear to see all"))
			return b.String()
		}
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
	segments := []string{}
	if c.filter.IsActive() && !c.find.active {
		segments = append(segments, c.theme.Info.Render("filters: "+strings.Join(c.filter.Labels(), " · ")))
	}
	if !c.activeHighlight.Anchor.IsZero() {
		g := c.theme.Glyphs
		switch c.activeHighlight.Group {
		case HighlightGroupFamily:
			if c.activeHighlight.Loading {
				segments = append(segments, g.FamilyLoading+" fetching family of "+c.activeHighlight.Anchor.Normalized())
			} else {
				parents := c.highlights.CountKind(HighlightFamilyParent)
				children := c.highlights.CountKind(HighlightFamilyChild)
				segments = append(segments, fmt.Sprintf("%s %s: %d%s %d%s",
					g.FamilyAnchor, c.activeHighlight.Anchor.Normalized(),
					parents, g.FamilyParent, children, g.FamilyChild))
			}
		case HighlightGroupCitations:
			if c.activeHighlight.Loading {
				segments = append(segments, g.CitationLoading+" fetching citations of "+c.activeHighlight.Anchor.Normalized())
			} else {
				cites := c.highlights.CountKind(HighlightCitationCites)
				citedby := c.highlights.CountKind(HighlightCitationCitedBy)
				segments = append(segments, fmt.Sprintf("%s %s: %d%s %d%s",
					g.CitationAnchor, c.activeHighlight.Anchor.Normalized(),
					cites, g.CitationCites, citedby, g.CitationCitedBy))
			}
		}
		if c.activeHighlight.FilterToRelations {
			var collapsedLabel string
			switch c.activeHighlight.Group {
			case HighlightGroupFamily:
				parents := c.highlights.CountKind(HighlightFamilyParent)
				children := c.highlights.CountKind(HighlightFamilyChild)
				collapsedLabel = fmt.Sprintf("collapsed %d%s parents · %d%s children", parents, g.FamilyParent, children, g.FamilyChild)
			case HighlightGroupCitations:
				cites := c.highlights.CountKind(HighlightCitationCites)
				citedby := c.highlights.CountKind(HighlightCitationCitedBy)
				collapsedLabel = fmt.Sprintf("collapsed %d%s cites · %d%s cited by", cites, g.CitationCites, citedby, g.CitationCitedBy)
			default:
				collapsedLabel = "collapsed"
			}
			segments = append(segments, c.theme.Warn.Render(collapsedLabel))
		}
	}
	filterSummary := strings.Join(segments, "  ")
	b.WriteString(renderTableStatusLine(c.theme, w, c.page.Cursor(), c.page.Total(), filterSummary))
	b.WriteByte('\n')
	// Only reserve a leading glyph column when at least one highlight is
	// active; this keeps the default catalog layout (and its tests) byte-
	// identical until the user toggles a highlight.
	showGlyphCol := c.highlights.Len() > 0 || c.activeHighlight.Loading
	if showGlyphCol {
		b.WriteString(c.theme.Header.Render("  "))
	}
	b.WriteString(renderTableHeader(c.theme, cols, c.activeSort, c.sortAscending, c.focusedColIdx))

	for i, p := range c.patents {
		absolute := c.loadedBase + i
		isSelectedRow := absolute == c.page.Cursor()
		highlightKind := c.highlights.Kind(p.Number)
		line := renderStyledTableRow(c.theme, p, cols, projectID, absolute, c.focusedColIdx, isSelectedRow, highlightKind, c.classDescs)
		if showGlyphCol {
			line = c.highlights.Glyph(c.theme, p.Number) + " " + line
		}
		rowStyle := tableRowStyle(c.theme, absolute)
		b.WriteByte('\n')
		switch {
		case c.visualMode && c.inVisualRange(absolute):
			b.WriteString(c.theme.Visual.Render(render.Pad(line, w)))
		case isSelectedRow:
			if style, ok := c.highlights.Style(c.theme, p.Number); ok && highlightKind.IsRelation() {
				b.WriteString(style.Copy().Bold(true).Underline(true).Render(render.Pad(line, w)))
			} else {
				b.WriteString(c.theme.Selected.Render(render.Pad(line, w)))
			}
		default:
			if style, ok := c.highlights.Style(c.theme, p.Number); ok {
				b.WriteString(style.Render(render.Pad(line, w)))
			} else {
				b.WriteString(rowStyle(render.Pad(line, w)))
			}
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
