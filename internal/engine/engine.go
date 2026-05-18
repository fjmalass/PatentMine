// Package engine is the daemon core. It owns the store, the ingest worker
// pool, and the event bus, and exposes the operations that frontends invoke
// (directly in tests, or over RPC in the running system).
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/store"
)

// ingestWorkers is the number of concurrent ingest jobs.
const ingestWorkers = 4

// IngestFactory builds the Job that crawls a patent family. It is injected so
// the engine does not depend on the ingest package directly: a stub factory
// works for tests, the real crawler is wired in at daemon startup.
type IngestFactory func(root domain.PatentNumber, depth int) Job

// Option customizes an Engine.
type Option func(*Engine)

// Engine is the daemon core. Its methods are the single set of operations the
// system supports; RPC handlers and embedded callers both go through them.
type Engine struct {
	repo       store.Repository
	bus        *Bus
	pool       *workerPool
	ingest     IngestFactory
	logger     *slog.Logger
	activities *observability.Recorder
	metrics    *observability.Metrics
}

// WithLogger records structured logs for engine operations.
func WithLogger(logger *slog.Logger) Option {
	return func(e *Engine) {
		if logger != nil {
			e.logger = logger
		}
	}
}

// WithActivityRecorder enables semantic activity journaling.
func WithActivityRecorder(rec *observability.Recorder) Option {
	return func(e *Engine) {
		e.activities = rec
	}
}

// WithMetrics enables in-process timings and counters.
func WithMetrics(metrics *observability.Metrics) Option {
	return func(e *Engine) {
		e.metrics = metrics
	}
}

