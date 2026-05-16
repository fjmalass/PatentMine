package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func (m *Model) activeMode() viewMode {
	if m.mode == viewHelpPopup {
		return previousModeOr(m, viewList)
	}
	return m.mode
}

func (m *Model) screenTitle() string {
	return screenTitleForMode(m.mode)
}

func screenTitleForMode(mode viewMode) string {
	if spec, ok := lookupModeSpec(mode); ok && spec.title != "" {
		return spec.title
	}
	return cases.Title(language.Und, cases.NoLower).String(string(mode))
}

func (m *Model) screenSubtitle() string {
	if spec, ok := lookupModeSpec(m.mode); ok && spec.subtitle != nil {
		return spec.subtitle(m)
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
	return screenColorForMode(m.mode)
}

func (m *Model) renderScreenHeader() string {
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.screenColor()))
	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle))
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.screenColor())).Render("PatentMine"))
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
		Foreground(lipgloss.Color(m.screenColor())).
		Padding(0, 1).
		Render(fmt.Sprintf(m.text.T(TextValueProjectTag), pName, m.ProjectID))
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
				Render(sepBullet + label)
			b.WriteString(badge)
		}
		break
	}

	// Unpaid invoice warning
	if count := m.unpaidCounts[m.ProjectID]; count > 0 {
		warn := lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorWarning)).
			Bold(true).
			Render(fmt.Sprintf(sepBullet+"%d unpaid", count))
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
			ellipsis := ""
			if len(parts) > maxBreadcrumbs {
				parts = parts[len(parts)-maxBreadcrumbs:]
				ellipsis = sepBreadcrumbEllipsis
			}
			trail := ellipsis + strings.Join(parts, sepBreadcrumb)
			b.WriteString("  ")
			b.WriteString(subtle.Render(fmt.Sprintf(m.text.T(TextValueBreadcrumbFormat), depth, trail)))
		}
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
			b.WriteString(summaryStyle.Render(m.text.T(TextValueProjectSummaryLead) + p.Summary))
			break
		}
	}

	// Render and pad each line of the header
	headerLines := strings.Split(b.String(), "\n")
	style := lipgloss.NewStyle().Width(m.width)

	var res strings.Builder
	for i, line := range headerLines {
		res.WriteString(style.Render(line))
		if i < len(headerLines)-1 {
			res.WriteString("\n")
		}
	}

	return res.String()
}

func (m *Model) renderPopup(title, content string) string {
	var b strings.Builder
	b.WriteString(m.renderPopupHeader(title))
	b.WriteString(content)
	return b.String()
}

func (m *Model) renderPopupHeader(label string) string {
	// Use standard surface background instead of themed background
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(m.screenColor())).
		Bold(true)

	res := titleStyle.Render(label)

	// Add search info if active
	query := m.popupSearchQuery
	if m.input.Focused() && strings.HasPrefix(m.input.Value(), "/") {
		query = strings.TrimPrefix(m.input.Value(), "/")
	}
	if m.popupSearchActive || query != "" {
		searchText := m.text.T(TextValueSearchLabel)
		if query != "" {
			searchText += query
		}
		searchStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorWarning)).
			Italic(true)
		res += searchStyle.Render(searchText)
	}

	// Hint style: regular weight, same background
	hint := BuildHelperLine(m.activeKeys, m.text)
	if hint != "" {
		hintStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorSubtle)).
			Padding(0, 0)
		res += "\n" + hintStyle.Render(hint)
	}

	// Add a separator rule to separate header from payload
	popupWidth := max(20, m.overlayContentWidth())
	rule := lipgloss.NewStyle().
		Foreground(lipgloss.Color(ColorSubtle)).
		Render(strings.Repeat(sepRuleChar, popupWidth))

	// Ensure all lines in the header have the correct background and width
	headerLines := strings.Split(res+"\n"+rule, "\n")
	style := lipgloss.NewStyle().Width(m.overlayContentWidth())

	var final strings.Builder
	for i, line := range headerLines {
		final.WriteString(style.Render(line))
		if i < len(headerLines)-1 {
			final.WriteString("\n")
		}
	}

	return final.String() + "\n\n"
}

func (m *Model) renderPopupTitle(label string) string {
	query := m.popupSearchQuery
	if m.input.Focused() && strings.HasPrefix(m.input.Value(), "/") {
		query = strings.TrimPrefix(m.input.Value(), "/")
	}

	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.screenColor()))
	res := accent.Render(label)

	if m.popupSearchActive || query != "" {
		searchText := strings.TrimSpace(m.text.T(TextValueSearchLabel))
		if query != "" {
			searchText += query
		}
		searchStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorBlack)).
			Bold(true).
			Padding(0, 1)
		res += "  " + searchStyle.Render(searchText)
	}
	return res
}
