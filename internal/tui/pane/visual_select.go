package pane

// VisualState tracks vim-style visual-mode range selection over an integer
// row index space. It is intentionally row-type agnostic so overlays can reuse
// the same model the catalog/citations panes implement inline.
//
// Lifecycle: Toggle flips between off and on; when on, Anchor pins one end of
// the range and the caller-supplied cursor pins the other. SelectAll opens a
// full-list range. Clear resets to the zero value.
type VisualState struct {
	Mode   bool
	Anchor int
}

// Toggle flips visual mode on/off. When turning on, the cursor's current
// position becomes the anchor so a freshly opened range covers exactly the
// focused row.
func (v *VisualState) Toggle(cursor int) {
	if v.Mode {
		v.Mode = false
		v.Anchor = 0
		return
	}
	v.Mode = true
	v.Anchor = cursor
}

// SelectAll opens a visual range covering [0, total-1]. The caller is
// expected to move the cursor to total-1 so InRange returns true for every
// row.
func (v *VisualState) SelectAll(total int) {
	if total <= 0 {
		v.Clear()
		return
	}
	v.Mode = true
	v.Anchor = 0
}

// Clear leaves visual mode and forgets the anchor.
func (v *VisualState) Clear() {
	v.Mode = false
	v.Anchor = 0
}

// InRange reports whether row idx is between Anchor and cursor (inclusive) when
// visual mode is on. Returns false when off.
func (v *VisualState) InRange(cursor, idx int) bool {
	if !v.Mode {
		return false
	}
	lo, hi := v.Anchor, cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	return idx >= lo && idx <= hi
}
