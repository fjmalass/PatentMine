package domain

import "testing"

func TestMembershipStateTransitions(t *testing.T) {
	cases := []struct {
		from MembershipState
		to   MembershipState
		want bool
	}{
		{MembershipStored, MembershipUnderReview, true},
		{MembershipStored, MembershipIgnored, true},
		{MembershipStored, MembershipDeleted, true},
		{MembershipStored, MembershipStored, true}, // no-op allowed
		{MembershipUnderReview, MembershipStored, true},
		{MembershipIgnored, MembershipStored, true},
		{MembershipDeleted, MembershipStored, true}, // undelete
		{MembershipDeleted, MembershipIgnored, false},
		{MembershipDeleted, MembershipUnderReview, false},
	}
	for _, c := range cases {
		if got := c.from.CanTransitionTo(c.to); got != c.want {
			t.Errorf("%s -> %s: got %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestParseMembershipState(t *testing.T) {
	if _, err := ParseMembershipState("stored"); err != nil {
		t.Errorf("ParseMembershipState(stored) error: %v", err)
	}
	if _, err := ParseMembershipState("bogus"); err == nil {
		t.Error("ParseMembershipState(bogus) expected error")
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
