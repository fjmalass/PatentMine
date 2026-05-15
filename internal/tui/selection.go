package tui

import (
	"fmt"
	"strconv"

	"patentmine/internal/domain"
	"patentmine/internal/storage"
)

func (m *Model) isInSelection(idx int) bool {
	if !m.visualMode {
		return false
	}
	current := m.activeSelectionIndex()
	start, end := m.selectionStart, current
	if start > end {
		start, end = end, start
	}
	return idx >= start && idx <= end
}

func (m *Model) activeSelectionIndex() int {
	switch {
	case m.isCitationView():
		return m.citationSelection()
	case m.mode == viewReview:
		return m.reviewSelected
	case m.mode == viewList:
		return m.selected
	case m.mode == viewClassifications:
		return m.classificationSelected
	case m.mode == viewInventors:
		return m.inventorSelected
	case m.mode == viewFamily:
		return m.familySelected
	case m.mode == viewDetail:
		return m.detailSelected
	default:
		return 0
	}
}

func (m *Model) selectedIndices() []int {
	current := m.activeSelectionIndex()
	if !m.visualMode {
		return []int{current}
	}
	start, end := m.selectionStart, current
	if start > end {
		start, end = end, start
	}
	var res []int
	for i := start; i <= end; i++ {
		res = append(res, i)
	}
	return res
}

func (m *Model) setActiveSelectionIndex(val int) {
	count := m.activeItemCount()
	if count == 0 {
		return
	}
	val = clamp(val, 0, count-1)
	switch {
	case m.isCitationView():
		m.setCitationSelection(val)
	case m.mode == viewReview:
		m.reviewSelected = val
	case m.mode == viewList:
		m.selected = val
	case m.mode == viewClassifications:
		m.classificationSelected = val
	case m.mode == viewInventors:
		m.inventorSelected = val
	case m.mode == viewFamily:
		m.familySelected = val
	case m.mode == viewDetail:
		m.detailSelected = val
	}
}

func (m *Model) activeItemCount() int {
	switch {
	case m.isCitationView():
		edges, _ := m.currentCitationEdges()
		return len(edges)
	case m.mode == viewReview:
		edges, _ := m.currentReviewCitationEdges()
		return len(edges)
	case m.mode == viewList:
		return len(m.patents)
	case m.mode == viewClassifications:
		cls, _ := m.repo.ListClassifications(m.ctx, m.ProjectID, m.current.Number)
		return len(cls)
	case m.mode == viewInventors:
		return len(m.current.Inventors)
	case m.mode == viewFamily:
		return len(m.buildFamilyTree())
	case m.mode == viewDetail:
		return len(m.detailFields())
	default:
		return 0
	}
}

func (m *Model) pageSize() int {
	if m.height <= 0 {
		return 20
	}
	return max(5, m.height-14)
}

