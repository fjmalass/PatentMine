package pane

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

type familyGraphLoadedMsg struct {
	requestID uint64
	result    proto.FamilyGraphResult
	err       error
}

type familyGraphRowKind uint8

const (
	familyGraphRowHeader familyGraphRowKind = iota
	familyGraphRowNode
)

const familyGraphMaxDepthHint = 6

type familyGraphRow struct {
	kind  familyGraphRowKind
	depth int
	label string
	node  *proto.FamilyGraphNode
}

// FamilyGraph shows one bounded BFS parent/child graph rooted at a patent.
type FamilyGraph struct {
	client   *rpc.Client
	theme    render.Theme
	root     domain.PatentNumber
	project  domain.ProjectID
	handlers map[command.ID]cmdHandler

	depth     int
	countries []string
	maxNodes  int

	nodes        []proto.FamilyGraphNode
	edges        []proto.FamilyGraphEdge
	rows         []familyGraphRow
	nodeLabels   map[domain.PatentNumber]string
	parentRefs   map[domain.PatentNumber][]string
	childRefs    map[domain.PatentNumber][]string
	depthCounts  map[int]int
	crossEdges   int
	visualMode   bool
	visualAnchor int
	lastActive   domain.PatentNumber
	savedVisual  []domain.PatentNumber
	gvHighlight  map[domain.PatentNumber]bool
	page         render.Paginator
	loading      bool
	loadErr      string
	loadID       uint64
	truncated    bool
	hidden       int
	inconsistent int
}

// NewFamilyGraph builds a bounded family graph pane for one patent.
func NewFamilyGraph(client *rpc.Client, theme render.Theme, root domain.PatentNumber, depth int, countries []string) *FamilyGraph {
	g := &FamilyGraph{
		client:    client,
		theme:     theme,
		root:      root,
		depth:     depth,
		countries: append([]string(nil), countries...),
		maxNodes:  60,
		page:      render.NewPaginator(defaultPageSize),
		loading:   true,
	}
	g.handlers = map[command.ID]cmdHandler{
		command.NavDown:      func(inv Invocation) tea.Cmd { g.move(1, inv.Repeat); return nil },
		command.NavUp:        func(inv Invocation) tea.Cmd { g.move(-1, inv.Repeat); return nil },
		command.NavPageDown:  func(Invocation) tea.Cmd { g.moveTo(func() { g.page.PageDown() }); return nil },
		command.NavPageUp:    func(Invocation) tea.Cmd { g.moveTo(func() { g.page.PageUp() }); return nil },
		command.NavTop:       func(Invocation) tea.Cmd { g.jumpToFirstNode(); g.trackActive(); return nil },
		command.NavBottom:    func(Invocation) tea.Cmd { g.jumpToLastNode(); g.trackActive(); return nil },
		command.ReselectLast: func(Invocation) tea.Cmd { return g.reselectLast() },
		command.Refresh:      func(Invocation) tea.Cmd { g.loading = true; return g.load() },
		command.Filter:       g.applyFilter,
		command.SelectVisual: func(Invocation) tea.Cmd { return g.toggleVisual() },
		command.SelectAll:    func(Invocation) tea.Cmd { return g.selectAllVisual() },
		command.SelectClear: func(Invocation) tea.Cmd {
			if g.visualMode {
				g.saveVisual()
				g.clearVisual()
			}
			return nil
		},
		command.FamilyDepthMore: func(Invocation) tea.Cmd { return g.adjustDepth(1) },
		command.FamilyDepthLess: func(Invocation) tea.Cmd { return g.adjustDepth(-1) },
		command.CrawlFamily:     func(Invocation) tea.Cmd { return g.crawlSelected(domain.CrawlProfileFamily) },
		command.CrawlCitations:  func(Invocation) tea.Cmd { return g.crawlSelected(domain.CrawlProfileCitations) },
		command.CrawlCitedBy:    func(Invocation) tea.Cmd { return g.crawlSelected(domain.CrawlProfileCitedBy) },
		command.CrawlAll:        func(Invocation) tea.Cmd { return g.crawlSelected(domain.CrawlProfileAll) },
		command.LookupPatent:    func(Invocation) tea.Cmd { return g.crawlSelected("") },
	}
	return g
}

