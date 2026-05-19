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
	"strings"
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
// works for tests, the real crawler is wired in at daemon startup. force makes
// the crawl bypass the local file cache and re-fetch from the web.
type IngestFactory func(root domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool) Job

// FileImporter loads a patent record from a local file into the store. Like
// IngestFactory it is injected, so the engine never imports the ingest package.
type FileImporter interface {
	ImportFile(ctx context.Context, path string) error
}

// Option customizes an Engine.
type Option func(*Engine)

// Engine is the daemon core. Its methods are the single set of operations the
// system supports; RPC handlers and embedded callers both go through them.
type Engine struct {
	repo         store.Repository
	bus          *Bus
	pool         *workerPool
	ingest       IngestFactory
	fileImporter FileImporter
	logger       *slog.Logger
	activities   *observability.Recorder
	metrics      *observability.Metrics
}

// WithFileImporter wires the file-import backend used by ImportFile.
func WithFileImporter(fi FileImporter) Option {
	return func(e *Engine) { e.fileImporter = fi }
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

// ensureRecord resolves a number to its record, creating a stub patent (a
// patent row plus one document) when the number is not yet known. Membership
// rows reference the patent table by foreign key, so a patent must exist as at
// least a stub before it can be added to a project. The returned bool reports
// whether a new stub was created — the caller uses it to trigger a first fetch.
func (e *Engine) ensureRecord(ctx context.Context, n domain.PatentNumber) (domain.PatentNumber, bool, error) {
	record, err := e.repo.RecordOf(ctx, n)
	if err == nil {
		return record, false, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return domain.PatentNumber{}, false, err
	}
	stub := domain.Patent{
		Number:        n,
		DisplayNumber: n,
		FetchState:    domain.FetchStub,
	}
	if err := e.repo.SavePatent(ctx, stub); err != nil {
		return domain.PatentNumber{}, false, err
	}
	if err := e.repo.SaveDocument(ctx, n, domain.Document{
		Number: n,
		Stage:  domain.GuessStage(n),
	}); err != nil {
		return domain.PatentNumber{}, false, err
	}
	return n, true, nil
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

// DeletePatent permanently removes a patent and all associated data.
func (e *Engine) DeletePatent(ctx context.Context, n domain.PatentNumber) (err error) {
	defer e.observeDuration("engine.delete_patent", time.Now(), &err)
	if err := e.repo.DeletePatent(ctx, n); err != nil {
		e.log(ctx, slog.LevelError, "delete patent failed", slog.String("number", n.String()), slog.String("error", err.Error()))
		return err
	}
	e.log(ctx, slog.LevelInfo, "patent deleted", slog.String("number", n.String()))
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
// record before the membership is created. A patent that has never been
// fetched is recorded as a stub first, so it can be tracked in a project
// ahead of ingestion.
func (e *Engine) AddToProject(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (err error) {
	defer e.observeDuration("engine.add_to_project", time.Now(), &err)
	record, created, err := e.ensureRecord(ctx, patent)
	if err != nil {
		return err
	}
	before, _ := e.existingMembership(ctx, project, record)
	state := domain.ReviewStateStored
	if !created {
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
	// Any patent in "stored" state has no full record yet — kick a single-patent
	// fetch so bibliographic data fills in shortly after the patent is added.
	// This covers both brand-new stubs (created==true) and existing stubs that
	// were previously discovered as citation edges.
	if state == domain.ReviewStateStored && e.ingest != nil {
		if _, fetchErr := e.StartFamilyIngest(record, 0, domain.CrawlProfileAll, false); fetchErr != nil {
			e.log(ctx, slog.LevelWarn, "auto-fetch on add failed to start",
				slog.String("record", record.String()), slog.String("error", fetchErr.Error()))
		}
	}
	return nil
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
	if err != nil {
		return err
	}
	if !current.ReviewState.CanTransitionTo(target) {
		return fmt.Errorf("engine: cannot move membership from %q to %q", current.ReviewState, target)
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

// PatentTags returns the tags a patent carries within a project. It is empty
// when no project is named: tags, like membership, are project-scoped.
func (e *Engine) PatentTags(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (tags []domain.Tag, err error) {
	defer e.observeDuration("engine.patent_tags", time.Now(), &err)
	if project == "" {
		return nil, nil
	}
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return nil, err
	}
	return e.repo.PatentTags(ctx, project, record)
}

// AssignTag tags a patent within a project, creating the named tag when the
// project does not have it yet. The patent may be given by any of its document
// numbers; it is resolved to the record before the tag is assigned.
func (e *Engine) AssignTag(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, name string) (err error) {
	defer e.observeDuration("engine.assign_tag", time.Now(), &err)
	name = strings.TrimSpace(name)
	if project == "" {
		return errors.New("engine: tag needs a project")
	}
	if name == "" {
		return errors.New("engine: tag name must not be empty")
	}
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return err
	}
	tag, err := e.repo.CreateTag(ctx, project, name)
	if err != nil {
		e.log(ctx, slog.LevelError, "create tag failed", slog.String("project_id", string(project)), slog.String("name", name), slog.String("error", err.Error()))
		return err
	}
	if err := e.repo.TagPatent(ctx, tag.ID, record); err != nil {
		e.log(ctx, slog.LevelError, "tag patent failed", slog.String("record", record.String()), slog.String("name", name), slog.String("error", err.Error()))
		return err
	}
	e.log(ctx, slog.LevelInfo, "patent tagged", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("tag", name))
	e.recordActivity(ctx, observability.Record{
		Action:   "tag.assign",
		Entity:   "tag",
		EntityID: string(project) + "/" + record.String(),
		Status:   "committed",
		After:    map[string]any{"tag": name, "tag_id": tag.ID},
		Metadata: map[string]any{"requested_number": patent.String()},
	})
	e.announceChange()
	return nil
}

// RemoveTag removes a tag from a patent within a project. It reports an error
// when the patent does not carry a tag of that name.
func (e *Engine) RemoveTag(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, name string) (err error) {
	defer e.observeDuration("engine.remove_tag", time.Now(), &err)
	name = strings.TrimSpace(name)
	if project == "" {
		return errors.New("engine: tag needs a project")
	}
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return err
	}
	tags, err := e.repo.PatentTags(ctx, project, record)
	if err != nil {
		return err
	}
	for _, t := range tags {
		if !strings.EqualFold(t.Name, name) {
			continue
		}
		if err := e.repo.UntagPatent(ctx, t.ID, record); err != nil {
			e.log(ctx, slog.LevelError, "untag patent failed", slog.String("record", record.String()), slog.String("name", name), slog.String("error", err.Error()))
			return err
		}
		e.log(ctx, slog.LevelInfo, "patent untagged", slog.String("project_id", string(project)), slog.String("record", record.String()), slog.String("tag", t.Name))
		e.recordActivity(ctx, observability.Record{
			Action:   "tag.remove",
			Entity:   "tag",
			EntityID: string(project) + "/" + record.String(),
			Status:   "committed",
			After:    map[string]any{"tag": t.Name, "tag_id": t.ID},
		})
		e.announceChange()
		return nil
	}
	return fmt.Errorf("engine: patent %s has no tag %q", record, name)
}

// StartFamilyIngest enqueues a family-graph crawl and returns its job id. A
// force crawl bypasses the local file cache and re-fetches from the web.
func (e *Engine) StartFamilyIngest(root domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool) (id JobID, err error) {
	defer e.observeDuration("engine.start_family_ingest", time.Now(), &err)
	if e.ingest == nil {
		return "", errors.New("engine: no ingest factory configured")
	}
	if root.IsZero() {
		return "", errors.New("engine: ingest root must not be empty")
	}
	id, err = e.pool.submit(e.ingest(root, depth, profile, force))
	if err != nil {
		e.log(context.Background(), slog.LevelError, "ingest enqueue failed", slog.String("root", root.String()), slog.Int("depth", depth), slog.String("profile", string(profile)), slog.String("error", err.Error()))
		return "", err
	}
	e.log(context.Background(), slog.LevelInfo, "ingest enqueued", slog.String("job_id", string(id)), slog.String("root", root.String()), slog.Int("depth", depth), slog.Bool("force", force))
	e.recordActivity(context.Background(), observability.Record{
		Action:   "ingest.start",
		Entity:   "job",
		EntityID: string(id),
		Status:   "queued",
		After:    map[string]any{"job_id": string(id), "root": root.String(), "depth": depth, "force": force},
	})
	return id, nil
}

// ImportFile loads a patent record from a local fixture file into the store.
func (e *Engine) ImportFile(ctx context.Context, path string) (err error) {
	defer e.observeDuration("engine.import_file", time.Now(), &err)
	if e.fileImporter == nil {
		return errors.New("engine: no file importer configured")
	}
	if err := e.fileImporter.ImportFile(ctx, path); err != nil {
		e.log(ctx, slog.LevelError, "import file failed", slog.String("path", path), slog.String("error", err.Error()))
		return err
	}
	e.log(ctx, slog.LevelInfo, "file imported", slog.String("path", path))
	e.recordActivity(ctx, observability.Record{
		Action: "import.file", Entity: "patent", EntityID: path, Status: "committed",
	})
	e.announceChange()
	return nil
}

// Relations returns family-graph edges of one kind from a patent, as full
// listing rows.
func (e *Engine) Relations(ctx context.Context, q store.PatentQuery) (out []domain.PatentRow, total int, err error) {
	defer e.observeDuration("engine.relations", time.Now(), &err)
	if !q.RelationKind.Valid() {
		return nil, 0, fmt.Errorf("engine: invalid relation kind %q", q.RelationKind)
	}
	out, err = e.repo.ListPatents(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	total, err = e.repo.CountPatents(ctx, q)
	return out, total, err
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
		if m.ReviewState == domain.ReviewStateIgnored || m.ReviewState == domain.ReviewStateDeleted {
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
