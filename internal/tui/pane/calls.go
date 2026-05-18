package pane

import (
	"context"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
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
			return StatusMsg{Key: text.StatusIngestStartFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusIngestStarted, Args: []any{number.String(), res.JobID}}
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
			return StatusMsg{Key: text.StatusAddFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusAdded, Args: []any{number.String(), string(project)}}
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
			return StatusMsg{Key: text.StatusSetStateFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusSetState, Args: []any{number.String(), string(state), string(project)}}
	}
}

// CreateProjectCmd creates a project with the given name.
func CreateProjectCmd(client *rpc.Client, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.ProjectResult
		if err := client.Call(ctx, proto.MethodProjectCreate,
			proto.ProjectCreateParams{Name: name}, &res); err != nil {
			return StatusMsg{Key: text.StatusProjectCreateFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusProjectCreated, Args: []any{res.Project.Name}}
	}
}
