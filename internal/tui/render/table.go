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
	FocusedColIdx int                   // focused sort column index (-1 if none, disables col highlight)
	ActiveSort    string                // active database sorting key
	SortAscending bool                  // ascending sort order
	FocusActive   bool                  // whether this table has active input focus (col highlight)
	VisualMode    bool                  // whether visual mode is active
	IsRowCursor   func(rowIdx int) bool // returns whether a row is under the cursor
	IsRowSelected func(rowIdx int) bool // returns whether a row is highlighted as selected/visual
	IsRowMarked   func(rowIdx int) bool // returns whether a row is permanently marked (e.g. source item)
	MarkGlyph     string                // override mark indicator glyph (defaults to Theme.Glyphs.RowMark)

	// ForceExactWidth, when true, makes RenderTable guarantee that every
	// data row (and header) has exactly TargetWidth display columns.
	// It does this by padding the entire row after layout, rather than
	// trying to absorb the remainder into any specific column.
	// This is useful for cases where the table must sit inside a larger
	// container (like stats subtables) and the right edge must be perfectly
	// straight even when the last column contains double-width icons.
	ForceExactWidth bool
	TargetWidth     int
}

// RenderTable draws a mathematically padded, truncated, and styled table.
//
// When ForceExactWidth is true and TargetWidth > 0, every data row (and the
// header) is forced to exactly TargetWidth display columns by appending
// padding after the last column. This is the recommended mode when the
// table must live inside a larger container (e.g. stats subtables) and
// the right edge must stay perfectly straight even if the last column
// contains double-width icons.
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
	headerTarget := maxW
	if params.ForceExactWidth && params.TargetWidth > 0 {
		headerTarget = params.TargetWidth
	}
	if currentW < headerTarget {
		trailingSpaces = baseHeaderStyle.Render(strings.Repeat(" ", headerTarget-currentW))
	}
	headerLine := prefixStyled + strings.Join(hdrParts, "") + trailingSpaces
	b.WriteString(Truncate(headerLine, headerTarget))
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

		// Determine the width target for this row.
		// When ForceExactWidth is set, we force the entire row to TargetWidth
		// by padding after the last column. This is the preferred way to
		// guarantee right-edge alignment when the last column may contain
		// double-width icons.
		target := maxW
		if params.ForceExactWidth && params.TargetWidth > 0 {
			target = params.TargetWidth
		}

		used := StringWidth(rowLine)
		if target > used {
			filler := rowStyle.Render(strings.Repeat(" ", target-used))
			rowLine += filler
		} else if target < used && params.ForceExactWidth {
			// If we're forcing exact width and the row is too wide,
			// truncate the whole row (rare, but keeps behavior predictable).
			rowLine = Truncate(rowLine, target)
		}

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
