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

const projectColumns = `id, name, created_at,
	application_number, filing_date, art_unit, attorney_docket_number`

// SaveProject inserts or updates a project by its id. The inventor list and
// examiner history are persisted into their side tables in the same
// transaction: inventors are replaced wholesale (the slice is the new
// canonical order), examiners are appended — older entries stay so the
// assignment timeline is preserved.
func (r *Repo) SaveProject(ctx context.Context, p domain.Project) (err error) {
	defer r.observeDuration("save_project", time.Now(), &err)
	if p.ID == "" {
		return errors.New("store/sqlite: cannot save project with empty id")
	}
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: save project %s: begin: %w", p.ID, err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx,
		`INSERT INTO project (id, name, created_at,
			application_number, filing_date, art_unit, attorney_docket_number)
		 VALUES (?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			created_at=excluded.created_at,
			application_number=excluded.application_number,
			filing_date=excluded.filing_date,
			art_unit=excluded.art_unit,
			attorney_docket_number=excluded.attorney_docket_number`,
		string(p.ID), p.Name, encodeTime(p.CreatedAt),
		p.ApplicationNumber, encodeTime(p.FilingDate),
		p.ArtUnit, p.AttorneyDocketNumber); err != nil {
		return fmt.Errorf("store/sqlite: save project %s: %w", p.ID, err)
	}

	if _, err = tx.ExecContext(ctx,
		`DELETE FROM project_inventor WHERE project_id = ?`, string(p.ID)); err != nil {
		return fmt.Errorf("store/sqlite: replace inventors: %w", err)
	}
	for i, name := range p.Inventors {
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO project_inventor (project_id, ordering, name) VALUES (?,?,?)`,
			string(p.ID), i, name); err != nil {
			return fmt.Errorf("store/sqlite: insert inventor %d: %w", i, err)
		}
	}

	for _, ex := range p.Examiners {
		if ex.Name == "" {
			continue
		}
		recorded := ex.RecordedAt
		if recorded.IsZero() {
			recorded = time.Now().UTC()
		}
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO project_examiner (project_id, name, recorded_at)
			 VALUES (?,?,?)
			 ON CONFLICT(project_id, recorded_at, name) DO NOTHING`,
			string(p.ID), ex.Name, encodeTime(recorded)); err != nil {
			return fmt.Errorf("store/sqlite: insert examiner: %w", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: save project %s: commit: %w", p.ID, err)
	}
	return nil
}

// Project returns one project, or store.ErrNotFound.
func (r *Repo) Project(ctx context.Context, id domain.ProjectID) (project domain.Project, err error) {
	defer r.observeDuration("project", time.Now(), &err)
	row := r.reader.QueryRowContext(ctx,
		`SELECT `+projectColumns+` FROM project WHERE id = ?`, string(id))
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Project{}, store.ErrNotFound
	}
	if err != nil {
		return domain.Project{}, fmt.Errorf("store/sqlite: get project %s: %w", id, err)
	}
	if err := r.loadProjectLists(ctx, &p); err != nil {
		return domain.Project{}, err
	}
	return p, nil
}

