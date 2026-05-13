package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/storage"

	_ "modernc.org/sqlite"
)

type Repository struct {
	db     *sql.DB
	logger *slog.Logger
}

func Open(path string, logger *slog.Logger) (*Repository, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if logger == nil {
		logger = slog.Default()
	}
	return &Repository{db: db, logger: logger}, nil
}

func (r *Repository) Close() error {
	return r.db.Close()
}

func (r *Repository) Setup(ctx context.Context) error {
	if err := r.createTables(ctx); err != nil {
		return err
	}
	if err := r.runMigrations(ctx); err != nil {
		return err
	}

	// Ensure default project exists
	now := nowString()
	if _, err := r.db.ExecContext(ctx, `insert or ignore into projects (id, name, created_at, updated_at) values ('default', 'Default Project', ?, ?)`, now, now); err != nil {
		return err
	}
	return nil
}

func (r *Repository) runMigrations(ctx context.Context) error {
	alterations := []string{
		`alter table project_ids add column kind_code text not null default ''`,
		`alter table project_ids add column country_code text not null default ''`,
		`alter table project_ids add column relevant_passages text not null default ''`,
		`alter table project_ids add column in_full integer not null default 0`,
		`alter table project_ids_npl add column in_full integer not null default 0`,
		`create trigger if not exists trg_project_patents_delete_ids
			after delete on project_patents
			begin
				delete from project_ids where project_id = OLD.project_id and patent_number = OLD.patent_number;
			end`,
		`create trigger if not exists trg_patents_delete_ids
			after delete on patents
			begin
				delete from project_ids where patent_number = OLD.number;
			end`,
	}
	for _, stmt := range alterations {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			if !strings.Contains(err.Error(), "duplicate column name") {
				return err
			}
		}
	}
	return nil
}

