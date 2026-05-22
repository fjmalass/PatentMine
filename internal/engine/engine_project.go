package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/store"
)

// Projects returns every project.
func (e *Engine) Projects(ctx context.Context) (projects []domain.Project, err error) {
	defer e.observeDuration("engine.projects", time.Now(), &err)
	return e.repo.ListProjects(ctx)
}

// CreateProject creates a project with a generated id.
func (e *Engine) CreateProject(ctx context.Context, name string) (project domain.Project, err error) {
	defer e.observeDuration("engine.create_project", time.Now(), &err)
	if name == "" {
		return domain.Project{}, errors.New("engine: project name must not be empty")
	}
	p := domain.Project{
		ID:        domain.ProjectID(fmt.Sprintf("p-%d", time.Now().UnixNano())),
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	if err := e.repo.SaveProject(ctx, p); err != nil {
		e.log(ctx, slog.LevelError, "create project failed", slog.String("name", name), slog.String("error", err.Error()))
		return domain.Project{}, err
	}
	e.log(ctx, slog.LevelInfo, "project created", slog.String("project_id", string(p.ID)), slog.String("name", p.Name))
	e.recordActivity(ctx, observability.Record{
		Action:   "project.create",
		Entity:   "project",
		EntityID: string(p.ID),
		Status:   "committed",
		After:    p,
	})
	e.announceChange()
	return p, nil
}

// AddToProject adds a patent to a project in the default Stored state. The
// patent may be given by any of its document numbers; it is resolved to the
// record before the membership is created. A patent that has never been
// fetched is recorded as a stub first, so it can be tracked in a project
// ahead of crawling. fetchStarted reports whether an automatic single-patent
// fetch was enqueued; false means the caller should prompt the user to fetch
// manually (e.g. with F).
func (e *Engine) AddToProject(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (fetchStarted bool, err error) {
	defer e.observeDuration("engine.add_to_project", time.Now(), &err)
	record, created, err := e.ensureRecord(ctx, patent)
	if err != nil {
		return false, err
	}
	before, exists := e.existingMembership(ctx, project, record)
	state := domain.ReviewStateStored
	if exists {
		state = before.ReviewState
	} else if !created {
		if p, err := e.repo.Patent(ctx, record); err == nil && p.FetchState == domain.FetchCached {
			state = domain.ReviewStateCached
		}
	}
	after := domain.Membership{
		Project:     project,
		Patent:      record,
		ReviewState: state,
		AddedAt:     time.Now().UTC(),
	}
	err = e.repo.AddMembership(ctx, after)
	if err != nil {
		e.log(ctx, slog.LevelError, "add membership failed", slog.String("project_id", string(project)), slog.String("patent", patent.String()), slog.String("error", err.Error()))
		return false, err
	}
	e.log(ctx, slog.LevelInfo, "membership added", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("requested", patent.String()))
	e.recordActivity(ctx, observability.Record{
		Action:   "membership.add",
		Entity:   "membership",
		EntityID: string(project) + "/" + record.String(),
		Status:   "committed",
		Before:   before,
		After:    after,
		Metadata: map[string]any{"requested_number": patent.String()},
	})
	e.announceChange()
	// Any patent in "stored" state has no full record yet — kick a single-patent
	// fetch so bibliographic data fills in shortly after the patent is added.
	// This covers both brand-new stubs (created==true) and existing stubs that
	// were previously discovered as citation edges.
	if !exists && state == domain.ReviewStateStored && e.crawl != nil {
		if jobID, fetchErr := e.StartFamilyCrawl(record, 0, domain.CrawlProfileAll, false); fetchErr != nil {
			e.log(ctx, slog.LevelWarn, "auto-fetch on add failed to start",
				slog.String("record", record.String()), slog.String("error", fetchErr.Error()))
		} else {
			fetchStarted = true
			go e.cleanupIfNotFound(project, record, created, jobID)
		}
	}
	return fetchStarted, nil
}

// cleanupIfNotFound watches for the done event of a single-patent fetch job
// and removes the membership (and stub patent if freshly created) when the
// root patent was not found.
func (e *Engine) cleanupIfNotFound(project domain.ProjectID, record domain.PatentNumber, stubCreated bool, id JobID) {
	ch, unsub := e.pool.bus.Subscribe()
	defer unsub()
	for ev := range ch {
		if proto.EventKind(ev.Method) != proto.EventCrawlDone {
			continue
		}
		var d proto.CrawlDone
		if err := json.Unmarshal(ev.Params, &d); err != nil || d.JobID != string(id) {
			continue
		}
		if d.Error == "" {
			return
		}
		ctx := context.Background()
		membership, err := e.repo.Membership(ctx, project, record)
		if err == nil && membership.ReviewState != domain.ReviewStateStored {
			return
		}
		if stubCreated {
			if err := e.repo.DeletePatent(ctx, record); err != nil {
				e.log(ctx, slog.LevelWarn, "cleanup stub after not-found failed",
					slog.String("record", record.String()), slog.String("error", err.Error()))
			}
		} else {
			if err := e.repo.DeleteMembership(ctx, project, record); err != nil {
				e.log(ctx, slog.LevelWarn, "cleanup membership after not-found failed",
					slog.String("record", record.String()), slog.String("error", err.Error()))
			}
		}
		e.announceChange()
		return
	}
}

// SetReviewState changes a membership's state, rejecting transitions the
// domain rules disallow. This is where the state machine is enforced.
func (e *Engine) SetReviewState(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, target domain.ReviewState) (err error) {
	defer e.observeDuration("engine.set_review_state", time.Now(), &err)
	if !target.Valid() {
		return fmt.Errorf("engine: invalid review state %q", target)
	}
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return err
	}
	current, err := e.repo.Membership(ctx, project, record)
	if errors.Is(err, store.ErrNotFound) {
		// Patent not yet in the project — add it so the state change can proceed.
		if _, addErr := e.AddToProject(ctx, project, record); addErr != nil {
			return addErr
		}
		current, err = e.repo.Membership(ctx, project, record)
	}
	if err != nil {
		return err
	}
	if !current.ReviewState.CanTransitionTo(target) {
		return fmt.Errorf("engine: cannot move membership from %q to %q", current.ReviewState, target)
	}
	if current.ReviewState == target {
		e.incCounter("engine.review_state.noop_total", 1)
		e.log(ctx, slog.LevelDebug, "review state unchanged", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("state", string(target)))
		return nil
	}
	if err := e.repo.SetReviewState(ctx, project, record, target); err != nil {
		e.log(ctx, slog.LevelError, "set review state failed", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("target", string(target)), slog.String("error", err.Error()))
		return err
	}
	after := current
	after.ReviewState = target
	e.log(ctx, slog.LevelInfo, "review state changed", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("from", string(current.ReviewState)), slog.String("to", string(target)))
	e.recordActivity(ctx, observability.Record{
		Action:   "membership.set_state",
		Entity:   "membership",
		EntityID: string(project) + "/" + record.String(),
		Status:   "committed",
		Before:   current,
		After:    after,
		Metadata: map[string]any{"requested_number": patent.String()},
	})
	e.announceChange()
	e.incCounter("engine.review_state.changed_total", 1)
	return nil
}

// SetReviewStateBatch changes multiple memberships' states, enforcing domain rules and announcing changes once.
func (e *Engine) SetReviewStateBatch(ctx context.Context, project domain.ProjectID, patents []domain.PatentNumber, target domain.ReviewState) (err error) {
	defer e.observeDuration("engine.set_review_state_batch", time.Now(), &err)
	if !target.Valid() {
		return fmt.Errorf("engine: invalid review state %q", target)
	}

	records := make([]domain.PatentNumber, 0, len(patents))
	for _, patent := range patents {
		record, err := e.recordNumber(ctx, patent)
		if err != nil {
			return err
		}
		records = append(records, record)
	}

	// First, check memberships and add missing ones
	beforeMemberships := make(map[domain.PatentNumber]domain.Membership)
	for _, record := range records {
		current, err := e.repo.Membership(ctx, project, record)
		if errors.Is(err, store.ErrNotFound) {
			if _, addErr := e.AddToProject(ctx, project, record); addErr != nil {
				return addErr
			}
			current, err = e.repo.Membership(ctx, project, record)
		}
		if err != nil {
			return err
		}
		if !current.ReviewState.CanTransitionTo(target) {
			return fmt.Errorf("engine: cannot move membership from %q to %q", current.ReviewState, target)
		}
		beforeMemberships[record] = current
	}

	// Update states
	if err := e.repo.SetReviewStates(ctx, project, records, target); err != nil {
		e.log(ctx, slog.LevelError, "set review state batch failed", slog.String("project_id", string(project)), slog.Int("count", len(records)), slog.String("target", string(target)), slog.String("error", err.Error()))
		return err
	}

	// Record activities
	for _, record := range records {
		e.log(ctx, slog.LevelInfo, "review state changed", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("to", string(target)))
		rec := observability.Record{
			Action:   "membership.set_state",
			Entity:   "membership",
			EntityID: string(project) + "/" + record.String(),
			Status:   "committed",
			Metadata: map[string]any{"requested_number": record.String()},
		}
		if current, ok := beforeMemberships[record]; ok {
			after := current
			after.ReviewState = target
			rec.Before = current
			rec.After = after
		}
		e.recordActivity(ctx, rec)
	}

	e.announceChange()
	e.incCounter("engine.review_state.changed_total", int64(len(records)))
	return nil
}

// ReviewStateOf returns a patent's workflow state in a project. ok is
// false when no project is named or the patent is not one of its members —
// review state is a property of the (patent, project) pair, not the patent.
func (e *Engine) ReviewStateOf(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (state domain.ReviewState, ok bool, err error) {
	defer e.observeDuration("engine.review_state_of", time.Now(), &err)
	if project == "" {
		return "", false, nil
	}
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return "", false, err
	}
	m, err := e.repo.Membership(ctx, project, record)
	if errors.Is(err, store.ErrNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return m.ReviewState, true, nil
}
