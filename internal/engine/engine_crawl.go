package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/store"
)

// StartFamilyCrawl enqueues a family-graph crawl and returns its job id. A
// force crawl bypasses the local file cache and re-fetches from the web.
func (e *Engine) StartFamilyCrawl(root domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool) (id JobID, err error) {
	defer e.observeDuration("engine.start_family_crawl", time.Now(), &err)
	if e.crawl == nil {
		return "", errors.New("engine: no crawl factory configured")
	}
	if root.IsZero() {
		return "", errors.New("engine: crawl root must not be empty")
	}
	id, err = e.pool.submit(e.crawl(root, depth, profile, force))
	if err != nil {
		e.log(context.Background(), slog.LevelError, "crawl enqueue failed", slog.String("root", root.String()), slog.Int("depth", depth), slog.String("profile", string(profile)), slog.String("error", err.Error()))
		return "", err
	}
	e.log(context.Background(), slog.LevelInfo, "crawl enqueued", slog.String("job_id", string(id)), slog.String("root", root.String()), slog.Int("depth", depth), slog.Bool("force", force))
	e.recordActivity(context.Background(), observability.Record{
		Action:   "crawl.start",
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

// ExportIDS builds an Information Disclosure Statement for the curated IDS
// entries of a project. Only patents with explicit IDS entries are disclosed.
func (e *Engine) ExportIDS(ctx context.Context, projectID domain.ProjectID) (ids domain.IDS, err error) {
	defer e.observeDuration("engine.export_ids", time.Now(), &err)
	if _, err := e.repo.Project(ctx, projectID); err != nil {
		return domain.IDS{}, err
	}
	entries, err := e.repo.ListIDSEntries(ctx, projectID)
	if err != nil {
		return domain.IDS{}, err
	}
	ids = domain.IDS{
		Project:     projectID,
		Status:      domain.IDSDraft,
		GeneratedAt: time.Now().UTC(),
	}
	for _, entry := range entries {
		patent, err := e.repo.Patent(ctx, entry.Patent)
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
		ids.Entries = append(ids.Entries, domain.IDSReference{
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

// CancelCrawl stops a running or queued crawl job.
func (e *Engine) CancelCrawl(id JobID) (err error) {
	defer e.observeDuration("engine.cancel_crawl", time.Now(), &err)
	if !e.pool.cancel(id) {
		return fmt.Errorf("engine: no such job %q", id)
	}
	e.log(context.Background(), slog.LevelInfo, "crawl cancelled", slog.String("job_id", string(id)))
	e.recordActivity(context.Background(), observability.Record{
		Action:   "crawl.cancel",
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
