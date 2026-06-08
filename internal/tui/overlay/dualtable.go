package overlay

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/tui/render"
)

// dualTablePane describes one of the two stacked tables a dualTable renders: its
// heading, columns, the number of (already sorted/filtered) rows the consumer
// holds, and how to render a cell. Keeping the row data in the consumer lets each
// entity (documents, patents) own its loading, sorting, and actions while the
// dualTable owns only the shared navigation and layout.
type dualTablePane struct {
	heading  string
	columns  []render.TableColumn
	rowCount int
	cell     func(row, col int) string
}

// dualTable is the shared "associated (top) / all (bottom)" two-pane scaffold
// behind the office-action document and patent association overlays. It owns the
// active pane, each pane's row cursor / scroll offset / column cursor, and the
// per-pane sort toggle, and routes the common navigation keys (tab to switch
// panes, j/k/↑/↓ to move, ←/→ to move the column cursor, ';' jump, '.' to sort).
// Entity-specific actions (assign, remove, open, …) and the row data stay with
// the consumer, which reads the active pane / cursor / column to act.
type dualTable struct {
	active    int    // 0 = top (associated), 1 = bottom (all)
	cursor    [2]int // selected row per pane
	offset    [2]int // scroll offset per pane
	colCursor  [2]int // focused column per pane
	sortCol    [2]int // last-sorted column per pane
	sortDesc   [2]bool
	lastWindow [2]int // last-rendered visible rows per pane, for page up/down
	jump       JumpNavigator
}

// newDualTable starts on the top pane, descending sort hint cleared.
func newDualTable() dualTable { return dualTable{} }

// Active reports the focused pane (0 = associated, 1 = all).
func (d *dualTable) Active() int { return d.active }

// SetActive focuses a pane.
func (d *dualTable) SetActive(pane int) { d.active = pane }

// Cursor reports the selected row index within the active pane.
func (d *dualTable) Cursor() int { return d.cursor[d.active] }

// CursorIn reports the selected row index within the given pane.
func (d *dualTable) CursorIn(pane int) int { return d.cursor[pane] }

// SortColumn / SortDescending report the active pane's current sort selection,
// for the consumer to order its rows.
func (d *dualTable) SortColumn() int      { return d.sortCol[d.active] }
func (d *dualTable) SortDescending() bool { return d.sortDesc[d.active] }

// Per-pane accessors for consumers (like the document overlay) that keep their
// own render but drive navigation through the shared controller.
func (d *dualTable) ColCursorIn(pane int) int  { return d.colCursor[pane] }
func (d *dualTable) SortColIn(pane int) int     { return d.sortCol[pane] }
func (d *dualTable) SortDescIn(pane int) bool   { return d.sortDesc[pane] }

// GutterPrefix returns the jump-mode line gutter for the active pane (empty when
// jump mode is off), so a consumer's own render can show jump targets.
func (d *dualTable) GutterPrefix(line int) string { return d.jump.GutterPrefix(line) }

// JumpActive reports whether jump-to-line mode is awaiting a target.
func (d *dualTable) JumpActive() bool { return d.jump.Active }

// JumpHint renders the jump-mode hint suffix for the active pane's cursor.
func (d *dualTable) JumpHint() string { return d.jump.HintSuffix(d.cursor[d.active], -1, false) }

// initSort seeds a pane's initial sort column and direction (call before first
// render). Lets a consumer preserve its historical default ordering.
func (d *dualTable) initSort(pane, col int, desc bool) {
	d.sortCol[pane] = col
	d.sortDesc[pane] = desc
}

// clamp keeps the cursor and column indices in range after the row counts change
// (e.g. a reload or a filter). rowCounts/colCounts are the current sizes of each
// pane.
func (d *dualTable) clamp(rowCounts, colCounts [2]int) {
	for p := 0; p < 2; p++ {
		d.cursor[p] = clampIndex(d.cursor[p], rowCounts[p])
		d.colCursor[p] = clampIndex(d.colCursor[p], colCounts[p])
		d.sortCol[p] = clampIndex(d.sortCol[p], colCounts[p])
	}
}

