package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/changes"
)

// undoLastChange reverses the most recent change. The affected views are
// marked dirty so flushDirty refreshes them at the end of the turn.
func (m *Model) undoLastChange() (tea.Model, tea.Cmd) {
	scope, label, err := m.history.Undo(m.ctx)
	if err != nil {
		m.message = err.Error()
		return m, nil
	}
	m.markDirty(scopeDirty(scope))
	m.message = "undo: " + label
	return m, nil
}

// redoChange re-applies the most recently undone change.
func (m *Model) redoChange() (tea.Model, tea.Cmd) {
	scope, label, err := m.history.Redo(m.ctx)
	if err != nil {
		m.message = err.Error()
		return m, nil
	}
	m.markDirty(scopeDirty(scope))
	m.message = "redo: " + label
	return m, nil
}

// recCommand saves or replays a recording of the session's changes.
//
//	:rec save <path>  — write every change applied this session to a file
//	:rec play <path>  — re-apply every change from a recording file
func (m *Model) recCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) < 2 {
		m.err = "usage: :rec save <path> | :rec play <path>"
		return m, nil
	}
	switch args[0] {
	case "save":
		if err := m.history.SaveRecording(args[1]); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = fmt.Sprintf("recording saved: %s (%d changes)", args[1], m.history.RecordingLen())
	case "play":
		n, err := changes.Replay(m.ctx, m.repo, args[1])
		if err != nil {
			m.err = fmt.Sprintf("replay stopped after %d changes: %v", n, err)
			return m, nil
		}
		m.message = fmt.Sprintf("replayed %d changes from %s", n, args[1])
		m.markDirty(dirtyList | dirtyDetail | dirtyFamily | dirtyProjectTags)
	default:
		m.err = "usage: :rec save <path> | :rec play <path>"
	}
	return m, nil
}
