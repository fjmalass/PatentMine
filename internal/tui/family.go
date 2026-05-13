package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"patentmine/internal/config"
	"patentmine/internal/domain"
	"patentmine/internal/importer"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var validFamilyRelations = map[string]string{
	domain.FamilyRelationContinuation: domain.FamilyRelationContinuation,
	domain.FamilyRelationDivisional:   domain.FamilyRelationDivisional,
	domain.FamilyRelationCIP:          domain.FamilyRelationCIP,
	"continuation-in-part":            domain.FamilyRelationCIP,
	domain.FamilyRelationPCT:          domain.FamilyRelationPCT,
}

// familyNode is one entry in the rendered flat family tree.
type familyNode struct {
	number       string
	depth        int    // negative = ancestor, 0 = current, positive = descendant
	relType      string // relation type from parent to this node (empty for roots)
	parentIdx    int    // index in the flat slice of this node's parent (-1 for roots)
	prefix       string // indentation prefix inherited from ancestor connectors
	connector    string // "├─ ", "└─ ", or "" for tree roots
	title        string
	assignee     string
	expiration   string
	expSource    string
	countryCode  string
	grantYear    string
	importSource string
	isDuplicate  bool
}

// buildFamilyTree returns an ordered flat list of all family tree nodes
// reachable from m.current, using DFS from ancestor roots downward through
// descendants. parentIdx links each node back to its parent entry.
// Results are cached in m.familyTreeCache; callers that modify the tree must
// clear familyTreeCacheFor to force a rebuild.
func (m *Model) buildFamilyTree() []familyNode {
	startedAt := time.Now()
	if m.familyTreeCacheFor == m.current.Number && len(m.familyTreeCache) > 0 {
		logFamilyDebug(m.logger, "tui.family_tree.cache_hit",
			"patent", m.current.Number,
			"node_count", len(m.familyTreeCache),
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
		return m.familyTreeCache
	}
	nodes := m.buildFamilyTreeFresh()
	m.familyTreeCache = nodes
	m.familyTreeCacheFor = m.current.Number
	logFamilyDebug(m.logger, "tui.family_tree.cache_miss",
		"patent", m.current.Number,
		"node_count", len(nodes),
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)
	return nodes
}

const maxFamilyNodes = 250

func familyDebugEnabled(logger *slog.Logger) bool {
	return logger != nil && logger.Enabled(context.Background(), slog.LevelDebug)
}

func logFamilyDebug(logger *slog.Logger, msg string, attrs ...any) {
	if !familyDebugEnabled(logger) {
		return
	}
	logger.Debug(msg, attrs...)
}

func (m *Model) invalidateFamilyCaches() {
	m.familyTreeCache = nil
	m.familyTreeCacheFor = ""
	m.familyPatentCache = nil
	m.familyPatentCacheMisses = nil
}

func (m *Model) loadFamilyPatentMetadata(numbers []string) (map[string]domain.Patent, int, int, int, int, error) {
	if m.familyPatentCache == nil {
		m.familyPatentCache = make(map[string]domain.Patent)
	}
	if m.familyPatentCacheMisses == nil {
		m.familyPatentCacheMisses = make(map[string]bool)
	}

	result := make(map[string]domain.Patent, len(numbers))
	toFetch := make([]string, 0, len(numbers))
	cacheHits := 0
	cacheMisses := 0
	alreadyMissing := 0
	seen := make(map[string]bool, len(numbers))
	for _, number := range numbers {
		if number == "" || seen[number] {
			continue
		}
		seen[number] = true
		if p, ok := m.familyPatentCache[number]; ok {
			result[number] = p
			cacheHits++
			continue
		}
		if m.familyPatentCacheMisses[number] {
			alreadyMissing++
			continue
		}
		toFetch = append(toFetch, number)
	}
	if len(toFetch) == 0 {
		return result, cacheHits, cacheMisses, alreadyMissing, 0, nil
	}

	fetched, err := m.repo.ListFamilyPatents(m.ctx, m.ProjectID, toFetch)
	if err != nil {
		return nil, cacheHits, cacheMisses, alreadyMissing, len(toFetch), err
	}
	for _, number := range toFetch {
		p, ok := fetched[number]
		if ok {
			m.familyPatentCache[number] = p
			result[number] = p
			cacheMisses++
			continue
		}
		m.familyPatentCacheMisses[number] = true
	}
	return result, cacheHits, cacheMisses, alreadyMissing, len(toFetch), nil
}

func (m *Model) buildFamilyTreeFresh() []familyNode {
	startedAt := time.Now()
	stageStartedAt := time.Now()
	allEdges, _ := m.repo.ListAllFamilyEdges(m.ctx, m.ProjectID)
	listEdgesMs := time.Since(stageStartedAt).Milliseconds()
	if len(allEdges) == 0 {
		logFamilyDebug(m.logger, "tui.family_tree.build",
			"patent", m.current.Number,
			"edge_count", 0,
			"node_count", 0,
			"list_edges_ms", listEdgesMs,
			"total_ms", time.Since(startedAt).Milliseconds(),
		)
		return nil
	}

	stageStartedAt = time.Now()
	graph := domain.BuildFamilyGraph(m.current.Number, allEdges)
	graphMs := time.Since(stageStartedAt).Milliseconds()
	if graph == nil || len(graph.Nodes) == 0 {
		logFamilyDebug(m.logger, "tui.family_tree.build",
			"patent", m.current.Number,
			"edge_count", len(allEdges),
			"node_count", 0,
			"list_edges_ms", listEdgesMs,
			"build_graph_ms", graphMs,
			"total_ms", time.Since(startedAt).Milliseconds(),
		)
		return nil
	}

	nodes := make([]familyNode, len(graph.Nodes))
	for i, tn := range graph.Nodes {
		nodes[i] = familyNode{
			number:    tn.Number,
			depth:     tn.Depth,
			relType:   tn.RelationType,
			parentIdx: tn.ParentIdx,
		}
	}

	// 2. Compute visual connectors (prefix/connector)
	// This logic remains in TUI as it's purely for rendering.
	stageStartedAt = time.Now()
	type nodeInfo struct {
		childrenLinePrefix string
	}
	infos := make([]nodeInfo, len(nodes))
	lastChildForParent := make(map[int]int, len(nodes))
	for i := range nodes {
		if nodes[i].parentIdx >= 0 {
			lastChildForParent[nodes[i].parentIdx] = i
		}
	}

	for i := range nodes {
		pIdx := nodes[i].parentIdx
		if pIdx < 0 {
			nodes[i].prefix = ""
			nodes[i].connector = ""
			infos[i].childrenLinePrefix = ""
			continue
		}

		isLast := lastChildForParent[pIdx] == i

		nodes[i].prefix = infos[pIdx].childrenLinePrefix
		nodes[i].connector = "├─ "
		infos[i].childrenLinePrefix = nodes[i].prefix + "│ "
		if isLast {
			nodes[i].connector = "└─ "
			infos[i].childrenLinePrefix = nodes[i].prefix + "  "
		}
	}
	connectorsMs := time.Since(stageStartedAt).Milliseconds()

	// 3. Enrich with patent info (use local cache to avoid redundant lookups)
	stageStartedAt = time.Now()
	numbers := make([]string, 0, len(nodes))
	for _, node := range nodes {
		numbers = append(numbers, node.number)
	}
	patentCache, cacheHits, cacheMisses, knownMisses, bulkFetchCount, err := m.loadFamilyPatentMetadata(numbers)
	if err != nil {
		logFamilyDebug(m.logger, "tui.family_tree.family_metadata_error",
			"patent", m.current.Number,
			"requested_count", len(numbers),
			"error", err,
		)
		patentCache = map[string]domain.Patent{}
	}
	for i := range nodes {
		num := nodes[i].number
		p, ok := patentCache[num]
		if ok {
			nodes[i].title = p.Title
			nodes[i].assignee = p.Assignee
			nodes[i].expiration = p.ExpirationDate
			nodes[i].expSource = p.ExpirationSource
			nodes[i].countryCode = p.CountryCode
			if len(p.GrantDate) >= 4 {
				nodes[i].grantYear = p.GrantDate[:4]
			} else if len(p.PublicationDate) >= 4 {
				nodes[i].grantYear = p.PublicationDate[:4]
			}
			nodes[i].importSource = p.ImportSource
		}
	}
	enrichMs := time.Since(stageStartedAt).Milliseconds()

	logFamilyDebug(m.logger, "tui.family_tree.build",
		"patent", m.current.Number,
		"edge_count", len(allEdges),
		"node_count", len(nodes),
		"list_edges_ms", listEdgesMs,
		"build_graph_ms", graphMs,
		"connectors_ms", connectorsMs,
		"enrich_patents_ms", enrichMs,
		"bulk_fetch_count", bulkFetchCount,
		"requested_patent_count", len(numbers),
		"cache_hits", cacheHits,
		"cache_misses", cacheMisses,
		"known_misses", knownMisses,
		"total_ms", time.Since(startedAt).Milliseconds(),
	)

	return nodes
}

// familyCurrentIdx returns the index of the current patent (depth == 0) in the list.
func familyCurrentIdx(nodes []familyNode) int {
	for i, n := range nodes {
		if n.depth == 0 {
			return i
		}
	}
	return 0
}

var familyRelationBadgeLabels = map[string]string{
	domain.FamilyRelationContinuation: "cont.",
	domain.FamilyRelationDivisional:   "div.",
	domain.FamilyRelationCIP:          "cip",
	domain.FamilyRelationPCT:          "pct",
}

// familyDepthBadge returns a compact, unambiguous depth marker relative to the current patent.
func familyDepthBadge(depth int) string {
	if depth < 0 {
		return fmt.Sprintf("↑%d", -depth)
	}
	return fmt.Sprintf("↓%d", depth)
}

func familyRelationBadge(relType string) string {
	if label, ok := familyRelationBadgeLabels[relType]; ok {
		return label
	}
	return relType
}

// importSourceBadge renders a compact source indicator: [g] or [u].
func importSourceBadge(base lipgloss.Style, source string) string {
	switch source {
	case ImportSourceUSPTO:
		return base.Foreground(lipgloss.Color(ColorDepth)).Render("[u]")
	case ImportSourceGoogle:
		return base.Foreground(lipgloss.Color(ColorDim)).Render("[g]")
	}
	return ""
}

// familyRelationColor returns the terminal color for a family relation type.
func familyRelationColor(relType string) string {
	switch relType {
	case domain.FamilyRelationContinuation:
		return ColorFamilyContinuation
	case domain.FamilyRelationCIP:
		return ColorFamilyCIP
	case domain.FamilyRelationDivisional:
		return ColorFamilyDivisional
	case domain.FamilyRelationPCT:
		return ColorFamilyPCT
	default:
		return ColorDim
	}
}

// viewFamilyOverlay renders the interactive family tree as a single navigable column.
// j/k move up/down.
func (m *Model) viewFamilyOverlay() string {
	startedAt := time.Now()
	nodes := m.buildFamilyTree()

	base := overlayBase()
	subtle := base.Foreground(lipgloss.Color(ColorSubtle))
	accent := base.Foreground(lipgloss.Color(ColorAccentFamily)).Bold(true)
	currentStyle := base.Foreground(lipgloss.Color(ColorAccent)).Bold(true)
	cursorStyle := base.Foreground(lipgloss.Color(ColorAccentFamily)).Bold(true)

	if len(nodes) == 0 {
		logFamilyDebug(m.logger, "tui.family_overlay.render",
			"patent", m.current.Number,
			"node_count", 0,
			"visible_rows", 0,
			"selected", m.familySelected,
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
		return m.renderPopup("Family · "+m.current.Number, subtle.Render("No family relationships defined."))
	}

	sel := clamp(m.familySelected, 0, len(nodes)-1)
	window := pageWindow(sel, len(nodes), m.overlayPageSize())

	var body strings.Builder
	statusLine := subtle.Render(pageStatus(m.text.T(TextValuePageStatus), window))
	if m.familyRefreshElapsed != "" {
		statusLine += subtle.Render(" · ") + base.Foreground(lipgloss.Color(ColorDim)).Italic(true).Render(m.familyRefreshElapsed)
	}
	body.WriteString(statusLine + "\n\n")

	titleWidth := max(20, m.overlayWidth()-56)

	for i := window.Start; i < window.End; i++ {
		node := nodes[i]
		isCurrent := node.number == m.current.Number
		isSelected := i == sel

		var line strings.Builder

		// Leading cursor column (2 chars wide to keep tree aligned).
		if isSelected {
			line.WriteString(cursorStyle.Render("▶ "))
		} else {
			line.WriteString(base.Render("  "))
		}

		// Tree prefix + connector.
		line.WriteString(base.Render(node.prefix + node.connector))

		// Current-patent marker.
		if isCurrent {
			line.WriteString(accent.Render("● "))
		}

		// Patent number.
		numStyle := base
		if isCurrent {
			numStyle = accent.Underline(true)
		}
		line.WriteString(numStyle.Render(node.number))

		// Depth badge (compact relative position indicator).
		if !isCurrent {
			relColor := ColorDim
			if node.depth < 0 {
				relColor = ColorDepth // Ancestors
			}
			line.WriteString(base.Render(" "))
			line.WriteString(base.Foreground(lipgloss.Color(relColor)).Render("«" + familyDepthBadge(node.depth) + "»"))
		}

		// Grant/publication year badge.
		if node.grantYear != "" {
			line.WriteString(subtle.Render(" (" + node.grantYear + ")"))
		}

		// Import source badge.
		if node.importSource != "" {
			line.WriteString(base.Render(" "))
			line.WriteString(importSourceBadge(base, node.importSource))
		}

		// Title (truncated).
		if node.title != "" {
			titleStyle := subtle
			if isCurrent {
				titleStyle = currentStyle.Bold(true).Underline(true)
			}
			line.WriteString(base.Render(" "))
			line.WriteString(titleStyle.Render(m.truncate(node.title, titleWidth)))
		}

		// Relation-type badge.
		if node.relType != "" {
			relColor := familyRelationColor(node.relType)
			line.WriteString(base.Render(" "))
			line.WriteString(base.Foreground(lipgloss.Color(relColor)).Render("[" + familyRelationBadge(node.relType) + "]"))
		}

		if nodeNeedsRefresh(node, selectedFamilyPatentValue(node)) {
			line.WriteString(base.Render(" "))
			line.WriteString(subtle.Italic(true).Render(FamilyNodeStatusLabelUnloaded))
		}

		body.WriteString(line.String() + "\n")
	}

	if selectedPatent, ok := m.selectedFamilyPatent(nodes[sel]); ok {
		body.WriteString("\n")
		body.WriteString(subtle.Render(strings.Repeat("─", max(24, m.overlayWidth()-6))) + "\n")
		body.WriteString("\n")

		labelStyle := base.Foreground(lipgloss.Color(ColorSubtle))
		valueStyle := base.Foreground(lipgloss.Color(ColorWhite))
		previewTitleWidth := max(20, m.overlayWidth()-22)
		nameValue := selectedPatent.Assignee
		if strings.TrimSpace(nameValue) == "" {
			nameValue = "-"
		}
		titleValue := selectedPatent.Title
		if strings.TrimSpace(titleValue) == "" {
			titleValue = "-"
		}
		expiresValue := selectedPatent.ExpirationDate
		if strings.TrimSpace(expiresValue) == "" {
			expiresValue = "-"
		} else {
			expiresValue = m.formatExpiration(selectedPatent)
		}
		rows := []struct {
			label string
			value string
		}{
			{label: "Number", value: selectedPatent.Number},
			{label: "Country", value: patentCountryLabel(selectedPatent)},
			{label: "Expires", value: expiresValue},
			{label: "Name", value: nameValue},
			{label: "Title", value: m.truncate(titleValue, previewTitleWidth)},
		}
		for _, row := range rows {
			body.WriteString(labelStyle.Render(fmt.Sprintf("%-8s", row.label+":")) + " " + valueStyle.Render(row.value) + "\n")
		}
		if nodeNeedsRefresh(nodes[sel], selectedPatent) {
			body.WriteString("\n")
			body.WriteString(subtle.Render("No stored metadata for this family member. Press ctrl+r to refresh selected.") + "\n")
		}
	}

	logFamilyDebug(m.logger, "tui.family_overlay.render",
		"patent", m.current.Number,
		"node_count", len(nodes),
		"visible_rows", window.End-window.Start,
		"selected", sel,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)

	return m.renderPopup("Family · "+m.current.Number, body.String())
}

func (m *Model) selectedFamilyPatent(node familyNode) (domain.Patent, bool) {
	if node.number == "" {
		return domain.Patent{}, false
	}
	return selectedFamilyPatentValue(node), true
}

func selectedFamilyPatentValue(node familyNode) domain.Patent {
	return domain.Patent{
		Number:           node.number,
		CountryCode:      node.countryCode,
		Title:            node.title,
		Assignee:         node.assignee,
		ExpirationDate:   node.expiration,
		ExpirationSource: node.expSource,
	}
}

func nodeNeedsRefresh(node familyNode, p domain.Patent) bool {
	if strings.TrimSpace(node.number) == "" {
		return false
	}
	return strings.TrimSpace(p.Title) == "" && strings.TrimSpace(p.Assignee) == "" && strings.TrimSpace(p.ExpirationDate) == ""
}

func patentCountryLabel(p domain.Patent) string {
	if strings.TrimSpace(p.CountryCode) != "" {
		return p.CountryCode
	}
	code := domain.PatentCountryFromNumber(p.Number)
	if code == "" {
		return "-"
	}
	return code
}

func (m *Model) selectedFamilyNode() (familyNode, bool) {
	nodes := m.buildFamilyTree()
	if m.familySelected < 0 || m.familySelected >= len(nodes) {
		return familyNode{}, false
	}
	return nodes[m.familySelected], true
}

func (m *Model) moveFamilySelection(delta int) *Model {
	nodes := m.buildFamilyTree()
	if len(nodes) == 0 {
		return m
	}
	m.familySelected = clamp(m.familySelected+delta, 0, len(nodes)-1)
	return m
}

func (m *Model) refreshSelectedFamilyMember() (tea.Model, tea.Cmd) {
	node, ok := m.selectedFamilyNode()
	if !ok {
		m.err = "no family member selected"
		return m, nil
	}
	if node.number == "" {
		m.err = "selected family member has no patent number"
		return m, nil
	}
	if node.number == m.current.Number {
		return m.pullFamilyCommand()
	}

	ctx, cancel := context.WithCancel(m.ctx)
	m.loading = true
	m.loadingMsg = fmt.Sprintf("Refreshing family member %s...", node.number)
	m.cancel = cancel

	repo := m.repo
	projectID := m.ProjectID
	logger := m.logger
	targetNumber := node.number
	selection := m.familySelected

	return m, tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			startedAt := time.Now()
			bundle, err := m.importPatent(targetNumber)
			if err != nil {
				return refreshResultMsg{err: fmt.Errorf("family member refresh failed: %w", err)}
			}

			bundle.Patent.ReviewState = domain.ReviewStateCached
			if existing, err := repo.GetPatent(ctx, projectID, targetNumber); err == nil && existing.Number != "" && existing.ReviewState != "" {
				bundle.Patent.ReviewState = existing.ReviewState
			}
			if err := repo.UpsertPatentBundle(ctx, projectID, bundle); err != nil {
				logger.Error("family member store failed", "patent", targetNumber, "error", err)
				return refreshResultMsg{err: err}
			}
			m.invalidateFamilyCaches()

			p, err := repo.GetPatent(ctx, projectID, m.current.Number)
			if err != nil {
				return refreshResultMsg{err: err}
			}
			return refreshResultMsg{
				patent:         p,
				mode:           viewFamily,
				familySelected: selection,
				elapsed:        time.Since(startedAt),
				message:        fmt.Sprintf("refreshed family member %s", targetNumber),
				action:         ActivityFamilyRefresh,
				source:         bundle.Patent.ImportSource,
			}
		},
	)
}

