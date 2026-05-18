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

// TagPatent assigns a tag to a patent. An existing assignment is left as is.
func (r *Repo) TagPatent(ctx context.Context, tagID int64, patent domain.PatentNumber) (err error) {
	defer r.observeDuration("tag_patent", time.Now(), &err)
	if patent.IsZero() {
		return errors.New("store/sqlite: tag assignment needs a patent")
	}
	_, err = r.writer.ExecContext(ctx,
		`INSERT INTO patent_tag (tag_id, patent_number) VALUES (?,?)
		 ON CONFLICT(tag_id, patent_number) DO NOTHING`,
		tagID, patent.Normalized())
	if err != nil {
		return fmt.Errorf("store/sqlite: tag patent: %w", err)
	}
	return nil
}

// UntagPatent removes a tag from a patent. A missing assignment is a no-op.
func (r *Repo) UntagPatent(ctx context.Context, tagID int64, patent domain.PatentNumber) (err error) {
	defer r.observeDuration("untag_patent", time.Now(), &err)
	_, err = r.writer.ExecContext(ctx,
		`DELETE FROM patent_tag WHERE tag_id = ? AND patent_number = ?`,
		tagID, patent.Normalized())
	if err != nil {
		return fmt.Errorf("store/sqlite: untag patent: %w", err)
	}
	return nil
}

// PatentTags returns the tags a patent carries within a project, ordered by
// name. The project filter keeps a patent shared across projects from leaking
// one project's tags into another.
func (r *Repo) PatentTags(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (out []domain.Tag, err error) {
	defer r.observeDuration("patent_tags", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT t.id, t.project_id, t.name, t.created_at
		 FROM tag t
		 JOIN patent_tag pt ON pt.tag_id = t.id
		 WHERE t.project_id = ? AND pt.patent_number = ?
		 ORDER BY t.name`,
		string(project), patent.Normalized())
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list patent tags: %w", err)
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
		return nil, fmt.Errorf("store/sqlite: list patent tags: %w", err)
	}
	return out, nil
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
