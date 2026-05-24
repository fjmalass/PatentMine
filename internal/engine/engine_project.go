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
		Metadata: map[string]any{
			"requested_number": patent.String(),
			"project":          string(project),
		},
	})
	e.announceChange()
	// Any patent in "stored" state has no full record yet — kick a single-patent
	// fetch so bibliographic data fills in shortly after the patent is added.
	// This covers both brand-new stubs (created==true) and existing stubs that
	// were previously discovered as citation edges.
	if !exists && state == domain.ReviewStateStored && e.crawl != nil {
		if jobID, fetchErr := e.StartFamilyCrawl(ctx, record, 0, domain.CrawlProfileAll, false); fetchErr != nil {
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

// SetReviewState changes multiple memberships' states, enforcing domain rules and announcing changes once.
// When a single patent is targeted, it is passed in a slice of length 1.
func (e *Engine) SetReviewState(ctx context.Context, project domain.ProjectID, patents []domain.PatentNumber, target domain.ReviewState) (err error) {
	defer e.observeDuration("engine.set_review_state", time.Now(), &err)
	if !target.Valid() {
		return fmt.Errorf("engine: invalid review state %q", target)
	}

	records := make([]domain.PatentNumber, 0, len(patents))
	createdMap := make(map[domain.PatentNumber]bool)
	for _, patent := range patents {
		record, created, err := e.ensureRecord(ctx, patent)
		if err != nil {
			return err
		}
		records = append(records, record)
		createdMap[record] = created
	}

	// First, check memberships and add missing ones
	beforeMemberships := make(map[domain.PatentNumber]domain.Membership)
	var addedRecords []domain.PatentNumber

	for _, record := range records {
		current, err := e.repo.Membership(ctx, project, record)
		if errors.Is(err, store.ErrNotFound) {
			initialState := domain.ReviewStateStored
			created := createdMap[record]
			if !created {
				if p, err := e.repo.Patent(ctx, record); err == nil && p.FetchState == domain.FetchCached {
					initialState = domain.ReviewStateCached
				}
			}

			m := domain.Membership{
				Project:     project,
				Patent:      record,
				ReviewState: initialState,
				AddedAt:     time.Now().UTC(),
			}
			if err := e.repo.AddMembership(ctx, m); err != nil {
				e.log(ctx, slog.LevelError, "add membership in SetReviewState failed",
					slog.String("project_id", string(project)),
					slog.String("patent", record.String()),
					slog.String("error", err.Error()))
				return err
			}

			e.log(ctx, slog.LevelInfo, "membership added",
				slog.String("project_id", string(project)),
				slog.String("record", record.String()),
				slog.String("requested", record.String()))

			e.recordActivity(ctx, observability.Record{
				Action:   "membership.add",
				Entity:   "membership",
				EntityID: string(project) + "/" + record.String(),
				Status:   "committed",
				Before:   domain.Membership{},
				After:    m,
				Metadata: map[string]any{"requested_number": record.String()},
			})

			addedRecords = append(addedRecords, record)

			current, err = e.repo.Membership(ctx, project, record)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		if !current.ReviewState.CanTransitionTo(target) {
			return fmt.Errorf("engine: cannot move membership from %q to %q", current.ReviewState, target)
		}
		beforeMemberships[record] = current
	}

	// Segregate actual changes from no-ops
	changedRecords := make([]domain.PatentNumber, 0, len(records))
	noopsCount := 0
	for _, record := range records {
		current := beforeMemberships[record]
		if current.ReviewState == target {
			noopsCount++
			e.log(ctx, slog.LevelDebug, "review state unchanged", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("state", string(target)))
			continue
		}
		changedRecords = append(changedRecords, record)
	}

	if noopsCount > 0 {
		e.incCounter("engine.review_state.noop_total", int64(noopsCount))
	}

	if len(changedRecords) > 0 {
		// Update states
		if err := e.repo.SetReviewStates(ctx, project, changedRecords, target); err != nil {
			e.log(ctx, slog.LevelError, "set review state batch failed", slog.String("project_id", string(project)), slog.Int("count", len(changedRecords)), slog.String("target", string(target)), slog.String("error", err.Error()))
			return err
		}

		items := make([]domain.MutationItem, 0, len(changedRecords))
		for idx, record := range changedRecords {
			current := beforeMemberships[record]
			after := current
			after.ReviewState = target
			items = append(items, domain.MutationItem{
				Patent:  record,
				Kind:    "membership.state",
				Ordinal: idx + 1,
				Before:  current,
				After:   after,
				Inverse: map[string]any{"state": current.ReviewState},
			})
		}
		mutationGroupID := e.saveMutationGroup(ctx, project, "membership.set_state", changedRecords, map[string]any{"target_state": target}, items)

		// Record activities
		for _, record := range changedRecords {
			e.log(ctx, slog.LevelInfo, "review state changed", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("to", string(target)))
			metadata := map[string]any{
				"requested_number": record.String(),
				"project":          string(project),
			}
			if mutationGroupID != "" {
				metadata["mutation_group_id"] = mutationGroupID
			}
			rec := observability.Record{
				Action:   "membership.set_state",
				Entity:   "membership",
				EntityID: string(project) + "/" + record.String(),
				Status:   "committed",
				Metadata: metadata,
			}
			if current, ok := beforeMemberships[record]; ok {
				after := current
				after.ReviewState = target
				rec.Before = current
				rec.After = after
			}
			e.recordActivity(ctx, rec)
		}
		e.incCounter("engine.review_state.changed_total", int64(len(changedRecords)))
	}

	// Start family crawls for newly added stub memberships that start as "stored".
	if e.crawl != nil {
		for _, record := range addedRecords {
			initialState := domain.ReviewStateStored
			created := createdMap[record]
			if !created {
				if p, err := e.repo.Patent(ctx, record); err == nil && p.FetchState == domain.FetchCached {
					initialState = domain.ReviewStateCached
				}
			}

			if initialState == domain.ReviewStateStored {
				if jobID, fetchErr := e.StartFamilyCrawl(ctx, record, 0, domain.CrawlProfileAll, false); fetchErr != nil {
					e.log(ctx, slog.LevelWarn, "auto-fetch on add failed to start",
						slog.String("record", record.String()), slog.String("error", fetchErr.Error()))
				} else {
					go e.cleanupIfNotFound(project, record, created, jobID)
				}
			}
		}
	}

	e.announceChange()
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
