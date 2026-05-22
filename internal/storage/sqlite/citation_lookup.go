package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"patentmine/internal/citationlookup"
	"patentmine/internal/domain"
	"patentmine/internal/storage"
)

type citationGraphCacheMeta struct {
	RequestedPatent     string
	ProvenanceSource    string
	ProvenanceVersion   string
	FreshFor            time.Duration
	PartialData         bool
	ForwardPageCap      int
	ForwardPagesFetched int
	ForwardPageCapHit   bool
	LastError           string
}

func (r *Repository) GetCitationGraph(ctx context.Context, projectID domain.ProjectID, number domain.PatentNumber) (domain.CitationGraph, bool, error) {
	graph, ok, err := r.getCitationGraphExact(ctx, projectID, number)
	if err != nil || ok {
		return graph, ok, err
	}
	if alias := citationGraphLookupAlias(number); alias != "" && alias != number {
		return r.getCitationGraphExact(ctx, projectID, alias)
	}
	return domain.CitationGraph{}, false, nil
}

func (r *Repository) getCitationGraphExact(ctx context.Context, projectID domain.ProjectID, number domain.PatentNumber) (domain.CitationGraph, bool, error) {
	var graph domain.CitationGraph
	var refreshedAt, freshUntil, updatedAt string
	var partial, capHit int
	err := r.db.QueryRowContext(ctx, `select project_id, patent_number, requested_patent, status, provenance_source,
		provenance_version, refreshed_at, fresh_until, partial_data, expected_citations, expected_cited_by,
		forward_page_cap, forward_pages_fetched, forward_page_cap_hit, last_error, updated_at
		from patent_graph_cache where project_id = ? and patent_number = ?`, projectID, number).
		Scan(&graph.Cache.ProjectID, &graph.Cache.PatentNumber, &graph.Cache.RequestedPatent, &graph.Cache.Status,
			&graph.Cache.ProvenanceSource, &graph.Cache.ProvenanceVersion, &refreshedAt, &freshUntil, &partial,
			&graph.Cache.ExpectedCitations, &graph.Cache.ExpectedCitedBy, &graph.Cache.ForwardPageCap,
			&graph.Cache.ForwardPagesFetched, &capHit, &graph.Cache.LastError, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CitationGraph{}, false, nil
	}
	if err != nil {
		return domain.CitationGraph{}, false, err
	}
	graph.Cache.RefreshedAt = parseTime(refreshedAt)
	graph.Cache.FreshUntil = parseTime(freshUntil)
	graph.Cache.PartialData = partial != 0
	graph.Cache.ForwardPageCapHit = capHit != 0
	graph.Cache.UpdatedAt = parseTime(updatedAt)

	if patent, err := r.GetPatent(ctx, projectID, number); err == nil {
		graph.Patent = patent
	}
	cited, err := r.ListCitations(ctx, projectID, number, domain.RelationCites, storage.ListCitationsOptions{})
	if err != nil {
		return domain.CitationGraph{}, false, err
	}
	citing, err := r.ListCitations(ctx, projectID, number, domain.RelationCitedBy, storage.ListCitationsOptions{})
	if err != nil {
		return domain.CitationGraph{}, false, err
	}
	graph.Cited = cited
	graph.Citing = citing
	return graph, true, nil
}

