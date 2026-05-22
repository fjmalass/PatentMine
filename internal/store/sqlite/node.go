package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

// patentUpsertSQL inserts or updates a full patent record. It is shared by
// SavePatent and SaveNode so the two paths can never drift apart.
const patentUpsertSQL = `
	INSERT INTO patent (number, country, serial, kind, title, abstract, assignee,
		inventors, fetch_state, source, application_date, publication_date,
		grant_date, fetched_at, display_number,
		first_claim, expiration_date, expiration_source, source_url, classifications)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(number) DO UPDATE SET
		country=excluded.country, serial=excluded.serial, kind=excluded.kind,
		title=excluded.title, abstract=excluded.abstract, assignee=excluded.assignee,
		inventors=excluded.inventors, fetch_state=excluded.fetch_state,
		source=excluded.source, application_date=excluded.application_date,
		publication_date=excluded.publication_date, grant_date=excluded.grant_date,
		fetched_at=excluded.fetched_at, display_number=excluded.display_number,
		first_claim=excluded.first_claim, expiration_date=excluded.expiration_date,
		expiration_source=excluded.expiration_source, source_url=excluded.source_url,
		classifications=excluded.classifications`

// documentUpsertSQL inserts or updates one life-stage document.
const documentUpsertSQL = `
	INSERT INTO document (number, record_number, country, serial, kind, stage, dated)
	VALUES (?,?,?,?,?,?,?)
	ON CONFLICT(number) DO UPDATE SET
		record_number=excluded.record_number, country=excluded.country,
		serial=excluded.serial, kind=excluded.kind, stage=excluded.stage,
		dated=excluded.dated`

// patentUpsertArgs returns the bind arguments for patentUpsertSQL.
func patentUpsertArgs(p domain.Patent) ([]any, error) {
	inventors, err := json.Marshal(p.Inventors)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: encode inventors: %w", err)
	}
	classifications, err := json.Marshal(p.Classifications)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: encode classifications: %w", err)
	}
	display := p.DisplayNumber
	if display.IsZero() {
		display = p.Number
	}
	return []any{
		p.Number.Normalized(), p.Number.Country, p.Number.Serial, p.Number.Kind,
		p.Title, p.Abstract, p.Assignee, string(inventors),
		string(p.FetchState), string(p.Source),
		encodeTime(p.ApplicationDate), encodeTime(p.PublicationDate),
		encodeTime(p.GrantDate), encodeTime(p.FetchedAt), display.Normalized(),
		p.FirstClaim, encodeTime(p.ExpirationDate), p.ExpirationSource, p.SourceURL,
		string(classifications),
	}, nil
}

// SaveNode atomically writes one crawled patent: its record, documents,
// neighbour stubs, and family-graph edges. Every write in the batch shares one
// transaction, so a node with many citations commits once instead of once per
// row. A stub never overwrites an existing record (ON CONFLICT DO NOTHING) and
// duplicate edges are ignored.
func (r *Repo) SaveNode(ctx context.Context, batch store.NodeBatch) (err error) {
	defer r.observeDuration("save_node", time.Now(), &err)

	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: save node: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if !batch.Patent.Number.IsZero() {
		p := batch.Patent
		if !p.FetchState.Valid() {
			return fmt.Errorf("store/sqlite: invalid fetch state %q", p.FetchState)
		}
		args, argErr := patentUpsertArgs(p)
		if argErr != nil {
			return argErr
		}
		if _, err := tx.ExecContext(ctx, patentUpsertSQL, args...); err != nil {
			return fmt.Errorf("store/sqlite: save node patent %s: %w", p.Number, err)
		}
		// A fully fetched patent promotes any membership still in the default
		// "stored" state to "cached" — the same rule SavePatent applies.
		if p.FetchState == domain.FetchCached {
			if _, err := tx.ExecContext(ctx,
				`UPDATE membership SET state = ? WHERE patent_number = ? AND state = ?`,
				string(domain.ReviewStateCached), p.Number.Normalized(), string(domain.ReviewStateStored)); err != nil {
				return fmt.Errorf("store/sqlite: save node membership state: %w", err)
			}
		}
		for _, doc := range batch.Documents {
			if err := execDocumentUpsert(ctx, tx, p.Number, doc); err != nil {
				return err
			}
		}
	}

	for _, stub := range batch.Stubs {
		if stub.Number.IsZero() {
			continue
		}
		stubPatent := domain.Patent{
			Number:        stub.Number,
			DisplayNumber: stub.Number,
			FetchState:    domain.FetchStub,
		}
		args, argErr := patentUpsertArgs(stubPatent)
		if argErr != nil {
			return argErr
		}
		// A stub must not clobber a record that already exists, so the insert
		// does nothing on conflict rather than upserting empty fields.
		if _, err := tx.ExecContext(ctx, patentInsertOrIgnoreSQL, args...); err != nil {
			return fmt.Errorf("store/sqlite: save node stub %s: %w", stub.Number, err)
		}
		if _, err := tx.ExecContext(ctx, documentInsertOrIgnoreSQL,
			stub.Number.Normalized(), stub.Number.Normalized(),
			stub.Number.Country, stub.Number.Serial, stub.Number.Kind,
			string(stub.Stage), encodeTime(time.Time{})); err != nil {
			return fmt.Errorf("store/sqlite: save node stub document %s: %w", stub.Number, err)
		}
	}

	for _, rel := range batch.Relations {
		if rel.From.IsZero() || rel.To.IsZero() || !rel.Kind.Valid() {
			return fmt.Errorf("store/sqlite: save node: invalid relation %s->%s %q", rel.From, rel.To, rel.Kind)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO relation (from_number, to_number, kind) VALUES (?,?,?)
			 ON CONFLICT(from_number, to_number, kind) DO NOTHING`,
			rel.From.Normalized(), rel.To.Normalized(), string(rel.Kind)); err != nil {
			return fmt.Errorf("store/sqlite: save node relation: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: save node: commit: %w", err)
	}
	return nil
}

// patentInsertOrIgnoreSQL inserts a patent only when its number is new.
const patentInsertOrIgnoreSQL = `
	INSERT INTO patent (number, country, serial, kind, title, abstract, assignee,
		inventors, fetch_state, source, application_date, publication_date,
		grant_date, fetched_at, display_number,
		first_claim, expiration_date, expiration_source, source_url, classifications)
	VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(number) DO NOTHING`

// documentInsertOrIgnoreSQL inserts a document only when its number is new.
const documentInsertOrIgnoreSQL = `
	INSERT INTO document (number, record_number, country, serial, kind, stage, dated)
	VALUES (?,?,?,?,?,?,?)
	ON CONFLICT(number) DO NOTHING`

// execDocumentUpsert writes one document within a transaction.
func execDocumentUpsert(ctx context.Context, tx *sql.Tx, recordNumber domain.PatentNumber, doc domain.Document) error {
	if doc.Number.IsZero() {
		return errors.New("store/sqlite: document needs a number")
	}
	if !doc.Stage.Valid() {
		return fmt.Errorf("store/sqlite: invalid document stage %q", doc.Stage)
	}
	if _, err := tx.ExecContext(ctx, documentUpsertSQL,
		doc.Number.Normalized(), recordNumber.Normalized(),
		doc.Number.Country, doc.Number.Serial, doc.Number.Kind,
		string(doc.Stage), encodeTime(doc.Dated)); err != nil {
		return fmt.Errorf("store/sqlite: save node document %s: %w", doc.Number, err)
	}
	return nil
}
