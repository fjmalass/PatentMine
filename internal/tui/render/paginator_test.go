package render

import "testing"

func TestPaginatorMoveAndClamp(t *testing.T) {
	p := NewPaginator(5)
	p.SetTotal(20)

	p.MoveDown(3)
	if p.Cursor() != 3 {
		t.Fatalf("cursor = %d, want 3", p.Cursor())
	}
	// Cannot move above the first row.
	p.MoveUp(100)
	if p.Cursor() != 0 {
		t.Fatalf("cursor = %d, want 0 after clamping up", p.Cursor())
	}
	// Cannot move past the last row.
	p.MoveDown(100)
	if p.Cursor() != 19 {
		t.Fatalf("cursor = %d, want 19 after clamping down", p.Cursor())
	}
}

func TestPaginatorWindowFollowsCursor(t *testing.T) {
	p := NewPaginator(5)
	p.SetTotal(20)

	// Cursor within the first page: window starts at 0.
	p.MoveDown(2)
	if start, end := p.Window(); start != 0 || end != 5 {
		t.Fatalf("window = [%d,%d), want [0,5)", start, end)
	}
	// Cursor past the page: window scrolls so the cursor stays visible.
	p.MoveDown(5) // cursor at 7
	start, end := p.Window()
	if start != 3 || end != 8 {
		t.Fatalf("window = [%d,%d), want [3,8)", start, end)
	}
	if p.CursorInPage() != p.Cursor()-start {
		t.Fatalf("CursorInPage = %d, want %d", p.CursorInPage(), p.Cursor()-start)
	}
}

func TestPaginatorPageMoves(t *testing.T) {
	p := NewPaginator(10)
	p.SetTotal(100)

	p.PageDown()
	if p.Cursor() != 10 {
		t.Fatalf("cursor after PageDown = %d, want 10", p.Cursor())
	}
	p.PageUp()
	if p.Cursor() != 0 {
		t.Fatalf("cursor after PageUp = %d, want 0", p.Cursor())
	}
	p.Bottom()
	if p.Cursor() != 99 {
		t.Fatalf("cursor after Bottom = %d, want 99", p.Cursor())
	}
	p.Top()
	if p.Cursor() != 0 {
		t.Fatalf("cursor after Top = %d, want 0", p.Cursor())
	}
}

func TestPaginatorScrollTo(t *testing.T) {
	p := NewPaginator(5)
	p.SetTotal(20)

	// A target with room below it leads the window.
	p.ScrollTo(8)
	if p.Cursor() != 8 {
		t.Fatalf("cursor = %d, want 8", p.Cursor())
	}
	if start, end := p.Window(); start != 8 || end != 13 {
		t.Fatalf("window = [%d,%d), want [8,13)", start, end)
	}
	// A target near the end clamps the window to the list end.
	p.ScrollTo(18)
	if start, end := p.Window(); start != 15 || end != 20 {
		t.Fatalf("window = [%d,%d), want [15,20)", start, end)
	}
	// A negative target clamps to the first row.
	p.ScrollTo(-3)
	if p.Cursor() != 0 {
		t.Fatalf("cursor = %d, want 0 after clamping", p.Cursor())
	}
	// ScrollTo on an empty list is a no-op.
	p.SetTotal(0)
	p.ScrollTo(5)
	if start, end := p.Window(); start != 0 || end != 0 {
		t.Fatalf("window = [%d,%d), want [0,0) on an empty list", start, end)
	}
}

func TestPaginatorEmptyList(t *testing.T) {
	p := NewPaginator(10)
	p.SetTotal(0)
	p.MoveDown(5)
	if p.Cursor() != 0 {
		t.Fatalf("cursor = %d, want 0 on an empty list", p.Cursor())
	}
	if start, end := p.Window(); start != 0 || end != 0 {
		t.Fatalf("window = [%d,%d), want [0,0) on an empty list", start, end)
	}
}

func TestPaginatorShrinkingTotalKeepsCursorValid(t *testing.T) {
	p := NewPaginator(5)
	p.SetTotal(50)
	p.Bottom() // cursor 49
	p.SetTotal(10)
	if p.Cursor() != 9 {
		t.Fatalf("cursor = %d, want 9 after the list shrank", p.Cursor())
	}
	if start, _ := p.Window(); start < 0 || start > 9 {
		t.Fatalf("window start = %d, out of range after shrink", start)
	}
}
