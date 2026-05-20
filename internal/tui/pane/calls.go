package pane

import (
	"context"
	"strings"
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
// StatusMsg. depth selects how far the family walk explicitly; a negative
// depth defers to the crawler's configured default. profile selects which
// family-graph edges to follow. force bypasses the local file cache.
func IngestCmd(client *rpc.Client, number domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.IngestStartResult
		err := client.Call(ctx, proto.MethodIngestFamily,
			proto.IngestFamilyParams{Root: number, Depth: depth, Profile: profile, Force: force}, &res)
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
		var res proto.MembershipAddResult
		if err := client.Call(ctx, proto.MethodMembershipAdd,
			proto.MembershipParams{Project: project, Patent: number}, &res); err != nil {
			return StatusMsg{Key: text.StatusAddFailed, Args: []any{err.Error()}, Error: true}
		}
		if !res.FetchStarted {
			return StatusMsg{Key: text.StatusAddedNoIngest, Args: []any{number.String()}}
		}
		return StatusMsg{Key: text.StatusAdded, Args: []any{number.String(), string(project)}}
	}
}

// SetReviewStateCmd changes a patent's state in the active project.
func SetReviewStateCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber, state domain.ReviewState) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.Empty
		if err := client.Call(ctx, proto.MethodReviewState,
			proto.ReviewStateParams{Project: project, Patent: number, State: string(state)}, &res); err != nil {
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

// CreateTagTaxonomyCmd registers a tag in the project's taxonomy.
func CreateTagTaxonomyCmd(client *rpc.Client, project domain.ProjectID, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res domain.Tag
		if err := client.Call(ctx, proto.MethodTagCreate,
			proto.TagCreateParams{Project: project, Name: name}, &res); err != nil {
			return StatusMsg{Key: text.StatusTagTaxonomyAddFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusTagTaxonomyAdded, Args: []any{name, string(project)}}
	}
}

// DeleteTagTaxonomyCmd removes a tag from the project's taxonomy.
func DeleteTagTaxonomyCmd(client *rpc.Client, project domain.ProjectID, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.Empty
		if err := client.Call(ctx, proto.MethodTagDelete,
			proto.TagDeleteParams{Project: project, Name: name}, &res); err != nil {
			return StatusMsg{Key: text.StatusTagTaxonomyDeleteFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusTagTaxonomyDeleted, Args: []any{name, string(project)}}
	}
}

// ListTagTaxonomyCmd lists all taxonomy tags in the project.
func ListTagTaxonomyCmd(client *rpc.Client, project domain.ProjectID) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.TagListResult
		if err := client.Call(ctx, proto.MethodTagList,
			proto.TagListParams{Project: project}, &res); err != nil {
			return StatusMsg{Key: text.StatusTagTaxonomyListFailed, Args: []any{err.Error()}, Error: true}
		}
		var names []string
		for _, t := range res.Tags {
			names = append(names, t.Name)
		}
		if len(names) == 0 {
			return StatusMsg{Key: text.StatusFilter, Args: []any{"taxonomy: (no tags registered)"}}
		}
		return StatusMsg{Key: text.StatusFilter, Args: []any{"taxonomy: " + strings.Join(names, ", ")}}
	}
}

// AssignPatentTagCmd assigns a taxonomy tag to a patent.
func AssignPatentTagCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.Empty
		if err := client.Call(ctx, proto.MethodPatentTagAdd,
			proto.TagParams{Project: project, Patent: number, Name: name}, &res); err != nil {
			return StatusMsg{Key: text.StatusTagPatentAddFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusTagPatentAdded, Args: []any{name, number.String()}}
	}
}

// RemovePatentTagCmd removes a tag assignment from a patent.
func RemovePatentTagCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.Empty
		if err := client.Call(ctx, proto.MethodPatentTagDelete,
			proto.TagParams{Project: project, Patent: number, Name: name}, &res); err != nil {
			return StatusMsg{Key: text.StatusTagPatentDeleteFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusTagPatentDeleted, Args: []any{name, number.String()}}
	}
}

// ListPatentTagsCmd lists all tags assigned to a patent.
func ListPatentTagsCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.PatentTagListResult
		if err := client.Call(ctx, proto.MethodPatentTagList,
			proto.PatentTagListParams{Project: project, Patent: number}, &res); err != nil {
			return StatusMsg{Key: text.StatusTagPatentListFailed, Args: []any{err.Error()}, Error: true}
		}
		var tagStrings []string
		for _, t := range res.Tags {
			var assignedStr string
			if !t.AssignedAt.IsZero() {
				assignedStr = " (assigned " + t.AssignedAt.Format("2006-01-02 15:04:05") + ")"
			}
			tagStrings = append(tagStrings, t.Name+assignedStr)
		}
		if len(tagStrings) == 0 {
			return StatusMsg{Key: text.StatusFilter, Args: []any{"patent tags: (none assigned)"}}
		}
		return StatusMsg{Key: text.StatusFilter, Args: []any{"patent tags: " + strings.Join(tagStrings, ", ")}}
	}
}
