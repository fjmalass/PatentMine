package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"patentmine/internal/domain"
)

func (m *Model) activeMode() viewMode {
	spec, ok := lookupModeSpec(m.mode)
	if !ok || !spec.isOverlay || spec.background == nil {
		return m.mode
	}
	return spec.background(m)
}

func (m *Model) screenTitle() string {
	return screenTitleForMode(m.activeMode())
}

func screenTitleForMode(mode viewMode) string {
	if spec, ok := lookupModeSpec(mode); ok && spec.title != "" {
		return spec.title
	}
	return strings.Title(string(mode))
}

func (m *Model) screenSubtitle() string {
	active := m.activeMode()
	switch active {
	case viewDetail, viewCites, viewCitedBy, viewClassifications, viewClassificationDetail, viewInventors, viewFamily, viewText, viewNotes, viewNoteEdit, viewIDSEdit, viewDateEdit, viewAbstract, viewClaim:
		if m.current.Number != "" {
			suffix := ""
			if active != viewDetail {
				suffix = " · " + m.current.Number
			}
			if strings.TrimSpace(m.current.Title) != "" {
				return m.current.Title + suffix
			}
			return m.current.Number
		}
	case viewPreview:
		if m.pendingBundle.Patent.Title != "" {
			return m.pendingBundle.Patent.Title
		}
	}
	return ""
}

func screenColorForMode(mode viewMode) string {
	if spec, ok := lookupModeSpec(mode); ok && spec.themeColor != "" {
		return spec.themeColor
	}
	return ColorThemeList
}

func (m *Model) screenColor() string {
	return screenColorForMode(m.activeMode())
}

func (m *Model) renderScreenHeader() string {
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.screenColor()))
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

func (m *Model) renderPopup(title, content string) string {
	var b strings.Builder
	b.WriteString(m.renderPopupHeader(title))
	b.WriteString(content)
	return b.String()
}

func (m *Model) renderPopupHeader(label string) string {
	spec := mustModeSpec(m.mode)

	// Style for the main title: themed background, bold
	titleStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(m.screenColor())).
		Foreground(lipgloss.Color(ColorBlack)).
		Bold(true).
		Padding(0, 1)

	res := titleStyle.Render(label)

	// Add search info if active
	query := m.popupSearchQuery
	if m.input.Focused() && strings.HasPrefix(m.input.Value(), "/") {
		query = strings.TrimPrefix(m.input.Value(), "/")
	}
	if m.popupSearchActive || query != "" {
		searchText := "search:/"
		if query != "" {
			searchText += query
		}
		// Search text style: slightly different but consistent
		searchStyle := lipgloss.NewStyle().
			Background(lipgloss.Color(ColorWhite)).
			Foreground(lipgloss.Color(ColorBlack)).
			Padding(0, 1).
			MarginLeft(1)
		res += searchStyle.Render(searchText)
	}

	// Style for the hint: same background as title, regular weight, tight to title
	if spec.helpHint != "" {
		hintStyle := lipgloss.NewStyle().
			Background(lipgloss.Color(m.screenColor())).
			Foreground(lipgloss.Color(ColorBlack)).
			Padding(0, 1)
		res += "\n" + hintStyle.Render(spec.helpHint)
	}

	return res + "\n\n"
}

func (m *Model) renderPopupTitle(label string) string {
	query := m.popupSearchQuery
	if m.input.Focused() && strings.HasPrefix(m.input.Value(), "/") {
		query = strings.TrimPrefix(m.input.Value(), "/")
	}

	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.screenColor()))
	res := accent.Render(label)

	if m.popupSearchActive || query != "" {
		searchText := "search:/"
		if query != "" {
			searchText += query
		}
		searchStyle := lipgloss.NewStyle().
			Background(lipgloss.Color(m.screenColor())).
			Foreground(lipgloss.Color(ColorBlack)).
			Bold(true).
			Padding(0, 1)
		res += "  " + searchStyle.Render(searchText)
	}
	return res
}
