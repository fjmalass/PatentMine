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
	"patentmine/internal/store"
)

// SaveDraft inserts or updates a draft by its id. Its prose sections and claims
// are persisted into their side tables in the same transaction, replaced
// wholesale (the supplied slices are the new canonical order) — the same
// pattern SaveProject uses for inventors. The office-action link is stored as
// NULL when empty so the foreign key is simply absent.
func (r *Repo) SaveDraft(ctx context.Context, d domain.Draft) (err error) {
	defer r.observeDuration("save_draft", time.Now(), &err)
	if d.ID == "" {
		return errors.New("store/sqlite: cannot save draft with empty id")
	}
	if d.Project == "" {
		return errors.New("store/sqlite: draft requires a project id")
	}
	if !d.Kind.Valid() {
		return fmt.Errorf("store/sqlite: invalid draft kind %q", d.Kind)
	}
	if d.Status == "" {
		d.Status = domain.DraftStatusDraft
	}
	if !d.Status.Valid() {
		return fmt.Errorf("store/sqlite: invalid draft status %q", d.Status)
	}

	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: save draft %s: begin: %w", d.ID, err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var oaID any
	if d.OfficeActionID != "" {
		oaID = d.OfficeActionID
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO draft (id, project_id, kind, title, status, office_action_id, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			project_id=excluded.project_id,
			kind=excluded.kind,
			title=excluded.title,
			status=excluded.status,
			office_action_id=excluded.office_action_id,
			updated_at=excluded.updated_at`,
		string(d.ID), string(d.Project), string(d.Kind), d.Title, string(d.Status),
		oaID, encodeTime(d.CreatedAt), encodeTime(d.UpdatedAt)); err != nil {
		return fmt.Errorf("store/sqlite: save draft %s: %w", d.ID, err)
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM draft_section WHERE draft_id = ?`, string(d.ID)); err != nil {
		return fmt.Errorf("store/sqlite: replace draft sections: %w", err)
	}
	for i, s := range d.Sections {
		pinned, mErr := json.Marshal(s.Pinned)
		if mErr != nil {
			return fmt.Errorf("store/sqlite: encode pinned: %w", mErr)
		}
		humanEdited := 0
		if s.HumanEdited {
			humanEdited = 1
		}
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO draft_section
			 (draft_id, ordinal, kind, heading, body, pinned_json, ai_provider, ai_model, generated_at, human_edited)
			 VALUES (?,?,?,?,?,?,?,?,?,?)`,
			string(d.ID), i, string(s.Kind), s.Heading, s.Body, string(pinned),
			s.AIProvider, s.AIModel, encodeTime(s.GeneratedAt), humanEdited); err != nil {
			return fmt.Errorf("store/sqlite: insert draft section %d: %w", i, err)
		}
	}

	if _, err = tx.ExecContext(ctx, `DELETE FROM draft_claim WHERE draft_id = ?`, string(d.ID)); err != nil {
		return fmt.Errorf("store/sqlite: replace draft claims: %w", err)
	}
	for i, c := range d.Claims {
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO draft_claim
			 (draft_id, ordinal, number, claim_type, depends_on, status, base_text, text)
			 VALUES (?,?,?,?,?,?,?,?)`,
			string(d.ID), i, c.Number, string(c.Type), c.DependsOn,
			string(statusOrDefault(c.Status)), c.BaseText, c.Text); err != nil {
			return fmt.Errorf("store/sqlite: insert draft claim %d: %w", i, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: save draft %s: commit: %w", d.ID, err)
	}
	return nil
}

func statusOrDefault(s domain.AmendmentStatus) domain.AmendmentStatus {
	if s == "" {
		return domain.AmendOriginal
	}
	return s
}

const draftColumns = `id, project_id, kind, title, status, office_action_id, created_at, updated_at`

// Draft returns one draft with its sections and claims, or store.ErrNotFound.
func (r *Repo) Draft(ctx context.Context, id domain.DraftID) (d domain.Draft, err error) {
	defer r.observeDuration("draft", time.Now(), &err)
	row := r.reader.QueryRowContext(ctx, `SELECT `+draftColumns+` FROM draft WHERE id = ?`, string(id))
	d, err = scanDraft(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Draft{}, store.ErrNotFound
	}
	if err != nil {
		return domain.Draft{}, fmt.Errorf("store/sqlite: get draft %s: %w", id, err)
	}
	if d.Sections, err = r.draftSections(ctx, id); err != nil {
		return domain.Draft{}, err
	}
	if d.Claims, err = r.draftClaims(ctx, id); err != nil {
		return domain.Draft{}, err
	}
	return d, nil
}

// ListDrafts returns every draft of a project as summaries (no sections/claims
// loaded), newest first.
func (r *Repo) ListDrafts(ctx context.Context, project domain.ProjectID) (out []domain.Draft, err error) {
	defer r.observeDuration("list_drafts", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT `+draftColumns+` FROM draft WHERE project_id = ? ORDER BY updated_at DESC`, string(project))
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list drafts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		d, err := scanDraft(rows)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: scan draft: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: list drafts: %w", err)
	}
	return out, nil
}

// DeleteDraft removes a draft and (by cascade) its sections and claims.
func (r *Repo) DeleteDraft(ctx context.Context, id domain.DraftID) (err error) {
	defer r.observeDuration("delete_draft", time.Now(), &err)
	_, err = r.writer.ExecContext(ctx, `DELETE FROM draft WHERE id = ?`, string(id))
	if err != nil {
		return fmt.Errorf("store/sqlite: delete draft %s: %w", id, err)
	}
	return nil
}

func scanDraft(s rowScanner) (domain.Draft, error) {
	var (
		d         domain.Draft
		id        string
		project   string
		kind      string
		status    string
		oaID      sql.NullString
		createdAt string
		updatedAt string
	)
	if err := s.Scan(&id, &project, &kind, &d.Title, &status, &oaID, &createdAt, &updatedAt); err != nil {
		return domain.Draft{}, err
	}
	d.ID = domain.DraftID(id)
	d.Project = domain.ProjectID(project)
	d.Kind = domain.DraftKind(kind)
	d.Status = domain.DraftStatus(status)
	d.OfficeActionID = oaID.String
	created, err := decodeTime(createdAt)
	if err != nil {
		return domain.Draft{}, err
	}
	d.CreatedAt = created
	updated, err := decodeTime(updatedAt)
	if err != nil {
		return domain.Draft{}, err
	}
	d.UpdatedAt = updated
	return d, nil
}

func (r *Repo) draftSections(ctx context.Context, id domain.DraftID) ([]domain.DraftSection, error) {
	rows, err := r.reader.QueryContext(ctx,
		`SELECT ordinal, kind, heading, body, pinned_json, ai_provider, ai_model, generated_at, human_edited
		 FROM draft_section WHERE draft_id = ? ORDER BY ordinal`, string(id))
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list draft sections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.DraftSection
	for rows.Next() {
		var (
			s           domain.DraftSection
			kind        string
			pinnedJSON  string
			generatedAt string
			humanEdited int
		)
		if err := rows.Scan(&s.Ordinal, &kind, &s.Heading, &s.Body, &pinnedJSON,
			&s.AIProvider, &s.AIModel, &generatedAt, &humanEdited); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan draft section: %w", err)
		}
		s.Kind = domain.SectionKind(kind)
		s.HumanEdited = humanEdited != 0
		if pinnedJSON != "" {
			if err := json.Unmarshal([]byte(pinnedJSON), &s.Pinned); err != nil {
				return nil, fmt.Errorf("store/sqlite: decode pinned: %w", err)
			}
		}
		if generatedAt != "" {
			t, err := decodeTime(generatedAt)
			if err != nil {
				return nil, err
			}
			s.GeneratedAt = t
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *Repo) draftClaims(ctx context.Context, id domain.DraftID) ([]domain.DraftClaim, error) {
	rows, err := r.reader.QueryContext(ctx,
		`SELECT ordinal, number, claim_type, depends_on, status, base_text, text
		 FROM draft_claim WHERE draft_id = ? ORDER BY ordinal`, string(id))
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list draft claims: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.DraftClaim
	for rows.Next() {
		var (
			c         domain.DraftClaim
			claimType string
			status    string
		)
		if err := rows.Scan(&c.Ordinal, &c.Number, &claimType, &c.DependsOn, &status, &c.BaseText, &c.Text); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan draft claim: %w", err)
		}
		c.Type = domain.ClaimType(claimType)
		c.Status = domain.AmendmentStatus(status)
		out = append(out, c)
	}
	return out, rows.Err()
}

const officeActionColumns = `id, project_id, application_number, mail_date, oa_type, examiner, art_unit,
	blob_path, blob_hash, extracted_text, notes, response_due, status, source, imported_at, name, last_opened_at`

// SaveOfficeAction inserts or updates an office action by its id.
func (r *Repo) SaveOfficeAction(ctx context.Context, oa domain.OfficeAction) (err error) {
	defer r.observeDuration("save_office_action", time.Now(), &err)
	if oa.ID == "" {
		return errors.New("store/sqlite: cannot save office action with empty id")
	}
	if oa.Project == "" {
		return errors.New("store/sqlite: office action requires a project id")
	}
	if oa.Type != "" && !oa.Type.Valid() {
		return fmt.Errorf("store/sqlite: invalid office action type %q", oa.Type)
	}
	_, err = r.writer.ExecContext(ctx,
		`INSERT INTO office_action
		 (id, project_id, application_number, mail_date, oa_type, examiner, art_unit,
		  blob_path, blob_hash, extracted_text, notes, response_due, status, source, imported_at, name, last_opened_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			project_id=excluded.project_id,
			application_number=excluded.application_number,
			mail_date=excluded.mail_date,
			oa_type=excluded.oa_type,
			examiner=excluded.examiner,
			art_unit=excluded.art_unit,
			blob_path=excluded.blob_path,
			blob_hash=excluded.blob_hash,
			extracted_text=excluded.extracted_text,
			notes=excluded.notes,
			response_due=excluded.response_due,
			status=excluded.status,
			source=excluded.source,
			imported_at=excluded.imported_at,
			name=excluded.name,
			last_opened_at=excluded.last_opened_at`,
		oa.ID, string(oa.Project), oa.ApplicationNumber, encodeTime(oa.MailDate), string(oa.Type),
		oa.Examiner, oa.ArtUnit, oa.BlobPath, oa.BlobHash, oa.ExtractedText, oa.Notes,
		encodeTime(oa.ResponseDue), string(oa.Status), oa.Source, encodeTime(oa.ImportedAt), oa.Name, encodeTime(oa.LastOpenedAt))
	if err != nil {
		return fmt.Errorf("store/sqlite: save office action %s: %w", oa.ID, err)
	}
	return nil
}

// OfficeAction returns one office action, or store.ErrNotFound.
func (r *Repo) OfficeAction(ctx context.Context, id string) (oa domain.OfficeAction, err error) {
	defer r.observeDuration("office_action", time.Now(), &err)
	row := r.reader.QueryRowContext(ctx, `SELECT `+officeActionColumns+` FROM office_action WHERE id = ?`, id)
	oa, err = scanOfficeAction(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.OfficeAction{}, store.ErrNotFound
	}
	if err != nil {
		return domain.OfficeAction{}, fmt.Errorf("store/sqlite: get office action %s: %w", id, err)
	}
	return oa, nil
}

// ListOfficeActions returns every office action of a project, newest mail date first.
func (r *Repo) ListOfficeActions(ctx context.Context, project domain.ProjectID) (out []domain.OfficeAction, err error) {
	defer r.observeDuration("list_office_actions", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT `+officeActionColumns+` FROM office_action WHERE project_id = ? ORDER BY mail_date DESC`, string(project))
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list office actions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		oa, err := scanOfficeAction(rows)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: scan office action: %w", err)
		}
		out = append(out, oa)
	}
	return out, rows.Err()
}

// DeleteOfficeAction removes an office action. Drafts that referenced it have
// their office_action_id reset to NULL by the foreign key.
func (r *Repo) DeleteOfficeAction(ctx context.Context, id string) (err error) {
	defer r.observeDuration("delete_office_action", time.Now(), &err)
	_, err = r.writer.ExecContext(ctx, `DELETE FROM office_action WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store/sqlite: delete office action %s: %w", id, err)
	}
	return nil
}

func scanOfficeAction(s rowScanner) (domain.OfficeAction, error) {
	var (
		oa           domain.OfficeAction
		id           string
		project      string
		oaType       string
		mailDate     string
		responseDue  string
		status       string
		importedAt   string
		lastOpenedAt string
	)
	if err := s.Scan(&id, &project, &oa.ApplicationNumber, &mailDate, &oaType, &oa.Examiner, &oa.ArtUnit,
		&oa.BlobPath, &oa.BlobHash, &oa.ExtractedText, &oa.Notes, &responseDue, &status, &oa.Source, &importedAt, &oa.Name, &lastOpenedAt); err != nil {
		return domain.OfficeAction{}, err
	}
	oa.ID = id
	oa.Project = domain.ProjectID(project)
	oa.Type = domain.OAType(oaType)
	oa.Status = domain.OAStatus(status)
	md, err := decodeTime(mailDate)
	if err != nil {
		return domain.OfficeAction{}, err
	}
	oa.MailDate = md
	rd, err := decodeTime(responseDue)
	if err != nil {
		return domain.OfficeAction{}, err
	}
	oa.ResponseDue = rd
	ia, err := decodeTime(importedAt)
	if err != nil {
		return domain.OfficeAction{}, err
	}
	oa.ImportedAt = ia
	lo, err := decodeTime(lastOpenedAt)
	if err == nil {
		oa.LastOpenedAt = lo
	}
	return oa, nil
}

const matterDocumentColumns = `id, project_id, office_action_id, kind, display_name,
	blob_path, blob_hash, extracted_text, added_at, last_opened_at`

// SaveMatterDocument inserts or updates one matter document by its id.
func (r *Repo) SaveMatterDocument(ctx context.Context, d domain.MatterDocument) (err error) {
	defer r.observeDuration("save_matter_document", time.Now(), &err)
	if d.ID == "" {
		return errors.New("store/sqlite: cannot save matter document with empty id")
	}
	if d.Project == "" {
		return errors.New("store/sqlite: matter document requires a project id")
	}
	if d.Kind != "" && !d.Kind.Valid() {
		return fmt.Errorf("store/sqlite: invalid matter document kind %q", d.Kind)
	}
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: save matter document %s: begin: %w", d.ID, err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO matter_document
		 (id, project_id, office_action_id, kind, display_name, blob_path, blob_hash, extracted_text, added_at, last_opened_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			project_id=excluded.project_id,
			office_action_id=excluded.office_action_id,
			kind=excluded.kind,
			display_name=excluded.display_name,
			blob_path=excluded.blob_path,
			blob_hash=excluded.blob_hash,
			extracted_text=excluded.extracted_text,
			added_at=excluded.added_at,
			last_opened_at=excluded.last_opened_at`,
		d.ID, string(d.Project), d.OfficeActionID, string(d.Kind), d.DisplayName,
		d.BlobPath, d.BlobHash, d.ExtractedText, encodeTime(d.AddedAt), encodeTime(d.LastOpenedAt))
	if err != nil {
		return fmt.Errorf("store/sqlite: save matter document %s: %w", d.ID, err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM matter_document_office_action WHERE document_id = ?`, d.ID); err != nil {
		return fmt.Errorf("store/sqlite: replace matter document office actions: %w", err)
	}
	for _, oaID := range matterDocumentOfficeActionIDs(d) {
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO matter_document_office_action (document_id, office_action_id, created_at)
			 VALUES (?,?,?) ON CONFLICT(document_id, office_action_id) DO NOTHING`,
			d.ID, oaID, encodeTime(time.Now().UTC())); err != nil {
			return fmt.Errorf("store/sqlite: save matter document office action: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: save matter document %s: commit: %w", d.ID, err)
	}
	return nil
}

// MatterDocument returns one matter document, or store.ErrNotFound.
func (r *Repo) MatterDocument(ctx context.Context, id string) (d domain.MatterDocument, err error) {
	defer r.observeDuration("matter_document", time.Now(), &err)
	row := r.reader.QueryRowContext(ctx, `SELECT `+matterDocumentColumns+` FROM matter_document WHERE id = ?`, id)
	d, err = scanMatterDocument(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.MatterDocument{}, store.ErrNotFound
	}
	if err != nil {
		return domain.MatterDocument{}, fmt.Errorf("store/sqlite: get matter document %s: %w", id, err)
	}
	d.Tags, err = r.MatterDocumentTags(ctx, d.Project, d.ID)
	if err != nil {
		return domain.MatterDocument{}, err
	}
	d.OfficeActionIDs, err = r.MatterDocumentOfficeActions(ctx, d.ID)
	if err != nil {
		return domain.MatterDocument{}, err
	}
	if d.OfficeActionID == "" && len(d.OfficeActionIDs) > 0 {
		d.OfficeActionID = d.OfficeActionIDs[0]
	}
	return d, nil
}

// ListMatterDocuments returns a project's documents, newest first.
func (r *Repo) ListMatterDocuments(ctx context.Context, project domain.ProjectID) (out []domain.MatterDocument, err error) {
	defer r.observeDuration("list_matter_documents", time.Now(), &err)
	var rows *sql.Rows
	if project == "" {
		rows, err = r.reader.QueryContext(ctx,
			`SELECT `+matterDocumentColumns+` FROM matter_document ORDER BY added_at DESC`)
	} else {
		rows, err = r.reader.QueryContext(ctx,
			`SELECT `+matterDocumentColumns+` FROM matter_document WHERE project_id = ? ORDER BY added_at DESC`, string(project))
	}
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list matter documents: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		d, err := scanMatterDocument(rows)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: scan matter document: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out))
	for _, d := range out {
		ids = append(ids, d.ID)
	}
	tags, err := r.MatterDocumentTagsByDocument(ctx, project, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Tags = tags[out[i].ID]
	}
	oas, err := r.MatterDocumentOfficeActionsByDocument(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].OfficeActionIDs = oas[out[i].ID]
		if out[i].OfficeActionID == "" && len(out[i].OfficeActionIDs) > 0 {
			out[i].OfficeActionID = out[i].OfficeActionIDs[0]
		}
	}
	return out, nil
}