func (g *FamilyGraph) Scope() command.Scope { return command.ScopeFamily }

func (g *FamilyGraph) Title() string { return "Family graph · " + g.root.String() }

func (g *FamilyGraph) Init() tea.Cmd { return g.load() }

func (g *FamilyGraph) load() tea.Cmd {
	if g.client == nil {
		return nil
	}
	client := g.client
	requestID := nextAsyncID()
	g.loadID = requestID
	depth := g.depth
	if depth <= 0 {
		depth = 2
	}
	countries := append([]string(nil), g.countries...)
	maxNodes := g.maxNodes
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.FamilyGraphResult
		err := client.Call(ctx, proto.MethodFamilyGraph, proto.FamilyGraphParams{
			Root:      g.root,
			Depth:     depth,
			MaxNodes:  maxNodes,
			Project:   g.project,
			Countries: countries,
		}, &res)
		return familyGraphLoadedMsg{requestID: requestID, result: res, err: err}
	}
}

func (g *FamilyGraph) applyFilter(inv Invocation) tea.Cmd {
	msg, err := g.parseFilter(inv.Args)
	if err != nil {
		return func() tea.Msg { return StatusMsg{Key: text.StatusUsage, Args: []any{err.Error()}, Error: true} }
	}
	g.loading = true
	g.page.Top()
	return tea.Batch(
		g.load(),
		func() tea.Msg { return StatusMsg{Key: text.StatusFilter, Args: []any{msg}} },
	)
}

func (g *FamilyGraph) parseFilter(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf(":filter country <US EP ...> | :filter depth <n> | :filter clear")
	}
	switch args[0] {
	case "clear", "none", "reset":
		g.countries = nil
		return "family graph country filter cleared", nil
	case "country", "countries":
		if len(args) < 2 || args[1] == "clear" || args[1] == "none" {
			g.countries = nil
			return "family graph country filter cleared", nil
		}
		g.countries = append([]string(nil), args[1:]...)
		return "family graph countries: " + strings.Join(g.countries, ", "), nil
	case "depth", "bfs":
		if len(args) != 2 {
			return "", fmt.Errorf("depth filter expects one integer")
		}
		depth, err := strconv.Atoi(args[1])
		if err != nil || depth < 1 {
			return "", fmt.Errorf("depth must be a positive integer")
		}
		g.depth = depth
		return fmt.Sprintf("family graph depth: %d", depth), nil
	default:
		return "", fmt.Errorf("unknown filter type %q", args[0])
	}
}

func (g *FamilyGraph) Command(id command.ID, inv Invocation) (Pane, tea.Cmd) {
	if handler, ok := g.handlers[id]; ok {
		return g, handler(inv)
	}
	return g, nil
}

func (g *FamilyGraph) Handles() []command.ID { return handlerIDs(g.handlers) }

// HandleKey clears visual mode on Esc without shadowing the global Back binding.
func (g *FamilyGraph) HandleKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool) {
	if msg.String() != "esc" || !g.visualMode {
		return g, nil, false
	}
	g.saveVisual()
	g.clearVisual()
	return g, nil, true
}

func (g *FamilyGraph) adjustDepth(delta int) tea.Cmd {
	next := g.depth + delta
	if next < 1 {
		next = 1
	}
	if next > familyGraphMaxDepthHint {
		next = familyGraphMaxDepthHint
	}
	if next == g.depth {
		return nil
	}
	g.depth = next
	g.loading = true
	return tea.Batch(
		g.load(),
		func() tea.Msg {
			return StatusMsg{Key: text.StatusFilter, Args: []any{fmt.Sprintf("family graph depth: %d/%d", g.depth, familyGraphMaxDepthHint)}}
		},
	)
}

func (g *FamilyGraph) crawlSelected(profile domain.CrawlProfile) tea.Cmd {
	numbers := g.Selections()
	if len(numbers) < 2 {
		n, ok := g.Selection()
		if !ok {
			return status(text.StatusNoPatentSelected, true)
		}
		return CrawlCmd(g.client, n, crawlDepth(profile), profile, false)
	}
	return MultiCrawlCmd(g.client, numbers, crawlDepth(profile), profile, false)
}

