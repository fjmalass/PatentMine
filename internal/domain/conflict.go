package domain

import (
	"fmt"
	"strings"
	"time"
)

// ConflictStatus is the lifecycle of a flagged conflict. A conflict opens when an
// examiner, attorney, or search turns up prior art (or another record/document)
// that is material to the matter; it closes either resolved (addressed in
// prosecution) or waived (judged immaterial after review).
type ConflictStatus string

const (
	ConflictOpen     ConflictStatus = "open"
	ConflictResolved ConflictStatus = "resolved"
	ConflictWaived   ConflictStatus = "waived"
)

// Valid reports whether the ConflictStatus is a known value.
func (s ConflictStatus) Valid() bool {
	switch s {
	case ConflictOpen, ConflictResolved, ConflictWaived:
		return true
	default:
		return false
	}
}

// Label returns the human-readable status for the TUI.
func (s ConflictStatus) Label() string {
	switch s {
	case ConflictOpen:
		return "Open"
	case ConflictResolved:
		return "Resolved"
	case ConflictWaived:
		return "Waived"
	default:
		return string(s)
	}
}

// ParseConflictStatus converts a string into a ConflictStatus, defaulting an
// empty string to ConflictOpen.
func ParseConflictStatus(s string) (ConflictStatus, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ConflictOpen, nil
	}
	st := ConflictStatus(s)
	if !st.Valid() {
		return "", fmt.Errorf("domain: unknown conflict status %q", s)
	}
	return st, nil
}

// Conflict is a project-scoped, user-asserted edge marking one record (a loaded
// patent, application, or prior-art reference) as conflicting within a matter —
// material prior art, an interfering application, a reference that reads on the
// claims. It is deliberately NOT a row in the global family-graph `relation`
// table: that graph is global, immutable, and record↔record only, whereas a
// conflict is project-scoped, annotated (Reason), and has a lifecycle (Status).
//
// Record is the flagged record. Against and DocumentID are optional second
// endpoints: Against names what it conflicts with (empty = the matter's subject
// application), and DocumentID links a matter document instead of/along with a
// record, so the same edge type spans patent↔patent and reference↔document.
type Conflict struct {
	ID         string         `json:"id"`
	Project    ProjectID      `json:"project"`
	Record     PatentNumber   `json:"record"`
	Against    PatentNumber   `json:"against,omitempty"`
	DocumentID string         `json:"document_id,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	Status     ConflictStatus `json:"status"`
	FlaggedBy  string         `json:"flagged_by,omitempty"`
	FlaggedAt  time.Time      `json:"flagged_at"`
	ResolvedAt time.Time      `json:"resolved_at,omitempty"`
}
