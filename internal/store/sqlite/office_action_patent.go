package sqlite

// office_action_patent.go implements the patent↔office-action review-assignment
// join. An office action behaves like an assignable label on patents (mirrors
// patent_tag in tag.go): a prior-art / reference patent is assigned for review,
// optionally marked reviewed, and released. Rows are keyed by the canonical
// record number, so links follow the patent across re-stamps and are carried to
// the surviving record by MergeRecords (see recordNumberChildTables).

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

// AssignPatentsToOfficeAction links the given patents to an office action for
// review in one transaction, defaulting each new link to OAReviewToReview. An
// existing link is left untouched (its status and assigned date are preserved),
// so re-assigning is idempotent and never resets review progress.
func (r *Repo) AssignPatentsToOfficeAction(ctx context.Context, officeActionID string, patents []domain.PatentNumber, assignedAt time.Time) (err error) {
	defer r.observeDuration("assign_patents_office_action", time.Now(), &err)
	if strings.TrimSpace(officeActionID) == "" {
		return errors.New("store/sqlite: assign needs an office action id")
	}
	if assignedAt.IsZero() {
		assignedAt = time.Now()
	}
	if len(patents) == 0 {
		return nil
	}
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: assign patents tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO patent_office_action (office_action_id, patent_number, status, assigned_at)
		 VALUES (?,?,?,?)
		 ON CONFLICT(patent_number, office_action_id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("store/sqlite: assign patents prepare: %w", err)
	}
	defer stmt.Close()

	encodedAssignedAt := encodeTime(assignedAt)
	for _, patent := range patents {
		if patent.IsZero() {
			continue
		}
		if _, err = stmt.ExecContext(ctx, officeActionID, patent.Normalized(),
			string(domain.OAReviewToReview), encodedAssignedAt); err != nil {
			return fmt.Errorf("store/sqlite: assign patent exec: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: assign patents commit: %w", err)
	}
	return nil
}

// ReleasePatentsFromOfficeAction removes the review links between an office action
// and the given patents in one transaction. Releasing an unassigned patent is a
// no-op.
func (r *Repo) ReleasePatentsFromOfficeAction(ctx context.Context, officeActionID string, patents []domain.PatentNumber) (err error) {
	defer r.observeDuration("release_patents_office_action", time.Now(), &err)
	if strings.TrimSpace(officeActionID) == "" {
		return errors.New("store/sqlite: release needs an office action id")
	}
	if len(patents) == 0 {
		return nil
	}
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: release patents tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx,
		`DELETE FROM patent_office_action WHERE office_action_id = ? AND patent_number = ?`)
	if err != nil {
		return fmt.Errorf("store/sqlite: release patents prepare: %w", err)
	}
	defer stmt.Close()

	for _, patent := range patents {
		if _, err = stmt.ExecContext(ctx, officeActionID, patent.Normalized()); err != nil {
			return fmt.Errorf("store/sqlite: release patent exec: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: release patents commit: %w", err)
	}
	return nil
}

// SetOfficeActionReviewStatus updates the review status of one assigned patent.
// Moving to OAReviewReviewed stamps reviewed_at with now; any other status clears
// it. Returns store.ErrNotFound when the patent is not assigned to the action.
func (r *Repo) SetOfficeActionReviewStatus(ctx context.Context, officeActionID string, patent domain.PatentNumber, status domain.OAReviewStatus, now time.Time) (err error) {
	defer r.observeDuration("set_office_action_review_status", time.Now(), &err)
	if strings.TrimSpace(officeActionID) == "" {
		return errors.New("store/sqlite: review status needs an office action id")
	}
	if !status.Valid() {
		return fmt.Errorf("store/sqlite: invalid review status %q", status)
	}
	reviewedAt := ""
	if status == domain.OAReviewReviewed {
		if now.IsZero() {
			now = time.Now()
		}
		reviewedAt = encodeTime(now)
	}
	res, err := r.writer.ExecContext(ctx,
		`UPDATE patent_office_action SET status = ?, reviewed_at = ?
		 WHERE office_action_id = ? AND patent_number = ?`,
		string(status), reviewedAt, officeActionID, patent.Normalized())
	if err != nil {
		return fmt.Errorf("store/sqlite: set review status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store/sqlite: set review status rows: %w", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// PatentsForOfficeAction returns the review links assigned to one office action,
// newest assignment first. The Patent field is the canonical record number.
func (r *Repo) PatentsForOfficeAction(ctx context.Context, officeActionID string) (out []domain.OfficeActionPatent, err error) {
	defer r.observeDuration("patents_for_office_action", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT patent_number, status, assigned_at, reviewed_at
		 FROM patent_office_action
		 WHERE office_action_id = ?
		 ORDER BY assigned_at DESC, patent_number`, officeActionID)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: patents for office action: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		link, err := scanOfficeActionPatent(rows, officeActionID)
		if err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

// OfficeActionsForPatents returns, for each of the given patents, the review
// links it carries, keyed by the patent's canonical record number
// (PatentNumber.Normalized()). Patents with no assignment are absent from the
// map. Used to annotate the patent list without an N+1 query.
func (r *Repo) OfficeActionsForPatents(ctx context.Context, patents []domain.PatentNumber) (out map[string][]domain.OfficeActionPatent, err error) {
	defer r.observeDuration("office_actions_for_patents", time.Now(), &err)
	out = make(map[string][]domain.OfficeActionPatent)
	if len(patents) == 0 {
		return out, nil
	}
	keys := make([]any, 0, len(patents))
	seen := make(map[string]bool, len(patents))
	for _, p := range patents {
		if p.IsZero() {
			continue
		}
		k := p.Normalized()
		if seen[k] {
			continue
		}
		seen[k] = true
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return out, nil
	}
	marks := make([]string, len(keys))
	for i := range marks {
		marks[i] = "?"
	}
	query := `SELECT office_action_id, patent_number, status, assigned_at, reviewed_at
		 FROM patent_office_action
		 WHERE patent_number IN (` + strings.Join(marks, ",") + `)
		 ORDER BY assigned_at DESC, office_action_id`
	rows, err := r.reader.QueryContext(ctx, query, keys...)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: office actions for patents: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var officeActionID string
		link, err := scanOfficeActionPatentWithOA(rows, &officeActionID)
		if err != nil {
			return nil, err
		}
		key := link.Patent.Normalized()
		out[key] = append(out[key], link)
	}
	return out, rows.Err()
}

// OfficeActionAssignedCounts returns the number of patents assigned to each
// office action in a project, keyed by office action id. Office actions with no
// assignment are absent from the map.
func (r *Repo) OfficeActionAssignedCounts(ctx context.Context, project domain.ProjectID) (out map[string]int, err error) {
	defer r.observeDuration("office_action_assigned_counts", time.Now(), &err)
	out = make(map[string]int)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT poa.office_action_id, COUNT(*)
		 FROM patent_office_action poa
		 JOIN office_action oa ON oa.id = poa.office_action_id
		 WHERE oa.project_id = ?
		 GROUP BY poa.office_action_id`, string(project))
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: office action assigned counts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan assigned count: %w", err)
		}
		out[id] = n
	}
	return out, rows.Err()
}

// scanOfficeActionPatent reads one row of (patent_number, status, assigned_at,
// reviewed_at) into a link stamped with the given office action id.
func scanOfficeActionPatent(rows *sql.Rows, officeActionID string) (domain.OfficeActionPatent, error) {
	var patentNumber, status, assignedAt, reviewedAt string
	if err := rows.Scan(&patentNumber, &status, &assignedAt, &reviewedAt); err != nil {
		return domain.OfficeActionPatent{}, fmt.Errorf("store/sqlite: scan office action patent: %w", err)
	}
	return buildOfficeActionPatent(officeActionID, patentNumber, status, assignedAt, reviewedAt)
}

// scanOfficeActionPatentWithOA reads a row that also carries the office action id
// (the leading column), writing it into officeActionID.
func scanOfficeActionPatentWithOA(rows *sql.Rows, officeActionID *string) (domain.OfficeActionPatent, error) {
	var patentNumber, status, assignedAt, reviewedAt string
	if err := rows.Scan(officeActionID, &patentNumber, &status, &assignedAt, &reviewedAt); err != nil {
		return domain.OfficeActionPatent{}, fmt.Errorf("store/sqlite: scan office action patent: %w", err)
	}
	return buildOfficeActionPatent(*officeActionID, patentNumber, status, assignedAt, reviewedAt)
}

func buildOfficeActionPatent(officeActionID, patentNumber, status, assignedAt, reviewedAt string) (domain.OfficeActionPatent, error) {
	number, err := domain.ParsePatentNumber(patentNumber)
	if err != nil {
		return domain.OfficeActionPatent{}, fmt.Errorf("store/sqlite: parse assigned patent %q: %w", patentNumber, err)
	}
	assigned, err := decodeTime(assignedAt)
	if err != nil {
		return domain.OfficeActionPatent{}, err
	}
	reviewed, err := decodeTime(reviewedAt)
	if err != nil {
		return domain.OfficeActionPatent{}, err
	}
	return domain.OfficeActionPatent{
		OfficeActionID: officeActionID,
		Patent:         number,
		Status:         domain.OAReviewStatus(status),
		AssignedAt:     assigned,
		ReviewedAt:     reviewed,
	}, nil
}
