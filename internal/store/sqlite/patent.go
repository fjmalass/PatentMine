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
	p.grant_date, p.fetched_at, p.display_number`

// patentRowColumns returns the lightweight listing column set in scanPatentRow's
// order, optionally including a project membership state.
func patentRowColumns(includeMembership bool) string {
	if includeMembership {
		return `p.country, p.serial, p.kind, p.display_number, p.title, p.fetch_state, m.state`
	}
	return `p.country, p.serial, p.kind, p.display_number, p.title, p.fetch_state, ''`
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
			grant_date, fetched_at, display_number)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(number) DO UPDATE SET
			country=excluded.country, serial=excluded.serial, kind=excluded.kind,
			title=excluded.title, abstract=excluded.abstract, assignee=excluded.assignee,
			inventors=excluded.inventors, fetch_state=excluded.fetch_state,
			source=excluded.source, application_date=excluded.application_date,
			publication_date=excluded.publication_date, grant_date=excluded.grant_date,
			fetched_at=excluded.fetched_at, display_number=excluded.display_number`,
		p.Number.Normalized(), p.Number.Country, p.Number.Serial, p.Number.Kind,
		p.Title, p.Abstract, p.Assignee, string(inventors),
		string(p.FetchState), string(p.Source),
		encodeTime(p.ApplicationDate), encodeTime(p.PublicationDate),
		encodeTime(p.GrantDate), encodeTime(p.FetchedAt), display.Normalized())
	if err != nil {
		return fmt.Errorf("store/sqlite: save patent %s: %w", p.Number, err)
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
	where, args := patentFilter(q)
	limit := q.Limit
	if limit <= 0 {
		limit = store.DefaultPageSize
	}
	query := `SELECT ` + patentRowColumns(q.Project != "") + ` FROM patent p` + where +
		` ORDER BY p.number LIMIT ? OFFSET ?`
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
		row                          domain.PatentRow
		country, serial, kind, shown string
		fetchState, membershipState  string
	)
	if err := s.Scan(&country, &serial, &kind, &shown, &row.Title, &fetchState, &membershipState); err != nil {
		return domain.PatentRow{}, err
	}
	row.Number = domain.PatentNumber{Country: country, Serial: serial, Kind: kind}
	row.FetchState = domain.FetchState(fetchState)
	row.MembershipState = domain.MembershipState(membershipState)
	if shown != "" {
		display, err := domain.ParsePatentNumber(shown)
		if err != nil {
			return domain.PatentRow{}, fmt.Errorf("decode display number %q: %w", shown, err)
		}
		row.DisplayNumber = display
	}
	return row, nil
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
	if q.Project != "" {
		sb.WriteString(" JOIN membership m ON m.patent_number = p.number AND m.project_id = ?")
		args = append(args, string(q.Project))
	}
	var conds []string
	if q.Project != "" && q.State != "" {
		conds = append(conds, "m.state = ?")
		args = append(args, string(q.State))
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
		displayNumber                    string
	)
	if err := s.Scan(&country, &serial, &kind, &p.Title, &p.Abstract, &p.Assignee,
		&inventors, &fetchState, &source, &appDate, &pubDate, &grant, &fetched,
		&displayNumber); err != nil {
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
