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
		return m.patentSelected
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
		m.patentSelected = val
	case m.mode == viewClassifications:
		m.classificationSelected = val
	case m.mode == viewInventors:
		m.inventorSelected = val
	case m.mode == viewFamily:
		m.familySelected = val
	case m.mode == viewDetail:
		m.detailSelected = val
	}
	m.trackVisualEnd(val)
}

// trackVisualEnd saves the current visual range whenever the cursor moves.
// Called from setActiveSelectionIndex so the save always happens in the correct view.
func (m *Model) trackVisualEnd(val int) {
	if !m.visualMode {
		return
	}
	m.lastVisualStart = m.selectionStart
	m.lastVisualEnd = val
	m.lastVisualValid = true
	if m.mode == viewList {
		start, end := m.selectionStart, val
		if start > end {
			start, end = end, start
		}
		m.lastVisualNumbers = make([]string, 0, end-start+1)
		for i := start; i <= end && i < len(m.patents); i++ {
			m.lastVisualNumbers = append(m.lastVisualNumbers, m.patents[i].Number)
		}
	} else {
		m.lastVisualNumbers = nil
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

// clearVisualMode exits visual mode. The last visual range was already tracked
// by trackVisualEnd on each cursor movement, so no save is needed here.
func (m *Model) clearVisualMode() {
	m.visualMode = false
}

// restoreVisualSelection re-enters visual mode with the last saved selection (gv).
// List view: resolves saved patent numbers to current indices (sort-agnostic).
// Other views: restores raw indices.
func (m *Model) restoreVisualSelection() *Model {
	if !m.lastVisualValid {
		return m
	}
	if m.mode != viewList && !m.isCitationView() && m.mode != viewReview {
		return m
	}
	if m.mode == viewList && len(m.lastVisualNumbers) > 0 {
		first, last := m.findVisualRange(m.lastVisualNumbers)
		if first == -1 {
			return m
		}
		m.visualMode = true
		m.selectionStart = first
		m.patentSelected = clamp(last, 0, len(m.patents)-1)
		// Re-save with current positions so subsequent gv is idempotent.
		m.lastVisualStart = first
		m.lastVisualEnd = m.patentSelected
	} else {
		m.visualMode = true
		m.selectionStart = m.lastVisualStart
		m.setActiveSelectionIndex(m.lastVisualEnd)
	}
	return m
}

func (m *Model) findVisualRange(numbers []string) (first, last int) {
	idx := make(map[string]int, len(m.patents))
	for i, p := range m.patents {
		idx[p.Number] = i
	}
	first, last = -1, -1
	for _, num := range numbers {
		if i, ok := idx[num]; ok {
			if first == -1 || i < first {
				first = i
			}
			if i > last {
				last = i
			}
		}
	}
	return first, last
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

func rowIndexLabel(idx int) string {
	return fmt.Sprintf("%3d", idx+1)
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

func (m *Model) goToRow(idx int) *Model {
	if idx <= 0 {
		idx = 1
	}
	target := idx - 1
	m.setActiveSelectionIndex(target)
	return m
}

func (m *Model) goToClassification(idx int) *Model {
	classifications, _ := m.repo.ListClassifications(m.ctx, m.ProjectID, m.current.Number)
	if len(classifications) == 0 {
		return m
	}
	m.classificationSelected = clamp(idx-1, 0, len(classifications)-1)
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
// Patent kind: patentNumbers pre-resolved from m.patents or family nodes.
// Citation kind: lazy — stores query params; edges fetched at apply time.
type selectionContext struct {
	kind       selectionKind
	indices    []int  // sorted ascending
	livePatent string // first-selected; already live-mutated (tag space-toggle target)
	// patent kind:
	patentNumbers []string
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
		return s.patentNumbers, nil
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
		return selectionContext{kind: selKindPatent, indices: indices, livePatent: live, patentNumbers: nums}

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
			patentNumbers: []string{live},
		}

	default: // viewDetail, viewReview, or any other — single current patent
		live := m.current.Number
		return selectionContext{
			kind:       selKindPatent,
			indices:    []int{0},
			livePatent: live,
			patentNumbers: []string{live},
		}
	}
}
