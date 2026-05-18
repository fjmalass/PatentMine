package ingest

import (
	"context"

	"patentmine/internal/domain"
	"patentmine/internal/engine"
	"patentmine/internal/proto"
)

// useConfiguredDepth tells Crawl to fall back to its configured depth.
const useConfiguredDepth = -1

// NewFamilyJob adapts a family-graph crawl to engine.Job. The engine's worker
// pool runs it; crawl progress is forwarded as ingest.progress events. A
// non-positive depth defers to the crawler's configured default.
func NewFamilyJob(crawler *Crawler, root domain.PatentNumber, depth int) engine.Job {
	if depth <= 0 {
		depth = useConfiguredDepth
	}
	return engine.JobFunc(func(ctx context.Context, id engine.JobID, emit func(proto.Event)) error {
		return crawler.Crawl(ctx, root, depth, func(p Progress) {
			emit(proto.NewEvent(proto.EventIngestProgress, proto.IngestProgress{
				JobID:   string(id),
				Fetched: p.Fetched,
				Found:   p.Found,
				Message: p.Message,
			}))
		})
	})
}

// Factory returns an engine.IngestFactory bound to a crawler, ready to hand to
// engine.New.
func Factory(crawler *Crawler) engine.IngestFactory {
	return func(root domain.PatentNumber, depth int) engine.Job {
		return NewFamilyJob(crawler, root, depth)
	}
}