// New builds an Engine. The pool's jobs are children of ctx, so cancelling ctx
// stops all background work. ingest may be nil if no crawl will be started.
func New(ctx context.Context, repo store.Repository, ingest IngestFactory, opts ...Option) *Engine {
	bus := NewBus()
	eng := &Engine{
		repo:   repo,
		bus:    bus,
		pool:   newWorkerPool(ctx, ingestWorkers, bus, nil),
		ingest: ingest,
		logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
	for _, opt := range opts {
		opt(eng)
	}
	eng.pool.metrics = eng.metrics
	eng.bus.metrics = eng.metrics
	return eng
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
func (e *Engine) Patent(ctx context.Context, n domain.PatentNumber) (patent domain.Patent, err error) {
	defer e.observeDuration("engine.patent", time.Now(), &err)
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
func (e *Engine) ListPatents(ctx context.Context, q store.PatentQuery) (rows []domain.PatentRow, total int, err error) {
	defer e.observeDuration("engine.list_patents", time.Now(), &err)
	patents, err := e.repo.ListPatents(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err = e.repo.CountPatents(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	return patents, total, nil
}

// SavePatent inserts or updates a patent and announces the change.
func (e *Engine) SavePatent(ctx context.Context, p domain.Patent) (err error) {
	defer e.observeDuration("engine.save_patent", time.Now(), &err)
	before, _ := e.existingPatent(ctx, p.Number)
	if err := e.repo.SavePatent(ctx, p); err != nil {
		e.log(ctx, slog.LevelError, "save patent failed", slog.String("number", p.Number.String()), slog.String("error", err.Error()))
		return err
	}
	e.log(ctx, slog.LevelInfo, "patent saved", slog.String("number", p.Number.String()))
	e.recordActivity(ctx, observability.Record{
		Action:   "patent.save",
		Entity:   "patent",
		EntityID: p.Number.String(),
		Status:   "committed",
		Before:   before,
		After:    p,
	})
	e.announceChange()
	return nil
}

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
// record before the membership is created.
func (e *Engine) AddToProject(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (err error) {
	defer e.observeDuration("engine.add_to_project", time.Now(), &err)
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return err
	}
	before, _ := e.existingMembership(ctx, project, record)
	after := domain.Membership{
		Project: project,
		Patent:  record,
		State:   domain.MembershipStored,
		AddedAt: time.Now().UTC(),
	}
	err = e.repo.AddMembership(ctx, domain.Membership{
		Project: after.Project,
		Patent:  after.Patent,
		State:   after.State,
		AddedAt: after.AddedAt,
	})
	if err != nil {
		e.log(ctx, slog.LevelError, "add membership failed", slog.String("project_id", string(project)), slog.String("patent", patent.String()), slog.String("error", err.Error()))
		return err
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
	return nil
}

// SetMembershipState changes a membership's state, rejecting transitions the
// domain rules disallow. This is where the state machine is enforced.
func (e *Engine) SetMembershipState(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, target domain.MembershipState) (err error) {
	defer e.observeDuration("engine.set_membership_state", time.Now(), &err)
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
		e.log(ctx, slog.LevelError, "set membership state failed", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("target", string(target)), slog.String("error", err.Error()))
		return err
	}
	after := current
	after.State = target
	e.log(ctx, slog.LevelInfo, "membership state changed", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("from", string(current.State)), slog.String("to", string(target)))
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
	return nil
}

// StartFamilyIngest enqueues a family-graph crawl and returns its job id.
func (e *Engine) StartFamilyIngest(root domain.PatentNumber, depth int) (id JobID, err error) {
	defer e.observeDuration("engine.start_family_ingest", time.Now(), &err)
	if e.ingest == nil {
		return "", errors.New("engine: no ingest factory configured")
	}
	if root.IsZero() {
		return "", errors.New("engine: ingest root must not be empty")
	}
	id, err = e.pool.submit(e.ingest(root, depth))
	if err != nil {
		e.log(context.Background(), slog.LevelError, "ingest enqueue failed", slog.String("root", root.String()), slog.Int("depth", depth), slog.String("error", err.Error()))
		return "", err
	}
	e.log(context.Background(), slog.LevelInfo, "ingest enqueued", slog.String("job_id", string(id)), slog.String("root", root.String()), slog.Int("depth", depth))
	e.recordActivity(context.Background(), observability.Record{
		Action:   "ingest.start",
		Entity:   "job",
		EntityID: string(id),
		Status:   "queued",
		After:    map[string]any{"job_id": string(id), "root": root.String(), "depth": depth},
	})
	return id, nil
}

// Relations returns family-graph edges of one kind from a patent.
func (e *Engine) Relations(ctx context.Context, number domain.PatentNumber, kind domain.RelationKind) (rels []domain.Relation, err error) {
	defer e.observeDuration("engine.relations", time.Now(), &err)
	if !kind.Valid() {
		return nil, fmt.Errorf("engine: invalid relation kind %q", kind)
	}
	return e.repo.Relations(ctx, number, kind)
}

// ExportIDS builds an Information Disclosure Statement for a project: every
// patent that is a member of the project and not ignored or deleted. The
// result is a draft that the patent office filing is generated from.
func (e *Engine) ExportIDS(ctx context.Context, projectID domain.ProjectID) (ids domain.IDS, err error) {
	defer e.observeDuration("engine.export_ids", time.Now(), &err)
	if _, err := e.repo.Project(ctx, projectID); err != nil {
		return domain.IDS{}, err
	}
	members, err := e.repo.Memberships(ctx, projectID)
	if err != nil {
		return domain.IDS{}, err
	}
	ids = domain.IDS{
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
func (e *Engine) CancelIngest(id JobID) (err error) {
	defer e.observeDuration("engine.cancel_ingest", time.Now(), &err)
	if !e.pool.cancel(id) {
		return fmt.Errorf("engine: no such job %q", id)
	}
	e.log(context.Background(), slog.LevelInfo, "ingest cancelled", slog.String("job_id", string(id)))
	e.recordActivity(context.Background(), observability.Record{
		Action:   "ingest.cancel",
		Entity:   "job",
		EntityID: string(id),
		Status:   "committed",
		After:    map[string]any{"job_id": string(id), "cancelled": true},
	})
	return nil
}

// MetricsSnapshot returns the current timing/counter aggregates.
func (e *Engine) MetricsSnapshot() proto.MetricsSnapshot {
	if e.metrics == nil {
		return proto.MetricsSnapshot{Timestamp: time.Now().UTC(), Timings: map[string]proto.TimingMetric{}, Counters: map[string]int64{}, Gauges: map[string]int64{}}
	}
	snap := e.metrics.Snapshot()
	timings := make(map[string]proto.TimingMetric, len(snap.Timings))
	for k, v := range snap.Timings {
		avgNanos := v.AvgNanos()
		timings[k] = proto.TimingMetric{
			Count:      v.Count,
			Errors:     v.Errors,
			TotalNanos: v.TotalNanos,
			AvgNanos:   avgNanos,
			AvgMillis:  avgNanos / int64(time.Millisecond),
			MinNanos:   v.MinNanos,
			MinMillis:  v.MinNanos / int64(time.Millisecond),
			MaxNanos:   v.MaxNanos,
			MaxMillis:  v.MaxNanos / int64(time.Millisecond),
			LastNanos:  v.LastNanos,
			LastMillis: v.LastNanos / int64(time.Millisecond),
		}
	}
	return proto.MetricsSnapshot{
		Timestamp: snap.Timestamp,
		Timings:   timings,
		Counters:  snap.Counters,
		Gauges:    snap.Gauges,
	}
}

// Metrics returns the engine's in-process metrics recorder, when configured.
func (e *Engine) Metrics() *observability.Metrics {
	return e.metrics
}

// Logger returns the engine's structured logger.
func (e *Engine) Logger() *slog.Logger {
	return e.logger
}

// announceChange notifies subscribers that stored data changed.
func (e *Engine) announceChange() {
	e.bus.Publish(proto.NewEvent(proto.EventDBChanged, proto.Empty{}))
}

func (e *Engine) existingPatent(ctx context.Context, n domain.PatentNumber) (domain.Patent, bool) {
	p, err := e.repo.Patent(ctx, n)
	if err == nil {
		return p, true
	}
	return domain.Patent{}, false
}

func (e *Engine) existingMembership(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (domain.Membership, bool) {
	m, err := e.repo.Membership(ctx, project, patent)
	if err == nil {
		return m, true
	}
	return domain.Membership{}, false
}

func (e *Engine) log(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	logger := observability.WithContextAttrs(ctx, e.logger)
	args := make([]any, 0, len(attrs))
	for _, attr := range attrs {
		args = append(args, attr)
	}
	logger.Log(ctx, level, msg, args...)
}

func (e *Engine) recordActivity(ctx context.Context, rec observability.Record) {
	if e.activities == nil {
		return
	}
	if err := e.activities.Record(ctx, rec); err != nil {
		e.log(ctx, slog.LevelWarn, "activity record failed", slog.String("action", rec.Action), slog.String("error", err.Error()))
	}
}

func (e *Engine) observeDuration(name string, start time.Time, errp *error) {
	if e.metrics == nil {
		return
	}
	failed := errp != nil && *errp != nil
	e.metrics.ObserveDuration(name, time.Since(start), failed)
}
