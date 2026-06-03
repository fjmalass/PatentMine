package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
)

// ExportFamilyGraph generates the family graph, formats it as Mermaid, and saves it.
// If the path ends in .pdf, it renders the diagram via mermaid.ink and saves the PDF,
// and also saves a matching .txt ASCII preview.
func (e *Engine) ExportFamilyGraph(ctx context.Context, p proto.FamilyGraphParams, path string) (proto.FamilyGraphExportResult, error) {
	res, err := e.FamilyGraph(ctx, p)
	if err != nil {
		return proto.FamilyGraphExportResult{}, err
	}

	if len(res.Nodes) == 0 {
		return proto.FamilyGraphExportResult{}, fmt.Errorf("family graph is empty")
	}

	// Generate mermaid graph text
	mermaidText := e.familyGraphMermaidText(res)

	if path == "" {
		return proto.FamilyGraphExportResult{
			Mermaid: mermaidText,
		}, nil
	}

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return proto.FamilyGraphExportResult{}, err
		}
	}

	// Build the chronology/timeline of events
	var events []timelineEvent
	for _, node := range res.Nodes {
		pat, err := e.repo.Patent(ctx, node.Patent.Number)
		if err != nil {
			continue // skip stubs/missing patents
		}
		title := pat.Title
		if title == "" {
			title = "(stub)"
		}
		var numStr string
		if !pat.DisplayNumber.IsZero() {
			numStr = exportDisplayString(pat.DisplayNumber)
		} else {
			numStr = exportDisplayString(pat.Number)
		}

		if !pat.ApplicationDate.IsZero() {
			events = append(events, timelineEvent{
				Date:        pat.ApplicationDate,
				Year:        pat.ApplicationDate.Year(),
				PatentNum:   numStr,
				EventType:   "Filed",
				PatentTitle: title,
			})
		}
		if !pat.PublicationDate.IsZero() {
			events = append(events, timelineEvent{
				Date:        pat.PublicationDate,
				Year:        pat.PublicationDate.Year(),
				PatentNum:   numStr,
				EventType:   "Published",
				PatentTitle: title,
			})
		}
		if !pat.GrantDate.IsZero() {
			events = append(events, timelineEvent{
				Date:        pat.GrantDate,
				Year:        pat.GrantDate.Year(),
				PatentNum:   numStr,
				EventType:   "Issued",
				PatentTitle: title,
			})
		}
		if !pat.ExpirationDate.IsZero() {
			events = append(events, timelineEvent{
				Date:        pat.ExpirationDate,
				Year:        pat.ExpirationDate.Year(),
				PatentNum:   numStr,
				EventType:   "Expired",
				PatentTitle: title,
			})
		}
	}

	// Sort events chronologically
	slices.SortFunc(events, func(a, b timelineEvent) int {
		if !a.Date.Equal(b.Date) {
			if a.Date.Before(b.Date) {
				return -1
			}
			return 1
		}
		return strings.Compare(a.PatentNum, b.PatentNum)
	})

	ext := strings.ToLower(filepath.Ext(path))
	asciiText := e.FamilyGraphASCII(res)

	// Format ASCII timeline
	var timelineBuilder strings.Builder
	timelineBuilder.WriteString("\nFamily Timeline:\n")
	if len(events) == 0 {
		timelineBuilder.WriteString("  (no date information available)\n")
	} else {
		for _, ev := range events {
			fmt.Fprintf(&timelineBuilder, "  %s : %s %s (%s)\n",
				ev.Date.Format("2006-01-02"),
				ev.PatentNum,
				ev.EventType,
				ev.PatentTitle,
			)
		}
	}
	asciiText = asciiText + timelineBuilder.String()

	asciiPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".txt"
	if err := os.WriteFile(asciiPath, []byte(asciiText), 0644); err != nil {
		return proto.FamilyGraphExportResult{}, fmt.Errorf("write ascii preview: %w", err)
	}

	// Write timeline mermaid diagram
	timelinePath := strings.TrimSuffix(path, filepath.Ext(path)) + ".timeline.mermaid"
	timelineMermaid := e.familyGraphMermaidTimeline(res, events)
	if err := os.WriteFile(timelinePath, []byte(timelineMermaid), 0644); err != nil {
		return proto.FamilyGraphExportResult{}, fmt.Errorf("write timeline mermaid: %w", err)
	}

	if ext == ".pdf" {
		data := map[string]any{
			"code":    mermaidText,
			"mermaid": map[string]any{"theme": "default"},
		}
		jsonBytes, err := json.Marshal(data)
		if err != nil {
			return proto.FamilyGraphExportResult{}, err
		}
		encoded := base64.URLEncoding.EncodeToString(jsonBytes)
		encoded = strings.TrimRight(encoded, "=")
		url := "https://mermaid.ink/pdf/base64:" + encoded

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return proto.FamilyGraphExportResult{}, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return proto.FamilyGraphExportResult{}, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			errMsg := string(bodyBytes)
			if errMsg == "" {
				errMsg = resp.Status
			}
			return proto.FamilyGraphExportResult{}, fmt.Errorf("mermaid.ink returned status %d: %s", resp.StatusCode, errMsg)
		}

		outFile, err := os.Create(path)
		if err != nil {
			return proto.FamilyGraphExportResult{}, err
		}
		defer outFile.Close()

		if _, err := io.Copy(outFile, resp.Body); err != nil {
			return proto.FamilyGraphExportResult{}, err
		}

		// Fetch timeline PDF
		timelinePDFPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".timeline.pdf"
		timelineData := map[string]any{
			"code":    timelineMermaid,
			"mermaid": map[string]any{"theme": "default"},
		}
		timelineJSONBytes, err := json.Marshal(timelineData)
		if err == nil {
			timelineEncoded := base64.URLEncoding.EncodeToString(timelineJSONBytes)
			timelineEncoded = strings.TrimRight(timelineEncoded, "=")
			timelineURL := "https://mermaid.ink/pdf/base64:" + timelineEncoded

			timelineReq, err := http.NewRequestWithContext(ctx, "GET", timelineURL, nil)
			if err == nil {
				timelineReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
				timelineResp, err := client.Do(timelineReq)
				if err == nil && timelineResp.StatusCode == http.StatusOK {
					defer timelineResp.Body.Close()
					timelineOutFile, err := os.Create(timelinePDFPath)
					if err == nil {
						io.Copy(timelineOutFile, timelineResp.Body)
						timelineOutFile.Close()
					}
				}
			}
		}

		return proto.FamilyGraphExportResult{
			Path:      path,
			ASCIIPath: asciiPath,
		}, nil
	}

	// For other extensions, write the mermaid file
	if err := os.WriteFile(path, []byte(mermaidText), 0644); err != nil {
		return proto.FamilyGraphExportResult{}, err
	}

	return proto.FamilyGraphExportResult{
		Path:      path,
		ASCIIPath: asciiPath,
	}, nil
}

