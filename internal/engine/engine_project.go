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
		Action:   observability.ActionProjectCreate,
		Entity:   "project",
		EntityID: string(p.ID),
		Status:   "committed",
		After:    p,
	})
	e.announceChange()
	return p, nil
}

// AddToProject adds a patent to a project in the default Unknown state. The
// patent may be given by any of its document numbers; it is resolved to the
// record before the membership is created. A patent that has never been
// fetched is recorded as a stub first, so it can be tracked in a project
// ahead of crawling. fetchStarted reports whether an automatic single-patent
// fetch was enqueued; false means the caller should prompt the user to fetch
// manually (e.g. with F).
// AddToProject adds a patent to a project in the default Unknown state. The
// patent may be given by any of its document numbers; it is resolved to the
// record before the membership is created. A patent that has never been
// fetched is recorded as a stub first, so it can be tracked in a project
// ahead of crawling. fetchStarted reports whether an automatic single-patent
// fetch was enqueued; false means the caller should prompt the user to fetch
// manually (e.g. with F).
func (e *Engine) AddToProject(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (AddToProjectResult, error) {
	return e.addToProject(ctx, project, patent, "", "")
}

// AddToProjectFromSource adds a patent and forces a single-patent fetch from
// the selected provider. It never falls back to another source.
func (e *Engine) AddToProjectFromSource(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, source domain.Source, applicationNumber string) (AddToProjectResult, error) {
	return e.addToProject(ctx, project, patent, source, applicationNumber)
}

// AddToProjectResult reports the outcome of an add-to-project call.
// AlreadyExisted is true when the membership was already present; in that
// case no insert, log-info, metric, activity, or auto-crawl was performed.
type AddToProjectResult struct {
	FetchStarted   bool
	JobID          JobID
	Candidates     []domain.USPTOCandidate
	AlreadyExisted bool
}

func (e *Engine) addToProject(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, source domain.Source, applicationNumber string) (res AddToProjectResult, err error) {
	defer e.observeDuration("engine.add_to_project", time.Now(), &err)

	if source == domain.SourceUSPTO && e.usptoSearcher != nil {
		if applicationNumber != "" {
			// Candidate already selected by user — skip re-search, use directly.
			if exact, err2 := domain.ParsePatentNumber("US" + applicationNumber); err2 == nil {
				patent = exact
			}
			e.log(ctx, slog.LevelInfo, "engine.add.uspto.candidate_selected",
				slog.String("requested_number", patent.String()),
				slog.String("app_number", applicationNumber))
			if e.metrics != nil {
				e.metrics.IncCounter("engine.add.uspto_candidate.direct", 1)
			}
		} else {
			searchStart := time.Now()
			candidates, searchErr := e.usptoSearcher(ctx, patent)
			searchDur := time.Since(searchStart)

			if e.metrics != nil {
				e.metrics.ObserveDuration("uspto.candidate.search", searchDur, searchErr != nil)
				if searchErr != nil {
					e.metrics.IncCounter("uspto.candidate.search.error", 1)
				} else {
					e.metrics.IncCounter("uspto.candidate.search.success", 1)
					e.metrics.IncCounter("uspto.candidate.count", int64(len(candidates)))
				}
			}

			if searchErr != nil {
				e.log(ctx, slog.LevelWarn, "uspto searcher failed during add (may be grant number vs application mismatch)",
					slog.String("requested_number", patent.String()),
					slog.String("error", searchErr.Error()),
					slog.Duration("duration", searchDur))
				if e.metrics != nil {
					e.metrics.IncCounter("engine.add.uspto_search.error", 1)
				}
				e.recordActivity(ctx, observability.Record{
					Action:   observability.ActionUSPTOCandidateSearch,
					Entity:   "uspto_resolution",
					Status:   "error",
					EntityID: patent.String(),
					Attributes: map[string]any{
						"error":       searchErr.Error(),
						"duration_ms": searchDur.Milliseconds(),
					},
				})
				return AddToProjectResult{}, searchErr
			}
			if len(candidates) > 1 {
				e.log(ctx, slog.LevelInfo, "engine.add.uspto.candidates_found",
					slog.String("requested_number", patent.String()),
					slog.Int("count", len(candidates)))
				if e.metrics != nil {
					e.metrics.IncCounter("engine.add.uspto_candidate.picker_shown", 1)
				}
				return AddToProjectResult{Candidates: candidates}, nil
			}
			if len(candidates) == 1 {
				if exact, err2 := domain.ParsePatentNumber("US" + candidates[0].ApplicationNumber); err2 == nil {
					patent = exact
				}
				e.log(ctx, slog.LevelInfo, "engine.add.uspto.auto_selected",
					slog.String("requested_number", patent.String()),
					slog.String("app_number", candidates[0].ApplicationNumber))
				if e.metrics != nil {
					e.metrics.IncCounter("engine.add.uspto_candidate.auto_selected", 1)
				}
			}
		}
	}

	record, created, err := e.ensureRecord(ctx, patent)
	if err != nil {
		return AddToProjectResult{}, err
	}
	if _, exists := e.existingMembership(ctx, project, record); exists {
		// Duplicate add: do nothing to storage, log + metric so the caller and
		// observers can see that the request was a no-op rather than a fresh
		// insert. The TUI surfaces this as a distinct status line.
		e.log(ctx, slog.LevelWarn, "membership add skipped: already exists",
			slog.String("project_id", string(project)),
			slog.String("record", record.String()),
			slog.String("requested", patent.String()),
			slog.String("source", string(source)))
		if e.metrics != nil {
			e.metrics.IncCounter("engine.membership.add_duplicate_total", 1)
			if source != "" {
				e.metrics.IncCounter("engine.membership.add_duplicate.source."+string(source)+"_total", 1)
			}
		}
		e.recordActivity(ctx, observability.Record{
			Action:   observability.ActionMembershipAdd,
			Entity:   "membership",
			EntityID: string(project) + "/" + record.String(),
			Status:   "skipped",
			Attributes: map[string]any{
				"requested_number": patent.String(),
				"project":          string(project),
				"source":           string(source),
				"reason":           "already_exists",
			},
		})
		return AddToProjectResult{AlreadyExisted: true}, nil
	}
	after := domain.Membership{
		Project:     project,
		Patent:      record,
		ReviewState: domain.ReviewStateUnderReview,
		AddedAt:     time.Now().UTC(),
	}
	if err := e.repo.AddMembership(ctx, after); err != nil {
		e.log(ctx, slog.LevelError, "add membership failed", slog.String("project_id", string(project)), slog.String("patent", patent.String()), slog.String("error", err.Error()))
		return AddToProjectResult{}, err
	}
	e.log(ctx, slog.LevelInfo, "membership added", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("requested", patent.String()))
	if e.metrics != nil {
		e.metrics.IncCounter("engine.membership.add_total", 1)
		if source != "" {
			e.metrics.IncCounter("engine.membership.add.source."+string(source)+"_total", 1)
		}
	}
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionMembershipAdd,
		Entity:   "membership",
		EntityID: string(project) + "/" + record.String(),
		Status:   "committed",
		Before:   domain.Membership{},
		After:    after,
		Attributes: map[string]any{
			"requested_number": patent.String(),
			"project":          string(project),
			"source":           string(source),
		},
	})
	e.announceChange()
	if source != "" && e.crawl != nil {
		ch, unsub := e.pool.bus.Subscribe()
		jobID, fetchErr := e.StartFamilyCrawlFromSource(ctx, record, 0, domain.CrawlProfileAll, source, true)
		if fetchErr != nil {
			unsub()
			e.log(ctx, slog.LevelWarn, "source-specific fetch on add failed to start",
				slog.String("record", record.String()), slog.String("source", string(source)), slog.String("error", fetchErr.Error()))
			// Clean up newly created database state on start failure
			if created {
				_ = e.repo.DeletePatent(ctx, record)
			} else {
				_ = e.repo.DeleteMembership(ctx, project, record)
			}
			e.announceChange()
			return AddToProjectResult{}, fetchErr
		}
		if created {
			go e.cleanupIfNotFound(project, record, created, jobID)
		}
		go e.autoAssignDepth1Neighbors(project, record, jobID, ch, unsub)
		return AddToProjectResult{FetchStarted: true, JobID: jobID}, nil
	}
	// Any unknown project membership for a stub patent kicks a single-patent
	// fetch so bibliographic data fills in shortly after the patent is added.
	// This covers both brand-new stubs (created==true) and existing stubs that
	// were previously discovered as citation edges.
	needsFetch := created
	if !created {
		if p, err := e.repo.Patent(ctx, record); err == nil {
			needsFetch = p.FetchState == domain.FetchStub
		}
	}
	if needsFetch && e.crawl != nil {
		ch, unsub := e.pool.bus.Subscribe()
		jobID, fetchErr := e.StartFamilyCrawl(ctx, record, 0, domain.CrawlProfileAll, false)
		if fetchErr != nil {
			unsub()
			e.log(ctx, slog.LevelWarn, "auto-fetch on add failed to start",
				slog.String("record", record.String()), slog.String("error", fetchErr.Error()))
			return AddToProjectResult{}, nil
		}
		go e.cleanupIfNotFound(project, record, created, jobID)
		go e.autoAssignDepth1Neighbors(project, record, jobID, ch, unsub)
		return AddToProjectResult{FetchStarted: true, JobID: jobID}, nil
	}
	return AddToProjectResult{}, nil
}