func (m *Model) moveFamilyToParent() *Model {
	nodes := m.buildFamilyTree()
	if m.familySelected < 0 || m.familySelected >= len(nodes) {
		return m
	}
	pidx := nodes[m.familySelected].parentIdx
	if pidx < 0 {
		return m
	}
	m.familySelected = pidx
	return m
}

func (m *Model) moveFamilyToFirstChild() *Model {
	nodes := m.buildFamilyTree()
	sel := m.familySelected
	for i := sel + 1; i < len(nodes); i++ {
		if nodes[i].parentIdx == sel {
			m.familySelected = i
			return m
		}
	}
	return m
}

func (m *Model) openSelectedFamilyMember() (tea.Model, tea.Cmd) {
	nodes := m.buildFamilyTree()
	if m.familySelected < 0 || m.familySelected >= len(nodes) {
		return m, nil
	}
	node := nodes[m.familySelected]
	if node.depth == 0 {
		return m, nil
	}
	return m.openPatent(node.number)
}

func (m *Model) removeSelectedFamilyEdge() (tea.Model, tea.Cmd) {
	nodes := m.buildFamilyTree()
	if m.familySelected < 0 || m.familySelected >= len(nodes) {
		return m, nil
	}
	node := nodes[m.familySelected]
	if node.parentIdx < 0 {
		m.err = "no parent edge to remove (node is a root)"
		return m, nil
	}
	parent := nodes[node.parentIdx]
	if err := m.repo.RemoveFamilyEdge(m.ctx, m.ProjectID, parent.number, node.number); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.familySelected = clamp(m.familySelected, 0, max(0, len(nodes)-2))
	m.invalidateFamilyCaches()
	m.message = fmt.Sprintf("removed family edge: %s → %s", parent.number, node.number)
	return m, nil
}

