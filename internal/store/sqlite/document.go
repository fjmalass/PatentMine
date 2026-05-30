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

// SaveDocument inserts or updates one life-stage document of a record.
func (r *Repo) SaveDocument(ctx context.Context, recordNumber domain.PatentNumber, doc domain.Document) (err error) {
	defer r.observeDuration("save_document", time.Now(), &err)
	if recordNumber.IsZero() {
		return errors.New("store/sqlite: document needs a record number")
	}
	if doc.Number.IsZero() {
		return errors.New("store/sqlite: document needs a number")
	}
	if !doc.Stage.Valid() {
		return fmt.Errorf("store/sqlite: invalid document stage %q", doc.Stage)
	}
	_, err = r.writer.ExecContext(ctx, `
		INSERT INTO document (number, record_number, country, serial, kind, stage, dated)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(number) DO UPDATE SET
			record_number=excluded.record_number, country=excluded.country,
			serial=excluded.serial, kind=excluded.kind, stage=excluded.stage,
			dated=excluded.dated`,
		doc.Number.Normalized(), recordNumber.Normalized(),
		doc.Number.Country, doc.Number.Serial, doc.Number.Kind,
		string(doc.Stage), encodeTime(doc.Dated))
	if err != nil {
		return fmt.Errorf("store/sqlite: save document %s: %w", doc.Number, err)
	}
	return nil
}

// Documents returns every document of a record.
func (r *Repo) Documents(ctx context.Context, recordNumber domain.PatentNumber) (out []domain.Document, err error) {
	defer r.observeDuration("documents", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx,
		`SELECT country, serial, kind, stage, dated FROM document
		 WHERE record_number = ? ORDER BY number`, recordNumber.Normalized())
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list documents: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out = nil
	for rows.Next() {
		var country, serial, kind, stage, dated string
		if err := rows.Scan(&country, &serial, &kind, &stage, &dated); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan document: %w", err)
		}
		when, err := decodeTime(dated)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.Document{
			Number: domain.PatentNumber{Country: country, Serial: serial, Kind: kind},
			Stage:  domain.Stage(stage),
			Dated:  when,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: list documents: %w", err)
	}
	return out, nil
}

// RecordOf returns the record number a document number belongs to, or
// store.ErrNotFound when the number is unknown.
func (r *Repo) RecordOf(ctx context.Context, number domain.PatentNumber) (record domain.PatentNumber, err error) {
	defer r.observeDuration("record_of", time.Now(), &err)
	var recordNumber string
	err = r.reader.QueryRowContext(ctx,
		`SELECT record_number FROM document WHERE number = ?`, number.Normalized()).
		Scan(&recordNumber)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PatentNumber{}, store.ErrNotFound
	}
	if err != nil {
		return domain.PatentNumber{}, fmt.Errorf("store/sqlite: record of %s: %w", number, err)
	}
	return domain.ParsePatentNumber(recordNumber)
}

// MergeRecords folds the absorb record into keep: its documents, memberships,
// and relations are repointed and the absorb patent row is deleted. It is a
// no-op when keep and absorb are the same record.
func (r *Repo) MergeRecords(ctx context.Context, keep, absorb domain.PatentNumber) (err error) {
	defer r.observeDuration("merge_records", time.Now(), &err)
	if keep.IsZero() || absorb.IsZero() {
		return errors.New("store/sqlite: merge needs two record numbers")
	}
	if keep.Equal(absorb) {
		return nil
	}
	k, a := keep.Normalized(), absorb.Normalized()

	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: begin merge: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	exec := func(query string, args ...any) error {
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("store/sqlite: merge %s -> %s: %w", a, k, err)
		}
		return nil
	}

	// Move documents to keep. Repoint memberships and relations; OR IGNORE
	// skips a move that would collide with a row keep already has, and the
	// leftovers still pointing at absorb are then deleted.
	steps := []struct {
		query string
		args  []any
	}{
		{`UPDATE document SET record_number = ? WHERE record_number = ?`, []any{k, a}},
		{`UPDATE OR IGNORE membership SET patent_number = ? WHERE patent_number = ?`, []any{k, a}},
		{`DELETE FROM membership WHERE patent_number = ?`, []any{a}},
		{`UPDATE OR IGNORE relation SET from_number = ? WHERE from_number = ?`, []any{k, a}},
		{`UPDATE OR IGNORE relation SET to_number = ? WHERE to_number = ?`, []any{k, a}},
		{`DELETE FROM relation WHERE from_number = ? OR to_number = ?`, []any{a, a}},
		{`DELETE FROM relation WHERE from_number = to_number`, nil},
		{`DELETE FROM patent WHERE number = ?`, []any{a}},
	}
	for _, s := range steps {
		if err := exec(s.query, s.args...); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: commit merge: %w", err)
	}
	return nil
}

// CheckCollisions scans the database for collision/merge candidates.
func (r *Repo) CheckCollisions(ctx context.Context) (out []store.CollisionCandidate, err error) {
	defer r.observeDuration("check_collisions", time.Now(), &err)

	rows, err := r.reader.QueryContext(ctx, `
		SELECT d.country || d.serial AS app_num, d.record_number, p.title
		FROM document d
		JOIN patent p ON d.record_number = p.number
		WHERE (d.country, d.serial) IN (
			SELECT country, serial
			FROM document
			GROUP BY country, serial
			HAVING count(distinct record_number) > 1
		)
		ORDER BY d.country, d.serial, d.record_number
	`)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: check collisions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	byApp := make(map[string]*store.CollisionCandidate)
	var order []string

	for rows.Next() {
		var appNum, recNum, title string
		if err := rows.Scan(&appNum, &recNum, &title); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan collision candidate: %w", err)
		}
		pn, err := domain.ParsePatentNumber(recNum)
		if err != nil {
			continue
		}
		cand, exists := byApp[appNum]
		if !exists {
			cand = &store.CollisionCandidate{
				ApplicationNumber: appNum,
			}
			byApp[appNum] = cand
			order = append(order, appNum)
		}
		// Deduplicate record numbers within the same candidate group
		found := false
		for _, existing := range cand.RecordNumbers {
			if existing.Equal(pn) {
				found = true
				break
			}
		}
		if !found {
			cand.RecordNumbers = append(cand.RecordNumbers, pn)
			cand.Titles = append(cand.Titles, title)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: list collisions: %w", err)
	}

	out = make([]store.CollisionCandidate, 0, len(order))
	for _, appNum := range order {
		out = append(out, *byApp[appNum])
	}
	return out, nil
}