func (g *FamilyGraph) Update(msg tea.Msg) (Pane, tea.Cmd) {
	switch m := msg.(type) {
	case familyGraphLoadedMsg:
		if m.requestID != g.loadID {
			return g, nil
		}
		g.loading = false
		if m.err != nil {
			g.loadErr = m.err.Error()
			return g, nil
		}
		g.loadErr = ""
		g.nodes = m.result.Nodes
		g.edges = m.result.Edges
		g.truncated = m.result.Truncated
		g.hidden = m.result.HiddenByCountry
		g.inconsistent = m.result.InconsistencyCount
		g.depth = m.result.Depth
		g.countries = append([]string(nil), m.result.Countries...)
		g.rebuildRows()
		g.page.SetTotal(len(g.rows))
		g.jumpToFirstNode()
	case ProjectChangedMsg:
		var project domain.ProjectID
		if m.Project != nil {
			project = m.Project.ID
		}
		if project != g.project {
			g.project = project
			g.loading = true
			return g, g.load()
		}
	}
	return g, nil
}

func (g *FamilyGraph) Selection() (domain.PatentNumber, bool) {
	row := g.selectedRow()
	if row == nil || row.node == nil {
		return domain.PatentNumber{}, false
	}
	return row.node.Patent.Number, true
}

func (g *FamilyGraph) selectedNumber() domain.PatentNumber {
	row := g.selectedRow()
	if row == nil || row.node == nil {
		return g.root
	}
	return row.node.Patent.Number
}

