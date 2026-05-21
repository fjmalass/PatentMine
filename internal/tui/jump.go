package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type jumpLabel struct {
	key       string
	preferred bool
}

func (m *Model) hasJumpTargets() bool {
	return len(m.jumpLabelsCache) > 0
}

func (m *Model) jumpTargetCount() int {
	return len(m.jumpLabelsCache)
}

func (m *Model) jumpLabels() []jumpLabel {
	switch {
	case m.mode == viewList:
		window := pageWindow(m.patentSelected, len(m.patents), m.pageSize())
		cols := m.listColumns()
		labels := make([]string, 0, len(cols)+(window.End-window.Start))
		for _, c := range cols {
			labels = append(labels, c.jumpLabel)
		}
		return m.fallbackJumpLabels(len(cols)+window.End-window.Start, labels)
	case m.mode == viewDetail:
		fields := m.detailFields()
		labels := make([]jumpLabel, 0, len(fields))
		for _, field := range fields {
			labels = append(labels, jumpLabel{key: field.jumpLabel, preferred: true})
		}
		return labels
	case m.isCitationView():
		edges, err := m.currentCitationEdges()
		if err != nil || len(edges) == 0 {
			return nil
		}
		avail := m.overlayContentWidth() - (listRowPrefixWidth + overlayIndexWidth)
		cols := m.citationColumns(avail)
		start := pageStart(clamp(m.citationLocalIdx, 0, len(edges)-1), m.overlayPageSize())
		end := min(start+m.overlayPageSize(), len(edges))
		labels := make([]string, 0, len(cols)+(end-start))
		for _, c := range cols {
			labels = append(labels, c.jumpLabel)
		}
		return m.fallbackJumpLabels(len(cols)+(end-start), labels)
	case m.mode == viewReview:
		edges, err := m.currentReviewCitationEdges()
		if err != nil || len(edges) == 0 {
			return nil
		}
		avail := m.overlayContentWidth() - (listRowPrefixWidth + overlayIndexWidth)
		cols := m.reviewOverlayColumns(avail)
		start := pageStart(clamp(m.reviewSelected, 0, len(edges)-1), m.overlayPageSize())
		end := min(start+m.overlayPageSize(), len(edges))
		labels := make([]string, 0, len(cols)+(end-start))
		for _, c := range cols {
			labels = append(labels, c.jumpLabel)
		}
		return m.fallbackJumpLabels(len(cols)+(end-start), labels)
	case m.mode == viewFamily:
		nodes := m.buildFamilyTree()
		return m.fallbackJumpLabels(len(nodes), nil)
	default:
		return nil
	}
}

func (m *Model) jumpPrefix(idx int) string {
	labels := m.jumpLabelsCache
	if !m.jumpMode || idx < 0 || idx >= len(labels) {
		return ""
	}
	label := labels[idx]
	if label.key == "" {
		return ""
	}
	color := ColorYellow
	style := lipgloss.NewStyle().Bold(true)
	if !label.preferred {
		color = ColorDim
		style = style.Italic(true).Bold(false)
	}
	return style.Foreground(lipgloss.Color(color)).Render(label.key) + " "
}

func (m *Model) applyJump(key string) (tea.Model, tea.Cmd) {
	if len(key) != 1 {
		return m, nil
	}
	index := m.indexJumpLabel(key)
	if index < 0 {
		return m, nil
	}
	m.jumpMode = false

	switch {
	case m.mode == viewList:
		cols := m.listColumns()
		colCount := len(cols)
		if index < colCount {
			m.sortColumnIndex = index
			return m, nil
		}
		index -= colCount
		window := pageWindow(m.patentSelected, len(m.patents), m.pageSize())
		target := window.Start + index
		if target < len(m.patents) {
			m.patentSelected = target
			return m.openPatent(string(m.patents[target].Number))
		}
	case m.mode == viewDetail:
		m.detailSelected = index
	case m.isCitationView():
		avail := m.overlayContentWidth() - (listRowPrefixWidth + overlayIndexWidth)
		cols := m.citationColumns(avail)
		colCount := len(cols)
		if index < colCount {
			m.sortColumnIndex = index
			return m, nil
		}
		index -= colCount
		edges, err := m.currentCitationEdges()
		if err != nil || len(edges) == 0 {
			return m, nil
		}
		start := pageStart(clamp(m.citationLocalIdx, 0, len(edges)-1), m.overlayPageSize())
		m.citationLocalIdx = clamp(start+index, 0, len(edges)-1)
	case m.mode == viewReview:
		avail := m.overlayContentWidth() - (listRowPrefixWidth + overlayIndexWidth)
		cols := m.reviewOverlayColumns(avail)
		colCount := len(cols)
		if index < colCount {
			m.sortColumnIndex = index
			return m, nil
		}
		index -= colCount
		edges, err := m.currentReviewCitationEdges()
		if err != nil || len(edges) == 0 {
			return m, nil
		}
		start := pageStart(clamp(m.reviewSelected, 0, len(edges)-1), m.overlayPageSize())
		m.reviewSelected = clamp(start+index, 0, len(edges)-1)
	case m.mode == viewFamily:
		nodes := m.buildFamilyTree()
		m.familySelected = clamp(index, 0, len(nodes)-1)
	}
	return m, nil
}

func (m *Model) indexJumpLabel(target string) int {
	for i, label := range m.jumpLabelsCache {
		if label.key == target {
			return i
		}
	}
	return -1
}

func (m *Model) fallbackJumpLabels(count int, preferred []string) []jumpLabel {
	if count <= 0 {
		return nil
	}
	labels := make([]jumpLabel, 0, count)
	used := map[string]bool{}

	for _, label := range preferred {
		if label == "" {
			labels = append(labels, jumpLabel{})
			continue
		}
		if used[label] {
			labels = append(labels, jumpLabel{})
			continue
		}
		labels = append(labels, jumpLabel{key: label, preferred: true})
		used[label] = true
	}

	poolIdx := 0
	for i := 0; i < count; i++ {
		if i < len(labels) && labels[i].key != "" {
			continue
		}
		for poolIdx < len(jumpFallbackLabels) {
			char := string(jumpFallbackLabels[poolIdx])
			poolIdx++
			if !used[char] {
				if i < len(labels) {
					labels[i] = jumpLabel{key: char, preferred: false}
				} else {
					labels = append(labels, jumpLabel{key: char, preferred: false})
				}
				used[char] = true
				break
			}
		}
	}

	if len(labels) > count {
		return labels[:count]
	}
	return labels
}

func indexString(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}
