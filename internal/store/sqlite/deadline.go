package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"context"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

const deadlineColumns = `id, kind, patent_number, project_id, office_action_id,
	title, window_opens, due_date, grace_ends, status, created_at, updated_at`

// SaveDeadline inserts or updates one tracked deadline by its id.
func (r *Repo) SaveDeadline(ctx context.Context, d domain.Deadline) (err error) {
	defer r.observeDuration("save_deadline", time.Now(), &err)
	if d.ID == "" {
		return errors.New("store/sqlite: cannot save deadline with empty id")
	}
	if d.Kind != "" && !d.Kind.Valid() {
		return fmt.Errorf("store/sqlite: invalid deadline kind %q", d.Kind)
	}
	_, err = r.writer.ExecContext(ctx,
		`INSERT INTO deadline
		 (id, kind, patent_number, project_id, office_action_id, title, window_opens, due_date, grace_ends, status, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			kind=excluded.kind,
			patent_number=excluded.patent_number,
			project_id=excluded.project_id,
			office_action_id=excluded.office_action_id,
			title=excluded.title,
			window_opens=excluded.window_opens,
			due_date=excluded.due_date,
			grace_ends=excluded.grace_ends,
			status=excluded.status,
			updated_at=excluded.updated_at`,
		d.ID, string(d.Kind), d.PatentNumber, string(d.Project), d.OfficeActionID, d.Title,
		encodeTime(d.WindowOpens), encodeTime(d.DueDate), encodeTime(d.GraceEnds),
		string(d.Status), encodeTime(d.CreatedAt), encodeTime(d.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store/sqlite: save deadline %s: %w", d.ID, err)
	}
	return nil
}

// Deadline returns one deadline, or store.ErrNotFound.
func (r *Repo) Deadline(ctx context.Context, id string) (d domain.Deadline, err error) {
	defer r.observeDuration("deadline", time.Now(), &err)
	row := r.reader.QueryRowContext(ctx, `SELECT `+deadlineColumns+` FROM deadline WHERE id = ?`, id)
	d, err = scanDeadline(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Deadline{}, store.ErrNotFound
	}
	if err != nil {
		return domain.Deadline{}, fmt.Errorf("store/sqlite: get deadline %s: %w", id, err)
	}
	return d, nil
}

// ListPendingDeadlines returns every pending deadline, soonest due first.
func (r *Repo) ListPendingDeadlines(ctx context.Context) (out []domain.Deadline, err error) {
	defer r.observeDuration("list_pending_deadlines", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT `+deadlineColumns+` FROM deadline WHERE status = 'pending' ORDER BY due_date`)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list pending deadlines: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		d, err := scanDeadline(rows)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: scan deadline: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ListDeadlinesForPatent returns a patent's deadlines, soonest due first.
func (r *Repo) ListDeadlinesForPatent(ctx context.Context, patentNumber string) (out []domain.Deadline, err error) {
	defer r.observeDuration("list_deadlines_for_patent", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT `+deadlineColumns+` FROM deadline WHERE patent_number = ? ORDER BY due_date`, patentNumber)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list deadlines for patent: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		d, err := scanDeadline(rows)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: scan deadline: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// SetDeadlineStatus marks a deadline done/dismissed/pending.
func (r *Repo) SetDeadlineStatus(ctx context.Context, id string, status domain.DeadlineStatus) (err error) {
	defer r.observeDuration("set_deadline_status", time.Now(), &err)
	if !status.Valid() {
		return fmt.Errorf("store/sqlite: invalid deadline status %q", status)
	}
	res, err := r.writer.ExecContext(ctx,
		`UPDATE deadline SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), encodeTime(time.Now().UTC()), id)
	if err != nil {
		return fmt.Errorf("store/sqlite: set deadline status %s: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// DeleteDeadlinesForPatent removes a patent's deadlines (used when re-deriving
// the maintenance schedule so it never double-tracks).
func (r *Repo) DeleteDeadlinesForPatent(ctx context.Context, patentNumber string, kind domain.DeadlineKind) (err error) {
	defer r.observeDuration("delete_deadlines_for_patent", time.Now(), &err)
	_, err = r.writer.ExecContext(ctx,
		`DELETE FROM deadline WHERE patent_number = ? AND kind = ?`, patentNumber, string(kind))
	if err != nil {
		return fmt.Errorf("store/sqlite: delete deadlines for patent %s: %w", patentNumber, err)
	}
	return nil
}

func scanDeadline(s rowScanner) (domain.Deadline, error) {
	var (
		d           domain.Deadline
		id          string
		kind        string
		project     string
		status      string
		windowOpens string
		dueDate     string
		graceEnds   string
		createdAt   string
		updatedAt   string
	)
	if err := s.Scan(&id, &kind, &d.PatentNumber, &project, &d.OfficeActionID, &d.Title,
		&windowOpens, &dueDate, &graceEnds, &status, &createdAt, &updatedAt); err != nil {
		return domain.Deadline{}, err
	}
	d.ID = id
	d.Kind = domain.DeadlineKind(kind)
	d.Project = domain.ProjectID(project)
	d.Status = domain.DeadlineStatus(status)
	for dst, src := range map[*time.Time]string{
		&d.WindowOpens: windowOpens, &d.DueDate: dueDate, &d.GraceEnds: graceEnds,
		&d.CreatedAt: createdAt, &d.UpdatedAt: updatedAt,
	} {
		t, err := decodeTime(src)
		if err != nil {
			return domain.Deadline{}, err
		}
		*dst = t
	}
	return d, nil
}

// ReminderSent reports whether a reminder for subject at threshold via channel
// was already sent — the dedupe behind the reminder engine.
func (r *Repo) ReminderSent(ctx context.Context, subject string, thresholdDays int, channel string) (sent bool, err error) {
	defer r.observeDuration("reminder_sent", time.Now(), &err)
	var n int
	err = r.reader.QueryRowContext(ctx,
		`SELECT count(*) FROM reminder_log WHERE subject = ? AND threshold_days = ? AND channel = ?`,
		subject, thresholdDays, channel).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store/sqlite: reminder sent check: %w", err)
	}
	return n > 0, nil
}

// MarkReminderSent records that a reminder for subject at threshold via channel
// was sent, so it is not sent again.
func (r *Repo) MarkReminderSent(ctx context.Context, subject string, thresholdDays int, channel string) (err error) {
	defer r.observeDuration("mark_reminder_sent", time.Now(), &err)
	_, err = r.writer.ExecContext(ctx,
		`INSERT INTO reminder_log (subject, threshold_days, channel, sent_at)
		 VALUES (?,?,?,?)
		 ON CONFLICT(subject, threshold_days, channel) DO UPDATE SET sent_at=excluded.sent_at`,
		subject, thresholdDays, channel, encodeTime(time.Now().UTC()))
	if err != nil {
		return fmt.Errorf("store/sqlite: mark reminder sent: %w", err)
	}
	return nil
}

// ListOpenOfficeActions returns every office action awaiting a response with a
// deadline set, across all projects — the source of synthesized OA-response
// deadlines for the reminder engine and the deadlines view.
func (r *Repo) ListOpenOfficeActions(ctx context.Context) (out []domain.OfficeAction, err error) {
	defer r.observeDuration("list_open_office_actions", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT `+officeActionColumns+` FROM office_action
		 WHERE response_due != '' AND status IN ('', 'open') ORDER BY response_due`)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list open office actions: %w", err)
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
