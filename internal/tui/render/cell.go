package render

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// ellipsis marks text shortened by Truncate.
const ellipsis = "…"

// IsEmoji reports whether r is a rune in standard emoji blocks.
func IsEmoji(r rune) bool {
	return (r >= 0x1F300 && r <= 0x1F9FF) || // Misc Symbols & Pictographs, Food, etc.
		(r >= 0x1FA70 && r <= 0x1FAFF) ||     // Symbols & Pictographs Extended-A
		(r >= 0x1F600 && r <= 0x1F64F) ||     // Emoticons
		(r >= 0x1F680 && r <= 0x1F6FF) ||     // Transport & Map Symbols
		(r >= 0x2600 && r <= 0x27BF) ||       // Miscellaneous Symbols, Dingbats
		(r >= 0x1F1E6 && r <= 0x1F1FF)        // Regional Indicator Symbols (flags)
}

// isCJK reports whether r is a Chinese, Japanese, or Korean character,
// or a full-width forms character that is truly double-width in monospace fonts.
func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) ||     // CJK Unified Ideographs Extension A
		(r >= 0x3040 && r <= 0x309F) ||     // Hiragana
		(r >= 0x30A0 && r <= 0x30FF) ||     // Katakana
		(r >= 0xAC00 && r <= 0xD7AF) ||     // Hangul Syllables
		(r >= 0xF900 && r <= 0xFAFF) ||     // CJK Compatibility Ideographs
		(r >= 0xFF00 && r <= 0xFFEF) ||     // Halfwidth and Fullwidth Forms
		(r >= 0x20000 && r <= 0x2EBEF)      // CJK Unified Ideographs Extension B-F
}

// StringWidth returns the visual width of s in terminal cells, treating standard
// emojis/icons as exactly 1 cell wide to match how they render in standard terminals.
func StringWidth(s string) int {
	clean := ansi.Strip(s)
	width := 0
	for _, r := range clean {
		if r == '\uFE0F' || r == '\uFE0E' {
			continue // variation selectors are zero-width
		}
		if IsEmoji(r) {
			width += 1
		} else {
			w := ansi.StringWidth(string(r))
			if w == 2 && !isCJK(r) {
				width += 1
			} else {
				width += w
			}
		}
	}
	return width
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
