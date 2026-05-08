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
	stmts := []string{
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
			status text not null default 'stored'
		)`,
		`create table if not exists patent_text_sections (
			patent_number text not null,
			section_type text not null,
			ordinal integer not null,
			text text not null,
			primary key (patent_number, section_type, ordinal),
			foreign key (patent_number) references patents(number)
		)`,
		fmt.Sprintf(`create table if not exists citation_edges (
			source_patent text not null,
			target_patent text not null,
			relation_type text not null,
			status text not null default '%s',
			created_at text not null default '',
			refreshed_at text not null default '',
			labeled_at text not null default '',
			primary key (source_patent, target_patent, relation_type)
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
			patent_number text not null,
			body text not null,
			created_at text not null,
			foreign key (patent_number) references patents(number)
		)`,
		`create table if not exists reference_entries (
			id integer primary key autoincrement,
			patent_number text not null unique,
			citation_label text not null,
			created_at text not null,
			foreign key (patent_number) references patents(number)
		)`,
		`create table if not exists ai_artifacts (
			id integer primary key autoincrement,
			patent_number text not null,
			artifact_type text not null,
			compared_patent_number text not null,
			provider text not null,
			body text not null,
			created_at text not null,
			foreign key (patent_number) references patents(number)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return r.migrate(ctx)
}

func (r *Repository) migrate(ctx context.Context) error {
	hasExpirationDate, err := r.hasColumn(ctx, "patents", "expiration_date")
	if err != nil {
		return err
	}
	if !hasExpirationDate {
		if _, err := r.db.ExecContext(ctx, `alter table patents add column expiration_date text not null default ''`); err != nil {
			return err
		}
	}
	hasExpirationEstimated, err := r.hasColumn(ctx, "patents", "expiration_estimated")
	if err != nil {
		return err
	}
	if !hasExpirationEstimated {
		if _, err := r.db.ExecContext(ctx, `alter table patents add column expiration_estimated integer not null default 0`); err != nil {
			return err
		}
	}
	hasPatentCreatedAt, err := r.hasColumn(ctx, "patents", "created_at")
	if err != nil {
		return err
	}
	if !hasPatentCreatedAt {
		if _, err := r.db.ExecContext(ctx, `alter table patents add column created_at text not null default ''`); err != nil {
			return err
		}
	}
	hasPatentStatus, err := r.hasColumn(ctx, "patents", "status")
	if err != nil {
		return err
	}
	if !hasPatentStatus {
		if _, err := r.db.ExecContext(ctx, `alter table patents add column status text not null default 'stored'`); err != nil {
			return err
		}
	}
	for _, column := range []struct {
		name string
		stmt string
	}{
		{name: "status", stmt: fmt.Sprintf(`alter table citation_edges add column status text not null default '%s'`, domain.CitationStatusUnderReview)},
		{name: "created_at", stmt: `alter table citation_edges add column created_at text not null default ''`},
		{name: "refreshed_at", stmt: `alter table citation_edges add column refreshed_at text not null default ''`},
		{name: "labeled_at", stmt: `alter table citation_edges add column labeled_at text not null default ''`},
	} {
		hasColumn, err := r.hasColumn(ctx, "citation_edges", column.name)
		if err != nil {
			return err
		}
		if !hasColumn {
			if _, err := r.db.ExecContext(ctx, column.stmt); err != nil {
				return err
			}
		}
	}
	migratedAt := nowString()
	if _, err := r.db.ExecContext(ctx, `update patents set created_at = ? where created_at = ''`, migratedAt); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `update citation_edges set status = ? where status = ''`, domain.CitationStatusUnderReview); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `update citation_edges set created_at = ? where created_at = ''`, migratedAt); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `update citation_edges set refreshed_at = ? where refreshed_at = ''`, migratedAt); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `update citation_edges set labeled_at = created_at where labeled_at = '' and created_at != ''`); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `update citation_edges set labeled_at = ? where labeled_at = ''`, migratedAt); err != nil {
		return err
	}
	return nil
}

