package domain

import (
	"fmt"
	"strings"
	"time"
)

// DeadlineKind classifies a tracked due date. The workspace unifies prosecution
// response deadlines and post-grant maintenance/renewal fees behind one model so
// a single reminder engine surfaces them all.
type DeadlineKind string

const (
	// DeadlineOAResponse is the statutory deadline to respond to an office action.
	DeadlineOAResponse DeadlineKind = "oa_response"
	// DeadlineMaintenanceFee is a U.S. post-grant maintenance fee window.
	DeadlineMaintenanceFee DeadlineKind = "maintenance_fee"
	// DeadlineAnnuity is a foreign renewal annuity (jurisdiction-specific).
	DeadlineAnnuity DeadlineKind = "annuity"
	// DeadlineCustom is any other user-entered date.
	DeadlineCustom DeadlineKind = "custom"
)

// Valid reports whether the DeadlineKind is a known value.
func (k DeadlineKind) Valid() bool {
	switch k {
	case DeadlineOAResponse, DeadlineMaintenanceFee, DeadlineAnnuity, DeadlineCustom:
		return true
	default:
		return false
	}
}

// Label returns the human-readable name of a deadline kind for the TUI.
func (k DeadlineKind) Label() string {
	switch k {
	case DeadlineOAResponse:
		return "OA response"
	case DeadlineMaintenanceFee:
		return "Maintenance fee"
	case DeadlineAnnuity:
		return "Annuity"
	case DeadlineCustom:
		return "Custom"
	default:
		return string(k)
	}
}

// DeadlineStatus is the lifecycle of a tracked deadline. A pending deadline is
// reminded; a done (paid/filed) or dismissed one is not.
type DeadlineStatus string

const (
	DeadlinePending   DeadlineStatus = "pending"
	DeadlineDone      DeadlineStatus = "done"
	DeadlineDismissed DeadlineStatus = "dismissed"
)

// Valid reports whether the DeadlineStatus is a known value.
func (s DeadlineStatus) Valid() bool {
	switch s {
	case DeadlinePending, DeadlineDone, DeadlineDismissed:
		return true
	default:
		return false
	}
}

// ParseDeadlineStatus converts a string into a DeadlineStatus.
func ParseDeadlineStatus(s string) (DeadlineStatus, error) {
	v := DeadlineStatus(strings.TrimSpace(strings.ToLower(s)))
	if !v.Valid() {
		return "", fmt.Errorf("domain: unknown deadline status %q", s)
	}
	return v, nil
}

// Deadline is one tracked due date, owned by a patent (maintenance/annuity), a
// matter/project (custom), or an office action (response). It is the single
// thing the reminder engine watches. WindowOpens and GraceEnds bound the period
// around DueDate when relevant (a maintenance fee can be paid in the 6-month
// window before the anniversary and, with a surcharge, in the 6-month grace
// after).
type Deadline struct {
	ID             string         `json:"id"`
	Kind           DeadlineKind   `json:"kind"`
	PatentNumber   string         `json:"patent_number,omitempty"`
	Project        ProjectID      `json:"project,omitempty"`
	OfficeActionID string         `json:"office_action_id,omitempty"`
	Title          string         `json:"title"`
	WindowOpens    time.Time      `json:"window_opens,omitempty"`
	DueDate        time.Time      `json:"due_date"`
	GraceEnds      time.Time      `json:"grace_ends,omitempty"`
	Status         DeadlineStatus `json:"status"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// DaysUntilDue reports whole days from now until the due date (negative when
// overdue). Zero when no due date is set.
func (d Deadline) DaysUntilDue() int {
	if d.DueDate.IsZero() {
		return 0
	}
	return int(time.Until(d.DueDate).Hours() / 24)
}

// usMaintenanceYears are the anniversaries (from grant) at which a U.S. utility
// patent's maintenance fees fall due (37 CFR 1.362).
var usMaintenanceYears = []struct {
	stage  string
	months int // months from grant to the anniversary deadline
}{
	{"1st (3.5 yr)", 42},
	{"2nd (7.5 yr)", 90},
	{"3rd (11.5 yr)", 138},
}

// USMaintenanceDeadlines derives the three U.S. maintenance-fee deadlines for a
// utility patent granted on grantDate. The fee may be paid without surcharge in
// the 6-month window before each anniversary (WindowOpens), is due at the
// anniversary (DueDate), and may still be paid with a surcharge through the
// 6-month grace after (GraceEnds). Dates only — fee amounts depend on entity
// size and the current USPTO schedule and are intentionally not encoded here.
// Returns nil for a zero grant date.
func USMaintenanceDeadlines(patentNumber string, grantDate time.Time) []Deadline {
	if grantDate.IsZero() {
		return nil
	}
	now := time.Now().UTC()
	out := make([]Deadline, 0, len(usMaintenanceYears))
	for _, m := range usMaintenanceYears {
		due := grantDate.AddDate(0, m.months, 0)
		out = append(out, Deadline{
			Kind:         DeadlineMaintenanceFee,
			PatentNumber: patentNumber,
			Title:        patentNumber + " maintenance fee " + m.stage,
			WindowOpens:  due.AddDate(0, -6, 0),
			DueDate:      due,
			GraceEnds:    due.AddDate(0, 6, 0),
			Status:       DeadlinePending,
			CreatedAt:    now,
			UpdatedAt:    now,
		})
	}
	return out
}
