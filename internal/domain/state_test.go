package domain

import "testing"

func TestReviewStateTransitions(t *testing.T) {
	cases := []struct {
		from ReviewState
		to   ReviewState
		want bool
	}{
		{ReviewStateLoad, ReviewStateUnderReview, true},
		{ReviewStateLoad, ReviewStateIgnored, true},
		{ReviewStateLoad, ReviewStateDeleted, true},
		{ReviewStateLoad, ReviewStateLoad, true}, // no-op allowed
		{ReviewStateUnderReview, ReviewStateLoad, true},
		{ReviewStateIgnored, ReviewStateLoad, true},
		{ReviewStateDeleted, ReviewStateLoad, true}, // undelete
		{ReviewStateDeleted, ReviewStateIgnored, false},
		{ReviewStateDeleted, ReviewStateUnderReview, false},
	}
	for _, c := range cases {
		if got := c.from.CanTransitionTo(c.to); got != c.want {
			t.Errorf("%s -> %s: got %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestParseReviewState(t *testing.T) {
	if _, err := ParseReviewState("load"); err != nil {
		t.Errorf("ParseReviewState(load) error: %v", err)
	}
	if _, err := ParseReviewState("bogus"); err == nil {
		t.Error("ParseReviewState(bogus) expected error")
	}
}

func TestParseFetchState(t *testing.T) {
	if _, err := ParseFetchState("cached"); err != nil {
		t.Errorf("ParseFetchState(cached) error: %v", err)
	}
	if _, err := ParseFetchState("bogus"); err == nil {
		t.Error("ParseFetchState(bogus) expected error")
	}
}

func TestRelationKindInverse(t *testing.T) {
	cases := []struct{ k, want RelationKind }{
		{RelationCites, RelationCitedBy},
		{RelationCitedBy, RelationCites},
		{RelationParent, RelationChild},
		{RelationChild, RelationParent},
	}
	for _, c := range cases {
		if got := c.k.Inverse(); got != c.want {
			t.Errorf("%s.Inverse() = %s, want %s", c.k, got, c.want)
		}
		if c.k.Inverse().Inverse() != c.k {
			t.Errorf("%s.Inverse().Inverse() should round-trip", c.k)
		}
	}
}

func TestIDSStatusTransitions(t *testing.T) {
	if !IDSDraft.CanTransitionTo(IDSFiled) {
		t.Error("draft -> filed should be allowed")
	}
	if IDSFiled.CanTransitionTo(IDSDraft) {
		t.Error("filed -> draft should be rejected")
	}
}
