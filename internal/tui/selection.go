package tui

import (
	"fmt"
	"strconv"
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
