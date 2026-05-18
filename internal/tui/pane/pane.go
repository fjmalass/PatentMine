// Package pane holds the TUI's screens. Each screen is a Pane that owns only
// its own state; the App owns the pane stack. Adding a screen means adding a
// Pane type — never extending a shared "model" struct. This decomposition is
// the structural defence against the god-object that sank the prior attempt.
package pane

import (
	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
)

// ResizeMsg reports the body area available to a pane after the app reserves
// its header and status lines.
type ResizeMsg struct {
	Width  int
	Height int
}

// ProjectChangedMsg reports that the app's active project changed.
type ProjectChangedMsg struct {
	Project *domain.Project
}

// Pane is one screen of the TUI.
type Pane interface {
	// Context reports the keymap context the pane uses, so the App can pick
	// the right key bindings while this pane is focused.
	Context() command.Context
	// Title is shown in the header bar.
	Title() string
	// Init returns a command to run when the pane is first shown.
	Init() tea.Cmd
	// Command applies a resolved command intent forwarded by the App, repeated
	// the given number of times (for count-prefixed chords like "3j").
	Command(id command.ID, repeat int) (Pane, tea.Cmd)
	// Update applies a non-command message: an rpc result, a daemon event, or
	// a resize.
	Update(msg tea.Msg) (Pane, tea.Cmd)
	// View renders the pane body into w columns by h rows.
	View(w, h int) string
	// Selection reports the highlighted patent, when the pane has one. The App
	// uses it to open detail/citation views for the current row.
	Selection() (domain.PatentNumber, bool)
}

// StatusMsg asks the App to show a line of status text. Panes emit it instead
// of drawing status themselves, so status appears in one consistent place.
type StatusMsg struct {
	Text  string
	Error bool
}

// status returns a tea.Cmd that emits a StatusMsg.
func status(text string, isErr bool) tea.Cmd {
	return func() tea.Msg { return StatusMsg{Text: text, Error: isErr} }
}
