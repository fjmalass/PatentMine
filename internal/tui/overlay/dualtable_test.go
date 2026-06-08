package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestDualTableNavigation(t *testing.T) {
	d := newDualTable()
	rows := [2]int{2, 5}  // associated has 2, all has 5
	cols := [2]int{3, 4}

	// Starts on the top (associated) pane.
	if d.Active() != 0 {
		t.Fatalf("active = %d, want 0", d.Active())
	}

	// j/k move within the active pane and clamp at the ends.
	d.HandleNav(key("j"), rows, cols)
	if d.Cursor() != 1 {
		t.Fatalf("after j cursor = %d, want 1", d.Cursor())
	}
	d.HandleNav(key("j"), rows, cols) // clamp at last row (index 1)
	if d.Cursor() != 1 {
		t.Fatalf("cursor should clamp at 1, got %d", d.Cursor())
	}

	// tab switches panes; each pane keeps its own cursor.
	if handled, _ := d.HandleNav(key("tab"), rows, cols); !handled || d.Active() != 1 {
		t.Fatalf("tab did not switch to pane 1 (active=%d)", d.Active())
	}
	if d.Cursor() != 0 {
		t.Fatalf("pane 1 cursor = %d, want 0", d.Cursor())
	}
	d.HandleNav(key("down"), rows, cols)
	d.HandleNav(key("down"), rows, cols)
	if d.Cursor() != 2 {
		t.Fatalf("pane 1 cursor = %d, want 2", d.Cursor())
	}
	// Switching back preserves pane 0's cursor.
	d.HandleNav(key("tab"), rows, cols)
	if d.Active() != 0 || d.Cursor() != 1 {
		t.Fatalf("pane 0 cursor not preserved: active=%d cursor=%d", d.Active(), d.Cursor())
	}

	// Column cursor wraps within the active pane's column count (3).
	d.HandleNav(key("left"), rows, cols)
	if d.colCursor[0] != 2 {
		t.Fatalf("left wrap col = %d, want 2", d.colCursor[0])
	}
	d.HandleNav(key("right"), rows, cols)
	if d.colCursor[0] != 0 {
		t.Fatalf("right wrap col = %d, want 0", d.colCursor[0])
	}
}

func TestDualTableSortToggle(t *testing.T) {
	d := newDualTable()
	rows := [2]int{3, 3}
	cols := [2]int{3, 3}

	// Move column cursor to col 1, then sort: ascending, sortRequested true.
	d.HandleNav(key("right"), rows, cols)
	handled, sortReq := d.HandleNav(key("."), rows, cols)
	if !handled || !sortReq {
		t.Fatalf("'.' handled=%v sortReq=%v, want both true", handled, sortReq)
	}
	if d.SortColumn() != 1 || d.SortDescending() {
		t.Fatalf("first sort: col=%d desc=%v, want 1/false", d.SortColumn(), d.SortDescending())
	}
	// Sorting the same column again toggles to descending.
	d.HandleNav(key("."), rows, cols)
	if d.SortColumn() != 1 || !d.SortDescending() {
		t.Fatalf("re-sort: col=%d desc=%v, want 1/true", d.SortColumn(), d.SortDescending())
	}
}

func TestDualTableClamp(t *testing.T) {
	d := newDualTable()
	// Push cursors out, then clamp to smaller counts.
	d.HandleNav(key("tab"), [2]int{9, 9}, [2]int{3, 3})
	for i := 0; i < 8; i++ {
		d.HandleNav(key("down"), [2]int{9, 9}, [2]int{3, 3})
	}
	d.clamp([2]int{9, 2}, [2]int{3, 3}) // pane 1 shrank to 2 rows
	if d.CursorIn(1) != 1 {
		t.Fatalf("clamp cursor = %d, want 1", d.CursorIn(1))
	}
}
