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

// crawl depth selectors. A negative depth crawls the configured family depth;
// zero looks up only the named patent.
const (
	crawlFamilyDepth = -1
	lookupDepth      = 0
)

// crawlDepth returns the depth to use for a given profile. An empty profile
// means a single-patent lookup (depth 0); any other profile follows the full
// family crawl (depth -1, which defers to the daemon's configured default).
func crawlDepth(profile domain.CrawlProfile) int {
	if profile == "" {
		return lookupDepth
	}
	return crawlFamilyDepth
}

// CrawlCmd enqueues a crawl or lookup for number and reports the outcome as a
// StatusMsg. depth selects how far the family walk explicitly; a negative
// depth defers to the crawler's configured default. profile selects which
// family-graph edges to follow. force bypasses the local file cache.
func CrawlCmd(client *rpc.Client, number domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.CrawlStartResult
		err := client.Call(ctx, proto.MethodCrawlFamily,
			proto.CrawlFamilyParams{Root: number, Depth: depth, Profile: profile, Force: force}, &res)
		if err != nil {
			return StatusMsg{Key: text.StatusCrawlStartFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusCrawlStarted, Args: []any{number.String(), res.JobID, depth}}
	}
}

// MultiCrawlCmd starts a crawl or lookup for each number concurrently and
// returns a single MultiCrawlStartedMsg with all job IDs so the app can show
// one aggregate overlay for multi-selection.
func MultiCrawlCmd(client *rpc.Client, numbers []domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool) tea.Cmd {
	return func() tea.Msg {
		type rpcResult struct {
			number domain.PatentNumber
			jobID  string
			err    error
		}
		ch := make(chan rpcResult, len(numbers))
		for _, n := range numbers {
			go func(n domain.PatentNumber) {
				ctx, cancel := callContext()
				defer cancel()
				var res proto.CrawlStartResult
				err := client.Call(ctx, proto.MethodCrawlFamily,
					proto.CrawlFamilyParams{Root: n, Depth: depth, Profile: profile, Force: force}, &res)
				if err != nil {
					ch <- rpcResult{number: n, err: err}
				} else {
					ch <- rpcResult{number: n, jobID: res.JobID}
				}
			}(n)
		}
		var jobIDs []string
		var failErrs []string
		for range numbers {
			r := <-ch
			if r.err != nil {
				failErrs = append(failErrs, r.err.Error())
			} else {
				jobIDs = append(jobIDs, r.jobID)
			}
		}
		if len(jobIDs) == 0 {
			return StatusMsg{Key: text.StatusCrawlStartFailed, Args: []any{strings.Join(failErrs, "; ")}, Error: true}
		}
		if len(failErrs) > 0 {
			jobIDs = append(jobIDs, "(errors: "+strings.Join(failErrs, "; ")+")")
		}
		return MultiCrawlStartedMsg{
			Numbers: numbers,
			JobIDs:  jobIDs,
			Depth:   depth,
		}
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
			return StatusMsg{Key: text.StatusAddedNoCrawl, Args: []any{number.String()}}
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
