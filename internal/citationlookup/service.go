package citationlookup

import (
	"context"
	"fmt"
	"time"

	"patentmine/internal/domain"
)

type GraphStore interface {
	GetCitationGraph(context.Context, domain.ProjectID, domain.PatentNumber) (domain.CitationGraph, bool, error)
	EnqueueCitationRefresh(context.Context, domain.ProjectID, domain.PatentNumber, EnqueueOptions) (domain.CitationRefreshJob, bool, error)
}

type EnqueueOptions struct {
	RequestedPatent string
	Priority        int
	DesiredDepth    int
	NextAttemptAt   time.Time
	TraceID         string
}

type LookupRequest struct {
	ProjectID       domain.ProjectID
	PatentNumber    string
	TenantID        string
	TraceID         string
	DesiredDepth    int
	RefreshPriority int
}

type LookupResponse struct {
	HTTPStatus     int
	Status         string
	PatentNumber   domain.PatentNumber
	Graph          domain.CitationGraph
	RefreshPending bool
	JobID          int64
	RetryAfter     time.Duration
	Provenance     string
	TraceID        string
}

type Service struct {
	store      GraphStore
	now        func() time.Time
	retryAfter time.Duration
	metrics    *Metrics
}

func NewService(store GraphStore) *Service {
	return &Service{
		store:      store,
		now:        time.Now,
		retryAfter: 30 * time.Second,
		metrics:    NewMetrics(),
	}
}

func (s *Service) WithMetrics(metrics *Metrics) *Service {
	s.metrics = metrics
	return s
}

func (s *Service) Stats() domain.CitationLookupStats {
	if s == nil || s.metrics == nil {
		return domain.CitationLookupStats{}
	}
	return s.metrics.Snapshot()
}

func (s *Service) Lookup(ctx context.Context, req LookupRequest) (LookupResponse, error) {
	if req.ProjectID == "" {
		req.ProjectID = domain.ProjectID(req.TenantID)
	}
	if req.ProjectID == "" {
		return LookupResponse{}, fmt.Errorf("tenant/project identity is required")
	}
	number := NormalizePatentID(req.PatentNumber)
	if number == "" {
		return LookupResponse{}, fmt.Errorf("patent number is required")
	}
	if req.DesiredDepth == 0 {
		req.DesiredDepth = domain.CitationRefreshDepthCitations
	}

	graph, ok, err := s.store.GetCitationGraph(ctx, req.ProjectID, number)
	if err != nil {
		return LookupResponse{}, err
	}
	if ok && graph.Cache.Status == domain.CitationLookupStatusFresh && graph.Cache.FreshUntil.After(s.now()) {
		if s.metrics != nil {
			s.metrics.freshCacheHits.Add(1)
		}
		return LookupResponse{
			HTTPStatus:   200,
			Status:       domain.CitationLookupStatusFresh,
			PatentNumber: number,
			Graph:        graph,
			Provenance:   graph.Cache.ProvenanceSource,
			TraceID:      req.TraceID,
		}, nil
	}

	job, _, err := s.store.EnqueueCitationRefresh(ctx, req.ProjectID, number, EnqueueOptions{
		RequestedPatent: req.PatentNumber,
		Priority:        req.RefreshPriority,
		DesiredDepth:    req.DesiredDepth,
		NextAttemptAt:   s.now(),
		TraceID:         req.TraceID,
	})
	if err != nil {
		return LookupResponse{}, err
	}

	if ok {
		status := domain.CitationLookupStatusStale
		if graph.Cache.Status == domain.CitationLookupStatusFailed {
			status = domain.CitationLookupStatusFailed
			if s.metrics != nil {
				s.metrics.failedResponses.Add(1)
			}
		} else if s.metrics != nil {
			s.metrics.staleCacheHits.Add(1)
			s.metrics.staleResponses.Add(1)
		}
		return LookupResponse{
			HTTPStatus:     200,
			Status:         status,
			PatentNumber:   number,
			Graph:          graph,
			RefreshPending: true,
			JobID:          job.ID,
			RetryAfter:     s.retryAfter,
			Provenance:     graph.Cache.ProvenanceSource,
			TraceID:        req.TraceID,
		}, nil
	}

	if s.metrics != nil {
		s.metrics.missingCacheHits.Add(1)
		s.metrics.pendingResponses.Add(1)
	}
	return LookupResponse{
		HTTPStatus:     202,
		Status:         domain.CitationLookupStatusPending,
		PatentNumber:   number,
		RefreshPending: true,
		JobID:          job.ID,
		RetryAfter:     s.retryAfter,
		TraceID:        req.TraceID,
	}, nil
}
