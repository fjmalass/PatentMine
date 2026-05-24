package render

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

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

func TestPaginatorNavTopBottomWithRepeat(t *testing.T) {
	p := NewPaginator(5)
	p.SetTotal(100)

	// Bare NavTop / NavBottom act like Top / Bottom.
	p.NavBottom(1)
	if p.Cursor() != 99 {
		t.Fatalf("NavBottom(1) cursor = %d, want 99", p.Cursor())
	}
	p.NavTop(1)
	if p.Cursor() != 0 {
		t.Fatalf("NavTop(1) cursor = %d, want 0", p.Cursor())
	}

	// A repeat > 1 jumps to that 1-based line (vim `10G` / `10gg`).
	p.NavBottom(10)
	if p.Cursor() != 9 {
		t.Fatalf("NavBottom(10) cursor = %d, want 9", p.Cursor())
	}
	p.NavTop(42)
	if p.Cursor() != 41 {
		t.Fatalf("NavTop(42) cursor = %d, want 41", p.Cursor())
	}

	// A repeat past the end clamps to the last row.
	p.NavBottom(9999)
	if p.Cursor() != 99 {
		t.Fatalf("NavBottom(9999) cursor = %d, want 99", p.Cursor())
	}
}

func TestPaginatorGotoLine(t *testing.T) {
	p := NewPaginator(5)
	p.SetTotal(20)

	p.GotoLine(7)
	if p.Cursor() != 6 {
		t.Fatalf("GotoLine(7) cursor = %d, want 6", p.Cursor())
	}
	// Out-of-range high clamps to last.
	p.GotoLine(500)
	if p.Cursor() != 19 {
		t.Fatalf("GotoLine(500) cursor = %d, want 19", p.Cursor())
	}
	// Zero / negative clamps to first.
	p.GotoLine(0)
	if p.Cursor() != 0 {
		t.Fatalf("GotoLine(0) cursor = %d, want 0", p.Cursor())
	}
}

func TestPaginatorHandleKeyVisualChords(t *testing.T) {
	p := NewPaginator(5)
	p.SetTotal(20)
	p.GotoLine(7)

	if !p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")}) {
		t.Fatal("expected g prefix to be consumed")
	}
	if !p.GPending() {
		t.Fatal("expected g prefix to be pending")
	}
	if !p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")}) {
		t.Fatal("expected gg to be consumed")
	}
	if p.Cursor() != 0 {
		t.Fatalf("gg cursor = %d, want 0", p.Cursor())
	}

	p.GotoLine(4)
	if !p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")}) {
		t.Fatal("expected g prefix to be consumed")
	}
	if !p.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}) {
		t.Fatal("expected ga to be consumed")
	}
	start, end, active := p.VisualRange()
	if !active || start != 0 || end != 19 {
		t.Fatalf("ga visual range = (%d,%d,%t), want (0,19,true)", start, end, active)
	}
	if p.Cursor() != 19 {
		t.Fatalf("ga cursor = %d, want 19", p.Cursor())
	}

	p.ClearVisual()
	if !p.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlA}) {
		t.Fatal("expected ctrl+a to be consumed")
	}
	start, end, active = p.VisualRange()
	if !active || start != 0 || end != 19 {
		t.Fatalf("ctrl+a visual range = (%d,%d,%t), want (0,19,true)", start, end, active)
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
