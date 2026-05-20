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

// IDSEntry returns one curated IDS entry for a project/patent pair.
func (r *Repo) IDSEntry(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (entry domain.IDSEntry, err error) {
	defer r.observeDuration("ids_entry", time.Now(), &err)
	row := r.reader.QueryRowContext(ctx, `
		SELECT id, kind_code, country_code, in_full, relevant_passages, notes, status, added_at
		FROM project_ids WHERE project_id = ? AND patent_number = ?`,
		string(project), patent.Normalized())
	entry = domain.IDSEntry{Project: project, Patent: patent}
	var (
		status  string
		addedAt string
		inFull  int
	)
	if err := row.Scan(&entry.ID, &entry.KindCode, &entry.CountryCode, &inFull, &entry.RelevantPassages, &entry.Notes, &status, &addedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.IDSEntry{}, store.ErrNotFound
		}
		return domain.IDSEntry{}, fmt.Errorf("store/sqlite: get ids entry %s/%s: %w", project, patent, err)
	}
	entry.InFull = inFull != 0
	entry.Status = domain.IDSEntryStatus(status)
	entry.AddedAt, err = decodeTime(addedAt)
	if err != nil {
		return domain.IDSEntry{}, fmt.Errorf("store/sqlite: decode ids entry time: %w", err)
	}
	return entry, nil
}

// SaveIDSEntry inserts or updates one curated IDS entry.
func (r *Repo) SaveIDSEntry(ctx context.Context, entry domain.IDSEntry) (saved domain.IDSEntry, err error) {
	defer r.observeDuration("save_ids_entry", time.Now(), &err)
	if entry.Project == "" {
		return domain.IDSEntry{}, errors.New("store/sqlite: ids entry project must be set")
	}
	if entry.Patent.IsZero() {
		return domain.IDSEntry{}, errors.New("store/sqlite: ids entry patent must be set")
	}
	if !entry.Status.Valid() {
		entry.Status = domain.IDSEntryPending
	}
	addedAt := entry.AddedAt
	if addedAt.IsZero() {
		addedAt = time.Now().UTC()
	}
	_, err = r.writer.ExecContext(ctx, `
		INSERT INTO project_ids (project_id, patent_number, kind_code, country_code, in_full, relevant_passages, notes, status, added_at)
		VALUES (?,?,?,?,?,?,?,?,?)
		ON CONFLICT(project_id, patent_number) DO UPDATE SET
			kind_code=excluded.kind_code,
			country_code=excluded.country_code,
			in_full=excluded.in_full,
			relevant_passages=excluded.relevant_passages,
			notes=excluded.notes,
			status=excluded.status`,
		string(entry.Project), entry.Patent.Normalized(), entry.KindCode, entry.CountryCode,
		boolToInt(entry.InFull), entry.RelevantPassages, entry.Notes, string(entry.Status), encodeTime(addedAt))
	if err != nil {
		return domain.IDSEntry{}, fmt.Errorf("store/sqlite: save ids entry %s/%s: %w", entry.Project, entry.Patent, err)
	}
	saved, err = r.IDSEntry(ctx, entry.Project, entry.Patent)
	if err != nil {
		return domain.IDSEntry{}, err
	}
	return saved, nil
}

// DeleteIDSEntry removes one curated IDS entry.
func (r *Repo) DeleteIDSEntry(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (err error) {
	defer r.observeDuration("delete_ids_entry", time.Now(), &err)
	_, err = r.writer.ExecContext(ctx,
		`DELETE FROM project_ids WHERE project_id = ? AND patent_number = ?`,
		string(project), patent.Normalized())
	if err != nil {
		return fmt.Errorf("store/sqlite: delete ids entry %s/%s: %w", project, patent, err)
	}
	return nil
}

// ListIDSEntries returns every curated IDS entry for a project.
func (r *Repo) ListIDSEntries(ctx context.Context, project domain.ProjectID) (entries []domain.IDSEntry, err error) {
	defer r.observeDuration("list_ids_entries", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx, `
		SELECT id, patent_number, kind_code, country_code, in_full, relevant_passages, notes, status, added_at
		FROM project_ids WHERE project_id = ? ORDER BY added_at DESC`, string(project))
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list ids entries %s: %w", project, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			entry   domain.IDSEntry
			patent  string
			status  string
			addedAt string
			inFull  int
		)
		if err := rows.Scan(&entry.ID, &patent, &entry.KindCode, &entry.CountryCode, &inFull, &entry.RelevantPassages, &entry.Notes, &status, &addedAt); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan ids entry: %w", err)
		}
		entry.Project = project
		entry.Patent, err = domain.ParsePatentNumber(patent)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: decode ids entry patent %q: %w", patent, err)
		}
		entry.InFull = inFull != 0
		entry.Status = domain.IDSEntryStatus(status)
		entry.AddedAt, err = decodeTime(addedAt)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: decode ids entry time: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: list ids entries %s: %w", project, err)
	}
	return entries, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