// familyItems returns the direct parent and child edges of the current patent.
func (m *Model) familyItems() (parents []domain.FamilyEdge, children []domain.FamilyEdge) {
	parents, children, _ = m.repo.ListFamilyEdges(m.ctx, m.ProjectID, m.current.Number)
	return
}

// familyCommand handles :family parent|child|remove sub-commands.
func (m *Model) familyCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.err = "usage: :family parent|child <number> [type]  ·  :family remove <number>"
		return m, nil
	}
	action := strings.ToLower(args[0])

	if len(args) < 2 {
		m.err = "usage: :family parent|child <number> [continuation|divisional|cip|pct] or :family remove <number>"
		return m, nil
	}
	target := strings.ToUpper(strings.TrimSpace(args[1]))

	if action == "remove" {
		parents, children := m.familyItems()
		for _, e := range parents {
			if e.ParentNumber == target {
				if err := m.repo.RemoveFamilyEdge(m.ctx, m.ProjectID, e.ParentNumber, e.ChildNumber); err != nil {
					m.err = err.Error()
					return m, nil
				}
				m.invalidateFamilyCaches()
				m.message = "removed family edge: " + target + " → " + m.current.Number
				return m, nil
			}
		}
		for _, e := range children {
			if e.ChildNumber == target {
				if err := m.repo.RemoveFamilyEdge(m.ctx, m.ProjectID, e.ParentNumber, e.ChildNumber); err != nil {
					m.err = err.Error()
					return m, nil
				}
				m.invalidateFamilyCaches()
				m.message = "removed family edge: " + m.current.Number + " → " + target
				return m, nil
			}
		}
		m.err = "no family relationship found with " + target
		return m, nil
	}

	if action != "parent" && action != "child" {
		m.err = "usage: :family parent|child <number> [type] or :family remove <number>"
		return m, nil
	}

	if m.current.Number == "" {
		m.err = "no patent selected"
		return m, nil
	}
	if target == m.current.Number {
		m.err = "a patent cannot be its own parent or child"
		return m, nil
	}

	targetPatent, err := m.repo.GetPatent(m.ctx, m.ProjectID, target)
	if err != nil || targetPatent.Number == "" {
		m.err = fmt.Sprintf("patent %s not in project — add with :add %s first", target, target)
		return m, nil
	}
	if targetPatent.ReviewState != domain.ReviewStateStored {
		m.err = fmt.Sprintf("patent %s is not stored in this project (status: %s)", target, targetPatent.ReviewState)
		return m, nil
	}

	relType := domain.FamilyRelationContinuation
	if len(args) >= 3 {
		raw := strings.ToLower(args[2])
		canonical, ok := validFamilyRelations[raw]
		if !ok {
			m.err = fmt.Sprintf("unknown relation type %q — valid: continuation, divisional, cip, pct", raw)
			return m, nil
		}
		relType = canonical
	}

	var parentNumber, childNumber string
	if action == "parent" {
		parentNumber = target
		childNumber = m.current.Number
	} else {
		parentNumber = m.current.Number
		childNumber = target
	}

	edge := domain.FamilyEdge{
		ProjectID:    m.ProjectID,
		ParentNumber: parentNumber,
		ChildNumber:  childNumber,
		RelationType: relType,
	}
	if err := m.repo.AddFamilyEdge(m.ctx, edge); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.invalidateFamilyCaches()
	m.message = fmt.Sprintf("family edge added: %s → %s (%s)", parentNumber, childNumber, relType)
	m.setMode(viewFamily)
	return m, nil
}

