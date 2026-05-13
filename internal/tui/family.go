package tui

import (
	"context"
	"fmt"
	"strings"

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
	if m.familyTreeCacheFor == m.current.Number && len(m.familyTreeCache) > 0 {
		return m.familyTreeCache
	}
	return m.buildFamilyTreeFresh()
}

const maxFamilyNodes = 250

func (m *Model) buildFamilyTreeFresh() []familyNode {
	allEdges, _ := m.repo.ListAllFamilyEdges(m.ctx, m.ProjectID)
	if len(allEdges) == 0 {
		return nil
	}

	graph := domain.BuildFamilyGraph(m.current.Number, allEdges)
	if graph == nil || len(graph.Nodes) == 0 {
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
	type nodeInfo struct {
		childrenLinePrefix string
	}
	infos := make([]nodeInfo, len(nodes))

	for i := range nodes {
		pIdx := nodes[i].parentIdx
		if pIdx < 0 {
			nodes[i].prefix = ""
			nodes[i].connector = ""
			infos[i].childrenLinePrefix = ""
			continue
		}

		// Is this the last child of its parent?
		isLast := true
		for j := i + 1; j < len(nodes); j++ {
			if nodes[j].parentIdx == pIdx {
				isLast = false
				break
			}
		}

		nodes[i].prefix = infos[pIdx].childrenLinePrefix
		nodes[i].connector = "├─ "
		infos[i].childrenLinePrefix = nodes[i].prefix + "│ "
		if isLast {
			nodes[i].connector = "└─ "
			infos[i].childrenLinePrefix = nodes[i].prefix + "  "
		}
	}

	// 3. Enrich with patent info (use local cache to avoid redundant lookups)
	patentCache := make(map[string]domain.Patent)
	for i := range nodes {
		num := nodes[i].number
		p, ok := patentCache[num]
		if !ok {
			var err error
			p, err = m.repo.GetPatent(m.ctx, m.ProjectID, num)
			if err == nil {
				patentCache[num] = p
				ok = true
			}
		}
		if ok {
			nodes[i].title = p.Title
			if len(p.GrantDate) >= 4 {
				nodes[i].grantYear = p.GrantDate[:4]
			} else if len(p.PublicationDate) >= 4 {
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
// j/k move up/down, h moves to parent, l moves to first child.
func (m *Model) viewFamilyOverlay() string {
	nodes := m.buildFamilyTree()

	base := overlayBase()
	subtle := base.Foreground(lipgloss.Color(ColorSubtle))
	accent := base.Foreground(lipgloss.Color(ColorAccentFamily)).Bold(true)
	currentStyle := base.Foreground(lipgloss.Color(ColorAccent)).Bold(true)
	cursorStyle := base.Foreground(lipgloss.Color(ColorAccentFamily)).Bold(true)

	if len(nodes) == 0 {
		return m.renderPopup("Family · "+m.current.Number, subtle.Render("No family relationships defined."))
	}

	sel := clamp(m.familySelected, 0, len(nodes)-1)
	window := pageWindow(sel, len(nodes), m.pageSize()-4)

	var body strings.Builder
	body.WriteString(subtle.Render(pageStatus(m.text.T(TextValuePageStatus), window)) + "\n\n")

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

		// Depth label (compact relative indicator).
		if !isCurrent {
			relColor := ColorDim
			if node.depth < 0 {
				relColor = ColorDepth // Ancestors
			}
			label := familyLevelLabel(node.depth)
			line.WriteString(base.Render(" "))
			line.WriteString(base.Foreground(lipgloss.Color(relColor)).Render("«" + label + "»"))
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
			line.WriteString(base.Foreground(lipgloss.Color(relColor)).Render("[" + node.relType + "]"))
		}

		body.WriteString(line.String() + "\n")
	}

	return m.renderPopup("Family · "+m.current.Number, body.String())
}

func (m *Model) moveFamilySelection(delta int) *Model {
	nodes := m.buildFamilyTree()
	if len(nodes) == 0 {
		return m
	}
	m.familySelected = clamp(m.familySelected+delta, 0, len(nodes)-1)
	return m
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
				bundle.Patent.ImportSource = ImportSourceUSPTO
			} else {
				bundle.Patent.ImportSource = ImportSourceGoogle
			}
			bundle.Patent.Status = currentStatus

			if len(bundle.FamilyEdges) == 0 {
				p, _ := repo.GetPatent(ctx, projectID, currentNumber)
				source := ImportSourceGoogle
				if importSource == config.ImportSourceUSPTO && apiKey != "" {
					source = ImportSourceUSPTO
				}
				return refreshResultMsg{
					patent:  p,
					mode:    viewFamily,
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

			source := ImportSourceGoogle
			if importSource == config.ImportSourceUSPTO && apiKey != "" {
				source = ImportSourceUSPTO
			}
			msg := fmt.Sprintf("family refresh: %d/%d members imported", imported, len(members))
			if failed > 0 {
				msg += fmt.Sprintf(" (%d failed)", failed)
			}
			return refreshResultMsg{patent: p, mode: viewFamily, message: msg, action: ActivityFamilyRefresh, source: source}
		},
	)
}