func (e *Engine) familyGraphMermaidText(res proto.FamilyGraphResult) string {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	for _, node := range res.Nodes {
		id := mermaidNodeID(node.Patent.Number)
		shapeOpen, shapeClose := "[\"", "\"]"
		if node.Patent.Number == res.Root {
			shapeOpen, shapeClose = "((\"", "\"))"
		}
		fmt.Fprintf(&b, "  %s%s%s%s\n", id, shapeOpen, mermaidNodeText(node), shapeClose)
	}

	reduced := transitivelyReduced(res.Edges)

	for _, edge := range res.Edges {
		key := edge.Parent.Normalized() + "\x00" + edge.Child.Normalized()
		if !reduced[key] {
			continue
		}
		parentLabel := mermaidNodeID(edge.Parent)
		childLabel := mermaidNodeID(edge.Child)
		arrow := " --> "
		if edge.Inconsistent {
			arrow = " -.-> "
		}
		if edge.RelationType != "" {
			rel := strings.ToUpper(edge.RelationType)
			if edge.Inconsistent {
				arrow = " -.->|" + rel + "| "
			} else {
				arrow = " -->|" + rel + "| "
			}
		}
		b.WriteString("  ")
		b.WriteString(parentLabel)
		b.WriteString(arrow)
		b.WriteString(childLabel)
		b.WriteByte('\n')
	}
	return b.String()
}

