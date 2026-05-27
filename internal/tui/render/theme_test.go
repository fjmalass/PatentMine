package render

import (
	"fmt"
	"patentmine/internal/domain"
)

// ExampleTheme_ReviewStateGlyph demonstrates the preferred way to look up
// review state glyphs using the domain type.
func ExampleTheme_ReviewStateGlyph() {
	theme := NewTheme()

	fmt.Println("active:      ", theme.ReviewStateGlyph(domain.ReviewStateActive))
	fmt.Println("under_review:", theme.ReviewStateGlyph(domain.ReviewStateUnderReview))
	fmt.Println("ignored:     ", theme.ReviewStateGlyph(domain.ReviewStateIgnored))
	fmt.Println("deleted:     ", theme.ReviewStateGlyph(domain.ReviewStateDeleted))
	fmt.Println("unknown:     ", theme.ReviewStateGlyph(domain.ReviewStateUnknown))

	// Output:
	// active:       ✅
	// under_review: 🔍
	// ignored:      ⬜
	// deleted:      ❌
	// unknown:      ?
}

// ExampleTheme_ReviewStateGlyphFromString shows the string-based fallback,
// which is mainly useful when reading raw state values from activity logs
// or other untyped sources.
func ExampleTheme_ReviewStateGlyphFromString() {
	theme := NewTheme()

	fmt.Println(theme.ReviewStateGlyphFromString("active"))
	fmt.Println(theme.ReviewStateGlyphFromString("under_review"))

	// Output:
	// ✅
	// 🔍
}

// ExampleTheme_IDSEntryStatusGlyph demonstrates lookup for the four real
// IDS entry statuses.
func ExampleTheme_IDSEntryStatusGlyph() {
	theme := NewTheme()

	fmt.Println("pending:  ", theme.IDSEntryStatusGlyph(domain.IDSEntryPending))
	fmt.Println("submitted:", theme.IDSEntryStatusGlyph(domain.IDSEntrySubmitted))
	fmt.Println("accepted: ", theme.IDSEntryStatusGlyph(domain.IDSEntryAccepted))
	fmt.Println("ignored:  ", theme.IDSEntryStatusGlyph(domain.IDSEntryIgnored))

	// Output:
	// pending:   ⏳
	// submitted: 📨
	// accepted:  ✅
	// ignored:   👻
}

// ExampleTheme_IDSEntryNoneGlyph shows the special glyph used when a patent
// has no curated IDS entry in the current project.
func ExampleTheme_IDSEntryNoneGlyph() {
	theme := NewTheme()
	fmt.Println(theme.IDSEntryNoneGlyph())
	// Output:
	// ⭕
}

// ExampleTheme_IDSEntryStatusGlyphFromString demonstrates the string version,
// including the conventional "none" value used in some UI paths.
func ExampleTheme_IDSEntryStatusGlyphFromString() {
	theme := NewTheme()

	fmt.Println("none:    ", theme.IDSEntryStatusGlyphFromString("none"))
	fmt.Println("accepted:", theme.IDSEntryStatusGlyphFromString("accepted"))

	// Output:
	// none:     ⭕
	// accepted: ✅
}

// ExampleTheme_FetchStateGlyph shows glyphs for patent data completeness states.
func ExampleTheme_FetchStateGlyph() {
	theme := NewTheme()

	fmt.Println("cached: ", theme.FetchStateGlyph(domain.FetchCached))
	fmt.Println("stub:   ", theme.FetchStateGlyph(domain.FetchStub))
	fmt.Println("unknown:", theme.FetchStateGlyph(domain.FetchUnknown))

	// Output:
	// cached:  🗃️
	// stub:    🦴
	// unknown: ?
}

// ExampleTheme_FetchStateGlyphFromString shows the string-based variant for
// fetch states.
func ExampleTheme_FetchStateGlyphFromString() {
	theme := NewTheme()
	fmt.Println(theme.FetchStateGlyphFromString("stub"))
	// Output:
	// 🦴
}
