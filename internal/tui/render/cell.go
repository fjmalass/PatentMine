package render

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// ellipsis marks text shortened by Truncate.
const ellipsis = "…"

// StringWidth returns the visual width of s in terminal cells using Unicode
// grapheme cluster segmentation for accurate emoji and combining character handling.
func StringWidth(s string) int {
	return uniseg.StringWidth(ansi.Strip(s))
}

// Truncate shortens s to at most w display columns, marking a cut with an
// ellipsis. ANSI styling in s is preserved.
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if StringWidth(s) <= w {
		return s
	}
	if w == 1 {
		return ellipsis
	}
	return ansi.Cut(s, 0, w-1) + ellipsis
}

// MarkdownHeadings extracts # and ## headings from markdown text, joined by
// "; ". Returns "—" when there are no headings.
func MarkdownHeadings(markdown string) string {
	var headings []string
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") {
			headings = append(headings, trimmed)
		}
	}
	if len(headings) == 0 {
		return "—"
	}
	return strings.Join(headings, "; ")
}

// Pad right-pads s with spaces to exactly w display columns, truncating when
// s is wider.
func Pad(s string, w int) string {
	if w <= 0 {
		return ""
	}
	width := StringWidth(s)
	if width > w {
		return Truncate(s, w)
	}
	out := s
	for range w - width {
		out += " "
	}
	return out
}
