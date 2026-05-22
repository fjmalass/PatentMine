package render

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// GutterWidth returns the display width needed for a right-aligned line number
// column when showing up to maxLines lines (including the trailing space).
// For example, 999 lines → returns 4 (3 digits + 1 space).
func GutterWidth(maxLines int) int {
	return len(fmt.Sprintf("%d", max(maxLines, 1))) + 1
}

// LineGutter returns a dim-styled, right-aligned line number with a trailing
// space, ready to be prepended to a line of content.  The gutter width is
// derived from maxLines so all numbers share the same alignment.
func LineGutter(style lipgloss.Style, lineNum, maxLines int) string {
	if maxLines < 1 {
		maxLines = 1
	}
	w := len(fmt.Sprintf("%d", maxLines))
	return style.Render(fmt.Sprintf("%*d", w, lineNum)) + " "
}
