package render

import "github.com/charmbracelet/lipgloss"

// Theme colours, named so no raw colour code appears at a call site.
const (
	colorAccent   = "63"  // headings, highlights
	colorAltRow   = "235" // alternating row background
	colorFocus    = "22"  // focused sortable column background
	colorFocusAlt = "28"  // focused column on alternating rows
	colorFocusHdr = "29"  // focused column header background
	colorFocusSel = "35"  // focused column on selected rows
	colorSelected = "237" // selected-row background
	colorVisual   = "17"  // visual-range background (dark navy)
	colorDim      = "242" // de-emphasised text
	colorWarn     = "220" // warning / under-review text
	colorError    = "203" // error text
	colorOK       = "78"  // success text
	colorText     = "252" // default foreground

	// Marked states for source/flagged items
	colorMarked           = "99"  // soft violet text color for marked row
	colorMarkedBg         = "54"  // deep purple background for marked row
	colorMarkedAltBg      = "53"  // slightly darker deep purple background for alternate marked row
	colorMarkedSelBg      = "98"  // highlighted marked row background (hovered/cursor selected)
	colorMarkedFocusBg    = "55"  // focus cell background on marked rows
	colorMarkedFocusSelBg = "129" // focus cell background on marked row under cursor

	// Relation highlight backgrounds (Nordic Minimalist): tint the row
	// when relationship highlighting is toggled (g h or g c).

	// Family
	colorFamilyParent      = "95"  // deep terracotta/rose-brown
	colorFamilyChild       = "23"  // deep forest teal
	colorFamilyBoth        = "96"  // deep plum/vintage rose
	colorFamilyParentFocus = "131" // terracotta focus
	colorFamilyChildFocus  = "30"  // teal focus
	colorFamilyBothFocus   = "138" // rose focus

	// Citations
	colorCitationCites        = "24"  // deep slate blue
	colorCitationCitedBy      = "58"  // deep olive gold
	colorCitationBoth         = "101" // deep khaki
	colorCitationCitesFocus   = "60"  // slate blue focus
	colorCitationCitedByFocus = "100" // olive gold focus
	colorCitationBothFocus    = "107" // khaki focus

	// Unified relation anchor
	colorRelationAnchor      = "61" // deep lavender/indigo
	colorRelationAnchorFocus = "97" // lavender focus
)

// Default glyphs. Hoisted here so call sites never embed icon characters
// directly — swap the const, every status line and row marker follows.
const (
	glyphFamilyParent  = "↑"
	glyphFamilyChild   = "↓"
	glyphFamilyBoth    = "↕"
	glyphFamilyNone    = " "
	glyphFamilyLoading = "…"
	glyphFamilyAnchor  = "•"

	glyphCitationCites   = "→"
	glyphCitationCitedBy = "←"
	glyphCitationBoth    = "↔"
	glyphCitationNone    = " "
	glyphCitationLoading = "…"
	glyphCitationAnchor  = "•"

	glyphHistUnknown   = "❓"
	glyphHistSearch    = "🔻"
	glyphHistProject   = "📂"
	glyphHistCitations = "🔗"
	glyphHistFamily    = "🌳"
	glyphHistFulltext  = "📄"
	glyphHistIDS       = "📋"
	glyphHistPatent    = "👁️"
	glyphHistState     = "⚙️"
	glyphHistTagAdd    = "🏷️"
	glyphHistTagRemove = "➖"
	glyphHistColType   = "🔄"

	glyphReviewStateUnknown     = "?"
	glyphReviewStateUnderReview = "🔍"
	glyphReviewStateActive      = "✅"
	glyphReviewStateIgnored     = "☐"
	glyphReviewStateDeleted     = "❌"

	glyphFetchStateUnknown = "?"
	glyphFetchStateCached  = "🗃️"
	glyphFetchStateStub    = "🦴"
)

