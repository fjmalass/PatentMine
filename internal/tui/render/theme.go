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
	colorMarked         = "99"  // soft violet text color for marked row
	colorMarkedBg       = "54"  // deep purple background for marked row
	colorMarkedAltBg    = "53"  // slightly darker deep purple background for alternate marked row
	colorMarkedSelBg    = "98"  // highlighted marked row background (hovered/cursor selected)
	colorMarkedFocusBg  = "55"  // focus cell background on marked rows
	colorMarkedFocusSelBg = "129" // focus cell background on marked row under cursor

	// Family highlight backgrounds: tint the row when a "highlight family"
	// toggle (H) marks it as a parent or child of the anchor patent.
	colorFamilyParent = "94" // dark amber — ancestor / parent application
	colorFamilyChild  = "25" // dark blue-cyan — descendant / child application
	colorFamilyBoth   = "89" // deep magenta — both parent and child (cycle)
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
)

// Theme bundles the lipgloss styles the TUI draws with. One Theme is built at
// startup and shared; styles are values, so this is safe to copy.
type Theme struct {
	Title       lipgloss.Style
	Header      lipgloss.Style
	SortActive  lipgloss.Style
	FocusHeader lipgloss.Style
	Row         lipgloss.Style
	RowAlt      lipgloss.Style
	Selected    lipgloss.Style
	FocusCell   lipgloss.Style
	FocusCellAlt lipgloss.Style
	FocusSelected lipgloss.Style
	Visual      lipgloss.Style
	Dim         lipgloss.Style
	Status      lipgloss.Style
	Error       lipgloss.Style
	OK          lipgloss.Style
	Warn        lipgloss.Style
	MutedItalic lipgloss.Style
	HelpKey     lipgloss.Style
	Box         lipgloss.Style

	// Marked states
	Marked             lipgloss.Style
	MarkedAlt          lipgloss.Style
	MarkedSelected     lipgloss.Style
	FocusMarkedCell    lipgloss.Style
	FocusMarkedSelectedCell lipgloss.Style

	// Family highlight overlays: applied to catalog/relations rows when the
	// user toggles "highlight family of anchor" with the H keybinding.
	FamilyParent lipgloss.Style
	FamilyChild  lipgloss.Style
	FamilyBoth   lipgloss.Style

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
		},
	}
}
