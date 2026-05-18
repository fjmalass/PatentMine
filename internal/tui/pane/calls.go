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

// ingest depth selectors. A negative depth crawls the configured family depth;
// zero fetches only the named patent.
const (
	ingestFamilyDepth = -1
	ingestPatentDepth = 0
)

// IngestCmd enqueues an ingest for number and reports the outcome as a
// StatusMsg. depth selects a single-patent fetch (0) or a family crawl (<0);
// force bypasses the local file cache. The crawl runs in the daemon, so this
// call only enqueues it and the UI never blocks.
func IngestCmd(client *rpc.Client, number domain.PatentNumber, depth int, force bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.IngestStartResult
		err := client.Call(ctx, proto.MethodIngestFamily,
			proto.IngestFamilyParams{Root: number, Depth: depth, Force: force}, &res)
		if err != nil {
			return StatusMsg{Key: text.StatusIngestStartFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusIngestStarted, Args: []any{number.String(), res.JobID}}
	}
}

// ImportFileCmd loads a patent record from a local fixture file.
func ImportFileCmd(client *rpc.Client, path string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.Empty
		if err := client.Call(ctx, proto.MethodImportFile,
			proto.ImportFileParams{Path: path}, &res); err != nil {
			return StatusMsg{Key: text.StatusImportFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusImported, Args: []any{path}}
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

// AssignTagCmd tags a patent within the active project, creating the tag when
// the project does not have it yet.
func AssignTagCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.Empty
		if err := client.Call(ctx, proto.MethodTagAssign,
			proto.TagParams{Project: project, Patent: number, Name: name}, &res); err != nil {
			return StatusMsg{Key: text.StatusTagFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusTagged, Args: []any{number.String(), name, string(project)}}
	}
}

// RemoveTagCmd removes a tag from a patent within the active project.
func RemoveTagCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.Empty
		if err := client.Call(ctx, proto.MethodTagRemove,
			proto.TagParams{Project: project, Patent: number, Name: name}, &res); err != nil {
			return StatusMsg{Key: text.StatusUntagFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusUntagged, Args: []any{name, number.String()}}
	}
}

// DeletePatentCmd permanently removes a patent from the database.
func DeletePatentCmd(client *rpc.Client, number domain.PatentNumber) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.Empty
		if err := client.Call(ctx, proto.MethodPatentDelete,
			proto.PatentDeleteParams{Number: number}, &res); err != nil {
			return StatusMsg{Key: text.StatusDeleteFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusDeleted, Args: []any{number.String()}}
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
