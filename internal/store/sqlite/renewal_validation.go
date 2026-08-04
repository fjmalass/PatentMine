package sqlite

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"patentmine/internal/domain"
)

// SavePatentValidations upserts country-phase validation rows for a patent.
func (r *Repo) SavePatentValidations(ctx context.Context, validations []domain.PatentValidation) (err error) {
	defer r.observeDuration("save_patent_validations", time.Now(), &err)
	if len(validations) == 0 {
		return nil
	}
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: save patent validations tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO patent_validation
		(patent_number, country, status, source, certainty, event_code, event_date, last_checked_at, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(patent_number, country) DO UPDATE SET
			status = excluded.status,
			source = excluded.source,
			certainty = excluded.certainty,
			event_code = excluded.event_code,
			event_date = excluded.event_date,
			last_checked_at = excluded.last_checked_at,
			notes = excluded.notes`)
	if err != nil {
		return fmt.Errorf("store/sqlite: save patent validations prepare: %w", err)
	}
	defer stmt.Close()

	for _, v := range validations {
		if v.PatentNumber.IsZero() {
			return errors.New("store/sqlite: cannot save validation with empty patent number")
		}
		country := strings.ToUpper(strings.TrimSpace(v.Country))
		if country == "" {
			return errors.New("store/sqlite: cannot save validation with empty country")
		}
		if !v.Status.Valid() {
			return fmt.Errorf("store/sqlite: invalid validation status %q", v.Status)
		}
		if v.Certainty == "" {
			v.Certainty = domain.RenewalCertaintyDerived
		}
		if !v.Certainty.Valid() {
			return fmt.Errorf("store/sqlite: invalid validation certainty %q", v.Certainty)
		}
		if _, err := stmt.ExecContext(ctx,
			v.PatentNumber.Normalized(), country, string(v.Status), v.Source, string(v.Certainty),
			v.EventCode, encodeTime(v.EventDate), encodeTime(v.LastCheckedAt), v.Notes); err != nil {
			return fmt.Errorf("store/sqlite: save validation %s/%s: %w", v.PatentNumber, country, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: save patent validations commit: %w", err)
	}
	return nil
}

// PatentValidations returns country-phase validation rows for a patent.
func (r *Repo) PatentValidations(ctx context.Context, number domain.PatentNumber) (out []domain.PatentValidation, err error) {
	defer r.observeDuration("patent_validations", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx, `SELECT patent_number, country, status, source, certainty,
		event_code, event_date, last_checked_at, notes
		FROM patent_validation WHERE patent_number = ? ORDER BY country`, number.Normalized())
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list validations %s: %w", number, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var v domain.PatentValidation
		var patentNumber, status, certainty, eventDate, checkedAt string
		if err := rows.Scan(&patentNumber, &v.Country, &status, &v.Source, &certainty, &v.EventCode, &eventDate, &checkedAt, &v.Notes); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan validation: %w", err)
		}
		if n, err := domain.ParsePatentNumber(patentNumber); err == nil {
			v.PatentNumber = n
		} else {
			v.PatentNumber = number
		}
		v.Status = domain.RenewalValidationStatus(status)
		v.Certainty = domain.RenewalCertainty(certainty)
		v.EventDate, _ = decodeTime(eventDate)
		v.LastCheckedAt, _ = decodeTime(checkedAt)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: list validations %s: %w", number, err)
	}
	return out, nil
}

// SaveRenewalLegalEvents appends or refreshes raw legal-status events.
func (r *Repo) SaveRenewalLegalEvents(ctx context.Context, events []domain.RenewalLegalEvent) (err error) {
	defer r.observeDuration("save_renewal_legal_events", time.Now(), &err)
	if len(events) == 0 {
		return nil
	}
	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: save legal events tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO renewal_legal_event
		(id, patent_number, authority, country, code, description, event_date, raw_xml, fetched_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET raw_xml = excluded.raw_xml, fetched_at = excluded.fetched_at`)
	if err != nil {
		return fmt.Errorf("store/sqlite: save legal events prepare: %w", err)
	}
	defer stmt.Close()
	for _, e := range events {
		if e.PatentNumber.IsZero() {
			return errors.New("store/sqlite: cannot save legal event with empty patent number")
		}
		if strings.TrimSpace(e.Authority) == "" {
			return errors.New("store/sqlite: cannot save legal event with empty authority")
		}
		if e.FetchedAt.IsZero() {
			e.FetchedAt = time.Now().UTC()
		}
		id := renewalLegalEventID(e)
		if _, err := stmt.ExecContext(ctx, id, e.PatentNumber.Normalized(), e.Authority, strings.ToUpper(e.Country),
			e.Code, e.Description, encodeTime(e.EventDate), e.RawXML, encodeTime(e.FetchedAt)); err != nil {
			return fmt.Errorf("store/sqlite: save legal event %s: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: save legal events commit: %w", err)
	}
	return nil
}

func (r *Repo) RenewalLegalEvents(ctx context.Context, number domain.PatentNumber) (out []domain.RenewalLegalEvent, err error) {
	defer r.observeDuration("renewal_legal_events", time.Now(), &err)
	rows, err := r.reader.QueryContext(ctx, `SELECT patent_number, authority, country, code, description,
		event_date, raw_xml, fetched_at FROM renewal_legal_event
		WHERE patent_number = ? ORDER BY event_date DESC, country, code`, number.Normalized())
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list legal events %s: %w", number, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var e domain.RenewalLegalEvent
		var patentNumber, eventDate, fetchedAt string
		if err := rows.Scan(&patentNumber, &e.Authority, &e.Country, &e.Code, &e.Description, &eventDate, &e.RawXML, &fetchedAt); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan legal event: %w", err)
		}
		if n, err := domain.ParsePatentNumber(patentNumber); err == nil {
			e.PatentNumber = n
		} else {
			e.PatentNumber = number
		}
		e.EventDate, _ = decodeTime(eventDate)
		e.FetchedAt, _ = decodeTime(fetchedAt)
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: list legal events %s: %w", number, err)
	}
	return out, nil
}

func renewalLegalEventID(e domain.RenewalLegalEvent) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(e.PatentNumber.Normalized() + "\x00" + e.Authority + "\x00" + strings.ToUpper(e.Country) + "\x00" + e.Code + "\x00" + e.Description + "\x00" + encodeTime(e.EventDate)))
	return fmt.Sprintf("rle-%016x", h.Sum64())
}