// Theme bundles the lipgloss styles the TUI draws with. One Theme is built at
// startup and shared; styles are values, so this is safe to copy.
type Theme struct {
	Title         lipgloss.Style
	Header        lipgloss.Style
	SortActive    lipgloss.Style
	FocusHeader   lipgloss.Style
	Row           lipgloss.Style
	RowAlt        lipgloss.Style
	Selected      lipgloss.Style
	FocusCell     lipgloss.Style
	FocusCellAlt  lipgloss.Style
	FocusSelected lipgloss.Style
	Visual        lipgloss.Style
	Dim           lipgloss.Style
	Info          lipgloss.Style
	Status        lipgloss.Style
	Error         lipgloss.Style
	OK            lipgloss.Style
	Warn          lipgloss.Style
	MutedItalic   lipgloss.Style
	HelpKey       lipgloss.Style
	Box           lipgloss.Style

	// Marked states
	Marked                  lipgloss.Style
	MarkedAlt               lipgloss.Style
	MarkedSelected          lipgloss.Style
	FocusMarkedCell         lipgloss.Style
	FocusMarkedSelectedCell lipgloss.Style

	// Family highlight overlays: applied to catalog/relations rows when the
	// user toggles "highlight family of anchor" with the g h keybinding.
	FamilyParent lipgloss.Style
	FamilyChild  lipgloss.Style
	FamilyBoth   lipgloss.Style

	// Blended family focus cells: applied to the focused sort column in highlighted family rows
	FocusFamilyParent lipgloss.Style
	FocusFamilyChild  lipgloss.Style
	FocusFamilyBoth   lipgloss.Style

	// Citation highlight overlays: applied to catalog/relations rows when the
	// user toggles "highlight citations of anchor" with the g c keybinding.
	CitationCites   lipgloss.Style
	CitationCitedBy lipgloss.Style
	CitationBoth    lipgloss.Style

	// Blended citation focus cells: applied to the focused sort column in highlighted citation rows
	FocusCitationCites   lipgloss.Style
	FocusCitationCitedBy lipgloss.Style
	FocusCitationBoth    lipgloss.Style

	// Unified relation anchor styles
	RelationAnchor      lipgloss.Style
	FocusRelationAnchor lipgloss.Style

	// Jump Overlay Styles
	JumpGlobalLabel lipgloss.Style
	JumpGlobalValue lipgloss.Style
	JumpLocalLabel  lipgloss.Style
	JumpLocalValue  lipgloss.Style

	// Glyphs holds the single-character markers used across panes. They live
	// on the theme so a build can override them (ASCII-only terminals, custom
	// icon set) without grepping for runes in pane code.
	Glyphs ThemeGlyphs
}

// ThemeGlyphs collects the icon strings used to mark rows and decorate status
// lines. Strings (not runes) so a deployment can replace a single glyph with
// a short label like "(P)" or "loading" without touching call sites.
type ThemeGlyphs struct {
	FamilyParent  string
	FamilyChild   string
	FamilyBoth    string
	FamilyNone    string
	FamilyLoading string
	FamilyAnchor  string

	CitationCites   string
	CitationCitedBy string
	CitationBoth    string
	CitationNone    string
	CitationLoading string
	CitationAnchor  string

	ReviewStateUnknown     string
	ReviewStateUnderReview string
	ReviewStateActive      string
	ReviewStateIgnored     string
	ReviewStateDeleted     string

	FetchStateUnknown string
	FetchStateCached  string
	FetchStateStub    string

	HistUnknown   string
	HistSearch    string
	HistProject   string
	HistCitations string
	HistFamily    string
	HistFulltext  string
	HistIDS       string
	HistPatent    string
	HistState     string
	HistTagAdd    string
	HistTagRemove string
	HistColType   string
}

