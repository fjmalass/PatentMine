package engine

import (
	"context"
	"errors"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/store"
)

// PatentNoteOf returns the project-scoped patent note for a project/patent pair.
func (e *Engine) PatentNoteOf(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (note domain.PatentNote, ok bool, err error) {
	defer e.observeDuration("engine.patent_note_of", time.Now(), &err)
	if project == "" {
		return domain.PatentNote{}, false, nil
	}
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return domain.PatentNote{}, false, err
	}
	note, err = e.repo.PatentNote(ctx, project, record)
	if errors.Is(err, store.ErrNotFound) {
		return domain.PatentNote{}, false, nil
	}
	if err != nil {
		return domain.PatentNote{}, false, err
	}
	return note, true, nil
}

// SavePatentNote inserts or updates one project-scoped patent note.
func (e *Engine) SavePatentNote(ctx context.Context, note domain.PatentNote) (saved domain.PatentNote, err error) {
	defer e.observeDuration("engine.save_patent_note", time.Now(), &err)
	if note.Project == "" {
		return domain.PatentNote{}, errors.New("engine: patent note needs a project")
	}
	record, err := e.recordNumber(ctx, note.Patent)
	if err != nil {
		return domain.PatentNote{}, err
	}
	note.Patent = record

	var before *domain.PatentNote
	if prev, perr := e.repo.PatentNote(ctx, note.Project, record); perr == nil {
		cp := prev
		before = &cp
	}

	saved, err = e.repo.SavePatentNote(ctx, note)
	if err != nil {
		return domain.PatentNote{}, err
	}
	e.announceChange()
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionNotesSave,
		Entity:   "patent_note",
		EntityID: string(saved.Project) + "/" + saved.Patent.String(),
		Status:   "committed",
		Before:   before,
		After:    saved,
	})
	return saved, nil
}

// ListPatentNotes returns all notes for a project, sorted by date or patent number.
func (e *Engine) ListPatentNotes(ctx context.Context, project domain.ProjectID, sortByDate bool) (notes []domain.PatentNote, err error) {
	defer e.observeDuration("engine.list_patent_notes", time.Now(), &err)
	if project == "" {
		return nil, nil
	}
	return e.repo.ListPatentNotes(ctx, project, sortByDate)
}

// DeletePatentNote removes one project-scoped patent note.
func (e *Engine) DeletePatentNote(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (err error) {
	defer e.observeDuration("engine.delete_patent_note", time.Now(), &err)
	record, err := e.recordNumber(ctx, patent)
	if err != nil {
		return err
	}

	var before *domain.PatentNote
	if prev, perr := e.repo.PatentNote(ctx, project, record); perr == nil {
		cp := prev
		before = &cp
	}

	if err := e.repo.DeletePatentNote(ctx, project, record); err != nil {
		return err
	}
	e.announceChange()
	e.recordActivity(ctx, observability.Record{
		Action:   observability.ActionNotesDelete,
		Entity:   "patent_note",
		EntityID: string(project) + "/" + record.String(),
		Status:   "committed",
		Before:   before,
	})
	return nil
}
