package domain

import "time"

// MutationGroup records one user-visible batch mutation so it can be audited or replayed later.
type MutationGroup struct {
	ID                string
	Project           ProjectID
	Action            string
	CreatedAt         time.Time
	SelectionSnapshot []PatentNumber
	Attributes          map[string]any
}

// MutationItem records one patent-level change inside a MutationGroup.
type MutationItem struct {
	Patent  PatentNumber
	Kind    string
	Ordinal int
	Before  any
	After   any
	Inverse any
}
