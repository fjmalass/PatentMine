package filterexpr

import "testing"

func TestParseCanonicalizesAliasesAndNot(t *testing.T) {
	expr, err := Parse("classification:S04* and not status:under_review")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := CanonicalString(expr), "class:S04* and not state:under_review"; got != want {
		t.Fatalf("CanonicalString = %q, want %q", got, want)
	}
}

func TestParseSupportsQuotedSearchAndInventor(t *testing.T) {
	expr, err := Parse(`search:"widget sensor" and inventor:"Ada Lovelace"`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := CanonicalString(expr), `search:"widget sensor" and inventor:"Ada Lovelace"`; got != want {
		t.Fatalf("CanonicalString = %q, want %q", got, want)
	}
}

func TestParseSupportsInventorWildcard(t *testing.T) {
	expr, err := Parse(`inventor:"Ada*" or inventor:*Lovelace*`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := CanonicalString(expr), `inventor:Ada* or inventor:*Lovelace*`; got != want {
		t.Fatalf("CanonicalString = %q, want %q", got, want)
	}
}

func TestParseSupportsAssigneeWildcard(t *testing.T) {
	expr, err := Parse(`assignee:"Acme*" and owner:*Corp`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := CanonicalString(expr), `assignee:Acme* and assignee:*Corp`; got != want {
		t.Fatalf("CanonicalString = %q, want %q", got, want)
	}
}

func TestParseSupportsIDSStatus(t *testing.T) {
	expr, err := Parse("ids:pending or ids_status:none")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := CanonicalString(expr), "ids_status:pending or ids_status:none"; got != want {
		t.Fatalf("CanonicalString = %q, want %q", got, want)
	}
}

func TestParseRejectsLegacySyntax(t *testing.T) {
	_, err := Parse("state active")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != LegacySyntaxHint {
		t.Fatalf("legacy error = %q", got)
	}
}

func TestParseRejectsUnsupportedClassPattern(t *testing.T) {
	_, err := Parse("class:S04.[1-9]*")
	if err == nil {
		t.Fatal("expected error")
	}
	if got, want := err.Error(), "unsupported classification filter syntax: only literal values and '*' wildcard are supported right now (examples: class:S04, class:S04*)"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestValidateProjectScope(t *testing.T) {
	expr, err := Parse("tag:prior_art and not state:under_review")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := ValidateProjectScope(expr, false); err == nil {
		t.Fatal("expected project-scope error")
	}

	expr, err = Parse("ids_status:ignored")
	if err != nil {
		t.Fatalf("Parse ids_status: %v", err)
	}
	if err := ValidateProjectScope(expr, false); err == nil {
		t.Fatal("expected ids_status project-scope error")
	}

	expr, err = Parse("provenance:manual")
	if err != nil {
		t.Fatalf("Parse provenance: %v", err)
	}
	if err := ValidateProjectScope(expr, false); err == nil {
		t.Fatal("expected provenance project-scope error")
	}
}

func TestParseSupportsProvenance(t *testing.T) {
	expr, err := Parse("prov:manual or provenance:system or provenance:related or provenance:neighbors or provenance:neighbor")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := CanonicalString(expr), "provenance:manual or provenance:system or provenance:related or provenance:neighbors or provenance:neighbors"; got != want {
		t.Fatalf("CanonicalString = %q, want %q", got, want)
	}
}

func TestParseRejectsUnknownProvenance(t *testing.T) {
	_, err := Parse("prov:invalid")
	if err == nil {
		t.Fatal("expected error")
	}
	expected := `unknown provenance keyword "invalid": use manual, related, neighbors, or system`
	if got := err.Error(); got != expected {
		t.Fatalf("error = %q, want %q", got, expected)
	}
}
