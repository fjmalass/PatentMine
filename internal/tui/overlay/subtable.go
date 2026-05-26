package overlay

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/tui/render"
)

type subtableParams struct {
	Theme         render.Theme
	Columns       []render.TableColumn
	Page          *render.Paginator
	Total         int
	PageSize      int
	FocusActive   bool
	FocusedColIdx int
	ActiveSort    string
	SortAscending bool
	VisualMode    bool
	IsRowSelected func(absIdx int) bool
	IsRowMarked   func(absIdx int) bool
	MarkGlyph     string // override mark glyph (defaults to Theme.Glyphs.RowMark)
}

func renderSubtable(params subtableParams, maxW int, getCellValue func(absIdx, rowIdx, colIdx int) string) string {
	params.Page.SetTotal(params.Total)
	params.Page.SetPageSize(params.PageSize)
	start, end := params.Page.Window()

	cursor := params.Page.Cursor()
	focusActive := params.FocusActive
	tableParams := render.TableParams{
		Theme:         params.Theme,
		Columns:       params.Columns,
		RowCount:      end - start,
		FocusedColIdx: params.FocusedColIdx,
		ActiveSort:    params.ActiveSort,
		SortAscending: params.SortAscending,
		FocusActive:   focusActive,
		VisualMode:    params.VisualMode,
		MarkGlyph:     params.MarkGlyph,
		IsRowCursor: func(rowIdx int) bool {
			return focusActive && start+rowIdx == cursor
		},
	}
	if params.IsRowSelected != nil {
		tableParams.IsRowSelected = func(rowIdx int) bool {
			return params.IsRowSelected(start + rowIdx)
		}
	}
	if params.IsRowMarked != nil {
		tableParams.IsRowMarked = func(rowIdx int) bool {
			return params.IsRowMarked(start + rowIdx)
		}
	}

	return render.RenderTable(tableParams, maxW, func(rowIdx, colIdx int) string {
		return getCellValue(start+rowIdx, rowIdx, colIdx)
	})
}

func subtableStatus(page render.Paginator) string {
	if page.Total() == 0 {
		return "[0/0]"
	}
	return fmt.Sprintf("[%d/%d]", page.Cursor()+1, page.Total())
}

func handleSubtableMotionKey(page *render.Paginator, msg tea.KeyMsg, vimCount *int) bool {
	s := msg.String()
	if len(s) == 1 && s[0] >= '0' && s[0] <= '9' && (s != "0" || *vimCount > 0) {
		*vimCount = *vimCount*10 + int(s[0]-'0')
		return true
	}

	count := *vimCount
	*vimCount = 0

	switch s {
	case "j", "down":
		page.MoveDown(max(count, 1))
		return true
	case "k", "up":
		page.MoveUp(max(count, 1))
		return true
	case "ctrl+d", "pgdown":
		page.PageDown()
		return true
	case "ctrl+u", "pgup":
		page.PageUp()
		return true
	case "g", "home":
		page.NavTop(max(count, 1))
		return true
	case "G", "end":
		page.NavBottom(max(count, 1))
		return true
	}
	return false
}
