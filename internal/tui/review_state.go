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
		cursor := rowNoCursor
		if i == m.reviewStateSelected {
			cursor = rowCursor
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
			return fmt.Sprintf("Change Status"+sepBullet+"%d citations", sel.Count())
		}
		return fmt.Sprintf("Change Status"+sepBullet+"%d patents", sel.Count())
	}
	if sel.livePatent != "" {
		return "Change Status" + sepBullet + sel.livePatent
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
			m.logger.Error("review state update failed", "project", m.ProjectID, "patent", m.current.Number, "status", next, "error", err)
			m.err = err.Error()
			m.activeSelection = selectionContext{}
			return m.goBack()
		}
		m.logger.Info("review state updated", "project", m.ProjectID, "patent", m.current.Number, "status", next)
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
			m.logger.Error("citation review state update failed", "project", m.ProjectID, "patent", edge.TargetPatent, "status", next, "error", err)
			m.err = err.Error()
			m.activeSelection = selectionContext{}
			return m.goBack()
		}
		m.logger.Info("citation review state updated", "project", m.ProjectID, "patent", edge.TargetPatent, "status", next)
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
		m.logger.Info("bulk citation status update started", "project", m.ProjectID, "status", next, "count", len(indices))
		updatedCount := 0
		for _, idx := range indices {
			if idx < 0 || idx >= len(edges) {
				continue
			}
			edge := edges[idx]
			if err := m.repo.UpdateCitationReviewState(m.ctx, m.ProjectID, edge, next); err != nil {
				m.logger.Error("citation status update failed", "project", m.ProjectID, "patent", edge.TargetPatent, "status", next, "error", err)
				continue
			}
			m.logActivity(ActivityCitationReviewState, edge.TargetPatent, next)
			updatedCount++
		}
		m.logger.Info("bulk citation status update completed", "project", m.ProjectID, "status", next, "requested", len(indices), "updated", updatedCount)
		m.logActivity(ActivityBulkPrefix+ActivityCitationReviewState, next, fmt.Sprintf("%d", updatedCount))
		if updatedCount > 1 {
			m.message = fmt.Sprintf("updated status to %s for %d citations", next, updatedCount)
		} else if updatedCount == 1 && len(indices) > 0 && indices[0] < len(edges) {
			m.message = fmt.Sprintf("%s → %s", edges[indices[0]].TargetPatent, next)
		}
		m.activeSelection = selectionContext{}
		m.clearVisualMode()
		if len(m.backStack) > 0 {
			m.backStack[len(m.backStack)-1].visualMode = false
		}
		m = m.markDirty(dirtyDetail)
		return m.goBack()
	}

	// patent kind: list, family, or single
	patentNumbers, err := sel.PatentNumbers(m)
	if err != nil || len(patentNumbers) == 0 {
		m.activeSelection = selectionContext{}
		return m.goBack()
	}
	m.logger.Info("bulk patent status update started", "project", m.ProjectID, "status", next, "count", len(patentNumbers))
	updatedCount := 0
	for _, num := range patentNumbers {
		if err := m.repo.UpdatePatentReviewState(m.ctx, m.ProjectID, num, next); err != nil {
			m.logger.Error("status selection update failed", "project", m.ProjectID, "patent", num, "status", next, "error", err)
			continue
		}
		m.logActivity(ActivityPatentReviewState, num, next)
		updatedCount++
	}
	m.logger.Info("bulk patent status update completed", "project", m.ProjectID, "status", next, "requested", len(patentNumbers), "updated", updatedCount)
	m.logActivity(ActivityBulkPrefix+ActivityPatentReviewState, next, fmt.Sprintf("%d", updatedCount))
	if updatedCount > 1 {
		m.message = fmt.Sprintf("updated status to %s for %d patents", next, updatedCount)
	} else if updatedCount == 1 {
		m.message = fmt.Sprintf("%s → %s", patentNumbers[0], next)
	}
	m.activeSelection = selectionContext{}
	m.clearVisualMode()
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
			start, end := snap.selectionStart, snap.patentSelected
			if start > end {
				start, end = end, start
			}
			res := make([]int, 0, end-start+1)
			for i := start; i <= end; i++ {
				res = append(res, i)
			}
			return res
		}
		return []int{snap.patentSelected}
	}
	return []int{m.patentSelected}
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
		cursor := rowNoCursor
		if i == m.countrySelectSelected {
			cursor = rowCursor
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

	if m.bulkAction == bulkActionDelete {
		nums := make([]string, 0, count)
		for _, idx := range m.bulkActionIndices {
			if idx < len(m.patents) {
				nums = append(nums, m.patents[idx].Number)
			}
		}
		shown := nums
		extra := 0
		if len(shown) > 10 {
			shown, extra = nums[:10], len(nums)-10
		}
		content := fmt.Sprintf("Delete %d patent(s)?\n\n%s", len(nums), strings.Join(shown, "\n"))
		if extra > 0 {
			content += fmt.Sprintf("\n…and %d more", extra)
		}
		content += "\n\nIDS entries for these patents will be removed.\nFamily edges are preserved (soft-delete only)."
		return m.renderPopup("Confirm Delete", content)
	}

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
		m.logActivity(ActivityFilterCountry, "(clear)", "")
	} else {
		m.listFilter.Country = code
		m.message = "filtering by country: " + code
		m.logActivity(ActivityFilterCountry, code, "")
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
