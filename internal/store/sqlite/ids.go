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

// idsSelectColumns is the membership column list an IDS entry is read from.
const idsSelectColumns = `ids_kind_code, ids_in_full,
	ids_relevant_passages, ids_notes, ids_status, ids_added_at, ids_submitted_at`

// IDSEntry returns one curated IDS entry for a project/patent pair. Curated IDS
// data lives inline on the membership row; a membership whose ids_status is
// empty carries no entry and reports store.ErrNotFound.
func (r *Repo) IDSEntry(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (entry domain.IDSEntry, err error) {
	defer r.observeDuration("ids_entry", time.Now(), &err)
	row := r.reader.QueryRowContext(ctx,
		`SELECT rowid, `+idsSelectColumns+`
		 FROM membership WHERE project_id = ? AND patent_number = ?`,
		string(project), patent.Normalized())
	entry, ok, err := scanIDSEntry(row, project, patent)
	if err != nil {
		return domain.IDSEntry{}, err
	}
	if !ok {
		return domain.IDSEntry{}, store.ErrNotFound
	}
	return entry, nil
}

// scanIDSEntry reads the IDS columns of one membership row. ok is false when
// the row exists but carries no IDS entry (empty status).
func scanIDSEntry(s rowScanner, project domain.ProjectID, patent domain.PatentNumber) (domain.IDSEntry, bool, error) {
	entry := domain.IDSEntry{Project: project, Patent: patent}
	var (
		status      string
		addedAt     string
		submittedAt string
		inFull      int
	)
	if err := s.Scan(&entry.ID, &entry.KindCode, &inFull,
		&entry.RelevantPassages, &entry.Notes, &status, &addedAt, &submittedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.IDSEntry{}, false, nil
		}
		return domain.IDSEntry{}, false, fmt.Errorf("store/sqlite: get ids entry %s/%s: %w", project, patent, err)
	}
	if status == "" {
		return domain.IDSEntry{}, false, nil
	}
	entry.InFull = inFull != 0
	entry.Status = domain.IDSEntryStatus(status)
	when, err := decodeTime(addedAt)
	if err != nil {
		return domain.IDSEntry{}, false, fmt.Errorf("store/sqlite: decode ids entry time: %w", err)
	}
	entry.AddedAt = when
	if submittedAt != "" {
		t, err := decodeTime(submittedAt)
		if err != nil {
			return domain.IDSEntry{}, false, fmt.Errorf("store/sqlite: decode ids submitted_at: %w", err)
		}
		entry.SubmittedAt = t
	}
	return entry, true, nil
}

// SaveIDSEntry inserts or updates one curated IDS entry. The entry is stored on
// the patent's membership row; when the patent is not yet a project member, a
// membership is created for it so the curated data has a home.
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
	submittedAt := entry.SubmittedAt
	if entry.Status == domain.IDSEntrySubmitted && submittedAt.IsZero() {
		submittedAt = time.Now().UTC()
	}
	if entry.Status != domain.IDSEntrySubmitted && entry.Status != domain.IDSEntryAccepted {
		submittedAt = time.Time{}
	}
	_, err = r.writer.ExecContext(ctx, `
		INSERT INTO membership (project_id, patent_number, state, added_at,
			ids_kind_code, ids_in_full, ids_relevant_passages,
			ids_notes, ids_status, ids_added_at, ids_submitted_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(project_id, patent_number) DO UPDATE SET
			ids_kind_code=excluded.ids_kind_code,
			ids_in_full=excluded.ids_in_full,
			ids_relevant_passages=excluded.ids_relevant_passages,
			ids_notes=excluded.ids_notes,
			ids_status=excluded.ids_status,
			ids_added_at=excluded.ids_added_at,
			ids_submitted_at=excluded.ids_submitted_at`,
		string(entry.Project), entry.Patent.Normalized(),
		string(domain.ReviewStateUnknown), encodeTime(time.Now().UTC()),
		entry.KindCode, boolToInt(entry.InFull),
		entry.RelevantPassages, entry.Notes, string(entry.Status),
		encodeTime(addedAt), encodeTime(submittedAt))
	if err != nil {
		return domain.IDSEntry{}, fmt.Errorf("store/sqlite: save ids entry %s/%s: %w", entry.Project, entry.Patent, err)
	}
	saved, err = r.IDSEntry(ctx, entry.Project, entry.Patent)
	if err != nil {
		return domain.IDSEntry{}, err
	}
	return saved, nil
}

// DeleteIDSEntry clears the curated IDS data from a membership row. The
// membership itself is left in place.
func (r *Repo) DeleteIDSEntry(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber) (err error) {
	defer r.observeDuration("delete_ids_entry", time.Now(), &err)
	_, err = r.writer.ExecContext(ctx, `
		UPDATE membership SET
			ids_kind_code='', ids_in_full=0,
			ids_relevant_passages='', ids_notes='', ids_status='',
			ids_added_at='', ids_submitted_at=''
		WHERE project_id = ? AND patent_number = ?`,
		string(project), patent.Normalized())
	if err != nil {
		return fmt.Errorf("store/sqlite: delete ids entry %s/%s: %w", project, patent, err)
	}
	return nil
}

// ListIDSEntries returns every curated IDS entry for a project — the
// memberships whose ids_status is set.
func (r *Repo) ListIDSEntries(ctx context.Context, project domain.ProjectID) (entries []domain.IDSEntry, err error) {
	defer r.observeDuration("list_ids_entries", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT rowid, patent_number, `+idsSelectColumns+`
		 FROM membership WHERE project_id = ? AND ids_status != '' ORDER BY ids_added_at DESC`,
		string(project))
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list ids entries %s: %w", project, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			entry       domain.IDSEntry
			patent      string
			status      string
			addedAt     string
			submittedAt string
			inFull      int
		)
		if err := rows.Scan(&entry.ID, &patent, &entry.KindCode, &inFull,
			&entry.RelevantPassages, &entry.Notes, &status, &addedAt, &submittedAt); err != nil {
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
		if submittedAt != "" {
			entry.SubmittedAt, err = decodeTime(submittedAt)
			if err != nil {
				return nil, fmt.Errorf("store/sqlite: decode ids submitted_at: %w", err)
			}
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
