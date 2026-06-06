package render

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// TableColumn defines a generic, model-agnostic column descriptor.
type TableColumn struct {
	Key     string
	Label   string
	Width   int
	SortKey string // empty if not sortable
}

// TableParams bundles all styling, sizing, cursor, and sorting parameters.
type TableParams struct {
	Theme         Theme
	Columns       []TableColumn
	RowCount      int
	FocusedColIdx int          // focused sort column index (-1 if none, disables col highlight)
	ActiveSort    string       // active database sorting key
	SortAscending bool         // ascending sort order
	FocusActive   bool         // whether this table has active input focus (col highlight)
	VisualMode    bool         // whether visual mode is active
	IsRowCursor   func(rowIdx int) bool // returns whether a row is under the cursor
	IsRowSelected func(rowIdx int) bool // returns whether a row is highlighted as selected/visual
	IsRowMarked   func(rowIdx int) bool // returns whether a row is permanently marked (e.g. source item)
	MarkGlyph     string                // override mark indicator glyph (defaults to Theme.Glyphs.RowMark)
}

// RenderTable draws a mathematically padded, truncated, and styled table.
func RenderTable(params TableParams, maxW int, getCellValue func(rowIdx, colIdx int) string) string {
	var b strings.Builder

	// 1. Render Table Header
	baseHeaderStyle := params.Theme.Header.Underline(true)
	headerPrefix := params.Theme.Glyphs.RowNoCursor + params.Theme.Glyphs.RowNoMark + " "
	prefixStyled := baseHeaderStyle.Render(headerPrefix)
	currentW := StringWidth(headerPrefix)

	var hdrParts []string
	for i, col := range params.Columns {
		label := col.Label
		isSorted := col.SortKey == params.ActiveSort && params.ActiveSort != ""
		if isSorted {
			if params.SortAscending {
				label += " ▴"
			} else {
				label += " ▾"
			}
		}

		style := baseHeaderStyle
		switch {
		case params.FocusActive && params.FocusedColIdx >= 0 && i == params.FocusedColIdx:
			style = params.Theme.FocusHeader.Underline(true)
		case isSorted:
			style = params.Theme.SortActive.Underline(true)
		}

		cell := Pad(Truncate(label, col.Width), col.Width)
		hdrParts = append(hdrParts, style.Render(cell))
		currentW += col.Width
		if i < len(params.Columns)-1 {
			hdrParts = append(hdrParts, baseHeaderStyle.Render(" "))
			currentW += 1
		}
	}
	var trailingSpaces string
	if currentW < maxW {
		trailingSpaces = baseHeaderStyle.Render(strings.Repeat(" ", maxW-currentW))
	}
	headerLine := prefixStyled + strings.Join(hdrParts, "") + trailingSpaces
	b.WriteString(Truncate(headerLine, maxW))
	b.WriteString("\n")

	// 2. Render Rows
	markGlyph := params.MarkGlyph
	if markGlyph == "" {
		markGlyph = params.Theme.Glyphs.RowMark
	}

	for i := 0; i < params.RowCount; i++ {
		isSelectedRow := params.IsRowCursor != nil && params.IsRowCursor(i)
		isVisualRow := params.IsRowSelected != nil && params.IsRowSelected(i)
		isMarkedRow := params.IsRowMarked != nil && params.IsRowMarked(i)

		rowStyle := params.Theme.Row
		if isSelectedRow && isMarkedRow {
			rowStyle = params.Theme.MarkedSelected
		} else if isSelectedRow {
			rowStyle = params.Theme.Selected
		} else if isVisualRow {
			rowStyle = params.Theme.Visual
		} else if isMarkedRow {
			if i%2 == 1 {
				rowStyle = params.Theme.MarkedAlt
			} else {
				rowStyle = params.Theme.Marked
			}
		} else if i%2 == 1 {
			rowStyle = params.Theme.RowAlt
		}

		gap := rowStyle.Render(" ")
		var parts []string
		for colIdx, col := range params.Columns {
			val := getCellValue(i, colIdx)
			cell := Pad(Truncate(val, col.Width), col.Width)

			// Determine cell style (focus col highlight vs. row base style)
			style := rowStyle
			isFocusedCol := params.FocusActive && params.FocusedColIdx >= 0 && colIdx == params.FocusedColIdx
			if isFocusedCol {
				style = focusedCellStyleExtended(params.Theme, i, isSelectedRow, isVisualRow, isMarkedRow)
			}
			if colIdx > 0 {
				parts = append(parts, gap)
			}
			parts = append(parts, style.Render(cell))
		}

		cursorPart := params.Theme.Glyphs.RowNoCursor
		if isSelectedRow {
			cursorPart = params.Theme.Glyphs.RowCursor
		}
		markPart := params.Theme.Glyphs.RowNoMark
		if isMarkedRow {
			markPart = markGlyph
		}
		prefix := cursorPart + markPart + " "
		prefixStyled := rowStyle.Render(prefix)

		rowLine := prefixStyled + strings.Join(parts, "")
		b.WriteString(rowLine)
		b.WriteString("\n")
	}

	return b.String()
}

