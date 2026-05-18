package domain

import (
	"fmt"
	"slices"
	"time"
)

// IDSStatus is the lifecycle state of an Information Disclosure Statement.
type IDSStatus string

const (
	// IDSDraft is an IDS still being assembled; entries may change.
	IDSDraft IDSStatus = "draft"
	// IDSFiled is an IDS submitted to the patent office; it is now immutable.
	IDSFiled IDSStatus = "filed"
)

// Valid reports whether the IDSStatus is a known value.
func (s IDSStatus) Valid() bool {
	switch s {
	case IDSDraft, IDSFiled:
		return true
	default:
		return false
	}
}

// idsTransitions is the single source of truth for IDS status changes.
var idsTransitions = map[IDSStatus][]IDSStatus{
	IDSDraft: {IDSFiled},
	IDSFiled: {},
}

// CanTransitionTo reports whether moving from s to target is allowed.
func (s IDSStatus) CanTransitionTo(target IDSStatus) bool {
	if s == target {
		return s.Valid()
	}
	return slices.Contains(idsTransitions[s], target)
}

// ParseIDSStatus converts a string into an IDSStatus.
func ParseIDSStatus(s string) (IDSStatus, error) {
	st := IDSStatus(s)
	if !st.Valid() {
		return "", fmt.Errorf("domain: unknown IDS status %q", s)
	}
	return st, nil
}

// IDSEntry is one cited reference on an Information Disclosure Statement.
type IDSEntry struct {
	Number PatentNumber `json:"number"`
	Title  string       `json:"title"`
}

// IDS is the set of references disclosed to the patent office for a project.
type IDS struct {
	Project     ProjectID  `json:"project"`
	Status      IDSStatus  `json:"status"`
	GeneratedAt time.Time  `json:"generated_at"`
	Entries     []IDSEntry `json:"entries"`
}
