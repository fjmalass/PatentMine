package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	blob_path, blob_hash, extracted_text, notes, source, imported_at`

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
		  blob_path, blob_hash, extracted_text, notes, source, imported_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
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
			source=excluded.source,
			imported_at=excluded.imported_at`,
		oa.ID, string(oa.Project), oa.ApplicationNumber, encodeTime(oa.MailDate), string(oa.Type),
		oa.Examiner, oa.ArtUnit, oa.BlobPath, oa.BlobHash, oa.ExtractedText, oa.Notes, oa.Source, encodeTime(oa.ImportedAt))
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
		oa         domain.OfficeAction
		id         string
		project    string
		oaType     string
		mailDate   string
		importedAt string
	)
	if err := s.Scan(&id, &project, &oa.ApplicationNumber, &mailDate, &oaType, &oa.Examiner, &oa.ArtUnit,
		&oa.BlobPath, &oa.BlobHash, &oa.ExtractedText, &oa.Notes, &oa.Source, &importedAt); err != nil {
		return domain.OfficeAction{}, err
	}
	oa.ID = id
	oa.Project = domain.ProjectID(project)
	oa.Type = domain.OAType(oaType)
	md, err := decodeTime(mailDate)
	if err != nil {
		return domain.OfficeAction{}, err
	}
	oa.MailDate = md
	ia, err := decodeTime(importedAt)
	if err != nil {
		return domain.OfficeAction{}, err
	}
	oa.ImportedAt = ia
	return oa, nil
}
