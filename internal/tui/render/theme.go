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
	}
}
