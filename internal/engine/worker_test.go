package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"patentmine/internal/proto"
)

type panicJob struct{}

func (j panicJob) Run(ctx context.Context, id JobID, emit func(proto.Event)) error {
	panic("something went wrong in the job")
}

func TestWorkerPoolPanicRecovery(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	pool := newWorkerPool(context.Background(), 2, bus, nil)
	defer pool.shutdown()

	ch, unsub := bus.Subscribe()
	defer unsub()

	qj := queuedJob{
		id:  "panic-job-1",
		ctx: context.Background(),
		job: panicJob{},
	}

	// Submit panic job
	pool.queue <- qj

	// The worker pool should catch the panic and publish CrawlDone with the panic error
	select {
	case ev := <-ch:
		if ev.Method == proto.EventCrawlDone {
			var d proto.CrawlDone
			if err := json.Unmarshal(ev.Params, &d); err != nil {
				t.Fatalf("decode CrawlDone: %v", err)
			}
			if d.JobID != "panic-job-1" {
				t.Errorf("JobID = %q, want %q", d.JobID, "panic-job-1")
			}
			if !strings.Contains(d.Error, "something went wrong in the job") {
				t.Errorf("Error = %q, want it to contain %q", d.Error, "something went wrong in the job")
			}
		} else {
			t.Errorf("EventKind = %s, want EventCrawlDone", ev.Method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for job done event")
	}
}
