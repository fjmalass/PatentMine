package domain

import (
	"fmt"
	"slices"
	"strings"
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

// IDSEntryStatus is the workflow state of one curated IDS entry.
type IDSEntryStatus string

const (
	IDSEntryPending   IDSEntryStatus = "pending"
	IDSEntrySubmitted IDSEntryStatus = "submitted"
	IDSEntryAccepted  IDSEntryStatus = "accepted"
)

// Valid reports whether the IDSEntryStatus is a known value.
func (s IDSEntryStatus) Valid() bool {
	switch s {
	case IDSEntryPending, IDSEntrySubmitted, IDSEntryAccepted:
		return true
	default:
		return false
	}
}

// Next returns the next entry status in the old-style workflow.
func (s IDSEntryStatus) Next() IDSEntryStatus {
	switch s {
	case IDSEntryPending:
		return IDSEntrySubmitted
	case IDSEntrySubmitted:
		return IDSEntryAccepted
	case IDSEntryAccepted:
		return IDSEntryPending
	default:
		return IDSEntryPending
	}
}

// IDSEntry is one curated prior-art reference for a project/patent pair.
type IDSEntry struct {
	ID               int64          `json:"id"`
	Project          ProjectID      `json:"project"`
	Patent           PatentNumber   `json:"patent"`
	KindCode         string         `json:"kind_code,omitempty"`
	CountryCode      string         `json:"country_code,omitempty"`
	InFull           bool           `json:"in_full,omitempty"`
	RelevantPassages string         `json:"relevant_passages,omitempty"`
	Notes            string         `json:"notes,omitempty"`
	Status           IDSEntryStatus `json:"status,omitempty"`
	AddedAt          time.Time      `json:"added_at"`
}

// SummaryText returns a compact, stable IDS summary for tables and detail lines.
func (e IDSEntry) SummaryText() string {
	if e.Project == "" || e.Patent.IsZero() {
		return "-"
	}
	parts := []string{string(e.Status)}
	if !e.Status.Valid() {
		parts[0] = string(IDSEntryPending)
	}
	switch {
	case e.InFull:
		parts = append(parts, "full")
	case strings.TrimSpace(e.RelevantPassages) != "":
		parts = append(parts, strings.TrimSpace(e.RelevantPassages))
	}
	if note := strings.TrimSpace(e.Notes); note != "" {
		parts = append(parts, note)
	}
	return strings.Join(parts, " | ")
}

// IDSReference is one cited reference on an exported Information Disclosure Statement.
type IDSReference struct {
	Number PatentNumber `json:"number"`
	Title  string       `json:"title"`
}

// IDS is the set of references disclosed to the patent office for a project.
type IDS struct {
	Project     ProjectID      `json:"project"`
	Status      IDSStatus      `json:"status"`
	GeneratedAt time.Time      `json:"generated_at"`
	Entries     []IDSReference `json:"entries"`
}
