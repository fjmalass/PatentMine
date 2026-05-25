package engine

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/store"
)

// IDSEntryOf returns the curated IDS entry for a project/patent pair.
func (e *Engine) IDSEntryOf(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (entry domain.IDSEntry, ok bool, err error) {
	defer e.observeDuration("engine.ids_entry_of", time.Now(), &err)
	if project == "" {
		return domain.IDSEntry{}, false, nil
	}
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return domain.IDSEntry{}, false, err
	}
	entry, err = e.repo.IDSEntry(ctx, project, record)
	if errors.Is(err, store.ErrNotFound) {
		return domain.IDSEntry{}, false, nil
	}
	if err != nil {
		return domain.IDSEntry{}, false, err
	}
	return entry, true, nil
}

// SaveIDSEntry inserts or updates one curated IDS entry. The prior entry (when
// any) is captured as the Before snapshot on the activity record, so a future
// replay/undo can restore it.
func (e *Engine) SaveIDSEntry(ctx context.Context, entry domain.IDSEntry) (saved domain.IDSEntry, err error) {
	defer e.observeDuration("engine.save_ids_entry", time.Now(), &err)
	if entry.Project == "" {
		return domain.IDSEntry{}, errors.New("engine: ids entry needs a project")
	}
	record, err := e.recordNumber(ctx, entry.Patent)
	if err != nil {
		return domain.IDSEntry{}, err
	}
	entry.Patent = record
	if !entry.Status.Valid() {
		entry.Status = domain.IDSEntryPending
	}
	if strings.TrimSpace(entry.KindCode) == "" && record.Kind != "" {
		entry.KindCode = record.Kind
	}
	if entry.Status == domain.IDSEntrySubmitted && entry.SubmittedAt.IsZero() {
		entry.SubmittedAt = time.Now().UTC()
	}

	var (
		before    *domain.IDSEntry
		priorStat domain.IDSEntryStatus
	)
	if prev, perr := e.repo.IDSEntry(ctx, entry.Project, record); perr == nil {
		cp := prev
		before = &cp
		priorStat = prev.Status
	} else if !errors.Is(perr, store.ErrNotFound) {
		e.log(ctx, slog.LevelWarn, "ids entry prior lookup failed",
			slog.String("project_id", string(entry.Project)),
			slog.String("patent", record.String()),
			slog.String("error", perr.Error()))
	}

	saved, err = e.repo.SaveIDSEntry(ctx, entry)
	if err != nil {
		e.log(ctx, slog.LevelError, "save ids entry failed",
			slog.String("project_id", string(entry.Project)),
			slog.String("patent", record.String()),
			slog.String("error", err.Error()))
		e.incCounter("engine.ids_entry.save.error_total", 1)
		return domain.IDSEntry{}, err
	}

	e.incCounter("engine.ids_entry.save.committed_total", 1)
	if priorStat != saved.Status {
		e.incCounter("engine.ids_entry.status_transition_total", 1)
	}
	e.log(ctx, slog.LevelInfo, "ids entry saved",
		slog.String("project_id", string(saved.Project)),
		slog.String("patent", saved.Patent.String()),
		slog.String("status", string(saved.Status)),
		slog.String("prior_status", string(priorStat)),
		slog.Bool("in_full", saved.InFull),
		slog.Time("added_at", saved.AddedAt),
		slog.Time("submitted_at", saved.SubmittedAt))
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionIDSEntrySave,
		Entity:   "ids_entry",
		EntityID: string(saved.Project) + "/" + saved.Patent.String(),
		Status:   "committed",
		Before:   before,
		After:    saved,
		Metadata: map[string]any{
			"prior_status": string(priorStat),
			"status":       string(saved.Status),
		},
	})
	e.announceChange()
	return saved, nil
}

// DeleteIDSEntry removes one curated IDS entry. The deleted snapshot is kept
// on the activity record so undo / replay can recreate it.
func (e *Engine) DeleteIDSEntry(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (err error) {
	defer e.observeDuration("engine.delete_ids_entry", time.Now(), &err)
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return err
	}

	var before *domain.IDSEntry
	if prev, perr := e.repo.IDSEntry(ctx, project, record); perr == nil {
		cp := prev
		before = &cp
	} else if !errors.Is(perr, store.ErrNotFound) {
		e.log(ctx, slog.LevelWarn, "ids entry prior lookup failed",
			slog.String("project_id", string(project)),
			slog.String("patent", record.String()),
			slog.String("error", perr.Error()))
	}

	if err := e.repo.DeleteIDSEntry(ctx, project, record); err != nil {
		e.log(ctx, slog.LevelError, "delete ids entry failed",
			slog.String("project_id", string(project)),
			slog.String("patent", record.String()),
			slog.String("error", err.Error()))
		e.incCounter("engine.ids_entry.delete.error_total", 1)
		return err
	}
	e.incCounter("engine.ids_entry.delete.committed_total", 1)
	e.log(ctx, slog.LevelInfo, "ids entry deleted",
		slog.String("project_id", string(project)),
		slog.String("patent", record.String()))
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionIDSEntryDelete,
		Entity:   "ids_entry",
		EntityID: string(project) + "/" + record.String(),
		Status:   "committed",
		Before:   before,
	})
	e.announceChange()
	return nil
}
