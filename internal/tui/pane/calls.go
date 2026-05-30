package pane

import (
	"context"
	"fmt"
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

type patentTableColumnsLoadedMsg struct {
	requestID uint64
	project   domain.ProjectID
	columns   []domain.PatentTableColumn
	err       error
}

// LoadPatentTableColumnsCmd loads the server-owned patent-table schema.
func LoadPatentTableColumnsCmd(client *rpc.Client, project domain.ProjectID, requestID uint64) tea.Cmd {
	if client == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.PatentTableColumnsResult
		err := client.Call(ctx, proto.MethodPatentTableColumns,
			proto.PatentTableColumnsParams{Project: project}, &res)
		return patentTableColumnsLoadedMsg{requestID: requestID, project: project, columns: res.Columns, err: err}
	}
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
func CrawlCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.CrawlStartResult
		err := client.Call(ctx, proto.MethodCrawlFamily,
			proto.CrawlFamilyParams{Project: project, Root: number, Depth: depth, Profile: profile, Force: force}, &res)
		if err != nil {
			return StatusMsg{Key: text.StatusCrawlStartFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusCrawlStarted, Args: []any{number.String(), res.JobID, depth}}
	}
}

// MultiCrawlCmd starts a crawl or lookup for each number concurrently and
// returns a single MultiCrawlStartedMsg with all job IDs so the app can show
// one aggregate overlay for multi-selection.
func MultiCrawlCmd(client *rpc.Client, project domain.ProjectID, numbers []domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool) tea.Cmd {
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
					proto.CrawlFamilyParams{Project: project, Root: n, Depth: depth, Profile: profile, Force: force}, &res)
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

// AddUSPTOShowCandidatesMsg is returned when a broad search yields multiple candidate wrappers.
type AddUSPTOShowCandidatesMsg struct {
	Project    domain.ProjectID
	Patent     domain.PatentNumber
	Candidates []domain.USPTOCandidate
}

// AddToProjectCmd links a patent to the active project.
func AddToProjectCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber) tea.Cmd {
	return AddToProjectFromSourceCmd(client, project, number, "")
}

// AddToProjectFromSourceCmd links a patent and optionally forces a provider-specific fetch.
func AddToProjectFromSourceCmd(client *rpc.Client, project domain.ProjectID, number domain.PatentNumber, source domain.Source) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.MembershipAddResult
		if err := client.Call(ctx, proto.MethodMembershipAdd,
			proto.MembershipParams{Project: project, Patent: number, Source: source}, &res); err != nil {
			return StatusMsg{Key: text.StatusAddFailed, Args: []any{err.Error()}, Error: true}
		}
		if len(res.Candidates) > 0 {
			return AddUSPTOShowCandidatesMsg{Project: project, Patent: number, Candidates: res.Candidates}
		}
		if !res.FetchStarted {
			return StatusMsg{Key: text.StatusAddedNoCrawl, Args: []any{number.String()}}
		}
		if res.JobID != "" {
			return StatusMsg{Key: text.StatusCrawlStarted, Args: []any{number.String(), res.JobID, 0}}
		}
		return StatusMsg{Key: text.StatusAdded, Args: []any{number.String(), string(project)}}
	}
}

// AddToProjectWithCandidateCmd adds a patent using a pre-selected USPTO application
// number, bypassing the USPTO candidate search in the engine.
func AddToProjectWithCandidateCmd(client *rpc.Client, project domain.ProjectID, candidate domain.USPTOCandidate) tea.Cmd {
	appNum, err := domain.ParsePatentNumber("US" + candidate.ApplicationNumber)
	if err != nil {
		return func() tea.Msg {
			return StatusMsg{Key: text.StatusInvalidPatentNumber, Args: []any{err.Error()}, Error: true}
		}
	}
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.MembershipAddResult
		if err := client.Call(ctx, proto.MethodMembershipAdd,
			proto.MembershipParams{
				Project:           project,
				Patent:            appNum,
				Source:            domain.SourceUSPTO,
				ApplicationNumber: candidate.ApplicationNumber,
			}, &res); err != nil {
			return StatusMsg{Key: text.StatusAddFailed, Args: []any{err.Error()}, Error: true}
		}
		if !res.FetchStarted {
			return StatusMsg{Key: text.StatusAddedNoCrawl, Args: []any{appNum.String()}}
		}
		if res.JobID != "" {
			return StatusMsg{Key: text.StatusCrawlStarted, Args: []any{appNum.String(), res.JobID, 0}}
		}
		return StatusMsg{Key: text.StatusAdded, Args: []any{appNum.String(), string(project)}}
	}
}

