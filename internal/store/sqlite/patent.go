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

// patentColumns is the column list, p-aliased, in scanPatent's order.
const patentColumns = `p.country, p.serial, p.kind, p.title, p.abstract, p.assignee,
	p.inventors, p.fetch_state, p.source, p.application_date, p.publication_date,
	p.grant_date, p.fetched_at, p.display_number,
	p.first_claim, p.expiration_date, p.expiration_source, p.source_url`

// patentRowColumns returns the SELECT column list for scanPatentRow and any
// args those columns need (the tag subquery binds the project ID).
// When project is non-empty the membership state and project tags are included.
func patentRowColumns(project domain.ProjectID) (cols string, extraArgs []any) {
	const tagsSubq = `COALESCE((SELECT json_group_array(t.name) FROM patent_tag pt ` +
		`JOIN tag t ON t.id = pt.tag_id ` +
		`WHERE pt.patent_number = p.number AND t.project_id = ?), '[]')`
	if project != "" {
		return `p.country, p.serial, p.kind, p.display_number, p.title, ` +
			`p.inventors, p.expiration_date, p.fetch_state, COALESCE(m.state, ''), ` + tagsSubq,
			[]any{string(project)}
	}
	return `p.country, p.serial, p.kind, p.display_number, p.title, ` +
		`p.inventors, p.expiration_date, p.fetch_state, '', '[]'`, nil
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
	inventors, err := json.Marshal(p.Inventors)
	if err != nil {
		return fmt.Errorf("store/sqlite: encode inventors: %w", err)
	}
	// The display number defaults to the record number until documents set it.
	display := p.DisplayNumber
	if display.IsZero() {
		display = p.Number
	}
	_, err = r.writer.ExecContext(ctx, `
		INSERT INTO patent (number, country, serial, kind, title, abstract, assignee,
			inventors, fetch_state, source, application_date, publication_date,
			grant_date, fetched_at, display_number,
			first_claim, expiration_date, expiration_source, source_url)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(number) DO UPDATE SET
			country=excluded.country, serial=excluded.serial, kind=excluded.kind,
			title=excluded.title, abstract=excluded.abstract, assignee=excluded.assignee,
			inventors=excluded.inventors, fetch_state=excluded.fetch_state,
			source=excluded.source, application_date=excluded.application_date,
			publication_date=excluded.publication_date, grant_date=excluded.grant_date,
			fetched_at=excluded.fetched_at, display_number=excluded.display_number,
			first_claim=excluded.first_claim, expiration_date=excluded.expiration_date,
			expiration_source=excluded.expiration_source, source_url=excluded.source_url`,
		p.Number.Normalized(), p.Number.Country, p.Number.Serial, p.Number.Kind,
		p.Title, p.Abstract, p.Assignee, string(inventors),
		string(p.FetchState), string(p.Source),
		encodeTime(p.ApplicationDate), encodeTime(p.PublicationDate),
		encodeTime(p.GrantDate), encodeTime(p.FetchedAt), display.Normalized(),
		p.FirstClaim, encodeTime(p.ExpirationDate), p.ExpirationSource, p.SourceURL)
	if err != nil {
		return fmt.Errorf("store/sqlite: save patent %s: %w", p.Number, err)
	}
	// When a patent is fully fetched, every project membership that is currently
	// "stored" (the default for numbers discovered from family edges) moves to
	// "cached".
	if p.FetchState == domain.FetchCached {
		_, err = r.writer.ExecContext(ctx,
			`UPDATE membership SET state = ? WHERE patent_number = ? AND state = ?`,
			string(domain.ReviewStateCached), p.Number.Normalized(), string(domain.ReviewStateLoad))
		if err != nil {
			return fmt.Errorf("store/sqlite: update membership state on fetch: %w", err)
		}
	}
	return nil
}

// DeletePatent permanently removes a patent and all its associated documents,
// relations, and memberships.
func (r *Repo) DeletePatent(ctx context.Context, n domain.PatentNumber) (err error) {
	defer r.observeDuration("delete_patent", time.Now(), &err)
	num := n.Normalized()
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: delete patent %s: begin tx: %w", n, err)
	}
	defer func() { _ = tx.Rollback() }()
	// relation has no FK cascade — clear both sides.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM relation WHERE from_number = ? OR to_number = ?`, num, num); err != nil {
		return fmt.Errorf("store/sqlite: delete patent %s: relations: %w", n, err)
	}
	// membership has no FK cascade.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM membership WHERE patent_number = ?`, num); err != nil {
		return fmt.Errorf("store/sqlite: delete patent %s: memberships: %w", n, err)
	}
	// deleting the patent row cascades to document and patent_tag.
	res, err := tx.ExecContext(ctx, `DELETE FROM patent WHERE number = ?`, num)
	if err != nil {
		return fmt.Errorf("store/sqlite: delete patent %s: %w", n, err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return store.ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: delete patent %s: commit: %w", n, err)
	}
	return nil
}

