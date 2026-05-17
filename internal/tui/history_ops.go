package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/changes"
)

// recordingPathForToday is the default recording file when the user runs
// :rec stop or :rec play without an explicit path. Recordings live under the
// DefaultRecordingDir root, named by the current date.
func recordingPathForToday() string {
	return filepath.Join(DefaultRecordingDir, fmt.Sprintf("recording-%s.json", time.Now().Format(dateFmtDate)))
}

// recordingFiles lists saved recording paths under the DefaultRecordingDir
// root, newest name first. Date-named files sort chronologically.
func recordingFiles() []string {
	entries, err := os.ReadDir(DefaultRecordingDir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			files = append(files, filepath.Join(DefaultRecordingDir, e.Name()))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files
}

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

// recCommand starts, stops, or replays a recording of applied changes.
//
//	:rec start        begin capturing every change applied
//	:rec stop [path]  stop capturing and write the recording to a file
//	:rec play [path]  re-apply every change from a recording file
//	:rec list         show saved recordings
//
// When path is omitted it defaults to recordingPathForToday(). With no/unknown
// subcommand (and for list) it opens the recording info popup.
func (m *Model) recCommand(args []string) (tea.Model, tea.Cmd) {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case recSubStart:
		if m.history.IsRecording() {
			m.message = "already recording"
		} else {
			m.history.StartRecording()
			m.message = "recording started"
		}
		return m.navigateTo(viewRecordingInfo), nil
	case recSubStop:
		if !m.history.IsRecording() {
			m.err = "not recording — start with :rec start"
			return m, nil
		}
		path := recordingPathForToday()
		if len(args) > 1 {
			path = args[1]
		}
		count := m.history.RecordingLen()
		if err := m.history.SaveRecording(path); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.history.StopRecording()
		m.message = fmt.Sprintf("recording stopped — %d change(s) saved to %s", count, path)
		return m, nil
	case recSubPlay:
		path := recordingPathForToday()
		if len(args) > 1 {
			path = args[1]
		}
		n, err := changes.Replay(m.ctx, m.repo, path)
		if err != nil {
			m.err = fmt.Sprintf("replay stopped after %d change(s): %v", n, err)
			return m, nil
		}
		m.message = fmt.Sprintf("replayed %d change(s) from %s", n, path)
		m.markDirty(dirtyList | dirtyDetail | dirtyFamily | dirtyProjectTags)
		return m, nil
	case recSubList:
		return m.navigateTo(viewRecordingInfo), nil
	default:
		return m.navigateTo(viewRecordingInfo), nil
	}
}

// viewRecordingInfo renders the recording status popup, telling the user how
// to stop an active recording or how to start one.
func (m *Model) viewRecordingInfo() string {
	var b strings.Builder
	today := recordingPathForToday()
	if m.history.IsRecording() {
		active := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorError)).Bold(true)
		b.WriteString(active.Render(recBadgeLabel + "  recording in progress"))
		b.WriteString(fmt.Sprintf("\n\n%d change(s) captured so far.\n\n", m.history.RecordingLen()))
		b.WriteString("Every change you apply is added to the recording.\n\n")
		b.WriteString("Stop and save:\n")
		b.WriteString("  :rec stop [path]\n\n")
		b.WriteString("With no path it saves to:\n")
		b.WriteString("  " + today + "\n")
	} else {
		b.WriteString("Recordings capture the changes you apply so they can be\n")
		b.WriteString("replayed later against any project.\n\n")
		b.WriteString("  :rec start         begin a recording\n")
		b.WriteString("  :rec stop [path]   stop and save it to a file\n")
		b.WriteString("  :rec play [path]   replay a saved recording\n")
		b.WriteString("  :rec list          show saved recordings\n\n")
		b.WriteString("Path defaults to a dated file under " + DefaultRecordingDir + "/.\n")
	}

	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle))
	files := recordingFiles()
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Saved recordings (%s/):\n", DefaultRecordingDir))
	if len(files) == 0 {
		b.WriteString(subtle.Render("  (none yet)") + "\n")
	} else {
		for _, f := range files {
			b.WriteString("  " + f + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(subtle.Render("[q/esc] to close"))
	return m.renderPopup("Recording", b.String())
}
