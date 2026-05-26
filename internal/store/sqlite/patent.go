package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/filterexpr"
	"patentmine/internal/store"
)

// patentColumns is the column list, p-aliased, in scanPatent's order.
const patentColumns = `p.country, p.serial, p.kind, p.title, p.abstract, p.assignee,
	p.inventors, p.fetch_state, p.source, p.application_date, p.publication_date,
	p.grant_date, p.fetched_at, p.display_number,
	p.first_claim, p.expiration_date, p.expiration_source, p.source_url, p.classifications`

// patentRowColumns returns the SELECT column list for scanPatentRow. When
// project is non-empty the membership state and curated IDS columns are
// included; tags are not selected here — ListPatents fills them for the whole
// page in one query (see attachTags) rather than a per-row subquery.
func patentRowColumns(project domain.ProjectID) (cols string, extraArgs []any) {
	relationCounts := `,
		(SELECT COUNT(1) FROM relation rel WHERE
			(rel.from_number = p.number AND rel.kind = 'cites') OR
			(rel.to_number = p.number AND rel.kind = 'cited_by')),
		(SELECT COUNT(1) FROM relation rel WHERE
			(rel.from_number = p.number AND rel.kind = 'cited_by') OR
			(rel.to_number = p.number AND rel.kind = 'cites')),
		(SELECT COUNT(1) FROM relation rel WHERE
			(rel.from_number = p.number AND rel.kind = 'parent') OR
			(rel.to_number = p.number AND rel.kind = 'child'))`
	if project != "" {
		return `p.country, p.serial, p.kind, p.display_number, p.title, ` +
				`p.inventors, p.publication_date, p.expiration_date, p.fetch_state, COALESCE(m.state, ''), '[]', ` +
				`COALESCE(m.ids_kind_code, ''), COALESCE(m.ids_in_full, 0), ` +
				`COALESCE(m.ids_relevant_passages, ''), COALESCE(m.ids_notes, ''), COALESCE(m.ids_status, ''), ` +
				`COALESCE(m.ids_added_at, ''), COALESCE(m.ids_submitted_at, '')` +
				relationCounts + `, p.classifications`,
			nil
	}
	return `p.country, p.serial, p.kind, p.display_number, p.title, ` +
		`p.inventors, p.publication_date, p.expiration_date, p.fetch_state, '', '[]', '', 0, '', '', '', '', ''` +
		relationCounts + `, p.classifications`, nil
}

// SavePatent inserts or updates a patent by its number.
func (r *Repo) SavePatent(ctx context.Context, p domain.Patent) (err error) {
	defer r.observeDuration("save_patent", time.Now(), &err)
	if p.Number.IsZero() {
		return errors.New("store/sqlite: cannot save patent with empty number")
	}
	if !p.FetchState.Valid() {
		return fmt.Errorf("store/sqlite: invalid fetch state %q", p.FetchState)
	}
	args, err := patentUpsertArgs(p)
	if err != nil {
		return err
	}
	_, err = r.writer.ExecContext(ctx, patentUpsertSQL, args...)
	if err != nil {
		return fmt.Errorf("store/sqlite: save patent %s: %w", p.Number, err)
	}
	return nil
}

