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
	for i := 0; i < params.RowCount; i++ {
		isSelectedRow := i == params.Cursor && params.FocusActive
		rowStyle := params.Theme.Row
		if isSelectedRow {
			rowStyle = params.Theme.Selected
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
				style = focusedCellStyle(params.Theme, i, isSelectedRow)
			}
			rowParts = append(rowParts, style.Render(cell))
		}

		prefix := params.PrefixNormal
		if isSelectedRow {
			prefix = params.PrefixCursor
		}
		prefixStyled := rowStyle.Render(prefix)

		rowLine := prefixStyled + strings.Join(rowParts, " ")
		b.WriteString(rowLine)
		b.WriteString("\n")
	}

	return b.String()
}

// focusedCellStyle calculates cell styling on the focused column/row.
func focusedCellStyle(theme Theme, absoluteIndex int, selected bool) lipgloss.Style {
	if selected {
		return theme.FocusSelected
	}
	if absoluteIndex%2 != 0 {
		return theme.FocusCellAlt
	}
	return theme.FocusCell
}
