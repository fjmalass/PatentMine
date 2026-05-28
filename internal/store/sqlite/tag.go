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

// CreateTag returns the project's tag of the given name, creating it when the
// project does not have it yet. The (project_id, name) uniqueness constraint
// makes the insert idempotent: a second call with the same name returns the
// existing row unchanged.
func (r *Repo) CreateTag(ctx context.Context, project domain.ProjectID, name string) (tag domain.Tag, err error) {
	defer r.observeDuration("create_tag", time.Now(), &err)
	if project == "" {
		return domain.Tag{}, errors.New("store/sqlite: tag needs a project")
	}
	if name == "" {
		return domain.Tag{}, errors.New("store/sqlite: tag name must not be empty")
	}
	_, err = r.writer.ExecContext(ctx,
		`INSERT INTO tag (project_id, name, created_at) VALUES (?,?,?)
		 ON CONFLICT(project_id, name) DO NOTHING`,
		string(project), name, encodeTime(time.Now()))
	if err != nil {
		return domain.Tag{}, fmt.Errorf("store/sqlite: create tag: %w", err)
	}
	row := r.reader.QueryRowContext(ctx,
		`SELECT id, project_id, name, created_at FROM tag
		 WHERE project_id = ? AND name = ?`, string(project), name)
	t, err := scanTag(row)
	if err != nil {
		return domain.Tag{}, fmt.Errorf("store/sqlite: load tag: %w", err)
	}
	return t, nil
}

// DeleteTag removes a tag from the project's taxonomy.
func (r *Repo) DeleteTag(ctx context.Context, project domain.ProjectID, name string) (err error) {
	defer r.observeDuration("delete_tag", time.Now(), &err)
	if project == "" {
		return errors.New("store/sqlite: tag needs a project")
	}
	if name == "" {
		return errors.New("store/sqlite: tag name must not be empty")
	}
	_, err = r.writer.ExecContext(ctx,
		`DELETE FROM tag WHERE project_id = ? AND name = ?`,
		string(project), name)
	if err != nil {
		return fmt.Errorf("store/sqlite: delete tag: %w", err)
	}
	return nil
}

