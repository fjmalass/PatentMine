// Project, membership, add-related, orphan and review-state RPC handlers.
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"patentmine/internal/domain"
	"patentmine/internal/engine"
	"patentmine/internal/proto"
)

func (s *Server) projectList(ctx context.Context, _ json.RawMessage) (any, error) {
	projects, err := s.engine.Projects(ctx)
	if err != nil {
		return nil, err
	}
	return proto.ProjectListResult{Projects: projects}, nil
}

func (s *Server) projectCreate(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ProjectCreateParams](raw)
	if err != nil {
		return nil, err
	}
	project, err := s.engine.CreateProject(ctx, p.Name)
	if err != nil {
		return nil, err
	}
	return proto.ProjectResult{Project: project}, nil
}

func (s *Server) membershipAdd(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.MembershipParams](raw)
	if err != nil {
		return nil, err
	}
	var fetchStarted bool
	var jobID engine.JobID
	var candidates []domain.USPTOCandidate
	var mergeWarning *proto.MergeWarning
	var autoSelected *domain.USPTOCandidate
	var alreadyInDB bool
	if p.Source != "" {
		fetchStarted, jobID, candidates, mergeWarning, autoSelected, alreadyInDB, err = s.engine.AddToProjectFromSourceWithOptions(ctx, p.Project, p.Patent, p.Source, p.ApplicationNumber, p.ConfirmMerge)
	} else {
		fetchStarted, jobID, candidates, mergeWarning, autoSelected, alreadyInDB, err = s.engine.AddToProjectWithOptions(ctx, p.Project, p.Patent, p.ConfirmMerge)
	}
	if err != nil {
		return nil, err
	}
	return proto.MembershipAddResult{
		FetchStarted:          fetchStarted,
		JobID:                 string(jobID),
		Candidates:            candidates,
		MergeWarning:          mergeWarning,
		AutoSelectedCandidate: autoSelected,
		AlreadyInDB:           alreadyInDB,
	}, nil
}

func (s *Server) addRelated(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.AddRelatedParams](raw)
	if err != nil {
		return nil, err
	}
	added, err := s.engine.AddRelated(ctx, p.Project, p.Patent)
	if err != nil {
		return nil, err
	}
	return proto.AddRelatedResult{Added: len(added), Neighbors: added}, nil
}

func (s *Server) orphanList(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.OrphanListParams](raw)
	if err != nil {
		return nil, err
	}
	rows, total, err := s.engine.OrphanPatents(ctx, p.Limit, p.Offset)
	if err != nil {
		return nil, err
	}
	return proto.OrphanListResult{Patents: rows, Total: total}, nil
}

func (s *Server) reviewState(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ReviewStateParams](raw)
	if err != nil {
		return nil, err
	}
	state, err := domain.ParseReviewState(p.State)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadParams, err)
	}
	if err := s.engine.SetReviewStateWithOptions(ctx, p.Project, p.Patents, state, p.AddedMethod); err != nil {
		return nil, err
	}
	records := make([]domain.PatentNumber, 0, len(p.Patents))
	for _, patent := range p.Patents {
		record, err := s.engine.Patent(ctx, patent)
		if err != nil {
			return nil, err
		}
		records = append(records, record.Number)
	}
	return proto.ReviewStateResult{Patents: records}, nil
}

func (s *Server) projectUpdate(ctx context.Context, raw json.RawMessage) (any, error) {
	p, err := decodeParams[proto.ProjectUpdateParams](raw)
	if err != nil {
		return nil, err
	}
	saved, err := s.engine.UpdateProject(ctx, p.Project)
	if err != nil {
		return nil, err
	}
	return proto.ProjectResult{Project: saved}, nil
}
