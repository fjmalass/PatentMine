package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"patentmine/internal/config"
	"patentmine/internal/domain"
	"patentmine/internal/importer"
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
	grantYear    string
	importSource string
}

// buildFamilyTree returns an ordered flat list of all family tree nodes
// reachable from m.current, using DFS from ancestor roots downward through
// descendants. parentIdx links each node back to its parent entry.
// Results are cached in m.familyTreeCache; callers that modify the tree must
// clear familyTreeCacheFor to force a rebuild.
func (m *Model) buildFamilyTree() []familyNode {
	if m.familyTreeCacheFor == m.current.Number && len(m.familyTreeCache) > 0 {
		return m.familyTreeCache
	}
	return m.buildFamilyTreeFresh()
}

func (m *Model) buildFamilyTreeFresh() []familyNode {
	allEdges, _ := m.repo.ListAllFamilyEdges(m.ctx, m.ProjectID)

	parentToChildren := map[string][]domain.FamilyEdge{}
	childToParents := map[string][]domain.FamilyEdge{}
	for _, e := range allEdges {
		parentToChildren[e.ParentNumber] = append(parentToChildren[e.ParentNumber], e)
		childToParents[e.ChildNumber] = append(childToParents[e.ChildNumber], e)
	}

	current := m.current.Number
	if current == "" {
		return nil
	}

	// BFS to find all connected nodes and assign depths relative to current.
	// We use a modified BFS that ensures a child's depth is always parent.depth + 1.
	depths := map[string]int{current: 0}

	// First, find all reachable nodes to establish the set.
	reachable := map[string]bool{current: true}
	q := []string{current}
	for i := 0; i < len(q); i++ {
		n := q[i]
		for _, e := range childToParents[n] {
			if !reachable[e.ParentNumber] {
				reachable[e.ParentNumber] = true
				q = append(q, e.ParentNumber)
			}
		}
		for _, e := range parentToChildren[n] {
			if !reachable[e.ChildNumber] {
				reachable[e.ChildNumber] = true
				q = append(q, e.ChildNumber)
			}
		}
	}

	// Calculate depths based on relationships.
	// Roots get an initial depth, then propagate.
	// This is a simple approximation for a DAG.
	for i := 0; i < 10; i++ { // Iterate a few times to stabilize depths
		changed := false
		for n := range reachable {
			for _, e := range childToParents[n] {
				if d, ok := depths[n]; ok {
					if oldD, ok2 := depths[e.ParentNumber]; !ok2 || oldD > d-1 {
						depths[e.ParentNumber] = d - 1
						changed = true
					}
				}
			}
			for _, e := range parentToChildren[n] {
				if d, ok := depths[n]; ok {
					if oldD, ok2 := depths[e.ChildNumber]; !ok2 || oldD < d+1 {
						depths[e.ChildNumber] = d + 1
						changed = true
					}
				}
			}
		}
		if !changed {
			break
		}
	}

	nodeSet := reachable

	// Find roots: nodes with no parent within the connected component.
	var roots []string
	for node := range nodeSet {
		hasParent := false
		for _, e := range childToParents[node] {
			if nodeSet[e.ParentNumber] {
				hasParent = true
				break
			}
		}
		if !hasParent {
			roots = append(roots, node)
		}
	}
	sort.Strings(roots)

	// DFS from roots, producing the flat ordered node list.
	//
	// linePrefix: the visual prefix for this node's rendered line (inherited
	//             from the grandparent's childrenLinePrefix calculation).
	// childrenLinePrefix: the prefix that will become linePrefix for each
	//                     direct child of this node.
	var nodes []familyNode
	visited := map[string]bool{}

	var dfs func(number, relType, linePrefix, connector, childrenLinePrefix string, parentIdx int)
	dfs = func(number, relType, linePrefix, connector, childrenLinePrefix string, parentIdx int) {
		idx := len(nodes)
		nodes = append(nodes, familyNode{
			number:    number,
			depth:     depths[number],
			relType:   relType,
			parentIdx: parentIdx,
			prefix:    linePrefix,
			connector: connector,
		})

		if visited[number] {
			return
		}
		visited[number] = true

		var childEdges []domain.FamilyEdge
		for _, e := range parentToChildren[number] {
			// Only follow edges that represent the next hierarchical level
			if nodeSet[e.ChildNumber] && depths[e.ChildNumber] == depths[number]+1 {
				childEdges = append(childEdges, e)
			}
		}
		sort.Slice(childEdges, func(i, j int) bool {
			return childEdges[i].ChildNumber < childEdges[j].ChildNumber
		})

		for i, e := range childEdges {
			isLast := i == len(childEdges)-1
			childConnector := "├─ "
			grandChildrenLinePrefix := childrenLinePrefix + "│  "
			if isLast {
				childConnector = "└─ "
				grandChildrenLinePrefix = childrenLinePrefix + "   "
			}
			dfs(e.ChildNumber, e.RelationType, childrenLinePrefix, childConnector, grandChildrenLinePrefix, idx)
		}
	}

	for _, root := range roots {
		dfs(root, "", "", "", "", -1)
	}

	for i, node := range nodes {
		if p, err := m.repo.GetPatent(m.ctx, m.ProjectID, node.number); err == nil {
			if p.Title != "" {
				nodes[i].title = p.Title
			}
			switch {
			case len(p.GrantDate) >= 4:
				nodes[i].grantYear = p.GrantDate[:4]
			case len(p.PublicationDate) >= 4:
				nodes[i].grantYear = p.PublicationDate[:4]
			}
			nodes[i].importSource = p.ImportSource
		}
	}

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

// familyLevelLabel returns a human-readable depth label relative to the current patent.
func familyLevelLabel(depth int) string {
	switch depth {
	case -1:
		return "Parent"
	case -2:
		return "Grandparent"
	case -3:
		return "Great-grandparent"
	case 1:
		return "Child"
	case 2:
		return "Grandchild"
	case 3:
		return "Great-grandchild"
	}
	if depth < 0 {
		return fmt.Sprintf("Ancestor (%d)", -depth)
	}
	return fmt.Sprintf("Descendant (%d)", depth)
}

// importSourceBadge renders a compact source indicator: [g] or [u].
func importSourceBadge(base lipgloss.Style, source string) string {
	switch source {
	case "uspto":
		return base.Foreground(lipgloss.Color(ColorDepth)).Render("[u]")
	case "google":
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
// j/k move up/down, h moves to parent, l moves to first child.
func (m *Model) viewFamilyOverlay() string {
	nodes := m.buildFamilyTree()

	base := overlayBase()
	subtle := base.Foreground(lipgloss.Color(ColorSubtle))
	accent := base.Foreground(lipgloss.Color(ColorAccentFamily)).Bold(true)
	currentStyle := base.Foreground(lipgloss.Color(ColorAccent)).Bold(true)
	levelStyle := base.Foreground(lipgloss.Color(ColorDepth))
	cursorStyle := base.Foreground(lipgloss.Color(ColorAccentFamily)).Bold(true)

	var b strings.Builder
	b.WriteString(m.renderPopupTitle("Family · "+m.current.Number) + "\n\n")

	if len(nodes) == 0 {
		b.WriteString(subtle.Render("No family relationships defined."))
		b.WriteString("\n\n")
		b.WriteString(subtle.Render(":family parent <number> [type]  ·  :family child <number> [type]"))
		return b.String()
	}

	sel := clamp(m.familySelected, 0, len(nodes)-1)
	window := pageWindow(sel, len(nodes), m.pageSize()-3)

	b.WriteString(subtle.Render(pageStatus(m.text.T(TextValuePageStatus), window)) + "\n\n")

	titleWidth := max(20, m.overlayWidth()-46)

	for i := window.Start; i < window.End; i++ {
		node := nodes[i]
		isCurrent := node.depth == 0
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

		// Current-patent dot marker.
		if isCurrent {
			line.WriteString(accent.Render("● "))
		}

		// Patent number.
		numStyle := base
		if isCurrent {
			numStyle = accent.Underline(true)
		}
		line.WriteString(numStyle.Render(node.number))

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
			line.WriteString(base.Foreground(lipgloss.Color(relColor)).Render("[" + node.relType + "]"))
		}

		// Depth label (omitted for the current patent).
		if !isCurrent {
			line.WriteString(base.Render(" "))
			line.WriteString(levelStyle.Render(familyLevelLabel(node.depth)))
		}

		b.WriteString(line.String() + "\n")
	}

	return b.String()
}

func (m *Model) moveFamilySelection(delta int) *Model {
	nodes := m.buildFamilyTreeFresh()
	m.familyTreeCache = nodes
	m.familyTreeCacheFor = m.current.Number
	if len(nodes) == 0 {
		return m
	}
	m.familySelected = clamp(m.familySelected+delta, 0, len(nodes)-1)
	return m
}

func (m *Model) moveFamilyToParent() *Model {
	nodes := m.buildFamilyTreeFresh()
	m.familyTreeCache = nodes
	m.familyTreeCacheFor = m.current.Number
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
	nodes := m.buildFamilyTreeFresh()
	m.familyTreeCache = nodes
	m.familyTreeCacheFor = m.current.Number
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
	m.familyTreeCacheFor = "" // invalidate: tree changed
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
				m.familyTreeCacheFor = ""
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
				m.familyTreeCacheFor = ""
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
	if targetPatent.Status != domain.CitationStatusStored {
		m.err = fmt.Sprintf("patent %s is not stored in this project (status: %s)", target, targetPatent.Status)
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
	m.familyTreeCacheFor = "" // invalidate: tree changed
	m.message = fmt.Sprintf("family edge added: %s → %s (%s)", parentNumber, childNumber, relType)
	m.mode = viewFamily
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
	currentStatus := m.current.Status
	logger := m.logger

	return m, tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
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
				bundle.Patent.ImportSource = "uspto"
			} else {
				bundle.Patent.ImportSource = "google"
			}
			bundle.Patent.Status = currentStatus

			if len(bundle.FamilyEdges) == 0 {
				p, _ := repo.GetPatent(ctx, projectID, currentNumber)
				source := "google"
				if importSource == config.ImportSourceUSPTO && apiKey != "" {
					source = "uspto"
				}
				return refreshResultMsg{
					patent:  p,
					mode:    viewFamily,
					message: "no family edges found for " + currentNumber,
					action:  "family.refresh",
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
					memberBundle.Patent.ImportSource = "uspto"
				} else {
					memberBundle.Patent.ImportSource = "google"
				}
				memberBundle.Patent.Status = domain.CitationStatusIgnored
				if err := repo.UpsertPatentBundle(ctx, projectID, memberBundle); err != nil {
					logger.Error("family member store failed", "patent", num, "error", err)
					failed++
					continue
				}
				imported++
			}

			bundle.Patent.Status = currentStatus
			_ = repo.UpsertPatentBundle(ctx, projectID, bundle)

			p, err := repo.GetPatent(ctx, projectID, currentNumber)
			if err != nil {
				return refreshResultMsg{err: err}
			}

			source := "google"
			if importSource == config.ImportSourceUSPTO && apiKey != "" {
				source = "uspto"
			}
			msg := fmt.Sprintf("family refresh: %d/%d members imported", imported, len(members))
			if failed > 0 {
				msg += fmt.Sprintf(" (%d failed)", failed)
			}
			return refreshResultMsg{patent: p, mode: viewFamily, message: msg, action: "family.refresh", source: source}
		},
	)
}