// ProjectTags returns every tag of a project, ordered by name.
func (r *Repo) ProjectTags(ctx context.Context, project domain.ProjectID) (out []domain.Tag, err error) {
	defer r.observeDuration("project_tags", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT id, project_id, name, created_at FROM tag
		 WHERE project_id = ? ORDER BY name`, string(project))
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list project tags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out = nil
	for rows.Next() {
		t, err := scanTag(rows)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: scan tag: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: list project tags: %w", err)
	}
	return out, nil
}

// TagByName returns one tag in a project without loading the whole taxonomy.
func (r *Repo) TagByName(ctx context.Context, project domain.ProjectID, name string) (tag domain.Tag, err error) {
	defer r.observeDuration("tag_by_name", time.Now(), &err)
	row := r.reader.QueryRowContext(ctx,
		`SELECT id, project_id, name, created_at FROM tag
		 WHERE project_id = ? AND lower(name) = lower(?)`, string(project), name)
	t, err := scanTag(row)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.Tag{}, store.ErrNotFound
		}
		return domain.Tag{}, fmt.Errorf("store/sqlite: get tag by name: %w", err)
	}
	return t, nil
}

// TagPatents assigns a tag to multiple patents in a single transaction.
func (r *Repo) TagPatents(ctx context.Context, tagID int64, patents []domain.PatentNumber, assignedAt time.Time) (err error) {
	defer r.observeDuration("tag_patents", time.Now(), &err)
	if assignedAt.IsZero() {
		assignedAt = time.Now()
	}
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: tag patents tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO patent_tag (tag_id, record_id, created_at) VALUES (?, (SELECT record_id FROM patent WHERE number = ?), ?) ON CONFLICT(tag_id, record_id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("store/sqlite: tag patents prepare: %w", err)
	}
	defer stmt.Close()

	encodedTimeStr := encodeTime(assignedAt)
	for _, patent := range patents {
		if patent.IsZero() {
			continue
		}
		_, err := stmt.ExecContext(ctx, tagID, patent.Normalized(), encodedTimeStr)
		if err != nil {
			return fmt.Errorf("store/sqlite: tag patent exec: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: tag patents commit: %w", err)
	}
	return nil
}

// UntagPatents removes a tag from multiple patents in a single transaction.
func (r *Repo) UntagPatents(ctx context.Context, tagID int64, patents []domain.PatentNumber) (err error) {
	defer r.observeDuration("untag_patents", time.Now(), &err)
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: untag patents tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `DELETE FROM patent_tag WHERE tag_id = ? AND record_id = (SELECT record_id FROM patent WHERE number = ?)`)
	if err != nil {
		return fmt.Errorf("store/sqlite: untag patents prepare: %w", err)
	}
	defer stmt.Close()

	for _, patent := range patents {
		_, err := stmt.ExecContext(ctx, tagID, patent.Normalized())
		if err != nil {
			return fmt.Errorf("store/sqlite: untag patent exec: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: untag patents commit: %w", err)
	}
	return nil
}

// PatentTag returns a single assigned tag without loading every tag on the patent.
func (r *Repo) PatentTag(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, name string) (tag domain.Tag, err error) {
	defer r.observeDuration("patent_tag", time.Now(), &err)
	row := r.reader.QueryRowContext(ctx,
		`SELECT t.id, t.project_id, t.name, t.created_at, pt.created_at
		 FROM tag t
		 JOIN patent_tag pt ON pt.tag_id = t.id
		 JOIN patent p ON p.record_id = pt.record_id
		 WHERE t.project_id = ? AND p.number = ? AND lower(t.name) = lower(?)`,
		string(project), patent.Normalized(), name)
	t, err := scanAssignedTag(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Tag{}, store.ErrNotFound
		}
		return domain.Tag{}, fmt.Errorf("store/sqlite: get patent tag: %w", err)
	}
	return t, nil
}

// PatentTags returns the tags a patent carries within a project, ordered by
// name. The project filter keeps a patent shared across projects from leaking
// one project's tags into another.
func (r *Repo) PatentTags(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (out []domain.Tag, err error) {
	defer r.observeDuration("patent_tags", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT t.id, t.project_id, t.name, t.created_at, pt.created_at
		 FROM tag t
		 JOIN patent_tag pt ON pt.tag_id = t.id
		 JOIN patent p ON p.record_id = pt.record_id
		 WHERE t.project_id = ? AND p.number = ?
		 ORDER BY t.name`,
		string(project), patent.Normalized())
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list patent tags: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out = nil
	for rows.Next() {
		t, err := scanAssignedTag(rows)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: scan patent tag: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: list patent tags: %w", err)
	}
	return out, nil
}

func scanAssignedTag(s rowScanner) (domain.Tag, error) {
	var (
		t          domain.Tag
		projectID  string
		createdAt  string
		assignedAt string
	)
	if err := s.Scan(&t.ID, &projectID, &t.Name, &createdAt, &assignedAt); err != nil {
		return domain.Tag{}, err
	}
	t.Project = domain.ProjectID(projectID)
	created, err := decodeTime(createdAt)
	if err != nil {
		return domain.Tag{}, err
	}
	t.CreatedAt = created
	assigned, err := decodeTime(assignedAt)
	if err != nil {
		return domain.Tag{}, err
	}
	t.AssignedAt = assigned
	return t, nil
}

// scanTag reads one tag row in the column order used by every tag query.
func scanTag(s rowScanner) (domain.Tag, error) {
	var (
		t         domain.Tag
		projectID string
		createdAt string
	)
	if err := s.Scan(&t.ID, &projectID, &t.Name, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Tag{}, store.ErrNotFound
		}
		return domain.Tag{}, err
	}
	t.Project = domain.ProjectID(projectID)
	created, err := decodeTime(createdAt)
	if err != nil {
		return domain.Tag{}, err
	}
	t.CreatedAt = created
	return t, nil
}
