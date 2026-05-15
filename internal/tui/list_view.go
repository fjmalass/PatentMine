package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"patentmine/internal/domain"
	"patentmine/internal/storage"
)

func (m *Model) styleRow(index int, selected int, content string) string {
	return m.styleRowW(index, selected, content, m.width)
}

func (m *Model) styleRowW(index int, selected int, content string, targetWidth int) string {
	style := lipgloss.NewStyle()
	if m.isInSelection(index) {
		style = style.Background(lipgloss.Color(ColorSelection))
	} else if index == selected {
		style = style.Background(lipgloss.Color(ColorHighlight))
	} else if index%2 != 0 {
		style = style.Background(lipgloss.Color(ColorAltRow))
	}
	if targetWidth > 0 {
		cw := lipgloss.Width(content)
		if cw < targetWidth {
			content += strings.Repeat(" ", targetWidth-cw)
		}
	}
	return style.Render(content)
}

// styleRowOverlay is like styleRowW but uses ColorSurface as the even-row base
// so that all rows inside overlay popups have a consistent background.
func (m *Model) styleRowOverlay(index int, selected int, content string, targetWidth int) string {
	style := overlayBase()
	if m.isInSelection(index) {
		style = style.Background(lipgloss.Color(ColorSelection))
	} else if index == selected {
		if m.isPopupSearchMode() && m.popupSearchQuery != "" {
			style = style.
				Background(lipgloss.Color(ColorWarning)).
				Foreground(lipgloss.Color(ColorBlack)).
				Bold(true)
		} else {
			style = style.Background(lipgloss.Color(ColorHighlight))
		}
	} else if index%2 != 0 {
		style = style.Background(lipgloss.Color(ColorAltRow))
	}
	if targetWidth > 0 {
		cw := lipgloss.Width(content)
		if cw < targetWidth {
			content += strings.Repeat(" ", targetWidth-cw)
		}
	}
	return style.Render(content)
}

type listColumn struct {
	label     string
	width     int
	id        string
	jumpLabel string
}

const listColumnIDS = domain.SortColumnIDS

func fitColumns(cols []listColumn, available int, minWidths map[string]int, shrinkOrder []string) []listColumn {
	if len(cols) == 0 || available <= 0 {
		return cols
	}

	fitted := append([]listColumn(nil), cols...)
	totalWidth := func(columns []listColumn) int {
		total := 0
		for i, col := range columns {
			total += col.width
			if i < len(columns)-1 {
				total += 2
			}
		}
		return total
	}

	for totalWidth(fitted) > available {
		shrunk := false
		for _, id := range shrinkOrder {
			for i := range fitted {
				minWidth := minWidths[fitted[i].id]
				if fitted[i].id == id && fitted[i].width > minWidth {
					fitted[i].width--
					shrunk = true
					break
				}
			}
			if shrunk {
				break
			}
		}
		if !shrunk {
			break
		}
	}

	return fitted
}

func (m *Model) listColumns() []listColumn {
	numWidth := max(6, m.listNumWidth)
	titleWidth := 40
	invWidth := 20
	cpcWidth := 15
	expWidth := 12
	reviewStateWidth := 10
	idsWidth := 11
	updatedWidth := 16
	notesWidth := 6
	tagsWidth := 10

	return []listColumn{
		{"Number", numWidth, domain.SortColumnNumber, jumpLabelPublication},
		{"Title", titleWidth, domain.SortColumnTitle, ""},
		{"Inventor", invWidth, domain.SortColumnInventor, jumpLabelInventors},
		{"Classification", cpcWidth, domain.SortColumnCPC, jumpLabelClassification},
		{"Expires", expWidth, domain.SortColumnExpiration, jumpLabelExpiration},
		{"ReviewState", reviewStateWidth, domain.SortColumnReviewState, keyReviewState},
		{"Updated", updatedWidth, domain.SortColumnUpdated, jumpLabelUpdated},
		{"Notes", notesWidth, domain.SortColumnNotes, jumpLabelNotes},
		{"Tags", tagsWidth, domain.SortColumnTags, ""},
		{"IDS", idsWidth, listColumnIDS, keyIDS},
	}
}

