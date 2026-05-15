package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"patentmine/internal/domain"
)

func (m *Model) selectableReviewStates() []string {
	return []string{
		domain.ReviewStateStored,
		domain.ReviewStateUnderReview,
		domain.ReviewStateIgnored,
		domain.ReviewStateCached,
	}
}

func (m *Model) viewReviewStateSelect() string {
	var body strings.Builder
	statuses := m.selectableReviewStates()
	for i, s := range statuses {
		cursor := "  "
		if i == m.reviewStateSelected {
			cursor = "> "
		}
		style := overlayBase()
		if color, ok := ReviewStateColors[s]; ok {
			style = style.Foreground(lipgloss.Color(color))
		}
		if i == m.reviewStateSelected {
			style = style.Bold(true).Underline(true)
		}
		body.WriteString(cursor + style.Render(s) + "\n")
	}
	return m.renderPopup(m.reviewStateSelectTitle(), body.String())
}

func (m *Model) reviewStateSelectTitle() string {
	sel := m.activeSelection
	if sel.IsMulti() {
		if sel.kind == selKindCitation {
			return fmt.Sprintf("Change Status · %d citations", sel.Count())
		}
		return fmt.Sprintf("Change Status · %d patents", sel.Count())
	}
	if sel.livePatent != "" {
		return "Change Status · " + sel.livePatent
	}
	return "Change Status"
}

func (m *Model) applyReviewStateSelection() (tea.Model, tea.Cmd) {
	statuses := m.selectableReviewStates()
	if m.reviewStateSelected < 0 || m.reviewStateSelected >= len(statuses) {
		return m.goBack()
	}
	next := statuses[m.reviewStateSelected]
	sel := m.activeSelection
	prevMode := previousModeOr(m, viewList)

	// detail: also patches backStack so restored current has updated ReviewState
	if prevMode == viewDetail {
		if err := m.repo.UpdatePatentReviewState(m.ctx, m.ProjectID, m.current.Number, next); err != nil {
			m.err = err.Error()
			m.activeSelection = selectionContext{}
			return m.goBack()
		}
		m.message = fmt.Sprintf("%s → %s", m.current.Number, next)
		m.logActivity(ActivityPatentReviewState, m.current.Number, next)
		if len(m.backStack) > 0 {
			m.backStack[len(m.backStack)-1].current.ReviewState = next
		}
		m.activeSelection = selectionContext{}
		m = m.markDirty(dirtyDetail | dirtyFamily)
		return m.goBack()
	}

	// review queue: single edge with different resolution (multi-parent source)
	if prevMode == viewReview {
		edge, ok, err := m.selectedReviewCitationEdge()
		if err != nil || !ok {
			m.activeSelection = selectionContext{}
			return m.goBack()
		}
		if err := m.repo.UpdateCitationReviewState(m.ctx, m.ProjectID, edge, next); err != nil {
			m.err = err.Error()
			m.activeSelection = selectionContext{}
			return m.goBack()
		}
		m.message = fmt.Sprintf("%s → %s", edge.TargetPatent, next)
		m.logActivity(ActivityCitationReviewState, edge.TargetPatent, next)
		m.activeSelection = selectionContext{}
		m = m.markDirty(dirtyDetail)
		return m.goBack()
	}

	if sel.kind == selKindCitation {
		edges, indices, err := sel.CitationEdges(m)
		if err != nil || len(edges) == 0 {
			m.activeSelection = selectionContext{}
			return m.goBack()
		}
		updatedCount := 0
		for _, idx := range indices {
			if idx < 0 || idx >= len(edges) {
				continue
			}
			edge := edges[idx]
			if err := m.repo.UpdateCitationReviewState(m.ctx, m.ProjectID, edge, next); err != nil {
				m.logger.Error("citation status update failed", "patent", edge.TargetPatent, "error", err)
				continue
			}
			m.logActivity(ActivityCitationReviewState, edge.TargetPatent, next)
			updatedCount++
		}
		if updatedCount > 1 {
			m.message = fmt.Sprintf("updated status to %s for %d citations", next, updatedCount)
		} else if updatedCount == 1 && len(indices) > 0 && indices[0] < len(edges) {
			m.message = fmt.Sprintf("%s → %s", edges[indices[0]].TargetPatent, next)
		}
		m.activeSelection = selectionContext{}
		m.visualMode = false
		if len(m.backStack) > 0 {
			m.backStack[len(m.backStack)-1].visualMode = false
		}
		m = m.markDirty(dirtyDetail)
		return m.goBack()
	}

	// patent kind: list, family, or single
	patentNums, err := sel.PatentNumbers(m)
	if err != nil || len(patentNums) == 0 {
		m.activeSelection = selectionContext{}
		return m.goBack()
	}
	updatedCount := 0
	for _, num := range patentNums {
		if err := m.repo.UpdatePatentReviewState(m.ctx, m.ProjectID, num, next); err != nil {
			m.logger.Error("status selection update failed", "patent", num, "error", err)
			continue
		}
		m.logActivity(ActivityPatentReviewState, num, next)
		updatedCount++
	}
	if updatedCount > 1 {
		m.message = fmt.Sprintf("updated status to %s for %d patents", next, updatedCount)
	} else if updatedCount == 1 {
		m.message = fmt.Sprintf("%s → %s", patentNums[0], next)
	}
	m.activeSelection = selectionContext{}
	m.visualMode = false
	if len(m.backStack) > 0 {
		m.backStack[len(m.backStack)-1].visualMode = false
	}
	m = m.markDirty(dirtyDetail | dirtyFamily)
	return m.goBack()
}

