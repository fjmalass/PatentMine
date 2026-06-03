package engine

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/store"
)

const (
	defaultFamilyGraphDepth    = 2
	defaultFamilyGraphMaxNodes = 60
	maxFamilyGraphDepth        = 10
	maxFamilyGraphNodes        = 400
)

type familyEdgeKey struct {
	parent domain.PatentNumber
	child  domain.PatentNumber
}

type familyEdgeState struct {
	parentSeen bool
	childSeen  bool
}

type familyNodeState struct {
	patent   domain.Patent
	depth    int
	parents  map[domain.PatentNumber]bool
	children map[domain.PatentNumber]bool
}

type familyQueueNode struct {
	number domain.PatentNumber
	depth  int
}

// FamilyGraph returns a bounded BFS parent/child DAG around one patent.
func (e *Engine) FamilyGraph(ctx context.Context, p proto.FamilyGraphParams) (out proto.FamilyGraphResult, err error) {
	defer e.observeDuration("engine.family_graph", time.Now(), &err)
	if p.Root.IsZero() {
		return proto.FamilyGraphResult{}, errors.New("engine: family graph root must not be empty")
	}

	depthLimit := p.Depth
	if depthLimit <= 0 {
		depthLimit = defaultFamilyGraphDepth
	}
	depthLimit = min(depthLimit, maxFamilyGraphDepth)

	maxNodes := p.MaxNodes
	if maxNodes <= 0 {
		maxNodes = defaultFamilyGraphMaxNodes
	}
	maxNodes = min(maxNodes, maxFamilyGraphNodes)

	allowedCountries := normalizeCountries(p.Countries)
	nodes := map[domain.PatentNumber]*familyNodeState{}
	edges := map[familyEdgeKey]familyEdgeState{}
	visited := map[domain.PatentNumber]bool{}
	queued := map[domain.PatentNumber]bool{}
	hiddenByCountry := 0
	hiddenNodes := map[domain.PatentNumber]bool{}
	truncated := false
	inconsistent := 0

	queue := []familyQueueNode{{number: p.Root, depth: 0}}
	queued[p.Root] = true

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if visited[item.number] {
			continue
		}
		visited[item.number] = true

		patent, loadErr := e.repo.Patent(ctx, item.number)
		if loadErr != nil {
			if errors.Is(loadErr, store.ErrNotFound) {
				patent = domain.Patent{Number: item.number, DisplayNumber: item.number, FetchState: domain.FetchStub}
			} else {
				return proto.FamilyGraphResult{}, loadErr
			}
		}
		ensureFamilyNode(nodes, patent, item.depth)
		if item.depth >= depthLimit {
			continue
		}

		rels, relErr := e.repo.AllRelations(ctx, item.number)
		if relErr != nil {
			return proto.FamilyGraphResult{}, relErr
		}
		for _, rel := range rels {
			parent, child, ok := familyEdgeFromRelation(rel, item.number)
			if !ok {
				continue
			}
			state := edges[familyEdgeKey{parent: parent, child: child}]
			if rel.Kind == domain.RelationParent {
				state.parentSeen = true
			} else {
				state.childSeen = true
			}
			edges[familyEdgeKey{parent: parent, child: child}] = state

			parentVisible := familyNumberVisible(parent, p.Root, allowedCountries)
			childVisible := familyNumberVisible(child, p.Root, allowedCountries)
			if !parentVisible {
				hiddenNodes[parent] = true
			}
			if !childVisible {
				hiddenNodes[child] = true
			}
			if parentVisible && childVisible {
				parentNode, ok, loadErr := ensureVisibleFamilyNode(ctx, e.repo, nodes, parent, min(item.depth+1, depthLimit), maxNodes, len(queue))
				if loadErr != nil {
					return proto.FamilyGraphResult{}, loadErr
				}
				if !ok {
					truncated = true
					continue
				}
				childNode, ok, loadErr := ensureVisibleFamilyNode(ctx, e.repo, nodes, child, min(item.depth+1, depthLimit), maxNodes, len(queue))
				if loadErr != nil {
					return proto.FamilyGraphResult{}, loadErr
				}
				if !ok {
					truncated = true
					continue
				}
				parentNode.children[child] = true
				childNode.parents[parent] = true
			}

			for _, neighbor := range []domain.PatentNumber{parent, child} {
				if neighbor == item.number {
					continue
				}
				if !familyNumberVisible(neighbor, p.Root, allowedCountries) {
					continue
				}
				if visited[neighbor] || queued[neighbor] {
					continue
				}
				if len(nodes)+len(queue) >= maxNodes {
					truncated = true
					continue
				}
				queue = append(queue, familyQueueNode{number: neighbor, depth: item.depth + 1})
				queued[neighbor] = true
			}
		}
	}
	hiddenByCountry = len(hiddenNodes)

	for key, state := range edges {
		if state.parentSeen || state.childSeen {
			continue
		}
		inconsistent++
		e.log(ctx, slog.LevelWarn, "family relation mismatch",
			slog.String("root", p.Root.String()),
			slog.String("parent", key.parent.String()),
			slog.String("child", key.child.String()),
			slog.Bool("has_parent_edge", state.parentSeen),
			slog.Bool("has_child_edge", state.childSeen))
		e.incCounter("engine.family_graph.inconsistent_edges_total", 1)
	}

	resultNodes := make([]proto.FamilyGraphNode, 0, len(nodes))
	for _, node := range nodes {
		parents := sortedPatentNumbers(node.parents)
		children := sortedPatentNumbers(node.children)
		resultNodes = append(resultNodes, proto.FamilyGraphNode{
			Patent:   e.familyPatentRow(ctx, p.Project, node.patent),
			Depth:    node.depth,
			Parents:  parents,
			Children: children,
		})
	}
	slices.SortFunc(resultNodes, func(a, b proto.FamilyGraphNode) int {
		if a.Depth != b.Depth {
			return a.Depth - b.Depth
		}
		if cmp := strings.Compare(a.Patent.Number.Country, b.Patent.Number.Country); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Patent.Number.Normalized(), b.Patent.Number.Normalized())
	})

	resultEdges := make([]proto.FamilyGraphEdge, 0, len(edges))
	for key, state := range edges {
		if _, ok := nodes[key.parent]; !ok {
			continue
		}
		if _, ok := nodes[key.child]; !ok {
			continue
		}
		resultEdges = append(resultEdges, proto.FamilyGraphEdge{
			Parent:       key.parent,
			Child:        key.child,
			Inconsistent: !(state.parentSeen || state.childSeen),
		})
	}
	slices.SortFunc(resultEdges, func(a, b proto.FamilyGraphEdge) int {
		if cmp := strings.Compare(a.Parent.Normalized(), b.Parent.Normalized()); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Child.Normalized(), b.Child.Normalized())
	})

	return proto.FamilyGraphResult{
		Root:               p.Root,
		Depth:              depthLimit,
		MaxNodes:           maxNodes,
		Countries:          sortedCountryList(allowedCountries),
		Nodes:              resultNodes,
		Edges:              resultEdges,
		Truncated:          truncated,
		HiddenByCountry:    hiddenByCountry,
		InconsistencyCount: inconsistent,
	}, nil
}

