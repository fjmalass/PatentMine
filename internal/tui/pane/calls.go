package pane

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
)

// callTimeout bounds a single request to the daemon.
const callTimeout = 15 * time.Second

// callContext returns a context bounded by callTimeout.
func callContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), callTimeout)
}

var asyncSeq atomic.Uint64

func nextAsyncID() uint64 {
	return asyncSeq.Add(1)
}

// ingestFamilyCmd starts a family-graph ingest for number and reports the
// outcome as a StatusMsg. The crawl itself runs in the daemon; this call only
// enqueues it, so the UI never blocks.
func ingestFamilyCmd(client *rpc.Client, number domain.PatentNumber) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.IngestStartResult
		err := client.Call(ctx, proto.MethodIngestFamily,
			proto.IngestFamilyParams{Root: number}, &res)
		if err != nil {
			return StatusMsg{Text: "ingest failed: " + err.Error(), Error: true}
		}
		return StatusMsg{Text: fmt.Sprintf("ingest started for %s (%s)", number, res.JobID)}
	}
}

// AddToProjectCmd links a patent to the active project.
func AddToProjectCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.Empty
		if err := client.Call(ctx, proto.MethodMembershipAdd,
			proto.MembershipParams{Project: project, Patent: number}, &res); err != nil {
			return StatusMsg{Text: "add to project failed: " + err.Error(), Error: true}
		}
		return StatusMsg{Text: fmt.Sprintf("added %s to %s", number, project)}
	}
}

// SetMembershipStateCmd changes a patent's state in the active project.
func SetMembershipStateCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber, state domain.MembershipState) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.Empty
		if err := client.Call(ctx, proto.MethodMembershipState,
			proto.MembershipStateParams{Project: project, Patent: number, State: string(state)}, &res); err != nil {
			return StatusMsg{Text: "set state failed: " + err.Error(), Error: true}
		}
		return StatusMsg{Text: fmt.Sprintf("set %s to %s in %s", number, state, project)}
	}
}

// projectRequiredCmd reports that an action needs an active project, which the
// project view (Phase 8) will provide.
func projectRequiredCmd() tea.Cmd {
	return status("select an active project first", true)
}
