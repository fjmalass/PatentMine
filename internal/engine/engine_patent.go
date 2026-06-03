package engine

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/store"
)

// Patent returns one patent.
func (e *Engine) Patent(ctx context.Context, n domain.PatentNumber) (patent domain.Patent, err error) {
	defer e.observeDuration("engine.patent", time.Now(), &err)
	record, err := e.recordNumber(ctx, n)
	if err != nil {
		return domain.Patent{}, err
	}
	return e.repo.Patent(ctx, record)
}

// PatentInventorStats fetches the patent and aggregates metrics for all its inventors.
func (e *Engine) PatentInventorStats(ctx context.Context, number domain.PatentNumber, project domain.ProjectID) (stats []domain.InventorStats, err error) {
	defer e.observeDuration("engine.patent_inventor_stats", time.Now(), &err)
	p, err := e.Patent(ctx, number)
	if err != nil {
		return nil, err
	}
	return e.repo.PatentInventorStats(ctx, project, p.Inventors)
}

// AllAssigneesHistory aggregates metrics for all assignees in the active dataset.
func (e *Engine) AllAssigneesHistory(ctx context.Context, project domain.ProjectID) (stats []domain.AllAssigneesHistory, err error) {
	defer e.observeDuration("engine.all_assignees_history", time.Now(), &err)
	return e.repo.AllAssigneesHistory(ctx, project)
}

// PatentClassificationStats aggregates metrics for all classification codes in the active dataset.
func (e *Engine) PatentClassificationStats(ctx context.Context, project domain.ProjectID) (stats []domain.ClassificationStats, err error) {
	defer e.observeDuration("engine.patent_classification_stats", time.Now(), &err)
	return e.repo.PatentClassificationStats(ctx, project)
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

// OrphanPatents returns one page of patents that have no membership in any
// project, plus the total orphan count. This is the "stuff in the database
// nobody owns" view — useful for cleanup and for spotting patents discovered
// as citation stubs but never assigned to a project.
func (e *Engine) OrphanPatents(ctx context.Context, limit, offset int) (rows []domain.PatentRow, total int, err error) {
	defer e.observeDuration("engine.orphan_patents", time.Now(), &err)
	rows, total, err = e.repo.ListOrphanPatents(ctx, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	if e.metrics != nil {
		e.metrics.SetGauge("engine.orphan_patents.total", int64(total))
	}
	return rows, total, nil
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
		Action:   observability.ActionPatentSave,
		Entity:   "patent",
		EntityID: p.Number.String(),
		Status:   "committed",
		Before:   before,
		After:    p,
	})
	e.announceChange()
	return nil
}

// PatentSnapshot is a complete backup of a patent and its family-graph relation
// edges, recorded prior to a hard purge (permanent delete) so it can be replayed.
type PatentSnapshot struct {
	Patent    domain.Patent     `json:"patent"`
	Relations []domain.Relation `json:"relations"`
}

// DeletePatent soft-deletes a patent: documents, memberships, notes, tags,
// and outbound family-graph edges are removed; the row is demoted to
// FetchStub so inbound edges from other patents still resolve. When no
// inbound edge remains, the row is hard-purged inside the same transaction.
// The full pre-delete state is captured as a PatentSnapshot in the activity
// log so RestorePatent can replay it.
func (e *Engine) DeletePatent(ctx context.Context, n domain.PatentNumber) (err error) {
	return e.DeletePatents(ctx, []domain.PatentNumber{n})
}

