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

func (r *Repo) saveSourceData(ctx context.Context, tx *sql.Tx, batch store.NodeBatch) error {
	now := encodeTime(time.Now().UTC())
	record := batch.Patent.Number

	for _, ident := range batch.AuthorityIdentifiers {
		if ident.RecordNumber.IsZero() {
			ident.RecordNumber = record
		}
		if ident.RecordNumber.IsZero() || ident.Authority == "" || ident.IdentifierType == "" || ident.Identifier == "" {
			continue
		}
		if ident.Confidence == 0 {
			ident.Confidence = 100
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO authority_identifier
			(authority, identifier_type, identifier, raw_identifier, record_id, document_number,
			 country, kind, dated, source, confidence, created_at, updated_at)
			VALUES (?,?,?,?, (SELECT record_id FROM patent WHERE number = ?), ?,?,?,?,?,?,?,?)
			ON CONFLICT(authority, identifier_type, identifier) DO UPDATE SET
				raw_identifier=excluded.raw_identifier,
				record_id=excluded.record_id,
				document_number=excluded.document_number,
				country=excluded.country,
				kind=excluded.kind,
				dated=excluded.dated,
				source=excluded.source,
				confidence=excluded.confidence,
				updated_at=excluded.updated_at`,
			ident.Authority, ident.IdentifierType, ident.Identifier, ident.RawIdentifier,
			ident.RecordNumber.Normalized(), ident.DocumentNumber, ident.Country, ident.Kind,
			ident.Dated, ident.Source, ident.Confidence, now, now); err != nil {
			return fmt.Errorf("store/sqlite: save authority identifier: %w", err)
		}
	}

	if app := batch.USPTOApplication; app != nil && app.ApplicationNumber != "" {
		if app.RecordNumber.IsZero() {
			app.RecordNumber = record
		}
		if app.FetchedAt == "" {
			app.FetchedAt = now
		}
		for _, table := range []string{"uspto_party", "uspto_event", "uspto_continuity", "uspto_foreign_priority", "uspto_assignment"} {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE application_number = ?`, app.ApplicationNumber); err != nil {
				return fmt.Errorf("store/sqlite: clear %s: %w", table, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO uspto_application
			(application_number, record_id, invention_title, filing_date, effective_filing_date,
			 application_status_code, application_status_text, application_status_date,
			 application_type_code, application_type_label, application_type_category,
			 first_inventor_to_file, national_stage, first_inventor_name, first_applicant_name,
			 customer_number, group_art_unit_number, examiner_name, docket_number,
			 application_confirmation_number, uspc_symbol_text, uspc_class, uspc_subclass,
			 small_entity_status, business_entity_status, publication_category_json,
			 last_ingestion_datetime, fetched_at, pgpub_xml_url, pgpub_xml_name, patent_grant_xml_url, patent_grant_xml_name)
			VALUES (?, (SELECT record_id FROM patent WHERE number = ?), ?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(application_number) DO UPDATE SET
				record_id=excluded.record_id,
				invention_title=excluded.invention_title,
				filing_date=excluded.filing_date,
				effective_filing_date=excluded.effective_filing_date,
				application_status_code=excluded.application_status_code,
				application_status_text=excluded.application_status_text,
				application_status_date=excluded.application_status_date,
				application_type_code=excluded.application_type_code,
				application_type_label=excluded.application_type_label,
				application_type_category=excluded.application_type_category,
				first_inventor_to_file=excluded.first_inventor_to_file,
				national_stage=excluded.national_stage,
				first_inventor_name=excluded.first_inventor_name,
				first_applicant_name=excluded.first_applicant_name,
				customer_number=excluded.customer_number,
				group_art_unit_number=excluded.group_art_unit_number,
				examiner_name=excluded.examiner_name,
				docket_number=excluded.docket_number,
				application_confirmation_number=excluded.application_confirmation_number,
				uspc_symbol_text=excluded.uspc_symbol_text,
				uspc_class=excluded.uspc_class,
				uspc_subclass=excluded.uspc_subclass,
				small_entity_status=excluded.small_entity_status,
				business_entity_status=excluded.business_entity_status,
				publication_category_json=excluded.publication_category_json,
				last_ingestion_datetime=excluded.last_ingestion_datetime,
				fetched_at=excluded.fetched_at,
				pgpub_xml_url=excluded.pgpub_xml_url,
				pgpub_xml_name=excluded.pgpub_xml_name,
				patent_grant_xml_url=excluded.patent_grant_xml_url,
				patent_grant_xml_name=excluded.patent_grant_xml_name`,
			app.ApplicationNumber, app.RecordNumber.Normalized(), app.InventionTitle,
			app.FilingDate, app.EffectiveFilingDate, app.ApplicationStatusCode,
			app.ApplicationStatusText, app.ApplicationStatusDate, app.ApplicationTypeCode,
			app.ApplicationTypeLabel, app.ApplicationTypeCategory, boolInt(app.FirstInventorToFile),
			boolInt(app.NationalStage), app.FirstInventorName, app.FirstApplicantName,
			app.CustomerNumber, app.GroupArtUnitNumber, app.ExaminerName, app.DocketNumber,
			app.ApplicationConfirmationNumber, app.USPCSymbolText, app.USPCClass, app.USPCSubclass,
			boolInt(app.SmallEntityStatus), app.BusinessEntityStatus, defaultJSON(app.PublicationCategoryJSON, "[]"),
			app.LastIngestionDateTime, app.FetchedAt,
			app.PGPubXMLURL, app.PGPubXMLName, app.PatentGrantXMLURL, app.PatentGrantXMLName); err != nil {
			return fmt.Errorf("store/sqlite: save uspto application: %w", err)
		}
	}

	for _, p := range batch.USPTOParties {
		if p.ApplicationNumber == "" || p.Role == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO uspto_party
			(application_number, role, ordinal, name_text, first_name, middle_name, last_name,
			 name_suffix, organization_name, registration_number, active_indicator,
			 practitioner_category, address_json, telecom_json)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.ApplicationNumber, p.Role, p.Ordinal, p.NameText, p.FirstName, p.MiddleName,
			p.LastName, p.NameSuffix, p.OrganizationName, p.RegistrationNumber,
			p.ActiveIndicator, p.PractitionerCategory, defaultJSON(p.AddressJSON, "{}"), defaultJSON(p.TelecomJSON, "{}")); err != nil {
			return fmt.Errorf("store/sqlite: save uspto party: %w", err)
		}
	}

	for _, e := range batch.USPTOEvents {
		if e.ApplicationNumber == "" || e.EventCode == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO uspto_event
			(application_number, ordinal, event_code, event_description_text, event_date)
			VALUES (?,?,?,?,?)`,
			e.ApplicationNumber, e.Ordinal, e.EventCode, e.EventDescriptionText, e.EventDate); err != nil {
			return fmt.Errorf("store/sqlite: save uspto event: %w", err)
		}
	}

	for _, c := range batch.USPTOContinuities {
		if c.ApplicationNumber == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO uspto_continuity
			(application_number, ordinal, parent_application_number_text, child_application_number_text,
			 parent_application_filing_date, parent_application_status_code, parent_application_status_text,
			 claim_parentage_type_code, claim_parentage_type_description_text, parent_record_id, child_record_id)
			VALUES (?,?,?,?,?,?,?,?,?,
			        COALESCE((SELECT record_id FROM patent WHERE number = ?), ''),
			        COALESCE((SELECT record_id FROM patent WHERE number = ?), ''))`,
			c.ApplicationNumber, c.Ordinal, c.ParentApplicationNumberText, c.ChildApplicationNumberText,
			c.ParentApplicationFilingDate, c.ParentApplicationStatusCode, c.ParentApplicationStatusText,
			c.ClaimParentageTypeCode, c.ClaimParentageTypeDescriptionText,
			numberString(c.ParentRecordNumber), numberString(c.ChildRecordNumber)); err != nil {
			return fmt.Errorf("store/sqlite: save uspto continuity: %w", err)
		}
	}

	for _, fp := range batch.USPTOForeignPriority {
		if fp.ApplicationNumber == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO uspto_foreign_priority
			(application_number, ordinal, foreign_application_number, filing_date, ip_office_name, authority, linked_record_id)
			VALUES (?,?,?,?,?,?, COALESCE((SELECT record_id FROM patent WHERE number = ?), ''))`,
			fp.ApplicationNumber, fp.Ordinal, fp.ForeignApplicationNumber, fp.FilingDate,
			fp.IPOfficeName, fp.Authority, numberString(fp.LinkedRecordNumber)); err != nil {
			return fmt.Errorf("store/sqlite: save uspto foreign priority: %w", err)
		}
	}

	for _, snap := range batch.SourceSnapshots {
		if snap.ID == "" || snap.PatentNumber.IsZero() || snap.Source == "" {
			continue
		}
		if snap.FetchedAt == "" {
			snap.FetchedAt = now
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO source_snapshot
			(id, record_id, source, source_record_id, source_url, fetched_at, payload_kind,
			 payload_hash, payload_path, response_bytes, http_status, etag, last_modified, summary_json)
			VALUES (?, (SELECT record_id FROM patent WHERE number = ?), ?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET summary_json=excluded.summary_json`,
			snap.ID, snap.PatentNumber.Normalized(), snap.Source, snap.SourceRecordID,
			snap.SourceURL, snap.FetchedAt, snap.PayloadKind, snap.PayloadHash, snap.PayloadPath,
			snap.ResponseBytes, snap.HTTPStatus, snap.ETag, snap.LastModified, defaultJSON(snap.SummaryJSON, "{}")); err != nil {
			return fmt.Errorf("store/sqlite: save source snapshot: %w", err)
		}
	}

	for _, diff := range batch.SourceDiffs {
		if diff.ID == "" || diff.PatentNumber.IsZero() || diff.FieldPath == "" {
			continue
		}
		if diff.RecordedAt == "" {
			diff.RecordedAt = now
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO source_diff
			(id, record_id, field_path, uspto_value, google_value, chosen_value, chosen_source,
			 severity, recorded_at, uspto_snapshot_id, google_snapshot_id,
			 reconciled_at, reconciled_by, reconciled_choice)
			VALUES (?, (SELECT record_id FROM patent WHERE number = ?), ?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET
				uspto_value=excluded.uspto_value,
				google_value=excluded.google_value,
				chosen_value=excluded.chosen_value,
				chosen_source=excluded.chosen_source,
				severity=excluded.severity,
				recorded_at=excluded.recorded_at,
				reconciled_at=excluded.reconciled_at,
				reconciled_by=excluded.reconciled_by,
				reconciled_choice=excluded.reconciled_choice`,
			diff.ID, diff.PatentNumber.Normalized(), diff.FieldPath, diff.USPTOValue,
			diff.GoogleValue, diff.ChosenValue, diff.ChosenSource, diff.Severity,
			diff.RecordedAt, diff.USPTOSnapshotID, diff.GoogleSnapshotID,
			diff.ReconciledAt, diff.ReconciledBy, diff.ReconciledChoice); err != nil {
			return fmt.Errorf("store/sqlite: save source diff: %w", err)
		}
	}

	return nil
}

func numberString(n domain.PatentNumber) string {
	if n.IsZero() {
		return ""
	}
	return n.Normalized()
}

func defaultJSON(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// USPTOApplication returns the saved USPTO application attrs for a patent record, or ErrNotFound.
// The result's RecordNumber field carries the patent's human-readable number,
// resolved via patent.record_id from the input number.
func (r *Repo) USPTOApplication(ctx context.Context, n domain.PatentNumber) (domain.USPTOApplication, error) {
	row := r.reader.QueryRowContext(ctx, `
		SELECT u.application_number, p.number, u.invention_title, u.filing_date, u.effective_filing_date,
			   u.application_status_code, u.application_status_text, u.application_status_date,
			   u.application_type_code, u.application_type_label, u.application_type_category,
			   u.first_inventor_to_file, u.national_stage, u.first_inventor_name, u.first_applicant_name,
			   u.customer_number, u.group_art_unit_number, u.examiner_name, u.docket_number,
			   u.application_confirmation_number, u.uspc_symbol_text, u.uspc_class, u.uspc_subclass,
			   u.small_entity_status, u.business_entity_status, u.publication_category_json,
			   u.last_ingestion_datetime, u.fetched_at, u.pgpub_xml_url, u.pgpub_xml_name, u.patent_grant_xml_url, u.patent_grant_xml_name
		FROM uspto_application u JOIN patent p ON p.record_id = u.record_id
		WHERE p.number = ?`, n.Normalized())

	var app domain.USPTOApplication
	var recordNumStr string
	var firstInventorToFile, nationalStage, smallEntityStatus int

	err := row.Scan(
		&app.ApplicationNumber, &recordNumStr, &app.InventionTitle, &app.FilingDate, &app.EffectiveFilingDate,
		&app.ApplicationStatusCode, &app.ApplicationStatusText, &app.ApplicationStatusDate,
		&app.ApplicationTypeCode, &app.ApplicationTypeLabel, &app.ApplicationTypeCategory,
		&firstInventorToFile, &nationalStage, &app.FirstInventorName, &app.FirstApplicantName,
		&app.CustomerNumber, &app.GroupArtUnitNumber, &app.ExaminerName, &app.DocketNumber,
		&app.ApplicationConfirmationNumber, &app.USPCSymbolText, &app.USPCClass, &app.USPCSubclass,
		&smallEntityStatus, &app.BusinessEntityStatus, &app.PublicationCategoryJSON,
		&app.LastIngestionDateTime, &app.FetchedAt,
		&app.PGPubXMLURL, &app.PGPubXMLName, &app.PatentGrantXMLURL, &app.PatentGrantXMLName,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.USPTOApplication{}, store.ErrNotFound
		}
		return domain.USPTOApplication{}, fmt.Errorf("store/sqlite: query uspto application: %w", err)
	}

	app.FirstInventorToFile = firstInventorToFile != 0
	app.NationalStage = nationalStage != 0
	app.SmallEntityStatus = smallEntityStatus != 0

	if parsed, err := domain.ParsePatentNumber(recordNumStr); err == nil {
		app.RecordNumber = parsed
	}

	return app, nil
}

// USPTOXMLDownload returns the per-document download record, or ErrNotFound.
func (r *Repo) USPTOXMLDownload(ctx context.Context, applicationNumber, kind string) (domain.USPTOXMLDownload, error) {
	row := r.reader.QueryRowContext(ctx, `
		SELECT application_number, kind, local_path, bytes, download_count,
		       first_downloaded_at, last_downloaded_at, last_accessed_at
		FROM uspto_xml_download
		WHERE application_number = ? AND kind = ?`, applicationNumber, kind)
	var rec domain.USPTOXMLDownload
	err := row.Scan(
		&rec.ApplicationNumber, &rec.Kind, &rec.LocalPath, &rec.Bytes, &rec.DownloadCount,
		&rec.FirstDownloadedAt, &rec.LastDownloadedAt, &rec.LastAccessedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.USPTOXMLDownload{}, store.ErrNotFound
		}
		return domain.USPTOXMLDownload{}, fmt.Errorf("store/sqlite: query uspto xml download: %w", err)
	}
	return rec, nil
}

// RecordUSPTOXMLDownload inserts or updates a per-document download record.
func (r *Repo) RecordUSPTOXMLDownload(ctx context.Context, rec domain.USPTOXMLDownload) error {
	_, err := r.writer.ExecContext(ctx, `
		INSERT INTO uspto_xml_download
		    (application_number, kind, local_path, bytes, download_count,
		     first_downloaded_at, last_downloaded_at, last_accessed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(application_number, kind) DO UPDATE SET
		    local_path=excluded.local_path,
		    bytes=excluded.bytes,
		    download_count=excluded.download_count,
		    last_downloaded_at=excluded.last_downloaded_at,
		    last_accessed_at=excluded.last_accessed_at`,
		rec.ApplicationNumber, rec.Kind, rec.LocalPath, rec.Bytes, rec.DownloadCount,
		rec.FirstDownloadedAt, rec.LastDownloadedAt, rec.LastAccessedAt,
	)
	if err != nil {
		return fmt.Errorf("store/sqlite: record uspto xml download: %w", err)
	}
	return nil
}

// ListSourceDiffs returns all recorded source comparison diffs for a patent
// (newest first by recorded_at). This is the query side for Option A
// reconciliation (the diffs hold the raw values + user Chosen + reconciled metadata).
func (r *Repo) ListSourceDiffs(ctx context.Context, patent domain.PatentNumber) ([]domain.SourceDiff, error) {
	if patent.IsZero() {
		return nil, nil
	}
	rows, err := r.reader.QueryContext(ctx, `
		SELECT d.id, p.number, d.field_path,
		       d.uspto_value, d.google_value, d.chosen_value, d.chosen_source,
		       d.severity, d.recorded_at,
		       d.uspto_snapshot_id, d.google_snapshot_id,
		       d.reconciled_at, d.reconciled_by, d.reconciled_choice
		FROM source_diff d JOIN patent p ON p.record_id = d.record_id
		WHERE p.number = ?
		ORDER BY d.recorded_at DESC, d.id ASC`, patent.Normalized())
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list source diffs: %w", err)
	}
	defer rows.Close()

	var out []domain.SourceDiff
	for rows.Next() {
		var d domain.SourceDiff
		var pnum string
		if err := rows.Scan(
			&d.ID, &pnum, &d.FieldPath,
			&d.USPTOValue, &d.GoogleValue, &d.ChosenValue, &d.ChosenSource,
			&d.Severity, &d.RecordedAt,
			&d.USPTOSnapshotID, &d.GoogleSnapshotID,
			&d.ReconciledAt, &d.ReconciledBy, &d.ReconciledChoice,
		); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan source diff: %w", err)
		}
		if pn, err := domain.ParsePatentNumber(pnum); err == nil {
			d.PatentNumber = pn
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: iterate source diffs: %w", err)
	}
	return out, nil
}

