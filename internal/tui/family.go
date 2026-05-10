package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// familyItems returns direct parents and children of the current patent.
func (m Model) familyItems() (parents []domain.FamilyEdge, children []domain.FamilyEdge) {
	parents, children, _ = m.repo.ListFamilyEdges(m.ctx, m.ProjectID, m.current.Number)
	return
}

func (m Model) familyItemCount() int {
	p, c := m.familyItems()
	return len(p) + len(c)
}

// familyEdgeAt returns the edge at logical index i (parents first, then children).
func familyEdgeAt(parents, children []domain.FamilyEdge, i int) (edge domain.FamilyEdge, isParent bool, ok bool) {
	if i < 0 {
		return domain.FamilyEdge{}, false, false
	}
	if i < len(parents) {
		return parents[i], true, true
	}
	j := i - len(parents)
	if j < len(children) {
		return children[j], false, true
	}
	return domain.FamilyEdge{}, false, false
}

// familyLevelLabel returns a human-readable depth label relative to the current patent.
// Negative depth = ancestor direction, positive = descendant direction.
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

// familyRelationColor returns the terminal color code for a family relation type.
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

// viewFamilyOverlay renders the family tree in a two-column layout:
// ancestors on the left, descendants on the right.
func (m Model) viewFamilyOverlay() string {
	allEdges, _ := m.repo.ListAllFamilyEdges(m.ctx, m.ProjectID)

	base := overlayBase()
	subtle := base.Foreground(lipgloss.Color(ColorSubtle))
	accent := base.Foreground(lipgloss.Color(ColorAccentFamily)).Bold(true)
	currentStyle := base.Foreground(lipgloss.Color(ColorAccent)).Bold(true)
	levelStyle := base.Foreground(lipgloss.Color(ColorDepth))

	var b strings.Builder
	b.WriteString(currentStyle.Render(m.current.Number))
	if m.current.Title != "" {
		b.WriteString(subtle.Render(" · " + m.truncate(m.current.Title, 40)))
	}
	b.WriteString("\n")

	if len(allEdges) == 0 {
		b.WriteString("\n")
		b.WriteString(subtle.Render("No family relationships defined."))
		b.WriteString("\n\n")
		b.WriteString(subtle.Render(":family parent <number> [type]  ·  :family child <number> [type]"))
		return b.String()
	}

	// Build adjacency maps
	parentToChildren := map[string][]domain.FamilyEdge{}
	childToParents := map[string][]domain.FamilyEdge{}
	for _, e := range allEdges {
		parentToChildren[e.ParentNumber] = append(parentToChildren[e.ParentNumber], e)
		childToParents[e.ChildNumber] = append(childToParents[e.ChildNumber], e)
	}

	current := m.current.Number

	// BFS up (ancestors, negative depths)
	depths := map[string]int{current: 0}
	queue := []string{current}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, e := range childToParents[n] {
			if _, seen := depths[e.ParentNumber]; !seen {
				depths[e.ParentNumber] = depths[n] - 1
				queue = append(queue, e.ParentNumber)
			}
		}
	}
	// BFS down (descendants, positive depths)
	queue = []string{current}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, e := range parentToChildren[n] {
			if _, seen := depths[e.ChildNumber]; !seen {
				depths[e.ChildNumber] = depths[n] + 1
				queue = append(queue, e.ChildNumber)
			}
		}
	}

	ancestorSet := map[string]bool{}
	descendantSet := map[string]bool{}
	for node, d := range depths {
		if d < 0 {
			ancestorSet[node] = true
		}
		if d > 0 {
			descendantSet[node] = true
		}
	}

	// buildTreeLines renders nodes from nodeSet into a []string, one line per node.
	// rootRelTypes supplies the relation type for each root node.
	buildTreeLines := func(nodeSet map[string]bool, roots []string, rootRelTypes map[string]string) []string {
		var lines []string

		var renderNode func(number, prefix, connector, relType string)
		var renderChildren func(parent, prefix string, visited map[string]bool)

		renderNode = func(number, prefix, connector, relType string) {
			depth := depths[number]
			sp := base.Render(" ")
			line := base.Render(prefix+connector) + base.Render(number)
			line += sp + levelStyle.Render(familyLevelLabel(depth))
			if relType != "" {
				relColor := familyRelationColor(relType)
				line += base.Foreground(lipgloss.Color(relColor)).Render(" [" + relType + "]")
			}
			lines = append(lines, line)
		}

		renderChildren = func(parent, prefix string, visited map[string]bool) {
			var childEdges []domain.FamilyEdge
			for _, e := range parentToChildren[parent] {
				if nodeSet[e.ChildNumber] && !visited[e.ChildNumber] {
					childEdges = append(childEdges, e)
				}
			}
			sort.Slice(childEdges, func(i, j int) bool {
				return childEdges[i].ChildNumber < childEdges[j].ChildNumber
			})
			for i, e := range childEdges {
				isLast := i == len(childEdges)-1
				connector := "├─ "
				if isLast {
					connector = "└─ "
				}
				renderNode(e.ChildNumber, prefix, connector, e.RelationType)
				childPrefix := prefix
				if isLast {
					childPrefix += "   "
				} else {
					childPrefix += "│  "
				}
				visited[e.ChildNumber] = true
				renderChildren(e.ChildNumber, childPrefix, visited)
			}
		}

		for _, root := range roots {
			renderNode(root, "", "", rootRelTypes[root])
			visited := map[string]bool{root: true}
			renderChildren(root, "", visited)
		}
		return lines
	}

	// Ancestor roots: no parent in ancestorSet
	var ancestorRoots []string
	for node := range ancestorSet {
		hasParent := false
		for _, e := range childToParents[node] {
			if ancestorSet[e.ParentNumber] {
				hasParent = true
				break
			}
		}
		if !hasParent {
			ancestorRoots = append(ancestorRoots, node)
		}
	}
	sort.Strings(ancestorRoots)

	// Descendant roots: no parent in descendantSet (direct children of current)
	var descendantRoots []string
	descendantRootRel := map[string]string{}
	for node := range descendantSet {
		hasParent := false
		for _, e := range childToParents[node] {
			if descendantSet[e.ParentNumber] {
				hasParent = true
				break
			}
		}
		if !hasParent {
			descendantRoots = append(descendantRoots, node)
			for _, e := range parentToChildren[current] {
				if e.ChildNumber == node {
					descendantRootRel[node] = e.RelationType
					break
				}
			}
		}
	}
	sort.Strings(descendantRoots)

	leftLines := buildTreeLines(ancestorSet, ancestorRoots, map[string]string{})
	rightLines := buildTreeLines(descendantSet, descendantRoots, descendantRootRel)

	// Two-column layout
	colWidth := max((m.overlayWidth()-6)/2, 20)
	gap := base.Render("  ")
	padLine := func(s string, w int) string {
		v := lipgloss.Width(s)
		if v >= w {
			return s
		}
		return s + base.Render(strings.Repeat(" ", w-v))
	}

	b.WriteString("\n")
	b.WriteString(padLine(accent.Render("Ancestors"), colWidth) + gap + accent.Render("Descendants") + "\n")
	sep := subtle.Render(strings.Repeat("─", colWidth))
	b.WriteString(sep + gap + sep + "\n")

	if len(leftLines) == 0 {
		leftLines = []string{subtle.Render("(none)")}
	}
	if len(rightLines) == 0 {
		rightLines = []string{subtle.Render("(none)")}
	}

	nLines := max(len(leftLines), len(rightLines))
	for i := range nLines {
		left, right := "", ""
		if i < len(leftLines) {
			left = leftLines[i]
		}
		if i < len(rightLines) {
			right = rightLines[i]
		}
		b.WriteString(padLine(left, colWidth) + gap + right + "\n")
	}

	b.WriteString("\n")
	b.WriteString(subtle.Render(keyEnter + " opens · " + keyDelete + " removes · :family parent/child <num> · " + keyRefs + " pull · " + keyEsc + " back"))
	return b.String()
}