func citationGraphLookupAlias(number domain.PatentNumber) domain.PatentNumber {
	value := strings.ToUpper(strings.TrimSpace(string(number)))
	if !strings.HasPrefix(value, "US") || len(value) < 5 {
		return ""
	}
	last := value[len(value)-1]
	prev := value[len(value)-2]
	if last < '0' || last > '9' || prev < 'A' || prev > 'Z' {
		return ""
	}
	base := value[:len(value)-2]
	if len(base) <= 2 {
		return ""
	}
	for _, r := range base[2:] {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return domain.PatentNumber(base)
}

func (r *Repository) EnqueueCitationRefresh(ctx context.Context, projectID domain.ProjectID, number domain.PatentNumber, opts citationlookup.EnqueueOptions) (domain.CitationRefreshJob, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.CitationRefreshJob{}, false, err
	}
	defer tx.Rollback()

	dedupeKey := citationRefreshDedupeKey(projectID, number)
	if job, ok, err := getActiveCitationRefreshJob(ctx, tx, dedupeKey); err != nil {
		return domain.CitationRefreshJob{}, false, err
	} else if ok {
		now := nowString()
		if _, err := tx.ExecContext(ctx, `update citation_refresh_jobs set trace_count = trace_count + 1, updated_at = ? where id = ?`, now, job.ID); err != nil {
			return domain.CitationRefreshJob{}, false, err
		}
		job.TraceCount++
		job.UpdatedAt = parseTime(now)
		return job, false, tx.Commit()
	}

	if opts.NextAttemptAt.IsZero() {
		opts.NextAttemptAt = time.Now().UTC()
	}
	if opts.DesiredDepth == 0 {
		opts.DesiredDepth = domain.CitationRefreshDepthCitations
	}
	now := nowString()
	res, err := tx.ExecContext(ctx, `insert into citation_refresh_jobs
		(project_id, patent_number, dedupe_key, status, retry_count, next_attempt_at, priority, desired_depth, trace_count, last_error, created_at, updated_at)
		values (?, ?, ?, ?, 0, ?, ?, ?, 1, '', ?, ?)`,
		projectID, number, dedupeKey, domain.CitationRefreshJobQueued, opts.NextAttemptAt.UTC().Format(time.RFC3339Nano), opts.Priority, opts.DesiredDepth, now, now)
	if err != nil {
		return domain.CitationRefreshJob{}, false, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.CitationRefreshJob{}, false, err
	}
	job, err := getCitationRefreshJobTx(ctx, tx, id)
	if err != nil {
		return domain.CitationRefreshJob{}, false, err
	}
	return job, true, tx.Commit()
}

func (r *Repository) ClaimNextCitationRefreshJob(ctx context.Context, now time.Time) (domain.CitationRefreshJob, bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.CitationRefreshJob{}, false, err
	}
	defer tx.Rollback()

	var id int64
	err = tx.QueryRowContext(ctx, `select id from citation_refresh_jobs
		where status = ? and next_attempt_at <= ?
		order by priority desc, next_attempt_at asc, created_at asc limit 1`,
		domain.CitationRefreshJobQueued, now.UTC().Format(time.RFC3339Nano)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CitationRefreshJob{}, false, nil
	}
	if err != nil {
		return domain.CitationRefreshJob{}, false, err
	}
	claimedAt := nowString()
	res, err := tx.ExecContext(ctx, `update citation_refresh_jobs set status = ?, claimed_at = ?, updated_at = ? where id = ? and status = ?`,
		domain.CitationRefreshJobRunning, claimedAt, claimedAt, id, domain.CitationRefreshJobQueued)
	if err != nil {
		return domain.CitationRefreshJob{}, false, err
	}
	if changed, _ := res.RowsAffected(); changed != 1 {
		return domain.CitationRefreshJob{}, false, tx.Commit()
	}
	job, err := getCitationRefreshJobTx(ctx, tx, id)
	if err != nil {
		return domain.CitationRefreshJob{}, false, err
	}
	return job, true, tx.Commit()
}