// pullFamilyCommand fetches all direct family members detected on Google Patents,
// stores each as a project patent, then re-upserts the current patent so all
// family edges link up.
func (m *Model) pullFamilyCommand() (tea.Model, tea.Cmd) {
	if m.current.Number == "" {
		m.err = "no patent selected"
		return m, nil
	}

	importSource := m.importCfg.ImportSource
	apiKey := m.importCfg.USPTO.APIKey

	var rawURL string
	if importSource != config.ImportSourceUSPTO || apiKey == "" {
		rawURL = m.current.SourceGoogleURL
		if rawURL == "" {
			var err error
			rawURL, err = importer.GooglePatentsURL(m.current.Number)
			if err != nil {
				m.err = err.Error()
				return m, nil
			}
		}
	}

	ctx, cancel := context.WithCancel(m.ctx)
	m.loading = true
	m.loadingMsg = fmt.Sprintf("Pulling family for %s...", m.current.Number)
	m.cancel = cancel

	repo := m.repo
	projectID := m.ProjectID
	currentNumber := m.current.Number
	currentStatus := m.current.ReviewState
	logger := m.logger

	return m, tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			startedAt := time.Now()
			var bundle domain.PatentBundle
			var err error
			if importSource == config.ImportSourceUSPTO && apiKey != "" {
				bundle, err = importer.ImportUSPTO(currentNumber, apiKey, logger)
			} else {
				bundle, err = importer.ImportGooglePatents(rawURL, logger)
			}
			if err != nil {
				return refreshResultMsg{err: fmt.Errorf("import failed: %w", err)}
			}
			if importSource == config.ImportSourceUSPTO && apiKey != "" {
				bundle.Patent.ImportSource = ImportSourceUSPTO
			} else {
				bundle.Patent.ImportSource = ImportSourceGoogle
			}
			bundle.Patent.ReviewState = currentStatus

			if len(bundle.FamilyEdges) == 0 {
				p, _ := repo.GetPatent(ctx, projectID, currentNumber)
				source := ImportSourceGoogle
				if importSource == config.ImportSourceUSPTO && apiKey != "" {
					source = ImportSourceUSPTO
				}
				return refreshResultMsg{
					patent:  p,
					mode:    viewFamily,
					elapsed: time.Since(startedAt),
					message: "no family edges found for " + currentNumber,
					action:  ActivityFamilyRefresh,
					source:  source,
				}
			}

			seen := map[string]bool{currentNumber: true}
			var members []string
			for _, e := range bundle.FamilyEdges {
				for _, num := range []string{e.ParentNumber, e.ChildNumber} {
					if !seen[num] {
						seen[num] = true
						members = append(members, num)
					}
				}
			}

			imported, failed := 0, 0
			for _, num := range members {
				var memberBundle domain.PatentBundle
				var err error
				if importSource == config.ImportSourceUSPTO && apiKey != "" {
					memberBundle, err = importer.ImportUSPTO(num, apiKey, logger)
				} else {
					var memberURL string
					memberURL, err = importer.GooglePatentsURL(num)
					if err == nil {
						memberBundle, err = importer.ImportGooglePatents(memberURL, logger)
					}
				}
				if err != nil {
					logger.Warn("family member fetch failed", "patent", num, "error", err)
					failed++
					continue
				}
				if importSource == config.ImportSourceUSPTO && apiKey != "" {
					memberBundle.Patent.ImportSource = ImportSourceUSPTO
				} else {
					memberBundle.Patent.ImportSource = ImportSourceGoogle
				}
				memberBundle.Patent.ReviewState = domain.ReviewStateCached
				if err := repo.UpsertPatentBundle(ctx, projectID, memberBundle); err != nil {
					logger.Error("family member store failed", "patent", num, "error", err)
					failed++
					continue
				}
				imported++
			}

			bundle.Patent.ReviewState = currentStatus
			_ = repo.UpsertPatentBundle(ctx, projectID, bundle)

			p, err := repo.GetPatent(ctx, projectID, currentNumber)
			if err != nil {
				return refreshResultMsg{err: err}
			}

			source := ImportSourceGoogle
			if importSource == config.ImportSourceUSPTO && apiKey != "" {
				source = ImportSourceUSPTO
			}
			msg := fmt.Sprintf("family refresh: %d/%d members imported", imported, len(members))
			if failed > 0 {
				msg += fmt.Sprintf(" (%d failed)", failed)
			}
			return refreshResultMsg{patent: p, mode: viewFamily, elapsed: time.Since(startedAt), message: msg, action: ActivityFamilyRefresh, source: source}
		},
	)
}
