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
	prevMode := previousModeOr(m, viewList)
	switch {
	case prevMode == viewDetail:
		if m.current.Number != "" {
			return "Change Status · " + m.current.Number
		}
	case prevMode == viewCites || prevMode == viewCitedBy:
		if edge, ok, err := m.selectedCitationEdge(); err == nil && ok {
			return "Change Status · " + edge.TargetPatent
		}
	case prevMode == viewFamily:
		nodes := m.buildFamilyTree()
		if m.familySelected >= 0 && m.familySelected < len(nodes) {
			return "Change Status · " + nodes[m.familySelected].number
		}
	case prevMode == viewList:
		if len(m.backStack) > 0 {
			snap := m.backStack[len(m.backStack)-1]
			if snap.visualMode {
				start, end := snap.selectionStart, snap.selected
				if start > end {
					start, end = end, start
				}
				return fmt.Sprintf("Change Status · %d patents", end-start+1)
			}
			if snap.selected >= 0 && snap.selected < len(snap.patents) {
				return "Change Status · " + snap.patents[snap.selected].Number
			}
		} else if m.selected >= 0 && m.selected < len(m.patents) {
			return "Change Status · " + m.patents[m.selected].Number
		}
	}
	return "Change Status"
}

func (m *Model) applyReviewStateSelection() (tea.Model, tea.Cmd) {
	statuses := m.selectableReviewStates()
	if m.reviewStateSelected < 0 || m.reviewStateSelected >= len(statuses) {
		return m.goBack()
	}
	next := statuses[m.reviewStateSelected]

	prevMode := previousModeOr(m, viewList)

	if prevMode == viewDetail {
		if err := m.repo.UpdatePatentReviewState(m.ctx, m.ProjectID, m.current.Number, next); err != nil {
			m.err = err.Error()
			return m.goBack()
		}
		m.message = fmt.Sprintf("%s → %s", m.current.Number, next)
		m.logActivity(ActivityPatentReviewState, m.current.Number, next)
		return m.goBack()
	}

	if prevMode == viewCites || prevMode == viewCitedBy {
		edge, ok, err := m.selectedCitationEdge()
		if err != nil || !ok {
			return m.goBack()
		}
		if err := m.repo.UpdateCitationReviewState(m.ctx, m.ProjectID, edge, next); err != nil {
			m.err = err.Error()
			return m.goBack()
		}
		m.message = fmt.Sprintf("%s → %s", edge.TargetPatent, next)
		m.logActivity(ActivityCitationReviewState, edge.TargetPatent, next)
		return m.goBack()
	}

	if prevMode == viewFamily {
		nodes := m.buildFamilyTree()
		if m.familySelected < 0 || m.familySelected >= len(nodes) {
			return m.goBack()
		}
		node := nodes[m.familySelected]
		if err := m.repo.UpdatePatentReviewState(m.ctx, m.ProjectID, node.number, next); err != nil {
			m.err = err.Error()
			return m.goBack()
		}
		m.message = fmt.Sprintf("%s → %s", node.number, next)
		m.logActivity(ActivityPatentReviewState, node.number, next)
		m.invalidateFamilyCaches()
		return m.goBack()
	}

	// viewList: activeSelectionIndex() returns 0 for overlay modes, so read
	// selection from the pre-overlay backStack snapshot instead.
	indices := m.listSelectionFromSnapshot()

	updatedCount := 0
	for _, idx := range indices {
		if idx < 0 || idx >= len(m.patents) {
			continue
		}
		p := m.patents[idx]
		if err := m.repo.UpdatePatentReviewState(m.ctx, m.ProjectID, p.Number, next); err != nil {
			m.logger.Error("status selection update failed", "patent", p.Number, "error", err)
			continue
		}
		m.patents[idx].ReviewState = next
		m.logActivity(ActivityPatentReviewState, p.Number, next)
		updatedCount++
	}

	if updatedCount > 1 {
		m.message = fmt.Sprintf("updated status to %s for %d patents", next, updatedCount)
	} else if updatedCount == 1 {
		p := m.patents[indices[0]]
		m.message = fmt.Sprintf("%s → %s", p.Number, next)
	}

	m.visualMode = false
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
	activeCode := m.countryFilter

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
		m.countryFilter = EmptyFilter
		m.message = "country filter cleared"
	} else {
		m.countryFilter = code
		m.message = "filtering by country: " + code
	}

	m.setMode(viewList)
	return m.refreshList()
}
