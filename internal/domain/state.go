package domain

import (
	"fmt"
	"slices"
)

// FetchState records how complete a stored patent record is. It is independent
// of any project: it describes the patent itself.
type FetchState string

const (
	// FetchStub means only a reference exists (e.g. discovered as a citation)
	// without the full patent body having been fetched.
	FetchStub FetchState = "stub"
	// FetchCached means the full patent body has been fetched and stored.
	FetchCached FetchState = "cached"
)

// Valid reports whether the FetchState is a known value.
func (s FetchState) Valid() bool {
	switch s {
	case FetchStub, FetchCached:
		return true
	default:
		return false
	}
}

// ParseFetchState converts a string into a FetchState, rejecting unknown input.
func ParseFetchState(s string) (FetchState, error) {
	fs := FetchState(s)
	if !fs.Valid() {
		return "", fmt.Errorf("domain: unknown fetch state %q", s)
	}
	return fs, nil
}

// MembershipState is the per-(patent, project) workflow state. The same patent
// can sit in different states across different projects.
type MembershipState string

const (
	// MembershipStored is the default state of a patent added to a project.
	MembershipStored MembershipState = "stored"
	// MembershipUnderReview marks a patent awaiting human review.
	MembershipUnderReview MembershipState = "under_review"
	// MembershipIgnored marks a patent the user has set aside.
	MembershipIgnored MembershipState = "ignored"
	// MembershipDeleted is a soft delete: the row stays for history and undo.
	MembershipDeleted MembershipState = "deleted"
)

// Valid reports whether the MembershipState is a known value.
func (s MembershipState) Valid() bool {
	switch s {
	case MembershipStored, MembershipUnderReview, MembershipIgnored, MembershipDeleted:
		return true
	default:
		return false
	}
}

// ParseMembershipState converts a string into a MembershipState.
func ParseMembershipState(s string) (MembershipState, error) {
	ms := MembershipState(s)
	if !ms.Valid() {
		return "", fmt.Errorf("domain: unknown membership state %q", s)
	}
	return ms, nil
}

// membershipTransitions is the single source of truth for which membership
// state changes are allowed. Keeping it as data (not scattered if-statements)
// means a new state or rule is one edit here.
var membershipTransitions = map[MembershipState][]MembershipState{
	MembershipStored:      {MembershipUnderReview, MembershipIgnored, MembershipDeleted},
	MembershipUnderReview: {MembershipStored, MembershipIgnored, MembershipDeleted},
	MembershipIgnored:     {MembershipStored, MembershipUnderReview, MembershipDeleted},
	MembershipDeleted:     {MembershipStored},
}

// CanTransitionTo reports whether moving from s to target is allowed. A no-op
// transition (s == target) is always allowed.
func (s MembershipState) CanTransitionTo(target MembershipState) bool {
	if s == target {
		return s.Valid()
	}
	return slices.Contains(membershipTransitions[s], target)
}
