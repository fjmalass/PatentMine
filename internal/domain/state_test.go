package domain

import "testing"

func TestReviewStateTransitions(t *testing.T) {
	t.Run("cached patent allows transition to/from any valid review state", func(t *testing.T) {
		cases := []struct {
			from ReviewState
			to   ReviewState
			want bool
		}{
			{ReviewStateUnknown, ReviewStateUnderReview, true},
			{ReviewStateUnderReview, ReviewStateActive, true},
			{ReviewStateActive, ReviewStateIgnored, true},
			{ReviewStateActive, ReviewStateActive, true},  // no-op allowed
			{ReviewStateDeleted, ReviewStateActive, true}, // undelete
			{ReviewStateDeleted, ReviewStateIgnored, true},
			{ReviewStateDeleted, ReviewStateUnderReview, true},
			{ReviewStateDeleted, ReviewStateUnknown, true},
			{ReviewStateNone, ReviewStateActive, false}, // invalid from
			{ReviewStateActive, ReviewStateNone, false}, // invalid to
		}
		for _, c := range cases {
			if got := c.from.CanTransitionTo(c.to, FetchCached); got != c.want {
				t.Errorf("%s -> %s (cached): got %v, want %v", c.from, c.to, got, c.want)
			}
		}
	})

	t.Run("stub patent only allows unknown review state", func(t *testing.T) {
		cases := []struct {
			from ReviewState
			to   ReviewState
			want bool
		}{
			{ReviewStateUnknown, ReviewStateUnknown, true}, // no-op
			{ReviewStateUnknown, ReviewStateUnderReview, false},
			{ReviewStateUnknown, ReviewStateActive, false},
			{ReviewStateUnknown, ReviewStateIgnored, false},
			{ReviewStateUnknown, ReviewStateDeleted, false},
		}
		for _, c := range cases {
			if got := c.from.CanTransitionTo(c.to, FetchStub); got != c.want {
				t.Errorf("%s -> %s (stub): got %v, want %v", c.from, c.to, got, c.want)
			}
		}
	})
}

func TestParseReviewState(t *testing.T) {
	if _, err := ParseReviewState("active"); err != nil {
		t.Errorf("ParseReviewState(active) error: %v", err)
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