// ListSourceSnapshots returns fetch snapshots for provenance/audit (newest first).
func (r *Repo) ListSourceSnapshots(ctx context.Context, patent domain.PatentNumber) ([]domain.SourceSnapshot, error) {
	if patent.IsZero() {
		return nil, nil
	}
	rows, err := r.reader.QueryContext(ctx, `
		SELECT s.id, p.number, s.source, s.source_record_id, s.source_url, s.fetched_at,
		       s.payload_kind, s.payload_hash, s.payload_path, s.response_bytes, s.http_status,
		       s.etag, s.last_modified, s.summary_json
		FROM source_snapshot s JOIN patent p ON p.record_id = s.record_id
		WHERE p.number = ?
		ORDER BY s.fetched_at DESC, s.id ASC`, patent.Normalized())
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list source snapshots: %w", err)
	}
	defer rows.Close()

	var out []domain.SourceSnapshot
	for rows.Next() {
		var s domain.SourceSnapshot
		var pnum string
		var summary string
		if err := rows.Scan(
			&s.ID, &pnum, &s.Source, &s.SourceRecordID, &s.SourceURL, &s.FetchedAt,
			&s.PayloadKind, &s.PayloadHash, &s.PayloadPath, &s.ResponseBytes, &s.HTTPStatus,
			&s.ETag, &s.LastModified, &summary,
		); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan source snapshot: %w", err)
		}
		if pn, err := domain.ParsePatentNumber(pnum); err == nil {
			s.PatentNumber = pn
		}
		s.SummaryJSON = summary
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: iterate source snapshots: %w", err)
	}
	return out, nil
}