// FamilyGraphASCII formats the family graph result as a plain ASCII text diagram.
func (e *Engine) FamilyGraphASCII(res proto.FamilyGraphResult) string {
	if len(res.Nodes) == 0 {
		return ""
	}

	nodeMap := make(map[domain.PatentNumber]*proto.FamilyGraphNode, len(res.Nodes))
	for i := range res.Nodes {
		nodeMap[res.Nodes[i].Patent.Number] = &res.Nodes[i]
	}

	occurrences := make(map[domain.PatentNumber]int)
	nodeLabels := make(map[domain.PatentNumber]string, len(res.Nodes))
	relTypes := make(map[string]string)
	for _, edge := range res.Edges {
		key := edge.Parent.Normalized() + "\x00" + edge.Child.Normalized()
		relTypes[key] = edge.RelationType
	}
	depthCounts := make(map[int]int)
	crossEdges := 0
	depthByNumber := make(map[domain.PatentNumber]int, len(res.Nodes))

	for i, node := range res.Nodes {
		nodeLabels[node.Patent.Number] = fmt.Sprintf("N%02d", i+1)
		depthByNumber[node.Patent.Number] = node.Depth
		depthCounts[node.Depth]++
	}

	parentRefs := make(map[domain.PatentNumber][]string, len(res.Nodes))
	childRefs := make(map[domain.PatentNumber][]string, len(res.Nodes))
	for _, edge := range res.Edges {
		parentLabel := nodeLabels[edge.Parent]
		childLabel := nodeLabels[edge.Child]
		if parentLabel == "" || childLabel == "" {
			continue
		}
		childRefs[edge.Parent] = appendUniqueString(childRefs[edge.Parent], childLabel)
		parentRefs[edge.Child] = appendUniqueString(parentRefs[edge.Child], parentLabel)
		if depthByNumber[edge.Child] != depthByNumber[edge.Parent]+1 {
			crossEdges++
		}
	}

	type asciiRow struct {
		isHeader   bool
		headerText string
		node       *proto.FamilyGraphNode
		indent     string
		isCycle    bool
		isCrossRef bool
		parent     domain.PatentNumber
	}

	var rows []asciiRow

	// Root Section
	rows = append(rows, asciiRow{isHeader: true, headerText: "Root Patent:"})
	rootNode := nodeMap[res.Root]
	if rootNode != nil {
		rows = append(rows, asciiRow{
			node:   rootNode,
			indent: "  ● ",
		})
	}

	// Parents Section (Predecessors)
	hasParents := rootNode != nil && len(rootNode.Parents) > 0
	if hasParents {
		rows = append(rows, asciiRow{isHeader: true, headerText: ""})
		rows = append(rows, asciiRow{isHeader: true, headerText: "Parents (Predecessors):"})

		parentsDAG := make(map[domain.PatentNumber]bool)
		var collect func(num domain.PatentNumber)
		collect = func(num domain.PatentNumber) {
			if parentsDAG[num] {
				return
			}
			parentsDAG[num] = true
			node := nodeMap[num]
			if node == nil {
				return
			}
			for _, p := range node.Parents {
				collect(p)
			}
		}
		for _, p := range rootNode.Parents {
			collect(p)
		}

		var sources []domain.PatentNumber
		for num := range parentsDAG {
			node := nodeMap[num]
			if node == nil || node.Patent.FetchState == domain.FetchStub {
				continue
			}
			hasParentsInDAG := false
			for _, p := range node.Parents {
				pNode := nodeMap[p]
				if pNode != nil && pNode.Patent.FetchState != domain.FetchStub {
					hasParentsInDAG = true
					break
				}
			}
			if !hasParentsInDAG {
				sources = append(sources, num)
			}
		}

		slices.SortFunc(sources, func(a, b domain.PatentNumber) int {
			if cmp := strings.Compare(a.Country, b.Country); cmp != 0 {
				return cmp
			}
			return strings.Compare(a.Normalized(), b.Normalized())
		})

		childrenInDAG := make(map[domain.PatentNumber][]domain.PatentNumber)
		for num := range parentsDAG {
			node := nodeMap[num]
			if node == nil || node.Patent.FetchState == domain.FetchStub {
				continue
			}
			for _, p := range node.Parents {
				if parentsDAG[p] {
					childrenInDAG[p] = append(childrenInDAG[p], num)
				}
			}
		}
		for _, p := range rootNode.Parents {
			if parentsDAG[p] {
				childrenInDAG[p] = append(childrenInDAG[p], res.Root)
			}
		}

		reachable := func(start, target domain.PatentNumber) bool {
			visited := map[domain.PatentNumber]bool{start: true}
			queue := []domain.PatentNumber{start}
			for len(queue) > 0 {
				curr := queue[0]
				queue = queue[1:]
				if curr == target && curr != start {
					return true
				}
				for _, child := range childrenInDAG[curr] {
					if !visited[child] {
						visited[child] = true
						queue = append(queue, child)
					}
				}
			}
			return false
		}

		reducedChildren := make(map[domain.PatentNumber][]domain.PatentNumber)
		for u, children := range childrenInDAG {
			var kept []domain.PatentNumber
			for _, v := range children {
				hasLongerPath := false
				for _, w := range children {
					if w == v {
						continue
					}
					if reachable(w, v) {
						hasLongerPath = true
						break
					}
				}
				if !hasLongerPath {
					kept = append(kept, v)
				}
			}
			reducedChildren[u] = kept
		}
		childrenInDAG = reducedChildren

		for p := range childrenInDAG {
			slices.SortFunc(childrenInDAG[p], func(a, b domain.PatentNumber) int {
				if cmp := strings.Compare(a.Country, b.Country); cmp != 0 {
					return cmp
				}
				return strings.Compare(a.Normalized(), b.Normalized())
			})
		}

		visitedParents := make(map[domain.PatentNumber]bool)
		pathParents := []domain.PatentNumber{res.Root}
		var walkParentsDAG func(domain.PatentNumber, domain.PatentNumber, string, bool, []domain.PatentNumber)
		walkParentsDAG = func(num domain.PatentNumber, parent domain.PatentNumber, indent string, isLast bool, path []domain.PatentNumber) {
			if num == res.Root {
				return
			}
			node := nodeMap[num]
			if node == nil || node.Patent.FetchState == domain.FetchStub {
				return
			}
			prefix := "├── "
			if isLast {
				prefix = "└── "
			}
			fullIndent := indent + prefix

			if slices.Contains(path, num) {
				rows = append(rows, asciiRow{
					node:    node,
					indent:  fullIndent,
					isCycle: true,
					parent:  parent,
				})
				return
			}
			if visitedParents[num] {
				rows = append(rows, asciiRow{
					node:       node,
					indent:     fullIndent,
					isCrossRef: true,
					parent:  parent,
				})
				return
			}
			visitedParents[num] = true
			rows = append(rows, asciiRow{
				node:   node,
				indent: fullIndent,
				parent:  parent,
			})

			children := childrenInDAG[num]
			var childrenToWalk []domain.PatentNumber
			for _, child := range children {
				if child != res.Root {
					childrenToWalk = append(childrenToWalk, child)
				}
			}
			if len(childrenToWalk) > 0 {
				nextIndent := indent
				if isLast {
					nextIndent += "    "
				} else {
					nextIndent += "│   "
				}
				newPath := append(path, num)
				for i, child := range childrenToWalk {
					walkParentsDAG(child, num, nextIndent, i == len(childrenToWalk)-1, newPath)
				}
			}
		}

		for i, src := range sources {
			walkParentsDAG(src, domain.PatentNumber{}, "  ", i == len(sources)-1, pathParents)
		}
	}

	// Children Section (Successors)
	hasChildren := rootNode != nil && len(rootNode.Children) > 0
	if hasChildren {
		rows = append(rows, asciiRow{isHeader: true, headerText: ""})
		rows = append(rows, asciiRow{isHeader: true, headerText: "Children (Successors):"})

		childrenDAG := make(map[domain.PatentNumber]bool)
		var collect func(num domain.PatentNumber)
		collect = func(num domain.PatentNumber) {
			if childrenDAG[num] {
				return
			}
			childrenDAG[num] = true
			node := nodeMap[num]
			if node == nil {
				return
			}
			for _, c := range node.Children {
				collect(c)
			}
		}
		for _, c := range rootNode.Children {
			collect(c)
		}

		descendantChildren := make(map[domain.PatentNumber][]domain.PatentNumber)
		for num := range childrenDAG {
			node := nodeMap[num]
			if node == nil || node.Patent.FetchState == domain.FetchStub {
				continue
			}
			for _, c := range node.Children {
				if childrenDAG[c] {
					descendantChildren[num] = append(descendantChildren[num], c)
				}
			}
		}
		for _, c := range rootNode.Children {
			if childrenDAG[c] {
				descendantChildren[res.Root] = append(descendantChildren[res.Root], c)
			}
		}

		reachableDesc := func(start, target domain.PatentNumber) bool {
			visited := map[domain.PatentNumber]bool{start: true}
			queue := []domain.PatentNumber{start}
			for len(queue) > 0 {
				curr := queue[0]
				queue = queue[1:]
				if curr == target && curr != start {
					return true
				}
				for _, child := range descendantChildren[curr] {
					if !visited[child] {
						visited[child] = true
						queue = append(queue, child)
					}
				}
			}
			return false
		}

		reducedDescendants := make(map[domain.PatentNumber][]domain.PatentNumber)
		for u, children := range descendantChildren {
			var kept []domain.PatentNumber
			for _, v := range children {
				hasLongerPath := false
				for _, w := range children {
					if w == v {
						continue
					}
					if reachableDesc(w, v) {
						hasLongerPath = true
						break
					}
				}
				if !hasLongerPath {
					kept = append(kept, v)
				}
			}
			reducedDescendants[u] = kept
		}
		descendantChildren = reducedDescendants

		for p := range descendantChildren {
			slices.SortFunc(descendantChildren[p], func(a, b domain.PatentNumber) int {
				if cmp := strings.Compare(a.Country, b.Country); cmp != 0 {
					return cmp
				}
				return strings.Compare(a.Normalized(), b.Normalized())
			})
		}

		visitedChildren := make(map[domain.PatentNumber]bool)
		pathChildren := []domain.PatentNumber{res.Root}
		rootChildren := descendantChildren[res.Root]
		var walkChildrenDAG func(domain.PatentNumber, domain.PatentNumber, string, bool, []domain.PatentNumber)
		walkChildrenDAG = func(num domain.PatentNumber, parent domain.PatentNumber, indent string, isLast bool, path []domain.PatentNumber) {
			node := nodeMap[num]
			if node == nil || node.Patent.FetchState == domain.FetchStub {
				return
			}
			prefix := "├── "
			if isLast {
				prefix = "└── "
			}
			fullIndent := indent + prefix

			if slices.Contains(path, num) {
				rows = append(rows, asciiRow{
					node:    node,
					indent:  fullIndent,
					isCycle: true,
					parent:  parent,
				})
				return
			}
			if visitedChildren[num] {
				rows = append(rows, asciiRow{
					node:       node,
					indent:     fullIndent,
					isCrossRef: true,
					parent:  parent,
				})
				return
			}
			visitedChildren[num] = true
			rows = append(rows, asciiRow{
				node:   node,
				indent: fullIndent,
				parent:  parent,
			})

			children := descendantChildren[num]
			if len(children) > 0 {
				nextIndent := indent
				if isLast {
					nextIndent += "    "
				} else {
					nextIndent += "│   "
				}
				newPath := append(path, num)
				for i, child := range children {
					walkChildrenDAG(child, num, nextIndent, i == len(children)-1, newPath)
				}
			}
		}

		for i, child := range rootChildren {
			walkChildrenDAG(child, res.Root, "  ", i == len(rootChildren)-1, pathChildren)
		}
	}

	for _, r := range rows {
		if !r.isHeader && r.node != nil {
			occurrences[r.node.Patent.Number]++
		}
	}

	headStrings := make(map[int]string)
	maxHeadWidth := 0
	for idx, r := range rows {
		if r.isHeader || r.node == nil {
			continue
		}
		node := *r.node
		number := node.Patent.DisplayNumber
		if number.IsZero() {
			number = node.Patent.Number
		}

		relTypeBracket := ""
		if !r.parent.IsZero() {
			key := r.parent.Normalized() + "\x00" + node.Patent.Number.Normalized()
			if relType, ok := relTypes[key]; ok && relType != "" {
				relTypeBracket = "[" + relType + "] "
			}
		}

		labelStr := ""
		if r.isCycle {
			label := nodeLabels[node.Patent.Number]
			if label == "" {
				label = "N??"
			}
			labelStr = fmt.Sprintf(" -> %s (Cycle)", label)
		} else if r.isCrossRef {
			label := nodeLabels[node.Patent.Number]
			if label == "" {
				label = "N??"
			}
			labelStr = fmt.Sprintf(" -> %s (Cross-Ref)", label)
		} else {
			if occurrences[node.Patent.Number] > 1 {
				label := nodeLabels[node.Patent.Number]
				labelStr = " [" + label + "]"
			}
		}

		head := fmt.Sprintf("  %s%s%s%s", r.indent, relTypeBracket, exportDisplayString(number), labelStr)
		headStrings[idx] = head
		if len(head) > maxHeadWidth {
			maxHeadWidth = len(head)
		}
	}

	var b strings.Builder

	// Write summary line
	summaryParts := []string{
		fmt.Sprintf("root:%s", res.Root.String()),
		fmt.Sprintf("depth:%d/10", res.Depth),
		fmt.Sprintf("nodes:%d", len(res.Nodes)),
		fmt.Sprintf("edges:%d", len(res.Edges)),
	}
	if crossEdges > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("cross:%d", crossEdges))
	}
	if res.Truncated {
		summaryParts = append(summaryParts, "truncated")
	}
	if res.HiddenByCountry > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("hidden:%d", res.HiddenByCountry))
	}
	if res.InconsistencyCount > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("inconsistent:%d", res.InconsistencyCount))
	}
	b.WriteString(strings.Join(summaryParts, "  "))
	b.WriteString("\n")

	// Write filter line/legend
	b.WriteString(" depth: use [ ] or -/+  ·  y: copy Mermaid  ·  legend: R=root M=merge B=branch X=cross-edge\n")
	b.WriteString("─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────\n")

	// Write rows
	for idx, r := range rows {
		if r.isHeader {
			b.WriteString(r.headerText)
			b.WriteString("\n")
			continue
		}
		if r.node == nil {
			b.WriteString("\n")
			continue
		}

		node := *r.node
		head := headStrings[idx]
		headPadded := padRight(head, maxHeadWidth)

		title := strings.TrimSpace(node.Patent.Title)
		if title == "" {
			title = "(stub)"
		}

		countryPadded := padRight(countryOrDash(node.Patent.Number.Country), 2)
		statePadded := padRight(fetchStateAscii(node.Patent), 2)

		line := fmt.Sprintf("%s  %s  %s  %s",
			headPadded,
			countryPadded,
			statePadded,
			title,
		)
		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

func padRight(s string, length int) string {
	if len(s) >= length {
		return s
	}
	return s + strings.Repeat(" ", length-len(s))
}

func countryOrDash(c string) string {
	if c == "" {
		return "--"
	}
	return c
}

func fetchStateAscii(row domain.PatentRow) string {
	if row.ReviewState != "" {
		switch row.ReviewState {
		case domain.ReviewStateUnknown:
			return "?"
		case domain.ReviewStateUnderReview:
			return "⚙"
		case domain.ReviewStateActive:
			return "✓"
		case domain.ReviewStateIgnored:
			return "✗"
		case domain.ReviewStateDeleted:
			return "✗"
		case domain.ReviewStateNeedsTriage:
			return "?"
		}
	}
	switch row.FetchState {
	case domain.FetchCached:
		return "✓"
	case domain.FetchStub:
		return "?"
	default:
		return " "
	}
}

func appendUniqueString(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}

func transitivelyReduced(edges []proto.FamilyGraphEdge) map[string]bool {
	adj := make(map[domain.PatentNumber][]domain.PatentNumber)
	for _, edge := range edges {
		adj[edge.Parent] = append(adj[edge.Parent], edge.Child)
	}

	isReachableWithoutDirect := func(u, v domain.PatentNumber) bool {
		visited := make(map[domain.PatentNumber]bool)
		visited[u] = true
		queue := []domain.PatentNumber{}
		for _, child := range adj[u] {
			if child != v {
				queue = append(queue, child)
				visited[child] = true
			}
		}

		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]
			if curr == v {
				return true
			}
			for _, neighbor := range adj[curr] {
				if !visited[neighbor] {
					visited[neighbor] = true
					queue = append(queue, neighbor)
				}
			}
		}
		return false
	}

	kept := make(map[string]bool)
	for _, edge := range edges {
		if !isReachableWithoutDirect(edge.Parent, edge.Child) {
			key := edge.Parent.Normalized() + "\x00" + edge.Child.Normalized()
			kept[key] = true
		}
	}
	return kept
}

