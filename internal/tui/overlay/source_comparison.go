// SourceComparisonOverlay provides a split-screen view for reviewing and
// choosing between conflicting values from different sources (USPTO vs Google)
// for a patent's root bibliographic data.
//
// Triggered when SourceDiffs exist for the current patent (especially after
// a Google-primary success in compare mode, now that we do best-effort USPTO
// enrichment).
//
// Default choice is always USPTO (as requested).
package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

const (
	// chosenSourceFormat is the generic prefix used to indicate which source
	// was chosen for a given field in the comparison overlay. It is used for
	// any source (not just USPTO/Google) so the UI stays generic.
	chosenSourceFormat = "[%s]"
)

// SourceComparisonResolveMsg is emitted when the user accepts choices in the
// comparison overlay. The app/engine should persist the chosen values for the
// conflicting fields (and the ChosenSource on the diffs).
type SourceComparisonResolveMsg struct {
	Patent  domain.PatentNumber
	Diffs   []domain.SourceDiff // with updated ChosenValue / ChosenSource
	Default domain.Source       // the default the user accepted (USPTO)
}

type SourceComparisonOverlay struct {
	theme  render.Theme
	patent domain.PatentNumber
	diffs  []domain.SourceDiff

	cursor int // which diff row is focused
}

func NewSourceComparisonOverlay(theme render.Theme, patent domain.PatentNumber, diffs []domain.SourceDiff) *SourceComparisonOverlay {
	return &SourceComparisonOverlay{
		theme:  theme,
		patent: patent,
		diffs:  diffs,
	}
}

func (o *SourceComparisonOverlay) Title() string {
	return fmt.Sprintf("Source Comparison — %s (default: USPTO)", o.patent)
}

func (o *SourceComparisonOverlay) Command(command.ID, int) (Overlay, tea.Cmd) {
	return o, nil
}

func (o *SourceComparisonOverlay) Handles() []command.ID {
	return nil // or specific commands if we wire save/choose
}

func (o *SourceComparisonOverlay) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, 90, 70, 60, 20)
}

func (o *SourceComparisonOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch msg.String() {
	case "q", "esc":
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case "up", "k":
		if o.cursor > 0 {
			o.cursor--
		}
	case "down", "j":
		if o.cursor < len(o.diffs)-1 {
			o.cursor++
		}
	case "u", "U": // choose USPTO for current row
		if o.cursor < len(o.diffs) {
			o.diffs[o.cursor].ChosenValue = o.diffs[o.cursor].USPTOValue
			o.diffs[o.cursor].ChosenSource = string(domain.SourceUSPTO)
		}
	case "g", "G": // choose Google for current row
		if o.cursor < len(o.diffs) {
			o.diffs[o.cursor].ChosenValue = o.diffs[o.cursor].GoogleValue
			o.diffs[o.cursor].ChosenSource = string(domain.SourceGoogle)
		}
	case "enter", "c", "C": // choose all from USPTO (default)
		for i := range o.diffs {
			o.diffs[i].ChosenValue = o.diffs[i].USPTOValue
			o.diffs[i].ChosenSource = string(domain.SourceUSPTO)
		}
		resolved := make([]domain.SourceDiff, len(o.diffs))
		copy(resolved, o.diffs)
		return o, func() tea.Msg {
			return SourceComparisonResolveMsg{
				Patent:  o.patent,
				Diffs:   resolved,
				Default: domain.SourceUSPTO,
			}
		}, true
	}
	return o, nil, true
}

func (o *SourceComparisonOverlay) View(maxW, _ int) string {
	var b strings.Builder

	if len(o.diffs) == 0 {
		b.WriteString(o.theme.OK.Render("No differences detected — USPTO and Google agreed on every compared field."))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("press q or esc to close"))
		return b.String()
	}
	b.WriteString(o.theme.Dim.Render("[u] = choose USPTO  |  [g] = choose Google  |  [enter] = accept all as [USPTO]  |  [q/esc] = cancel"))
	b.WriteString("\n\n")

	// Clear column headers with separator (user request) — no duplicate title
	colHeader := o.theme.HelpKey.Render(
		render.Pad("Field", 22) + "  " +
			render.Pad("USPTO", 34) + " │ " +
			render.Pad("Google", 34) + " │ " +
			"Chosen value + source",
	)
	b.WriteString(colHeader)
	b.WriteString("\n")
	sep := o.theme.Dim.Render(strings.Repeat("─", 22) + "  " + strings.Repeat("─", 34) + "─┼─" + strings.Repeat("─", 34) + "─┼─" + strings.Repeat("─", 30))
	b.WriteString(sep)
	b.WriteString("\n")

	// Wider value columns so long fields like classifications don't vanish
	const fieldW = 22
	const valW = 32

	for i, d := range o.diffs {
		prefix := "  "
		rowStyle := o.theme.Row
		if i == o.cursor {
			prefix = "> "
			rowStyle = o.theme.Selected
		}

		field := o.theme.HelpKey.Render(render.Pad(d.FieldPath, fieldW))

		usptoVal := render.Truncate(d.USPTOValue, valW)
		googleVal := render.Truncate(d.GoogleValue, valW)
		chosenVal := render.Truncate(d.ChosenValue, valW)

		// Only mark ✓ when the chosen value is non-empty for that source
		usptoIcon := " "
		googleIcon := " "
		if d.ChosenSource == string(domain.SourceUSPTO) && d.ChosenValue != "" && d.ChosenValue == d.USPTOValue {
			usptoIcon = "✓"
			usptoVal = o.theme.Selected.Render(usptoVal)
		} else if d.ChosenSource == string(domain.SourceGoogle) && d.ChosenValue != "" && d.ChosenValue == d.GoogleValue {
			googleIcon = "✓"
			googleVal = o.theme.Selected.Render(googleVal)
		}

		// Always show the actual chosen text + a small source badge (user request)
		sourceBadge := ""
		if d.ChosenSource != "" {
			sourceBadge = fmt.Sprintf(chosenSourceFormat, d.ChosenSource)
		}
		chosenDisplay := rowStyle.Render(chosenVal)
		if sourceBadge != "" {
			chosenDisplay += " " + o.theme.Dim.Render(sourceBadge)
		}

		line := fmt.Sprintf("%s%s  U:%s %s  │  G:%s %s  │  %s",
			prefix,
			field,
			usptoIcon, usptoVal,
			googleIcon, googleVal,
			chosenDisplay)

		b.WriteString(line)
		b.WriteByte('\n')
	}

	b.WriteString("\n")
	b.WriteString(o.theme.HelpKey.Render("Default choice is USPTO"))
	b.WriteString("  ")
	b.WriteString(o.theme.Dim.Render("(use [enter] to accept defaults and close)"))

	return b.String()
}
