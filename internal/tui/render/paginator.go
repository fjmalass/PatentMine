// Package render holds the TUI's reusable presentation pieces: the Paginator
// shared by every list pane, the colour theme, and layout helpers. None of it
// holds application state — panes embed these values.
package render

import (
	tea "github.com/charmbracelet/bubbletea"
)

// JumpAnchor is one labelled scroll target in a long pane: jump mode lists the
// anchors, and selecting one scrolls the pane straight to that line.
type JumpAnchor struct {
	// Key is the single key that selects this anchor in jump mode.
	Key rune
	// Label is the human-readable name shown in the jump overlay.
	Label string
	// Line is the 0-based body line the anchor scrolls to.
	Line int
	// Value is the current value of the field for display in the jump list.
	Value string
	// Local reports whether the field is project-scoped (local) rather than patent-scoped (global).
	Local bool
}

// Paginator tracks a cursor over a list and the visible window around it. Every
// list pane embeds one instead of re-deriving scroll arithmetic, so paging
// behaves identically across the catalog, citations, and project views.
type Paginator struct {
	total             int
	pageSize          int
	cursor            int // absolute index into the list, 0-based
	offset            int // absolute index of the first visible row
	visualMode        bool
	visualAnchor      int
	savedVisualAnchor int
	savedVisualCursor int
	hasSavedVisual    bool
	gPending          bool
}

// NewPaginator returns a Paginator with the given page size (clamped to >= 1).
func NewPaginator(pageSize int) Paginator {
	p := Paginator{pageSize: 1}
	p.SetPageSize(pageSize)
	return p
}

// SetTotal updates the list length, keeping the cursor and window valid.
func (p *Paginator) SetTotal(n int) {
	if n < 0 {
		n = 0
	}
	p.total = n
	p.clamp()
}

// SetPageSize updates the visible row count, keeping the window valid.
func (p *Paginator) SetPageSize(n int) {
	if n < 1 {
		n = 1
	}
	p.pageSize = n
	p.clamp()
}

// Total returns the list length.
func (p Paginator) Total() int { return p.total }

// PageSize returns the visible row count.
func (p Paginator) PageSize() int { return p.pageSize }

// Cursor returns the absolute cursor index.
func (p Paginator) Cursor() int { return p.cursor }

// Offset returns the absolute index of the first visible row.
func (p Paginator) Offset() int { return p.offset }

// CursorInPage returns the cursor's row within the visible window.
func (p Paginator) CursorInPage() int { return p.cursor - p.offset }

// Window returns the half-open range [start, end) of visible indices.
func (p Paginator) Window() (start, end int) {
	end = min(p.offset+p.pageSize, p.total)
	return p.offset, end
}

// MoveDown advances the cursor by n rows (n is clamped to >= 1).
func (p *Paginator) MoveDown(n int) { p.move(max(n, 1)) }

// MoveUp retreats the cursor by n rows (n is clamped to >= 1).
func (p *Paginator) MoveUp(n int) { p.move(-max(n, 1)) }

// PageDown moves the cursor down one page.
func (p *Paginator) PageDown() { p.move(p.pageSize) }

// PageUp moves the cursor up one page.
func (p *Paginator) PageUp() { p.move(-p.pageSize) }

// Top moves the cursor to the first row.
func (p *Paginator) Top() {
	p.cursor = 0
	p.clamp()
}

// Bottom moves the cursor to the last row.
func (p *Paginator) Bottom() {
	p.cursor = p.total - 1
	p.clamp()
}

// ScrollTo scrolls the window so index leads the visible rows, keeping the
// cursor on index. The offset is clamped so the window never runs past the
// list end — jump targets near the bottom still fill the page.
func (p *Paginator) ScrollTo(index int) {
	if p.total == 0 {
		p.cursor, p.offset = 0, 0
		return
	}
	p.cursor = min(max(index, 0), p.total-1)
	maxOffset := max(p.total-p.pageSize, 0)
	p.offset = min(max(index, 0), maxOffset)
}

