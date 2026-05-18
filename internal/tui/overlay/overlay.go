// Package overlay holds the TUI's modal layers. Like a Pane, each Overlay owns
// only its own state; the App owns the overlay stack and draws each overlay
// over a dimmed copy of the pane behind it.
package overlay

import (
	"slices"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
)

// Overlay is one modal layer.
type Overlay interface {
	// Title is shown at the top of the overlay box.
	Title() string
	// Command applies a resolved command intent (e.g. scrolling).
	Command(id command.ID, repeat int) (Overlay, tea.Cmd)
	// Handles reports every command ID the overlay services, so the App's
	// wiring check can confirm overlay key bindings reach a handler.
	Handles() []command.ID
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

// cmdHandler carries out one command for an overlay.
type cmdHandler func(repeat int) tea.Cmd

// handlerIDs returns the sorted command IDs of a handler table.
func handlerIDs(handlers map[command.ID]cmdHandler) []command.ID {
	ids := make([]command.ID, 0, len(handlers))
	for id := range handlers {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
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

// Purpose names what a TextInput overlay is collecting, so the App routes the
// submitted value to the right action.
type Purpose string

const (
	// PurposeCreateProject collects a name for a new project.
	PurposeCreateProject Purpose = "create-project"
)

// TextSubmitMsg carries a value entered in a TextInput overlay.
type TextSubmitMsg struct {
	Purpose Purpose
	Value   string
}