// ResolveAuthority looks up which patent record carries the given
// (authority, identifier_type, identifier) tuple. It is the read side of
// authority_identifier and the resolver ingest paths use to dedup patents
// already known under a different authority (Google vs USPTO).
//
// Returns store.ErrNotFound when no row matches; the lookup is read-only.
func (r *Repo) ResolveAuthority(ctx context.Context, ref domain.AuthorityRef) (id domain.RecordID, err error) {
	defer r.observeDuration("resolve_authority", time.Now(), &err)
	if ref.IsZero() {
		return "", errors.New("store/sqlite: resolve authority needs all three fields")
	}
	var recordID string
	err = r.reader.QueryRowContext(ctx, `
		SELECT record_id FROM authority_identifier
		WHERE authority = ? AND identifier_type = ? AND identifier = ?`,
		ref.Authority, ref.IdentifierType, ref.Identifier).Scan(&recordID)
	if errors.Is(err, sql.ErrNoRows) {
		if r.metrics != nil {
			r.metrics.IncCounter("store.sqlite.resolve_authority.miss", 1)
		}
		return "", store.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store/sqlite: resolve authority %s: %w", ref, err)
	}
	if r.metrics != nil {
		r.metrics.IncCounter("store.sqlite.resolve_authority.hit", 1)
	}
	return domain.RecordID(recordID), nil
}

// RecordIDForNumber returns the surrogate record_id for a patent given its
// human-readable number. It powers back-compat shims in ingest, RPC, and TUI
// paths that still pass a PatentNumber as the key.
func (r *Repo) RecordIDForNumber(ctx context.Context, n domain.PatentNumber) (id domain.RecordID, err error) {
	defer r.observeDuration("record_id_for_number", time.Now(), &err)
	if n.IsZero() {
		return "", errors.New("store/sqlite: record id lookup needs a number")
	}
	var recordID string
	err = r.reader.QueryRowContext(ctx,
		`SELECT record_id FROM patent WHERE number = ?`, n.Normalized()).Scan(&recordID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", store.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store/sqlite: record id for %s: %w", n, err)
	}
	return domain.RecordID(recordID), nil
}