func (p *Paginator) move(delta int) {
	p.cursor += delta
	p.clamp()
}

// clamp keeps the cursor inside the list and scrolls the window to follow it.
func (p *Paginator) clamp() {
	if p.total == 0 {
		p.cursor, p.offset = 0, 0
		return
	}
	p.cursor = min(max(p.cursor, 0), p.total-1)

	if p.cursor < p.offset {
		p.offset = p.cursor
	} else if p.cursor >= p.offset+p.pageSize {
		p.offset = p.cursor - p.pageSize + 1
	}
	maxOffset := max(p.total-p.pageSize, 0)
	p.offset = min(max(p.offset, 0), maxOffset)
}

// VisualMode returns whether visual mode is active.
func (p Paginator) VisualMode() bool {
	return p.visualMode
}

// ToggleVisual toggles visual mode.
func (p *Paginator) ToggleVisual() {
	if p.visualMode {
		p.SaveVisual()
		p.ClearVisual()
	} else {
		p.visualMode = true
		p.visualAnchor = p.cursor
	}
}

// ClearVisual exits visual mode.
func (p *Paginator) ClearVisual() {
	p.visualMode = false
	p.visualAnchor = 0
}

// SaveVisual captures the current visual selection boundaries.
func (p *Paginator) SaveVisual() {
	p.savedVisualAnchor = p.visualAnchor
	p.savedVisualCursor = p.cursor
	p.hasSavedVisual = true
}

// ReselectLast re-enters visual mode restoring the last saved selection.
// Returns true if a saved selection was successfully restored.
func (p *Paginator) ReselectLast() bool {
	if p.hasSavedVisual {
		p.visualMode = true
		p.visualAnchor = p.savedVisualAnchor
		p.cursor = p.savedVisualCursor
		p.clamp()
		return true
	}
	return false
}

// IsRowSelected returns whether the absolute index is within the visual selection range.
func (p Paginator) IsRowSelected(absIdx int) bool {
	if !p.visualMode {
		return false
	}
	lo := min(p.visualAnchor, p.cursor)
	hi := max(p.visualAnchor, p.cursor)
	return absIdx >= lo && absIdx <= hi
}

// VisualRange returns the start and end absolute indices (inclusive) of the visual selection.
// Returns active=false if visual mode is not active.
func (p Paginator) VisualRange() (start, end int, active bool) {
	if !p.visualMode {
		return 0, 0, false
	}
	return min(p.visualAnchor, p.cursor), max(p.visualAnchor, p.cursor), true
}

// HasSavedVisual returns whether there is a saved visual selection.
func (p Paginator) HasSavedVisual() bool {
	return p.hasSavedVisual
}

// SavedVisualAnchor returns the anchor index of the saved visual selection.
func (p Paginator) SavedVisualAnchor() int {
	return p.savedVisualAnchor
}

// SavedVisualCursor returns the cursor index of the saved visual selection.
func (p Paginator) SavedVisualCursor() int {
	return p.savedVisualCursor
}

// GPending returns whether a 'g' prefix key is pending.
func (p Paginator) GPending() bool {
	return p.gPending
}

// HandleKey handles standard visual mode keys on the paginator (v, g, gv, esc, q, Q).
// It returns true if the key was consumed by the paginator.
func (p *Paginator) HandleKey(msg tea.KeyMsg) bool {
	keyStr := msg.String()

	// If 'g' prefix is pending, handle 'v' to restore visual selection,
	// or clear the pending state for any other key.
	if p.gPending {
		p.gPending = false
		if keyStr == "v" {
			p.ReselectLast()
			return true
		}
	}

	switch keyStr {
	case "g":
		p.gPending = true
		return true

	case "v":
		p.ToggleVisual()
		return true

	case "esc", "q", "Q":
		if p.visualMode {
			p.SaveVisual()
			p.ClearVisual()
			return true
		}
	}
	return false
}
