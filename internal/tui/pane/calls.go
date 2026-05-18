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

// projectRequiredCmd reports that an action needs an active project, which the
// project view (Phase 8) will provide.
func projectRequiredCmd() tea.Cmd {
	return status("this action needs an active project (project view: Phase 8)", true)
}