func (m *Model) fitListColumns(cols []listColumn, available int) []listColumn {
	if len(cols) == 0 || available <= 0 {
		return cols
	}
	minWidths := map[string]int{
		domain.SortColumnNumber:      12,
		domain.SortColumnTitle:       18,
		domain.SortColumnInventor:    10,
		domain.SortColumnCPC:         8,
		domain.SortColumnExpiration:  10,
		domain.SortColumnReviewState: 8,
		domain.SortColumnUpdated:     10,
		domain.SortColumnNotes:       5,
		domain.SortColumnTags:        5,
		listColumnIDS:                7,
	}
	shrinkOrder := []string{
		domain.SortColumnTitle,
		domain.SortColumnInventor,
		domain.SortColumnCPC,
		domain.SortColumnUpdated,
		domain.SortColumnNumber,
		listColumnIDS,
		domain.SortColumnReviewState,
		domain.SortColumnExpiration,
		domain.SortColumnNotes,
		domain.SortColumnTags,
	}
	return fitColumns(cols, available, minWidths, shrinkOrder)
}

func (m *Model) viewList() string {
	if len(m.patents) == 0 {
		return m.text.T(TextListEmpty) + "\n"
	}
	var b strings.Builder

	window := pageWindow(m.selected, len(m.patents), m.pageSize())
	status := pageStatus(m.text.T(TextValuePageStatus), window)
	if m.listFilter.IsActive() && m.totalPatents > 0 {
		status += fmt.Sprintf("  (%d total)", m.totalPatents)
	}
	if labels := m.listFilter.Labels(); len(labels) > 0 {
		status += "  · " + strings.Join(labels, " · ")
	}

	subtleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle))
	b.WriteString(m.styleLine(subtleStyle.Render(status)) + "\n")
	b.WriteString(m.styleLine("") + "\n")
	idsByPatent := map[string]string{}
	if idsEntries, err := m.repo.ListIDSEntries(m.ctx, m.ProjectID); err == nil {
		for _, entry := range idsEntries {
			idsByPatent[entry.PatentNumber] = string(entry.Status)
		}
	}
	tagsByPatent, _ := m.repo.ListPatentTagsForProject(m.ctx, m.ProjectID)

	idxWidth := len(fmt.Sprintf("%d", len(m.patents)))
	if idxWidth < 2 {
		idxWidth = 2
	}

	// Account for jump prefix width in header if jump targets exist
	jumpPrefixWidth := 0
	if m.hasJumpTargets() {
		jumpPrefixWidth = 2
	}

	cols := m.listColumns()
	cols = m.fitListColumns(cols, m.width-(2+jumpPrefixWidth+idxWidth+2))

	// Clamp sortColumnIndex
	if m.sortColumnIndex >= len(cols) {
		m.sortColumnIndex = len(cols) - 1
	}

	header := m.pad("  ", 2) +
		m.pad("", jumpPrefixWidth) +
		m.pad("#", idxWidth+2)

	for i, c := range cols {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Underline(true)

		sortIndicator := ""
		if m.sortColumn == c.id {
			if m.sortOrder == domain.SortOrderDesc {
				sortIndicator = " ▾"
			} else {
				sortIndicator = " ▴"
			}
		}
		avail := c.width - lipgloss.Width(sortIndicator)
		if avail < 1 {
			avail = 1
		}
		displayLabel := c.label
		if lipgloss.Width(c.label) > avail {
			displayLabel = m.truncate(c.label, avail)
		}
		label := displayLabel + sortIndicator

		if i == m.sortColumnIndex {
			style = style.Foreground(lipgloss.Color(ColorYellow)).Underline(true).Bold(true)
		}

		jumpColLabel := ""
		if m.jumpMode {
			jumpColLabel = c.jumpLabel
		}
		jumpColPrefix := ""
		if jumpColLabel != "" {
			jumpColPrefix = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorYellow)).Render(jumpColLabel) + " "
		}

		colWidth := c.width
		if jumpColLabel != "" {
			colWidth = max(colWidth, lipgloss.Width(jumpColLabel+" "+label))
		}

		padding := 2
		if i == len(cols)-1 {
			padding = 0
		}
		header += m.pad(jumpColPrefix+style.Render(label), colWidth+padding)
	}

	b.WriteString(m.styleLine(header) + "\n")

	for i := window.Start; i < window.End; i++ {
		p := m.patents[i]
		prefix := "  "
		if i == m.selected {
			prefix = "> "
		}

		jumpRowPrefix := ""
		if m.jumpMode {
			jumpIdx := len(cols) + (i - window.Start)
			if jumpIdx < len(m.jumpLabelsCache) {
				label := m.jumpLabelsCache[jumpIdx]
				color := ColorYellow
				if !label.preferred {
					color = ColorThemeDetail
				}
				jumpRowPrefix = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(label.key) + " "
			}
		}
		if jumpRowPrefix == "" && jumpPrefixWidth > 0 {
			jumpRowPrefix = strings.Repeat(" ", jumpPrefixWidth)
		}

		rowValues := map[string]string{
			domain.SortColumnNumber:      p.Number,
			domain.SortColumnTitle:       p.Title,
			domain.SortColumnInventor:    formatInventorsShort(p.Inventors),
			domain.SortColumnCPC:         p.ClassificationLabel,
			domain.SortColumnExpiration:  p.ExpirationDate,
			domain.SortColumnReviewState: p.ReviewState,
			domain.SortColumnUpdated:     formatStoredTime(p.UpdatedAt, "-"),
			domain.SortColumnNotes:       "-",
			listColumnIDS:                "-",
		}
		if rowValues[domain.SortColumnCPC] == "" {
			rowValues[domain.SortColumnCPC] = "-"
		}
		if rowValues[domain.SortColumnExpiration] == "" {
			rowValues[domain.SortColumnExpiration] = "-"
		}
		if p.NotesCount > 0 {
			rowValues[domain.SortColumnNotes] = fmt.Sprintf("%d", p.NotesCount)
		}
		if tags, ok := tagsByPatent[p.Number]; ok {
			rowValues[domain.SortColumnTags] = formatTags(tags)
		} else {
			rowValues[domain.SortColumnTags] = "-"
		}
		if idsStatus, ok := idsByPatent[p.Number]; ok && idsStatus != "" {
			rowValues[listColumnIDS] = idsStatus
		}

		idxLabel := fmt.Sprintf("%*d", idxWidth, i+1)
		row := m.pad(prefix, 2) +
			m.pad(jumpRowPrefix, jumpPrefixWidth) +
			m.pad(idxLabel, idxWidth+2)

		for j, c := range cols {
			val := rowValues[c.id]

			// Use the same width calculation as the header for alignment
			colWidth := c.width
			jumpColLabel := ""
			if m.jumpMode && j < len(m.jumpLabelsCache) {
				jumpColLabel = m.jumpLabelsCache[j].key
			}
			if jumpColLabel != "" {
				colWidth = max(colWidth, lipgloss.Width(jumpColLabel+" "+c.label))
			}
			val = m.truncate(val, c.width)

			padding := 2
			if j == len(cols)-1 {
				padding = 0
			}
			row += m.pad(val, colWidth+padding)
		}

		style := lipgloss.NewStyle()
		if color, ok := ReviewStateColors[p.ReviewState]; ok {
			style = style.Foreground(lipgloss.Color(color))
		}

		if i == m.selected {
			style = style.Bold(true)
		}

		b.WriteString(m.styleRow(i, m.selected, style.Render(row)) + "\n")
	}
	return b.String()
}