func mermaidNodeText(node proto.FamilyGraphNode) string {
	number := node.Patent.DisplayNumber
	if number.IsZero() {
		number = node.Patent.Number
	}
	title := strings.TrimSpace(node.Patent.Title)
	if title == "" {
		return mermaidEscape(exportDisplayString(number))
	}
	if len(title) > 48 {
		title = strings.TrimSpace(title[:45]) + "..."
	}
	return mermaidEscape(exportDisplayString(number) + "<br/>" + title)
}


func mermaidNodeID(number domain.PatentNumber) string {
	parts := []string{strings.ToLower(number.Country), strings.ToLower(number.Serial)}
	if number.Kind != "" {
		parts = append(parts, strings.ToLower(number.Kind))
	}
	return "p_" + strings.Join(parts, "_")
}

func mermaidEscape(text string) string {
	replacer := strings.NewReplacer(
		`\\`, `\\\\`,
		`"`, `\\"`,
	)
	return replacer.Replace(text)
}

type timelineEvent struct {
	Date        time.Time
	Year        int
	PatentNum   string
	EventType   string // "Filed", "Published", "Issued", "Expired"
	PatentTitle string
}

func (e *Engine) familyGraphMermaidTimeline(res proto.FamilyGraphResult, events []timelineEvent) string {
	var b strings.Builder
	b.WriteString("timeline\n")
	b.WriteString("    title Patent Family Timeline\n")

	// Group events by year
	eventsByYear := make(map[int][]timelineEvent)
	var years []int
	for _, ev := range events {
		if len(eventsByYear[ev.Year]) == 0 {
			years = append(years, ev.Year)
		}
		eventsByYear[ev.Year] = append(eventsByYear[ev.Year], ev)
	}
	slices.Sort(years)

	for _, yr := range years {
		evs := eventsByYear[yr]
		b.WriteString(fmt.Sprintf("    %d : ", yr))
		var evStrings []string
		for _, ev := range evs {
			evStrings = append(evStrings, fmt.Sprintf("%s %s", ev.PatentNum, ev.EventType))
		}
		b.WriteString(strings.Join(evStrings, " : "))
		b.WriteByte('\n')
	}
	return b.String()
}

func exportDisplayString(n domain.PatentNumber) string {
	if n.IsZero() {
		return ""
	}
	emoji := "🖋️"
	if n.IsGrant() {
		emoji = "🏆"
	} else if n.IsPublication() {
		emoji = "📄"
	}
	return n.DisplayString() + " " + emoji
}
