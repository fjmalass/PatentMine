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