// AddRelatedCmd asks the daemon to grant membership in project for every
// family-graph neighbor of patent that does not already have one. The reply
// reports the count added so the status line can confirm the result.
func AddRelatedCmd(client *rpc.Client, project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.AddRelatedResult
		if err := client.Call(ctx, proto.MethodAddRelated,
			proto.AddRelatedParams{Project: project, Patent: patent}, &res); err != nil {
			return StatusMsg{Key: text.StatusAddRelatedFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusAddRelatedDone, Args: []any{res.Added, patent.String()}}
	}
}

// SetReviewStateCmd changes multiple memberships' states. When a single patent is targeted,
// it is passed in a slice of length 1.
func SetReviewStateCmd(client *rpc.Client, project domain.ProjectID, patents []domain.PatentNumber, state domain.ReviewState) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.ReviewStateResult
		if err := client.Call(ctx, proto.MethodReviewState,
			proto.ReviewStateParams{Project: project, Patents: patents, State: string(state)}, &res); err != nil {
			return StatusMsg{Key: text.StatusSetStateFailed, Args: []any{err.Error()}, Error: true}
		}
		if len(res.Patents) > 0 {
			patents = res.Patents
		}
		return ReviewStateChangedMsg{Project: project, Patents: patents, State: state}
	}
}

// CycleIDSEntryStatusesCmd advances each selected patent's IDS status in the
// active project. Missing entries follow IDSDetail's behavior: they start from
// pending, so the first cycle saves submitted.
func CycleIDSEntryStatusesCmd(client *rpc.Client, project domain.ProjectID, patents []domain.PatentNumber) tea.Cmd {
	return func() tea.Msg {
		saved := make([]domain.IDSEntry, 0, len(patents))
		for _, patent := range patents {
			ctx, cancel := callContext()
			var existing proto.PatentResult
			err := client.Call(ctx, proto.MethodPatentGet,
				proto.PatentGetParams{Number: patent, Project: project}, &existing)
			cancel()
			if err != nil {
				return IDSEntriesChangedMsg{Entries: saved, Err: err}
			}

			entry := domain.IDSEntry{Project: project, Patent: existing.Patent.Number, Status: domain.IDSEntryPending}
			if entry.Patent.IsZero() {
				entry.Patent = patent
			}
			if existing.IDSEntry != nil {
				entry = *existing.IDSEntry
				if !entry.Status.Valid() {
					entry.Status = domain.IDSEntryPending
				}
			}
			entry.Status = entry.Status.Next()

			ctx, cancel = callContext()
			var result proto.IDSEntryResult
			err = client.Call(ctx, proto.MethodIDSEntrySave,
				proto.IDSEntrySaveParams{Entry: entry}, &result)
			cancel()
			if err != nil {
				return IDSEntriesChangedMsg{Entries: saved, Err: err}
			}
			saved = append(saved, result.Entry)
		}
		return IDSEntriesChangedMsg{Entries: saved}
	}
}

// BulkSetIDSStatusCmd sets the IDS entry status for every patent in patents
// using a single daemon round-trip. Missing entries are created with
// InFull=true so the picker's "default to in full" promise holds; existing
// entries keep every other field. The single returned IDSEntriesChangedMsg
// lets the App flush every pane in one Update→View cycle.
func BulkSetIDSStatusCmd(client *rpc.Client, project domain.ProjectID, patents []domain.PatentNumber, status domain.IDSEntryStatus) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.IDSEntriesResult
		err := client.Call(ctx, proto.MethodIDSEntryBulkSetStatus,
			proto.IDSEntryBulkSetStatusParams{
				Project:       project,
				Patents:       patents,
				Status:        status,
				DefaultInFull: true,
			}, &res)
		return IDSEntriesChangedMsg{Entries: res.Entries, Err: err}
	}
}

