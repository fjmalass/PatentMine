package pane

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// column widths for patent tables.
const (
	colIndex          = 4
	colNumber         = 16
	colInventor       = 14
	colClassification = 12
	colExpires        = 8
	colCitations      = 5
	colCitedBy        = 5
	colParents        = 7
	colTags           = 10
	colIDS            = 8
	colState          = 10
	headerRows        = 2
	defaultPageSize   = 10
)

// tableCol is one table column descriptor.
type tableCol struct {
	key      domain.PatentTableColumnKey
	label    string
	sortKey  domain.SortColumn // zero = not sortable
	width    int
	cellType domain.ColumnCellType
}

// patentTableColumns returns the visible columns for a patent table.
func patentTableColumns(bodyWidth int, schema []domain.PatentTableColumn) []tableCol {
	lookup := make(map[domain.PatentTableColumnKey]domain.PatentTableColumn, len(schema))
	for _, col := range schema {
		lookup[col.Key] = col
	}
	column := func(key domain.PatentTableColumnKey, width int) tableCol {
		spec := lookup[key]
		sortKey := spec.SortKey
		if !spec.Sortable {
			sortKey = ""
		}
		if width == 0 {
			width = spec.Width
		}
		return tableCol{
			key:      key,
			label:    spec.Label,
			sortKey:  sortKey,
			width:    width,
			cellType: spec.CellType,
		}
	}

	var cols []tableCol
	switch {
	case bodyWidth >= 145:
		cols = []tableCol{
			column(domain.PatentColumnIndex, colIndex),
			column(domain.PatentColumnNumber, colNumber),
			column(domain.PatentColumnTitle, 0),
			column(domain.PatentColumnInventor, colInventor),
			column(domain.PatentColumnClassification, colClassification),
			column(domain.PatentColumnExpires, colExpires),
			column(domain.PatentColumnCitations, colCitations),
			column(domain.PatentColumnCitedBy, colCitedBy),
			column(domain.PatentColumnParents, colParents),
			column(domain.PatentColumnTags, colTags),
			column(domain.PatentColumnIDS, colIDS),
			column(domain.PatentColumnReviewState, colState),
		}

	case bodyWidth >= 130:
		cols = []tableCol{
			column(domain.PatentColumnIndex, colIndex),
			column(domain.PatentColumnNumber, colNumber),
			column(domain.PatentColumnTitle, 0),
			column(domain.PatentColumnInventor, colInventor),
			column(domain.PatentColumnClassification, colClassification),
			column(domain.PatentColumnExpires, colExpires),
			column(domain.PatentColumnCitations, colCitations),
			column(domain.PatentColumnCitedBy, colCitedBy),
			column(domain.PatentColumnParents, colParents),
			column(domain.PatentColumnIDS, colIDS),
			column(domain.PatentColumnReviewState, colState),
		}

	case bodyWidth >= 110:
		cols = []tableCol{
			column(domain.PatentColumnIndex, colIndex),
			column(domain.PatentColumnNumber, colNumber),
			column(domain.PatentColumnTitle, 0),
			column(domain.PatentColumnInventor, colInventor),
			column(domain.PatentColumnClassification, colClassification),
			column(domain.PatentColumnExpires, colExpires),
			column(domain.PatentColumnTags, colTags),
			column(domain.PatentColumnIDS, colIDS),
			column(domain.PatentColumnReviewState, colState),
		}

	case bodyWidth >= 80:
		cols = []tableCol{
			column(domain.PatentColumnIndex, colIndex),
			column(domain.PatentColumnNumber, colNumber),
			column(domain.PatentColumnTitle, 0),
			column(domain.PatentColumnInventor, 12),
			column(domain.PatentColumnClassification, 12),
			column(domain.PatentColumnExpires, colExpires),
			column(domain.PatentColumnReviewState, colState),
		}

	case bodyWidth >= 65:
		cols = []tableCol{
			column(domain.PatentColumnIndex, colIndex),
			column(domain.PatentColumnNumber, colNumber),
			column(domain.PatentColumnTitle, 0),
			column(domain.PatentColumnInventor, 10),
			column(domain.PatentColumnClassification, 10),
			column(domain.PatentColumnReviewState, colState),
		}

	default:
		cols = []tableCol{
			column(domain.PatentColumnIndex, colIndex),
			column(domain.PatentColumnNumber, colNumber),
			column(domain.PatentColumnTitle, 0),
			column(domain.PatentColumnReviewState, colState),
		}
	}

	fixedW := 0
	for _, col := range cols {
		fixedW += col.width
	}
	fixedW += len(cols) - 1

	cols[2].width = max(bodyWidth-fixedW, 1)
	return cols
}