// DeletePatent permanently removes a patent and all its associated documents,
// relations, and memberships. Deleting the patent row is enough: the ON DELETE
// CASCADE foreign keys on document, patent_tag, membership, and relation remove
// every dependent row.
func (r *Repo) DeletePatent(ctx context.Context, n domain.PatentNumber) (err error) {
	defer r.observeDuration("delete_patent", time.Now(), &err)
	res, err := r.writer.ExecContext(ctx, `DELETE FROM patent WHERE number = ?`, n.Normalized())
	if err != nil {
		return fmt.Errorf("store/sqlite: delete patent %s: %w", n, err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return store.ErrNotFound
	}
	return nil
}

// patentDemoteSQL clears the bibliographic columns of a patent row and sets
// fetch_state to "stub". Identity columns (country/serial/kind/display_number)
// are preserved so the row remains a valid stub referenced by surviving edges.
const patentDemoteSQL = `UPDATE patent SET
	title='', abstract='', assignee='', inventors='[]',
	fetch_state=?, source='',
	application_date=?, publication_date=?, grant_date=?, fetched_at=?,
	first_claim='', expiration_date='', expiration_source='', source_url='',
	classifications='[]', classifications_text=''
WHERE number = ?`

// SoftDeletePatent demotes a patent to FetchStub, dropping its documents,
// memberships, notes, tags, and outbound relations. Inbound relations from
// other patents survive so the family-graph topology stays intact. When no
// inbound relation remains, the row is hard-purged to avoid orphan stubs.
func (r *Repo) SoftDeletePatent(ctx context.Context, n domain.PatentNumber) (err error) {
	defer r.observeDuration("soft_delete_patent", time.Now(), &err)
	return r.softDeletePatents(ctx, []domain.PatentNumber{n})
}

// SoftDeletePatents is the batch variant of SoftDeletePatent. All demotions
// commit together; orphan-stub purging runs after every demote so a cycle of
// mutually-referencing patents deleted in one batch is fully purged.
func (r *Repo) SoftDeletePatents(ctx context.Context, patents []domain.PatentNumber) (err error) {
	defer r.observeDuration("soft_delete_patents", time.Now(), &err)
	return r.softDeletePatents(ctx, patents)
}

func (r *Repo) softDeletePatents(ctx context.Context, patents []domain.PatentNumber) error {
	if len(patents) == 0 {
		return nil
	}
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: soft delete patents tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	zero := encodeTime(time.Time{})
	for _, p := range patents {
		key := p.Normalized()
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM patent WHERE number = ?`, key).Scan(&exists); err != nil {
			return fmt.Errorf("store/sqlite: soft delete check %s: %w", p, err)
		}
		if exists == 0 {
			return store.ErrNotFound
		}
		for _, q := range []string{
			`DELETE FROM document WHERE record_number = ?`,
			`DELETE FROM membership WHERE patent_number = ?`,
			`DELETE FROM patent_tag WHERE patent_number = ?`,
			`DELETE FROM project_patent_note WHERE patent_number = ?`,
			`DELETE FROM relation WHERE from_number = ?`,
		} {
			if _, err := tx.ExecContext(ctx, q, key); err != nil {
				return fmt.Errorf("store/sqlite: soft delete dependents %s: %w", p, err)
			}
		}
		if _, err := tx.ExecContext(ctx, patentDemoteSQL,
			string(domain.FetchStub), zero, zero, zero, zero, key,
		); err != nil {
			return fmt.Errorf("store/sqlite: soft delete demote %s: %w", p, err)
		}
	}

	orphanPurged := int64(0)
	for _, p := range patents {
		key := p.Normalized()
		var inbound int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(1) FROM relation WHERE to_number = ?`, key).Scan(&inbound); err != nil {
			return fmt.Errorf("store/sqlite: soft delete orphan check %s: %w", p, err)
		}
		if inbound > 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM patent WHERE number = ?`, key); err != nil {
			return fmt.Errorf("store/sqlite: soft delete orphan purge %s: %w", p, err)
		}
		orphanPurged++
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: soft delete commit: %w", err)
	}
	committed = true
	if orphanPurged > 0 && r.metrics != nil {
		r.metrics.IncCounter("store.sqlite.soft_delete.orphan_purge_total", orphanPurged)
		r.metrics.IncCounter("store.sqlite.soft_delete.demote_total", int64(len(patents))-orphanPurged)
	} else if r.metrics != nil {
		r.metrics.IncCounter("store.sqlite.soft_delete.demote_total", int64(len(patents)))
	}
	return nil
}

// DeletePatents permanently removes multiple patents and all their associated
// documents, relations, and memberships in a single transaction.
func (r *Repo) DeletePatents(ctx context.Context, patents []domain.PatentNumber) (err error) {
	defer r.observeDuration("delete_patents", time.Now(), &err)
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: delete patents tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.PrepareContext(ctx, `DELETE FROM patent WHERE number = ?`)
	if err != nil {
		return fmt.Errorf("store/sqlite: delete patents prepare: %w", err)
	}
	defer stmt.Close()

	for _, patent := range patents {
		res, execErr := stmt.ExecContext(ctx, patent.Normalized())
		if execErr != nil {
			return fmt.Errorf("store/sqlite: delete patent %s: %w", patent, execErr)
		}
		if rows, _ := res.RowsAffected(); rows == 0 {
			return store.ErrNotFound
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: delete patents commit: %w", err)
	}
	return nil
}

// Patent returns one patent, or store.ErrNotFound.
func (r *Repo) Patent(ctx context.Context, n domain.PatentNumber) (patent domain.Patent, err error) {
	defer r.observeDuration("patent", time.Now(), &err)
	if recNum, err := r.RecordOf(ctx, n); err == nil {
		n = recNum
	}
	row := r.reader.QueryRowContext(ctx,
		`SELECT `+patentColumns+` FROM patent p WHERE p.number = ?`, n.Normalized())
	p, err := scanPatent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Patent{}, store.ErrNotFound
	}
	if err != nil {
		return domain.Patent{}, fmt.Errorf("store/sqlite: get patent %s: %w", n, err)
	}
	// The detail view needs the full life-stage document set.
	docs, err := r.Documents(ctx, p.Number)
	if err != nil {
		return domain.Patent{}, err
	}
	p.Documents = docs
	return p, nil
}

// ListPatents returns one page of lightweight listing rows matching q.
func (r *Repo) ListPatents(ctx context.Context, q store.PatentQuery) (out []domain.PatentRow, err error) {
	defer r.observeDuration("list_patents", time.Now(), &err)
	var tagSortStart time.Time
	if q.SortColumn == domain.SortByTags {
		tagSortStart = time.Now()
		defer func() { r.observeTagSortDuration(tagSortStart, &err, len(out), q) }()
	}
	cols, colArgs := patentRowColumns(q.Project)
	where, whereArgs, err := patentFilter(q)
	if err != nil {
		return nil, err
	}
	orderBy, orderArgs, err := patentSortExpr(q)
	if err != nil {
		return nil, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = store.DefaultPageSize
	}
	query := `SELECT ` + cols + ` FROM patent p` + where +
		` ORDER BY ` + orderBy + ` LIMIT ? OFFSET ?`
	args := append(colArgs, whereArgs...)
	args = append(args, orderArgs...)
	args = append(args, limit, max(q.Offset, 0))

	rows, err := r.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list patents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out = nil
	for rows.Next() {
		p, err := scanPatentRow(rows)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: scan patent row: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: list patents: %w", err)
	}
	if q.Project != "" {
		if err := r.attachTags(ctx, q.Project, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *Repo) observeTagSortDuration(start time.Time, errp *error, rows int, q store.PatentQuery) {
	if r.metrics == nil {
		return
	}
	failed := errp != nil && *errp != nil
	r.metrics.IncCounter("store.sqlite.list_patents.sort_tags_total", 1)
	r.metrics.ObserveDuration("store.sqlite.list_patents.sort_tags", time.Since(start), failed)
	r.metrics.SetGauge("store.sqlite.list_patents.sort_tags.limit", int64(q.Limit))
	r.metrics.SetGauge("store.sqlite.list_patents.sort_tags.offset", int64(q.Offset))
	r.metrics.SetGauge("store.sqlite.list_patents.sort_tags.rows", int64(rows))
}

// attachTags fills the Tags field of every row in one query. It replaces a
// correlated json_group_array subquery that ran once per row, so a 100-row
// page costs one tag query instead of a hundred.
func (r *Repo) attachTags(ctx context.Context, project domain.ProjectID, rows []domain.PatentRow) error {
	if len(rows) == 0 {
		return nil
	}
	args := make([]any, 0, len(rows)+1)
	args = append(args, string(project))
	placeholders := make([]string, len(rows))
	index := make(map[string]int, len(rows))
	for i, row := range rows {
		n := row.Number.Normalized()
		placeholders[i] = "?"
		args = append(args, n)
		index[n] = i
	}
	query := `SELECT pt.patent_number, t.name FROM patent_tag pt
		JOIN tag t ON t.id = pt.tag_id
		WHERE t.project_id = ? AND pt.patent_number IN (` + strings.Join(placeholders, ",") + `)
		ORDER BY t.name`
	res, err := r.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("store/sqlite: attach tags: %w", err)
	}
	defer func() { _ = res.Close() }()
	for res.Next() {
		var number, name string
		if err := res.Scan(&number, &name); err != nil {
			return fmt.Errorf("store/sqlite: scan tag row: %w", err)
		}
		if i, ok := index[number]; ok {
			rows[i].Tags = append(rows[i].Tags, name)
		}
	}
	if err := res.Err(); err != nil {
		return fmt.Errorf("store/sqlite: attach tags: %w", err)
	}
	return nil
}

// scanPatentRow reads one lightweight listing row.
func scanPatentRow(s rowScanner) (domain.PatentRow, error) {
	var (
		row                          domain.PatentRow
		country, serial, kind, shown string
		inventorsJSON, pubDate       string
		expirationDate               string
		fetchState, reviewState      string
		tagsJSON                     string
		idsKindCode                  string
		idsRelevant, idsNotes        string
		idsStatus, idsAddedAt        string
		idsSubmittedAt               string
		idsInFull                    int
		citationsCount, citedByCount int
		parentsCount                 int
		classificationsJSON          string
	)
	if err := s.Scan(&country, &serial, &kind, &shown, &row.Title,
		&inventorsJSON, &pubDate, &expirationDate, &fetchState, &reviewState, &tagsJSON,
		&idsKindCode, &idsInFull, &idsRelevant, &idsNotes, &idsStatus, &idsAddedAt, &idsSubmittedAt,
		&citationsCount, &citedByCount, &parentsCount, &classificationsJSON); err != nil {
		return domain.PatentRow{}, err
	}
	row.Number = domain.PatentNumber{Country: country, Serial: serial, Kind: kind}
	row.FetchState = domain.FetchState(fetchState)
	row.ReviewState = domain.ReviewState(reviewState)
	if t, err := decodeTime(pubDate); err == nil {
		row.PublicationDate = t
	}
	if shown != "" {
		display, err := domain.ParsePatentNumber(shown)
		if err != nil {
			return domain.PatentRow{}, fmt.Errorf("decode display number %q: %w", shown, err)
		}
		row.DisplayNumber = display
	}
	if err := json.Unmarshal([]byte(inventorsJSON), &row.Inventors); err != nil {
		row.Inventors = nil
	}
	if t, err := decodeTime(expirationDate); err == nil {
		row.ExpirationDate = t
	}
	if err := json.Unmarshal([]byte(tagsJSON), &row.Tags); err != nil {
		row.Tags = nil
	}
	if err := json.Unmarshal([]byte(classificationsJSON), &row.Classifications); err != nil {
		row.Classifications = nil
	}
	row.CitationsCount = citationsCount
	row.CitedByCount = citedByCount
	row.ParentsCount = parentsCount
	if idsStatus != "" || idsKindCode != "" || idsRelevant != "" || idsNotes != "" || idsAddedAt != "" || idsInFull != 0 {
		row.IDSEntry = &domain.IDSEntry{
			Patent:           row.Number,
			KindCode:         idsKindCode,
			InFull:           idsInFull != 0,
			RelevantPassages: idsRelevant,
			Notes:            idsNotes,
			Status:           domain.IDSEntryStatus(idsStatus),
		}
		if t, err := decodeTime(idsAddedAt); err == nil {
			row.IDSEntry.AddedAt = t
		}
		if t, err := decodeTime(idsSubmittedAt); err == nil {
			row.IDSEntry.SubmittedAt = t
		}
	}
	return row, nil
}

// patentSortExpr returns an ORDER BY expression for the given query. The RPC
// layer should validate sort support before this runs; this check keeps the
// repository explicit if a caller bypasses that contract.
func patentSortExpr(q store.PatentQuery) (string, []any, error) {
	if q.SortColumn == "" {
		return "p.number ASC", nil, nil
	}
	if !domain.PatentTableAllowsSort(q.Project, q.SortColumn) {
		return "", nil, fmt.Errorf("store/sqlite: unsupported sort column %q for project %q", q.SortColumn, q.Project)
	}
	dir := "ASC"
	if !q.SortAscending {
		dir = "DESC"
	}
	switch q.SortColumn {
	case domain.SortByNumber:
		return "p.number " + dir, nil, nil
	case domain.SortByTitle:
		return "p.title " + dir, nil, nil
	case domain.SortByInventor:
		return "json_extract(p.inventors, '$[0]') " + dir, nil, nil
	case domain.SortByExpires:
		return "p.expiration_date " + dir, nil, nil
	case domain.SortByReviewState:
		if q.Project != "" {
			return "COALESCE(m.state, p.fetch_state) " + dir, nil, nil
		}
		return "p.fetch_state " + dir, nil, nil
	case domain.SortByIDS:
		return "COALESCE(m.ids_status, '') " + dir, nil, nil
	case domain.SortByTags:
		return `COALESCE((SELECT group_concat(name, ' ') FROM (` +
			`SELECT t.name AS name FROM patent_tag pt JOIN tag t ON t.id = pt.tag_id ` +
			`WHERE t.project_id = ? AND pt.patent_number = p.number ORDER BY t.name` +
			`)), '') ` + dir, []any{string(q.Project)}, nil
	case domain.SortByClassification:
		return "json_extract(p.classifications, '$[0]') " + dir, nil, nil
	default:
		return "", nil, fmt.Errorf("store/sqlite: unsupported sort column %q", q.SortColumn)
	}
}

// CountPatents returns the total rows matching q, ignoring its paging.
func (r *Repo) CountPatents(ctx context.Context, q store.PatentQuery) (count int, err error) {
	defer r.observeDuration("count_patents", time.Now(), &err)
	where, args, err := patentFilter(q)
	if err != nil {
		return 0, err
	}
	var n int
	if err := r.reader.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM patent p`+where, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("store/sqlite: count patents: %w", err)
	}
	return n, nil
}