// TagPatentCmd tags multiple patents within the active project, creating the tag when
// the project does not have it yet. When a single patent is targeted, it is passed in a slice of length 1.
func TagPatentCmd(client *rpc.Client, project domain.ProjectID, patents []domain.PatentNumber, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.Empty
		if err := client.Call(ctx, proto.MethodTagPatent,
			proto.TagParams{Project: project, Patents: patents, Name: name}, &res); err != nil {
			return StatusMsg{Key: text.StatusTagFailed, Args: []any{err.Error()}, Error: true}
		}
		if len(patents) == 1 {
			return StatusMsg{Key: text.StatusTagged, Args: []any{patents[0].String(), name, string(project)}}
		}
		return StatusMsg{Key: text.StatusTagged, Args: []any{fmt.Sprintf("%d patents", len(patents)), name, string(project)}}
	}
}

// UntagPatentCmd removes a tag from multiple patents within the active project.
func UntagPatentCmd(client *rpc.Client, project domain.ProjectID, patents []domain.PatentNumber, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.Empty
		if err := client.Call(ctx, proto.MethodUntagPatent,
			proto.TagParams{Project: project, Patents: patents, Name: name}, &res); err != nil {
			return StatusMsg{Key: text.StatusUntagFailed, Args: []any{err.Error()}, Error: true}
		}
		if len(patents) == 1 {
			return StatusMsg{Key: text.StatusUntagged, Args: []any{name, patents[0].String()}}
		}
		return StatusMsg{Key: text.StatusUntagged, Args: []any{name, fmt.Sprintf("%d patents", len(patents))}}
	}
}

// DeletePatentCmd permanently removes a patent from the database.
func DeletePatentCmd(client *rpc.Client, number domain.PatentNumber) tea.Cmd {
	return DeletePatentsCmd(client, []domain.PatentNumber{number})
}

// DeletePatentsCmd permanently removes one or more patents from the database.
func DeletePatentsCmd(client *rpc.Client, patents []domain.PatentNumber) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.Empty
		if len(patents) == 1 {
			if err := client.Call(ctx, proto.MethodPatentDelete,
				proto.PatentDeleteParams{Number: patents[0]}, &res); err != nil {
				return StatusMsg{Key: text.StatusDeleteFailed, Args: []any{err.Error()}, Error: true}
			}
			return StatusMsg{Key: text.StatusDeleted, Args: []any{patents[0].String()}}
		}
		if err := client.Call(ctx, proto.MethodPatentDeleteBulk,
			proto.PatentDeleteBulkParams{Patents: patents}, &res); err != nil {
			return StatusMsg{Key: text.StatusDeleteFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusBatchDeleted, Args: []any{len(patents)}}
	}
}

// ClearPatentCacheCmd clears the parsed XML body cache for one or more patents.
func ClearPatentCacheCmd(client *rpc.Client, patents []domain.PatentNumber) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.PatentClearCacheResult
		if err := client.Call(ctx, proto.MethodPatentClearCache,
			proto.PatentClearCacheParams{Patents: patents}, &res); err != nil {
			return StatusMsg{Key: text.StatusClearCacheFailed, Args: []any{err.Error()}, Error: true}
		}
		sizeStr := fmt.Sprintf("%.2f MB", float64(res.BytesSaved)/1024.0/1024.0)
		if len(patents) == 0 {
			return StatusMsg{Key: text.StatusAllCacheCleared, Args: []any{res.ClearedCount, sizeStr}}
		}
		return StatusMsg{Key: text.StatusCacheCleared, Args: []any{len(patents), sizeStr}}
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

// TagPatentStrictCmd assigns a taxonomy tag to multiple patents.
func TagPatentStrictCmd(client *rpc.Client, project domain.ProjectID, patents []domain.PatentNumber, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.Empty
		if err := client.Call(ctx, proto.MethodTagPatentStrict,
			proto.TagParams{Project: project, Patents: patents, Name: name}, &res); err != nil {
			return StatusMsg{Key: text.StatusTagPatentAddFailed, Args: []any{err.Error()}, Error: true}
		}
		if len(patents) == 1 {
			return StatusMsg{Key: text.StatusTagPatentAdded, Args: []any{name, patents[0].String()}}
		}
		return StatusMsg{Key: text.StatusTagPatentAdded, Args: []any{name, fmt.Sprintf("%d patents", len(patents))}}
	}
}