func (r *Repository) hasTable(ctx context.Context, table string) (bool, error) {
	var name string
	err := r.db.QueryRowContext(ctx, `select name from sqlite_master where type='table' and name=?`, table).Scan(&name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *Repository) hasColumn(ctx context.Context, table, column string) (bool, error) {
	rows, err := r.db.QueryContext(ctx, `pragma table_info(`+table+`)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (r *Repository) UpsertPatentBundle(ctx context.Context, bundle domain.PatentBundle) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := upsertPatent(ctx, tx, bundle.Patent); err != nil {
		return err
	}
	if err := replaceTexts(ctx, tx, bundle.Patent.Number, bundle.Sections); err != nil {
		return err
	}
	if err := replaceCitations(ctx, tx, bundle.Patent.Number, bundle.Citations); err != nil {
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
		if _, err := tx.ExecContext(ctx, `insert into reference_entries (patent_number, citation_label, created_at)
			values (?, ?, ?)
			on conflict(patent_number) do update set citation_label = excluded.citation_label`,
			firstNonEmpty(ref.PatentNumber, bundle.Patent.Number), label, nowString()); err != nil {
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
	status := p.Status
	if status == "" {
		status = domain.CitationStatusStored
	}
	_, err = tx.ExecContext(ctx, `insert into patents
		(number, title, abstract, assignee, inventors_json, publication_date, grant_date, expiration_date, expiration_estimated, source_url, created_at, status)
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
			status = excluded.status`,
		p.Number, p.Title, p.Abstract, p.Assignee, string(inventors), p.PublicationDate, p.GrantDate, p.ExpirationDate, boolInt(p.ExpirationEstimated), p.SourceURL, createdAt.Format(time.RFC3339), status)
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

func replaceCitations(ctx context.Context, tx *sql.Tx, number string, edges []domain.CitationEdge) error {
	refreshedAt := nowString()
	for _, edge := range edges {
		source := firstNonEmpty(edge.SourcePatent, number)
		status := firstNonEmpty(edge.Status, domain.CitationStatusUnderReview)
		if _, err := tx.ExecContext(ctx, `insert into citation_edges (source_patent, target_patent, relation_type, status, created_at, refreshed_at, labeled_at)
			values (?, ?, ?, ?, ?, ?, ?)
			on conflict(source_patent, target_patent, relation_type) do update set
				refreshed_at = excluded.refreshed_at`,
			source, edge.TargetPatent, edge.RelationType, status, refreshedAt, refreshedAt, refreshedAt); err != nil {
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

func (r *Repository) GetPatent(ctx context.Context, number string) (domain.Patent, error) {
	row := r.db.QueryRowContext(ctx, `
		select 
			p.number, p.title, p.abstract, p.assignee, p.inventors_json, p.publication_date, p.grant_date, p.expiration_date, p.expiration_estimated, p.source_url, p.created_at, p.status,
			group_concat(c.code, ', ') as classification_label
		from patents p
		left join patent_classifications c on p.number = c.patent_number
		where p.number = ?
		group by p.number`, number)
	return scanPatent(row)
}

func (r *Repository) ListPatents(ctx context.Context, filter string) ([]domain.Patent, error) {
	args := []any{}
	query := `
		select 
			p.number, p.title, p.abstract, p.assignee, p.inventors_json, p.publication_date, p.grant_date, p.expiration_date, p.expiration_estimated, p.source_url, p.created_at, p.status,
			group_concat(c.code, ', ') as classification_label
		from patents p
		left join patent_classifications c on p.number = c.patent_number
		where 1=1`
	if strings.TrimSpace(filter) != "" {
		query += ` and lower(p.number || ' ' || p.title || ' ' || p.abstract || ' ' || p.assignee || ' ' || p.inventors_json || ' ' || p.publication_date || ' ' || p.grant_date || ' ' || p.expiration_date || ' ' || p.source_url) like ?`
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(filter))+"%")
	}
	query += ` group by p.number order by p.number`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPatents(rows)
}

func (r *Repository) ListCitations(ctx context.Context, number, relationType string) ([]domain.CitationEdge, error) {
	query := `
		select 
			ce.source_patent, ce.target_patent, ce.relation_type, ce.status, ce.created_at, ce.refreshed_at, ce.labeled_at,
			p.title, p.inventors_json, p.expiration_date
		from citation_edges ce
		left join patents p on ce.target_patent = p.number
		where ce.source_patent = ? and ce.relation_type = ? 
		order by ce.target_patent`
	rows, err := r.db.QueryContext(ctx, query, number, relationType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCitationEdges(rows)
}

func (r *Repository) ListCitationsByStatus(ctx context.Context, status string) ([]domain.CitationEdge, error) {
	query := `
		select 
			ce.source_patent, ce.target_patent, ce.relation_type, ce.status, ce.created_at, ce.refreshed_at, ce.labeled_at,
			p.title, p.inventors_json, p.expiration_date
		from citation_edges ce
		left join patents p on ce.target_patent = p.number
		where ce.status = ? 
		order by ce.labeled_at desc, ce.source_patent, ce.target_patent`
	rows, err := r.db.QueryContext(ctx, query, status)
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

func (r *Repository) UpdateCitationStatus(ctx context.Context, edge domain.CitationEdge, status string) error {
	_, err := r.db.ExecContext(ctx, `update citation_edges set status = ?, labeled_at = ? where source_patent = ? and target_patent = ? and relation_type = ?`,
		status, nowString(), edge.SourcePatent, edge.TargetPatent, edge.RelationType)
	return err
}

func (r *Repository) UpdatePatentStatus(ctx context.Context, number string, status string) error {
	_, err := r.db.ExecContext(ctx, `update patents set status = ? where number = ?`, status, number)
	return err
}

func (r *Repository) UpdateClassificationDescription(ctx context.Context, system, code, description string) error {
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

func (r *Repository) DeletePatent(ctx context.Context, number string) error {
	return r.UpdatePatentStatus(ctx, number, domain.CitationStatusIgnored)
}

func (r *Repository) ListClassifications(ctx context.Context, number string) ([]domain.Classification, error) {
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

func (r *Repository) ListTextSections(ctx context.Context, number string) ([]domain.PatentTextSection, error) {
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

func (r *Repository) AddNote(ctx context.Context, number, body string) (domain.ResearchNote, error) {
	created := nowString()
	res, err := r.db.ExecContext(ctx, `insert into research_notes (patent_number, body, created_at) values (?, ?, ?)`, number, body, created)
	if err != nil {
		return domain.ResearchNote{}, err
	}
	id, _ := res.LastInsertId()
	return domain.ResearchNote{ID: id, PatentNumber: number, Body: body, CreatedAt: parseTime(created)}, nil
}

func (r *Repository) ListNotes(ctx context.Context, number string) ([]domain.ResearchNote, error) {
	rows, err := r.db.QueryContext(ctx, `select id, patent_number, body, created_at from research_notes where patent_number = ? order by created_at desc`, number)
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

func (r *Repository) AddReference(ctx context.Context, number, label string) (domain.ReferenceEntry, error) {
	created := nowString()
	_, err := r.db.ExecContext(ctx, `insert into reference_entries (patent_number, citation_label, created_at)
		values (?, ?, ?)
		on conflict(patent_number) do update set citation_label = excluded.citation_label`, number, label, created)
	if err != nil {
		return domain.ReferenceEntry{}, err
	}
	return domain.ReferenceEntry{PatentNumber: number, CitationLabel: label, CreatedAt: parseTime(created)}, nil
}

func (r *Repository) ListReferences(ctx context.Context) ([]domain.ReferenceEntry, error) {
	rows, err := r.db.QueryContext(ctx, `select id, patent_number, citation_label, created_at from reference_entries order by created_at, patent_number`)
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

func (r *Repository) AddAIArtifact(ctx context.Context, artifact domain.AIArtifact) (domain.AIArtifact, error) {
	artifact.CreatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx, `insert into ai_artifacts (patent_number, artifact_type, compared_patent_number, provider, body, created_at) values (?, ?, ?, ?, ?, ?)`,
		artifact.PatentNumber, artifact.ArtifactType, artifact.ComparedPatentNumber, artifact.Provider, artifact.Body, artifact.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return domain.AIArtifact{}, err
	}
	artifact.ID, _ = res.LastInsertId()
	return artifact, nil
}

func (r *Repository) ListAIArtifacts(ctx context.Context, number string) ([]domain.AIArtifact, error) {
	rows, err := r.db.QueryContext(ctx, `select id, patent_number, artifact_type, compared_patent_number, provider, body, created_at from ai_artifacts where patent_number = ? order by created_at desc`, number)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AIArtifact
	for rows.Next() {
		var artifact domain.AIArtifact
		var created string
		if err := rows.Scan(&artifact.ID, &artifact.PatentNumber, &artifact.ArtifactType, &artifact.ComparedPatentNumber, &artifact.Provider, &artifact.Body, &created); err != nil {
			return nil, err
		}
		artifact.CreatedAt = parseTime(created)
		out = append(out, artifact)
	}
	return out, rows.Err()
}

func scanPatent(row interface{ Scan(...any) error }) (domain.Patent, error) {
	var p domain.Patent
	var inventors string
	var expirationEstimated int
	var createdAt string
	var classificationLabel sql.NullString
	if err := row.Scan(&p.Number, &p.Title, &p.Abstract, &p.Assignee, &inventors, &p.PublicationDate, &p.GrantDate, &p.ExpirationDate, &expirationEstimated, &p.SourceURL, &createdAt, &p.Status, &classificationLabel); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Patent{}, fmt.Errorf("patent not found")
		}
		return domain.Patent{}, err
	}
	if err := json.Unmarshal([]byte(inventors), &p.Inventors); err != nil {
		return domain.Patent{}, err
	}
	p.ExpirationEstimated = expirationEstimated == 1
	p.StoredAt = parseTime(createdAt)
	if p.Status == "" {
		p.Status = domain.CitationStatusStored
	}
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
	return time.Now().UTC().Format(time.RFC3339)
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
