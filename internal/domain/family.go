package domain

import (
	"sort"
)

// FamilyTreeNode represents a node in a patent family graph.
type FamilyTreeNode struct {
	Number       PatentNumber
	Depth        int    // relative to the "current" node if applicable, or absolute from roots
	RelationType string // relation from parent to this node
	ParentIdx    int    // index of parent in the flat list, -1 if root
}

const MaxFamilyTreeNodes = 250

// FamilyGraph represents a processed patent family graph.
type FamilyGraph struct {
	Target    PatentNumber
	Nodes     []FamilyTreeNode
	Reachable map[PatentNumber]bool
	Depths    map[PatentNumber]int
}

// BuildFamilyGraph processes a connected component of the family tree containing the target patent.
func BuildFamilyGraph(target PatentNumber, allEdges []FamilyEdge) *FamilyGraph {
	if len(allEdges) == 0 || target == "" {
		return &FamilyGraph{Target: target}
	}

	parentToChildren := map[PatentNumber][]FamilyEdge{}
	childToParents := map[PatentNumber][]FamilyEdge{}

	for _, e := range allEdges {
		parentToChildren[e.ParentNumber] = append(parentToChildren[e.ParentNumber], e)
		childToParents[e.ChildNumber] = append(childToParents[e.ChildNumber], e)
	}

	// 1. Find reachable nodes (connected component containing 'target')
	reachable := map[PatentNumber]bool{target: true}
	q := []PatentNumber{target}
	for i := 0; i < len(q); i++ {
		if len(reachable) >= MaxFamilyTreeNodes {
			break
		}
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

	// 2. Compute depths (shortest path from roots or target)
	depths := make(map[PatentNumber]int, len(reachable))
	inDegree := make(map[PatentNumber]int, len(reachable))

	for node := range reachable {
		for _, e := range childToParents[node] {
			if reachable[e.ParentNumber] {
				inDegree[node]++
			}
		}
	}

	var roots []PatentNumber
	for node := range reachable {
		if inDegree[node] == 0 {
			roots = append(roots, node)
			depths[node] = 0
		}
	}

	if len(roots) == 0 && len(reachable) > 0 {
		roots = append(roots, target)
		depths[target] = 0
	}

	queue := make([]PatentNumber, len(roots))
	copy(queue, roots)
	visitedDepths := make(map[PatentNumber]bool, len(reachable))
	for _, r := range roots {
		visitedDepths[r] = true
	}

	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]

		for _, e := range parentToChildren[n] {
			child := e.ChildNumber
			if !reachable[child] || visitedDepths[child] {
				continue
			}
			depths[child] = depths[n] + 1
			visitedDepths[child] = true
			queue = append(queue, child)
		}
	}

	targetDepth := depths[target]

	// 3. Build visual tree (each node appears exactly once)
	var nodes []FamilyTreeNode
	visited := map[PatentNumber]bool{}

	var dfs func(number PatentNumber, relType string, parentIdx int)
	dfs = func(number PatentNumber, relType string, parentIdx int) {
		if len(nodes) >= MaxFamilyTreeNodes || visited[number] {
			return
		}

		visited[number] = true
		idx := len(nodes)
		nodes = append(nodes, FamilyTreeNode{
			Number:       number,
			Depth:        depths[number] - targetDepth,
			RelationType: relType,
			ParentIdx:    parentIdx,
		})

		var childEdges []FamilyEdge
		for _, e := range parentToChildren[number] {
			if reachable[e.ChildNumber] {
				childEdges = append(childEdges, e)
			}
		}

		sort.Slice(childEdges, func(i, j int) bool {
			return childEdges[i].ChildNumber < childEdges[j].ChildNumber
		})

		for _, e := range childEdges {
			if len(nodes) >= MaxFamilyTreeNodes {
				break
			}
			dfs(e.ChildNumber, e.RelationType, idx)
		}
	}

	canReachTarget := map[PatentNumber]bool{target: true}
	work := []PatentNumber{target}
	for len(work) > 0 {
		n := work[0]
		work = work[1:]
		for _, e := range childToParents[n] {
			if !canReachTarget[e.ParentNumber] {
				canReachTarget[e.ParentNumber] = true
				work = append(work, e.ParentNumber)
			}
		}
	}

	sort.Slice(roots, func(i, j int) bool {
		reachesI := canReachTarget[roots[i]]
		reachesJ := canReachTarget[roots[j]]
		if reachesI != reachesJ {
			return reachesI
		}
		return roots[i] < roots[j]
	})

	for _, root := range roots {
		if len(nodes) >= MaxFamilyTreeNodes {
			break
		}
		dfs(root, "", -1)
	}

	return &FamilyGraph{
		Target:    target,
		Nodes:     nodes,
		Reachable: reachable,
		Depths:    depths,
	}
}
