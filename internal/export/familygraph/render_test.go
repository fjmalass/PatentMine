package familygraph

import (
	"strings"
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
)

func sampleResult() proto.FamilyGraphResult {
	root := domain.MustParsePatentNumber("US0000001B2")
	parent := domain.MustParsePatentNumber("EP0000002A1")
	child := domain.MustParsePatentNumber("JP0000003A1")
	return proto.FamilyGraphResult{
		Root: root,
		Nodes: []proto.FamilyGraphNode{
			{Patent: domain.PatentRow{Number: root, DisplayNumber: root, Title: "Root patent"}, Depth: 0},
			{Patent: domain.PatentRow{Number: parent, DisplayNumber: parent, Title: "Parent patent"}, Depth: 1},
			{Patent: domain.PatentRow{Number: child, DisplayNumber: child, Title: ""}, Depth: 1},
		},
		Edges: []proto.FamilyGraphEdge{
			{Parent: parent, Child: root},
			{Parent: root, Child: child, Inconsistent: true},
		},
	}
}

func TestMermaidRendersNodesEdgesAndCycleStyle(t *testing.T) {
	out := Mermaid(sampleResult())
	for _, want := range []string{
		"flowchart TD",
		"p_us_0000001_b2((\"US0000001B2<br/>Root patent\"))",
		"p_ep_0000002_a1[\"EP0000002A1<br/>Parent patent\"]",
		"p_jp_0000003_a1[\"JP0000003A1\"]",
		"p_ep_0000002_a1 --> p_us_0000001_b2",
		"p_us_0000001_b2 -.-> p_jp_0000003_a1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("mermaid missing %q\n%s", want, out)
		}
	}
	// The cross/back edge (index 1) is dashed and red.
	if !strings.Contains(out, "linkStyle 1 stroke:"+cycleColor) {
		t.Fatalf("mermaid missing cycle linkStyle\n%s", out)
	}
}

func TestDOTRendersDigraphRootAndCycleStyle(t *testing.T) {
	out := DOT(sampleResult())
	for _, want := range []string{
		"digraph family {",
		"shape=doublecircle",
		`"p_ep_0000002_a1" -> "p_us_0000001_b2";`,
		`color="` + cycleColor + `"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dot missing %q\n%s", want, out)
		}
	}
}
