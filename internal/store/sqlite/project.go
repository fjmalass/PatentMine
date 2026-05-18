package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

// SaveProject inserts or updates a project by its id.
func (r *Repo) SaveProject(ctx context.Context, p domain.Project) error {
	if p.ID == "" {
		return errors.New("store/sqlite: cannot save project with empty id")
	}
	_, err := r.writer.ExecContext(ctx,
		`INSERT INTO project (id, name, created_at) VALUES (?,?,?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, created_at=excluded.created_at`,
		string(p.ID), p.Name, encodeTime(p.CreatedAt))
	if err != nil {
		return fmt.Errorf("store/sqlite: save project %s: %w", p.ID, err)
	}
	return nil
}

// Project returns one project, or store.ErrNotFound.
func (r *Repo) Project(ctx context.Context, id domain.ProjectID) (domain.Project, error) {
	row := r.reader.QueryRowContext(ctx,
		`SELECT id, name, created_at FROM project WHERE id = ?`, string(id))
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Project{}, store.ErrNotFound
	}
	if err != nil {
		return domain.Project{}, fmt.Errorf("store/sqlite: get project %s: %w", id, err)
	}
	return p, nil
}

// ListProjects returns every project, ordered by name.
func (r *Repo) ListProjects(ctx context.Context) ([]domain.Project, error) {
	rows, err := r.reader.QueryContext(ctx,
		`SELECT id, name, created_at FROM project ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: scan project: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: list projects: %w", err)
	}
	return out, nil
}

func scanProject(s rowScanner) (domain.Project, error) {
	var (
		p         domain.Project
		id        string
		createdAt string
	)
	if err := s.Scan(&id, &p.Name, &createdAt); err != nil {
		return domain.Project{}, err
	}
	p.ID = domain.ProjectID(id)
	t, err := decodeTime(createdAt)
	if err != nil {
		return domain.Project{}, err
	}
	p.CreatedAt = t
	return p, nil
}

// AddMembership links a patent to a project. An existing link is left as is;
// state changes go through SetMembershipState.
func (r *Repo) AddMembership(ctx context.Context, m domain.Membership) error {
	if m.Project == "" || m.Patent.IsZero() {
		return errors.New("store/sqlite: membership needs a project and a patent")
	}
	if !m.State.Valid() {
		return fmt.Errorf("store/sqlite: invalid membership state %q", m.State)
	}
	_, err := r.writer.ExecContext(ctx,
		`INSERT INTO membership (project_id, patent_number, state, added_at)
		 VALUES (?,?,?,?)
		 ON CONFLICT(project_id, patent_number) DO NOTHING`,
		string(m.Project), m.Patent.Normalized(), string(m.State), encodeTime(m.AddedAt))
	if err != nil {
		return fmt.Errorf("store/sqlite: add membership: %w", err)
	}
	return nil
}

// Membership returns one membership, or store.ErrNotFound.
func (r *Repo) Membership(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (domain.Membership, error) {
	row := r.reader.QueryRowContext(ctx,
		`SELECT project_id, patent_number, state, added_at FROM membership
		 WHERE project_id = ? AND patent_number = ?`,
		string(project), patent.Normalized())
	var projectID, patentNumber, state, addedAt string
	if err := row.Scan(&projectID, &patentNumber, &state, &addedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Membership{}, store.ErrNotFound
		}
		return domain.Membership{}, fmt.Errorf("store/sqlite: get membership: %w", err)
	}
	added, err := decodeTime(addedAt)
	if err != nil {
		return domain.Membership{}, err
	}
	return domain.Membership{
		Project: domain.ProjectID(projectID),
		Patent:  patent,
		State:   domain.MembershipState(state),
		AddedAt: added,
	}, nil
}

// SetMembershipState changes a membership's state, or returns store.ErrNotFound
// when the patent is not a member of the project.
func (r *Repo) SetMembershipState(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, state domain.MembershipState) error {
	if !state.Valid() {
		return fmt.Errorf("store/sqlite: invalid membership state %q", state)
	}
	res, err := r.writer.ExecContext(ctx,
		`UPDATE membership SET state = ? WHERE project_id = ? AND patent_number = ?`,
		string(state), string(project), patent.Normalized())
	if err != nil {
		return fmt.Errorf("store/sqlite: set membership state: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store/sqlite: set membership state: %w", err)
	}
	if affected == 0 {
		return store.ErrNotFound
	}
	return nil
}

// Memberships returns every membership of a project, ordered by patent number.
func (r *Repo) Memberships(ctx context.Context, project domain.ProjectID) ([]domain.Membership, error) {
	rows, err := r.reader.QueryContext(ctx,
		`SELECT project_id, patent_number, state, added_at FROM membership
		 WHERE project_id = ? ORDER BY patent_number`, string(project))
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list memberships: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.Membership
	for rows.Next() {
		var projectID, patentNumber, state, addedAt string
		if err := rows.Scan(&projectID, &patentNumber, &state, &addedAt); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan membership: %w", err)
		}
		patent, err := domain.ParsePatentNumber(patentNumber)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: membership patent %q: %w", patentNumber, err)
		}
		added, err := decodeTime(addedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.Membership{
			Project: domain.ProjectID(projectID),
			Patent:  patent,
			State:   domain.MembershipState(state),
			AddedAt: added,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: list memberships: %w", err)
	}
	return out, nil
}