// Patent returns one patent, or store.ErrNotFound.
func (r *Repo) Patent(ctx context.Context, n domain.PatentNumber) (patent domain.Patent, err error) {
	defer r.observeDuration("patent", time.Now(), &err)
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
	cols, colArgs := patentRowColumns(q.Project)
	where, whereArgs := patentFilter(q)
	limit := q.Limit
	if limit <= 0 {
		limit = store.DefaultPageSize
	}
	query := `SELECT ` + cols + ` FROM patent p` + where +
		` ORDER BY ` + patentSortExpr(q.SortColumn, q.SortAscending) + ` LIMIT ? OFFSET ?`
	args := append(colArgs, whereArgs...)
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
	return out, nil
}

// scanPatentRow reads one lightweight listing row.
func scanPatentRow(s rowScanner) (domain.PatentRow, error) {
	var (
		row                           domain.PatentRow
		country, serial, kind, shown  string
		inventorsJSON, expirationDate string
		fetchState, reviewState       string
		tagsJSON                      string
	)
	if err := s.Scan(&country, &serial, &kind, &shown, &row.Title,
		&inventorsJSON, &expirationDate, &fetchState, &reviewState, &tagsJSON); err != nil {
		return domain.PatentRow{}, err
	}
	row.Number = domain.PatentNumber{Country: country, Serial: serial, Kind: kind}
	row.FetchState = domain.FetchState(fetchState)
	row.ReviewState = domain.ReviewState(reviewState)
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
	return row, nil
}

// patentSortExpr returns an ORDER BY expression for the given sort column and
// direction. An empty col always returns number ASC (the default stable order).
func patentSortExpr(col domain.SortColumn, asc bool) string {
	if col == "" {
		return "p.number ASC"
	}
	dir := "ASC"
	if !asc {
		dir = "DESC"
	}
	switch col {
	case domain.SortByTitle:
		return "p.title " + dir
	case domain.SortByInventor:
		return "json_extract(p.inventors, '$[0]') " + dir
	case domain.SortByExpires:
		return "p.expiration_date " + dir
	case domain.SortByReviewState:
		return "COALESCE(m.state, p.fetch_state) " + dir
	default:
		return "p.number " + dir
	}
}

// CountPatents returns the total rows matching q, ignoring its paging.
func (r *Repo) CountPatents(ctx context.Context, q store.PatentQuery) (count int, err error) {
	defer r.observeDuration("count_patents", time.Now(), &err)
	where, args := patentFilter(q)
	var n int
	if err := r.reader.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM patent p`+where, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("store/sqlite: count patents: %w", err)
	}
	return n, nil
}

// patentFilter builds the shared FROM-suffix (JOIN + WHERE) for list and count.
func patentFilter(q store.PatentQuery) (string, []any) {
	var sb strings.Builder
	var args []any
	var conds []string

	if q.Project != "" {
		// When we are in the main catalog (no relation filter), we only show
		// project members. When we are listing relations, we show all related
		// patents and use the project JOIN to decorate them with state/tags.
		joinType := "JOIN"
		if !q.Relation.IsZero() && q.ReviewState == domain.ReviewStateNone {
			joinType = "LEFT JOIN"
		}
		sb.WriteString(" " + joinType + " membership m ON m.patent_number = p.number AND m.project_id = ?")
		args = append(args, string(q.Project))
		if q.ReviewState != domain.ReviewStateNone {
			conds = append(conds, "m.state = ?")
			args = append(args, string(q.ReviewState))
		}
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
		conds = append(conds, "(p.number LIKE ? OR p.title LIKE ?)")
		like := "%" + q.Search + "%"
		args = append(args, like, like)
	}

	if len(conds) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
	}
	return sb.String(), args
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
	)
	if err := s.Scan(&country, &serial, &kind, &p.Title, &p.Abstract, &p.Assignee,
		&inventors, &fetchState, &source, &appDate, &pubDate, &grant, &fetched,
		&displayNumber, &p.FirstClaim, &expiration, &p.ExpirationSource, &p.SourceURL); err != nil {
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
