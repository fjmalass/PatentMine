package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

// PatentNote returns one project-scoped patent note for a patent.
func (r *Repo) PatentNote(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (note domain.PatentNote, err error) {
	defer r.observeDuration("patent_note", time.Now(), &err)
	row := r.reader.QueryRowContext(ctx, `
		SELECT project_id, patent_number, markdown, added_at, updated_at
		FROM project_patent_note
		WHERE project_id = ? AND patent_number = ?`,
		string(project), patent.Normalized())
	note, err = scanPatentNote(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PatentNote{}, store.ErrNotFound
	}
	if err != nil {
		return domain.PatentNote{}, fmt.Errorf("store/sqlite: get patent note %s/%s: %w", project, patent, err)
	}
	return note, nil
}

func scanPatentNote(s rowScanner) (domain.PatentNote, error) {
	var (
		note               domain.PatentNote
		project, patent    string
		addedAt, updatedAt string
	)
	if err := s.Scan(&project, &patent, &note.Markdown, &addedAt, &updatedAt); err != nil {
		return domain.PatentNote{}, err
	}
	parsedPatent, err := domain.ParsePatentNumber(patent)
	if err != nil {
		return domain.PatentNote{}, fmt.Errorf("decode patent note patent %q: %w", patent, err)
	}
	note.Project = domain.ProjectID(project)
	note.Patent = parsedPatent
	note.AddedAt, err = decodeTime(addedAt)
	if err != nil {
		return domain.PatentNote{}, fmt.Errorf("decode patent note added_at: %w", err)
	}
	note.UpdatedAt, err = decodeTime(updatedAt)
	if err != nil {
		return domain.PatentNote{}, fmt.Errorf("decode patent note updated_at: %w", err)
	}
	return note, nil
}

// SavePatentNote inserts or updates one project-scoped patent note.
func (r *Repo) SavePatentNote(ctx context.Context, note domain.PatentNote) (saved domain.PatentNote, err error) {
	defer r.observeDuration("save_patent_note", time.Now(), &err)
	if note.Project == "" {
		return domain.PatentNote{}, errors.New("store/sqlite: patent note project must be set")
	}
	if note.Patent.IsZero() {
		return domain.PatentNote{}, errors.New("store/sqlite: patent note patent must be set")
	}
	note.Markdown = strings.TrimSpace(note.Markdown)
	if note.Markdown == "" {
		return domain.PatentNote{}, errors.New("store/sqlite: patent note markdown must not be empty")
	}
	now := time.Now().UTC()
	addedAt := note.AddedAt
	if addedAt.IsZero() {
		addedAt = now
	}
	updatedAt := note.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}
	_, err = r.writer.ExecContext(ctx, `
		INSERT INTO project_patent_note (project_id, patent_number, markdown, added_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(project_id, patent_number) DO UPDATE SET
			markdown=excluded.markdown,
			updated_at=excluded.updated_at`,
		string(note.Project), note.Patent.Normalized(), note.Markdown, encodeTime(addedAt), encodeTime(updatedAt))
	if err != nil {
		return domain.PatentNote{}, fmt.Errorf("store/sqlite: save patent note %s/%s: %w", note.Project, note.Patent, err)
	}
	return r.PatentNote(ctx, note.Project, note.Patent)
}

// DeletePatentNote removes one project-scoped patent note.
func (r *Repo) DeletePatentNote(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (err error) {
	defer r.observeDuration("delete_patent_note", time.Now(), &err)
	_, err = r.writer.ExecContext(ctx,
		`DELETE FROM project_patent_note WHERE project_id = ? AND patent_number = ?`,
		string(project), patent.Normalized())
	if err != nil {
		return fmt.Errorf("store/sqlite: delete patent note %s/%s: %w", project, patent, err)
	}
	return nil
}