// renderTableHeader builds the column header row with sort indicators and focus highlighting.
func renderTableHeader(theme render.Theme, cols []tableCol, activeSortKey domain.SortColumn, ascending bool, focusedColIdx int) string {
	var b strings.Builder
	for i, col := range cols {
		if i > 0 {
			b.WriteByte(' ')
		}
		label := col.label
		isSorted := col.sortKey == activeSortKey && activeSortKey != ""
		if isSorted {
			if ascending {
				label += " ▴"
			} else {
				label += " ▾"
			}
		}

		style := theme.Header
		switch {
		case focusedColIdx >= 0 && i == focusedColIdx:
			style = theme.FocusHeader
		case isSorted:
			style = theme.SortActive
		}
		b.WriteString(style.Render(render.Pad(render.Truncate(label, col.width), col.width)))
	}
	return b.String()
}

// patentCellValue returns the display string for a patent's specific column.
// classDescs (when non-nil) supplies cached classification descriptions keyed
// by lower-cased code; it enriches the Classification column with the first
// code's description.
func patentCellValue(theme render.Theme, row domain.PatentRow, col tableCol, projectID domain.ProjectID, absoluteIndex int, classDescs map[string]string) string {
	if col.cellType == domain.CellTypeState {
		if projectID != "" {
			return theme.ReviewStateGlyph(string(row.ReviewState))
		}
		return theme.FetchStateGlyph(string(row.FetchState))
	}

	switch col.key {
	case domain.PatentColumnIndex:
		return formatViewIndex(absoluteIndex)
	case domain.PatentColumnNumber:
		return numberToShowRow(row).String()
	case domain.PatentColumnTitle:
		return row.Title
	case domain.PatentColumnInventor:
		return formatInventorsShort(row.Inventors)
	case domain.PatentColumnClassification:
		return formatClassificationsShort(row.Classifications, classDescs)
	case domain.PatentColumnExpires:
		return formatExpires(row.ExpirationDate)
	case domain.PatentColumnCitations:
		return strconv.Itoa(row.CitationsCount)
	case domain.PatentColumnCitedBy:
		return strconv.Itoa(row.CitedByCount)
	case domain.PatentColumnParents:
		return strconv.Itoa(row.ParentsCount)
	case domain.PatentColumnTags:
		return formatTags(row.Tags)
	case domain.PatentColumnIDS:
		return formatIDSSummary(row.IDSEntry)
	default:
		return ""
	}
}

// renderTableRow formats one patent row across all columns.
func renderTableRow(theme render.Theme, row domain.PatentRow, cols []tableCol, projectID domain.ProjectID, absoluteIndex int, classDescs map[string]string) string {
	var b strings.Builder
	for i, col := range cols {
		if i > 0 {
			b.WriteByte(' ')
		}
		val := patentCellValue(theme, row, col, projectID, absoluteIndex, classDescs)
		b.WriteString(render.Pad(render.Truncate(val, col.width), col.width))
	}
	return b.String()
}

func renderStyledTableRow(theme render.Theme, row domain.PatentRow, cols []tableCol, projectID domain.ProjectID, absoluteIndex int, focusedColIdx int, selected bool, highlight HighlightKind, classDescs map[string]string) string {
	return renderGenericTableRow(theme, cols, absoluteIndex, focusedColIdx, selected, highlight, func(col tableCol) string {
		return patentCellValue(theme, row, col, projectID, absoluteIndex, classDescs)
	})
}

// renderGenericTableRow renders one row whose cell values are supplied by a
// callback. The callback receives each column descriptor in left-to-right order
// and returns the raw cell text. Padding, truncation, and focused-column
// styling match renderStyledTableRow so overlays that build their own column
// schema get the same visuals as the catalog/citations panes.
func renderGenericTableRow(theme render.Theme, cols []tableCol, absoluteIndex int, focusedColIdx int, selected bool, highlight HighlightKind, cellValue func(col tableCol) string) string {
	var b strings.Builder
	for i, col := range cols {
		if i > 0 {
			b.WriteByte(' ')
		}
		cell := render.Pad(render.Truncate(cellValue(col), col.width), col.width)
		if focusedColIdx >= 0 && i == focusedColIdx {
			cell = focusedCellStyleWithHighlight(theme, absoluteIndex, selected, highlight).Render(cell)
		}
		b.WriteString(cell)
	}
	return b.String()
}