// RenameMatterDocument updates only the display name of one matter document.
func (r *Repo) RenameMatterDocument(ctx context.Context, id, name string) (err error) {
	defer r.observeDuration("rename_matter_document", time.Now(), &err)
	res, err := r.writer.ExecContext(ctx,
		`UPDATE matter_document SET display_name = ? WHERE id = ?`, name, id)
	if err != nil {
		return fmt.Errorf("store/sqlite: rename matter document %s: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// DeleteMatterDocument removes one matter document row. The blob on disk is left
// for the engine to unlink (it owns the filesystem layout).
func (r *Repo) DeleteMatterDocument(ctx context.Context, id string) (err error) {
	defer r.observeDuration("delete_matter_document", time.Now(), &err)
	_, err = r.writer.ExecContext(ctx, `DELETE FROM matter_document WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store/sqlite: delete matter document %s: %w", id, err)
	}
	return nil
}

func scanMatterDocument(s rowScanner) (domain.MatterDocument, error) {
	var (
		d            domain.MatterDocument
		id           string
		project      string
		kind         string
		addedAt      string
		lastOpenedAt string
	)
	if err := s.Scan(&id, &project, &d.OfficeActionID, &kind, &d.DisplayName,
		&d.BlobPath, &d.BlobHash, &d.ExtractedText, &addedAt, &lastOpenedAt); err != nil {
		return domain.MatterDocument{}, err
	}
	d.ID = id
	d.Project = domain.ProjectID(project)
	d.Kind = domain.MatterDocKind(kind)
	if d.OfficeActionID != "" {
		d.OfficeActionIDs = []string{d.OfficeActionID}
	}
	at, err := decodeTime(addedAt)
	if err != nil {
		return domain.MatterDocument{}, err
	}
	d.AddedAt = at
	lo, err := decodeTime(lastOpenedAt)
	if err == nil {
		d.LastOpenedAt = lo
	}
	return d, nil
}

func matterDocumentOfficeActionIDs(d domain.MatterDocument) []string {
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	add(d.OfficeActionID)
	for _, id := range d.OfficeActionIDs {
		add(id)
	}
	return out
}

// MatterDocumentOfficeActions returns office-action ids assigned to one preparation document.
func (r *Repo) MatterDocumentOfficeActions(ctx context.Context, documentID string) (out []string, err error) {
	defer r.observeDuration("matter_document_office_actions", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT office_action_id FROM matter_document_office_action WHERE document_id = ? ORDER BY created_at, office_action_id`, documentID)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list matter document office actions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// MatterDocumentOfficeActionsByDocument returns office-action ids for multiple preparation documents keyed by id.
func (r *Repo) MatterDocumentOfficeActionsByDocument(ctx context.Context, documentIDs []string) (out map[string][]string, err error) {
	defer r.observeDuration("matter_document_office_actions_by_document", time.Now(), &err)
	out = make(map[string][]string, len(documentIDs))
	for _, id := range documentIDs {
		ids, err := r.MatterDocumentOfficeActions(ctx, id)
		if err != nil {
			return nil, err
		}
		out[id] = ids
	}
	return out, nil
}

const matterEventColumns = `id, project_id, office_action_id, kind, party,
	occurred_at, due_at, summary, comment, created_at`

// SaveMatterEvent inserts or updates one communications-log entry by its id.
func (r *Repo) SaveMatterEvent(ctx context.Context, e domain.MatterEvent) (err error) {
	defer r.observeDuration("save_matter_event", time.Now(), &err)
	if e.ID == "" {
		return errors.New("store/sqlite: cannot save matter event with empty id")
	}
	if e.Project == "" {
		return errors.New("store/sqlite: matter event requires a project id")
	}
	if e.Kind != "" && !e.Kind.Valid() {
		return fmt.Errorf("store/sqlite: invalid matter event kind %q", e.Kind)
	}
	_, err = r.writer.ExecContext(ctx,
		`INSERT INTO matter_event
		 (id, project_id, office_action_id, kind, party, occurred_at, due_at, summary, comment, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			project_id=excluded.project_id,
			office_action_id=excluded.office_action_id,
			kind=excluded.kind,
			party=excluded.party,
			occurred_at=excluded.occurred_at,
			due_at=excluded.due_at,
			summary=excluded.summary,
			comment=excluded.comment,
			created_at=excluded.created_at`,
		e.ID, string(e.Project), e.OfficeActionID, string(e.Kind), e.Party,
		encodeTime(e.OccurredAt), encodeTime(e.DueAt), e.Summary, e.Comment, encodeTime(e.CreatedAt))
	if err != nil {
		return fmt.Errorf("store/sqlite: save matter event %s: %w", e.ID, err)
	}
	return nil
}

// ListMatterEvents returns a project's communications-log entries, newest first.
func (r *Repo) ListMatterEvents(ctx context.Context, project domain.ProjectID) (out []domain.MatterEvent, err error) {
	defer r.observeDuration("list_matter_events", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT `+matterEventColumns+` FROM matter_event WHERE project_id = ? ORDER BY occurred_at DESC`, string(project))
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list matter events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		e, err := scanMatterEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: scan matter event: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteMatterEvent removes one communications-log entry.
func (r *Repo) DeleteMatterEvent(ctx context.Context, id string) (err error) {
	defer r.observeDuration("delete_matter_event", time.Now(), &err)
	_, err = r.writer.ExecContext(ctx, `DELETE FROM matter_event WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store/sqlite: delete matter event %s: %w", id, err)
	}
	return nil
}

func scanMatterEvent(s rowScanner) (domain.MatterEvent, error) {
	var (
		e          domain.MatterEvent
		id         string
		project    string
		kind       string
		occurredAt string
		dueAt      string
		createdAt  string
	)
	if err := s.Scan(&id, &project, &e.OfficeActionID, &kind, &e.Party,
		&occurredAt, &dueAt, &e.Summary, &e.Comment, &createdAt); err != nil {
		return domain.MatterEvent{}, err
	}
	e.ID = id
	e.Project = domain.ProjectID(project)
	e.Kind = domain.MatterEventKind(kind)
	for dst, src := range map[*time.Time]string{&e.OccurredAt: occurredAt, &e.DueAt: dueAt, &e.CreatedAt: createdAt} {
		t, err := decodeTime(src)
		if err != nil {
			return domain.MatterEvent{}, err
		}
		*dst = t
	}
	return e, nil
}

const timeEntryColumns = `id, project_id, office_action_id, activity, source,
	started_at, ended_at, seconds, validated, validated_at, note, created_at, updated_at`

// SaveTimeEntry inserts or updates one time entry by its id.
func (r *Repo) SaveTimeEntry(ctx context.Context, e domain.TimeEntry) (err error) {
	defer r.observeDuration("save_time_entry", time.Now(), &err)
	if e.ID == "" {
		return errors.New("store/sqlite: cannot save time entry with empty id")
	}
	if e.Project == "" {
		return errors.New("store/sqlite: time entry requires a project id")
	}
	if e.Activity != "" && !e.Activity.Valid() {
		return fmt.Errorf("store/sqlite: invalid time activity %q", e.Activity)
	}
	validated := 0
	if e.Validated {
		validated = 1
	}
	_, err = r.writer.ExecContext(ctx,
		`INSERT INTO time_entry
		 (id, project_id, office_action_id, activity, source, started_at, ended_at, seconds, validated, validated_at, note, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			project_id=excluded.project_id,
			office_action_id=excluded.office_action_id,
			activity=excluded.activity,
			source=excluded.source,
			started_at=excluded.started_at,
			ended_at=excluded.ended_at,
			seconds=excluded.seconds,
			validated=excluded.validated,
			validated_at=excluded.validated_at,
			note=excluded.note,
			updated_at=excluded.updated_at`,
		e.ID, string(e.Project), e.OfficeActionID, string(e.Activity), string(e.Source),
		encodeTime(e.StartedAt), encodeTime(e.EndedAt), e.Seconds, validated,
		encodeTime(e.ValidatedAt), e.Note, encodeTime(e.CreatedAt), encodeTime(e.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store/sqlite: save time entry %s: %w", e.ID, err)
	}
	return nil
}

// TimeEntry returns one time entry, or store.ErrNotFound.
func (r *Repo) TimeEntry(ctx context.Context, id string) (e domain.TimeEntry, err error) {
	defer r.observeDuration("time_entry", time.Now(), &err)
	row := r.reader.QueryRowContext(ctx, `SELECT `+timeEntryColumns+` FROM time_entry WHERE id = ?`, id)
	e, err = scanTimeEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.TimeEntry{}, store.ErrNotFound
	}
	if err != nil {
		return domain.TimeEntry{}, fmt.Errorf("store/sqlite: get time entry %s: %w", id, err)
	}
	return e, nil
}

// ListTimeEntries returns a project's time entries, newest first.
func (r *Repo) ListTimeEntries(ctx context.Context, project domain.ProjectID) ([]domain.TimeEntry, error) {
	return r.queryTimeEntries(ctx,
		`SELECT `+timeEntryColumns+` FROM time_entry WHERE project_id = ? ORDER BY created_at DESC`, string(project))
}

// ListUnvalidatedTime returns a project's unvalidated time entries, oldest first
// (the order an attorney reviews them).
func (r *Repo) ListUnvalidatedTime(ctx context.Context, project domain.ProjectID) ([]domain.TimeEntry, error) {
	return r.queryTimeEntries(ctx,
		`SELECT `+timeEntryColumns+` FROM time_entry WHERE project_id = ? AND validated = 0 ORDER BY created_at`, string(project))
}

func (r *Repo) queryTimeEntries(ctx context.Context, query string, args ...any) (out []domain.TimeEntry, err error) {
	defer r.observeDuration("list_time_entries", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list time entries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		e, err := scanTimeEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: scan time entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CountUnvalidatedTime returns how many of a project's time entries still await
// validation — the trigger for the validate-on-reopen prompt.
func (r *Repo) CountUnvalidatedTime(ctx context.Context, project domain.ProjectID) (n int, err error) {
	defer r.observeDuration("count_unvalidated_time", time.Now(), &err)
	err = r.reader.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM time_entry WHERE project_id = ? AND validated = 0`, string(project)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store/sqlite: count unvalidated time: %w", err)
	}
	return n, nil
}

// DeleteTimeEntry removes one time entry.
func (r *Repo) DeleteTimeEntry(ctx context.Context, id string) (err error) {
	defer r.observeDuration("delete_time_entry", time.Now(), &err)
	_, err = r.writer.ExecContext(ctx, `DELETE FROM time_entry WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store/sqlite: delete time entry %s: %w", id, err)
	}
	return nil
}

// SummarizeTime aggregates a project's recorded time by activity and review state.
func (r *Repo) SummarizeTime(ctx context.Context, project domain.ProjectID) (s domain.TimeSummary, err error) {
	defer r.observeDuration("summarize_time", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT activity, validated, COALESCE(SUM(seconds),0), COUNT(*)
		 FROM time_entry WHERE project_id = ? GROUP BY activity, validated`, string(project))
	if err != nil {
		return domain.TimeSummary{}, fmt.Errorf("store/sqlite: summarize time: %w", err)
	}
	defer func() { _ = rows.Close() }()
	s.Seconds = make(map[domain.TimeActivity]int)
	for rows.Next() {
		var (
			activity  string
			validated int
			secs      int
			count     int
		)
		if err := rows.Scan(&activity, &validated, &secs, &count); err != nil {
			return domain.TimeSummary{}, fmt.Errorf("store/sqlite: scan time summary: %w", err)
		}
		s.Seconds[domain.TimeActivity(activity)] += secs
		if validated == 1 {
			s.ValidatedSecs += secs
		} else {
			s.UnvalidatedSecs += secs
			s.UnvalidatedCount += count
		}
	}
	return s, rows.Err()
}

func scanTimeEntry(s rowScanner) (domain.TimeEntry, error) {
	var (
		e           domain.TimeEntry
		id          string
		project     string
		activity    string
		source      string
		startedAt   string
		endedAt     string
		validated   int
		validatedAt string
		createdAt   string
		updatedAt   string
	)
	if err := s.Scan(&id, &project, &e.OfficeActionID, &activity, &source,
		&startedAt, &endedAt, &e.Seconds, &validated, &validatedAt, &e.Note, &createdAt, &updatedAt); err != nil {
		return domain.TimeEntry{}, err
	}
	e.ID = id
	e.Project = domain.ProjectID(project)
	e.Activity = domain.TimeActivity(activity)
	e.Source = domain.TimeSource(source)
	e.Validated = validated == 1
	for dst, src := range map[*time.Time]string{
		&e.StartedAt: startedAt, &e.EndedAt: endedAt,
		&e.ValidatedAt: validatedAt, &e.CreatedAt: createdAt, &e.UpdatedAt: updatedAt,
	} {
		t, err := decodeTime(src)
		if err != nil {
			return domain.TimeEntry{}, err
		}
		*dst = t
	}
	return e, nil
}

// SaveAIUsage records one AI call against a matter.
func (r *Repo) SaveAIUsage(ctx context.Context, u domain.AIUsage) (err error) {
	defer r.observeDuration("save_ai_usage", time.Now(), &err)
	if u.ID == "" {
		return errors.New("store/sqlite: cannot save ai usage with empty id")
	}
	if u.Project == "" {
		return errors.New("store/sqlite: ai usage requires a project id")
	}
	_, err = r.writer.ExecContext(ctx,
		`INSERT INTO ai_usage
		 (id, project_id, office_action_id, draft_id, provider, model, prompts, prompt_tokens, completion_tokens, total_tokens, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		u.ID, string(u.Project), u.OfficeActionID, u.DraftID, u.Provider, u.Model,
		u.Prompts, u.PromptTokens, u.CompletionTokens, u.TotalTokens, encodeTime(u.CreatedAt))
	if err != nil {
		return fmt.Errorf("store/sqlite: save ai usage %s: %w", u.ID, err)
	}
	return nil
}

// SummarizeAIUsage aggregates a project's AI usage.
func (r *Repo) SummarizeAIUsage(ctx context.Context, project domain.ProjectID) (s domain.AIUsageSummary, err error) {
	defer r.observeDuration("summarize_ai_usage", time.Now(), &err)
	err = r.reader.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0), COALESCE(SUM(total_tokens),0)
		 FROM ai_usage WHERE project_id = ?`, string(project)).
		Scan(&s.Calls, &s.PromptTokens, &s.CompletionTokens, &s.TotalTokens)
	if err != nil {
		return domain.AIUsageSummary{}, fmt.Errorf("store/sqlite: summarize ai usage: %w", err)
	}
	return s, nil
}
