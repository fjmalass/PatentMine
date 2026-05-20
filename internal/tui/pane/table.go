package pane

import (
	"strconv"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// column widths for patent tables.
const (
	colIndex        = 4
	colNumber       = 16
	colInventor     = 18
	colExpires      = 10
	colTags         = 14
	colIDS          = 12
	colState        = 13
	colCount        = 8
	colGaps         = colCount - 1
	headerRows      = 2
	defaultPageSize = 10
)

// tableCol is one table column descriptor.
type tableCol struct {
	label   string
	sortKey domain.SortColumn // zero = not sortable
	width   int
}

// patentTableColumns returns the columns for a patent table.
func patentTableColumns(bodyWidth int, projectID domain.ProjectID) []tableCol {
	fixedW := colIndex + colNumber + colInventor + colExpires + colTags + colIDS + colState + colGaps
	titleW := max(bodyWidth-fixedW, 1)
	idsSort := domain.SortByIDS
	if projectID == "" {
		idsSort = ""
	}
	return []tableCol{
		{"#", "", colIndex},
		{"NUMBER", domain.SortByNumber, colNumber},
		{"TITLE", domain.SortByTitle, titleW},
		{"INVENTOR", domain.SortByInventor, colInventor},
		{"EXPIRES", domain.SortByExpires, colExpires},
		{"TAGS", "", colTags},
		{"IDS", idsSort, colIDS},
		{tableStateHeading(projectID), domain.SortByReviewState, colState},
	}
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
			style = theme.Selected
		case isSorted:
			style = theme.SortActive
		}
		b.WriteString(style.Render(render.Pad(render.Truncate(label, col.width), col.width)))
	}
	return b.String()
}

// renderTableRow formats one patent row across all columns.
func renderTableRow(row domain.PatentRow, cols []tableCol, projectID domain.ProjectID, absoluteIndex int) string {
	vals := [8]string{
		formatViewIndex(absoluteIndex),
		numberToShowRow(row).String(),
		row.Title,
		formatInventorsShort(row.Inventors),
		formatExpires(row.ExpirationDate),
		formatTags(row.Tags),
		formatIDSSummary(row.IDSEntry),
		tableStateText(row, projectID),
	}
	var b strings.Builder
	for i, col := range cols {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(render.Pad(render.Truncate(vals[i], col.width), col.width))
	}
	return b.String()
}

func renderStyledTableRow(theme render.Theme, row domain.PatentRow, cols []tableCol, projectID domain.ProjectID, absoluteIndex int) string {
	vals := [8]string{
		formatViewIndex(absoluteIndex),
		numberToShowRow(row).String(),
		row.Title,
		formatInventorsShort(row.Inventors),
		formatExpires(row.ExpirationDate),
		formatTags(row.Tags),
		formatIDSSummary(row.IDSEntry),
		styleStateText(theme, row, projectID),
	}
	var b strings.Builder
	for i, col := range cols {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(render.Pad(render.Truncate(vals[i], col.width), col.width))
	}
	return b.String()
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

func tableStateHeading(projectID domain.ProjectID) string {
	if projectID != "" {
		return "REVIEW STATE"
	}
	return "FETCH"
}

func tableStateText(row domain.PatentRow, projectID domain.ProjectID) string {
	if projectID != "" && row.ReviewState.Valid() {
		return string(row.ReviewState)
	}
	return string(row.FetchState)
}

func styleStateText(theme render.Theme, row domain.PatentRow, projectID domain.ProjectID) string {
	text := tableStateText(row, projectID)
	if projectID != "" && row.ReviewState.Valid() {
		switch row.ReviewState {
		case domain.ReviewStateUnderReview:
			return theme.Warn.Render(text)
		case domain.ReviewStateCached:
			return theme.Dim.Render(text)
		case domain.ReviewStateDeleted:
			return theme.Error.Render(text)
		}
		return text
	}
	switch row.FetchState {
	case domain.FetchCached:
		return theme.Dim.Render(text)
	case domain.FetchStub:
		return theme.MutedItalic.Render(text)
	default:
		return text
	}
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
