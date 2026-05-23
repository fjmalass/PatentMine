package pane

import (
	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// OverlayColumn is the public column descriptor used by overlays that drive
// their own tables (e.g. the per-patent classification popup). It mirrors the
// package-private tableCol but uses string keys so the overlay can name
// columns without coupling to domain.PatentTableColumnKey.
type OverlayColumn struct {
	Key      string
	Label    string
	SortKey  domain.SortColumn
	Width    int
	CellType domain.ColumnCellType
}

func (c OverlayColumn) toTableCol() tableCol {
	return tableCol{
		key:      domain.PatentTableColumnKey(c.Key),
		label:    c.Label,
		sortKey:  c.SortKey,
		width:    c.Width,
		cellType: c.CellType,
	}
}

func overlayColsToTableCols(cols []OverlayColumn) []tableCol {
	out := make([]tableCol, len(cols))
	for i, c := range cols {
		out[i] = c.toTableCol()
	}
	return out
}

// RenderOverlayTableHeader wraps renderTableHeader so overlays can draw a
// sort-aware column header identical to the catalog/citations panes.
func RenderOverlayTableHeader(theme render.Theme, cols []OverlayColumn, activeSort domain.SortColumn, ascending bool, focusedIdx int) string {
	return renderTableHeader(theme, overlayColsToTableCols(cols), activeSort, ascending, focusedIdx)
}

// RenderOverlayTableRow wraps renderGenericTableRow so overlays can render one
// row with the same pad/truncate/focus styling the catalog uses. The cellValue
// callback receives each OverlayColumn and returns the raw cell text.
func RenderOverlayTableRow(theme render.Theme, cols []OverlayColumn, absoluteIndex int, focusedIdx int, selected bool, cellValue func(col OverlayColumn) string) string {
	internal := overlayColsToTableCols(cols)
	return renderGenericTableRow(theme, internal, absoluteIndex, focusedIdx, selected, HighlightNone, func(col tableCol) string {
		for _, oc := range cols {
			if oc.Key == string(col.key) {
				return cellValue(oc)
			}
		}
		return ""
	})
}

// RenderOverlayStatusLine wraps renderTableStatusLine for overlays.
func RenderOverlayStatusLine(theme render.Theme, w, current, total int, extras ...string) string {
	return renderTableStatusLine(theme, w, current, total, extras...)
}

// MoveSortableOverlayColumn wraps moveSortableColumn so overlays can advance
// the focused column with the same skip-unsortable logic.
func MoveSortableOverlayColumn(cols []OverlayColumn, current, step int) int {
	return moveSortableColumn(overlayColsToTableCols(cols), current, step)
}
