// Package overlay holds the TUI's modal layers. Like a Pane, each Overlay owns
// only its own state; the App owns the overlay stack and draws each overlay
// over a dimmed copy of the pane behind it.
package overlay

import (
	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
)

// Overlay is one modal layer.
type Overlay interface {
	// Title is shown at the top of the overlay box.
	Title() string
	// Command applies a resolved command intent (e.g. scrolling).
	Command(id command.ID, repeat int) (Overlay, tea.Cmd)
	// View renders the overlay body within maxW columns by maxH rows.
	View(maxW, maxH int) string
}

// KeyHandler overlays consume raw key events directly, which prompt-like
// overlays need for text entry.
type KeyHandler interface {
	HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool)
}

// ContextSource reports the underlying pane context an overlay was opened from.
// Prompt overlays use this so their filtered command list reflects the screen
// behind them rather than the generic overlay context.
type ContextSource interface {
	SourceContext() command.Context
}

// PromptMode distinguishes palette search from direct command entry.
type PromptMode string

const (
	PromptPalette PromptMode = "palette"
	PromptDirect  PromptMode = "direct"
)

// PromptSubmitMsg asks the app to execute one typed command string.
type PromptSubmitMsg struct {
	Input string
}

// PromptCloseMsg asks the app to close the focused prompt overlay.
type PromptCloseMsg struct{}
