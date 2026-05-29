package domain

import (
	"fmt"
)

// FetchState records how complete a stored patent record is. It is independent
// of any project: it describes the patent itself.
type FetchState string

const (
	// FetchUnknown is the fallback/error fetch state.
	FetchUnknown FetchState = "unknown"
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
		return FetchUnknown, fmt.Errorf("domain: unknown fetch state %q", s)
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
	// Contrast with ReviewStateUnknown: that state means a membership row EXISTS
	// but the project workflow has not been classified yet.
	ReviewStateNone ReviewState = ""
	// ReviewStateUnknown is the default state of a patent added to a project
	// before it has been classified by review workflow.
	ReviewStateUnknown ReviewState = "unknown"
	// ReviewStateUnderReview marks a patent awaiting human review.
	ReviewStateUnderReview ReviewState = "under_review"
	// ReviewStateActive marks a reviewed patent that remains active in a project.
	ReviewStateActive ReviewState = "active"
	// ReviewStateIgnored marks a patent the user has set aside.
	ReviewStateIgnored ReviewState = "ignored"
	// ReviewStateDeleted is a soft delete: the row stays for history and undo.
	ReviewStateDeleted ReviewState = "deleted"
)

// Valid reports whether the ReviewState is a known value.
func (s ReviewState) Valid() bool {
	switch s {
	case ReviewStateUnknown, ReviewStateUnderReview, ReviewStateActive, ReviewStateIgnored, ReviewStateDeleted:
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

// CanTransitionTo reports whether moving from s to target is allowed under the given fetch state.
// A no-op transition (s == target) is always allowed.
func (s ReviewState) CanTransitionTo(target ReviewState, fetch FetchState) bool {
	if s == target {
		return s.Valid()
	}
	if !s.Valid() || !target.Valid() {
		return false
	}
	if fetch == FetchStub {
		// If fetch state is stub, only unknown or under_review reviewState is possible.
		return target == ReviewStateUnknown || target == ReviewStateUnderReview
	}
	// Otherwise, we can transition to/from any valid ReviewState.
	return true
}
