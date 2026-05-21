package engine

import (
	"context"
	"errors"
	"log/slog"
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

// PatentSnapshot is a complete backup of a patent and its family-graph relation
// edges, recorded prior to a hard purge (permanent delete) so it can be replayed.
type PatentSnapshot struct {
	Patent    domain.Patent     `json:"patent"`
	Relations []domain.Relation `json:"relations"`
}

// DeletePatent permanently removes a patent and all associated data.
func (e *Engine) DeletePatent(ctx context.Context, n domain.PatentNumber) (err error) {
	defer e.observeDuration("engine.delete_patent", time.Now(), &err)

	record, err := e.recordNumber(ctx, n)
	if err != nil {
		return err
	}

	// 1. Fetch the patent and its associated documents.
	patent, err := e.repo.Patent(ctx, record)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// If not found in repo, let's execute the repository deletion directly to ensure proper errors/logging behavior.
			return e.repo.DeletePatent(ctx, record)
		}
		return err
	}

	// 2. Query all relations involving the patent.
	relations, err := e.repo.AllRelations(ctx, record)
	if err != nil {
		return err
	}

	// 3. Compile the snapshot.
	snapshot := PatentSnapshot{
		Patent:    patent,
		Relations: relations,
	}

	// 4. Delete the patent from repository.
	if err := e.repo.DeletePatent(ctx, record); err != nil {
		e.log(ctx, slog.LevelError, "delete patent failed", slog.String("number", record.String()), slog.String("error", err.Error()))
		return err
	}

	e.log(ctx, slog.LevelInfo, "patent deleted", slog.String("number", record.String()))

	// 5. Record activity carrying the snapshot inside Before field.
	e.recordActivity(ctx, observability.Record{
		Action:   "patent.delete",
		Entity:   "patent",
		EntityID: record.String(),
		Status:   "committed",
		Before:   snapshot,
	})

	e.announceChange()
	return nil
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
		Action:   "patent.restore",
		Entity:   "patent",
		EntityID: snapshot.Patent.Number.String(),
		Status:   "committed",
		After:    snapshot.Patent,
		Metadata: map[string]any{"soft": soft},
	})

	e.announceChange()
	return nil
}
