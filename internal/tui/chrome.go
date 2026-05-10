package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"patentmine/internal/domain"
)

func (m Model) activeMode() viewMode {
	if (m.mode != viewHelpPopup && m.mode != viewNoteEdit) || len(m.backStack) == 0 {
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
	case viewFamily:
		return "Patent Family"
	case viewProjectIDS:
		return "IDS"
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
	case viewFamily:
		return "213"
	case viewProjectIDS:
		return "75"
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

	// Breadcrumb trail with depth counter. Each crumb is the patent number at
	// that level; crumbs without a patent context are skipped. Capped at 3
	// items for a stable width.  Format: [2] ‹ US10123456 › US20234567
	if len(m.backStack) > 0 {
		depth := len(m.backStack)
		var parts []string
		for _, snap := range m.backStack {
			if snap.current.Number != "" {
				parts = append(parts, snap.current.Number)
			}
		}
		if m.current.Number != "" {
			parts = append(parts, m.current.Number)
		}
		if len(parts) > 0 {
			const maxCrumbs = 3
			ellipsis := ""
			if len(parts) > maxCrumbs {
				parts = parts[len(parts)-maxCrumbs:]
				ellipsis = "… › "
			}
			trail := ellipsis + strings.Join(parts, " › ")
			b.WriteString("  ")
			b.WriteString(subtle.Render(fmt.Sprintf("[%d] ‹ %s", depth, trail)))
		}
	}

	// status:X only shown when user has changed from the default (stored).
	// The default stored-only view is implied and omitted to reduce noise.
	var filters []string
	if m.statusFilter != "" && m.statusFilter != domain.CitationStatusStored {
		filters = append(filters, "status:"+m.statusFilter)
	}
	if m.filter != EmptyFilter {
		filters = append(filters, fmt.Sprintf("filter:%s", m.filter))
	}
	if m.sortColumn != "" {
		sort := fmt.Sprintf("sort:%s %s", m.sortColumn, m.sortOrder)
		if m.sortColumn2 != "" {
			sort += "," + m.sortColumn2
		}
		filters = append(filters, sort)
	}

	if len(filters) > 0 {
		b.WriteString(" ")
		b.WriteString(subtle.Render("· " + strings.Join(filters, ", ")))
	}

	if m.isCitationView() && m.citesStatusFilter != "" {
		label := m.citesStatusFilter
		if label == domain.CitationStatusUnderReview {
			label = "under-review"
		}
		filters = append(filters, "refs:"+label)
	}
	if m.classFilter != EmptyFilter {
		classStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
		b.WriteString(" ")
		b.WriteString(subtle.Render("·"))
		b.WriteString(" ")
		b.WriteString(classStyle.Render("class:" + m.classFilter))
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

	// Project Summary
	for _, p := range m.projects {
		if p.ID == m.ProjectID && p.Summary != "" {
			b.WriteString("\n")
			summaryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDim)).Italic(true)
			b.WriteString(summaryStyle.Render("> " + p.Summary))
			break
		}
	}

	return b.String()
}

func (m Model) renderPopupTitle(label string) string {
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.screenAccent()))
	return accent.Render(label)
}
