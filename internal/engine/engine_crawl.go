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
	"patentmine/internal/uspto"
)

// StartFamilyCrawl enqueues a family-graph crawl and returns its job id. A
// force crawl bypasses the local file cache and re-fetches from the web.
func (e *Engine) StartFamilyCrawl(ctx context.Context, root domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool) (id JobID, err error) {
	return e.startFamilyCrawl(ctx, root, depth, profile, force, "")
}

// StartFamilyCrawlFromSource enqueues a crawl constrained to one provider.
// When force is true it bypasses cache (suitable for explicit "L" lookups).
func (e *Engine) StartFamilyCrawlFromSource(ctx context.Context, root domain.PatentNumber, depth int, profile domain.CrawlProfile, source domain.Source, force bool) (id JobID, err error) {
	if source == "" {
		return e.StartFamilyCrawl(ctx, root, depth, profile, force)
	}
	return e.startFamilyCrawl(ctx, root, depth, profile, force, source)
}

func (e *Engine) startFamilyCrawl(ctx context.Context, root domain.PatentNumber, depth int, profile domain.CrawlProfile, force bool, source domain.Source) (id JobID, err error) {
	defer e.observeDuration("engine.start_family_crawl", time.Now(), &err)
	if e.crawl == nil {
		return "", errors.New("engine: no crawl factory configured")
	}
	if root.IsZero() {
		return "", errors.New("engine: crawl root must not be empty")
	}
	// Subscribe before submitting: a cached/fast crawl can publish CrawlDone
	// from its worker before the auto-fetch goroutine would otherwise register,
	// and Bus.Publish only reaches already-registered subscribers. Subscribing
	// here, before the job is enqueued, guarantees the event is observed.
	autoFetchCh, autoFetchUnsub := e.pool.bus.Subscribe()
	id, err = e.pool.submit(e.crawl(root, depth, profile, force, source))
	if err != nil {
		autoFetchUnsub()
		e.log(ctx, slog.LevelError, "crawl enqueue failed", slog.String("root", root.String()), slog.Int("depth", depth), slog.String("profile", string(profile)), slog.String("source", string(source)), slog.String("error", err.Error()))
		return "", err
	}
	// Auto-fetch USPTO XML after the crawl finishes for the root, but only
	// when the resolved Patent.Source is USPTO. This makes :lookup carry
	// the same XML ingestion as :add.uspto whenever the live registry
	// chose USPTO (or was forced to it) for the root.
	go e.autoFetchUSPTOXMLAfterCrawl(root, id, autoFetchCh, autoFetchUnsub)
	sourceMode := e.currentSourceMode()
	e.log(ctx, slog.LevelInfo, "crawl enqueued", slog.String("job_id", string(id)), slog.String("root", root.String()), slog.Int("depth", depth), slog.String("source", string(source)), slog.String("source_mode", sourceMode), slog.Bool("force", force))
	if e.metrics != nil {
		e.metrics.IncCounter("engine.crawl.start_total", 1)
		if source != "" {
			e.metrics.IncCounter("engine.crawl.start.source."+string(source)+"_total", 1)
		}
	}
	e.recordActivity(ctx, observability.Record{
		Action:     observability.ActionCrawlStart,
		Entity:     "job",
		EntityID:   string(id),
		Status:     "queued",
		After:      map[string]any{"job_id": string(id), "root": root.String(), "depth": depth, "force": force, "source": string(source), "source_mode": sourceMode},
		Attributes: map[string]any{"source": string(source), "source_mode": sourceMode, "profile": string(profile)},
	})
	return id, nil
}

func (e *Engine) currentSourceMode() string {
	if e.sourceModes == nil {
		return ""
	}
	return e.sourceModes.SourceMode()
}

// CrawlConfig reports the daemon's default family-crawl depth.
func (e *Engine) CrawlConfig() proto.CrawlConfigResult {
	return proto.CrawlConfigResult{MaxDepth: e.crawlMaxDepth}
}

// SourceMode reports the current normal provider behavior.
func (e *Engine) SourceMode() proto.SourceModeResult {
	if e.sourceModes == nil {
		return proto.SourceModeResult{Mode: ""}
	}
	return proto.SourceModeResult{Mode: e.sourceModes.SourceMode()}
}

