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
	Cursor        int          // highlighted row index (-1 if none)
	FocusedColIdx int          // focused sort column index (-1 if none)
	ActiveSort    string       // active database sorting key
	SortAscending bool         // ascending sort order
	FocusActive   bool         // whether this table has active input focus
	PrefixCursor  string       // custom active row prefix, e.g. "→ "
	PrefixNormal  string       // custom normal row prefix, e.g. "  "
	VisualMode    bool         // whether visual mode is active
	IsRowSelected func(rowIdx int) bool // returns whether a row should be highlighted as selected/visual
	IsRowMarked   func(rowIdx int) bool // returns whether a row is permanently marked (e.g. source item)
	PrefixMarked  string                // custom normal row prefix for marked rows, e.g. "⚑ " (defaults to "⚑ " if empty)
}

// RenderTable draws a mathematically padded, truncated, and styled table.
func RenderTable(params TableParams, maxW int, getCellValue func(rowIdx, colIdx int) string) string {
	var b strings.Builder

	// 1. Render Table Header
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

		style := params.Theme.Header.Underline(true)
		switch {
		case params.FocusActive && params.FocusedColIdx >= 0 && i == params.FocusedColIdx:
			style = params.Theme.FocusHeader.Underline(true)
		case isSorted:
			style = params.Theme.SortActive.Underline(true)
		}
		hdrParts = append(hdrParts, style.Render(Pad(Truncate(label, col.Width), col.Width)))
	}
	headerLine := params.PrefixNormal + strings.Join(hdrParts, " ")
	b.WriteString(Truncate(headerLine, maxW))
	b.WriteString("\n")

	// 2. Render Rows
	prefixMarked := params.PrefixMarked
	if prefixMarked == "" {
		prefixMarked = "⚑ "
	}

	for i := 0; i < params.RowCount; i++ {
		isSelectedRow := i == params.Cursor && params.FocusActive
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

		var rowParts []string
		for colIdx, col := range params.Columns {
			val := getCellValue(i, colIdx)
			cell := Pad(Truncate(val, col.Width), col.Width)

			// Determine cell style (focus col highlight vs. row base style)
			style := rowStyle
			isFocusedCol := params.FocusActive && params.FocusedColIdx >= 0 && colIdx == params.FocusedColIdx
			if isFocusedCol {
				style = focusedCellStyleExtended(params.Theme, i, isSelectedRow, isVisualRow, isMarkedRow)
			}
			rowParts = append(rowParts, style.Render(cell))
		}

		prefix := params.PrefixNormal
		if isSelectedRow {
			if isMarkedRow {
				prefix = "→⚑"
			} else {
				prefix = params.PrefixCursor
			}
		} else if isMarkedRow {
			prefix = prefixMarked
		}
		prefixStyled := rowStyle.Render(prefix)

		rowLine := prefixStyled + strings.Join(rowParts, " ")
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