func focusedCellStyleWithHighlight(theme render.Theme, absoluteIndex int, selected bool, highlight HighlightKind) lipgloss.Style {
	if selected {
		return theme.FocusSelected
	}

	switch highlight {
	case HighlightFamilyParent:
		return theme.FocusFamilyParent
	case HighlightFamilyChild:
		return theme.FocusFamilyChild
	case HighlightFamilyBoth:
		return theme.FocusFamilyBoth
	case HighlightFamilyAnchor, HighlightCitationAnchor:
		return theme.FocusRelationAnchor
	case HighlightCitationCites:
		return theme.FocusCitationCites
	case HighlightCitationCitedBy:
		return theme.FocusCitationCitedBy
	case HighlightCitationBoth:
		return theme.FocusCitationBoth
	}

	if absoluteIndex%2 != 0 {
		return theme.FocusCellAlt
	}
	return theme.FocusCell
}

func formatViewIndex(absoluteIndex int) string {
	return strconv.Itoa(max(absoluteIndex, 0) + 1)
}

func tableRowStyle(theme render.Theme, absoluteIndex int) func(...string) string {
	if absoluteIndex%2 != 0 {
		return theme.RowAlt.Render
	}
	return theme.Row.Render
}

func renderTableStatusLine(theme render.Theme, w, current, total int, extras ...string) string {
	status := "[0/0]"
	if total > 0 {
		status = "[" + strconv.Itoa(max(current, 0)+1) + "/" + strconv.Itoa(total) + "]"
	}
	parts := []string{status}
	for _, extra := range extras {
		extra = strings.TrimSpace(extra)
		if extra != "" {
			parts = append(parts, extra)
		}
	}
	return theme.Dim.Render(render.Pad(" "+strings.Join(parts, "  "), w))
}

func moveSortableColumn(cols []tableCol, current, step int) int {
	if len(cols) == 0 {
		return -1
	}
	hasSortable := false
	for _, col := range cols {
		if col.sortKey != "" {
			hasSortable = true
			break
		}
	}
	if !hasSortable {
		return -1
	}
	if current < -1 {
		current = -1
	}
	if current >= len(cols) {
		current = len(cols) - 1
	}
	idx := current
	if current < 0 && step < 0 {
		idx = 0
	}
	for range len(cols) {
		idx = (idx + step + len(cols)) % len(cols)
		if cols[idx].sortKey != "" {
			return idx
		}
	}
	return -1
}

func clampFocusedSortableColumn(cols []tableCol, focusedColIdx int) int {
	if focusedColIdx < 0 {
		return -1
	}
	if focusedColIdx >= 0 && focusedColIdx < len(cols) && cols[focusedColIdx].sortKey != "" {
		return focusedColIdx
	}
	return moveSortableColumn(cols, focusedColIdx, 1)
}

func tableStateText(row domain.PatentRow, projectID domain.ProjectID) string {
	if projectID != "" && row.ReviewState.Valid() {
		return string(row.ReviewState)
	}
	return string(row.FetchState)
}

func numberToShowRow(row domain.PatentRow) domain.PatentNumber {
	if !row.DisplayNumber.IsZero() {
		return row.DisplayNumber
	}
	return row.Number
}

// formatInventorsShort returns the first inventor's name, appending "et al."
// when there are multiple inventors.
func formatInventorsShort(inventors []domain.Inventor) string {
	if len(inventors) == 0 {
		return "-"
	}
	if len(inventors) == 1 {
		return string(inventors[0])
	}
	return string(inventors[0]) + " et al."
}

// formatExpires formats an expiration date for display; returns "-" when zero.
func formatExpires(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02")
}

// formatTags formats a patent's tag list for display; returns "-" when empty.
func formatTags(tags []string) string {
	if len(tags) == 0 {
		return "-"
	}
	return strings.Join(tags, " ")
}

func formatIDSSummary(entry *domain.IDSEntry) string {
	if entry == nil {
		return "-"
	}
	return entry.SummaryText()
}

// formatClassificationsShort formats a patent's classification list for
// display in a narrow column. With no cached descriptions it returns a
// comma-separated list of codes. When descriptions are available, the first
// code is shown as "CODE — description" with a "(+N)" suffix counting the
// remaining codes.
func formatClassificationsShort(classifications []string, descs map[string]string) string {
	if len(classifications) == 0 {
		return "-"
	}
	if len(descs) == 0 {
		return strings.Join(classifications, ", ")
	}
	first := classifications[0]
	desc := descs[strings.ToLower(first)]
	if desc == "" {
		desc = descs[first]
	}
	head := first
	if desc != "" {
		head = first + " — " + desc
	}
	if len(classifications) == 1 {
		return head
	}
	return fmt.Sprintf("%s (+%d)", head, len(classifications)-1)
}
