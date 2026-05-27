package sqlite

import (
	"context"
	"fmt"

	"patentmine/internal/domain"
)

// SaveUSPTOAssignments writes every recorded assignment for one application
// as a full replacement: prior rows for the application_number are deleted,
// then the new bundle is inserted. uspto_assignment_party rows cascade with
// their parent assignment ordinal so they vanish on the same delete. The
// whole save is one transaction.
func (r *Repo) SaveUSPTOAssignments(ctx context.Context, applicationNumber string, assignments []domain.USPTOAssignment) error {
	if applicationNumber == "" {
		return fmt.Errorf("store/sqlite: save uspto assignments: missing application number")
	}

	tx, err := r.writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: save uspto assignments: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// CASCADE on FK clears uspto_assignment_party rows when assignment rows
	// are deleted, so one DELETE clears both tables.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM uspto_assignment WHERE application_number = ?`, applicationNumber); err != nil {
		return fmt.Errorf("store/sqlite: clear uspto_assignment: %w", err)
	}

	for _, a := range assignments {
		imageAvail := 0
		if a.ImageAvailable {
			imageAvail = 1
		}
		corresp := a.CorrespondenceAddressJSON
		if corresp == "" {
			corresp = "{}"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO uspto_assignment
			    (application_number, ordinal, reel_and_frame_number, reel_number, frame_number,
			     conveyance_text, assignment_received_date, assignment_recorded_date,
			     assignment_mailed_date, assignment_document_location_uri,
			     attorney_docket_number, page_total_quantity, image_available,
			     correspondence_address_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			applicationNumber, a.Ordinal, a.ReelAndFrameNumber, a.ReelNumber, a.FrameNumber,
			a.ConveyanceText, a.AssignmentReceivedDate, a.AssignmentRecordedDate,
			a.AssignmentMailedDate, a.AssignmentDocumentLocationURI,
			a.AttorneyDocketNumber, a.PageTotalQuantity, imageAvail,
			corresp,
		); err != nil {
			return fmt.Errorf("store/sqlite: save uspto assignment %d: %w", a.Ordinal, err)
		}
		for _, p := range a.Parties {
			addr := p.AddressJSON
			if addr == "" {
				addr = "{}"
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO uspto_assignment_party
				    (application_number, assignment_ordinal, role, ordinal,
				     name_text, execution_date, address_json)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				applicationNumber, a.Ordinal, p.Role, p.Ordinal,
				p.NameText, p.ExecutionDate, addr,
			); err != nil {
				return fmt.Errorf("store/sqlite: save uspto assignment party %d/%s/%d: %w",
					a.Ordinal, p.Role, p.Ordinal, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: save uspto assignments: commit: %w", err)
	}
	return nil
}

// USPTOAssignments returns every recorded assignment for one application in
// document order, with parties attached.
func (r *Repo) USPTOAssignments(ctx context.Context, applicationNumber string) ([]domain.USPTOAssignment, error) {
	if applicationNumber == "" {
		return nil, nil
	}
	rows, err := r.reader.QueryContext(ctx, `
		SELECT ordinal, reel_and_frame_number, reel_number, frame_number,
		       conveyance_text, assignment_received_date, assignment_recorded_date,
		       assignment_mailed_date, assignment_document_location_uri,
		       attorney_docket_number, page_total_quantity, image_available,
		       correspondence_address_json
		FROM uspto_assignment
		WHERE application_number = ?
		ORDER BY ordinal`, applicationNumber)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list uspto_assignment: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []domain.USPTOAssignment
	for rows.Next() {
		var a domain.USPTOAssignment
		var imgAvail int
		a.ApplicationNumber = applicationNumber
		if err := rows.Scan(
			&a.Ordinal, &a.ReelAndFrameNumber, &a.ReelNumber, &a.FrameNumber,
			&a.ConveyanceText, &a.AssignmentReceivedDate, &a.AssignmentRecordedDate,
			&a.AssignmentMailedDate, &a.AssignmentDocumentLocationURI,
			&a.AttorneyDocketNumber, &a.PageTotalQuantity, &imgAvail,
			&a.CorrespondenceAddressJSON,
		); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan uspto_assignment: %w", err)
		}
		a.ImageAvailable = imgAvail != 0
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store/sqlite: iterate uspto_assignment: %w", err)
	}

	for i := range out {
		prows, err := r.reader.QueryContext(ctx, `
			SELECT role, ordinal, name_text, execution_date, address_json
			FROM uspto_assignment_party
			WHERE application_number = ? AND assignment_ordinal = ?
			ORDER BY role, ordinal`, applicationNumber, out[i].Ordinal)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: list uspto_assignment_party: %w", err)
		}
		for prows.Next() {
			p := domain.USPTOAssignmentParty{
				ApplicationNumber: applicationNumber,
				AssignmentOrdinal: out[i].Ordinal,
			}
			if err := prows.Scan(&p.Role, &p.Ordinal, &p.NameText, &p.ExecutionDate, &p.AddressJSON); err != nil {
				_ = prows.Close()
				return nil, fmt.Errorf("store/sqlite: scan uspto_assignment_party: %w", err)
			}
			out[i].Parties = append(out[i].Parties, p)
		}
		_ = prows.Close()
	}
	return out, nil
}