func (m *Model) overlayPageSize() int {
	if m.height <= 0 {
		return 15
	}
	return max(3, m.height-18)
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func rowIndexLabel(zeroBasedIndex int) string {
	return fmt.Sprintf("%3d", zeroBasedIndex+1)
}

func isCountKey(key string) bool {
	return len(key) == 1 && key[0] >= '0' && key[0] <= '9'
}

func (m *Model) consumeCount(defaultValue int) int {
	if m.countBuffer == "" {
		return defaultValue
	}
	count, err := strconv.Atoi(m.countBuffer)
	m.countBuffer = EmptyCount
	if err != nil || count <= 0 {
		return defaultValue
	}
	return count
}

func (m *Model) tryAccumulateCount(key string) {
	if isCountKey(key) && !(key == "0" && m.countBuffer == "") {
		m.countBuffer += key
	}
}

func (m *Model) goToRow(index int) *Model {
	if index <= 0 {
		index = 1
	}
	target := index - 1
	m.setActiveSelectionIndex(target)
	return m
}

func (m *Model) goToClassification(index int) *Model {
	classifications, _ := m.repo.ListClassifications(m.ctx, m.ProjectID, m.current.Number)
	if len(classifications) == 0 {
		return m
	}
	m.classificationSelected = clamp(index-1, 0, len(classifications)-1)
	return m
}

// selectionKind distinguishes patent-list selections from citation-edge selections.
type selectionKind int8

const (
	selKindPatent   selectionKind = iota // indices → patent numbers
	selKindCitation                       // indices → citation edge targets (lazy fetch)
)

// selectionContext captures selection state at action time so overlay apply
// functions can resolve patent numbers without knowing the caller's view depth.
// Patent kind: patentNums pre-resolved from m.patents or family nodes.
// Citation kind: lazy — stores query params; edges fetched at apply time.
type selectionContext struct {
	kind       selectionKind
	indices    []int  // sorted ascending
	livePatent string // first-selected; already live-mutated (tag space-toggle target)
	// patent kind:
	patentNums []string
	// citation kind:
	parentNumber string
	relation     string
	sortColumn   string
	sortOrder    string
	reviewFilter string
}

func (s selectionContext) Count() int    { return len(s.indices) }
func (s selectionContext) IsMulti() bool { return len(s.indices) > 1 }

// PatentNumbers returns patent numbers for all selected items.
// Citation kind: lazily fetches edges from repo.
func (s selectionContext) PatentNumbers(m *Model) ([]string, error) {
	if s.kind == selKindPatent {
		return s.patentNums, nil
	}
	edges, err := s.resolveCitationEdges(m)
	if err != nil {
		return nil, err
	}
	nums := make([]string, 0, len(s.indices))
	for _, idx := range s.indices {
		if idx >= 0 && idx < len(edges) {
			nums = append(nums, edges[idx].TargetPatent)
		}
	}
	return nums, nil
}

// CitationEdges fetches citation edges and returns them with the stored indices.
// Returns nil, nil, nil for patent-kind selections.
func (s selectionContext) CitationEdges(m *Model) ([]domain.CitationEdge, []int, error) {
	if s.kind != selKindCitation {
		return nil, nil, nil
	}
	edges, err := s.resolveCitationEdges(m)
	return edges, s.indices, err
}

func (s selectionContext) resolveCitationEdges(m *Model) ([]domain.CitationEdge, error) {
	opts := storage.ListCitationsOptions{
		SortColumn:        s.sortColumn,
		SortOrder:         s.sortOrder,
		ReviewStateFilter: s.reviewFilter,
	}
	return m.repo.ListCitations(m.ctx, m.ProjectID, s.parentNumber, s.relation, opts)
}

// captureSelection snapshots the current selection so overlay apply functions
// can resolve patent numbers without knowing the caller's view or stack depth.
func (m *Model) captureSelection() selectionContext {
	switch {
	case m.mode == viewList:
		indices := m.selectedIndices()
		nums := make([]string, 0, len(indices))
		for _, idx := range indices {
			if idx >= 0 && idx < len(m.patents) {
				nums = append(nums, m.patents[idx].Number)
			}
		}
		live := ""
		if len(nums) > 0 {
			live = nums[0]
		}
		return selectionContext{kind: selKindPatent, indices: indices, livePatent: live, patentNums: nums}

	case m.isCitationView():
		relation := domain.RelationCites
		if m.mode == viewCitedBy {
			relation = domain.RelationCitedBy
		}
		indices := m.selectedIndices()
		live := ""
		if edges, err := m.citationEdgesForRelation(relation); err == nil && len(edges) > 0 && len(indices) > 0 {
			live = edges[clamp(indices[0], 0, len(edges)-1)].TargetPatent
		}
		return selectionContext{
			kind:         selKindCitation,
			indices:      indices,
			livePatent:   live,
			parentNumber: m.current.Number,
			relation:     relation,
			sortColumn:   m.sortColumn,
			sortOrder:    m.sortOrder,
			reviewFilter: m.citesReviewStateFilter,
		}

	case m.mode == viewFamily:
		nodes := m.buildFamilyTree()
		live := ""
		if m.familySelected >= 0 && m.familySelected < len(nodes) {
			live = nodes[m.familySelected].number
		}
		return selectionContext{
			kind:       selKindPatent,
			indices:    []int{m.familySelected},
			livePatent: live,
			patentNums: []string{live},
		}

	default: // viewDetail, viewReview, or any other — single current patent
		live := m.current.Number
		return selectionContext{
			kind:       selKindPatent,
			indices:    []int{0},
			livePatent: live,
			patentNums: []string{live},
		}
	}
}
