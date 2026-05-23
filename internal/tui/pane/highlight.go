package pane

import (
	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// HighlightKind names a non-default styling overlay applied to one row in a
// patent list. A row can carry at most one kind at a time; callers pick the
// most informative kind when the same patent is reachable through multiple
// relationships (e.g. parent and child of the anchor).
type HighlightKind int

const (
	// HighlightNone is the absence of any overlay.
	HighlightNone HighlightKind = iota
	// HighlightFamilyParent: the row is a parent of the family anchor.
	HighlightFamilyParent
	// HighlightFamilyChild: the row is a child of the family anchor.
	HighlightFamilyChild
	// HighlightFamilyBoth: the row is both parent and child (cycle).
	HighlightFamilyBoth
	// HighlightGotoVisual: the row was part of the last saved visual selection.
	HighlightGotoVisual
)

// IsFamily reports whether the kind belongs to the family-highlight group.
func (k HighlightKind) IsFamily() bool {
	switch k {
	case HighlightFamilyParent, HighlightFamilyChild, HighlightFamilyBoth:
		return true
	default:
		return false
	}
}

// HighlightSet is a small map of patent number → HighlightKind. It centralises
// the row-styling overlays used by list panes (gv reselection, family-of-anchor
// hint, future search-hit / same-inventor highlights) so the View loop is one
// switch instead of a growing chain of conditionals.
type HighlightSet struct {
	byNumber map[domain.PatentNumber]HighlightKind
}

// Set records kind for n. Setting HighlightNone removes the entry.
func (h *HighlightSet) Set(n domain.PatentNumber, k HighlightKind) {
	if h == nil {
		return
	}
	if k == HighlightNone {
		delete(h.byNumber, n)
		return
	}
	if h.byNumber == nil {
		h.byNumber = make(map[domain.PatentNumber]HighlightKind)
	}
	h.byNumber[n] = k
}

// Upgrade promotes the entry for n: if it already carries an opposite
// family kind, the result becomes HighlightFamilyBoth. Otherwise behaves like Set.
func (h *HighlightSet) Upgrade(n domain.PatentNumber, k HighlightKind) {
	if h == nil || k == HighlightNone {
		return
	}
	cur := h.Kind(n)
	switch {
	case cur == HighlightFamilyParent && k == HighlightFamilyChild,
		cur == HighlightFamilyChild && k == HighlightFamilyParent:
		h.Set(n, HighlightFamilyBoth)
	default:
		h.Set(n, k)
	}
}

// Kind returns the kind recorded for n, or HighlightNone.
func (h *HighlightSet) Kind(n domain.PatentNumber) HighlightKind {
	if h == nil {
		return HighlightNone
	}
	return h.byNumber[n]
}

// Len reports the number of entries.
func (h *HighlightSet) Len() int {
	if h == nil {
		return 0
	}
	return len(h.byNumber)
}

// CountKind reports how many entries carry kind k. HighlightFamilyBoth counts
// for both parent and child queries so status summaries match the intuitive
// "this row is a parent" view.
func (h *HighlightSet) CountKind(k HighlightKind) int {
	if h == nil {
		return 0
	}
	n := 0
	for _, v := range h.byNumber {
		if v == k || (v == HighlightFamilyBoth && (k == HighlightFamilyParent || k == HighlightFamilyChild)) {
			n++
		}
	}
	return n
}

// Clear removes every entry.
func (h *HighlightSet) Clear() {
	if h == nil {
		return
	}
	h.byNumber = nil
}

// ClearFamily removes only family-related entries; gv reselection survives.
func (h *HighlightSet) ClearFamily() {
	if h == nil {
		return
	}
	for n, k := range h.byNumber {
		if k.IsFamily() {
			delete(h.byNumber, n)
		}
	}
}

// HasFamily reports whether any family-kind entry is present.
func (h *HighlightSet) HasFamily() bool {
	if h == nil {
		return false
	}
	for _, k := range h.byNumber {
		if k.IsFamily() {
			return true
		}
	}
	return false
}

// Style returns the lipgloss style that should paint the row for n, or
// zero+false when the default row style applies. Callers still resolve the
// Selected overlay separately because it is cursor-driven, not patent-keyed.
func (h *HighlightSet) Style(theme render.Theme, n domain.PatentNumber) (lipgloss.Style, bool) {
	switch h.Kind(n) {
	case HighlightFamilyParent:
		return theme.FamilyParent, true
	case HighlightFamilyChild:
		return theme.FamilyChild, true
	case HighlightFamilyBoth:
		return theme.FamilyBoth, true
	case HighlightGotoVisual:
		return theme.Visual, true
	}
	return lipgloss.Style{}, false
}

// Glyph returns the leading marker string for n, drawn from the theme so
// call sites never spell out the icon. Callers prepend it (plus a separating
// space) to the row so columns stay aligned whether or not a highlight is
// active.
func (h *HighlightSet) Glyph(theme render.Theme, n domain.PatentNumber) string {
	switch h.Kind(n) {
	case HighlightFamilyParent:
		return theme.Glyphs.FamilyParent
	case HighlightFamilyChild:
		return theme.Glyphs.FamilyChild
	case HighlightFamilyBoth:
		return theme.Glyphs.FamilyBoth
	}
	return theme.Glyphs.FamilyNone
}
