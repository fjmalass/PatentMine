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
	p.first_claim, p.expiration_date, p.expiration_source, p.source_url, p.classifications`

// patentRowColumns returns the SELECT column list for scanPatentRow. When
// project is non-empty the membership state and curated IDS columns are
// included; tags are not selected here — ListPatents fills them for the whole
// page in one query (see attachTags) rather than a per-row subquery.
func patentRowColumns(project domain.ProjectID) (cols string, extraArgs []any) {
	if project != "" {
		return `p.country, p.serial, p.kind, p.display_number, p.title, ` +
				`p.inventors, p.expiration_date, p.fetch_state, COALESCE(m.state, ''), '[]', ` +
				`COALESCE(m.ids_kind_code, ''), COALESCE(m.ids_country_code, ''), COALESCE(m.ids_in_full, 0), ` +
				`COALESCE(m.ids_relevant_passages, ''), COALESCE(m.ids_notes, ''), COALESCE(m.ids_status, ''), COALESCE(m.ids_added_at, ''), p.classifications`,
			nil
	}
	return `p.country, p.serial, p.kind, p.display_number, p.title, ` +
		`p.inventors, p.expiration_date, p.fetch_state, '', '[]', '', '', 0, '', '', '', '', p.classifications`, nil
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
	// When a patent is fully fetched, every project membership that is currently
	// "stored" (the default for numbers discovered from family edges) moves to
	// "cached".
	if p.FetchState == domain.FetchCached {
		_, err = r.writer.ExecContext(ctx,
			`UPDATE membership SET state = ? WHERE patent_number = ? AND state = ?`,
			string(domain.ReviewStateCached), p.Number.Normalized(), string(domain.ReviewStateStored))
		if err != nil {
			return fmt.Errorf("store/sqlite: update membership state on fetch: %w", err)
		}
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
	var tagSortStart time.Time
	if q.SortColumn == domain.SortByTags {
		tagSortStart = time.Now()
		defer func() { r.observeTagSortDuration(tagSortStart, &err, len(out), q) }()
	}
	cols, colArgs := patentRowColumns(q.Project)
	where, whereArgs := patentFilter(q)
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
		row                           domain.PatentRow
		country, serial, kind, shown  string
		inventorsJSON, expirationDate string
		fetchState, reviewState       string
		tagsJSON                      string
		idsKindCode, idsCountryCode   string
		idsRelevant, idsNotes         string
		idsStatus, idsAddedAt         string
		idsInFull                     int
		classificationsJSON           string
	)
	if err := s.Scan(&country, &serial, &kind, &shown, &row.Title,
		&inventorsJSON, &expirationDate, &fetchState, &reviewState, &tagsJSON,
		&idsKindCode, &idsCountryCode, &idsInFull, &idsRelevant, &idsNotes, &idsStatus, &idsAddedAt,
		&classificationsJSON); err != nil {
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
	if err := json.Unmarshal([]byte(classificationsJSON), &row.Classifications); err != nil {
		row.Classifications = nil
	}
	if idsStatus != "" || idsKindCode != "" || idsCountryCode != "" || idsRelevant != "" || idsNotes != "" || idsAddedAt != "" || idsInFull != 0 {
		row.IDSEntry = &domain.IDSEntry{
			Patent:           row.Number,
			KindCode:         idsKindCode,
			CountryCode:      idsCountryCode,
			InFull:           idsInFull != 0,
			RelevantPassages: idsRelevant,
			Notes:            idsNotes,
			Status:           domain.IDSEntryStatus(idsStatus),
		}
		if t, err := decodeTime(idsAddedAt); err == nil {
			row.IDSEntry.AddedAt = t
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
		// Number stays a substring LIKE so a partial patent number still
		// matches mid-token; title and abstract go through the FTS5 index.
		conds = append(conds, "(p.number LIKE ? OR p.rowid IN "+
			"(SELECT rowid FROM patent_fts WHERE patent_fts MATCH ?))")
		args = append(args, "%"+q.Search+"%", ftsQuery(q.Search))
	}

	if q.Classification != "" {
		for _, part := range splitClassifications(q.Classification) {
			conds = append(conds, `EXISTS (SELECT 1 FROM json_each(p.classifications) WHERE json_each.value LIKE ?)`)
			args = append(args, part+"%")
		}
	}

	if q.Inventor != "" {
		conds = append(conds, `EXISTS (SELECT 1 FROM json_each(p.inventors) WHERE json_each.value = ?)`)
		args = append(args, q.Inventor)
	}

	if len(conds) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
	}
	return sb.String(), args
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

					if state != string(domain.ReviewStateStored) && state != string(domain.ReviewStateUnderReview) {
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
