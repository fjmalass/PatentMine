package render

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Composite draws fg onto bg starting at column x, row y, leaving the rest of
// bg visible around it. This is how an overlay shows the dimmed pane behind it.
func Composite(bg, fg string, x, y int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	for i, fgLine := range fgLines {
		row := y + i
		if row < 0 || row >= len(bgLines) {
			continue
		}
		base := bgLines[row]
		baseWidth := ansi.StringWidth(base)
		fgWidth := ansi.StringWidth(fgLine)

		left := ansi.Cut(base, 0, x)
		if pad := x - baseWidth; pad > 0 {
			left = base + strings.Repeat(" ", pad)
		}
		right := ""
		if end := x + fgWidth; end < baseWidth {
			right = ansi.Cut(base, end, baseWidth)
		}
		bgLines[row] = left + fgLine + right
	}
	return strings.Join(bgLines, "\n")
}

// Dim renders s as de-emphasised text — used for the background under an overlay.
func Dim(s string) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(colorDim))
	lines := strings.Split(ansi.Strip(s), "\n")
	for i, l := range lines {
		lines[i] = style.Render(l)
	}
	return strings.Join(lines, "\n")
}

// CenterOffset returns the top-left position that centres a width x height box
// inside an outer area.
func CenterOffset(outerW, outerH, boxW, boxH int) (x, y int) {
	return max((outerW-boxW)/2, 0), max((outerH-boxH)/2, 0)
}
