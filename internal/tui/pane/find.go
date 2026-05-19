package pane

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/tui/render"
)

// findBar is the inline / search bar state shared by catalog and citations.
// The zero value is inactive.
type findBar struct {
	active bool
	input  string
	prev   string // filter.Search at open time, restored on Esc
}

func (fb *findBar) open(currentSearch string) {
	fb.active = true
	fb.input = currentSearch
	fb.prev = currentSearch
}

// handleKey processes one raw key event while the bar is active.
// Returns the current search string and one of: "reload", "confirm", "cancel", "".
func (fb *findBar) handleKey(msg tea.KeyMsg) (search string, action string) {
	switch msg.Type {
	case tea.KeyEnter:
		fb.active = false
		return fb.input, "confirm"
	case tea.KeyEsc:
		fb.active = false
		fb.input = fb.prev
		return fb.prev, "cancel"
	case tea.KeyBackspace, tea.KeyDelete:
		if len(fb.input) > 0 {
			runes := []rune(fb.input)
			fb.input = string(runes[:len(runes)-1])
		}
		return fb.input, "reload"
	case tea.KeyCtrlW:
		trimmed := strings.TrimRight(fb.input, " ")
		if idx := strings.LastIndex(trimmed, " "); idx >= 0 {
			fb.input = fb.input[:idx+1]
		} else {
			fb.input = ""
		}
		return fb.input, "reload"
	case tea.KeyCtrlU:
		fb.input = ""
		return fb.input, "reload"
	case tea.KeyRunes:
		fb.input += string(msg.Runes)
		return fb.input, "reload"
	default:
		return fb.input, ""
	}
}

// view renders the find bar as a single line.
func (fb *findBar) view(w int, theme render.Theme, total int) string {
	bar := "/ " + fb.input + "▋"
	if total >= 0 {
		bar += "  (" + matchLabel(total) + ")"
	}
	return theme.Dim.Render(render.Pad(bar, w))
}

func matchLabel(n int) string {
	if n == 1 {
		return "1 match"
	}
	return fmt.Sprintf("%d matches", n)
}