func (r *Repository) UpsertCitationGraphRefresh(ctx context.Context, projectID domain.ProjectID, bundle domain.PatentBundle, meta citationlookup.RefreshMetadata) error {
	if err := r.UpsertPatentBundle(ctx, projectID, bundle); err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := upsertCitationGraphCacheTx(ctx, tx, projectID, bundle, citationGraphCacheMeta{
		RequestedPatent:     meta.RequestedPatent,
		ProvenanceSource:    firstNonEmpty(meta.ProvenanceSource, bundle.Patent.ImportSource),
		ProvenanceVersion:   meta.ProvenanceVersion,
		FreshFor:            meta.FreshFor,
		PartialData:         meta.PartialData,
		ForwardPageCap:      meta.ForwardPageCap,
		ForwardPagesFetched: meta.ForwardPagesFetched,
		ForwardPageCapHit:   meta.ForwardPageCapHit,
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) CompleteCitationRefreshJob(ctx context.Context, id int64) error {
	now := nowString()
	_, err := r.db.ExecContext(ctx, `update citation_refresh_jobs set status = ?, completed_at = ?, updated_at = ? where id = ?`,
		domain.CitationRefreshJobSucceeded, now, now, id)
	return err
}

func (r *Repository) RescheduleCitationRefreshJob(ctx context.Context, id int64, nextAttemptAt time.Time, message string) error {
	now := nowString()
	_, err := r.db.ExecContext(ctx, `update citation_refresh_jobs
		set status = ?, retry_count = retry_count + 1, next_attempt_at = ?, last_error = ?, updated_at = ?
		where id = ?`,
		domain.CitationRefreshJobQueued, nextAttemptAt.UTC().Format(time.RFC3339Nano), message, now, id)
	return err
}

func (r *Repository) FailCitationRefreshJob(ctx context.Context, id int64, message string) error {
	now := nowString()
	_, err := r.db.ExecContext(ctx, `update citation_refresh_jobs set status = ?, last_error = ?, completed_at = ?, updated_at = ? where id = ?`,
		domain.CitationRefreshJobFailed, message, now, now, id)
	return err
}

func (r *Repository) GetCitationRefreshJob(ctx context.Context, id int64) (domain.CitationRefreshJob, error) {
	return getCitationRefreshJob(ctx, r.db, id)
}

func (r *Repository) CitationLookupStats(ctx context.Context, now time.Time) (domain.CitationLookupStats, error) {
	var stats domain.CitationLookupStats
	rows, err := r.db.QueryContext(ctx, `select status, count(*) from citation_refresh_jobs group by status`)
	if err != nil {
		return domain.CitationLookupStats{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return domain.CitationLookupStats{}, err
		}
		switch status {
		case domain.CitationRefreshJobQueued:
			stats.QueuedJobs = count
		case domain.CitationRefreshJobRunning:
			stats.RunningJobs = count
		case domain.CitationRefreshJobSucceeded:
			stats.SucceededJobs = count
		case domain.CitationRefreshJobFailed:
			stats.FailedJobs = count
		}
	}
	if err := rows.Err(); err != nil {
		return domain.CitationLookupStats{}, err
	}

	var oldestQueued string
	err = r.db.QueryRowContext(ctx, `select coalesce(min(created_at), '') from citation_refresh_jobs where status = ?`, domain.CitationRefreshJobQueued).Scan(&oldestQueued)
	if err != nil {
		return domain.CitationLookupStats{}, err
	}
	if t := parseTime(oldestQueued); !t.IsZero() && now.After(t) {
		stats.OldestQueuedAge = now.Sub(t)
	}

	var oldestRefresh string
	err = r.db.QueryRowContext(ctx, `select coalesce(min(refreshed_at), '') from patent_graph_cache where refreshed_at != ''`).Scan(&oldestRefresh)
	if err != nil {
		return domain.CitationLookupStats{}, err
	}
	if t := parseTime(oldestRefresh); !t.IsZero() && now.After(t) {
		stats.OldestRefreshAge = now.Sub(t)
	}

	err = r.db.QueryRowContext(ctx, `select coalesce(sum(case when trace_count > 1 then trace_count - 1 else 0 end), 0) from citation_refresh_jobs`).Scan(&stats.CoalescedTraceCount)
	if err != nil {
		return domain.CitationLookupStats{}, err
	}
	err = r.db.QueryRowContext(ctx, `select count(*) from citation_refresh_jobs where last_error like '%429%' or last_error like '%Too Many%'`).Scan(&stats.Upstream429Count)
	if err != nil {
		return domain.CitationLookupStats{}, err
	}
	return stats, nil
}

func upsertCitationGraphCacheTx(ctx context.Context, tx *sql.Tx, projectID domain.ProjectID, bundle domain.PatentBundle, meta citationGraphCacheMeta) error {
	number := bundle.Patent.Number
	if number == "" {
		return fmt.Errorf("patent number is required")
	}
	now := time.Now().UTC()
	freshFor := meta.FreshFor
	if freshFor <= 0 {
		freshFor = 24 * time.Hour
	}
	expectedCitations := bundle.Patent.ExpectedCitations
	if bundle.ExpectedCitations != 0 || expectedCitations < 0 {
		expectedCitations = bundle.ExpectedCitations
	}
	expectedCitedBy := bundle.Patent.ExpectedCitedBy
	if bundle.ExpectedCitedBy != 0 || expectedCitedBy < 0 {
		expectedCitedBy = bundle.ExpectedCitedBy
	}
	_, err := tx.ExecContext(ctx, `insert into patent_graph_cache
		(project_id, patent_number, requested_patent, status, provenance_source, provenance_version, refreshed_at, fresh_until,
		 partial_data, expected_citations, expected_cited_by, forward_page_cap, forward_pages_fetched, forward_page_cap_hit, last_error, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(project_id, patent_number) do update set
			requested_patent = excluded.requested_patent,
			status = excluded.status,
			provenance_source = excluded.provenance_source,
			provenance_version = excluded.provenance_version,
			refreshed_at = excluded.refreshed_at,
			fresh_until = excluded.fresh_until,
			partial_data = excluded.partial_data,
			expected_citations = excluded.expected_citations,
			expected_cited_by = excluded.expected_cited_by,
			forward_page_cap = excluded.forward_page_cap,
			forward_pages_fetched = excluded.forward_pages_fetched,
			forward_page_cap_hit = excluded.forward_page_cap_hit,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at`,
		projectID, number, firstNonEmpty(meta.RequestedPatent, string(number)), domain.CitationLookupStatusFresh,
		meta.ProvenanceSource, meta.ProvenanceVersion, now.Format(time.RFC3339Nano), now.Add(freshFor).Format(time.RFC3339Nano),
		boolInt(meta.PartialData), expectedCitations, expectedCitedBy, meta.ForwardPageCap, meta.ForwardPagesFetched,
		boolInt(meta.ForwardPageCapHit), meta.LastError, now.Format(time.RFC3339Nano))
	return err
}

func getActiveCitationRefreshJob(ctx context.Context, tx *sql.Tx, dedupeKey string) (domain.CitationRefreshJob, bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `select id from citation_refresh_jobs
		where dedupe_key = ? and status in (?, ?)
		order by created_at asc limit 1`, dedupeKey, domain.CitationRefreshJobQueued, domain.CitationRefreshJobRunning).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.CitationRefreshJob{}, false, nil
	}
	if err != nil {
		return domain.CitationRefreshJob{}, false, err
	}
	job, err := getCitationRefreshJobTx(ctx, tx, id)
	return job, err == nil, err
}

