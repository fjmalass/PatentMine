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
