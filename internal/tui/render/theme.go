package render

import "github.com/charmbracelet/lipgloss"

// Theme colours, named so no raw colour code appears at a call site.
const (
	colorAccent   = "63"  // headings, highlights
	colorSelected = "237" // selected-row background
	colorDim      = "242" // de-emphasised text
	colorError    = "203" // error text
	colorOK       = "78"  // success text
	colorText     = "252" // default foreground
)

// Theme bundles the lipgloss styles the TUI draws with. One Theme is built at
// startup and shared; styles are values, so this is safe to copy.
type Theme struct {
	Title    lipgloss.Style
	Header   lipgloss.Style
	Row      lipgloss.Style
	Selected lipgloss.Style
	Dim      lipgloss.Style
	Status   lipgloss.Style
	Error    lipgloss.Style
	OK       lipgloss.Style
	HelpKey  lipgloss.Style
	Box      lipgloss.Style
}

// NewTheme builds the default theme.
func NewTheme() Theme {
	return Theme{
		Title: lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(colorAccent)),
		Header: lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(colorDim)),
		Row: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)),
		Selected: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorText)).
			Background(lipgloss.Color(colorSelected)).Bold(true),
		Dim: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorDim)),
		Status: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorDim)),
		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorError)),
		OK: lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorOK)),
		HelpKey: lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color(colorAccent)),
		Box: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorAccent)).
			Padding(0, 1),
	}
}
