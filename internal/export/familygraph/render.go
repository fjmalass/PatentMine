// Package familygraph renders a bounded patent family DAG (parent/child
// continuity graph) to diagram source. The functions are pure over a
// proto.FamilyGraphResult so the daemon, the REST API, and the TUI all produce
// identical Mermaid/Graphviz output from the same graph data.
package familygraph

import (
	"fmt"
	"strings"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
)

// Diagram styling constants. Declared once so both renderers and any caller
// reference the same colors; cross/back edges (cycles, depth-inconsistent
// links) are drawn dashed and red and the root node is highlighted.
const (
	mermaidRootClass = "root"
	cycleColor       = "#d9534f"
	rootFill         = "#cde4ff"
	maxTitleRunes    = 48
	truncatedAtRunes = 45
)

// Mermaid renders the family graph as a Mermaid flowchart (TD). Cross/back
// edges are dashed red; the root node is a highlighted double circle.
func Mermaid(res proto.FamilyGraphResult) string {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	for _, node := range res.Nodes {
		id := nodeID(node.Patent.Number)
		shapeOpen, shapeClose := "[\"", "\"]"
		if node.Patent.Number == res.Root {
			shapeOpen, shapeClose = "((\"", "\"))"
		}
		fmt.Fprintf(&b, "  %s%s%s%s\n", id, shapeOpen, mermaidNodeText(node), shapeClose)
		if node.Patent.Number == res.Root {
			fmt.Fprintf(&b, "  class %s %s\n", id, mermaidRootClass)
		}
	}
	var cycleEdges []int
	for i, edge := range res.Edges {
		arrow := " --> "
		if edge.Inconsistent {
			arrow = " -.-> "
			cycleEdges = append(cycleEdges, i)
		}
		fmt.Fprintf(&b, "  %s%s%s\n", nodeID(edge.Parent), arrow, nodeID(edge.Child))
	}
	fmt.Fprintf(&b, "  classDef %s fill:%s,stroke:#333,stroke-width:2px;\n", mermaidRootClass, rootFill)
	for _, i := range cycleEdges {
		fmt.Fprintf(&b, "  linkStyle %d stroke:%s,stroke-dasharray:4;\n", i, cycleColor)
	}
	return b.String()
}

// DOT renders the same family graph as Graphviz DOT, with cross/back edges
// drawn dashed and red so cycles are visible in a laid-out render.
func DOT(res proto.FamilyGraphResult) string {
	var b strings.Builder
	b.WriteString("digraph family {\n")
	b.WriteString("  rankdir=TB;\n")
	b.WriteString("  node [shape=box, style=rounded];\n")
	for _, node := range res.Nodes {
		id := dotNodeID(node.Patent.Number)
		attrs := fmt.Sprintf("label=\"%s\"", dotNodeLabel(node))
		if node.Patent.Number == res.Root {
			attrs += fmt.Sprintf(", shape=doublecircle, style=filled, fillcolor=\"%s\"", rootFill)
		}
		fmt.Fprintf(&b, "  %s [%s];\n", id, attrs)
	}
	for _, edge := range res.Edges {
		attrs := ""
		if edge.Inconsistent {
			attrs = fmt.Sprintf(" [style=dashed, color=\"%s\"]", cycleColor)
		}
		fmt.Fprintf(&b, "  %s -> %s%s;\n", dotNodeID(edge.Parent), dotNodeID(edge.Child), attrs)
	}
	b.WriteString("}\n")
	return b.String()
}

func dotNodeID(number domain.PatentNumber) string {
	return "\"" + nodeID(number) + "\""
}

// dotNodeLabel builds a Graphviz label (number on the first line, truncated
// title on the second) with DOT-special characters escaped.
func dotNodeLabel(node proto.FamilyGraphNode) string {
	number := node.Patent.DisplayNumber
	if number.IsZero() {
		number = node.Patent.Number
	}
	label := number.String()
	if title := strings.TrimSpace(node.Patent.Title); title != "" {
		if len(title) > maxTitleRunes {
			title = strings.TrimSpace(title[:truncatedAtRunes]) + "..."
		}
		label += `\n` + title
	}
	return strings.NewReplacer(`"`, `\"`).Replace(label)
}

func mermaidNodeText(node proto.FamilyGraphNode) string {
	number := node.Patent.DisplayNumber
	if number.IsZero() {
		number = node.Patent.Number
	}
	title := strings.TrimSpace(node.Patent.Title)
	if title == "" {
		return mermaidEscape(number.String())
	}
	if len(title) > maxTitleRunes {
		title = strings.TrimSpace(title[:truncatedAtRunes]) + "..."
	}
	return mermaidEscape(number.String() + "<br/>" + title)
}

func nodeID(number domain.PatentNumber) string {
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