// ListProjects returns every project, ordered by name.
func (r *Repo) ListProjects(ctx context.Context) (out []domain.Project, err error) {
	defer r.observeDuration("list_projects", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT `+projectColumns+` FROM project ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out = nil
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
	for i := range out {
		if err := r.loadProjectLists(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func scanProject(s rowScanner) (domain.Project, error) {
	var (
		p          domain.Project
		id         string
		createdAt  string
		filingDate string
	)
	if err := s.Scan(&id, &p.Name, &createdAt,
		&p.ApplicationNumber, &filingDate, &p.ArtUnit, &p.AttorneyDocketNumber); err != nil {
		return domain.Project{}, err
	}
	p.ID = domain.ProjectID(id)
	t, err := decodeTime(createdAt)
	if err != nil {
		return domain.Project{}, err
	}
	p.CreatedAt = t
	if filingDate != "" {
		f, err := decodeTime(filingDate)
		if err != nil {
			return domain.Project{}, err
		}
		p.FilingDate = f
	}
	return p, nil
}

// loadProjectLists fills the Inventors and Examiners slices of p from the side
// tables. The examiners are ordered oldest-first so callers can apply their own
// "latest" rule against RecordedAt.
func (r *Repo) loadProjectLists(ctx context.Context, p *domain.Project) error {
	invs, err := r.reader.QueryContext(ctx,
		`SELECT name FROM project_inventor WHERE project_id = ? ORDER BY ordering`,
		string(p.ID))
	if err != nil {
		return fmt.Errorf("store/sqlite: list inventors: %w", err)
	}
	defer func() { _ = invs.Close() }()
	var inventors []string
	for invs.Next() {
		var name string
		if err := invs.Scan(&name); err != nil {
			return err
		}
		inventors = append(inventors, name)
	}
	if err := invs.Err(); err != nil {
		return err
	}
	p.Inventors = inventors

	exs, err := r.reader.QueryContext(ctx,
		`SELECT name, recorded_at FROM project_examiner WHERE project_id = ? ORDER BY recorded_at`,
		string(p.ID))
	if err != nil {
		return fmt.Errorf("store/sqlite: list examiners: %w", err)
	}
	defer func() { _ = exs.Close() }()
	var examiners []domain.ProjectExaminer
	for exs.Next() {
		var name, recorded string
		if err := exs.Scan(&name, &recorded); err != nil {
			return err
		}
		t, err := decodeTime(recorded)
		if err != nil {
			return err
		}
		examiners = append(examiners, domain.ProjectExaminer{Name: name, RecordedAt: t})
	}
	if err := exs.Err(); err != nil {
		return err
	}
	p.Examiners = examiners
	return nil
}

// AddMembership links a patent to a project. An existing link is left as is;
// state changes go through SetReviewState.
func (r *Repo) AddMembership(ctx context.Context, m domain.Membership) (err error) {
	defer r.observeDuration("add_membership", time.Now(), &err)
	if m.Project == "" || m.Patent.IsZero() {
		return errors.New("store/sqlite: membership needs a project and a patent")
	}
	if !m.ReviewState.Valid() {
		return fmt.Errorf("store/sqlite: invalid review state %q", m.ReviewState)
	}

	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: add membership tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO membership (project_id, patent_number, state, added_at)
		 VALUES (?,?,?,?)
		 ON CONFLICT(project_id, patent_number) DO NOTHING`,
		string(m.Project), m.Patent.Normalized(), string(m.ReviewState), encodeTime(m.AddedAt))
	if err != nil {
		return fmt.Errorf("store/sqlite: add membership: %w", err)
	}

	addedMethod := m.AddedMethod
	if addedMethod == "" {
		addedMethod = "direct"
	}
	var parentPatentVal *string
	if !m.ParentPatentNumber.IsZero() {
		norm := m.ParentPatentNumber.Normalized()
		parentPatentVal = &norm
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO membership_provenance (project_id, patent_number, added_method, parent_patent_number, source_provider, source_mode)
		 VALUES (?,?,?,?,?,?)
		 ON CONFLICT(project_id, patent_number) DO UPDATE SET
		   added_method = CASE WHEN added_method IN ('', 'direct', 'manual') AND excluded.added_method NOT IN ('', 'direct', 'manual') THEN excluded.added_method ELSE added_method END`,
		string(m.Project), m.Patent.Normalized(), addedMethod, parentPatentVal, m.SourceProvider, m.SourceMode)
	if err != nil {
		return fmt.Errorf("store/sqlite: add membership provenance: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: add membership commit: %w", err)
	}
	return nil
}

// Membership returns one membership, or store.ErrNotFound.
func (r *Repo) Membership(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (membership domain.Membership, err error) {
	defer r.observeDuration("membership", time.Now(), &err)
	row := r.reader.QueryRowContext(ctx,
		`SELECT m.project_id, m.patent_number, m.state, m.added_at,
		        COALESCE(p.added_method, 'direct'), COALESCE(p.parent_patent_number, ''),
		        COALESCE(p.source_provider, ''), COALESCE(p.source_mode, '')
		 FROM membership m
		 LEFT JOIN membership_provenance p
		   ON m.project_id = p.project_id AND m.patent_number = p.patent_number
		 WHERE m.project_id = ? AND m.patent_number = ?`,
		string(project), patent.Normalized())
	var projectID, patentNumber, state, addedAt string
	var addedMethod, parentPatentNumber, sourceProvider, sourceMode string
	if err := row.Scan(&projectID, &patentNumber, &state, &addedAt, &addedMethod, &parentPatentNumber, &sourceProvider, &sourceMode); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Membership{}, store.ErrNotFound
		}
		return domain.Membership{}, fmt.Errorf("store/sqlite: get membership: %w", err)
	}
	added, err := decodeTime(addedAt)
	if err != nil {
		return domain.Membership{}, err
	}
	var parentPatent domain.PatentNumber
	if parentPatentNumber != "" {
		parentPatent, _ = domain.ParsePatentNumber(parentPatentNumber)
	}
	return domain.Membership{
		Project:            domain.ProjectID(projectID),
		Patent:             patent,
		ReviewState:        domain.ReviewState(state),
		AddedAt:            added,
		AddedMethod:        addedMethod,
		ParentPatentNumber: parentPatent,
		SourceProvider:     sourceProvider,
		SourceMode:         sourceMode,
	}, nil
}

// SetReviewStates changes multiple memberships' states in a single transaction, or returns store.ErrNotFound.
func (r *Repo) SetReviewStates(ctx context.Context, project domain.ProjectID, patents []domain.PatentNumber, state domain.ReviewState) (err error) {
	defer r.observeDuration("set_review_states", time.Now(), &err)
	if !state.Valid() {
		return fmt.Errorf("store/sqlite: invalid review state %q", state)
	}
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: set review states tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `UPDATE membership SET state = ? WHERE project_id = ? AND patent_number = ?`)
	if err != nil {
		return fmt.Errorf("store/sqlite: set review states prepare: %w", err)
	}
	defer stmt.Close()

	for _, patent := range patents {
		_, err := stmt.ExecContext(ctx, string(state), string(project), patent.Normalized())
		if err != nil {
			return fmt.Errorf("store/sqlite: set review states exec: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: set review states commit: %w", err)
	}
	return nil
}

// PromotePatentMemberships changes review states from one to another for a patent across all projects.
func (r *Repo) PromotePatentMemberships(ctx context.Context, patent domain.PatentNumber, from, to domain.ReviewState) (err error) {
	defer r.observeDuration("promote_patent_memberships", time.Now(), &err)
	if !from.Valid() || !to.Valid() {
		return fmt.Errorf("store/sqlite: invalid review state %q or %q", from, to)
	}
	_, err = r.writer.ExecContext(ctx, `
		UPDATE membership
		SET state = ?
		WHERE patent_number = ? AND state = ?`,
		string(to), patent.Normalized(), string(from))
	if err != nil {
		return fmt.Errorf("store/sqlite: promote patent memberships: %w", err)
	}
	return nil
}

// DeleteMembership permanently removes a patent from a project.
func (r *Repo) DeleteMembership(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (err error) {
	defer r.observeDuration("delete_membership", time.Now(), &err)
	_, err = r.writer.ExecContext(ctx,
		`DELETE FROM membership WHERE project_id = ? AND patent_number = ?`,
		string(project), patent.Normalized())
	if err != nil {
		return fmt.Errorf("store/sqlite: delete membership: %w", err)
	}
	return nil
}

// Memberships returns every membership of a project, ordered by patent number.
func (r *Repo) Memberships(ctx context.Context, project domain.ProjectID) (out []domain.Membership, err error) {
	defer r.observeDuration("memberships", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT m.project_id, m.patent_number, m.state, m.added_at,
		        COALESCE(p.added_method, 'direct'), COALESCE(p.parent_patent_number, ''),
		        COALESCE(p.source_provider, ''), COALESCE(p.source_mode, '')
		 FROM membership m
		 LEFT JOIN membership_provenance p
		   ON m.project_id = p.project_id AND m.patent_number = p.patent_number
		 WHERE m.project_id = ? ORDER BY m.added_at ASC, m.patent_number`, string(project))
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list memberships: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out = nil
	for rows.Next() {
		var projectID, patentNumber, state, addedAt string
		var addedMethod, parentPatentNumber, sourceProvider, sourceMode string
		if err := rows.Scan(&projectID, &patentNumber, &state, &addedAt, &addedMethod, &parentPatentNumber, &sourceProvider, &sourceMode); err != nil {
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
		var parentPatent domain.PatentNumber
		if parentPatentNumber != "" {
			parentPatent, _ = domain.ParsePatentNumber(parentPatentNumber)
		}
		out = append(out, domain.Membership{
			Project:            domain.ProjectID(projectID),
			Patent:             patent,
			ReviewState:        domain.ReviewState(state),
			AddedAt:            added,
			AddedMethod:        addedMethod,
			ParentPatentNumber: parentPatent,
			SourceProvider:     sourceProvider,
			SourceMode:         sourceMode,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: list memberships: %w", err)
	}
	return out, nil
}

// ProjectsForPatent returns the IDs of every project the patent belongs to.
func (r *Repo) ProjectsForPatent(ctx context.Context, patent domain.PatentNumber) (out []domain.ProjectID, err error) {
	defer r.observeDuration("projects_for_patent", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT project_id FROM membership WHERE patent_number = ? ORDER BY project_id`,
		patent.Normalized())
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: projects for patent: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan project id: %w", err)
		}
		out = append(out, domain.ProjectID(id))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: projects for patent: %w", err)
	}
	return out, nil
}
