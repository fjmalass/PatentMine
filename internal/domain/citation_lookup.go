package domain

import "time"

const (
	CitationLookupStatusFresh   = "fresh"
	CitationLookupStatusStale   = "stale"
	CitationLookupStatusPending = "pending"
	CitationLookupStatusFailed  = "failed"
)

const (
	CitationRefreshJobQueued    = "queued"
	CitationRefreshJobRunning   = "running"
	CitationRefreshJobSucceeded = "succeeded"
	CitationRefreshJobFailed    = "failed"
)

const (
	CitationRefreshDepthMetadataOnly = 0
	CitationRefreshDepthCitations    = 1
	CitationRefreshDepthFull         = 2
)

type CitationGraphCacheRecord struct {
	ProjectID           ProjectID
	PatentNumber        PatentNumber
	RequestedPatent     string
	Status              string
	ProvenanceSource    string
	ProvenanceVersion   string
	RefreshedAt         time.Time
	FreshUntil          time.Time
	PartialData         bool
	ExpectedCitations   int
	ExpectedCitedBy     int
	ForwardPageCap      int
	ForwardPagesFetched int
	ForwardPageCapHit   bool
	LastError           string
	UpdatedAt           time.Time
}

type CitationGraph struct {
	Patent Patent
	Cited  []CitationEdge
	Citing []CitationEdge
	Cache  CitationGraphCacheRecord
}

type CitationRefreshJob struct {
	ID            int64
	ProjectID     ProjectID
	PatentNumber  PatentNumber
	DedupeKey     string
	Status        string
	RetryCount    int
	NextAttemptAt time.Time
	Priority      int
	DesiredDepth  int
	TraceCount    int
	LastError     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ClaimedAt     time.Time
	CompletedAt   time.Time
}

type CitationLookupStats struct {
	QueuedJobs          int
	RunningJobs         int
	SucceededJobs       int
	FailedJobs          int
	OldestQueuedAge     time.Duration
	OldestRefreshAge    time.Duration
	CoalescedTraceCount int
	Upstream429Count    int
	FreshCacheHits      int64
	StaleCacheHits      int64
	MissingCacheHits    int64
	PendingResponses    int64
	FailedResponses     int64
	StaleResponses      int64
	UpstreamRequests    int64
	Timeouts            int64
	RefreshFailures     int64
}
