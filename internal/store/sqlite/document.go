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
		INSERT INTO document (number, record_id, country, serial, kind, stage, dated)
		VALUES (?, (SELECT record_id FROM patent WHERE number = ?), ?,?,?,?,?)
		ON CONFLICT(number) DO UPDATE SET
			record_id=excluded.record_id, country=excluded.country,
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
		`SELECT d.country, d.serial, d.kind, d.stage, d.dated FROM document d
		 JOIN patent p ON p.record_id = d.record_id
		 WHERE p.number = ? ORDER BY d.number`, recordNumber.Normalized())
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
		`SELECT p.number FROM document d JOIN patent p ON p.record_id = d.record_id
		 WHERE d.number = ? OR (d.country = ? AND d.serial = ? AND (d.kind = ? OR d.kind = '' OR ? = ''))
		 ORDER BY (d.kind = ?) DESC
		 LIMIT 1`,
		number.Normalized(),
		number.Country,
		number.Serial,
		number.Kind,
		number.Kind,
		number.Kind,
	).Scan(&recordNumber)
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

	// Resolve record_ids for both numbers inside the tx so concurrent writes
	// cannot remove either row between resolution and the merge.
	var keepID, absorbID string
	if err := tx.QueryRowContext(ctx, `SELECT record_id FROM patent WHERE number = ?`, k).Scan(&keepID); err != nil {
		return fmt.Errorf("store/sqlite: merge %s -> %s: resolve keep: %w", a, k, err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT record_id FROM patent WHERE number = ?`, a).Scan(&absorbID); err != nil {
		return fmt.Errorf("store/sqlite: merge %s -> %s: resolve absorb: %w", a, k, err)
	}

	exec := func(query string, args ...any) error {
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("store/sqlite: merge %s -> %s: %w", a, k, err)
		}
		return nil
	}

	// Move documents and edge endpoints to keep's record_id; collisions are
	// skipped with OR IGNORE and the leftovers still pointing at absorb are
	// then deleted. Finally absorb's patent row is removed.
	steps := []struct {
		query string
		args  []any
	}{
		{`UPDATE document SET record_id = ? WHERE record_id = ?`, []any{keepID, absorbID}},
		{`UPDATE OR IGNORE membership SET record_id = ? WHERE record_id = ?`, []any{keepID, absorbID}},
		{`DELETE FROM membership WHERE record_id = ?`, []any{absorbID}},
		{`UPDATE OR IGNORE relation SET from_record_id = ? WHERE from_record_id = ?`, []any{keepID, absorbID}},
		{`UPDATE OR IGNORE relation SET to_record_id = ? WHERE to_record_id = ?`, []any{keepID, absorbID}},
		{`DELETE FROM relation WHERE from_record_id = ? OR to_record_id = ?`, []any{absorbID, absorbID}},
		{`DELETE FROM relation WHERE from_record_id = to_record_id`, nil},
		{`DELETE FROM patent WHERE record_id = ?`, []any{absorbID}},
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
