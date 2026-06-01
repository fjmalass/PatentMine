package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"patentmine/internal/domain"
)

// assigneeDateLayouts are the formats an assignment / grant date may arrive in,
// tried in order; the first that parses is reformatted to domain.DateLayout so
// stored effective_date values sort lexically. Centralized here (no inline date
// literals — values come from the domain layout constants).
var assigneeDateLayouts = []string{
	domain.DateLayout,
	"2006-01-02T15:04:05",
	domain.DateTimeLayout,
	domain.CompactDateLayout,
	domain.USDateLayout,
}

// normalizeAssigneeDate reformats a source date string to domain.DateLayout when
// it parses, otherwise returns the trimmed original so nothing is lost.
func normalizeAssigneeDate(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return ""
	}
	if len(t) >= 10 {
		if v, err := time.Parse(time.RFC3339, t); err == nil {
			return v.Format(domain.DateLayout)
		}
	}
	for _, layout := range assigneeDateLayouts {
		if v, err := time.Parse(layout, t); err == nil {
			return v.Format(domain.DateLayout)
		}
	}
	return t
}

// ahRow is a transient assignee-history row assembled before insertion.
type ahRow struct {
	pullType string
	name     string
	effDate  string
	reel     string
	conv     string
	srcOrd   int // parent assignment ordinal; -1 for at_grant
}

// RebuildAssigneeHistoryForNumber resolves a patent's surrogate record id and
// rebuilds its assignee_history. pulledAt is the provenance timestamp recorded on
// the freshly derived rows (domain.DateTimeLayout). A patent with no record row is
// a no-op.
func (r *Repo) RebuildAssigneeHistoryForNumber(ctx context.Context, number domain.PatentNumber, pulledAt string) error {
	var recordID string
	err := r.reader.QueryRowContext(ctx,
		`SELECT id FROM record WHERE number = ?`, number.Normalized()).Scan(&recordID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("store/sqlite: resolve record id for assignee history: %w", err)
	}
	return r.RebuildAssigneeHistory(ctx, recordID, pulledAt)
}