// AddRelated grants project membership to every family-graph neighbor of
// patent that does not yet have one. It is the explicit "promote the stubs
// the lookup discovered" operation: the caller has chosen which patent's
// neighborhood matters, so the engine just walks the existing relations and
// inserts the missing rows. Returns the neighbor records that were promoted.
func (e *Engine) AddRelated(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (added []domain.PatentNumber, err error) {
	defer e.observeDuration("engine.add_related", time.Now(), &err)
	if project == "" {
		return nil, errors.New("engine: project required")
	}
	record, err := e.repo.RecordOf(ctx, patent)
	if err != nil {
		return nil, err
	}
	rels, err := e.repo.AllRelations(ctx, record)
	if err != nil {
		return nil, err
	}
	seen := map[domain.PatentNumber]bool{record: true}
	for _, rel := range rels {
		var neighbor domain.PatentNumber
		switch record {
		case rel.From:
			neighbor = rel.To
		case rel.To:
			neighbor = rel.From
		default:
			continue
		}
		if seen[neighbor] {
			continue
		}
		seen[neighbor] = true
		neighborRecord, err := e.repo.RecordOf(ctx, neighbor)
		if err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				return added, err
			}
			neighborRecord = neighbor
		}
		if _, exists := e.existingMembership(ctx, project, neighborRecord); exists {
			continue
		}
		m := domain.Membership{
			Project:     project,
			Patent:      neighborRecord,
			ReviewState: domain.ReviewStateUnderReview,
			AddedAt:     time.Now().UTC(),
		}
		if err := e.repo.AddMembership(ctx, m); err != nil {
			e.log(ctx, slog.LevelError, "add related: membership insert failed",
				slog.String("project_id", string(project)),
				slog.String("neighbor", neighborRecord.String()),
				slog.String("error", err.Error()))
			return added, err
		}
		e.recordActivity(ctx, observability.Record{
			Action:   observability.ActionMembershipAdd,
			Entity:   "membership",
			EntityID: string(project) + "/" + neighborRecord.String(),
			Status:   "committed",
			After:    m,
			Attributes: map[string]any{
				"requested_number": neighborRecord.String(),
				"project":          string(project),
				"source":           "add.related",
				"root":             record.String(),
			},
		})
		added = append(added, neighborRecord)
	}
	if e.metrics != nil {
		e.metrics.IncCounter("engine.add_related.total", 1)
		e.metrics.IncCounter("engine.add_related.neighbors_added", int64(len(added)))
	}
	e.log(ctx, slog.LevelInfo, "add related complete",
		slog.String("project_id", string(project)),
		slog.String("root", record.String()),
		slog.Int("added", len(added)))
	if len(added) > 0 {
		e.announceChange()
	}
	return added, nil
}