func (m Model) moveFamilySelection(delta int) Model {
	total := m.familyItemCount()
	if total == 0 {
		return m
	}
	m.familySelected = clamp(m.familySelected+delta, 0, total-1)
	return m
}

func (m Model) openSelectedFamilyMember() (tea.Model, tea.Cmd) {
	parents, children := m.familyItems()
	edge, _, ok := familyEdgeAt(parents, children, m.familySelected)
	if !ok {
		return m, nil
	}
	var number string
	if edge.ChildNumber == m.current.Number {
		number = edge.ParentNumber
	} else {
		number = edge.ChildNumber
	}
	return m.openPatent(number)
}

func (m Model) removeSelectedFamilyEdge() (tea.Model, tea.Cmd) {
	parents, children := m.familyItems()
	edge, _, ok := familyEdgeAt(parents, children, m.familySelected)
	if !ok {
		return m, nil
	}
	if err := m.repo.RemoveFamilyEdge(m.ctx, m.ProjectID, edge.ParentNumber, edge.ChildNumber); err != nil {
		m.err = err.Error()
		return m, nil
	}
	total := len(parents) + len(children) - 1
	m.familySelected = clamp(m.familySelected, 0, max(0, total-1))
	m.message = fmt.Sprintf("removed family edge: %s ↔ %s", edge.ParentNumber, edge.ChildNumber)
	return m, nil
}

