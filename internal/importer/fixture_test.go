package importer

import "testing"

func TestLoadFixture(t *testing.T) {
	bundle, err := LoadFixture("../../fixtures/US11611785B2.json")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Patent.Number != "US11611785B2" {
		t.Fatalf("unexpected patent number %q", bundle.Patent.Number)
	}
	if bundle.Patent.ExpirationDate != "2043-03-21" {
		t.Fatalf("unexpected expiration date %q", bundle.Patent.ExpirationDate)
	}
	if !bundle.Patent.ExpirationEstimated {
		t.Fatal("expected fixture expiration to be marked estimated")
	}
	if len(bundle.Sections) == 0 || len(bundle.Classifications) == 0 || len(bundle.Citations) == 0 {
		t.Fatalf("fixture should include sections, classifications, and citations: %+v", bundle)
	}
}
