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

func TestDocOriginValidParseAndCycle(t *testing.T) {
	if !OriginOffice.Valid() || !OriginThirdParty.Valid() || DocOrigin("bogus").Valid() {
		t.Fatal("DocOrigin validity wrong")
	}
	if o, err := ParseDocOrigin(" Office "); err != nil || o != OriginOffice {
		t.Fatalf("ParseDocOrigin(Office)=%q,%v", o, err)
	}
	if o, err := ParseDocOrigin(""); err != nil || o != "" {
		t.Fatalf("ParseDocOrigin(empty)=%q,%v want empty", o, err)
	}
	if _, err := ParseDocOrigin("nope"); err == nil {
		t.Fatal("ParseDocOrigin(nope) should error")
	}
	// Cycle wraps and starts at the first value for an unset origin.
	if got := DocOrigin("").Cycle(1); got != OriginOffice {
		t.Fatalf("empty.Cycle(1)=%q want office", got)
	}
	if got := OriginThirdParty.Cycle(1); got != OriginOffice {
		t.Fatalf("third_party.Cycle(1)=%q want office (wrap)", got)
	}
	if got := OriginOffice.Cycle(-1); got != OriginThirdParty {
		t.Fatalf("office.Cycle(-1)=%q want third_party (wrap back)", got)
	}
}

func TestDocStageValidParseAndCycle(t *testing.T) {
	if !StageReceived.Valid() || !StageFiled.Valid() || DocStage("bogus").Valid() {
		t.Fatal("DocStage validity wrong")
	}
	if s, err := ParseDocStage(" Under_Review "); err != nil || s != StageUnderReview {
		t.Fatalf("ParseDocStage(under_review)=%q,%v", s, err)
	}
	if got := StageFiled.Cycle(1); got != StageReceived {
		t.Fatalf("filed.Cycle(1)=%q want received (wrap)", got)
	}
	if got := StageReceived.Cycle(1); got != StageDraft {
		t.Fatalf("received.Cycle(1)=%q want draft", got)
	}
}

func TestInferOriginStage(t *testing.T) {
	cases := []struct {
		kind   MatterDocKind
		origin DocOrigin
		stage  DocStage
	}{
		{MatterDocOA, OriginOffice, StageReceived},
		{MatterDocReference, OriginThirdParty, StageReceived},
		{MatterDocResponse, OriginFirm, StageDraft},
		{MatterDocIDS, OriginFirm, StageFiled},
		{MatterDocKind("unknown"), OriginFirm, StageReceived},
	}
	for _, c := range cases {
		o, s := InferOriginStage(c.kind)
		if o != c.origin || s != c.stage {
			t.Errorf("InferOriginStage(%q)=(%q,%q) want (%q,%q)", c.kind, o, s, c.origin, c.stage)
		}
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