// HandleNav processes the shared navigation keys. rowCounts/colCounts give each
// pane's current sizes. It returns handled (the key was a navigation key and is
// consumed) and sortRequested (the user pressed '.', so the consumer should
// re-order the active pane by SortColumn/SortDescending). A key the dualTable
// does not own returns (false, false) so the consumer can handle it.
func (d *dualTable) HandleNav(msg tea.KeyMsg, rowCounts, colCounts [2]int) (handled, sortRequested bool) {
	if c, ok := d.jump.HandleKey(msg, d.cursor[d.active], rowCounts[d.active]); ok {
		d.cursor[d.active] = c
		return true, false
	}
	switch msg.String() {
	case "tab":
		d.active = 1 - d.active
		return true, false
	case "up", "k":
		d.move(-1, rowCounts)
		return true, false
	case "down", "j":
		d.move(1, rowCounts)
		return true, false
	case "pgdown", "ctrl+d":
		d.move(d.page(), rowCounts)
		return true, false
	case "pgup", "ctrl+u":
		d.move(-d.page(), rowCounts)
		return true, false
	case "left":
		d.colCursor[d.active] = wrapIndex(d.colCursor[d.active]-1, colCounts[d.active])
		return true, false
	case "right":
		d.colCursor[d.active] = wrapIndex(d.colCursor[d.active]+1, colCounts[d.active])
		return true, false
	case ";":
		d.jump.Active = true
		d.jump.PendingCount = 0
		d.jump.PendingG = false
		return true, false
	case ".":
		// Toggle direction when re-sorting the same column, else sort the focused
		// column ascending. The consumer reorders its rows on sortRequested.
		col := d.colCursor[d.active]
		if d.sortCol[d.active] == col {
			d.sortDesc[d.active] = !d.sortDesc[d.active]
		} else {
			d.sortCol[d.active] = col
			d.sortDesc[d.active] = false
		}
		return true, true
	}
	return false, false
}

func (d *dualTable) move(delta int, rowCounts [2]int) {
	n := rowCounts[d.active]
	if n == 0 {
		return
	}
	d.cursor[d.active] = max(0, min(d.cursor[d.active]+delta, n-1))
}

// page is the active pane's page step for PageUp/PageDown — one screenful less a
// row of overlap, from the last render (defaulting to 10 before the first render).
func (d *dualTable) page() int {
	w := d.lastWindow[d.active]
	if w < 2 {
		return 10
	}
	return w - 1
}

// SetWindow records a pane's visible row count for consumers that render the pane
// themselves (rather than via Render), so PageUp/PageDown have a page size.
func (d *dualTable) SetWindow(pane, rows int) { d.lastWindow[pane] = rows }

// Render lays out the two panes stacked with a divider, the active pane showing
// its column cursor. The top pane is given enough height for its rows (it is
// usually short — the items associated with the current office action), and the
// bottom pane takes the remainder with the active row scrolled into view.
func (d *dualTable) Render(theme render.Theme, w, h int, top, bottom dualTablePane) string {
	if h < 4 {
		h = 4
	}
	// Reserve: a heading + a divider line for each pane.
	topRowsAvail := max(min(top.rowCount, h/2-2), 1)
	bottomRowsAvail := max(h-topRowsAvail-4, 1)

	var b strings.Builder
	b.WriteString(d.renderPane(theme, w, top, 0, topRowsAvail))
	b.WriteByte('\n')
	b.WriteString(theme.Dim.Render(render.Truncate(strings.Repeat("─", max(w, 1)), max(w, 1))))
	b.WriteByte('\n')
	b.WriteString(d.renderPane(theme, w, bottom, 1, bottomRowsAvail))
	return b.String()
}

func (d *dualTable) renderPane(theme render.Theme, w int, pane dualTablePane, idx, rowsAvail int) string {
	var b strings.Builder
	heading := pane.heading
	if d.active == idx {
		b.WriteString(theme.Title.Render(render.Truncate(heading, w)))
	} else {
		b.WriteString(theme.Dim.Render(render.Truncate(heading, w)))
	}
	b.WriteByte('\n')

	// Scroll the active row into the visible window.
	d.lastWindow[idx] = rowsAvail
	d.offset[idx] = scrollOffset(d.offset[idx], d.cursor[idx], pane.rowCount, rowsAvail)
	start := d.offset[idx]
	end := min(start+rowsAvail, pane.rowCount)

	focusedCol := -1
	if d.active == idx {
		focusedCol = d.colCursor[idx]
	}
	body := render.RenderTable(render.TableParams{
		Theme:         theme,
		Columns:       pane.columns,
		RowCount:      end - start,
		FocusedColIdx: focusedCol,
		FocusActive:   d.active == idx,
		IsRowCursor:   func(r int) bool { return start+r == d.cursor[idx] && d.active == idx },
	}, w, func(r, c int) string {
		return pane.cell(start+r, c)
	})
	b.WriteString(body)
	return b.String()
}

// scrollOffset keeps cursor within [offset, offset+window).
func scrollOffset(offset, cursor, total, window int) int {
	if window <= 0 || total <= 0 {
		return 0
	}
	if cursor < offset {
		return cursor
	}
	if cursor >= offset+window {
		return cursor - window + 1
	}
	if offset > max(total-window, 0) {
		return max(total-window, 0)
	}
	return offset
}

func clampIndex(i, n int) int {
	if n <= 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	if i < 0 {
		return 0
	}
	return i
}

func wrapIndex(i, n int) int {
	if n <= 0 {
		return 0
	}
	return (i%n + n) % n
}