// patentFilter builds the shared FROM-suffix (JOIN + WHERE) for list and count.
func patentFilter(q store.PatentQuery) (string, []any, error) {
	var sb strings.Builder
	var args []any
	var conds []string
	var expr filterexpr.Expr
	if q.Filter != "" {
		var err error
		expr, err = filterexpr.Parse(q.Filter)
		if err != nil {
			return "", nil, err
		}
		if err := filterexpr.ValidateProjectScope(expr, q.Project != ""); err != nil {
			return "", nil, err
		}
	}

	if q.Project != "" {
		// When we are in the main catalog (no relation filter), we only show
		// project members. When we are listing relations, we show all related
		// patents and use the project JOIN to decorate them with state/tags.
		joinType := "JOIN"
		if !q.Relation.IsZero() && q.ReviewState == domain.ReviewStateNone && !filterexpr.UsesField(expr, filterexpr.FieldState) {
			joinType = "LEFT JOIN"
		}
		sb.WriteString(" " + joinType + " membership m ON m.patent_number = p.number AND m.project_id = ?")
		args = append(args, string(q.Project))
		if q.ReviewState != domain.ReviewStateNone {
			conds = append(conds, "m.state = ?")
			args = append(args, string(q.ReviewState))
		}
		if q.IDSStatus != "" {
			status, err := filterexpr.ParseIDSEntryStatus(q.IDSStatus)
			if err != nil {
				return "", nil, err
			}
			cond, condArgs := idsStatusCondition(status)
			conds = append(conds, cond)
			args = append(args, condArgs...)
		}
	} else if q.IDSStatus != "" {
		return "", nil, fmt.Errorf("ids_status filters require an active project")
	}

	if len(q.Numbers) > 0 {
		placeholders := make([]string, len(q.Numbers))
		for i, n := range q.Numbers {
			placeholders[i] = "?"
			args = append(args, n.Normalized())
		}
		conds = append(conds, fmt.Sprintf("p.number IN (%s)", strings.Join(placeholders, ",")))
	}

	if !q.Relation.IsZero() && q.RelationKind != "" {
		// A relation from A to B of kind K can be stored as (from=A, to=B, kind=K)
		// OR (from=B, to=A, kind=K.Inverse()). To find all patents related to N
		// by kind K, we check both directions.
		conds = append(conds, `EXISTS (SELECT 1 FROM relation rel WHERE `+
			`(rel.from_number = ? AND rel.kind = ? AND rel.to_number = p.number) OR `+
			`(rel.to_number = ? AND rel.kind = ? AND rel.from_number = p.number))`)
		args = append(args, q.Relation.Normalized(), string(q.RelationKind),
			q.Relation.Normalized(), string(q.RelationKind.Inverse()))
	}

	if q.Search != "" {
		// Number stays a substring LIKE so a partial patent number still
		// matches mid-token; title and abstract go through the FTS5 index.
		conds = append(conds, "(p.number LIKE ? OR p.rowid IN "+
			"(SELECT rowid FROM patent_fts WHERE patent_fts MATCH ?))")
		args = append(args, "%"+q.Search+"%", ftsQuery(q.Search))
	}

	if q.Classification != "" {
		for _, part := range splitClassifications(q.Classification) {
			conds = append(conds, `EXISTS (SELECT 1 FROM json_each(p.classifications) WHERE UPPER(json_each.value) LIKE ? ESCAPE '\')`)
			args = append(args, classificationLikePattern(part, false))
		}
	}

	if q.ClassificationCode != "" {
		conds = append(conds, `EXISTS (SELECT 1 FROM json_each(p.classifications) WHERE json_each.value = ?)`)
		args = append(args, q.ClassificationCode)
	}

	if q.Inventor != "" {
		conds = append(conds, `EXISTS (SELECT 1 FROM json_each(p.inventors) WHERE json_each.value = ?)`)
		args = append(args, q.Inventor)
	}

	if q.Assignee != "" {
		conds = append(conds, `p.assignee = ?`)
		args = append(args, q.Assignee)
	}

	if expr != nil {
		exprSQL, exprArgs, err := compileFilterExpr(expr, q)
		if err != nil {
			return "", nil, err
		}
		conds = append(conds, exprSQL)
		args = append(args, exprArgs...)
	}

	if len(conds) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
	}
	return sb.String(), args, nil
}