func (r *Repository) createTables(ctx context.Context) error {
	stmts := []string{
		fmt.Sprintf(`create table if not exists projects (
			id text primary key,
			name text not null,
			status text not null default '%s',
			summary_status text not null default '',
			summary text not null default '',
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
			source_google_url text not null,
			import_source text not null default '',
			created_at text not null default '',
			latest_assignment text not null default '',
			updated_at text not null default '',
			expected_citations integer not null default -1,
			expected_cited_by integer not null default -1,
			application_number text not null default '',
			application_date text not null default '',
			publication_number text not null default '',
			grant_number text not null default '',
			first_claim text not null default ''
		)`,
		fmt.Sprintf(`create table if not exists project_patents (
			project_id text not null,
			patent_number text not null,
			status text not null default '%s',
			created_at text not null,
			status_changed_at text not null default '',
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
		)`, domain.CitationStatusIgnored),
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
		`create table if not exists project_events (
			id integer primary key autoincrement,
			project_id text not null,
			event_type text not null,
			event_date text not null default '',
			due_date text not null default '',
			reference text not null default '',
			notes text not null default '',
			created_at text not null,
			foreign key (project_id) references projects(id)
		)`,
		`create table if not exists settings (
			key text primary key,
			value text not null
		)`,
		fmt.Sprintf(`create table if not exists project_invoices (
			id integer primary key autoincrement,
			project_id text not null,
			direction text not null default '%s',
			firm_name text not null default '',
			invoice_number text not null default '',
			amount text not null default '',
			currency text not null default 'USD',
			invoice_date text not null default '',
			due_date text not null default '',
			paid_date text not null default '',
			status text not null default '%s',
			description text not null default '',
			notes text not null default '',
			created_at text not null,
			foreign key (project_id) references projects(id)
		)`, domain.InvoiceDirectionToFirm, domain.InvoiceStatusOutstanding),
		fmt.Sprintf(`create table if not exists patent_family (
			project_id    text not null,
			parent_number text not null,
			child_number  text not null,
			relation_type text not null default '%s',
			created_at    text not null,
			primary key (project_id, parent_number, child_number),
			foreign key (project_id) references projects(id)
		)`, domain.FamilyRelationContinuation),
		`create table if not exists project_ids (
			id            integer primary key autoincrement,
			project_id    text not null,
			patent_number text not null,
			notes         text not null default '',
			status        text not null default 'pending',
			added_at      text not null,
			unique(project_id, patent_number),
			foreign key (project_id) references projects(id)
		)`,
		`create table if not exists project_ids_metadata (
			project_id      text primary key,
			app_number      text not null default '',
			filing_date     text not null default '',
			first_inventor  text not null default '',
			examiner_name   text not null default '',
			art_unit        text not null default '',
			attorney_docket text not null default '',
			updated_at      text not null default '',
			foreign key (project_id) references projects(id)
		)`,
		`create table if not exists project_ids_npl (
			id              integer primary key autoincrement,
			project_id      text not null,
			npl_author      text not null default '',
			npl_title       text not null default '',
			npl_date        text not null default '',
			npl_publisher   text not null default '',
			relevant_passages text not null default '',
			notes           text not null default '',
			status          text not null default 'pending',
			added_at        text not null,
			foreign key (project_id) references projects(id)
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
	if _, err := r.db.ExecContext(ctx, `insert into projects (id, name, status, summary_status, summary, comments, created_at, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Status, p.SummaryStatus, p.Summary, p.Comments, now, now); err != nil {
		return err
	}
	return nil
}

func (r *Repository) GetProject(ctx context.Context, id string) (domain.Project, error) {
	var p domain.Project
	var createdAt, updatedAt string
	err := r.db.QueryRowContext(ctx, `select id, name, status, summary_status, summary, comments, created_at, updated_at from projects where id = ?`, id).
		Scan(&p.ID, &p.Name, &p.Status, &p.SummaryStatus, &p.Summary, &p.Comments, &createdAt, &updatedAt)
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
	rows, err := r.db.QueryContext(ctx, `select id, name, status, summary_status, summary, comments, created_at, updated_at from projects order by updated_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		var p domain.Project
		var createdAt, updatedAt string
		if err := rows.Scan(&p.ID, &p.Name, &p.Status, &p.SummaryStatus, &p.Summary, &p.Comments, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		p.CreatedAt = parseTime(createdAt)
		p.UpdatedAt = parseTime(updatedAt)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateProject(ctx context.Context, p domain.Project) error {
	_, err := r.db.ExecContext(ctx, `update projects set name = ?, status = ?, summary_status = ?, summary = ?, comments = ?, updated_at = ? where id = ?`,
		p.Name, p.Status, p.SummaryStatus, p.Summary, p.Comments, nowString(), p.ID)
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
	r.logger.Info("repository.upsert_bundle", "project", projectID, "patent", bundle.Patent.Number)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := upsertPatent(ctx, tx, bundle.Patent, r.logger); err != nil {
		r.logger.Error("repository.upsert_patent failed", "patent", bundle.Patent.Number, "error", err)
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
		r.logger.Error("repository.project_patent_association failed", "project", projectID, "patent", bundle.Patent.Number, "error", err)
		return err
	}

	if _, err := tx.ExecContext(ctx, `update projects set updated_at = ? where id = ?`, now, projectID); err != nil {
		return err
	}

	if err := replaceTexts(ctx, tx, bundle.Patent.Number, bundle.Sections, r.logger); err != nil {
		return err
	}
	if err := replaceCitations(ctx, tx, projectID, bundle.Patent.Number, bundle.Citations, r.logger); err != nil {
		return err
	}
	if err := replaceClassifications(ctx, tx, bundle.Patent.Number, bundle.Classifications, r.logger); err != nil {
		return err
	}
	// Store family edges only when at least one side exists as a stored patent in this project
	for _, edge := range bundle.FamilyEdges {
		edge.ProjectID = projectID
		var exists int
		// Check if either parent or child is stored as a citation or patent in this project
		if err := tx.QueryRowContext(ctx, `select count(*) from project_patents where project_id = ? and patent_number in (?, ?) and status != 'ignored'`,
			projectID, edge.ParentNumber, edge.ChildNumber).Scan(&exists); err != nil || exists == 0 {
			continue // neither side known yet — skip
		}
		if _, err := tx.ExecContext(ctx,
			`insert into patent_family (project_id, parent_number, child_number, relation_type, created_at)
			 values (?, ?, ?, ?, ?)
			 on conflict(project_id, parent_number, child_number) do update set relation_type = excluded.relation_type`,
			projectID, edge.ParentNumber, edge.ChildNumber, edge.RelationType, nowString()); err != nil {
			return err
		}
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
	if err := tx.Commit(); err != nil {
		return err
	}
	r.logger.Info("repository.upsert_bundle ok", "project", projectID, "patent", bundle.Patent.Number, "citations", len(bundle.Citations), "sections", len(bundle.Sections), "classifications", len(bundle.Classifications))
	return nil
}

func upsertPatent(ctx context.Context, tx *sql.Tx, p domain.Patent, logger *slog.Logger) error {
	inventors, err := json.Marshal(p.Inventors)
	if err != nil {
		return err
	}
	createdAt := p.StoredAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err = tx.ExecContext(ctx, `insert into patents
		(number, title, abstract, assignee, inventors_json, publication_date, grant_date, expiration_date, expiration_estimated, source_google_url, import_source, created_at, latest_assignment, updated_at, expected_citations, expected_cited_by, application_number, application_date, publication_number, grant_number, first_claim)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(number) do update set
			title = excluded.title,
			abstract = excluded.abstract,
			assignee = excluded.assignee,
			inventors_json = excluded.inventors_json,
			publication_date = excluded.publication_date,
			grant_date = excluded.grant_date,
			expiration_date = excluded.expiration_date,
			expiration_estimated = excluded.expiration_estimated,
			source_google_url = excluded.source_google_url,
			import_source = case when excluded.import_source != '' then excluded.import_source else patents.import_source end,
			latest_assignment = case when excluded.latest_assignment != '' then excluded.latest_assignment else patents.latest_assignment end,
			updated_at = excluded.updated_at,
			expected_citations = case when excluded.expected_citations >= 0 then excluded.expected_citations else patents.expected_citations end,
			expected_cited_by = case when excluded.expected_cited_by >= 0 then excluded.expected_cited_by else patents.expected_cited_by end,
			application_number = case when excluded.application_number != '' then excluded.application_number else patents.application_number end,
			application_date = case when excluded.application_date != '' then excluded.application_date else patents.application_date end,
			publication_number = case when excluded.publication_number != '' then excluded.publication_number else patents.publication_number end,
			grant_number = case when excluded.grant_number != '' then excluded.grant_number else patents.grant_number end,
			first_claim = case when excluded.first_claim != '' then excluded.first_claim else patents.first_claim end`,
		p.Number, p.Title, p.Abstract, p.Assignee, string(inventors), p.PublicationDate, p.GrantDate, p.ExpirationDate, boolInt(p.ExpirationEstimated), p.SourceGoogleURL, p.ImportSource, createdAt.Format(time.RFC3339), p.LatestAssignment, nowString(), p.ExpectedCitations, p.ExpectedCitedBy, p.ApplicationNumber, p.ApplicationDate, p.PublicationNumber, p.GrantNumber, p.FirstClaim)
	return err
}

func replaceTexts(ctx context.Context, tx *sql.Tx, number string, sections []domain.PatentTextSection, logger *slog.Logger) error {
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

func replaceCitations(ctx context.Context, tx *sql.Tx, projectID string, number string, edges []domain.CitationEdge, logger *slog.Logger) error {
	if len(edges) == 0 {
		return nil
	}
	refreshedAt := nowString()
	refreshedRelations := map[string]bool{}
	for _, edge := range edges {
		source := firstNonEmpty(edge.SourcePatent, number)
		status := firstNonEmpty(edge.Status, domain.CitationStatusIgnored)
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

func replaceClassifications(ctx context.Context, tx *sql.Tx, number string, classifications []domain.Classification, logger *slog.Logger) error {
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
			p.number, p.title, p.abstract, p.assignee, p.inventors_json, p.publication_date, p.grant_date, p.expiration_date, p.expiration_estimated, p.source_google_url, p.import_source, pp.created_at, pp.status, p.latest_assignment,
			group_concat(c.code, ', ') as classification_label, pp.status_changed_at, p.updated_at, p.expected_citations, p.expected_cited_by,
			p.application_number, p.application_date, p.publication_number, p.grant_number, p.first_claim,
			(select count(*) from research_notes rn where rn.patent_number = p.number and rn.project_id = ?) as notes_count
		from patents p
		left join project_patents pp on p.number = pp.patent_number and pp.project_id = ?
		left join patent_classifications c on p.number = c.patent_number
		where p.number = ?
		group by p.number`, projectID, projectID, number)
	return scanPatent(row)
}

func sortExpr(col, direction string) string {
	switch strings.ToLower(col) {
	case domain.SortColumnNumber:
		return "p.number " + direction
	case domain.SortColumnTitle:
		return "p.title " + direction
	case domain.SortColumnDate:
		return "p.publication_date " + direction
	case domain.SortColumnStatus:
		return "pp.status " + direction
	case domain.SortColumnAssignee:
		return "p.assignee " + direction
	case domain.SortColumnInventor:
		return "p.inventors_json " + direction
	case domain.SortColumnClass, domain.SortColumnCPC:
		return "classification_label " + direction
	case domain.SortColumnExpiration:
		return "case when p.expiration_date = '' then 1 else 0 end " + direction + ", p.expiration_date " + direction
	case domain.SortColumnUpdated:
		return "p.updated_at " + direction
	case domain.SortColumnNotes:
		return "notes_count " + direction
	case domain.SortColumnIDS:
		return "case when pid.status is null or pid.status = '' then 1 else 0 end " + direction + ", pid.status " + direction
	}
	return ""
}

func (r *Repository) ListPatents(ctx context.Context, projectID string, opts storage.ListPatentsOptions) ([]domain.Patent, error) {
	args := []any{projectID}
	// statusClause is appended after the base WHERE; subqueries that reference project_id
	// need an extra projectID arg inserted immediately after the base projectID arg.
	var statusClause string
	switch strings.ToLower(opts.StatusFilter) {
	case domain.CitationStatusIgnored:
		// deleted project patents + citation references marked ignored
		statusClause = `(pp.status = 'ignored' or p.number in (select target_patent from citation_edges where project_id = ? and status = 'ignored'))`
		args = append(args, projectID)
	case domain.CitationStatusUnderReview:
		// citation references marked under review
		statusClause = `p.number in (select target_patent from citation_edges where project_id = ? and status = 'under_review')`
		args = append(args, projectID)
	case domain.CitationStatusCached:
		statusClause = `pp.status = 'cached'`
	case "none":
		statusClause = `1=1`
	default: // "" or "stored"
		statusClause = fmt.Sprintf("pp.status = '%s'", domain.CitationStatusStored)
	}
	query := `
		select
			p.number, p.title, p.abstract, p.assignee, p.inventors_json, p.publication_date, p.grant_date, p.expiration_date, p.expiration_estimated, p.source_google_url, p.import_source, pp.created_at, pp.status, p.latest_assignment,
			group_concat(c.code, ', ') as classification_label, pp.status_changed_at, p.updated_at, p.expected_citations, p.expected_cited_by,
			p.application_number, p.application_date, p.publication_number, p.grant_number, p.first_claim,
			(select count(*) from research_notes rn where rn.patent_number = p.number and rn.project_id = ?) as notes_count
		from patents p
		join project_patents pp on p.number = pp.patent_number
		left join project_ids pid on pid.project_id = pp.project_id and pid.patent_number = p.number
		left join patent_classifications c on p.number = c.patent_number
		where pp.project_id = ? and ` + statusClause
	args = append(args, projectID)

	if strings.TrimSpace(opts.Filter) != "" {
		query += ` and lower(p.number || ' ' || p.title || ' ' || p.abstract || ' ' || p.assignee || ' ' || p.inventors_json || ' ' || p.publication_date || ' ' || p.grant_date || ' ' || p.expiration_date || ' ' || p.source_google_url || ' ' || p.latest_assignment) like ?`
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(opts.Filter))+"%")
	}

	if len(opts.ClassFilters) > 0 {
		if strings.EqualFold(opts.ClassFilterOp, "or") {
			conds := make([]string, len(opts.ClassFilters))
			for i, f := range opts.ClassFilters {
				conds[i] = "code like ?"
				args = append(args, strings.ToUpper(strings.TrimSpace(f))+"%")
			}
			query += ` and p.number in (select patent_number from patent_classifications where ` + strings.Join(conds, " or ") + `)`
		} else {
			for _, f := range opts.ClassFilters {
				query += ` and p.number in (select patent_number from patent_classifications where code like ?)`
				args = append(args, strings.ToUpper(strings.TrimSpace(f))+"%")
			}
		}
	}

	query += ` group by p.number`

	orderBy := "pp.created_at desc, p.number"
	if opts.SortColumn != "" {
		direction := "asc"
		if strings.EqualFold(opts.SortOrder, domain.SortOrderDesc) {
			direction = "desc"
		}
		orderBy = sortExpr(opts.SortColumn, direction)
		if opts.SortColumn2 != "" {
			dir2 := "asc"
			if strings.EqualFold(opts.SortOrder2, domain.SortOrderDesc) {
				dir2 = "desc"
			}
			if expr2 := sortExpr(opts.SortColumn2, dir2); expr2 != "" {
				orderBy += ", " + expr2
			}
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
			p.title, p.inventors_json, p.expiration_date, p.import_source
		from citation_edges ce
		left join patents p on ce.target_patent = p.number
		where ce.project_id = ? and ce.source_patent = ? and ce.relation_type = ?`

	args := []any{projectID, number, relationType}
	if opts.StatusFilter != "" {
		query += ` and ce.status = ?`
		args = append(args, opts.StatusFilter)
	}

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

	rows, err := r.db.QueryContext(ctx, query+" order by "+orderBy, args...)
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
			p.title, p.inventors_json, p.expiration_date, p.import_source
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
		var title, inventorsJson, expirationDate, importSource sql.NullString
		if err := rows.Scan(&edge.SourcePatent, &edge.TargetPatent, &edge.RelationType, &edge.Status, &createdAt, &refreshedAt, &labeledAt, &title, &inventorsJson, &expirationDate, &importSource); err != nil {
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
		edge.TargetImportSource = importSource.String
		out = append(out, edge)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateCitationStatus(ctx context.Context, projectID string, edge domain.CitationEdge, status string) error {
	r.logger.Info("repository.update_citation_status", "project", projectID, "source", edge.SourcePatent, "target", edge.TargetPatent, "status", status)
	_, err := r.db.ExecContext(ctx, `update citation_edges set status = ?, labeled_at = ? where project_id = ? and source_patent = ? and target_patent = ? and relation_type = ?`,
		status, nowString(), projectID, edge.SourcePatent, edge.TargetPatent, edge.RelationType)
	return err
}

func (r *Repository) UpdatePatentStatus(ctx context.Context, projectID string, number string, status string) error {
	r.logger.Info("repository.update_patent_status", "project", projectID, "patent", number, "status", status)
	_, err := r.db.ExecContext(ctx, `update project_patents set status = ?, status_changed_at = ? where project_id = ? and patent_number = ?`, status, nowString(), projectID, number)
	return err
}

func (r *Repository) UpdatePatentDate(ctx context.Context, number string, dateType string, value string) error {
	var column string
	switch dateType {
	case "app":
		column = "application_date"
	case "pub":
		column = "publication_date"
	case "grant":
		column = "grant_date"
	default:
		return fmt.Errorf("invalid date type: %s", dateType)
	}
	r.logger.Info("repository.update_patent_date", "patent", number, "type", dateType, "value", value)
	_, err := r.db.ExecContext(ctx, fmt.Sprintf(`update patents set %s = ?, updated_at = ? where number = ?`, column), value, nowString(), number)
	return err
}

func (r *Repository) UpdatePatentNumber(ctx context.Context, number string, numType string, value string) error {
	var column string
	switch numType {
	case "app":
		column = "application_number"
	case "pub":
		column = "publication_number"
	case "grant":
		column = "grant_number"
	default:
		return fmt.Errorf("invalid number type: %s", numType)
	}
	r.logger.Info("repository.update_patent_number", "patent", number, "type", numType, "value", value)
	_, err := r.db.ExecContext(ctx, fmt.Sprintf(`update patents set %s = ?, updated_at = ? where number = ?`, column), value, nowString(), number)
	return err
}

func (r *Repository) UpdateClassificationDescription(ctx context.Context, projectID string, system, code, description string) error {
	r.logger.Debug("repository.update_classification_description", "system", system, "code", code)
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
	r.logger.Info("repository.delete_patent", "project", projectID, "patent", number)
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
	r.logger.Info("repository.add_note", "project", projectID, "patent", number)
	created := nowString()
	res, err := r.db.ExecContext(ctx, `insert into research_notes (project_id, patent_number, body, created_at) values (?, ?, ?, ?)`, projectID, number, body, created)
	if err != nil {
		r.logger.Error("repository.add_note failed", "project", projectID, "patent", number, "error", err)
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
	r.logger.Info("repository.add_reference", "project", projectID, "patent", number, "label", label)
	created := nowString()
	_, err := r.db.ExecContext(ctx, `insert into reference_entries (project_id, patent_number, citation_label, created_at)
		values (?, ?, ?, ?)
		on conflict(project_id, patent_number) do update set citation_label = excluded.citation_label`, projectID, number, label, created)
	if err != nil {
		r.logger.Error("repository.add_reference failed", "project", projectID, "patent", number, "error", err)
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
	r.logger.Info("repository.add_ai_analysis", "project", projectID, "patent", analysis.PatentNumber, "type", analysis.AnalysisType)
	analysis.CreatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx, `insert into ai_analyses (project_id, patent_number, analysis_type, compared_patent_number, provider, body, created_at) values (?, ?, ?, ?, ?, ?, ?)`,
		projectID, analysis.PatentNumber, analysis.AnalysisType, analysis.ComparedPatentNumber, analysis.Provider, analysis.Body, analysis.CreatedAt.Format(time.RFC3339))
	if err != nil {
		r.logger.Error("repository.add_ai_analysis failed", "project", projectID, "patent", analysis.PatentNumber, "error", err)
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
	var statusChangedAt sql.NullString
	var updatedAt sql.NullString
	var importSource sql.NullString
	if err := row.Scan(&p.Number, &p.Title, &p.Abstract, &p.Assignee, &inventors, &p.PublicationDate, &p.GrantDate, &p.ExpirationDate, &expirationEstimated, &p.SourceGoogleURL, &importSource, &createdAt, &status, &latestAssignment, &classificationLabel, &statusChangedAt, &updatedAt, &p.ExpectedCitations, &p.ExpectedCitedBy, &p.ApplicationNumber, &p.ApplicationDate, &p.PublicationNumber, &p.GrantNumber, &p.FirstClaim, &p.NotesCount); err != nil {
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
	if statusChangedAt.Valid && statusChangedAt.String != "" {
		p.StatusChangedAt = parseTime(statusChangedAt.String)
	}
	if updatedAt.Valid && updatedAt.String != "" {
		p.UpdatedAt = parseTime(updatedAt.String)
	}
	p.LatestAssignment = latestAssignment.String
	p.ClassificationLabel = classificationLabel.String
	p.ImportSource = importSource.String
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

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
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

// Settings

func (r *Repository) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.QueryRowContext(ctx, `select value from settings where key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (r *Repository) SetSetting(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx, `insert into settings (key, value) values (?, ?) on conflict(key) do update set value = excluded.value`, key, value)
	return err
}

// Project lifecycle events

func (r *Repository) AddProjectEvent(ctx context.Context, e domain.ProjectEvent) (domain.ProjectEvent, error) {
	r.logger.Info("repository.add_project_event", "project", e.ProjectID, "type", e.EventType)
	now := nowString()
	res, err := r.db.ExecContext(ctx,
		`insert into project_events (project_id, event_type, event_date, due_date, reference, notes, created_at) values (?, ?, ?, ?, ?, ?, ?)`,
		e.ProjectID, e.EventType, e.EventDate, e.DueDate, e.Reference, e.Notes, now)
	if err != nil {
		r.logger.Error("repository.add_project_event failed", "project", e.ProjectID, "error", err)
		return domain.ProjectEvent{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.ProjectEvent{}, err
	}
	e.ID = id
	e.CreatedAt = parseTime(now)
	return e, nil
}

func (r *Repository) ListProjectEvents(ctx context.Context, projectID string) ([]domain.ProjectEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`select id, project_id, event_type, event_date, due_date, reference, notes, created_at from project_events where project_id = ? order by event_date desc, created_at desc`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ProjectEvent
	for rows.Next() {
		var e domain.ProjectEvent
		var createdAt string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.EventType, &e.EventDate, &e.DueDate, &e.Reference, &e.Notes, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt = parseTime(createdAt)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteProjectEvent(ctx context.Context, id int64) error {
	r.logger.Info("repository.delete_project_event", "id", id)
	_, err := r.db.ExecContext(ctx, `delete from project_events where id = ?`, id)
	return err
}

// Project invoices

func (r *Repository) AddProjectInvoice(ctx context.Context, inv domain.ProjectInvoice) (domain.ProjectInvoice, error) {
	r.logger.Info("repository.add_project_invoice", "project", inv.ProjectID, "firm", inv.FirmName, "amount", inv.Amount)
	now := nowString()
	res, err := r.db.ExecContext(ctx,
		`insert into project_invoices (project_id, direction, firm_name, invoice_number, amount, currency, invoice_date, due_date, paid_date, status, description, notes, created_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.ProjectID, inv.Direction, inv.FirmName, inv.InvoiceNumber, inv.Amount, inv.Currency, inv.InvoiceDate, inv.DueDate, inv.PaidDate, inv.Status, inv.Description, inv.Notes, now)
	if err != nil {
		r.logger.Error("repository.add_project_invoice failed", "project", inv.ProjectID, "error", err)
		return domain.ProjectInvoice{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.ProjectInvoice{}, err
	}
	inv.ID = id
	inv.CreatedAt = parseTime(now)
	return inv, nil
}

func (r *Repository) ListProjectInvoices(ctx context.Context, projectID string) ([]domain.ProjectInvoice, error) {
	rows, err := r.db.QueryContext(ctx,
		`select id, project_id, direction, firm_name, invoice_number, amount, currency, invoice_date, due_date, paid_date, status, description, notes, created_at from project_invoices where project_id = ? order by invoice_date desc, created_at desc`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ProjectInvoice
	for rows.Next() {
		var inv domain.ProjectInvoice
		var createdAt string
		if err := rows.Scan(&inv.ID, &inv.ProjectID, &inv.Direction, &inv.FirmName, &inv.InvoiceNumber, &inv.Amount, &inv.Currency, &inv.InvoiceDate, &inv.DueDate, &inv.PaidDate, &inv.Status, &inv.Description, &inv.Notes, &createdAt); err != nil {
			return nil, err
		}
		inv.CreatedAt = parseTime(createdAt)
		out = append(out, inv)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateProjectInvoice(ctx context.Context, inv domain.ProjectInvoice) error {
	r.logger.Info("repository.update_project_invoice", "id", inv.ID, "status", inv.Status)
	_, err := r.db.ExecContext(ctx,
		`update project_invoices set direction=?, firm_name=?, invoice_number=?, amount=?, currency=?, invoice_date=?, due_date=?, paid_date=?, status=?, description=?, notes=? where id=?`,
		inv.Direction, inv.FirmName, inv.InvoiceNumber, inv.Amount, inv.Currency, inv.InvoiceDate, inv.DueDate, inv.PaidDate, inv.Status, inv.Description, inv.Notes, inv.ID)
	return err
}

func (r *Repository) DeleteProjectInvoice(ctx context.Context, id int64) error {
	r.logger.Info("repository.delete_project_invoice", "id", id)
	_, err := r.db.ExecContext(ctx, `delete from project_invoices where id = ?`, id)
	return err
}

func (r *Repository) CountUnpaidInvoicesByProject(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.QueryContext(ctx,
		`select project_id, count(*) from project_invoices where status != 'paid' group by project_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var projectID string
		var count int
		if err := rows.Scan(&projectID, &count); err != nil {
			return nil, err
		}
		out[projectID] = count
	}
	return out, rows.Err()
}

func (r *Repository) AddFamilyEdge(ctx context.Context, edge domain.FamilyEdge) error {
	r.logger.Info("repository.add_family_edge", "project", edge.ProjectID, "parent", edge.ParentNumber, "child", edge.ChildNumber)
	_, err := r.db.ExecContext(ctx,
		`insert into patent_family (project_id, parent_number, child_number, relation_type, created_at)
		 values (?, ?, ?, ?, ?)
		 on conflict(project_id, parent_number, child_number) do update set relation_type = excluded.relation_type`,
		edge.ProjectID, edge.ParentNumber, edge.ChildNumber, edge.RelationType, nowString())
	return err
}

func (r *Repository) ListFamilyEdges(ctx context.Context, projectID, number string) (parents []domain.FamilyEdge, children []domain.FamilyEdge, err error) {
	rows, err := r.db.QueryContext(ctx,
		`select project_id, parent_number, child_number, relation_type, created_at
		 from patent_family
		 where project_id = ? and (parent_number = ? or child_number = ?)
		 order by created_at`,
		projectID, number, number)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var e domain.FamilyEdge
		var createdAt string
		if err := rows.Scan(&e.ProjectID, &e.ParentNumber, &e.ChildNumber, &e.RelationType, &createdAt); err != nil {
			return nil, nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if e.ChildNumber == number {
			parents = append(parents, e)
		} else {
			children = append(children, e)
		}
	}
	return parents, children, rows.Err()
}

func (r *Repository) RemoveFamilyEdge(ctx context.Context, projectID, parentNumber, childNumber string) error {
	r.logger.Info("repository.remove_family_edge", "project", projectID, "parent", parentNumber, "child", childNumber)
	_, err := r.db.ExecContext(ctx,
		`delete from patent_family where project_id = ? and parent_number = ? and child_number = ?`,
		projectID, parentNumber, childNumber)
	return err
}

func (r *Repository) ListAllFamilyEdges(ctx context.Context, projectID string) ([]domain.FamilyEdge, error) {
	rows, err := r.db.QueryContext(ctx,
		`select project_id, parent_number, child_number, relation_type, created_at from patent_family where project_id = ? order by created_at asc`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.FamilyEdge
	for rows.Next() {
		var e domain.FamilyEdge
		var createdAt string
		if err := rows.Scan(&e.ProjectID, &e.ParentNumber, &e.ChildNumber, &e.RelationType, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) PurgeIgnored(ctx context.Context, projectID string) (int, error) {
	r.logger.Info("repository.purge_ignored", "project", projectID)
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res1, err := tx.ExecContext(ctx, `delete from citation_edges where project_id = ? and status = 'ignored'`, projectID)
	if err != nil {
		return 0, err
	}
	n1, _ := res1.RowsAffected()

	res2, err := tx.ExecContext(ctx, `delete from project_patents where project_id = ? and status = 'ignored'`, projectID)
	if err != nil {
		return 0, err
	}
	n2, _ := res2.RowsAffected()

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	_, _ = r.db.ExecContext(ctx, `VACUUM`)
	r.logger.Info("repository.purge_ignored ok", "project", projectID, "deleted", n1+n2)
	return int(n1 + n2), nil
}

func (r *Repository) Compact(ctx context.Context) error {
	r.logger.Info("repository.compact")
	_, err := r.db.ExecContext(ctx, `VACUUM`)
	return err
}

// IDS

func (r *Repository) AddIDSEntry(ctx context.Context, entry domain.IDSEntry) (domain.IDSEntry, error) {
	r.logger.Info("repository.add_ids_entry", "project", entry.ProjectID, "patent", entry.PatentNumber)
	now := nowString()
	if entry.Status == "" {
		entry.Status = domain.IDSStatusPending
	}
	res, err := r.db.ExecContext(ctx,
		`insert or ignore into project_ids (project_id, patent_number, kind_code, country_code, in_full, relevant_passages, notes, status, added_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ProjectID, entry.PatentNumber, entry.KindCode, entry.CountryCode, boolToInt(entry.InFull), entry.RelevantPassages, entry.Notes, string(entry.Status), now)
	if err != nil {
		r.logger.Error("repository.add_ids_entry failed", "project", entry.ProjectID, "patent", entry.PatentNumber, "error", err)
		return domain.IDSEntry{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.IDSEntry{}, err
	}
	entry.ID = id
	entry.AddedAt = parseTime(now)
	return entry, nil
}

func (r *Repository) ListIDSEntries(ctx context.Context, projectID string) ([]domain.IDSEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		`select id, project_id, patent_number, coalesce(kind_code,''), coalesce(country_code,''), coalesce(in_full,0), coalesce(relevant_passages,''), notes, status, added_at from project_ids where project_id = ? order by added_at desc`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.IDSEntry
	for rows.Next() {
		var e domain.IDSEntry
		var addedAt string
		var statusStr string
		var inFullInt int
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.PatentNumber, &e.KindCode, &e.CountryCode, &inFullInt, &e.RelevantPassages, &e.Notes, &statusStr, &addedAt); err != nil {
			return nil, err
		}
		e.InFull = inFullInt != 0
		e.Status = domain.IDSStatus(statusStr)
		e.AddedAt = parseTime(addedAt)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateIDSEntryStatus(ctx context.Context, id int64, status domain.IDSStatus) error {
	r.logger.Info("repository.update_ids_status", "id", id, "status", status)
	_, err := r.db.ExecContext(ctx, `update project_ids set status = ? where id = ?`, string(status), id)
	return err
}

func (r *Repository) UpdateIDSEntry(ctx context.Context, entry domain.IDSEntry) error {
	r.logger.Info("repository.update_ids_entry", "id", entry.ID)
	_, err := r.db.ExecContext(ctx,
		`update project_ids set kind_code=?, country_code=?, in_full=?, relevant_passages=?, notes=?, status=? where id=?`,
		entry.KindCode, entry.CountryCode, boolToInt(entry.InFull), entry.RelevantPassages, entry.Notes, string(entry.Status), entry.ID)
	return err
}

func (r *Repository) DeleteIDSEntry(ctx context.Context, id int64) error {
	r.logger.Info("repository.delete_ids_entry", "id", id)
	_, err := r.db.ExecContext(ctx, `delete from project_ids where id = ?`, id)
	return err
}

func (r *Repository) AddIDSNPLEntry(ctx context.Context, entry domain.IDSEntry) (domain.IDSEntry, error) {
	r.logger.Info("repository.add_ids_npl_entry", "project", entry.ProjectID, "author", entry.NPLAuthor)
	now := nowString()
	if entry.Status == "" {
		entry.Status = domain.IDSStatusPending
	}
	res, err := r.db.ExecContext(ctx,
		`insert into project_ids_npl (project_id, npl_author, npl_title, npl_date, npl_publisher, relevant_passages, notes, status, added_at) values (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ProjectID, entry.NPLAuthor, entry.NPLTitle, entry.NPLDate, entry.NPLPublisher, entry.RelevantPassages, entry.Notes, string(entry.Status), now)
	if err != nil {
		return domain.IDSEntry{}, err
	}
	id, _ := res.LastInsertId()
	entry.ID = id
	entry.IsNPL = true
	entry.AddedAt = parseTime(now)
	return entry, nil
}

func (r *Repository) ListIDSNPLEntries(ctx context.Context, projectID string) ([]domain.IDSEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		`select id, project_id, npl_author, npl_title, npl_date, npl_publisher, relevant_passages, notes, status, added_at from project_ids_npl where project_id = ? order by added_at desc`,
		projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.IDSEntry
	for rows.Next() {
		var e domain.IDSEntry
		var addedAt string
	var statusStr string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.NPLAuthor, &e.NPLTitle, &e.NPLDate, &e.NPLPublisher, &e.RelevantPassages, &e.Notes, &statusStr, &addedAt); err != nil {
			return nil, err
		}
		e.Status = domain.IDSStatus(statusStr)
		e.IsNPL = true
		e.AddedAt = parseTime(addedAt)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteIDSNPLEntry(ctx context.Context, id int64) error {
	r.logger.Info("repository.delete_ids_npl_entry", "id", id)
	_, err := r.db.ExecContext(ctx, `delete from project_ids_npl where id = ?`, id)
	return err
}

func (r *Repository) GetIDSMetadata(ctx context.Context, projectID string) (domain.IDSMetadata, error) {
	var m domain.IDSMetadata
	var updatedAt string
	err := r.db.QueryRowContext(ctx,
		`select project_id, app_number, filing_date, first_inventor, examiner_name, art_unit, attorney_docket, updated_at from project_ids_metadata where project_id = ?`,
		projectID).Scan(&m.ProjectID, &m.AppNumber, &m.FilingDate, &m.FirstInventor, &m.ExaminerName, &m.ArtUnit, &m.AttorneyDocket, &updatedAt)
	if err == sql.ErrNoRows {
		m.ProjectID = projectID
		return m, nil
	}
	if err != nil {
		return domain.IDSMetadata{}, err
	}
	m.UpdatedAt = parseTime(updatedAt)
	return m, nil
}

func (r *Repository) SaveIDSMetadata(ctx context.Context, meta domain.IDSMetadata) error {
	now := nowString()
	_, err := r.db.ExecContext(ctx,
		`insert into project_ids_metadata (project_id, app_number, filing_date, first_inventor, examiner_name, art_unit, attorney_docket, updated_at) values (?, ?, ?, ?, ?, ?, ?, ?) on conflict(project_id) do update set app_number=excluded.app_number, filing_date=excluded.filing_date, first_inventor=excluded.first_inventor, examiner_name=excluded.examiner_name, art_unit=excluded.art_unit, attorney_docket=excluded.attorney_docket, updated_at=excluded.updated_at`,
		meta.ProjectID, meta.AppNumber, meta.FilingDate, meta.FirstInventor, meta.ExaminerName, meta.ArtUnit, meta.AttorneyDocket, now)
	return err
}
