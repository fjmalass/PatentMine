// Package engine is the daemon core. It owns the store, the ingest worker
// pool, and the event bus, and exposes the operations that frontends invoke
// (directly in tests, or over RPC in the running system).
package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/store"
)

// ingestWorkers is the number of concurrent ingest jobs.
const ingestWorkers = 4

// IngestFactory builds the Job that crawls a patent family. It is injected so
// the engine does not depend on the ingest package directly: a stub factory
// works for tests, the real crawler is wired in at daemon startup.
type IngestFactory func(root domain.PatentNumber, depth int) Job

// Engine is the daemon core. Its methods are the single set of operations the
// system supports; RPC handlers and embedded callers both go through them.
type Engine struct {
	repo   store.Repository
	bus    *Bus
	pool   *workerPool
	ingest IngestFactory
}

// New builds an Engine. The pool's jobs are children of ctx, so cancelling ctx
// stops all background work. ingest may be nil if no crawl will be started.
func New(ctx context.Context, repo store.Repository, ingest IngestFactory) *Engine {
	bus := NewBus()
	return &Engine{
		repo:   repo,
		bus:    bus,
		pool:   newWorkerPool(ctx, ingestWorkers, bus),
		ingest: ingest,
	}
}

// Close stops the worker pool and the event bus. The store is not closed here:
// its lifetime is owned by whoever opened it.
func (e *Engine) Close() {
	e.pool.shutdown()
	e.bus.Close()
}

// Subscribe registers an event subscriber; the returned function unsubscribes.
func (e *Engine) Subscribe() (<-chan proto.Event, func()) {
	return e.bus.Subscribe()
}

// Patent returns one patent.
func (e *Engine) Patent(ctx context.Context, n domain.PatentNumber) (domain.Patent, error) {
	record, err := e.recordNumber(ctx, n)
	if err != nil {
		return domain.Patent{}, err
	}
	return e.repo.Patent(ctx, record)
}

// recordNumber resolves any of a record's document numbers (application,
// publication, grant) to the record's permanent number. An unknown number is
// returned unchanged, so the caller's own lookup reports it as missing.
func (e *Engine) recordNumber(ctx context.Context, n domain.PatentNumber) (domain.PatentNumber, error) {
	record, err := e.repo.RecordOf(ctx, n)
	if err == nil {
		return record, nil
	}
	if errors.Is(err, store.ErrNotFound) {
		return n, nil
	}
	return domain.PatentNumber{}, err
}

// ListPatents returns one page of lightweight listing rows and the unpaged total.
func (e *Engine) ListPatents(ctx context.Context, q store.PatentQuery) ([]domain.PatentRow, int, error) {
	patents, err := e.repo.ListPatents(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err := e.repo.CountPatents(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return patents, total, nil
}

// SavePatent inserts or updates a patent and announces the change.
func (e *Engine) SavePatent(ctx context.Context, p domain.Patent) error {
	if err := e.repo.SavePatent(ctx, p); err != nil {
		return err
	}
	e.announceChange()
	return nil
}

// Projects returns every project.
func (e *Engine) Projects(ctx context.Context) ([]domain.Project, error) {
	return e.repo.ListProjects(ctx)
}

// CreateProject creates a project with a generated id.
func (e *Engine) CreateProject(ctx context.Context, name string) (domain.Project, error) {
	if name == "" {
		return domain.Project{}, errors.New("engine: project name must not be empty")
	}
	p := domain.Project{
		ID:        domain.ProjectID(fmt.Sprintf("p-%d", time.Now().UnixNano())),
		Name:      name,
		CreatedAt: time.Now().UTC(),
	}
	if err := e.repo.SaveProject(ctx, p); err != nil {
		return domain.Project{}, err
	}
	e.announceChange()
	return p, nil
}

// AddToProject adds a patent to a project in the default Stored state. The
// patent may be given by any of its document numbers; it is resolved to the
// record before the membership is created.
func (e *Engine) AddToProject(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) error {
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return err
	}
	err = e.repo.AddMembership(ctx, domain.Membership{
		Project: project,
		Patent:  record,
		State:   domain.MembershipStored,
		AddedAt: time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	e.announceChange()
	return nil
}

// SetMembershipState changes a membership's state, rejecting transitions the
// domain rules disallow. This is where the state machine is enforced.
func (e *Engine) SetMembershipState(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, target domain.MembershipState) error {
	if !target.Valid() {
		return fmt.Errorf("engine: invalid membership state %q", target)
	}
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return err
	}
	current, err := e.repo.Membership(ctx, project, record)
	if err != nil {
		return err
	}
	if !current.State.CanTransitionTo(target) {
		return fmt.Errorf("engine: cannot move membership from %q to %q", current.State, target)
	}
	if err := e.repo.SetMembershipState(ctx, project, record, target); err != nil {
		return err
	}
	e.announceChange()
	return nil
}

// StartFamilyIngest enqueues a family-graph crawl and returns its job id.
func (e *Engine) StartFamilyIngest(root domain.PatentNumber, depth int) (JobID, error) {
	if e.ingest == nil {
		return "", errors.New("engine: no ingest factory configured")
	}
	if root.IsZero() {
		return "", errors.New("engine: ingest root must not be empty")
	}
	return e.pool.submit(e.ingest(root, depth))
}

// Relations returns family-graph edges of one kind from a patent.
func (e *Engine) Relations(ctx context.Context, number domain.PatentNumber, kind domain.RelationKind) ([]domain.Relation, error) {
	if !kind.Valid() {
		return nil, fmt.Errorf("engine: invalid relation kind %q", kind)
	}
	return e.repo.Relations(ctx, number, kind)
}

// ExportIDS builds an Information Disclosure Statement for a project: every
// patent that is a member of the project and not ignored or deleted. The
// result is a draft that the patent office filing is generated from.
func (e *Engine) ExportIDS(ctx context.Context, projectID domain.ProjectID) (domain.IDS, error) {
	if _, err := e.repo.Project(ctx, projectID); err != nil {
		return domain.IDS{}, err
	}
	members, err := e.repo.Memberships(ctx, projectID)
	if err != nil {
		return domain.IDS{}, err
	}
	ids := domain.IDS{
		Project:     projectID,
		Status:      domain.IDSDraft,
		GeneratedAt: time.Now().UTC(),
	}
	for _, m := range members {
		if m.State == domain.MembershipIgnored || m.State == domain.MembershipDeleted {
			continue // not disclosed to the patent office
		}
		patent, err := e.repo.Patent(ctx, m.Patent)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return domain.IDS{}, err
		}
		doc, ok := idsDocument(patent)
		if !ok {
			continue // only an application exists — nothing publishable to disclose
		}
		ids.Entries = append(ids.Entries, domain.IDSEntry{
			Number: doc.Number,
			Title:  patent.Title,
		})
	}
	return ids, nil
}

// idsDocument picks the document an IDS should disclose for a record: the
// grant if it has one, otherwise the publication. A record with only an
// application has nothing the patent office can be pointed at.
func idsDocument(p domain.Patent) (domain.Document, bool) {
	if doc, ok := p.DocumentFor(domain.StageGrant); ok {
		return doc, true
	}
	return p.DocumentFor(domain.StagePublication)
}

// CancelIngest stops a running or queued ingest job.
func (e *Engine) CancelIngest(id JobID) error {
	if !e.pool.cancel(id) {
		return fmt.Errorf("engine: no such job %q", id)
	}
	return nil
}

// announceChange notifies subscribers that stored data changed.
func (e *Engine) announceChange() {
	e.bus.Publish(proto.NewEvent(proto.EventDBChanged, proto.Empty{}))
}