// DeletePatents is the batch variant of DeletePatent. Inbound-edge accounting
// is computed across the whole batch so a cycle of mutually-referencing
// patents deleted together is fully purged rather than leaving orphan stubs.
func (e *Engine) DeletePatents(ctx context.Context, patents []domain.PatentNumber) (err error) {
	defer e.observeDuration("engine.soft_delete_patent", time.Now(), &err)
	records := uniquePatentNumbers(patents)
	if len(records) == 0 {
		return nil
	}

	snapshots := make([]PatentSnapshot, 0, len(records))
	inboundCounts := make([]int, 0, len(records))
	targetSet := make(map[string]struct{}, len(records))
	for _, patent := range records {
		targetSet[patent.Normalized()] = struct{}{}
	}
	for _, patent := range records {
		record, resolveErr := e.recordNumber(ctx, patent)
		if resolveErr != nil {
			return resolveErr
		}

		storedPatent, patentErr := e.repo.Patent(ctx, record)
		if patentErr != nil {
			if errors.Is(patentErr, store.ErrNotFound) {
				return store.ErrNotFound
			}
			return patentErr
		}

		relations, relErr := e.repo.AllRelations(ctx, record)
		if relErr != nil {
			return relErr
		}

		inbound := 0
		recordKey := record.Normalized()
		for _, rel := range relations {
			if rel.To.Normalized() != recordKey {
				continue
			}
			if _, inBatch := targetSet[rel.From.Normalized()]; inBatch {
				continue
			}
			inbound++
		}

		records[len(snapshots)] = record
		snapshots = append(snapshots, PatentSnapshot{Patent: storedPatent, Relations: relations})
		inboundCounts = append(inboundCounts, inbound)
	}

	if len(records) == 1 {
		if err := e.repo.SoftDeletePatent(ctx, records[0]); err != nil {
			e.log(ctx, slog.LevelError, "soft delete patent failed", slog.String("number", records[0].String()), slog.String("error", err.Error()))
			return err
		}
	} else {
		if err := e.repo.SoftDeletePatents(ctx, records); err != nil {
			e.log(ctx, slog.LevelError, "soft delete patents batch failed", slog.Int("count", len(records)), slog.String("error", err.Error()))
			return err
		}
	}

	batchSize := len(records)
	batchID := ""
	if batchSize > 1 {
		batchID = strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	}
	for i, record := range records {
		demoted := inboundCounts[i] > 0
		e.log(ctx, slog.LevelInfo, "patent soft-deleted",
			slog.String("number", record.String()),
			slog.Int("inbound_count", inboundCounts[i]),
			slog.Bool("demoted_to_stub", demoted),
		)
		attrs := map[string]any{
			"batch_size":      batchSize,
			"mode":            "soft",
			"demoted_to_stub": demoted,
			"inbound_count":   inboundCounts[i],
		}
		if batchID != "" {
			attrs["batch_id"] = batchID
		}
		e.recordActivity(ctx, observability.Record{
			Action:   observability.ActionPatentSoftDelete,
			Entity:   "patent",
			EntityID: record.String(),
			Status:   "committed",
			Before:   snapshots[i],
			Attributes: attrs,
		})
	}

	e.announceChange()
	return nil
}

func uniquePatentNumbers(numbers []domain.PatentNumber) []domain.PatentNumber {
	seen := make(map[string]struct{}, len(numbers))
	unique := make([]domain.PatentNumber, 0, len(numbers))
	for _, number := range numbers {
		if number.IsZero() {
			continue
		}
		key := number.Normalized()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, number)
	}
	return unique
}

// RestorePatent restores a patent from a backup snapshot.
// If soft is true, it restores the patent as a FetchStub with a single guess-stage document,
// but restores all relation edges to keep the topology intact.
// If soft is false, it performs a full hard restore of the patent, all documents, and all relation edges.
func (e *Engine) RestorePatent(ctx context.Context, snapshot PatentSnapshot, soft bool) (err error) {
	defer e.observeDuration("engine.restore_patent", time.Now(), &err)

	if soft {
		stub := domain.Patent{
			Number:        snapshot.Patent.Number,
			DisplayNumber: snapshot.Patent.Number,
			FetchState:    domain.FetchStub,
		}
		if err := e.repo.SavePatent(ctx, stub); err != nil {
			e.log(ctx, slog.LevelError, "restore soft patent failed", slog.String("number", snapshot.Patent.Number.String()), slog.String("error", err.Error()))
			return err
		}
		stubDoc := domain.Document{
			Number: snapshot.Patent.Number,
			Stage:  domain.GuessStage(snapshot.Patent.Number),
		}
		if err := e.repo.SaveDocument(ctx, snapshot.Patent.Number, stubDoc); err != nil {
			e.log(ctx, slog.LevelError, "restore soft document failed", slog.String("number", snapshot.Patent.Number.String()), slog.String("error", err.Error()))
			return err
		}
	} else {
		if err := e.repo.SavePatent(ctx, snapshot.Patent); err != nil {
			e.log(ctx, slog.LevelError, "restore hard patent failed", slog.String("number", snapshot.Patent.Number.String()), slog.String("error", err.Error()))
			return err
		}
		for _, doc := range snapshot.Patent.Documents {
			if err := e.repo.SaveDocument(ctx, snapshot.Patent.Number, doc); err != nil {
				e.log(ctx, slog.LevelError, "restore hard document failed", slog.String("number", doc.Number.String()), slog.String("error", err.Error()))
				return err
			}
		}
	}

	for _, rel := range snapshot.Relations {
		if err := e.repo.SaveRelation(ctx, rel); err != nil {
			e.log(ctx, slog.LevelError, "restore relation failed", slog.String("from", rel.From.String()), slog.String("to", rel.To.String()), slog.String("error", err.Error()))
			return err
		}
	}

	e.log(ctx, slog.LevelInfo, "patent restored", slog.String("number", snapshot.Patent.Number.String()), slog.Bool("soft", soft))

	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionPatentRestore,
		Entity:   "patent",
		EntityID: snapshot.Patent.Number.String(),
		Status:   "committed",
		After:    snapshot.Patent,
		Attributes: map[string]any{"soft": soft},
	})

	e.announceChange()
	return nil
}