func ensureFamilyNode(nodes map[domain.PatentNumber]*familyNodeState, patent domain.Patent, depth int) *familyNodeState {
	if existing, ok := nodes[patent.Number]; ok {
		if depth < existing.depth {
			existing.depth = depth
		}
		if existing.patent.FetchState == domain.FetchStub && patent.FetchState != domain.FetchStub {
			existing.patent = patent
		}
		return existing
	}
	node := &familyNodeState{
		patent:   patent,
		depth:    depth,
		parents:  map[domain.PatentNumber]bool{},
		children: map[domain.PatentNumber]bool{},
	}
	nodes[patent.Number] = node
	return node
}

func familyEdgeFromRelation(rel domain.Relation, focus domain.PatentNumber) (parent domain.PatentNumber, child domain.PatentNumber, ok bool) {
	switch rel.Kind {
	case domain.RelationParent:
		return rel.To, rel.From, rel.From == focus || rel.To == focus
	case domain.RelationChild:
		return rel.From, rel.To, rel.From == focus || rel.To == focus
	default:
		return domain.PatentNumber{}, domain.PatentNumber{}, false
	}
}

func (e *Engine) familyPatentRow(ctx context.Context, project domain.ProjectID, p domain.Patent) domain.PatentRow {
	shown := p.DisplayNumber
	if shown.IsZero() {
		shown = p.Number
	}
	row := domain.PatentRow{
		Number:          p.Number,
		DisplayNumber:   shown,
		Title:           p.Title,
		Inventors:       p.Inventors,
		ExpirationDate:  p.ExpirationDate,
		Classifications: p.Classifications,
		FetchState:      p.FetchState,
	}
	if state, ok, err := e.ReviewStateOf(ctx, project, p.Number); err == nil && ok {
		row.ReviewState = state
	}
	return row
}

func normalizeCountries(raw []string) map[string]bool {
	if len(raw) == 0 {
		return nil
	}
	out := map[string]bool{}
	for _, country := range raw {
		code := strings.ToUpper(strings.TrimSpace(country))
		if code != "" {
			out[code] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedCountryList(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for country := range set {
		out = append(out, country)
	}
	slices.Sort(out)
	return out
}

func countryAllowed(number domain.PatentNumber, allowed map[string]bool) bool {
	if len(allowed) == 0 {
		return true
	}
	return allowed[number.Country]
}

func familyNumberVisible(number, root domain.PatentNumber, allowed map[string]bool) bool {
	if number == root {
		return true
	}
	return countryAllowed(number, allowed)
}

func ensureVisibleFamilyNode(ctx context.Context, repo store.Repository, nodes map[domain.PatentNumber]*familyNodeState, number domain.PatentNumber, depth, maxNodes, queued int) (*familyNodeState, bool, error) {
	if node, ok := nodes[number]; ok {
		if depth < node.depth {
			node.depth = depth
		}
		return node, true, nil
	}
	if len(nodes)+queued >= maxNodes {
		return nil, false, nil
	}
	patent, err := repo.Patent(ctx, number)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return nil, false, err
		}
		patent = domain.Patent{Number: number, DisplayNumber: number, FetchState: domain.FetchStub}
	}
	return ensureFamilyNode(nodes, patent, depth), true, nil
}

func sortedPatentNumbers(set map[domain.PatentNumber]bool) []domain.PatentNumber {
	if len(set) == 0 {
		return nil
	}
	out := make([]domain.PatentNumber, 0, len(set))
	for number := range set {
		out = append(out, number)
	}
	slices.SortFunc(out, func(a, b domain.PatentNumber) int {
		if cmp := strings.Compare(a.Country, b.Country); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.Normalized(), b.Normalized())
	})
	return out
}
