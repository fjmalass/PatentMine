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

// SaveProvisionalCoverSheet inserts or updates a project's PTO/SB/16 cover sheet. There is
// one per project (UNIQUE project_id), so a save upserts on that key; the full
// structure is stored in data_json, while title and docket_number are mirrored
// into columns for searching.
func (r *Repo) SaveProvisionalCoverSheet(ctx context.Context, cs domain.ProvisionalCoverSheet) (err error) {
	defer r.observeDuration("save_provisional_cover_sheet", time.Now(), &err)
	if cs.ID == "" {
		return errors.New("store/sqlite: cannot save cover sheet with empty id")
	}
	if cs.Project == "" {
		return errors.New("store/sqlite: cover sheet requires a project id")
	}
	data, err := json.Marshal(cs)
	if err != nil {
		return fmt.Errorf("store/sqlite: encode cover sheet: %w", err)
	}
	approved := 0
	if cs.Approved {
		approved = 1
	}
	_, err = r.writer.ExecContext(ctx,
		`INSERT INTO provisional_cover_sheet (id, project_id, title, docket_number, approved, data_json, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?)
		 ON CONFLICT(project_id) DO UPDATE SET
			title=excluded.title,
			docket_number=excluded.docket_number,
			approved=excluded.approved,
			data_json=excluded.data_json,
			updated_at=excluded.updated_at`,
		cs.ID, string(cs.Project), cs.Title, cs.DocketNumber, approved, string(data),
		encodeTime(cs.CreatedAt), encodeTime(cs.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store/sqlite: save cover sheet %s: %w", cs.ID, err)
	}
	return nil
}

// ProvisionalCoverSheet returns a project's cover sheet, or store.ErrNotFound. The stored
// JSON is the source of truth for every field; the id/created_at are taken from
// the row so a reload reflects the persisted identity.
func (r *Repo) ProvisionalCoverSheet(ctx context.Context, project domain.ProjectID) (cs domain.ProvisionalCoverSheet, err error) {
	defer r.observeDuration("provisional_cover_sheet", time.Now(), &err)
	var (
		id        string
		dataJSON  string
		createdAt string
		updatedAt string
	)
	row := r.reader.QueryRowContext(ctx,
		`SELECT id, data_json, created_at, updated_at FROM provisional_cover_sheet WHERE project_id = ?`, string(project))
	if err = row.Scan(&id, &dataJSON, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ProvisionalCoverSheet{}, store.ErrNotFound
		}
		return domain.ProvisionalCoverSheet{}, fmt.Errorf("store/sqlite: get cover sheet for %s: %w", project, err)
	}
	if dataJSON != "" {
		if err := json.Unmarshal([]byte(dataJSON), &cs); err != nil {
			return domain.ProvisionalCoverSheet{}, fmt.Errorf("store/sqlite: decode cover sheet: %w", err)
		}
	}
	cs.ID = id
	cs.Project = project
	if cs.CreatedAt, err = decodeTime(createdAt); err != nil {
		return domain.ProvisionalCoverSheet{}, err
	}
	if cs.UpdatedAt, err = decodeTime(updatedAt); err != nil {
		return domain.ProvisionalCoverSheet{}, err
	}
	return cs, nil
}