// autoAssignDepth1Neighbors watches a freshly started single-patent fetch job
// and grants membership to every direct neighbor that the crawl ingests at
// depth 0. It is the engine-side companion of :add: the user added one patent;
// the citation/parent stubs the crawl just discovered get the same membership
// so they appear in the project right away. Depth-1 only — deeper transitive
// stubs stay orphan unless the user runs :add.related explicitly.
func (e *Engine) autoAssignDepth1Neighbors(project domain.ProjectID, root domain.PatentNumber, id JobID, ch <-chan proto.Event, unsub func()) {
	defer unsub()
	rootStr := root.String()
	added := 0
	for ev := range ch {
		switch proto.EventKind(ev.Method) {
		case proto.EventCrawlProgress:
			var p proto.CrawlProgress
			if err := json.Unmarshal(ev.Params, &p); err != nil || p.JobID != string(id) {
				continue
			}
			if p.Depth != 0 || p.RecordNumber != rootStr || len(p.Stubs) == 0 {
				continue
			}
			ctx := context.Background()
			for _, s := range p.Stubs {
				neighbor, parseErr := domain.ParsePatentNumber(s)
				if parseErr != nil {
					continue
				}
				if _, exists := e.existingMembership(ctx, project, neighbor); exists {
					continue
				}
				m := domain.Membership{
					Project:     project,
					Patent:      neighbor,
					ReviewState: domain.ReviewStateUnknown,
					AddedAt:     time.Now().UTC(),
				}
				if err := e.repo.AddMembership(ctx, m); err != nil {
					e.log(ctx, slog.LevelWarn, "auto-assign depth-1 neighbor membership failed",
						slog.String("project_id", string(project)),
						slog.String("neighbor", neighbor.String()),
						slog.String("error", err.Error()))
					continue
				}
				e.recordActivity(ctx, observability.Record{
					Action:   observability.ActionMembershipAdd,
					Entity:   "membership",
					EntityID: string(project) + "/" + neighbor.String(),
					Status:   "committed",
					After:    m,
					Attributes: map[string]any{
						"requested_number": neighbor.String(),
						"project":          string(project),
						"source":           "auto.depth1",
						"root":             rootStr,
						"job_id":           string(id),
					},
				})
				added++
				if e.metrics != nil {
					e.metrics.IncCounter("engine.auto_assign.depth1.neighbors_added", 1)
				}
			}
			e.announceChange()
		case proto.EventCrawlDone:
			var d proto.CrawlDone
			if err := json.Unmarshal(ev.Params, &d); err == nil && d.JobID == string(id) {
				e.pool.bus.Publish(proto.NewEvent(proto.EventMembershipAutoAssign, proto.MembershipAutoAssign{
					JobID:   string(id),
					Project: string(project),
					Root:    rootStr,
					Added:   added,
				}))
				return
			}
		}
	}
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

		e.recordActivity(ctx, observability.Record{
			Action:   observability.ActionCrawlRootNotFound,
			Entity:   "crawl",
			Status:   "error",
			EntityID: record.String(),
			Attributes: map[string]any{
				"job_id":  d.JobID,
				"error":   d.Error,
				"project": string(project),
			},
		})
		membership, err := e.repo.Membership(ctx, project, record)
		if err == nil && membership.ReviewState != domain.ReviewStateUnknown && membership.ReviewState != domain.ReviewStateUnderReview {
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
			initialState := domain.ReviewStateUnderReview

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
				Action:     observability.ActionMembershipAdd,
				Entity:     "membership",
				EntityID:   string(project) + "/" + record.String(),
				Status:     "committed",
				Before:     domain.Membership{},
				After:      m,
				Attributes: map[string]any{"requested_number": record.String()},
			})

			addedRecords = append(addedRecords, record)

			current, err = e.repo.Membership(ctx, project, record)
			if err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		p, err := e.repo.Patent(ctx, record)
		if err != nil {
			return err
		}
		if !current.ReviewState.CanTransitionTo(target, p.FetchState) {
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
			current := beforeMemberships[record]
			attrs := map[string]any{
				"requested_number": record.String(),
				"project":          string(project),
				"prior_state":      string(current.ReviewState),
				"state":            string(target),
			}
			if mutationGroupID != "" {
				attrs["mutation_group_id"] = mutationGroupID
			}
			rec := observability.Record{
				Action:     observability.ActionMembershipSetState,
				Entity:     "membership",
				EntityID:   string(project) + "/" + record.String(),
				Status:     "committed",
				Attributes: attrs,
			}
			after := current
			after.ReviewState = target
			rec.Before = current
			rec.After = after
			e.recordActivity(ctx, rec)
		}
		e.incCounter("engine.review_state.changed_total", int64(len(changedRecords)))
	}

	// Start family crawls for newly added unknown memberships backed by stubs.
	if e.crawl != nil {
		for _, record := range addedRecords {
			created := createdMap[record]
			needsFetch := created
			if !created {
				if p, err := e.repo.Patent(ctx, record); err == nil {
					needsFetch = p.FetchState == domain.FetchStub
				}
			}
			if needsFetch {
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