func getCitationRefreshJob(ctx context.Context, db *sql.DB, id int64) (domain.CitationRefreshJob, error) {
	return scanCitationRefreshJob(db.QueryRowContext(ctx, `select id, project_id, patent_number, dedupe_key, status, retry_count,
		next_attempt_at, priority, desired_depth, trace_count, last_error, created_at, updated_at, claimed_at, completed_at
		from citation_refresh_jobs where id = ?`, id))
}

func getCitationRefreshJobTx(ctx context.Context, tx *sql.Tx, id int64) (domain.CitationRefreshJob, error) {
	return scanCitationRefreshJob(tx.QueryRowContext(ctx, `select id, project_id, patent_number, dedupe_key, status, retry_count,
		next_attempt_at, priority, desired_depth, trace_count, last_error, created_at, updated_at, claimed_at, completed_at
		from citation_refresh_jobs where id = ?`, id))
}

func scanCitationRefreshJob(row interface{ Scan(...any) error }) (domain.CitationRefreshJob, error) {
	var job domain.CitationRefreshJob
	var nextAttemptAt, createdAt, updatedAt, claimedAt, completedAt string
	if err := row.Scan(&job.ID, &job.ProjectID, &job.PatentNumber, &job.DedupeKey, &job.Status, &job.RetryCount,
		&nextAttemptAt, &job.Priority, &job.DesiredDepth, &job.TraceCount, &job.LastError, &createdAt, &updatedAt, &claimedAt, &completedAt); err != nil {
		return domain.CitationRefreshJob{}, err
	}
	job.NextAttemptAt = parseTime(nextAttemptAt)
	job.CreatedAt = parseTime(createdAt)
	job.UpdatedAt = parseTime(updatedAt)
	job.ClaimedAt = parseTime(claimedAt)
	job.CompletedAt = parseTime(completedAt)
	return job, nil
}

func citationRefreshDedupeKey(projectID domain.ProjectID, number domain.PatentNumber) string {
	return strings.ToLower(string(projectID)) + ":" + strings.ToUpper(string(number))
}
