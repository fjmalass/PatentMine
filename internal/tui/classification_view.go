package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) viewClassifications() string {
	if m.current.Number == "" {
		return overlayBase().Render("No patent open. Open a patent first.") + "\n"
	}
	classifications, err := m.repo.ListClassifications(m.ctx, m.ProjectID, m.current.Number)
	if err != nil {
		return overlayBase().Render(err.Error()) + "\n"
	}
	if len(classifications) == 0 {
		return overlayBase().Render("No CPC/USPC classification codes stored for "+string(m.current.Number)+".\n"+
			"Re-import the patent or run :"+commandRefreshRefsDetails+" to fetch row details.") + "\n"
	}

	selected := clamp(m.classificationSelected, 0, len(classifications)-1)
	m.classificationSelected = selected
	window := pageWindow(selected, len(classifications), m.overlayPageSize())

	status := pageStatus(m.text.T(TextValuePageStatus), window)
	if m.popupSearchActive && m.popupSearchQuery != "" {
		status += sepFilterBar + "/" + m.popupSearchQuery
	}

	var body strings.Builder
	body.WriteString(overlayBase().Foreground(lipgloss.Color(ColorSubtle)).Render(status))
	body.WriteString("\n\n")

	rowWidth := max(44, m.overlayContentWidth())
	codeWidth := 18
	descriptionWidth := max(20, rowWidth-overlayIndexWidth-codeWidth-2)

	header := m.pad(rowNoCursor, listRowPrefixWidth) +
		m.pad("#", overlayIndexWidth) +
		m.pad("Code", codeWidth) +
		m.pad("Description", descriptionWidth)

	body.WriteString(overlayBase().Foreground(lipgloss.Color(ColorSubtle)).Underline(true).Render(header))
	body.WriteString("\n")

	for i := window.Start; i < window.End; i++ {
		cls := classifications[i]
		prefix := rowNoCursor
		if i == selected {
			prefix = rowCursor
		}
		row := m.pad(prefix, listRowPrefixWidth) +
			m.pad(rowIndexLabel(i), overlayIndexWidth) +
			m.renderCell(cls.Code, codeWidth, listColPad, m.popupSearchQuery) +
			m.renderCell(cls.Description, descriptionWidth, 0, m.popupSearchQuery)
		if rw := lipgloss.Width(row); rw < rowWidth {
			row += strings.Repeat(" ", rowWidth-rw)
		}
		body.WriteString(m.overlayRowStyle(i, selected).Render(row) + "\n")
	}
	return m.renderPopup("Classifications · "+string(m.current.Number), body.String())
}

func (m *Model) viewClassificationDetail() string {
	base := overlayBase()
	boldStyle := base.Bold(true)

	classifications, err := m.repo.ListClassifications(m.ctx, m.ProjectID, m.current.Number)
	if err != nil {
		return base.Render("Error loading classifications: "+err.Error()) + "\n"
	}
	if len(classifications) == 0 {
		return base.Render("No classification data stored for "+string(m.current.Number)+".\nRun :refresh-refs-details or re-import the patent.") + "\n"
	}
	selected := clamp(m.classificationSelected, 0, len(classifications)-1)
	cls := classifications[selected]

	stats, _ := m.repo.GetClassificationStats(m.ctx, m.ProjectID, cls.Code)

	var body strings.Builder
	body.WriteString(base.Render(fmt.Sprintf("System: %s", cls.System)) + "\n")
	body.WriteString(base.Render(fmt.Sprintf("Code:   %s", cls.Code)) + "\n\n")

	if cls.System == "CPC" {
		body.WriteString(boldStyle.Render("Hierarchy:") + "\n")
		body.WriteString(base.Render(fmt.Sprintf("  Section:  %s", cls.Section)) + "\n")
		body.WriteString(base.Render(fmt.Sprintf("  Class:    %s", cls.Class)) + "\n")
		body.WriteString(base.Render(fmt.Sprintf("  Subclass: %s", cls.Subclass)) + "\n")
		body.WriteString(base.Render(fmt.Sprintf("  Group:    %s", cls.MainGroup)) + "\n")
		body.WriteString(base.Render(fmt.Sprintf("  Subgroup: %s", cls.Subgroup)) + "\n\n")
	} else if cls.System == "USPC" {
		body.WriteString(boldStyle.Render("Hierarchy:") + "\n")
		body.WriteString(base.Render(fmt.Sprintf("  Class:    %s", cls.Class)) + "\n")
		body.WriteString(base.Render(fmt.Sprintf("  Subclass: %s", cls.Subclass)) + "\n\n")
	}

	body.WriteString(boldStyle.Render("Description:") + "\n")
	body.WriteString(base.Render(cls.Description) + "\n\n")

	body.WriteString(boldStyle.Render("Patents in Project:") + "\n")

	renderCount := func(row classificationDetailRow, val int, style lipgloss.Style) {
		prefix := rowNoCursor
		if row == m.classificationDetailSelected {
			prefix = rowCursor
		}
		line := fmt.Sprintf("%-13s %d", classDetailRowLabel[row]+":", val)
		if row == m.classificationDetailSelected {
			body.WriteString(style.Bold(true).Render(prefix+line) + "\n")
		} else {
			body.WriteString(style.Render(prefix+line) + "\n")
		}
	}

	renderCount(classDetailRowTotal, stats.Total, base)
	renderCount(classDetailRowStored, stats.Stored, base.Foreground(lipgloss.Color(ColorSuccess)))
	renderCount(classDetailRowUnderReview, stats.UnderReview, base.Foreground(lipgloss.Color(ColorWarning)))
	renderCount(classDetailRowIgnored, stats.Ignored, base.Foreground(lipgloss.Color(ColorDim)))
	renderCount(classDetailRowCached, stats.Cached, base)

	return m.renderPopup(fmt.Sprintf("Classification"+sepBullet+"%s", cls.Code), body.String())
}
