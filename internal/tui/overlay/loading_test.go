package overlay

import (
	"strings"
	"testing"

	"patentmine/internal/proto"
	"patentmine/internal/tui/render"
)

func TestLoadingViewShowsCrawlDepth(t *testing.T) {
	o := NewLoading(render.NewTheme(), []string{"job-1"}, "Crawling")
	msg := proto.NewEvent(proto.EventCrawlProgress, proto.CrawlProgress{
		JobID:           "job-1",
		CrawledCount:    3,
		DiscoveredCount: 8,
		PendingCount:    2,
		Depth:           2,
		MaxDepth:        4,
		Message:         "crawled US123",
	})
	if _, cmd := o.Update(msg); cmd != nil {
		cmd()
	}
	view := o.View(80, 12)
	if !strings.Contains(view, "depth 2/4") {
		t.Fatalf("loading view missing depth label\n%s", view)
	}
}

func TestLoadingWithErrors(t *testing.T) {
	o := NewLoading(render.NewTheme(), []string{"job-1"}, "Crawling").WithErrors([]string{"early resolution error"})
	
	// Test the active progress view contains the startup failure warning list
	activeView := o.View(80, 12)
	if !strings.Contains(activeView, "1/2 startup attempts failed") || !strings.Contains(activeView, "early resolution error") {
		t.Fatalf("active loading view missing startup failure report\n%s", activeView)
	}

	// Emitting the EventCrawlDone should complete it
	msg := proto.NewEvent(proto.EventCrawlDone, proto.CrawlDone{
		JobID: "job-1",
	})
	if _, cmd := o.Update(msg); cmd != nil {
		cmd()
	}
	if !o.Finished() {
		t.Fatalf("loading should be finished")
	}
	view := o.View(80, 12)
	if !strings.Contains(view, "1/2 attempts failed") || !strings.Contains(view, "early resolution error") {
		t.Fatalf("loading view missing error message\n%s", view)
	}
}
