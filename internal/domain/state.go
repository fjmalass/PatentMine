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

// ReviewState is the per-(patent, project) workflow state. The same patent
// can sit in different states across different projects.
type ReviewState string

const (
	// ReviewStateNone is the zero value: patent has no membership row in the
	// current project. Never stored in the database — appears only as a derived
	// value when a LEFT JOIN produces no matching membership row, and as the
	// zero value of PatentFilter.ReviewState when no state filter is active.
	// Contrast with ReviewStateLoad: that state means a membership row EXISTS
	// but the patent has not yet been fetched.
	ReviewStateNone ReviewState = ""
	// ReviewStateLoad is the default state of a patent added to a project.
	// It acts as a call to action: the user should load/fetch the patent.
	ReviewStateLoad ReviewState = "load"
	// ReviewStateUnderReview marks a patent awaiting human review.
	ReviewStateUnderReview ReviewState = "under_review"
	// ReviewStateIgnored marks a patent the user has set aside.
	ReviewStateIgnored ReviewState = "ignored"
	// ReviewStateCached marks a patent whose record is cached.
	ReviewStateCached ReviewState = "cached"
	// ReviewStateDeleted is a soft delete: the row stays for history and undo.
	ReviewStateDeleted ReviewState = "deleted"
)

// Valid reports whether the ReviewState is a known value.
func (s ReviewState) Valid() bool {
	switch s {
	case ReviewStateLoad, ReviewStateUnderReview, ReviewStateIgnored, ReviewStateCached, ReviewStateDeleted:
		return true
	default:
		return false
	}
}

// ParseReviewState converts a string into a ReviewState.
// The empty string is accepted and returns ReviewStateNone.
func ParseReviewState(s string) (ReviewState, error) {
	if s == "" {
		return ReviewStateNone, nil
	}
	ms := ReviewState(s)
	if !ms.Valid() {
		return ReviewStateNone, fmt.Errorf("domain: unknown review state %q", s)
	}
	return ms, nil
}

// reviewStateTransitions is the single source of truth for which review
// state changes are allowed. Keeping it as data (not scattered if-statements)
// means a new state or rule is one edit here.
var reviewStateTransitions = map[ReviewState][]ReviewState{
	ReviewStateLoad:        {ReviewStateUnderReview, ReviewStateIgnored, ReviewStateCached, ReviewStateDeleted},
	ReviewStateUnderReview: {ReviewStateLoad, ReviewStateIgnored, ReviewStateCached, ReviewStateDeleted},
	ReviewStateIgnored:     {ReviewStateLoad, ReviewStateUnderReview, ReviewStateCached, ReviewStateDeleted},
	ReviewStateCached:      {ReviewStateLoad, ReviewStateUnderReview, ReviewStateIgnored, ReviewStateDeleted},
	ReviewStateDeleted:     {ReviewStateLoad},
}

// CanTransitionTo reports whether moving from s to target is allowed. A no-op
// transition (s == target) is always allowed.
func (s ReviewState) CanTransitionTo(target ReviewState) bool {
	if s == target {
		return s.Valid()
	}
	return slices.Contains(reviewStateTransitions[s], target)
}
