package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) activeMode() viewMode {
	if m.mode != viewHelpPopup || len(m.backStack) == 0 {
		return m.mode
	}
	return m.backStack[len(m.backStack)-1].mode
}

func (m Model) screenTitle() string {
	return screenTitleForMode(m.activeMode())
}

func screenTitleForMode(mode viewMode) string {
	switch mode {
	case viewList:
		return "Patent List"
	case viewDetail:
		return "Detail"
	case viewCites:
		return "Citations"
	case viewCitedBy:
		return "Cited By"
	case viewClassifications:
		return "Classifications"
	case viewText:
		return "Full Text"
	case viewNotes:
		return "Notes"
	case viewRefs:
		return "References"
	case viewAI:
		return "AI"
	case viewHelp:
		return "Help"
	case viewPreview:
		return "Reference Preview"
	case viewReview:
		return "Review Queue"
	case viewConfirmDelete:
		return "Confirm Delete"
	case viewClassificationDetail:
		return "Classification Detail"
	case viewInventors:
		return "Inventors"
	case viewHelpPopup:
		return "Help"
	default:
		return strings.Title(string(mode))
	}
}

func (m Model) screenSubtitle() string {
	switch m.activeMode() {
	case viewDetail:
		if m.current.Number != "" {
			if strings.TrimSpace(m.current.Title) != "" {
				return m.current.Title
			}
			return fmt.Sprintf("Detail · %s", m.current.Number)
		}
	}
	switch m.activeMode() {
	case viewList:
		return ""
	case viewPreview:
		if m.pendingBundle.Patent.Title != "" {
			return m.pendingBundle.Patent.Title
		}
		return ""
	default:
		return ""
	}
}

func screenAccentForMode(mode viewMode) string {
	switch mode {
	case viewList:
		return "39"
	case viewDetail:
		return "51"
	case viewCites:
		return "214"
	case viewCitedBy:
		return "40"
	case viewClassifications:
		return "170"
	case viewText:
		return "250"
	case viewNotes:
		return "220"
	case viewRefs:
		return "27"
	case viewAI:
		return "141"
	case viewHelp, viewHelpPopup:
		return "245"
	case viewPreview:
		return "81"
	case viewReview:
		return "202"
	case viewConfirmDelete:
		return "196"
	case viewClassificationDetail:
		return "170"
	case viewInventors:
		return "51"
	default:
		return "39"
	}
}

func (m Model) screenAccent() string {
	return screenAccentForMode(m.activeMode())
}

func (m Model) renderScreenHeader() string {
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.screenAccent()))
	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle))
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorTheme)).Render("PatentMine"))
	b.WriteString(" ")

	// Project details
	pName := m.ProjectID
	if m.ProjectID == "default" {
		pName = "Default"
	}
	// Try to find the name if we have projects loaded
	for _, p := range m.projects {
		if p.ID == m.ProjectID {
			pName = p.Name
			break
		}
	}

	projectTag := lipgloss.NewStyle().
		Background(lipgloss.Color(ColorSurface)).
		Foreground(lipgloss.Color(ColorTheme)).
		Padding(0, 1).
		Render(fmt.Sprintf("PROJECT: %s (%s)", pName, m.ProjectID))
	b.WriteString(projectTag)

	// Summary status badge
	for _, p := range m.projects {
		if p.ID != m.ProjectID {
			continue
		}
		if p.SummaryStatus != "" {
			label := p.SummaryStatus
			if l, ok := SummaryStatusLabels[p.SummaryStatus]; ok {
				label = l
			}
			color := ColorSubtle
			if c, ok := SummaryStatusColors[p.SummaryStatus]; ok {
				color = c
			}
			badge := lipgloss.NewStyle().
				Foreground(lipgloss.Color(color)).
				Bold(true).
				Render(" · " + label)
			b.WriteString(badge)
		}
		break
	}

	// Unpaid invoice warning
	if count := m.unpaidCounts[m.ProjectID]; count > 0 {
		warn := lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorWarning)).
			Bold(true).
			Render(fmt.Sprintf(" · %d unpaid", count))
		b.WriteString(warn)
	}

	b.WriteString(" ")
	b.WriteString(accent.Render(m.screenTitle()))

	var filters []string
	if m.filter != EmptyFilter {
		filters = append(filters, fmt.Sprintf("filter:%s", m.filter))
	}
	if m.classFilter != EmptyFilter {
		filters = append(filters, fmt.Sprintf("class:%s", m.classFilter))
	}
	if m.sortColumn != "" {
		filters = append(filters, fmt.Sprintf("sort:%s %s", m.sortColumn, m.sortOrder))
	}

	if len(filters) > 0 {
		b.WriteString(" ")
		b.WriteString(subtle.Render("· " + strings.Join(filters, ", ")))
	}

	if subtitle := strings.TrimSpace(m.screenSubtitle()); subtitle != "" {
		b.WriteString(" ")
		b.WriteString(subtle.Render("· " + subtitle))
	}

	if m.loading {
		b.WriteString("  ")
		b.WriteString(m.spinner.View())
		b.WriteString(" ")
		b.WriteString(subtle.Render(m.loadingMsg))
	}

	return b.String()
}

func (m Model) renderPopupTitle(label string) string {
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.screenAccent()))
	return accent.Render(label)
}
