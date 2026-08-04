package overlay

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
)

// minAutoCaptureSeconds is the floor below which an editor session is too short
// to bother recording — it filters out a quick open-and-close from the billable
// time queue.
const minAutoCaptureSeconds = 5

// focusTimer accumulates wall-clock time per side of a split editor as focus
// moves between the read-only source/examiner pane (reading) and the editable
// notes/response pane (writing). The editor flushes it as auto, unvalidated time
// entries when it closes — the automatic half of time tracking, which the
// attorney then reviews via :time.validate.
type focusTimer struct {
	reading   time.Duration
	writing   time.Duration
	onWriting bool
	last      time.Time
}

// newFocusTimer starts the clock on the side the editor opens focused on.
func newFocusTimer(startWriting bool) focusTimer {
	return focusTimer{onWriting: startWriting, last: time.Now()}
}

// accrue adds the elapsed time since the last switch to the current side.
func (f *focusTimer) accrue() {
	now := time.Now()
	d := now.Sub(f.last)
	f.last = now
	if f.onWriting {
		f.writing += d
	} else {
		f.reading += d
	}
}

// switchTo accrues the current side and moves focus to the other.
func (f *focusTimer) switchTo(writing bool) {
	f.accrue()
	f.onWriting = writing
}

// flushCmd accrues the final interval and returns a command that logs the
// reading and writing time as auto (unvalidated) entries. Sides under the
// minimum are dropped. nil when nothing meaningful accrued.
func (f *focusTimer) flushCmd(client *rpc.Client, project domain.ProjectID, oaID string) tea.Cmd {
	f.accrue()
	return logAutoTimeCmd(client, project, oaID,
		int(f.reading.Round(time.Second)/time.Second),
		int(f.writing.Round(time.Second)/time.Second))
}

func logAutoTimeCmd(client *rpc.Client, project domain.ProjectID, oaID string, readingSecs, writingSecs int) tea.Cmd {
	if client == nil || project == "" {
		return nil
	}
	if readingSecs < minAutoCaptureSeconds && writingSecs < minAutoCaptureSeconds {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		log := func(act domain.TimeActivity, secs int) {
			if secs < minAutoCaptureSeconds {
				return
			}
			var res proto.TimeEntryResult
			_ = client.Call(ctx, proto.MethodTimeLog, proto.TimeLogParams{
				Project: project, OfficeActionID: oaID, Activity: act, Seconds: secs, Auto: true,
			}, &res)
		}
		log(domain.TimeReading, readingSecs)
		log(domain.TimeWriting, writingSecs)
		return nil
	}
}