// focusedCellStyleExtended calculates cell styling on the focused column/row, taking marked rows into account.
func focusedCellStyleExtended(theme Theme, absoluteIndex int, cursorSelected, visualSelected, marked bool) lipgloss.Style {
	if cursorSelected && marked {
		return theme.FocusMarkedSelectedCell
	}
	if cursorSelected || visualSelected {
		return theme.FocusSelected
	}
	if marked {
		return theme.FocusMarkedCell
	}
	if absoluteIndex%2 != 0 {
		return theme.FocusCellAlt
	}
	return theme.FocusCell
}

// MoveSortableColumn moves the focused column index by step (usually 1 or -1)
// among the sortable columns (those with a non-empty SortKey).
// It returns the new focused index, or -1 if no columns are sortable.
func MoveSortableColumn(cols []TableColumn, current, step int) int {
	if len(cols) == 0 {
		return -1
	}
	hasSortable := false
	for _, col := range cols {
		if col.SortKey != "" {
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
		if cols[idx].SortKey != "" {
			return idx
		}
	}
	return -1
}

// ClampFocusedSortableColumn ensures the focused column index is on a sortable
// column, or moves it to the first available sortable column.
func ClampFocusedSortableColumn(cols []TableColumn, focusedColIdx int) int {
	if focusedColIdx < 0 {
		return -1
	}
	if focusedColIdx >= 0 && focusedColIdx < len(cols) && cols[focusedColIdx].SortKey != "" {
		return focusedColIdx
	}
	return MoveSortableColumn(cols, focusedColIdx, 1)
}

// TableState manages the selection, focused column, and sorting state of a table view.
type TableState struct {
	FocusedColIdx int
	ActiveSort    string
	SortAscending bool
}

// NewTableState builds a default TableState.
func NewTableState() TableState {
	return TableState{
		FocusedColIdx: -1,
		SortAscending: true,
	}
}

// MoveFocus moves the focused column index by step (usually 1 or -1)
// among the sortable columns (those with a non-empty SortKey).
func (s *TableState) MoveFocus(cols []TableColumn, step int) {
	s.FocusedColIdx = MoveSortableColumn(cols, s.FocusedColIdx, step)
}

// ClampFocus ensures the focused column index is on a sortable column.
func (s *TableState) ClampFocus(cols []TableColumn) {
	s.FocusedColIdx = ClampFocusedSortableColumn(cols, s.FocusedColIdx)
}

// ApplySort applies sorting to the currently focused column.
// It returns true if the sorting state was successfully updated.
func (s *TableState) ApplySort(cols []TableColumn) bool {
	if s.FocusedColIdx < 0 || s.FocusedColIdx >= len(cols) {
		return false
	}
	col := cols[s.FocusedColIdx]
	if col.SortKey == "" {
		return false
	}
	if s.ActiveSort == col.SortKey {
		s.SortAscending = !s.SortAscending
	} else {
		s.ActiveSort = col.SortKey
		s.SortAscending = true
	}
	return true
}

