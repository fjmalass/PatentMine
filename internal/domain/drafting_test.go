package domain

import "testing"

func TestDraftKindValidAndClaims(t *testing.T) {
	cases := []struct {
		kind        DraftKind
		valid       bool
		needsClaims bool
	}{
		{DraftProvisional, true, false},
		{DraftNonprovisional, true, true},
		{DraftOAResponse, true, true},
		{DraftKind("bogus"), false, false},
	}
	for _, c := range cases {
		if got := c.kind.Valid(); got != c.valid {
			t.Errorf("%q Valid()=%v want %v", c.kind, got, c.valid)
		}
		if got := c.kind.NeedsClaims(); got != c.needsClaims {
			t.Errorf("%q NeedsClaims()=%v want %v", c.kind, got, c.needsClaims)
		}
	}
}

func TestParseDraftKind(t *testing.T) {
	if k, err := ParseDraftKind("  Provisional "); err != nil || k != DraftProvisional {
		t.Fatalf("ParseDraftKind(Provisional)=%q,%v", k, err)
	}
	if _, err := ParseDraftKind("nope"); err == nil {
		t.Fatal("ParseDraftKind(nope) should error")
	}
}

func TestAmendmentStatusLabelAndMarkup(t *testing.T) {
	if got := AmendCurrentlyAmended.Label(); got != "(Currently Amended)" {
		t.Errorf("label=%q", got)
	}
	if !AmendCurrentlyAmended.ShowsMarkup() {
		t.Error("currently-amended must show markup")
	}
	if AmendNew.ShowsMarkup() {
		t.Error("new claim must not show markup")
	}
	if !AmendCancelled.Valid() || AmendmentStatus("x").Valid() {
		t.Error("validity check wrong")
	}
}

func TestDefaultSections(t *testing.T) {
	prov := DefaultSections(DraftProvisional)
	for _, s := range prov {
		if s.Kind == SectionKind("claims") {
			t.Fatal("provisional skeleton must not contain a claims section")
		}
	}
	if len(prov) == 0 {
		t.Fatal("provisional skeleton is empty")
	}
	// Ordinals must be dense and zero-based so the store can round-trip them.
	for i, s := range prov {
		if s.Ordinal != i {
			t.Errorf("section %d has ordinal %d", i, s.Ordinal)
		}
		if s.Heading == "" {
			t.Errorf("section %d has empty heading", i)
		}
	}
	if oa := DefaultSections(DraftOAResponse); len(oa) != 1 || oa[0].Kind != SectionRemarks {
		t.Fatalf("oa response skeleton = %+v", oa)
	}
}

func TestOATypeValid(t *testing.T) {
	if !OANonFinal.Valid() || !OANoticeOfAllowance.Valid() {
		t.Error("known OA types must be valid")
	}
	if OAType("weird").Valid() {
		t.Error("unknown OA type must be invalid")
	}
}
