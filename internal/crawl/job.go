package crawl

import (
	"context"

	"patentmine/internal/domain"
	"patentmine/internal/engine"
	"patentmine/internal/proto"
)

// useConfiguredDepth tells Crawl to fall back to its configured depth.
const useConfiguredDepth = -1

// NewFamilyJob adapts a family-graph crawl to engine.Job. The engine's worker
// pool runs it; crawl progress is forwarded as crawl.progress events.
//
// depth selects how far the crawl walks: a negative depth defers to the
// crawler's configured default, zero fetches only the root patent (a single-
// patent lookup), and a positive depth bounds the family walk explicitly. force
// bypasses the local file cache.
func NewFamilyJob(crawler *Crawler, root domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool) engine.Job {
	return NewFamilyJobFromSource(crawler, root, depth, profile, force, "")
}

// NewFamilyJobFromSource adapts a source-constrained crawl to engine.Job.
func NewFamilyJobFromSource(crawler *Crawler, root domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool, source domain.Source) engine.Job {
	if depth < 0 {
		depth = useConfiguredDepth
	}
	return engine.JobFunc(func(ctx context.Context, id engine.JobID, emit func(proto.Event)) error {
		run := crawler.Crawl
		if source != "" {
			run = func(ctx context.Context, root domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool, emit func(Progress)) error {
				return crawler.CrawlFromSource(ctx, root, depth, profile, source, force, emit)
			}
		}
		return run(ctx, root, depth, profile, force, func(p Progress) {
			emit(proto.NewEvent(proto.EventCrawlProgress, proto.CrawlProgress{
				JobID:           string(id),
				CrawledCount:    p.CrawledCount,
				DiscoveredCount: p.DiscoveredCount,
				PendingCount:    p.PendingCount,
				Depth:           p.Depth,
				MaxDepth:        p.MaxDepth,
				CitationsCount:  p.CitationsCount,
				CitedByCount:    p.CitedByCount,
				ParentsCount:    p.ParentsCount,
				ChildrenCount:   p.ChildrenCount,
				Message:         p.Message,
				RecordNumber:    p.RecordNumber,
				Stubs:           p.Stubs,
				Sources:         p.Sources,
				Mode:            p.Mode,
			}))
		})
	})
}

// Factory returns an engine.CrawlFactory bound to a crawler, ready to hand to
// engine.New.
func Factory(crawler *Crawler) engine.CrawlFactory {
	return func(root domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool, source domain.Source) engine.Job {
		return NewFamilyJobFromSource(crawler, root, depth, profile, force, source)
	}
}
