package domain

import "time"

// ProjectID is the stable identifier of a project. A distinct type (not a bare
// string) keeps it from being mixed up with patent numbers or other IDs.
type ProjectID string

// Project groups patents for a body of work — typically one patent-office
// filing for which an IDS will be generated.
type Project struct {
	ID        ProjectID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	// Patent-office metadata used when rendering an IDS PDF (PTO/SB/08a etc.).
	// Empty until the user supplies it from the project metadata overlay.
	ApplicationNumber    string    `json:"application_number,omitempty"`
	FilingDate           time.Time `json:"filing_date"`
	FirstNamedInventor   string    `json:"first_named_inventor,omitempty"`
	ArtUnit              string    `json:"art_unit,omitempty"`
	ExaminerName         string    `json:"examiner_name,omitempty"`
	AttorneyDocketNumber string    `json:"attorney_docket_number,omitempty"`
}

// Membership links one patent to one project and carries its workflow state.
// A patent in several projects has one Membership per project.
type Membership struct {
	Patent      PatentNumber `json:"patent"`
	Project     ProjectID    `json:"project"`
	ReviewState ReviewState  `json:"review_state"`
	AddedAt     time.Time    `json:"added_at"`
}
