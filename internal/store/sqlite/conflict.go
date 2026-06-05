package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

const conflictColumns = `id, project_id, record_number, against_number, document_id,
	reason, status, flagged_by, flagged_at, resolved_at`

// SaveConflict inserts or updates one project-scoped conflict edge by its id.
func (r *Repo) SaveConflict(ctx context.Context, c domain.Conflict) (err error) {
	defer r.observeDuration("save_conflict", time.Now(), &err)
	if c.ID == "" {
		return errors.New("store/sqlite: cannot save conflict with empty id")
	}
	if c.Project == "" {
		return errors.New("store/sqlite: conflict requires a project id")
	}
	if c.Status != "" && !c.Status.Valid() {
		return fmt.Errorf("store/sqlite: invalid conflict status %q", c.Status)
	}
	_, err = r.writer.ExecContext(ctx,
		`INSERT INTO conflict
		 (id, project_id, record_number, against_number, document_id, reason, status, flagged_by, flagged_at, resolved_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			project_id=excluded.project_id,
			record_number=excluded.record_number,
			against_number=excluded.against_number,
			document_id=excluded.document_id,
			reason=excluded.reason,
			status=excluded.status,
			flagged_by=excluded.flagged_by,
			flagged_at=excluded.flagged_at,
			resolved_at=excluded.resolved_at`,
		c.ID, string(c.Project), c.Record.Normalized(), c.Against.Normalized(), c.DocumentID,
		c.Reason, string(c.Status), c.FlaggedBy, encodeTime(c.FlaggedAt), encodeTime(c.ResolvedAt))
	if err != nil {
		return fmt.Errorf("store/sqlite: save conflict %s: %w", c.ID, err)
	}
	return nil
}

// Conflict returns one conflict by id, or store.ErrNotFound.
func (r *Repo) Conflict(ctx context.Context, id string) (c domain.Conflict, err error) {
	defer r.observeDuration("conflict", time.Now(), &err)
	row := r.reader.QueryRowContext(ctx, `SELECT `+conflictColumns+` FROM conflict WHERE id = ?`, id)
	c, err = scanConflict(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Conflict{}, store.ErrNotFound
	}
	if err != nil {
		return domain.Conflict{}, fmt.Errorf("store/sqlite: get conflict %s: %w", id, err)
	}
	return c, nil
}

// ListConflicts returns a project's conflicts, newest first.
func (r *Repo) ListConflicts(ctx context.Context, project domain.ProjectID) (out []domain.Conflict, err error) {
	defer r.observeDuration("list_conflicts", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT `+conflictColumns+` FROM conflict WHERE project_id = ? ORDER BY flagged_at DESC`, string(project))
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list conflicts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		c, err := scanConflict(rows)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: scan conflict: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteConflict removes one conflict edge.
func (r *Repo) DeleteConflict(ctx context.Context, id string) (err error) {
	defer r.observeDuration("delete_conflict", time.Now(), &err)
	if _, err = r.writer.ExecContext(ctx, `DELETE FROM conflict WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store/sqlite: delete conflict %s: %w", id, err)
	}
	return nil
}

func scanConflict(s rowScanner) (domain.Conflict, error) {
	var (
		c          domain.Conflict
		id         string
		project    string
		record     string
		against    string
		status     string
		flaggedAt  string
		resolvedAt string
	)
	if err := s.Scan(&id, &project, &record, &against, &c.DocumentID,
		&c.Reason, &status, &c.FlaggedBy, &flaggedAt, &resolvedAt); err != nil {
		return domain.Conflict{}, err
	}
	c.ID = id
	c.Project = domain.ProjectID(project)
	// record/against are stored normalized; parse back, tolerating empties.
	c.Record, _ = domain.ParsePatentNumber(record)
	c.Against, _ = domain.ParsePatentNumber(against)
	c.Status = domain.ConflictStatus(status)
	if at, err := decodeTime(flaggedAt); err == nil {
		c.FlaggedAt = at
	}
	if at, err := decodeTime(resolvedAt); err == nil {
		c.ResolvedAt = at
	}
	return c, nil
}
