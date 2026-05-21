package citationlookup

import (
	"sync/atomic"

	"patentmine/internal/domain"
)

type Metrics struct {
	freshCacheHits   atomic.Int64
	staleCacheHits   atomic.Int64
	missingCacheHits atomic.Int64
	pendingResponses atomic.Int64
	failedResponses  atomic.Int64
	staleResponses   atomic.Int64
	upstreamRequests atomic.Int64
	upstream429      atomic.Int64
	timeouts         atomic.Int64
	refreshFailures  atomic.Int64
}

func NewMetrics() *Metrics {
	return &Metrics{}
}

func (m *Metrics) Snapshot() domain.CitationLookupStats {
	if m == nil {
		return domain.CitationLookupStats{}
	}
	return domain.CitationLookupStats{
		FreshCacheHits:   m.freshCacheHits.Load(),
		StaleCacheHits:   m.staleCacheHits.Load(),
		MissingCacheHits: m.missingCacheHits.Load(),
		PendingResponses: m.pendingResponses.Load(),
		FailedResponses:  m.failedResponses.Load(),
		StaleResponses:   m.staleResponses.Load(),
		UpstreamRequests: m.upstreamRequests.Load(),
		Upstream429Count: int(m.upstream429.Load()),
		Timeouts:         m.timeouts.Load(),
		RefreshFailures:  m.refreshFailures.Load(),
	}
}

func MergeStats(base, overlay domain.CitationLookupStats) domain.CitationLookupStats {
	base.Upstream429Count += overlay.Upstream429Count
	base.FreshCacheHits += overlay.FreshCacheHits
	base.StaleCacheHits += overlay.StaleCacheHits
	base.MissingCacheHits += overlay.MissingCacheHits
	base.PendingResponses += overlay.PendingResponses
	base.FailedResponses += overlay.FailedResponses
	base.StaleResponses += overlay.StaleResponses
	base.UpstreamRequests += overlay.UpstreamRequests
	base.Timeouts += overlay.Timeouts
	base.RefreshFailures += overlay.RefreshFailures
	return base
}
