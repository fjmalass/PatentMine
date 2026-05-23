package pane

import (
	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// HighlightGroup names a high-level grouping of relationship highlights
type HighlightGroup int

const (
	HighlightGroupNone HighlightGroup = iota
	HighlightGroupFamily
	HighlightGroupCitations
)

func (g HighlightGroup) String() string {
	switch g {
	case HighlightGroupFamily:
		return "family"
	case HighlightGroupCitations:
		return "citations"
	default:
		return "none"
	}
}

func (g HighlightGroup) Key() string {
	return g.String()
}

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

	// HighlightCitationCites: the row is cited by the anchor (anchor cites it).
	HighlightCitationCites
	// HighlightCitationCitedBy: the row cites the anchor (cites this patent).
	HighlightCitationCitedBy
	// HighlightCitationBoth: bidirectional citation cycle.
	HighlightCitationBoth

	// HighlightGotoVisual: the row was part of the last saved visual selection.
	HighlightGotoVisual
)

// Group returns the HighlightGroup this kind belongs to.
func (k HighlightKind) Group() HighlightGroup {
	switch k {
	case HighlightFamilyParent, HighlightFamilyChild, HighlightFamilyBoth:
		return HighlightGroupFamily
	case HighlightCitationCites, HighlightCitationCitedBy, HighlightCitationBoth:
		return HighlightGroupCitations
	default:
		return HighlightGroupNone
	}
}

// IsRelation reports whether the kind is a relationship-based highlight (family or citations).
func (k HighlightKind) IsRelation() bool {
	return k.Group() != HighlightGroupNone
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
// relation kind in the same group, the result becomes HighlightFamilyBoth
// or HighlightCitationBoth. Otherwise behaves like Set.
func (h *HighlightSet) Upgrade(n domain.PatentNumber, k HighlightKind) {
	if h == nil || k == HighlightNone {
		return
	}
	cur := h.Kind(n)
	switch {
	case cur == HighlightFamilyParent && k == HighlightFamilyChild,
		cur == HighlightFamilyChild && k == HighlightFamilyParent:
		h.Set(n, HighlightFamilyBoth)
	case cur == HighlightCitationCites && k == HighlightCitationCitedBy,
		cur == HighlightCitationCitedBy && k == HighlightCitationCites:
		h.Set(n, HighlightCitationBoth)
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

// CountKind reports how many entries carry kind k. Both states count
// for both queries so status summaries match the intuitive view.
func (h *HighlightSet) CountKind(k HighlightKind) int {
	if h == nil {
		return 0
	}
	n := 0
	for _, v := range h.byNumber {
		if v == k || (v == HighlightFamilyBoth && (k == HighlightFamilyParent || k == HighlightFamilyChild)) ||
			(v == HighlightCitationBoth && (k == HighlightCitationCites || k == HighlightCitationCitedBy)) {
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

// ClearRelation removes only relationship-related entries; gv reselection survives.
func (h *HighlightSet) ClearRelation() {
	if h == nil {
		return
	}
	for n, k := range h.byNumber {
		if k.IsRelation() {
			delete(h.byNumber, n)
		}
	}
}

// HasRelation reports whether any relationship-kind entry is present.
func (h *HighlightSet) HasRelation() bool {
	if h == nil {
		return false
	}
	for _, k := range h.byNumber {
		if k.IsRelation() {
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
	case HighlightCitationCites:
		return theme.CitationCites, true
	case HighlightCitationCitedBy:
		return theme.CitationCitedBy, true
	case HighlightCitationBoth:
		return theme.CitationBoth, true
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
	case HighlightCitationCites:
		return theme.Glyphs.CitationCites
	case HighlightCitationCitedBy:
		return theme.Glyphs.CitationCitedBy
	case HighlightCitationBoth:
		return theme.Glyphs.CitationBoth
	}
	return theme.Glyphs.FamilyNone
}