// SetSourceMode changes normal provider behavior at runtime and records the
// change in logs, metrics, and Activity History.
func (e *Engine) SetSourceMode(ctx context.Context, mode string) (result proto.SourceModeResult, err error) {
	defer e.observeDuration("engine.source_mode.set", time.Now(), &err)
	if e.sourceModes == nil {
		return proto.SourceModeResult{}, errors.New("engine: source mode controller not configured")
	}
	before := e.sourceModes.SourceMode()
	if err := e.sourceModes.SetSourceMode(mode); err != nil {
		return proto.SourceModeResult{}, err
	}
	after := e.sourceModes.SourceMode()
	e.observeSourceMode(after)
	e.log(ctx, slog.LevelInfo, "source mode changed", slog.String("before", before), slog.String("after", after))
	e.recordActivity(ctx, observability.Record{
		Action:     observability.ActionSourceModeSet,
		Entity:     "config",
		EntityID:   "source_mode",
		Status:     "committed",
		Before:     map[string]any{"mode": before},
		After:      map[string]any{"mode": after},
		Attributes: map[string]any{"mode": after},
	})
	return proto.SourceModeResult{Mode: after}, nil
}

func (e *Engine) observeSourceMode(mode string) {
	if e.metrics == nil {
		return
	}
	e.metrics.IncCounter("engine.source_mode.set_total", 1)
	for _, m := range domain.AllSourceModes() {
		s := string(m)
		value := int64(0)
		if mode == s {
			value = 1
		}
		e.metrics.SetGauge("engine.source_mode."+s, value)
	}
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
		Action: observability.ActionImportFile, Entity: "patent", EntityID: path, Status: "committed",
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
	if q.RelationKind == domain.RelationParent {
		e.attachParentage(ctx, q.Relation, out)
	}
	total, err = e.repo.CountPatents(ctx, q)
	return out, total, err
}

// attachParentage labels each parent row with the claim-parentage type relating
// it to the child patent, and records timing plus counts grouped by relationship
// category (continuation / CIP / division / provisional / national-stage /
// reissue / other). Failures are non-fatal: the listing is still returned, just
// without parentage labels.
func (e *Engine) attachParentage(ctx context.Context, child domain.PatentNumber, rows []domain.PatentRow) {
	start := time.Now()
	var err error
	defer e.observeDuration("engine.relations.parentage", start, &err)

	parentage, err := e.repo.ParentageForChild(ctx, child)
	if err != nil {
		e.log(ctx, slog.LevelWarn, "failed to load claim parentage for parents view",
			slog.String("child", child.String()), slog.String("error", err.Error()))
		return
	}

	counts := map[string]int{}
	for i := range rows {
		p, ok := parentage[rows[i].Number]
		if !ok {
			continue
		}
		rows[i].ParentageCode = p.Code
		rows[i].ParentageLabel = uspto.ParentageLabel(p.Code, p.Description)
		counts[uspto.ParentageCategory(p.Code, p.Description)]++
	}

	for category, n := range counts {
		e.incCounter("engine.parents."+category, int64(n))
	}
	e.setGauge("engine.parents.total", int64(len(rows)))
	e.log(ctx, slog.LevelInfo, "loaded continuity parents",
		slog.String("child", child.String()),
		slog.Int("total", len(rows)),
		slog.Int("continuation", counts[uspto.ParentageContinuation]),
		slog.Int("continuation_in_part", counts[uspto.ParentageContinuationInPart]),
		slog.Int("division", counts[uspto.ParentageDivision]),
		slog.Int("provisional", counts[uspto.ParentageProvisional]),
		slog.Int("national_stage", counts[uspto.ParentageNationalStage]),
		slog.Int("reissue", counts[uspto.ParentageReissue]),
		slog.Int("other", counts[uspto.ParentageOther]),
		slog.Duration("duration", time.Since(start)))
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
func (e *Engine) CancelCrawl(ctx context.Context, id JobID) (err error) {
	defer e.observeDuration("engine.cancel_crawl", time.Now(), &err)
	if !e.pool.cancel(id) {
		return fmt.Errorf("engine: no such job %q", id)
	}
	e.log(ctx, slog.LevelInfo, "crawl cancelled", slog.String("job_id", string(id)))
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionCrawlCancel,
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