// familyCommand handles :family parent|child|remove|pull sub-commands.
func (m Model) familyCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.err = "usage: :family parent|child <number> [type]  ·  :family remove <number>  ·  :family pull"
		return m, nil
	}
	action := strings.ToLower(args[0])

	if action == "pull" {
		return m.pullFamilyCommand()
	}

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
				m.message = "removed family edge: " + target + " ↔ " + m.current.Number
				return m, nil
			}
		}
		for _, e := range children {
			if e.ChildNumber == target {
				if err := m.repo.RemoveFamilyEdge(m.ctx, m.ProjectID, e.ParentNumber, e.ChildNumber); err != nil {
					m.err = err.Error()
					return m, nil
				}
				m.message = "removed family edge: " + m.current.Number + " ↔ " + target
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
	m.message = fmt.Sprintf("family edge added: %s → %s (%s)", parentNumber, childNumber, relType)
	m.mode = viewFamily
	return m, nil
}

// pullFamilyCommand fetches all direct family members detected on Google Patents,
// stores each as a project patent, then re-upserts the current patent so all
// family edges link up.
func (m Model) pullFamilyCommand() (tea.Model, tea.Cmd) {
	if m.current.Number == "" {
		m.err = "no patent selected"
		return m, nil
	}

	rawURL := m.current.SourceURL
	if rawURL == "" {
		var err error
		rawURL, err = importer.GooglePatentsURL(m.current.Number)
		if err != nil {
			m.err = err.Error()
			return m, nil
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
			// Fetch current patent to get fresh family edges
			bundle, err := importer.ImportGooglePatents(rawURL)
			if err != nil {
				return refreshResultMsg{err: fmt.Errorf("import failed: %w", err)}
			}
			bundle.Patent.Status = currentStatus

			if len(bundle.FamilyEdges) == 0 {
				p, _ := repo.GetPatent(ctx, projectID, currentNumber)
				return refreshResultMsg{
					patent:  p,
					mode:    viewFamily,
					message: "no family edges found on Google Patents for " + currentNumber,
				}
			}

			// Collect unique family member numbers (excluding current)
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
				memberURL, err := importer.GooglePatentsURL(num)
				if err != nil {
					failed++
					continue
				}
				memberBundle, err := importer.ImportGooglePatents(memberURL)
				if err != nil {
					logger.Warn("family member fetch failed", "patent", num, "error", err)
					failed++
					continue
				}
				memberBundle.Patent.Status = domain.CitationStatusIgnored
				if err := repo.UpsertPatentBundle(ctx, projectID, memberBundle); err != nil {
					logger.Error("family member store failed", "patent", num, "error", err)
					failed++
					continue
				}
				imported++
			}

			// Re-upsert current patent so family edges now connect stored members
			bundle.Patent.Status = currentStatus
			_ = repo.UpsertPatentBundle(ctx, projectID, bundle)

			p, err := repo.GetPatent(ctx, projectID, currentNumber)
			if err != nil {
				return refreshResultMsg{err: err}
			}

			msg := fmt.Sprintf("family pull: %d/%d members imported", imported, len(members))
			if failed > 0 {
				msg += fmt.Sprintf(" (%d failed)", failed)
			}
			return refreshResultMsg{patent: p, mode: viewFamily, message: msg}
		},
	)
}