// RebuildAssigneeHistory recomputes assignee_history for one record from the raw
// source tables (record.assignee for the at-grant owner, uspto_assignment /
// uspto_assignment_party for recorded transfers) as a full replacement, in one
// transaction. is_latest is set on every party of the most-recent ownership event.
//
// TODO(assignee-atgrant): the at-grant owner is taken from record.assignee (the
// reconciled projection). Enrich from uspto_grant_party (role=assignee) so joint
// at-grant owners are captured individually.
func (r *Repo) RebuildAssigneeHistory(ctx context.Context, recordID, pulledAt string) (err error) {
	defer r.observeDuration("rebuild_assignee_history", time.Now(), &err)
	if strings.TrimSpace(recordID) == "" {
		return nil
	}

	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: rebuild assignee history: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// At-grant owner from the record projection.
	var grantAssignee, grantDate string
	if err := tx.QueryRowContext(ctx,
		`SELECT assignee, grant_date FROM record WHERE id = ?`, recordID).
		Scan(&grantAssignee, &grantDate); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("store/sqlite: rebuild assignee history: load record: %w", err)
	}

	// Recorded assignments (assignee parties only), across every application that
	// belongs to this record, oldest event first.
	rows, err := tx.QueryContext(ctx, `
		SELECT a.ordinal, a.assignment_recorded_date, a.reel_and_frame_number,
		       a.conveyance_text, p.name_text
		FROM uspto_assignment a
		JOIN uspto_application ua ON ua.application_number = a.application_number
		JOIN uspto_assignment_party p
		  ON p.application_number = a.application_number
		 AND p.assignment_ordinal = a.ordinal
		 AND p.role = 'assignee'
		WHERE ua.record_id = ?
		ORDER BY a.assignment_recorded_date, a.ordinal, p.ordinal`, recordID)
	if err != nil {
		return fmt.Errorf("store/sqlite: rebuild assignee history: load assignments: %w", err)
	}
	var assignments []ahRow
	for rows.Next() {
		var srcOrd int
		var recorded, reel, conv, name string
		if err := rows.Scan(&srcOrd, &recorded, &reel, &conv, &name); err != nil {
			_ = rows.Close()
			return fmt.Errorf("store/sqlite: rebuild assignee history: scan assignment: %w", err)
		}
		if strings.TrimSpace(name) == "" {
			continue
		}
		assignments = append(assignments, ahRow{
			pullType: domain.PullTypeAssignment,
			name:     name,
			effDate:  normalizeAssigneeDate(recorded),
			reel:     reel,
			conv:     conv,
			srcOrd:   srcOrd,
		})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("store/sqlite: rebuild assignee history: iterate assignments: %w", err)
	}
	_ = rows.Close()

	// Assemble the final row set.
	var out []ahRow
	if strings.TrimSpace(grantAssignee) != "" {
		out = append(out, ahRow{
			pullType: domain.PullTypeAtGrant,
			name:     grantAssignee,
			effDate:  normalizeAssigneeDate(grantDate),
			srcOrd:   -1,
		})
	}
	out = append(out, assignments...)

	// Determine the latest ownership event: the assignment with the greatest
	// (effective_date, ordinal). Falls back to the at-grant row when there are no
	// assignments. latestKey identifies the event; every row in it is current.
	latestDate, latestOrd := "", -1
	hasAssignment := false
	for _, a := range assignments {
		hasAssignment = true
		if a.effDate > latestDate || (a.effDate == latestDate && a.srcOrd > latestOrd) {
			latestDate, latestOrd = a.effDate, a.srcOrd
		}
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM assignee_history WHERE record_id = ?`, recordID); err != nil {
		return fmt.Errorf("store/sqlite: rebuild assignee history: clear: %w", err)
	}

	ord := map[string]int{} // next ordinal per pull_type
	for _, row := range out {
		latest := 0
		if hasAssignment {
			if row.pullType == domain.PullTypeAssignment && row.effDate == latestDate && row.srcOrd == latestOrd {
				latest = 1
			}
		} else if row.pullType == domain.PullTypeAtGrant {
			latest = 1
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO assignee_history
			    (record_id, pull_type, ordinal, assignee_name, assignee_norm,
			     effective_date, pulled_at, reel_frame, conveyance_text, is_latest)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			recordID, row.pullType, ord[row.pullType], row.name, domain.NormalizeAssignee(row.name),
			row.effDate, pulledAt, row.reel, row.conv, latest,
		); err != nil {
			return fmt.Errorf("store/sqlite: rebuild assignee history: insert: %w", err)
		}
		ord[row.pullType]++
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: rebuild assignee history: commit: %w", err)
	}
	return nil
}

// AssigneeHistory returns one record's ownership timeline, oldest first, with the
// current owner(s) flagged. number is resolved to the surrogate record id.
func (r *Repo) AssigneeHistory(ctx context.Context, number domain.PatentNumber) (out []domain.AssigneeHistoryEntry, err error) {
	defer r.observeDuration("assignee_history", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx, `
		SELECT ah.record_id, ah.pull_type, ah.ordinal, ah.assignee_name, ah.assignee_norm,
		       ah.effective_date, ah.pulled_at, ah.reel_frame, ah.conveyance_text, ah.is_latest
		FROM assignee_history ah
		JOIN record r ON r.id = ah.record_id
		WHERE r.number = ?
		ORDER BY ah.effective_date, ah.pull_type, ah.ordinal`, number.Normalized())
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list assignee history: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var e domain.AssigneeHistoryEntry
		var latest int
		if err := rows.Scan(&e.RecordID, &e.PullType, &e.Ordinal, &e.AssigneeName, &e.AssigneeNorm,
			&e.EffectiveDate, &e.PulledAt, &e.ReelFrame, &e.ConveyanceText, &latest); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan assignee history: %w", err)
		}
		e.RecordNumber = number
		e.IsLatest = latest != 0
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: iterate assignee history: %w", err)
	}
	return out, nil
}

// ProjectAssignees rolls up assignee ownership across a project's members:
// current owners (deduped, live patents only), every assignee ever (deduped), and
// live/expired/not-fetched patent counts. Grouping is by domain.NormalizeAssignee;
// each group's display name is the most-frequent raw form. A patent is expired
// when its expiration_date is set and earlier than today.
func (r *Repo) ProjectAssignees(ctx context.Context, project domain.ProjectID) (rollup domain.ProjectAssigneeRollup, err error) {
	defer r.observeDuration("project_assignees", time.Now(), &err)

	rows, err := r.reader.QueryContext(ctx, `
		SELECT r.id, r.expiration_date,
		       COALESCE(ah.assignee_name, ''), COALESCE(ah.assignee_norm, ''),
		       COALESCE(ah.effective_date, ''), COALESCE(ah.is_latest, 0)
		FROM membership m
		JOIN record r ON r.number = m.patent_number
		LEFT JOIN assignee_history ah ON ah.record_id = r.id
		WHERE m.project_id = ?`, string(project))
	if err != nil {
		return rollup, fmt.Errorf("store/sqlite: project assignees: %w", err)
	}
	defer func() { _ = rows.Close() }()

	today := time.Now().UTC().Format(domain.DateLayout)
	hasHistory := map[string]bool{}
	expired := map[string]bool{}
	all := newAssigneeAgg()
	current := newAssigneeAgg()

	for rows.Next() {
		var recordID, expDate, name, norm, effDate string
		var latest int
		if err := rows.Scan(&recordID, &expDate, &name, &norm, &effDate, &latest); err != nil {
			return rollup, fmt.Errorf("store/sqlite: scan project assignees: %w", err)
		}
		isExpired := expDate != "" && expDate < today
		expired[recordID] = isExpired
		if norm == "" {
			continue // member with no assignee history yet
		}
		hasHistory[recordID] = true
		all.add(norm, name, recordID, effDate, latest != 0 && !isExpired)
		if latest != 0 && !isExpired {
			current.add(norm, name, recordID, effDate, true)
		}
	}
	if err := rows.Err(); err != nil {
		return rollup, fmt.Errorf("store/sqlite: iterate project assignees: %w", err)
	}

	for recordID, isExpired := range expired {
		if isExpired {
			rollup.ExpiredPatents++
		} else {
			rollup.LivePatents++
		}
		if !hasHistory[recordID] {
			rollup.NotFetched++
		}
	}
	rollup.CurrentOwners = current.groups()
	rollup.AllAssignees = all.groups()
	return rollup, nil
}

// assigneeAgg accumulates assignee groups keyed by normalized name.
type assigneeAgg struct {
	order   []string
	groups_ map[string]*assigneeAggEntry
}

type assigneeAggEntry struct {
	norm     string
	names    map[string]int      // raw name -> frequency, to pick a display label
	patents  map[string]struct{} // distinct record ids
	first    string
	last     string
	current  bool
}

func newAssigneeAgg() *assigneeAgg {
	return &assigneeAgg{groups_: map[string]*assigneeAggEntry{}}
}

func (a *assigneeAgg) add(norm, name, recordID, effDate string, current bool) {
	e := a.groups_[norm]
	if e == nil {
		e = &assigneeAggEntry{norm: norm, names: map[string]int{}, patents: map[string]struct{}{}}
		a.groups_[norm] = e
		a.order = append(a.order, norm)
	}
	if name != "" {
		e.names[name]++
	}
	e.patents[recordID] = struct{}{}
	if effDate != "" {
		if e.first == "" || effDate < e.first {
			e.first = effDate
		}
		if effDate > e.last {
			e.last = effDate
		}
	}
	if current {
		e.current = true
	}
}

// groups returns the accumulated groups sorted by patent count (descending) then
// display name, with the display label resolved to the most-frequent raw name.
func (a *assigneeAgg) groups() []domain.AssigneeGroup {
	out := make([]domain.AssigneeGroup, 0, len(a.order))
	for _, norm := range a.order {
		e := a.groups_[norm]
		display, best := norm, 0
		for name, n := range e.names {
			if n > best || (n == best && name < display) {
				display, best = name, n
			}
		}
		out = append(out, domain.AssigneeGroup{
			Norm:      e.norm,
			Display:   display,
			Patents:   len(e.patents),
			FirstDate: e.first,
			LastDate:  e.last,
			Current:   e.current,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Patents != out[j].Patents {
			return out[i].Patents > out[j].Patents
		}
		return out[i].Display < out[j].Display
	})
	return out
}