func compileFilterExpr(expr filterexpr.Expr, q store.PatentQuery) (string, []any, error) {
	switch e := expr.(type) {
	case filterexpr.AndExpr:
		return compileBooleanExpr("AND", e.Left, e.Right, q)
	case filterexpr.OrExpr:
		return compileBooleanExpr("OR", e.Left, e.Right, q)
	case filterexpr.NotExpr:
		sql, args, err := compileFilterExpr(e.Child, q)
		if err != nil {
			return "", nil, err
		}
		return "(NOT (" + sql + "))", args, nil
	case filterexpr.TermExpr:
		return compileFilterTerm(e, q)
	default:
		return "", nil, fmt.Errorf("store/sqlite: unsupported filter expression %T", expr)
	}
}

func compileBooleanExpr(op string, left, right filterexpr.Expr, q store.PatentQuery) (string, []any, error) {
	leftSQL, leftArgs, err := compileFilterExpr(left, q)
	if err != nil {
		return "", nil, err
	}
	rightSQL, rightArgs, err := compileFilterExpr(right, q)
	if err != nil {
		return "", nil, err
	}
	args := append(leftArgs, rightArgs...)
	return "(" + leftSQL + " " + op + " " + rightSQL + ")", args, nil
}

func compileFilterTerm(term filterexpr.TermExpr, q store.PatentQuery) (string, []any, error) {
	switch term.Field {
	case filterexpr.FieldState:
		return "m.state = ?", []any{string(term.State)}, nil
	case filterexpr.FieldIDSStatus:
		cond, args := idsStatusCondition(term.IDSStatus)
		return cond, args, nil
	case filterexpr.FieldTag:
		return `EXISTS (SELECT 1 FROM patent_tag pt JOIN tag t ON t.id = pt.tag_id ` +
			`WHERE t.project_id = ? AND pt.patent_number = p.number AND LOWER(t.name) = LOWER(?))`, []any{string(q.Project), term.Value}, nil
	case filterexpr.FieldClass:
		return `EXISTS (SELECT 1 FROM json_each(p.classifications) WHERE UPPER(json_each.value) LIKE ? ESCAPE '\')`, []any{classificationLikePattern(term.Class.Raw, term.Class.Wildcard)}, nil
	case filterexpr.FieldSearch:
		return `(p.number LIKE ? OR p.rowid IN (SELECT rowid FROM patent_fts WHERE patent_fts MATCH ?))`, []any{"%" + term.Value + "%", ftsQuery(term.Value)}, nil
	case filterexpr.FieldInventor:
		if term.Inventor.Wildcard {
			return `EXISTS (SELECT 1 FROM json_each(p.inventors) WHERE json_each.value LIKE ? ESCAPE '\')`, []any{wildcardLikePattern(term.Inventor.Raw, false)}, nil
		}
		return `EXISTS (SELECT 1 FROM json_each(p.inventors) WHERE json_each.value = ?)`, []any{term.Value}, nil
	case filterexpr.FieldAssignee:
		if term.Assignee.Wildcard {
			return `p.assignee LIKE ? ESCAPE '\'`, []any{wildcardLikePattern(term.Assignee.Raw, false)}, nil
		}
		return `p.assignee = ?`, []any{term.Value}, nil
	case filterexpr.FieldCountry:
		if term.Country.Wildcard {
			return `p.country LIKE ? ESCAPE '\'`, []any{wildcardLikePattern(term.Country.Raw, false)}, nil
		}
		return `p.country = ?`, []any{term.Value}, nil
	case filterexpr.FieldFetchState:
		return `p.fetch_state = ?`, []any{term.Value}, nil
	default:
		return "", nil, fmt.Errorf("store/sqlite: unsupported filter field %q", term.Field)
	}
}

