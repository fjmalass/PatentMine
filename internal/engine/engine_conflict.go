package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
)

// FlagConflictInput flags one record as conflicting within a matter. Record is
// the conflicting patent/application/reference (required). Against optionally
// names what it conflicts with (empty = the matter's subject application);
// DocumentID optionally links a matter document. Reason is free text.
type FlagConflictInput struct {
	Project    domain.ProjectID
	Record     string
	Against    string
	DocumentID string
	Reason     string
	FlaggedBy  string
}

// FlagConflict records a project-scoped conflict edge and returns it. The record
// number is normalized so the patent-list badge matches it against record.number.
func (e *Engine) FlagConflict(ctx context.Context, in FlagConflictInput) (c domain.Conflict, err error) {
	defer e.observeDuration("engine.flag_conflict", time.Now(), &err)
	if in.Project == "" {
		return domain.Conflict{}, errors.New("engine: conflict requires a project")
	}
	if _, err := e.repo.Project(ctx, in.Project); err != nil {
		return domain.Conflict{}, fmt.Errorf("engine: flag conflict: %w", err)
	}
	record, err := domain.ParsePatentNumber(in.Record)
	if err != nil {
		return domain.Conflict{}, fmt.Errorf("engine: flag conflict: record: %w", err)
	}
	var against domain.PatentNumber
	if in.Against != "" {
		against, err = domain.ParsePatentNumber(in.Against)
		if err != nil {
			return domain.Conflict{}, fmt.Errorf("engine: flag conflict: against: %w", err)
		}
	}
	now := time.Now().UTC()
	c = domain.Conflict{
		ID:         fmt.Sprintf("conf-%d", now.UnixNano()),
		Project:    in.Project,
		Record:     record,
		Against:    against,
		DocumentID: in.DocumentID,
		Reason:     in.Reason,
		Status:     domain.ConflictOpen,
		FlaggedBy:  in.FlaggedBy,
		FlaggedAt:  now,
	}
	if err := e.repo.SaveConflict(ctx, c); err != nil {
		return domain.Conflict{}, err
	}
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionConflictFlag,
		Entity:   observability.EntityConflict,
		EntityID: c.ID,
		Status:   observability.StatusCommitted,
		Attributes: map[string]any{
			"project": string(c.Project),
			"record":  c.Record.Normalized(),
		},
	})
	e.announceChange()
	return c, nil
}

// ResolveConflict sets an existing conflict's status to resolved or waived (any
// non-open status), stamping the resolution time. Passing open re-opens it.
func (e *Engine) ResolveConflict(ctx context.Context, id string, status domain.ConflictStatus) (c domain.Conflict, err error) {
	defer e.observeDuration("engine.resolve_conflict", time.Now(), &err)
	if id == "" {
		return domain.Conflict{}, errors.New("engine: conflict id required")
	}
	if !status.Valid() {
		return domain.Conflict{}, fmt.Errorf("engine: invalid conflict status %q", status)
	}
	c, err = e.repo.Conflict(ctx, id)
	if err != nil {
		return domain.Conflict{}, err
	}
	c.Status = status
	if status == domain.ConflictOpen {
		c.ResolvedAt = time.Time{}
	} else {
		c.ResolvedAt = time.Now().UTC()
	}
	if err := e.repo.SaveConflict(ctx, c); err != nil {
		return domain.Conflict{}, err
	}
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionConflictResolve,
		Entity:   observability.EntityConflict,
		EntityID: c.ID,
		Status:   observability.StatusCommitted,
		Attributes: map[string]any{
			"project": string(c.Project),
			"status":  string(c.Status),
		},
	})
	e.announceChange()
	return c, nil
}

// ListConflicts returns a project's conflict edges, newest first.
func (e *Engine) ListConflicts(ctx context.Context, project domain.ProjectID) (out []domain.Conflict, err error) {
	defer e.observeDuration("engine.list_conflicts", time.Now(), &err)
	if project == "" {
		return nil, errors.New("engine: list conflicts requires a project")
	}
	return e.repo.ListConflicts(ctx, project)
}

// DeleteConflict removes a conflict edge.
func (e *Engine) DeleteConflict(ctx context.Context, id string) (err error) {
	defer e.observeDuration("engine.delete_conflict", time.Now(), &err)
	if id == "" {
		return errors.New("engine: conflict id required")
	}
	c, err := e.repo.Conflict(ctx, id)
	if err != nil {
		return err
	}
	if err := e.repo.DeleteConflict(ctx, id); err != nil {
		return err
	}
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionConflictDelete,
		Entity:   observability.EntityConflict,
		EntityID: id,
		Status:   observability.StatusCommitted,
		Attributes: map[string]any{
			"project": string(c.Project),
		},
	})
	e.announceChange()
	return nil
}