func (m *Model) pad(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func formatInventorsShort(inventors []string) string {
	if len(inventors) == 0 {
		return "-"
	}
	if len(inventors) == 1 {
		return inventors[0]
	}
	return inventors[0] + " et al."
}

func (m *Model) truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if width <= 3 {
		if lipgloss.Width(s) <= width {
			return s
		}
		return strings.Repeat(".", width)
	}
	if lipgloss.Width(s) <= width {
		return s
	}

	var b strings.Builder
	currentWidth := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if currentWidth+rw > width-3 {
			b.WriteString("...")
			break
		}
		b.WriteRune(r)
		currentWidth += rw
	}
	return b.String()
}

func (m *Model) styleLine(content string) string {
	return lipgloss.NewStyle().Width(m.width).Render(content)
}

func (m *Model) moveSelection(delta int) *Model {
	count := m.activeItemCount()
	if count == 0 {
		return m
	}
	current := m.activeSelectionIndex()
	next := clamp(current+delta, 0, count-1)
	if m.mode == viewDetail {
		next = m.skipDetailSeparators(next, delta)
	}
	m.setActiveSelectionIndex(next)
	return m
}

func (m *Model) storedPatentCountForCountry(code string) int {
	if m.repo == nil || strings.TrimSpace(code) == "" {
		return 0
	}
	patents, err := m.repo.ListPatents(m.ctx, m.ProjectID, storage.ListPatentsOptions{
		CountryFilter:     code,
		ReviewStateFilter: domain.ReviewStateStored,
	})
	if err != nil {
		return 0
	}
	return len(patents)
}
