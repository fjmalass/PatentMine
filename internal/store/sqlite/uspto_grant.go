package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

// SaveUSPTOGrantIngest writes a parsed grant XML bundle atomically.
//
// The summary columns live on uspto_application — the parser must already have
// observed an application row (the FK requires it). Body, drawings, citations,
// classifications, and relations are full replacements for the (application,
// kind) pair: every prior row for the pair is deleted before inserts. This is
// the right semantics for a re-ingestion of the same XML.
func (r *Repo) SaveUSPTOGrantIngest(ctx context.Context, ingest domain.USPTOGrantIngest) error {
	app := strings.TrimSpace(ingest.Summary.ApplicationNumber)
	if app == "" {
		return fmt.Errorf("store/sqlite: save uspto grant: missing application number")
	}
	kind := ingest.Body.Kind
	if kind == "" {
		return fmt.Errorf("store/sqlite: save uspto grant: missing kind")
	}

	assistantsJSON, err := json.Marshal(ingest.Summary.AssistantExaminers)
	if err != nil {
		return fmt.Errorf("store/sqlite: save uspto grant: marshal examiners: %w", err)
	}
	fieldsJSON, err := json.Marshal(ingest.Summary.FieldOfSearch)
	if err != nil {
		return fmt.Errorf("store/sqlite: save uspto grant: marshal field of search: %w", err)
	}
	claimsJSON, err := json.Marshal(ingest.Body.Claims)
	if err != nil {
		return fmt.Errorf("store/sqlite: save uspto grant: marshal claims: %w", err)
	}

	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: save uspto grant: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// invention_title is set by both grant XML and the ODP catalog. Keep
	// whichever was already there if the XML one is empty (COALESCE on the
	// NULL-IF-empty pattern), otherwise let the XML overwrite.
	if _, err := tx.ExecContext(ctx, `
		UPDATE uspto_application SET
			invention_title = COALESCE(NULLIF(?, ''), invention_title),
			grant_doc_number = ?,
			grant_kind = ?,
			grant_date = ?,
			grant_dtd_version = ?,
			grant_status = ?,
			grant_date_produced = ?,
			grant_file_name = ?,
			grant_lang = ?,
			term_extension_days = ?,
			number_of_claims = ?,
			exemplary_claim = ?,
			number_of_drawing_sheets = ?,
			number_of_figures = ?,
			primary_examiner_first = ?,
			primary_examiner_last = ?,
			primary_examiner_department = ?,
			assistant_examiners_json = ?,
			attorney_org = ?,
			attorney_type = ?,
			field_of_search_json = ?,
			grant_parsed_at = ?
		WHERE application_number = ?`,
		ingest.Summary.InventionTitle,
		ingest.Summary.GrantDocNumber, ingest.Summary.GrantKind, ingest.Summary.GrantDate,
		ingest.Summary.GrantDTDVersion, ingest.Summary.GrantStatus, ingest.Summary.GrantDateProduced,
		ingest.Summary.GrantFileName, ingest.Summary.GrantLang,
		ingest.Summary.TermExtensionDays, ingest.Summary.NumberOfClaims, ingest.Summary.ExemplaryClaim,
		ingest.Summary.NumberOfDrawingSheets, ingest.Summary.NumberOfFigures,
		ingest.Summary.PrimaryExaminerFirst, ingest.Summary.PrimaryExaminerLast, ingest.Summary.PrimaryExaminerDept,
		string(assistantsJSON),
		ingest.Summary.AttorneyOrg, ingest.Summary.AttorneyType,
		string(fieldsJSON), ingest.Summary.ParsedAt,
		app,
	); err != nil {
		return fmt.Errorf("store/sqlite: save uspto grant summary: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO uspto_grant_body
		    (application_number, kind, abstract_text, abstract_xml, description_text, description_xml,
		     claim_statement, claims_text, claims_json, parsed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(application_number, kind) DO UPDATE SET
		    abstract_text=excluded.abstract_text,
		    abstract_xml=excluded.abstract_xml,
		    description_text=excluded.description_text,
		    description_xml=excluded.description_xml,
		    claim_statement=excluded.claim_statement,
		    claims_text=excluded.claims_text,
		    claims_json=excluded.claims_json,
		    parsed_at=excluded.parsed_at`,
		app, kind, ingest.Body.AbstractText, ingest.Body.AbstractXML,
		ingest.Body.DescriptionText, ingest.Body.DescriptionXML,
		ingest.Body.ClaimStatement, ingest.Body.ClaimsText, string(claimsJSON), ingest.Body.ParsedAt,
	); err != nil {
		return fmt.Errorf("store/sqlite: save uspto grant body: %w", err)
	}

	if err := replaceChildRows(ctx, tx, "uspto_drawing", app, kind); err != nil {
		return err
	}
	for _, d := range ingest.Drawings {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO uspto_drawing
			    (application_number, kind, ordinal, figure_num, figure_id, img_id,
			     file_name, width, height, alt_text, img_format, img_content)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			app, kind, d.Ordinal, d.FigureNum, d.FigureID, d.ImgID,
			d.FileName, d.Width, d.Height, d.AltText, d.ImgFormat, d.ImgContent,
		); err != nil {
			return fmt.Errorf("store/sqlite: save uspto drawing %d: %w", d.Ordinal, err)
		}
	}

	if err := replaceChildRows(ctx, tx, "uspto_grant_citation", app, kind); err != nil {
		return err
	}
	for _, c := range ingest.Citations {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO uspto_grant_citation
			    (application_number, kind, ordinal, citation_num, citation_type, category,
			     cited_country, cited_doc_number, cited_kind, cited_date, cited_name,
			     cpc_text, national_country, national_class, npl_text)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			app, kind, c.Ordinal, c.CitationNum, c.CitationType, c.Category,
			c.CitedCountry, c.CitedDocNumber, c.CitedKind, c.CitedDate, c.CitedName,
			c.CPCText, c.NationalCountry, c.NationalClass, c.NPLText,
		); err != nil {
			return fmt.Errorf("store/sqlite: save uspto citation %d: %w", c.Ordinal, err)
		}
	}

	if err := replaceChildRows(ctx, tx, "uspto_grant_classification", app, kind); err != nil {
		return err
	}
	for _, c := range ingest.Classifications {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO uspto_grant_classification
			    (application_number, kind, scheme, role, ordinal, full_code,
			     section, class, subclass, main_group, subgroup, symbol_position,
			     classification_value, classification_level, classification_status,
			     data_source, action_date, generating_office, version_date, scheme_origination)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			app, kind, c.Scheme, c.Role, c.Ordinal, c.FullCode,
			c.Section, c.Class, c.Subclass, c.MainGroup, c.Subgroup, c.SymbolPosition,
			c.ClassificationValue, c.ClassificationLevel, c.ClassificationStatus,
			c.DataSource, c.ActionDate, c.GeneratingOffice, c.VersionDate, c.SchemeOrigination,
		); err != nil {
			return fmt.Errorf("store/sqlite: save uspto classification %d: %w", c.Ordinal, err)
		}
	}

	if err := replaceChildRows(ctx, tx, "uspto_grant_relation", app, kind); err != nil {
		return err
	}
	for _, rel := range ingest.Relations {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO uspto_grant_relation
			    (application_number, kind, ordinal, relation_kind,
			     parent_country, parent_app_number, parent_app_date,
			     parent_grant_country, parent_grant_number, parent_grant_date,
			     child_country, child_app_number,
			     related_country, related_doc_number, related_kind, related_date)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			app, kind, rel.Ordinal, rel.RelationKind,
			rel.ParentCountry, rel.ParentAppNumber, rel.ParentAppDate,
			rel.ParentGrantCountry, rel.ParentGrantNumber, rel.ParentGrantDate,
			rel.ChildCountry, rel.ChildAppNumber,
			rel.RelatedCountry, rel.RelatedDocNumber, rel.RelatedKind, rel.RelatedDate,
		); err != nil {
			return fmt.Errorf("store/sqlite: save uspto relation %d: %w", rel.Ordinal, err)
		}
	}

	if err := replaceChildRows(ctx, tx, "uspto_grant_party", app, kind); err != nil {
		return err
	}
	for _, p := range ingest.Parties {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO uspto_grant_party
			    (application_number, kind, role, ordinal, sequence, designation,
			     app_type, rep_type, org_name, first_name, last_name,
			     residence_country, address_street, address_city, address_state,
			     address_country, address_postal, assignee_role)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			app, kind, p.Role, p.Ordinal, p.Sequence, p.Designation,
			p.AppType, p.RepType, p.OrgName, p.FirstName, p.LastName,
			p.ResidenceCountry, p.AddressStreet, p.AddressCity, p.AddressState,
			p.AddressCountry, p.AddressPostal, p.AssigneeRole,
		); err != nil {
			return fmt.Errorf("store/sqlite: save uspto grant party %s/%d: %w", p.Role, p.Ordinal, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: save uspto grant: commit: %w", err)
	}
	return nil
}

// replaceChildRows clears any prior rows for (application_number, kind) on a
// child table so the subsequent inserts are a full replacement.
func replaceChildRows(ctx context.Context, tx *sql.Tx, table, app, kind string) error {
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf("DELETE FROM %s WHERE application_number = ? AND kind = ?", table),
		app, kind); err != nil {
		return fmt.Errorf("store/sqlite: clear %s: %w", table, err)
	}
	return nil
}

// USPTOGrantParties returns every party (applicant/inventor/assignee/agent)
// extracted from the XML for one (application_number, kind), ordered by role
// then ordinal so the original document order is preserved within each role.
func (r *Repo) USPTOGrantParties(ctx context.Context, applicationNumber, kind string) ([]domain.USPTOGrantParty, error) {
	rows, err := r.reader.QueryContext(ctx, `
		SELECT application_number, kind, role, ordinal, sequence, designation,
		       app_type, rep_type, org_name, first_name, last_name,
		       residence_country, address_street, address_city, address_state,
		       address_country, address_postal, assignee_role
		FROM uspto_grant_party
		WHERE application_number = ? AND kind = ?
		ORDER BY role, ordinal`, applicationNumber, kind)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: query uspto grant party: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []domain.USPTOGrantParty
	for rows.Next() {
		var p domain.USPTOGrantParty
		if err := rows.Scan(
			&p.ApplicationNumber, &p.Kind, &p.Role, &p.Ordinal, &p.Sequence, &p.Designation,
			&p.AppType, &p.RepType, &p.OrgName, &p.FirstName, &p.LastName,
			&p.ResidenceCountry, &p.AddressStreet, &p.AddressCity, &p.AddressState,
			&p.AddressCountry, &p.AddressPostal, &p.AssigneeRole,
		); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan uspto grant party: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: iterate uspto grant party: %w", err)
	}
	return out, nil
}

// USPTOGrantBody returns the body extracted from the XML.
func (r *Repo) USPTOGrantBody(ctx context.Context, applicationNumber, kind string) (domain.USPTOGrantBody, error) {
	row := r.reader.QueryRowContext(ctx, `
		SELECT application_number, kind, abstract_text, abstract_xml,
		       description_text, description_xml, claim_statement,
		       claims_text, claims_json, parsed_at
		FROM uspto_grant_body
		WHERE application_number = ? AND kind = ?`, applicationNumber, kind)
	var body domain.USPTOGrantBody
	var claimsJSON string
	err := row.Scan(
		&body.ApplicationNumber, &body.Kind, &body.AbstractText, &body.AbstractXML,
		&body.DescriptionText, &body.DescriptionXML, &body.ClaimStatement,
		&body.ClaimsText, &claimsJSON, &body.ParsedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.USPTOGrantBody{}, store.ErrNotFound
		}
		return domain.USPTOGrantBody{}, fmt.Errorf("store/sqlite: query uspto grant body: %w", err)
	}
	if claimsJSON != "" {
		if err := json.Unmarshal([]byte(claimsJSON), &body.Claims); err != nil {
			return domain.USPTOGrantBody{}, fmt.Errorf("store/sqlite: decode claims: %w", err)
		}
	}
	return body, nil
}

// ClearUSPTOGrantBodies clears cached parsed patent bodies in uspto_grant_body for the given application numbers.
// If applicationNumbers is empty, it clears all cached patent bodies in the table.
// It returns the number of rows deleted, total text memory (bytes) saved, and any error.
func (r *Repo) ClearUSPTOGrantBodies(ctx context.Context, applicationNumbers []string) (int64, int64, error) {
	start := time.Now()
	var rowsDeleted, bytesSaved int64
	var opErr error
	defer func() {
		if r.metrics != nil {
			r.metrics.ObserveDuration("store.sqlite.clear_cache.duration", time.Since(start), opErr != nil)
			r.metrics.IncCounter("store.sqlite.clear_cache.count", 1)
			if opErr == nil {
				r.metrics.IncCounter("store.sqlite.clear_cache.rows_cleared", rowsDeleted)
				r.metrics.IncCounter("store.sqlite.clear_cache.bytes_saved", bytesSaved)
			}
		}
	}()

	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		opErr = fmt.Errorf("store/sqlite: begin clear uspto grant: %w", err)
		return 0, 0, opErr
	}
	defer func() { _ = tx.Rollback() }()

	if len(applicationNumbers) == 0 {
		// Calculate memory saved
		querySize := `
			SELECT COALESCE(SUM(
				LENGTH(COALESCE(abstract_text, '')) + 
				LENGTH(COALESCE(abstract_xml, '')) + 
				LENGTH(COALESCE(description_text, '')) + 
				LENGTH(COALESCE(description_xml, '')) + 
				LENGTH(COALESCE(claim_statement, '')) + 
				LENGTH(COALESCE(claims_text, '')) + 
				LENGTH(COALESCE(claims_json, ''))
			), 0) FROM uspto_grant_body`
		if err := tx.QueryRowContext(ctx, querySize).Scan(&bytesSaved); err != nil {
			opErr = fmt.Errorf("store/sqlite: calc bytes saved: %w", err)
			return 0, 0, opErr
		}

		res, err := tx.ExecContext(ctx, "DELETE FROM uspto_grant_body")
		if err != nil {
			opErr = fmt.Errorf("store/sqlite: delete all cached parsed bodies: %w", err)
			return 0, 0, opErr
		}
		rowsDeleted, err = res.RowsAffected()
		if err != nil {
			opErr = fmt.Errorf("store/sqlite: get rows affected: %w", err)
			return 0, 0, opErr
		}
	} else {
		// Dynamic IN clause
		placeholders := make([]string, len(applicationNumbers))
		args := make([]any, len(applicationNumbers))
		for i, appNum := range applicationNumbers {
			placeholders[i] = "?"
			args[i] = appNum
		}

		querySize := fmt.Sprintf(`
			SELECT COALESCE(SUM(
				LENGTH(COALESCE(abstract_text, '')) + 
				LENGTH(COALESCE(abstract_xml, '')) + 
				LENGTH(COALESCE(description_text, '')) + 
				LENGTH(COALESCE(description_xml, '')) + 
				LENGTH(COALESCE(claim_statement, '')) + 
				LENGTH(COALESCE(claims_text, '')) + 
				LENGTH(COALESCE(claims_json, ''))
			), 0) FROM uspto_grant_body WHERE application_number IN (%s)`, strings.Join(placeholders, ","))
		if err := tx.QueryRowContext(ctx, querySize, args...).Scan(&bytesSaved); err != nil {
			opErr = fmt.Errorf("store/sqlite: calc bytes saved bulk: %w", err)
			return 0, 0, opErr
		}

		queryDel := fmt.Sprintf("DELETE FROM uspto_grant_body WHERE application_number IN (%s)", strings.Join(placeholders, ","))
		res, err := tx.ExecContext(ctx, queryDel, args...)
		if err != nil {
			opErr = fmt.Errorf("store/sqlite: delete cached parsed bodies bulk: %w", err)
			return 0, 0, opErr
		}
		rowsDeleted, err = res.RowsAffected()
		if err != nil {
			opErr = fmt.Errorf("store/sqlite: get rows affected bulk: %w", err)
			return 0, 0, opErr
		}
	}

	if err := tx.Commit(); err != nil {
		opErr = fmt.Errorf("store/sqlite: commit clear uspto grant: %w", err)
		return 0, 0, opErr
	}

	return rowsDeleted, bytesSaved, nil
}

