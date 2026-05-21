package tui

import (
	"fmt"
	"strings"

	"patentmine/internal/domain"
	"patentmine/internal/storage"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) styleRow(idx int, selected int, content string) string {
	return m.styleRowW(idx, selected, content, m.width)
}

func (m *Model) styleRowW(idx int, selected int, content string, targetWidth int) string {
	style := lipgloss.NewStyle()
	if m.isInSelection(idx) {
		style = style.Background(lipgloss.Color(ColorSelection))
	} else if idx == selected {
		style = style.Background(lipgloss.Color(ColorHighlight))
	} else if idx%2 != 0 {
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

// styleRowOverlay is like styleRowW but uses ColorSurface as the even-row base.
// Delegates style selection to overlayRowStyle so the logic is shared.
func (m *Model) styleRowOverlay(idx int, selected int, content string, targetWidth int) string {
	style := m.overlayRowStyle(idx, selected)
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

const (
	listColumnIDS = domain.SortColumnIDS
	listColPad    = 2 // gap between adjacent columns in all list/overlay views

	colHeaderNumber         = "Number"
	colHeaderTitle          = "Title"
	colHeaderInventor       = "Inventor"
	colHeaderClassification = "Classification"
	colHeaderExpires        = "Expires"
	colHeaderReview         = "Review"
	colHeaderUpdated        = "Updated"
	colHeaderNotes          = "Notes"
	colHeaderTags           = "Tags"
	colHeaderIDS            = "IDS"
	colHeaderSource         = "Source"
)

// fitColumns shrinks columns to fit within availableWidth (total pixels for all column widths + inter-column gaps combined).
func fitColumns(cols []listColumn, availableWidth int, minWidths map[string]int, shrinkOrder []string) []listColumn {
	if len(cols) == 0 || availableWidth <= 0 {
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

	for totalWidth(fitted) > availableWidth {
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
	numWidth := max(6, m.numberColWidth)
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
		{colHeaderNumber, numWidth, domain.SortColumnNumber, jumpLabelPublication},
		{colHeaderTitle, titleWidth, domain.SortColumnTitle, ""},
		{colHeaderInventor, invWidth, domain.SortColumnInventor, jumpLabelInventors},
		{colHeaderClassification, cpcWidth, domain.SortColumnCPC, jumpLabelClassification},
		{colHeaderExpires, expWidth, domain.SortColumnExpiration, jumpLabelExpiration},
		{colHeaderReview, reviewStateWidth, domain.SortColumnReviewState, keyReviewState},
		{colHeaderUpdated, updatedWidth, domain.SortColumnUpdated, jumpLabelUpdated},
		{colHeaderNotes, notesWidth, domain.SortColumnNotes, ksNotes.jump},
		{colHeaderTags, tagsWidth, domain.SortColumnTags, ksTags.jump},
		{colHeaderIDS, idsWidth, listColumnIDS, keyIDS},
	}
}

func (m *Model) fitListColumns(cols []listColumn, availableWidth int) []listColumn {
	if len(cols) == 0 || availableWidth <= 0 {
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
	return fitColumns(cols, availableWidth, minWidths, shrinkOrder)
}

func (m *Model) viewList() string {
	var b strings.Builder

	window := pageWindow(m.patentSelected, len(m.patents), m.pageSize())
	status := pageStatus(m.text.T(TextValuePageStatus), window)
	if m.listFilter.IsActive() && m.totalPatents > 0 {
		status += fmt.Sprintf("  (%d total)", m.totalPatents)
	}
	f := &m.listFilter
	if labels := f.activeFilterLabels(); len(labels) > 0 {
		status += sepFilterBar + strings.Join(labels, sepBullet)
	}

	subtleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle))
	b.WriteString(m.styleLine(subtleStyle.Render(status)) + "\n")
	b.WriteString(m.styleLine("") + "\n")

	if len(m.patents) == 0 {
		dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDim)).Italic(true)
		b.WriteString(m.styleLine(dimStyle.Render(m.text.T(TextListEmpty))) + "\n")
		return b.String()
	}
	idsByPatent := map[domain.PatentNumber]string{}
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

	header := m.pad(rowNoCursor, listRowPrefixWidth) +
		m.pad("", jumpPrefixWidth) +
		m.pad("#", idxWidth+2)

	b.WriteString(m.styleLine(m.renderTableHeader(header, cols)) + "\n")

	for i := window.Start; i < window.End; i++ {
		p := m.patents[i]
		prefix := rowNoCursor
		if i == m.patentSelected {
			prefix = rowCursor
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
			domain.SortColumnNumber:      string(p.Number),
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

		// Full row style (FG + bold + BG) computed upfront and applied ONCE to the
		// entire plain-text row so alternating backgrounds cover every character,
		// including trailing spaces.  highlightTerm uses targeted resets (\033[22;24m)
		// that preserve FG/BG, so highlights don't punch holes in the background.
		rowStyle := lipgloss.NewStyle()
		if color, ok := ReviewStateColors[p.ReviewState]; ok {
			rowStyle = rowStyle.Foreground(lipgloss.Color(color))
		}
		if m.isInSelection(i) {
			rowStyle = rowStyle.Background(lipgloss.Color(ColorSelection))
		} else if i == m.patentSelected {
			rowStyle = rowStyle.Bold(true).Background(lipgloss.Color(ColorHighlight))
		} else if i%2 != 0 {
			rowStyle = rowStyle.Background(lipgloss.Color(ColorAltRow))
		}

		idxLabel := fmt.Sprintf("%*d", idxWidth, i+1)
		row := m.pad(prefix, 2) +
			m.pad(jumpRowPrefix, jumpPrefixWidth) +
			m.pad(idxLabel, idxWidth+2)

		for j, c := range cols {
			val := rowValues[c.id]

			colWidth := c.width
			jumpColLabel := ""
			if m.jumpMode && j < len(m.jumpLabelsCache) {
				jumpColLabel = m.jumpLabelsCache[j].key
			}
			if jumpColLabel != "" {
				colWidth = max(colWidth, lipgloss.Width(jumpColLabel+" "+c.label))
			}
			padding := listColPad
			if j == len(cols)-1 {
				padding = 0
			}
			highlightQuery := m.listFilter.FreeFormSearch
			if m.listSearchActive && m.listSearchQuery != "" {
				highlightQuery = m.listSearchQuery
			}
			row += m.renderCell(val, colWidth, padding, highlightQuery)
		}

		// Pad to full width inside the style so BG covers trailing spaces too.
		if rw := lipgloss.Width(row); rw < m.width {
			row += strings.Repeat(" ", m.width-rw)
		}
		b.WriteString(rowStyle.Render(row) + "\n")
	}
	return b.String()
}

// renderTableHeader builds a styled header row from prefix + columns.
// Handles sort indicators, active-sort highlighting, and jump-mode prefixes.
// Shared by viewList, viewCitations, and viewReviewQueue.
func (m *Model) renderTableHeader(prefix string, cols []listColumn) string {
	header := prefix
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

		padding := listColPad
		if i == len(cols)-1 {
			padding = 0
		}
		header += m.pad(jumpColPrefix+style.Render(label), colWidth+padding)
	}
	return header
}

// renderCell truncates val to cellWidth, applies optional search highlight
// (using targeted ANSI resets that preserve surrounding FG/BG), then pads.
// No style is applied here — the caller wraps the whole row in one Render()
// so background/foreground cover the entire row including trailing spaces.
func (m *Model) renderCell(val string, cellWidth, padding int, query string) string {
	val = m.truncate(val, cellWidth)
	if query != "" {
		val = highlightTerm(val, query)
	}
	return m.pad(val, cellWidth+padding)
}

// overlayRowStyle returns the full lipgloss.Style for one row inside an overlay
// popup (background + optional foreground/bold for selection and search state).
func (m *Model) overlayRowStyle(idx, selected int) lipgloss.Style {
	style := overlayBase()
	if m.isInSelection(idx) {
		style = style.Background(lipgloss.Color(ColorSelection))
	} else if idx == selected {
		if m.isPopupSearchMode() && m.popupSearchQuery != "" {
			style = style.
				Background(lipgloss.Color(ColorWarning)).
				Foreground(lipgloss.Color(ColorBlack)).
				Bold(true)
		} else {
			style = style.Background(lipgloss.Color(ColorHighlight))
		}
	} else if idx%2 != 0 {
		style = style.Background(lipgloss.Color(ColorAltRow))
	}
	return style
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

// tabularColumns returns the column set for the current mode if it is a
// sortable tabular view (list, citations, review overlay), else (nil, false).
func (m *Model) tabularColumns() ([]listColumn, bool) {
	jumpPW := 0
	if m.hasJumpTargets() {
		jumpPW = listJumpWidth
	}
	switch {
	case m.mode == viewList:
		return m.listColumns(), true
	case m.isCitationView():
		avail := m.overlayContentWidth() - (listRowPrefixWidth + jumpPW + overlayIndexWidth)
		return m.citationColumns(avail), true
	case m.mode == viewReview:
		avail := m.overlayContentWidth() - (listRowPrefixWidth + jumpPW + overlayIndexWidth)
		return m.reviewOverlayColumns(avail), true
	default:
		return nil, false
	}
}

func (m *Model) moveSelection(delta int) *Model {
	count := m.activeItemCount()
	if count == 0 {
		return m
	}
	current := m.activeSelectionIndex()
	next := clamp(current+delta, 0, count-1)
	if m.mode == viewDetail || m.mode == viewPopupPatentDetail {
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