func (g *FamilyGraph) View(w, h int) string {
	if g.loading && len(g.rows) == 0 {
		return g.theme.Dim.Render("loading family graph…")
	}
	if g.loadErr != "" {
		return g.theme.Error.Render("error: " + g.loadErr)
	}
	if h < 3 {
		h = 3
	}
	var lines []string
	lines = append(lines, g.summaryLine(w))
	if filter := g.filterLine(w); filter != "" {
		lines = append(lines, filter)
	}

	listHeight := max(h-len(lines), 1)
	g.page.SetTotal(len(g.rows))
	g.page.SetPageSize(listHeight)
	start, end := g.page.Window()
	for i, row := range g.rows[start:end] {
		line := g.renderRow(row, w)
		if row.kind == familyGraphRowNode && row.node != nil && g.gvHighlight != nil && g.gvHighlight[row.node.Patent.Number] {
			lines = append(lines, g.theme.Visual.Render(render.Pad(line, w)))
			continue
		}
		if row.kind == familyGraphRowNode && g.visualMode && g.inVisualRange(start+i) {
			lines = append(lines, g.theme.Visual.Render(render.Pad(line, w)))
			continue
		}
		if start+i == g.page.Cursor() && row.kind == familyGraphRowNode {
			lines = append(lines, g.theme.Selected.Render(render.Pad(line, w)))
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (g *FamilyGraph) summaryLine(w int) string {
	parts := []string{
		fmt.Sprintf("root:%s", g.root.String()),
		fmt.Sprintf("depth:%d/%d", g.depth, familyGraphMaxDepthHint),
		fmt.Sprintf("nodes:%d", len(g.nodes)),
		fmt.Sprintf("edges:%d", len(g.edges)),
	}
	if g.crossEdges > 0 {
		parts = append(parts, fmt.Sprintf("cross:%d", g.crossEdges))
	}
	if g.truncated {
		parts = append(parts, g.theme.Warn.Render("truncated"))
	}
	if g.hidden > 0 {
		parts = append(parts, fmt.Sprintf("hidden:%d", g.hidden))
	}
	if g.inconsistent > 0 {
		parts = append(parts, g.theme.Error.Render(fmt.Sprintf("inconsistent:%d", g.inconsistent)))
	}
	return g.theme.Header.Render(render.Truncate(strings.Join(parts, "  "), w))
}

func (g *FamilyGraph) filterLine(w int) string {
	if len(g.countries) == 0 {
		return g.theme.Dim.Render(render.Truncate(" depth: use [ ] or -/+  ·  legend: R=root M=merge B=branch X=cross-edge", w))
	}
	return g.theme.Dim.Render(render.Truncate(" countries: "+strings.Join(g.countries, ", ")+"  ·  depth: use [ ] or -/+  ·  legend: R=root M=merge B=branch X=cross-edge", w))
}

func (g *FamilyGraph) renderRow(row familyGraphRow, w int) string {
	if row.kind == familyGraphRowHeader {
		if strings.TrimSpace(row.label) == "" {
			return ""
		}
		return g.theme.Header.Render(render.Truncate(row.label, w))
	}
	if row.node == nil {
		return ""
	}
	return g.renderNodeLine(*row.node, w)
}

func (g *FamilyGraph) renderNodeLine(node proto.FamilyGraphNode, w int) string {
	number := node.Patent.DisplayNumber
	if number.IsZero() {
		number = node.Patent.Number
	}
	parents := shortPatentList(node.Parents, 2)
	children := shortPatentList(node.Children, 3)
	title := strings.TrimSpace(node.Patent.Title)
	if title == "" {
		title = "(stub)"
	}
	line := fmt.Sprintf("  %s %-4s %-7s %-15s %-2s  up:%-12s  dn:%-12s  %s  %s",
		depthGlyph(node.Depth),
		g.nodeLabel(node.Patent.Number),
		badgeText(g.nodeBadges(node)),
		number.String(),
		countryOrDash(node.Patent.Number.Country),
		shortRefList(g.parentRefs[node.Patent.Number], 3, parents),
		shortRefList(g.childRefs[node.Patent.Number], 3, children),
		g.stateText(node.Patent),
		title,
	)
	return render.Truncate(line, w)
}

func (g *FamilyGraph) stateText(row domain.PatentRow) string {
	if g.project != "" && row.ReviewState.Valid() {
		return styledReviewStateText(g.theme, row.ReviewState)
	}
	return fetchStateText(g.theme, row.FetchState)
}

func shortPatentList(numbers []domain.PatentNumber, maxItems int) string {
	if len(numbers) == 0 {
		return "-"
	}
	parts := make([]string, 0, min(len(numbers), maxItems)+1)
	for i, number := range numbers {
		if i >= maxItems {
			parts = append(parts, fmt.Sprintf("+%d", len(numbers)-i))
			break
		}
		parts = append(parts, number.String())
	}
	return strings.Join(parts, ",")
}

func depthGlyph(depth int) string {
	if depth <= 0 {
		return "●"
	}
	if depth == 1 {
		return "├─"
	}
	return strings.Repeat("│", min(depth-1, 3)) + "├─"
}

func (g *FamilyGraph) rebuildRows() {
	g.rebuildGraphIndex()
	rows := make([]familyGraphRow, 0, len(g.nodes)+g.depth+1)
	lastDepth := -1
	for i := range g.nodes {
		node := &g.nodes[i]
		if node.Depth != lastDepth {
			if len(rows) > 0 {
				rows = append(rows, familyGraphRow{kind: familyGraphRowHeader, label: ""})
			}
			rows = append(rows, familyGraphRow{
				kind:  familyGraphRowHeader,
				depth: node.Depth,
				label: g.depthHeader(*node),
			})
			lastDepth = node.Depth
		}
		rows = append(rows, familyGraphRow{kind: familyGraphRowNode, depth: node.Depth, node: node})
	}
	g.rows = rows
}

func (g *FamilyGraph) depthHeader(node proto.FamilyGraphNode) string {
	label := fmt.Sprintf("Depth %d  ·  %d node(s)", node.Depth, g.depthCounts[node.Depth])
	if node.Patent.Number == g.root {
		label += "  root"
	}
	return label
}

func (g *FamilyGraph) rebuildGraphIndex() {
	g.nodeLabels = make(map[domain.PatentNumber]string, len(g.nodes))
	g.parentRefs = make(map[domain.PatentNumber][]string, len(g.nodes))
	g.childRefs = make(map[domain.PatentNumber][]string, len(g.nodes))
	g.depthCounts = make(map[int]int)
	g.crossEdges = 0
	depthByNumber := make(map[domain.PatentNumber]int, len(g.nodes))
	for i, node := range g.nodes {
		g.nodeLabels[node.Patent.Number] = fmt.Sprintf("N%02d", i+1)
		depthByNumber[node.Patent.Number] = node.Depth
		g.depthCounts[node.Depth]++
	}
	for _, edge := range g.edges {
		parentLabel := g.nodeLabels[edge.Parent]
		childLabel := g.nodeLabels[edge.Child]
		if parentLabel == "" || childLabel == "" {
			continue
		}
		g.childRefs[edge.Parent] = appendUniqueString(g.childRefs[edge.Parent], childLabel)
		g.parentRefs[edge.Child] = appendUniqueString(g.parentRefs[edge.Child], parentLabel)
		if depthByNumber[edge.Child] != depthByNumber[edge.Parent]+1 {
			g.crossEdges++
		}
	}
}

func (g *FamilyGraph) nodeLabel(number domain.PatentNumber) string {
	if label := g.nodeLabels[number]; label != "" {
		return label
	}
	return "N??"
}

func (g *FamilyGraph) nodeBadges(node proto.FamilyGraphNode) []string {
	badges := make([]string, 0, 4)
	if node.Patent.Number == g.root {
		badges = append(badges, "R")
	}
	if len(g.parentRefs[node.Patent.Number]) > 1 {
		badges = append(badges, "M")
	}
	if len(g.childRefs[node.Patent.Number]) > 1 {
		badges = append(badges, "B")
	}
	for _, parent := range node.Parents {
		if depth := g.nodeDepth(parent); depth >= 0 && depth+1 != node.Depth {
			badges = appendUniqueString(badges, "X")
		}
	}
	for _, child := range node.Children {
		if depth := g.nodeDepth(child); depth >= 0 && depth != node.Depth+1 {
			badges = appendUniqueString(badges, "X")
		}
	}
	return badges
}

func (g *FamilyGraph) nodeDepth(number domain.PatentNumber) int {
	for _, node := range g.nodes {
		if node.Patent.Number == number {
			return node.Depth
		}
	}
	return -1
}

func badgeText(badges []string) string {
	if len(badges) == 0 {
		return "-"
	}
	return "[" + strings.Join(badges, "") + "]"
}

func shortRefList(refs []string, maxItems int, fallback string) string {
	if len(refs) == 0 {
		return fallback
	}
	parts := make([]string, 0, min(len(refs), maxItems)+1)
	for i, ref := range refs {
		if i >= maxItems {
			parts = append(parts, fmt.Sprintf("+%d", len(refs)-i))
			break
		}
		parts = append(parts, ref)
	}
	return strings.Join(parts, ",")
}

func appendUniqueString(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func (g *FamilyGraph) selectedRow() *familyGraphRow {
	if len(g.rows) == 0 {
		return nil
	}
	idx := min(g.page.Cursor(), len(g.rows)-1)
	if g.rows[idx].kind == familyGraphRowNode {
		return &g.rows[idx]
	}
	for i := idx + 1; i < len(g.rows); i++ {
		if g.rows[i].kind == familyGraphRowNode {
			return &g.rows[i]
		}
	}
	for i := idx - 1; i >= 0; i-- {
		if g.rows[i].kind == familyGraphRowNode {
			return &g.rows[i]
		}
	}
	return nil
}

func (g *FamilyGraph) toggleVisual() tea.Cmd {
	if g.visualMode {
		g.saveVisual()
		g.clearVisual()
		return nil
	}
	g.visualMode = true
	g.visualAnchor = g.page.Cursor()
	return nil
}

func (g *FamilyGraph) selectAllVisual() tea.Cmd {
	if len(g.rows) == 0 {
		return nil
	}
	if g.visualMode {
		g.saveVisual()
	}
	g.visualMode = true
	g.jumpToFirstNode()
	g.visualAnchor = g.page.Cursor()
	g.jumpToLastNode()
	return nil
}

func (g *FamilyGraph) clearVisual() {
	g.visualMode = false
	g.visualAnchor = 0
	g.gvHighlight = nil
}

func (g *FamilyGraph) inVisualRange(index int) bool {
	lo := min(g.visualAnchor, g.page.Cursor())
	hi := max(g.visualAnchor, g.page.Cursor())
	return index >= lo && index <= hi
}

func (g *FamilyGraph) Selections() []domain.PatentNumber {
	if !g.visualMode || len(g.rows) == 0 {
		return nil
	}
	lo := min(g.visualAnchor, g.page.Cursor())
	hi := max(g.visualAnchor, g.page.Cursor())
	seen := map[domain.PatentNumber]bool{}
	out := make([]domain.PatentNumber, 0, hi-lo+1)
	for i := lo; i <= hi && i < len(g.rows); i++ {
		row := g.rows[i]
		if row.kind != familyGraphRowNode || row.node == nil {
			continue
		}
		number := row.node.Patent.Number
		if !seen[number] {
			seen[number] = true
			out = append(out, number)
		}
	}
	return out
}

func (g *FamilyGraph) saveVisual() {
	g.savedVisual = g.Selections()
	if sel, ok := g.Selection(); ok {
		g.lastActive = sel
	}
	if len(g.savedVisual) == 0 && !g.lastActive.IsZero() {
		g.savedVisual = []domain.PatentNumber{g.lastActive}
	}
}

func (g *FamilyGraph) SaveVisualSelection() {
	if g.visualMode {
		g.saveVisual()
		g.clearVisual()
	}
}

func (g *FamilyGraph) reselectLast() tea.Cmd {
	targets := g.savedVisual
	if len(targets) == 0 && !g.lastActive.IsZero() {
		targets = []domain.PatentNumber{g.lastActive}
	}
	if len(targets) == 0 {
		return status(text.StatusNoPatentSelected, false)
	}
	idx := make(map[domain.PatentNumber]int, len(g.rows))
	for i, row := range g.rows {
		if row.kind == familyGraphRowNode && row.node != nil {
			idx[row.node.Patent.Number] = i
		}
	}
	highlights := make(map[domain.PatentNumber]bool, len(targets))
	first := -1
	last := -1
	for _, target := range targets {
		highlights[target] = true
		if pos, ok := idx[target]; ok {
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
	g.clearVisual()
	g.visualMode = true
	g.visualAnchor = first
	g.gvHighlight = highlights
	g.page.ScrollTo(last)
	return nil
}

func (g *FamilyGraph) move(delta, repeat int) {
	if len(g.rows) == 0 {
		return
	}
	steps := max(repeat, 1)
	for i := 0; i < steps; i++ {
		idx := g.page.Cursor() + delta
		for idx >= 0 && idx < len(g.rows) && g.rows[idx].kind != familyGraphRowNode {
			idx += delta
		}
		if idx < 0 || idx >= len(g.rows) {
			return
		}
		g.page.ScrollTo(idx)
	}
	g.trackActive()
	g.gvHighlight = nil
}

func (g *FamilyGraph) moveTo(motion func()) {
	if len(g.rows) == 0 {
		return
	}
	motion()
	if g.rows[g.page.Cursor()].kind != familyGraphRowNode {
		if row := g.selectedRow(); row != nil && row.node != nil {
			for i := range g.rows {
				if g.rows[i].kind == familyGraphRowNode && g.rows[i].node != nil && g.rows[i].node.Patent.Number == row.node.Patent.Number {
					g.page.ScrollTo(i)
					break
				}
			}
		}
	}
	g.trackActive()
	g.gvHighlight = nil
}

func (g *FamilyGraph) trackActive() {
	if sel, ok := g.Selection(); ok {
		g.lastActive = sel
	}
}

func (g *FamilyGraph) jumpToFirstNode() {
	for i, row := range g.rows {
		if row.kind == familyGraphRowNode {
			g.page.ScrollTo(i)
			return
		}
	}
	if len(g.rows) > 0 {
		g.page.Top()
	}
}

func (g *FamilyGraph) jumpToLastNode() {
	for i := len(g.rows) - 1; i >= 0; i-- {
		if g.rows[i].kind == familyGraphRowNode {
			g.page.ScrollTo(i)
			return
		}
	}
	if len(g.rows) > 0 {
		g.page.Bottom()
	}
}