// UntagPatentStrictCmd removes a tag assignment from multiple patents.
func UntagPatentStrictCmd(client *rpc.Client, project domain.ProjectID, patents []domain.PatentNumber, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.Empty
		if err := client.Call(ctx, proto.MethodUntagPatentStrict,
			proto.TagParams{Project: project, Patents: patents, Name: name}, &res); err != nil {
			return StatusMsg{Key: text.StatusTagPatentDeleteFailed, Args: []any{err.Error()}, Error: true}
		}
		if len(patents) == 1 {
			return StatusMsg{Key: text.StatusTagPatentDeleted, Args: []any{name, patents[0].String()}}
		}
		return StatusMsg{Key: text.StatusTagPatentDeleted, Args: []any{name, fmt.Sprintf("%d patents", len(patents))}}
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

// LookupClassificationCmd starts a lookup for a classification code.
func LookupClassificationCmd(client *rpc.Client, code string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res domain.Classification
		if err := client.Call(ctx, proto.MethodClassificationLookup,
			proto.ClassificationLookupParams{Code: code}, &res); err != nil {
			return StatusMsg{Key: text.StatusClassificationLookupFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusClassificationLookupSuccess, Args: []any{res.Code, res.Description}}
	}
}

// FetchUSPTOAssignmentsCmd pulls the patent's assignment chain from the USPTO
// Patent Assignment Search API and persists the result. Returns a status line
// reporting how many assignments and parties were saved.
func FetchUSPTOAssignmentsCmd(client *rpc.Client, number domain.PatentNumber) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.USPTOFetchAssignmentsResult
		if err := client.Call(ctx, proto.MethodUSPTOFetchAssignments,
			proto.USPTOFetchAssignmentsParams{Number: number}, &res); err != nil {
			return StatusMsg{Key: text.StatusAssignmentsFetchFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusAssignmentsFetched, Args: []any{number.String(), res.Assignments, res.Parties}}
	}
}

// FetchUSPTOXMLCmd downloads the pgpub or grant XML for a patent, or returns
// the cached path when the file is already on disk.
func FetchUSPTOXMLCmd(client *rpc.Client, number domain.PatentNumber, kind proto.USPTOXMLKind) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.USPTOFetchXMLResult
		if err := client.Call(ctx, proto.MethodUSPTOFetchXML,
			proto.USPTOFetchXMLParams{Number: number, Kind: kind}, &res); err != nil {
			return StatusMsg{Key: text.StatusXMLFetchFailed, Args: []any{err.Error()}, Error: true}
		}
		key := text.StatusXMLFetched
		if res.Cached {
			key = text.StatusXMLCached
		}
		return StatusMsg{Key: key, Args: []any{string(kind), res.LocalPath, res.Bytes, res.DownloadCount}}
	}
}

// USPTOXMLFetchedMsg is delivered when an interactive (Detail-pane Enter)
// XML fetch completes. The App handler closes the spinner overlay, opens the
// file, and emits a final status line.
type USPTOXMLFetchedMsg struct {
	Number        domain.PatentNumber
	Kind          proto.USPTOXMLKind
	LocalPath     string
	Bytes         int64
	Cached        bool
	DownloadCount int64
	Err           string
}

// FetchUSPTOXMLInteractiveCmd is the Detail-pane Enter variant: it returns a
// typed result the App can dispatch on, rather than a finished status line, so
// the App can close the spinner overlay and open the file.
func FetchUSPTOXMLInteractiveCmd(client *rpc.Client, number domain.PatentNumber, kind proto.USPTOXMLKind) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.USPTOFetchXMLResult
		if err := client.Call(ctx, proto.MethodUSPTOFetchXML,
			proto.USPTOFetchXMLParams{Number: number, Kind: kind}, &res); err != nil {
			return USPTOXMLFetchedMsg{Number: number, Kind: kind, Err: err.Error()}
		}
		return USPTOXMLFetchedMsg{
			Number:        number,
			Kind:          kind,
			LocalPath:     res.LocalPath,
			Bytes:         res.Bytes,
			Cached:        res.Cached,
			DownloadCount: res.DownloadCount,
		}
	}
}