// listSelectionFromSnapshot reads selected indices from the pre-overlay backStack
// snapshot. Overlay modes have m.mode != viewList so activeSelectionIndex() falls
// to default:0 — using the snapshot avoids that.
func (m *Model) listSelectionFromSnapshot() []int {
	if len(m.backStack) > 0 {
		snap := m.backStack[len(m.backStack)-1]
		if snap.visualMode {
			start, end := snap.selectionStart, snap.selected
			if start > end {
				start, end = end, start
			}
			res := make([]int, 0, end-start+1)
			for i := start; i <= end; i++ {
				res = append(res, i)
			}
			return res
		}
		return []int{snap.selected}
	}
	return []int{m.selected}
}

func (m *Model) selectableCountries() []string {
	countries := make([]string, 0, len(domain.PatentCountryCodes)+1)
	countries = append(countries, "(clear)")
	countries = append(countries, domain.PatentCountryCodes...)
	return countries
}

func (m *Model) countryCounts() map[string]int {
	counts := make(map[string]int)
	for _, p := range m.patents {
		code := patentCountryLabel(p)
		if code != "-" {
			counts[code]++
		}
	}
	return counts
}

func (m *Model) viewCountrySelect() string {
	var body strings.Builder
	countries := m.selectableCountries()
	counts := m.countryCounts()
	base := overlayBase()
	activeCode := m.listFilter.Country

	for i, c := range countries {
		cursor := "  "
		if i == m.countrySelectSelected {
			cursor = "> "
		}
		style := base
		label := c

		if c == "(clear)" {
			if activeCode == EmptyFilter {
				style = style.Foreground(lipgloss.Color(ColorYellow))
			}
		} else {
			if c == activeCode {
				style = style.Foreground(lipgloss.Color(ColorYellow))
			}
			if count, ok := counts[c]; ok {
				label = fmt.Sprintf("%s  (%d)", c, count)
			}
		}

		if i == m.countrySelectSelected {
			style = style.Bold(true).Underline(true)
		}

		body.WriteString(cursor + style.Render(label) + "\n")
	}

	return m.renderPopup("Select Country", body.String())
}

func (m *Model) viewBulkConfirm() string {
	count := len(m.bulkActionIndices)
	var actionVerb string
	switch m.bulkAction {
	case bulkActionStore:
		actionVerb = "save"
	case bulkActionIgnore:
		actionVerb = "ignore"
	case bulkActionUnderReview:
		actionVerb = "mark as under review"
	}
	content := fmt.Sprintf("Do you want to %s %d citation(s) in the database?", actionVerb, count)
	return m.renderPopup("Bulk Action Confirmation", content)
}

func (m *Model) applyCountrySelection() (tea.Model, tea.Cmd) {
	countries := m.selectableCountries()
	if m.countrySelectSelected < 0 || m.countrySelectSelected >= len(countries) {
		return m.goBack()
	}
	code := countries[m.countrySelectSelected]

	if code == "(clear)" {
		m.listFilter.Country = EmptyFilter
		m.message = "country filter cleared"
	} else {
		m.listFilter.Country = code
		m.message = "filtering by country: " + code
	}

	m.setMode(viewList)
	return m.refreshList()
}

func (m *Model) handleViewReviewStateSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyVimDown, keyArrowDown:
		m.reviewStateSelected = clamp(m.reviewStateSelected+m.consumeCount(1), 0, len(m.selectableReviewStates())-1)
	case keyVimUp, keyArrowUp:
		m.reviewStateSelected = max(0, m.reviewStateSelected-m.consumeCount(1))
	case keyEnter:
		return m.applyReviewStateSelection()
	case keyEsc, keyBack:
		return m.goBack()
	default:
		m.tryAccumulateCount(msg.String())
	}
	return m, nil
}

func (m *Model) handleViewCountrySelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyVimDown, keyArrowDown:
		m.countrySelectSelected = clamp(m.countrySelectSelected+m.consumeCount(1), 0, len(m.selectableCountries())-1)
	case keyVimUp, keyArrowUp:
		m.countrySelectSelected = max(0, m.countrySelectSelected-m.consumeCount(1))
	case keyEnter:
		return m.applyCountrySelection()
	case keyEsc, keyBack:
		return m.goBack()
	default:
		m.tryAccumulateCount(msg.String())
	}
	return m, nil
}
