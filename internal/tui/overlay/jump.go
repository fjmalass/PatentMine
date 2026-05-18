package overlay

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/tui/render"
)

// JumpSelectMsg asks the app to scroll the pane behind the jump overlay to a
// chosen anchor's line.
type JumpSelectMsg struct {
	Line int
}

// Jump is a transient overlay listing a pane's jump anchors: pressing an
// anchor's key scrolls the pane straight to that field. It consumes every key
// itself, so an anchor letter never collides with the pane's own bindings.
type Jump struct {
	theme   render.Theme
	anchors []render.JumpAnchor
}

// NewJump builds a jump overlay over the given anchors.
func NewJump(theme render.Theme, anchors []render.JumpAnchor) *Jump {
	return &Jump{theme: theme, anchors: anchors}
}

// Title implements Overlay.
func (j *Jump) Title() string { return "Jump to field" }

// Command implements Overlay: a jump overlay services no resolved commands,
// since HandleKey consumes every key.
func (j *Jump) Command(command.ID, int) (Overlay, tea.Cmd) { return j, nil }

// Handles implements Overlay.
func (j *Jump) Handles() []command.ID { return nil }

// HandleKey implements KeyHandler. An anchor's key scrolls to it and closes the
// overlay; any other key — including esc — closes it without moving.
func (j *Jump) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		for _, a := range j.anchors {
			if a.Key == msg.Runes[0] {
				line := a.Line
				return j, func() tea.Msg { return JumpSelectMsg{Line: line} }, true
			}
		}
	}
	return j, func() tea.Msg { return PromptCloseMsg{} }, true
}

// View implements Overlay, listing each anchor's key and label.
func (j *Jump) View(maxW, _ int) string {
	var b strings.Builder
	b.WriteString(j.theme.Dim.Render(
		render.Truncate("press a key to jump · any other key closes", maxW)))
	b.WriteString("\n\n")
	for _, a := range j.anchors {
		line := "  " + j.theme.HelpKey.Render(string(a.Key)) + "  " + a.Label
		b.WriteString(render.Truncate(line, maxW))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
