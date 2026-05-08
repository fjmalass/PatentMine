package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/storage"

	_ "modernc.org/sqlite"
)

type Repository struct {
	db *sql.DB
}

func Open(path string) (*Repository, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return &Repository{db: db}, nil
}

func (r *Repository) Close() error {
	return r.db.Close()
}

func (r *Repository) Setup(ctx context.Context) error {
	if err := r.createTables(ctx); err != nil {
		return err
	}

	// Ensure default project exists
	now := nowString()
	if _, err := r.db.ExecContext(ctx, `insert or ignore into projects (id, name, created_at, updated_at) values ('default', 'Default Project', ?, ?)`, now, now); err != nil {
		return err
	}
	return nil
}

func (r *Repository) createTables(ctx context.Context) error {
	stmts := []string{
		fmt.Sprintf(`create table if not exists projects (
			id text primary key,
			name text not null,
			status text not null default '%s',
			comments text not null default '',
			created_at text not null,
			updated_at text not null
		)`, domain.ProjectStatusActive),
		`create table if not exists patents (
			number text primary key,
			title text not null,
			abstract text not null,
			assignee text not null,
			inventors_json text not null,
			publication_date text not null,
			grant_date text not null,
			expiration_date text not null default '',
			expiration_estimated integer not null default 0,
			source_url text not null,
			created_at text not null default '',
			latest_assignment text not null default ''
		)`,
		fmt.Sprintf(`create table if not exists project_patents (
			project_id text not null,
			patent_number text not null,
			status text not null default '%s',
			created_at text not null,
			primary key (project_id, patent_number),
			foreign key (project_id) references projects(id),
			foreign key (patent_number) references patents(number)
		)`, domain.CitationStatusStored),
		`create table if not exists patent_text_sections (
			patent_number text not null,
			section_type text not null,
			ordinal integer not null,
			text text not null,
			primary key (patent_number, section_type, ordinal),
			foreign key (patent_number) references patents(number)
		)`,
		fmt.Sprintf(`create table if not exists citation_edges (
			project_id text not null,
			source_patent text not null,
			target_patent text not null,
			relation_type text not null,
			status text not null default '%s',
			created_at text not null default '',
			refreshed_at text not null default '',
			labeled_at text not null default '',
			primary key (project_id, source_patent, target_patent, relation_type),
			foreign key (project_id) references projects(id),
			foreign key (source_patent) references patents(number)
		)`, domain.CitationStatusUnderReview),
		`create table if not exists patent_classifications (
			patent_number text not null,
			system text not null,
			code text not null,
			primary key (patent_number, system, code),
			foreign key (patent_number) references patents(number)
		)`,
		`create table if not exists classification_definitions (
			system text not null,
			code text not null,
			section text not null default '',
			class text not null default '',
			subclass text not null default '',
			main_group text not null default '',
			subgroup text not null default '',
			description text not null default '',
			primary key (system, code)
		)`,
		`create table if not exists research_notes (
			id integer primary key autoincrement,
			project_id text not null,
			patent_number text not null,
			body text not null,
			created_at text not null,
			foreign key (project_id) references projects(id),
			foreign key (patent_number) references patents(number)
		)`,
		`create table if not exists reference_entries (
			id integer primary key autoincrement,
			project_id text not null,
			patent_number text not null,
			citation_label text not null,
			created_at text not null,
			unique(project_id, patent_number),
			foreign key (project_id) references projects(id),
			foreign key (patent_number) references patents(number)
		)`,
		`create table if not exists ai_analyses (
			id integer primary key autoincrement,
			project_id text not null,
			patent_number text not null,
			analysis_type text not null,
			compared_patent_number text not null,
			provider text not null,
			body text not null,
			created_at text not null,
			foreign key (project_id) references projects(id),
			foreign key (patent_number) references patents(number)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// Project Management

func (r *Repository) CreateProject(ctx context.Context, p domain.Project) error {
	now := nowString()
	if _, err := r.db.ExecContext(ctx, `insert into projects (id, name, status, comments, created_at, updated_at) values (?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Status, p.Comments, now, now); err != nil {
		return err
	}
	return nil
}

func (r *Repository) GetProject(ctx context.Context, id string) (domain.Project, error) {
	var p domain.Project
	var createdAt, updatedAt string
	err := r.db.QueryRowContext(ctx, `select id, name, status, comments, created_at, updated_at from projects where id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Status, &p.Comments, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Project{}, fmt.Errorf("project not found")
		}
		return domain.Project{}, err
	}
	p.CreatedAt = parseTime(createdAt)
	p.UpdatedAt = parseTime(updatedAt)
	return p, nil
}

func (r *Repository) ListProjects(ctx context.Context) ([]domain.Project, error) {
	rows, err := r.db.QueryContext(ctx, `select id, name, status, comments, created_at, updated_at from projects order by updated_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		var p domain.Project
		var createdAt, updatedAt string
		if err := rows.Scan(&p.ID, &p.Name, &p.Status, &p.Comments, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		p.CreatedAt = parseTime(createdAt)
		p.UpdatedAt = parseTime(updatedAt)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateProject(ctx context.Context, p domain.Project) error {
	_, err := r.db.ExecContext(ctx, `update projects set name = ?, status = ?, comments = ?, updated_at = ? where id = ?`,
		p.Name, p.Status, p.Comments, nowString(), p.ID)
	return err
}

func (r *Repository) DeleteProject(ctx context.Context, id string) error {
	// For now, simple delete. In a real app we might want to clean up related data or use soft-delete.
	_, err := r.db.ExecContext(ctx, `delete from projects where id = ?`, id)
	return err
}

func (r *Repository) AddPatentToProject(ctx context.Context, projectID, patentNumber string) error {
	now := nowString()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `insert or ignore into project_patents (project_id, patent_number, created_at) values (?, ?, ?)`,
		projectID, patentNumber, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `update projects set updated_at = ? where id = ?`, now, projectID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) RemovePatentFromProject(ctx context.Context, projectID, patentNumber string) error {
	_, err := r.db.ExecContext(ctx, `delete from project_patents where project_id = ? and patent_number = ?`, projectID, patentNumber)
	return err
}

func (r *Repository) UpsertPatentBundle(ctx context.Context, projectID string, bundle domain.PatentBundle) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := upsertPatent(ctx, tx, bundle.Patent); err != nil {
		return err
	}

	// Association to project
	status := bundle.Patent.Status
	if status == "" {
		status = domain.CitationStatusStored
	}
	now := nowString()
	if _, err := tx.ExecContext(ctx, `insert into project_patents (project_id, patent_number, status, created_at)
		values (?, ?, ?, ?)
		on conflict(project_id, patent_number) do update set
			status = case when excluded.status = 'stored' then 'stored' else project_patents.status end`,
		projectID, bundle.Patent.Number, status, now); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `update projects set updated_at = ? where id = ?`, now, projectID); err != nil {
		return err
	}

	if err := replaceTexts(ctx, tx, bundle.Patent.Number, bundle.Sections); err != nil {
		return err
	}
	if err := replaceCitations(ctx, tx, projectID, bundle.Patent.Number, bundle.Citations); err != nil {
		return err
	}
	if err := replaceClassifications(ctx, tx, bundle.Patent.Number, bundle.Classifications); err != nil {
		return err
	}
	for _, ref := range bundle.References {
		label := ref.CitationLabel
		if label == "" {
			label = CitationLabel(bundle.Patent)
		}
		if _, err := tx.ExecContext(ctx, `insert into reference_entries (project_id, patent_number, citation_label, created_at)
			values (?, ?, ?, ?)
			on conflict(project_id, patent_number) do update set citation_label = excluded.citation_label`,
			projectID, firstNonEmpty(ref.PatentNumber, bundle.Patent.Number), label, nowString()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func upsertPatent(ctx context.Context, tx *sql.Tx, p domain.Patent) error {
	inventors, err := json.Marshal(p.Inventors)
	if err != nil {
		return err
	}
	createdAt := p.StoredAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err = tx.ExecContext(ctx, `insert into patents
		(number, title, abstract, assignee, inventors_json, publication_date, grant_date, expiration_date, expiration_estimated, source_url, created_at, latest_assignment)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(number) do update set
			title = excluded.title,
			abstract = excluded.abstract,
			assignee = excluded.assignee,
			inventors_json = excluded.inventors_json,
			publication_date = excluded.publication_date,
			grant_date = excluded.grant_date,
			expiration_date = excluded.expiration_date,
			expiration_estimated = excluded.expiration_estimated,
			source_url = excluded.source_url,
			latest_assignment = case when excluded.latest_assignment != '' then excluded.latest_assignment else patents.latest_assignment end`,
		p.Number, p.Title, p.Abstract, p.Assignee, string(inventors), p.PublicationDate, p.GrantDate, p.ExpirationDate, boolInt(p.ExpirationEstimated), p.SourceURL, createdAt.Format(time.RFC3339), p.LatestAssignment)
	return err
}

func replaceTexts(ctx context.Context, tx *sql.Tx, number string, sections []domain.PatentTextSection) error {
	if _, err := tx.ExecContext(ctx, `delete from patent_text_sections where patent_number = ?`, number); err != nil {
		return err
	}
	for _, section := range sections {
		if _, err := tx.ExecContext(ctx, `insert into patent_text_sections (patent_number, section_type, ordinal, text) values (?, ?, ?, ?)`,
			number, section.SectionType, section.Ordinal, section.Text); err != nil {
			return err
		}
	}
	return nil
}

func replaceCitations(ctx context.Context, tx *sql.Tx, projectID string, number string, edges []domain.CitationEdge) error {
	refreshedAt := nowString()
	refreshedRelations := map[string]bool{
		domain.RelationCites:   true,
		domain.RelationCitedBy: true,
	}
	for _, edge := range edges {
		source := firstNonEmpty(edge.SourcePatent, number)
		status := firstNonEmpty(edge.Status, domain.CitationStatusUnderReview)
		refreshedRelations[edge.RelationType] = true
		if _, err := tx.ExecContext(ctx, `insert into citation_edges (project_id, source_patent, target_patent, relation_type, status, created_at, refreshed_at, labeled_at)
			values (?, ?, ?, ?, ?, ?, ?, ?)
			on conflict(project_id, source_patent, target_patent, relation_type) do update set
				refreshed_at = excluded.refreshed_at`,
			projectID, source, edge.TargetPatent, edge.RelationType, status, refreshedAt, refreshedAt, refreshedAt); err != nil {
			return err
		}
	}
	for relation := range refreshedRelations {
		if _, err := tx.ExecContext(ctx, `delete from citation_edges where project_id = ? and source_patent = ? and relation_type = ? and refreshed_at != ?`,
			projectID, number, relation, refreshedAt); err != nil {
			return err
		}
	}
	return nil
}

func replaceClassifications(ctx context.Context, tx *sql.Tx, number string, classifications []domain.Classification) error {
	if _, err := tx.ExecContext(ctx, `delete from patent_classifications where patent_number = ?`, number); err != nil {
		return err
	}
	for _, cls := range classifications {
		if strings.TrimSpace(cls.Code) == "" {
			continue
		}
		if cls.System == "" || cls.Section == "" && cls.Class == "" {
			// Ensure it's parsed if coming from older logic
			parsed := domain.ParseClassification(cls.Code)
			cls.System = parsed.System
			cls.Section = parsed.Section
			cls.Class = parsed.Class
			cls.Subclass = parsed.Subclass
			cls.MainGroup = parsed.MainGroup
			cls.Subgroup = parsed.Subgroup
		}

		if _, err := tx.ExecContext(ctx, `insert or ignore into patent_classifications (patent_number, system, code) values (?, ?, ?)`,
			number, cls.System, cls.Code); err != nil {
			return err
		}
		if cls.Description != "" {
			if _, err := tx.ExecContext(ctx, `insert into classification_definitions 
				(system, code, section, class, subclass, main_group, subgroup, description)
				values (?, ?, ?, ?, ?, ?, ?, ?)
				on conflict(system, code) do update set description = excluded.description`,
				cls.System, cls.Code, cls.Section, cls.Class, cls.Subclass, cls.MainGroup, cls.Subgroup, cls.Description); err != nil {
				return err
			}
		} else {
			// Just ensure the structure is there even without description
			if _, err := tx.ExecContext(ctx, `insert or ignore into classification_definitions 
				(system, code, section, class, subclass, main_group, subgroup, description)
				values (?, ?, ?, ?, ?, ?, ?, ?)`,
				cls.System, cls.Code, cls.Section, cls.Class, cls.Subclass, cls.MainGroup, cls.Subgroup, ""); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Repository) GetPatent(ctx context.Context, projectID string, number string) (domain.Patent, error) {
	row := r.db.QueryRowContext(ctx, `
		select 
			p.number, p.title, p.abstract, p.assignee, p.inventors_json, p.publication_date, p.grant_date, p.expiration_date, p.expiration_estimated, p.source_url, pp.created_at, pp.status, p.latest_assignment,
			group_concat(c.code, ', ') as classification_label
		from patents p
		left join project_patents pp on p.number = pp.patent_number and pp.project_id = ?
		left join patent_classifications c on p.number = c.patent_number
		where p.number = ?
		group by p.number`, projectID, number)
	return scanPatent(row)
}

func (r *Repository) ListPatents(ctx context.Context, projectID string, opts storage.ListPatentsOptions) ([]domain.Patent, error) {
	args := []any{projectID}
	query := fmt.Sprintf(`
		select 
			p.number, p.title, p.abstract, p.assignee, p.inventors_json, p.publication_date, p.grant_date, p.expiration_date, p.expiration_estimated, p.source_url, pp.created_at, pp.status, p.latest_assignment,
			group_concat(c.code, ', ') as classification_label
		from patents p
		join project_patents pp on p.number = pp.patent_number
		left join patent_classifications c on p.number = c.patent_number
		where pp.project_id = ? and pp.status != '%s'`, domain.CitationStatusCached)

	if strings.TrimSpace(opts.Filter) != "" {
		query += ` and lower(p.number || ' ' || p.title || ' ' || p.abstract || ' ' || p.assignee || ' ' || p.inventors_json || ' ' || p.publication_date || ' ' || p.grant_date || ' ' || p.expiration_date || ' ' || p.source_url || ' ' || p.latest_assignment) like ?`
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(opts.Filter))+"%")
	}

	if strings.TrimSpace(opts.ClassFilter) != "" {
		// We use a subquery to find patents that have the specific class to avoid double-counting in group_concat
		query += ` and p.number in (select patent_number from patent_classifications where code like ?)`
		args = append(args, opts.ClassFilter+"%")
	}

	query += ` group by p.number`

	orderBy := "pp.created_at desc, p.number"
	if opts.SortColumn != "" {
		direction := "asc"
		if strings.EqualFold(opts.SortOrder, domain.SortOrderDesc) {
			direction = "desc"
		}

		switch strings.ToLower(opts.SortColumn) {
		case domain.SortColumnNumber:
			orderBy = "p.number " + direction
		case domain.SortColumnTitle:
			orderBy = "p.title " + direction
		case domain.SortColumnDate:
			orderBy = "p.publication_date " + direction
		case domain.SortColumnStatus:
			orderBy = "pp.status " + direction
		case domain.SortColumnAssignee:
			orderBy = "p.assignee " + direction
		case domain.SortColumnInventor:
			orderBy = "p.inventors_json " + direction
		case domain.SortColumnClass, domain.SortColumnCPC:
			orderBy = "classification_label " + direction
		}
	}

	query += " order by " + orderBy
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPatents(rows)
}

func (r *Repository) ListCitations(ctx context.Context, projectID string, number, relationType string, opts storage.ListCitationsOptions) ([]domain.CitationEdge, error) {
	query := `
		select 
			ce.source_patent, ce.target_patent, ce.relation_type, ce.status, ce.created_at, ce.refreshed_at, ce.labeled_at,
			p.title, p.inventors_json, p.expiration_date
		from citation_edges ce
		left join patents p on ce.target_patent = p.number
		where ce.project_id = ? and ce.source_patent = ? and ce.relation_type = ?`

	orderBy := "ce.target_patent"
	if opts.SortColumn != "" {
		direction := "asc"
		if strings.EqualFold(opts.SortOrder, domain.SortOrderDesc) {
			direction = "desc"
		}
		switch strings.ToLower(opts.SortColumn) {
		case domain.SortColumnNumber:
			orderBy = "ce.target_patent " + direction
		case domain.SortColumnTitle:
			orderBy = "p.title " + direction
		case domain.SortColumnDate:
			orderBy = "p.publication_date " + direction
		case domain.SortColumnStatus:
			orderBy = "ce.status " + direction
		}
	}

	rows, err := r.db.QueryContext(ctx, query+" order by "+orderBy, projectID, number, relationType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCitationEdges(rows)
}

func (r *Repository) ListCitationsByStatus(ctx context.Context, projectID string, status string, opts storage.ListCitationsOptions) ([]domain.CitationEdge, error) {
	query := `
		select 
			ce.source_patent, ce.target_patent, ce.relation_type, ce.status, ce.created_at, ce.refreshed_at, ce.labeled_at,
			p.title, p.inventors_json, p.expiration_date
		from citation_edges ce
		left join patents p on ce.target_patent = p.number
		where ce.project_id = ? and ce.status = ?`

	orderBy := "ce.labeled_at desc, ce.source_patent, ce.target_patent"
	if opts.SortColumn != "" {
		direction := "asc"
		if strings.EqualFold(opts.SortOrder, domain.SortOrderDesc) {
			direction = "desc"
		}
		switch strings.ToLower(opts.SortColumn) {
		case domain.SortColumnNumber:
			orderBy = "ce.target_patent " + direction
		case domain.SortColumnTitle:
			orderBy = "p.title " + direction
		case domain.SortColumnDate:
			orderBy = "p.publication_date " + direction
		case domain.SortColumnStatus:
			orderBy = "ce.status " + direction
		}
	}

	rows, err := r.db.QueryContext(ctx, query+" order by "+orderBy, projectID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCitationEdges(rows)
}

func scanCitationEdges(rows *sql.Rows) ([]domain.CitationEdge, error) {
	var out []domain.CitationEdge
	for rows.Next() {
		var edge domain.CitationEdge
		var createdAt, refreshedAt, labeledAt string
		var title, inventorsJson, expirationDate sql.NullString
		if err := rows.Scan(&edge.SourcePatent, &edge.TargetPatent, &edge.RelationType, &edge.Status, &createdAt, &refreshedAt, &labeledAt, &title, &inventorsJson, &expirationDate); err != nil {
			return nil, err
		}
		if edge.Status == "" {
			edge.Status = domain.CitationStatusUnderReview
		}
		edge.CreatedAt = parseTime(createdAt)
		edge.RefreshedAt = parseTime(refreshedAt)
		edge.LabeledAt = parseTime(labeledAt)
		edge.TargetTitle = title.String
		if inventorsJson.Valid && inventorsJson.String != "" {
			_ = json.Unmarshal([]byte(inventorsJson.String), &edge.TargetInventors)
		}
		edge.TargetExpirationDate = expirationDate.String
		out = append(out, edge)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateCitationStatus(ctx context.Context, projectID string, edge domain.CitationEdge, status string) error {
	_, err := r.db.ExecContext(ctx, `update citation_edges set status = ?, labeled_at = ? where project_id = ? and source_patent = ? and target_patent = ? and relation_type = ?`,
		status, nowString(), projectID, edge.SourcePatent, edge.TargetPatent, edge.RelationType)
	return err
}

func (r *Repository) UpdatePatentStatus(ctx context.Context, projectID string, number string, status string) error {
	_, err := r.db.ExecContext(ctx, `update project_patents set status = ? where project_id = ? and patent_number = ?`, status, projectID, number)
	return err
}

func (r *Repository) UpdateClassificationDescription(ctx context.Context, projectID string, system, code, description string) error {
	cls := domain.ParseClassification(code)
	if cls.System != "" {
		system = cls.System
	}
	_, err := r.db.ExecContext(ctx, `insert into classification_definitions 
		(system, code, section, class, subclass, main_group, subgroup, description)
		values (?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(system, code) do update set description = excluded.description`,
		system, code, cls.Section, cls.Class, cls.Subclass, cls.MainGroup, cls.Subgroup, description)
	return err
}

func (r *Repository) DeletePatent(ctx context.Context, projectID string, number string) error {
	return r.UpdatePatentStatus(ctx, projectID, number, domain.CitationStatusIgnored)
}

func (r *Repository) ListClassifications(ctx context.Context, projectID string, number string) ([]domain.Classification, error) {
	rows, err := r.db.QueryContext(ctx, `
		select c.patent_number, c.system, c.code, 
			coalesce(d.section, ''), coalesce(d.class, ''), coalesce(d.subclass, ''), 
			coalesce(d.main_group, ''), coalesce(d.subgroup, ''), coalesce(d.description, '')
		from patent_classifications c
		left join classification_definitions d on c.system = d.system and c.code = d.code
		where c.patent_number = ? 
		order by c.system, c.code`, number)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Classification
	for rows.Next() {
		var c domain.Classification
		if err := rows.Scan(&c.PatentNumber, &c.System, &c.Code, &c.Section, &c.Class, &c.Subclass, &c.MainGroup, &c.Subgroup, &c.Description); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) ListTextSections(ctx context.Context, projectID string, number string) ([]domain.PatentTextSection, error) {
	rows, err := r.db.QueryContext(ctx, `select patent_number, section_type, ordinal, text from patent_text_sections where patent_number = ? order by section_type, ordinal`, number)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.PatentTextSection
	for rows.Next() {
		var section domain.PatentTextSection
		if err := rows.Scan(&section.PatentNumber, &section.SectionType, &section.Ordinal, &section.Text); err != nil {
			return nil, err
		}
		out = append(out, section)
	}
	return out, rows.Err()
}

func (r *Repository) AddNote(ctx context.Context, projectID string, number, body string) (domain.ResearchNote, error) {
	created := nowString()
	res, err := r.db.ExecContext(ctx, `insert into research_notes (project_id, patent_number, body, created_at) values (?, ?, ?, ?)`, projectID, number, body, created)
	if err != nil {
		return domain.ResearchNote{}, err
	}
	id, _ := res.LastInsertId()
	return domain.ResearchNote{ID: id, PatentNumber: number, Body: body, CreatedAt: parseTime(created)}, nil
}

func (r *Repository) ListNotes(ctx context.Context, projectID string, number string) ([]domain.ResearchNote, error) {
	rows, err := r.db.QueryContext(ctx, `select id, patent_number, body, created_at from research_notes where project_id = ? and patent_number = ? order by created_at desc`, projectID, number)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ResearchNote
	for rows.Next() {
		var note domain.ResearchNote
		var created string
		if err := rows.Scan(&note.ID, &note.PatentNumber, &note.Body, &created); err != nil {
			return nil, err
		}
		note.CreatedAt = parseTime(created)
		out = append(out, note)
	}
	return out, rows.Err()
}

func (r *Repository) AddReference(ctx context.Context, projectID string, number, label string) (domain.ReferenceEntry, error) {
	created := nowString()
	_, err := r.db.ExecContext(ctx, `insert into reference_entries (project_id, patent_number, citation_label, created_at)
		values (?, ?, ?, ?)
		on conflict(project_id, patent_number) do update set citation_label = excluded.citation_label`, projectID, number, label, created)
	if err != nil {
		return domain.ReferenceEntry{}, err
	}
	return domain.ReferenceEntry{PatentNumber: number, CitationLabel: label, CreatedAt: parseTime(created)}, nil
}

func (r *Repository) ListReferences(ctx context.Context, projectID string) ([]domain.ReferenceEntry, error) {
	rows, err := r.db.QueryContext(ctx, `select id, patent_number, citation_label, created_at from reference_entries where project_id = ? order by created_at, patent_number`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ReferenceEntry
	for rows.Next() {
		var ref domain.ReferenceEntry
		var created string
		if err := rows.Scan(&ref.ID, &ref.PatentNumber, &ref.CitationLabel, &created); err != nil {
			return nil, err
		}
		ref.CreatedAt = parseTime(created)
		out = append(out, ref)
	}
	return out, rows.Err()
}

func (r *Repository) AddAIAnalysis(ctx context.Context, projectID string, analysis domain.AIAnalysis) (domain.AIAnalysis, error) {
	analysis.CreatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx, `insert into ai_analyses (project_id, patent_number, analysis_type, compared_patent_number, provider, body, created_at) values (?, ?, ?, ?, ?, ?, ?)`,
		projectID, analysis.PatentNumber, analysis.AnalysisType, analysis.ComparedPatentNumber, analysis.Provider, analysis.Body, analysis.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return domain.AIAnalysis{}, err
	}
	analysis.ID, _ = res.LastInsertId()
	return analysis, nil
}

func (r *Repository) ListAIAnalyses(ctx context.Context, projectID string, number string) ([]domain.AIAnalysis, error) {
	rows, err := r.db.QueryContext(ctx, `select id, patent_number, analysis_type, compared_patent_number, provider, body, created_at from ai_analyses where project_id = ? and patent_number = ? order by created_at desc`, projectID, number)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AIAnalysis
	for rows.Next() {
		var analysis domain.AIAnalysis
		var created string
		if err := rows.Scan(&analysis.ID, &analysis.PatentNumber, &analysis.AnalysisType, &analysis.ComparedPatentNumber, &analysis.Provider, &analysis.Body, &created); err != nil {
			return nil, err
		}
		analysis.CreatedAt = parseTime(created)
		out = append(out, analysis)
	}
	return out, rows.Err()
}

func scanPatent(row interface{ Scan(...any) error }) (domain.Patent, error) {
	var p domain.Patent
	var inventors string
	var expirationEstimated int
	var createdAt sql.NullString
	var status sql.NullString
	var latestAssignment sql.NullString
	var classificationLabel sql.NullString
	if err := row.Scan(&p.Number, &p.Title, &p.Abstract, &p.Assignee, &inventors, &p.PublicationDate, &p.GrantDate, &p.ExpirationDate, &expirationEstimated, &p.SourceURL, &createdAt, &status, &latestAssignment, &classificationLabel); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Patent{}, fmt.Errorf("patent not found")
		}
		return domain.Patent{}, err
	}
	if err := json.Unmarshal([]byte(inventors), &p.Inventors); err != nil {
		return domain.Patent{}, err
	}
	p.ExpirationEstimated = expirationEstimated == 1
	if createdAt.Valid {
		p.StoredAt = parseTime(createdAt.String)
	}
	p.Status = status.String
	if p.Status == "" {
		p.Status = domain.CitationStatusStored
	}
	p.LatestAssignment = latestAssignment.String
	p.ClassificationLabel = classificationLabel.String
	return p, nil
}

func scanPatents(rows *sql.Rows) ([]domain.Patent, error) {
	var out []domain.Patent
	for rows.Next() {
		p, err := scanPatent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func CitationLabel(p domain.Patent) string {
	if p.Title == "" {
		return p.Number
	}
	return fmt.Sprintf("%s, %s", p.Number, p.Title)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) time.Time {
	t, _ := time.Parse(time.RFC3339, value)
	return t
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