func idsStatusCondition(status string) (string, []any) {
	if status == "none" {
		return "COALESCE(m.ids_status, '') = ''", nil
	}
	return "m.ids_status = ?", []any{status}
}

func classificationLikePattern(part string, wildcard bool) string {
	return wildcardLikePattern(strings.ToUpper(part), wildcard)
}

func wildcardLikePattern(part string, wildcard bool) string {
	var b strings.Builder
	for _, r := range part {
		switch r {
		case '*':
			b.WriteByte('%')
		case '%', '_', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	if !wildcard {
		b.WriteByte('%')
	}
	return b.String()
}

func splitClassifications(s string) []string {
	var parts []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

// ftsQuery turns a user's search string into an FTS5 MATCH expression: each
// whitespace-separated token becomes a quoted prefix term, so "wid ass" matches
// "widget assembly". Quoting neutralizes FTS5 operator characters in the input.
func ftsQuery(search string) string {
	fields := strings.Fields(search)
	if len(fields) == 0 {
		return `""`
	}
	var b strings.Builder
	for i, f := range fields {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteByte('"')
		b.WriteString(strings.ReplaceAll(f, `"`, `""`))
		b.WriteString(`"*`)
	}
	return b.String()
}

// scanPatent reads one patent row in patentColumns order. It does not load the
// record's documents — Patent does that for the detail view; list views read
// only the display_number column.
func scanPatent(s rowScanner) (domain.Patent, error) {
	var (
		p                                domain.Patent
		country, serial, kind            string
		inventors, fetchState, source    string
		appDate, pubDate, grant, fetched string
		displayNumber, expiration        string
		classifications                  string
	)
	if err := s.Scan(&country, &serial, &kind, &p.Title, &p.Abstract, &p.Assignee,
		&inventors, &fetchState, &source, &appDate, &pubDate, &grant, &fetched,
		&displayNumber, &p.FirstClaim, &expiration, &p.ExpirationSource, &p.SourceURL, &classifications); err != nil {
		return domain.Patent{}, err
	}
	p.Number = domain.PatentNumber{Country: country, Serial: serial, Kind: kind}
	p.FetchState = domain.FetchState(fetchState)
	p.Source = domain.Source(source)
	if displayNumber != "" {
		shown, err := domain.ParsePatentNumber(displayNumber)
		if err != nil {
			return domain.Patent{}, fmt.Errorf("decode display number %q: %w", displayNumber, err)
		}
		p.DisplayNumber = shown
	}
	if err := json.Unmarshal([]byte(inventors), &p.Inventors); err != nil {
		return domain.Patent{}, fmt.Errorf("decode inventors: %w", err)
	}
	if err := json.Unmarshal([]byte(classifications), &p.Classifications); err != nil {
		return domain.Patent{}, fmt.Errorf("decode classifications: %w", err)
	}
	var err error
	if p.ApplicationDate, err = decodeTime(appDate); err != nil {
		return domain.Patent{}, err
	}
	if p.PublicationDate, err = decodeTime(pubDate); err != nil {
		return domain.Patent{}, err
	}
	if p.GrantDate, err = decodeTime(grant); err != nil {
		return domain.Patent{}, err
	}
	if p.FetchedAt, err = decodeTime(fetched); err != nil {
		return domain.Patent{}, err
	}
	if p.ExpirationDate, err = decodeTime(expiration); err != nil {
		return domain.Patent{}, err
	}
	return p, nil
}

// SaveRelation inserts a family-graph edge; an existing edge is left untouched.
func (r *Repo) SaveRelation(ctx context.Context, rel domain.Relation) (err error) {
	defer r.observeDuration("save_relation", time.Now(), &err)
	if rel.From.IsZero() || rel.To.IsZero() {
		return errors.New("store/sqlite: relation endpoints must be non-empty")
	}
	if !rel.Kind.Valid() {
		return fmt.Errorf("store/sqlite: invalid relation kind %q", rel.Kind)
	}
	_, err = r.writer.ExecContext(ctx,
		`INSERT INTO relation (from_number, to_number, kind) VALUES (?,?,?)
		 ON CONFLICT(from_number, to_number, kind) DO NOTHING`,
		rel.From.Normalized(), rel.To.Normalized(), string(rel.Kind))
	if err != nil {
		return fmt.Errorf("store/sqlite: save relation: %w", err)
	}
	return nil
}

// Relations returns edges of the given kind originating at n.
func (r *Repo) Relations(ctx context.Context, n domain.PatentNumber, kind domain.RelationKind) (out []domain.Relation, err error) {
	defer r.observeDuration("relations", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT from_number, to_number, kind FROM relation
		 WHERE from_number = ? AND kind = ? ORDER BY to_number`,
		n.Normalized(), string(kind))
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list relations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out = nil
	for rows.Next() {
		var from, to, k string
		if err := rows.Scan(&from, &to, &k); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan relation: %w", err)
		}
		fromNum, err := domain.ParsePatentNumber(from)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: relation from %q: %w", from, err)
		}
		toNum, err := domain.ParsePatentNumber(to)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: relation to %q: %w", to, err)
		}
		out = append(out, domain.Relation{From: fromNum, To: toNum, Kind: domain.RelationKind(k)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: list relations: %w", err)
	}
	return out, nil
}

// AllRelations returns every family-graph edge where n is either the origin (from) or destination (to).
func (r *Repo) AllRelations(ctx context.Context, n domain.PatentNumber) (out []domain.Relation, err error) {
	defer r.observeDuration("all_relations", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT from_number, to_number, kind FROM relation
		 WHERE from_number = ? OR to_number = ? ORDER BY from_number, to_number`,
		n.Normalized(), n.Normalized())
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list all relations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out = nil
	for rows.Next() {
		var from, to, k string
		if err := rows.Scan(&from, &to, &k); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan all relation: %w", err)
		}
		fromNum, err := domain.ParsePatentNumber(from)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: all relation from %q: %w", from, err)
		}
		toNum, err := domain.ParsePatentNumber(to)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: all relation to %q: %w", to, err)
		}
		out = append(out, domain.Relation{From: fromNum, To: toNum, Kind: domain.RelationKind(k)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: list all relations: %w", err)
	}
	return out, nil
}

// PatentInventorStats aggregates database statistics for a set of inventors within a project.
func (r *Repo) PatentInventorStats(ctx context.Context, project domain.ProjectID, inventors []domain.Inventor) (out []domain.InventorStats, err error) {
	defer r.observeDuration("patent_inventor_stats", time.Now(), &err)
	if len(inventors) == 0 {
		return nil, nil
	}

	out = make([]domain.InventorStats, len(inventors))
	for idx, inv := range inventors {
		stats := domain.InventorStats{
			Inventor: string(inv),
			States:   make(map[string]int),
			Tags:     make(map[string]int),
		}

		query := `SELECT p.number, COALESCE(m.state, '') AS state, COALESCE(t.name, '') AS tag_name
			FROM patent p
			LEFT JOIN membership m ON m.patent_number = p.number AND m.project_id = ?
			LEFT JOIN patent_tag pt ON pt.patent_number = p.number
			LEFT JOIN tag t ON t.id = pt.tag_id AND t.project_id = ?
			WHERE EXISTS (
				SELECT 1 FROM json_each(p.inventors) WHERE json_each.value = ?
			)`

		rows, err := r.reader.QueryContext(ctx, query, string(project), string(project), string(inv))
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: query inventor stats for %q: %w", inv, err)
		}

		err = func() error {
			defer rows.Close()
			seen := make(map[string]bool)
			for rows.Next() {
				var number, state, tagName string
				if err := rows.Scan(&number, &state, &tagName); err != nil {
					return fmt.Errorf("store/sqlite: scan inventor stats row: %w", err)
				}

				if !seen[number] {
					stats.Total++
					seen[number] = true

					if state != string(domain.ReviewStateUnknown) && state != string(domain.ReviewStateUnderReview) && state != string(domain.ReviewStateActive) && state != string(domain.ReviewStateIgnored) {
						state = "other"
					}
					stats.States[state]++
				}

				if tagName != "" {
					stats.Tags[tagName]++
				}
			}
			return rows.Err()
		}()
		if err != nil {
			return nil, err
		}

		out[idx] = stats
	}

	return out, nil
}

// PatentAssigneeStats aggregates database statistics for all non-empty assignees within a project.
func (r *Repo) PatentAssigneeStats(ctx context.Context, project domain.ProjectID) (out []domain.AssigneeStats, err error) {
	defer r.observeDuration("patent_assignee_stats", time.Now(), &err)
	query := `SELECT p.assignee, p.number, COALESCE(m.state, ''), COALESCE(t.name, '')
		FROM patent p
		LEFT JOIN membership m ON m.patent_number = p.number AND m.project_id = ?
		LEFT JOIN patent_tag pt ON pt.patent_number = p.number
		LEFT JOIN tag t ON t.id = pt.tag_id AND t.project_id = ?
		WHERE TRIM(p.assignee) != ''`
	args := []any{string(project), string(project)}
	if project != "" {
		query += ` AND EXISTS (SELECT 1 FROM membership sm WHERE sm.project_id = ? AND sm.patent_number = p.number)`
		args = append(args, string(project))
	}
	query += ` ORDER BY p.assignee, p.number, t.name`
	rows, err := r.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: query assignee stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	byAssignee := make(map[string]*domain.AssigneeStats)
	seenPatents := make(map[string]map[string]bool)
	for rows.Next() {
		var assignee, number, state, tagName string
		if err := rows.Scan(&assignee, &number, &state, &tagName); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan assignee stats row: %w", err)
		}
		stats := byAssignee[assignee]
		if stats == nil {
			stats = &domain.AssigneeStats{Assignee: assignee, States: make(map[string]int), Tags: make(map[string]int)}
			byAssignee[assignee] = stats
			seenPatents[assignee] = make(map[string]bool)
		}
		if !seenPatents[assignee][number] {
			stats.Total++
			seenPatents[assignee][number] = true
			if state != "" {
				if state != string(domain.ReviewStateUnknown) && state != string(domain.ReviewStateUnderReview) && state != string(domain.ReviewStateActive) && state != string(domain.ReviewStateIgnored) {
					state = "other"
				}
				stats.States[state]++
			}
		}
		if tagName != "" {
			stats.Tags[tagName]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: assignee stats rows: %w", err)
	}
	for _, stats := range byAssignee {
		out = append(out, *stats)
	}
	slices.SortFunc(out, func(a, b domain.AssigneeStats) int {
		if a.Total != b.Total {
			return b.Total - a.Total
		}
		return strings.Compare(a.Assignee, b.Assignee)
	})
	return out, nil
}

// PatentClassificationStats aggregates database statistics for all classification codes within a project.
func (r *Repo) PatentClassificationStats(ctx context.Context, project domain.ProjectID) (out []domain.ClassificationStats, err error) {
	defer r.observeDuration("patent_classification_stats", time.Now(), &err)
	query := `SELECT json_each.value, p.number, COALESCE(m.state, ''), COALESCE(t.name, ''),
		COALESCE(cd.system, ''), COALESCE(cd.description, '')
		FROM patent p
		JOIN json_each(p.classifications)
		LEFT JOIN membership m ON m.patent_number = p.number AND m.project_id = ?
		LEFT JOIN patent_tag pt ON pt.patent_number = p.number
		LEFT JOIN tag t ON t.id = pt.tag_id AND t.project_id = ?
		LEFT JOIN classification_definition cd ON cd.code = json_each.value
		WHERE TRIM(json_each.value) != ''`
	args := []any{string(project), string(project)}
	if project != "" {
		query += ` AND EXISTS (SELECT 1 FROM membership sm WHERE sm.project_id = ? AND sm.patent_number = p.number)`
		args = append(args, string(project))
	}
	query += ` ORDER BY json_each.value, p.number, t.name`
	rows, err := r.reader.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: query classification stats: %w", err)
	}
	defer func() { _ = rows.Close() }()

	byCode := make(map[string]*domain.ClassificationStats)
	seenPatents := make(map[string]map[string]bool)
	for rows.Next() {
		var code, number, state, tagName, system, description string
		if err := rows.Scan(&code, &number, &state, &tagName, &system, &description); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan classification stats row: %w", err)
		}
		stats := byCode[code]
		if stats == nil {
			parsed := domain.ParseClassification(code)
			if system != "" {
				parsed.System = system
			}
			if description != "" {
				parsed.Description = description
			}
			stats = &domain.ClassificationStats{Classification: parsed, States: make(map[string]int), Tags: make(map[string]int)}
			byCode[code] = stats
			seenPatents[code] = make(map[string]bool)
		}
		if !seenPatents[code][number] {
			stats.Total++
			seenPatents[code][number] = true
			if state != "" {
				if state != string(domain.ReviewStateUnknown) && state != string(domain.ReviewStateUnderReview) && state != string(domain.ReviewStateActive) && state != string(domain.ReviewStateIgnored) {
					state = "other"
				}
				stats.States[state]++
			}
		}
		if tagName != "" {
			stats.Tags[tagName]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: classification stats rows: %w", err)
	}
	for _, stats := range byCode {
		out = append(out, *stats)
	}
	slices.SortFunc(out, func(a, b domain.ClassificationStats) int {
		if a.Total != b.Total {
			return b.Total - a.Total
		}
		return strings.Compare(a.Classification.Code, b.Classification.Code)
	})
	return out, nil
}