// NewTheme builds the default theme.
func NewTheme() Theme {
	return Theme{
		Title: lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(colorAccent)),
		Header: lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(colorDim)),
		SortActive: lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(colorAccent)),
		Row: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)),
		RowAlt: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorAltRow)),
		Selected: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorSelected)).Bold(true),
		FocusHeader: lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorFocusHdr)),
		FocusCell: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorFocus)),
		FocusCellAlt: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorFocusAlt)),
		FocusSelected: lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorFocusSel)),
		Visual: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorVisual)),
		Dim: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorDim)),
		Info: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorAccent)),
		Status: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorDim)),
		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorError)),
		OK: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorOK)),
		Warn: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorWarn)),
		MutedItalic: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorDim)).Italic(true),
		HelpKey: lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(colorAccent)),
		Box: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorAccent)).
			Padding(0, 1),

		// Marked states
		Marked: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMarked)).
			Background(lipgloss.Color(colorMarkedBg)),
		MarkedAlt: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMarked)).
			Background(lipgloss.Color(colorMarkedAltBg)),
		MarkedSelected: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorMarkedSelBg)).Bold(true),
		FocusMarkedCell: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorMarkedFocusBg)),
		FocusMarkedSelectedCell: lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorMarkedFocusSelBg)),

		// Family highlight overlays
		FamilyParent: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorFamilyParent)),
		FamilyChild: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorFamilyChild)),
		FamilyBoth: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorFamilyBoth)).Bold(true),

		// Blended family focus cells
		FocusFamilyParent: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorFamilyParentFocus)),
		FocusFamilyChild: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorFamilyChildFocus)),
		FocusFamilyBoth: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorFamilyBothFocus)).Bold(true),

		// Citation highlight overlays
		CitationCites: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorCitationCites)),
		CitationCitedBy: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorCitationCitedBy)),
		CitationBoth: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorCitationBoth)).Bold(true),

		// Blended citation focus cells
		FocusCitationCites: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorCitationCitesFocus)),
		FocusCitationCitedBy: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorCitationCitedByFocus)),
		FocusCitationBoth: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorCitationBothFocus)).Bold(true),

		// Unified relation anchor styles
		RelationAnchor: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorRelationAnchor)).Bold(true),
		FocusRelationAnchor: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorRelationAnchorFocus)).Bold(true),

		// Jump Overlay Styles
		JumpGlobalLabel: lipgloss.NewStyle().Foreground(lipgloss.Color(colorText)).Bold(true),
		JumpGlobalValue: lipgloss.NewStyle().Foreground(lipgloss.Color(colorDim)),
		JumpLocalLabel:  lipgloss.NewStyle().Foreground(lipgloss.Color(colorWarn)).Bold(true),
		JumpLocalValue:  lipgloss.NewStyle().Foreground(lipgloss.Color(colorMarked)),

		Glyphs: ThemeGlyphs{
			FamilyParent:  glyphFamilyParent,
			FamilyChild:   glyphFamilyChild,
			FamilyBoth:    glyphFamilyBoth,
			FamilyNone:    glyphFamilyNone,
			FamilyLoading: glyphFamilyLoading,
			FamilyAnchor:  glyphFamilyAnchor,

			CitationCites:   glyphCitationCites,
			CitationCitedBy: glyphCitationCitedBy,
			CitationBoth:    glyphCitationBoth,
			CitationNone:    glyphCitationNone,
			CitationLoading: glyphCitationLoading,
			CitationAnchor:  glyphCitationAnchor,

			ReviewStateUnknown:     glyphReviewStateUnknown,
			ReviewStateUnderReview: glyphReviewStateUnderReview,
			ReviewStateActive:      glyphReviewStateActive,
			ReviewStateIgnored:     glyphReviewStateIgnored,
			ReviewStateDeleted:     glyphReviewStateDeleted,

			FetchStateUnknown: glyphFetchStateUnknown,
			FetchStateCached:  glyphFetchStateCached,
			FetchStateStub:    glyphFetchStateStub,

			HistUnknown:   glyphHistUnknown,
			HistSearch:    glyphHistSearch,
			HistProject:   glyphHistProject,
			HistCitations: glyphHistCitations,
			HistFamily:    glyphHistFamily,
			HistFulltext:  glyphHistFulltext,
			HistIDS:       glyphHistIDS,
			HistPatent:    glyphHistPatent,
			HistState:     glyphHistState,
			HistTagAdd:    glyphHistTagAdd,
			HistTagRemove: glyphHistTagRemove,
			HistColType:   glyphHistColType,
		},
	}
}

// ReviewStateGlyph returns the glyph corresponding to the review state.
func (t Theme) ReviewStateGlyph(state string) string {
	switch state {
	case "unknown", "":
		return t.Glyphs.ReviewStateUnknown
	case "under_review":
		return t.Glyphs.ReviewStateUnderReview
	case "active":
		return t.Glyphs.ReviewStateActive
	case "ignored":
		return t.Glyphs.ReviewStateIgnored
	case "deleted":
		return t.Glyphs.ReviewStateDeleted
	default:
		return t.Glyphs.ReviewStateUnknown
	}
}

// FetchStateGlyph returns the glyph corresponding to the fetch state.
func (t Theme) FetchStateGlyph(state string) string {
	switch state {
	case "cached":
		return t.Glyphs.FetchStateCached
	case "stub":
		return t.Glyphs.FetchStateStub
	case "unknown":
		return t.Glyphs.FetchStateUnknown
	default:
		return t.Glyphs.FetchStateUnknown
	}
}
