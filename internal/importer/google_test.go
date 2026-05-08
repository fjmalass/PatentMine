package importer

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestGooglePatentsURL(t *testing.T) {
	got, err := GooglePatentsURL(" us11611785b2 ")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://patents.google.com/patent/US11611785B2/en?oq=US11611785B2+"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestGooglePatentsURLRejectsURLs(t *testing.T) {
	if _, err := GooglePatentsURL("https://patents.google.com/patent/US11611785B2/en"); err == nil {
		t.Fatal("expected full URL to be rejected")
	}
}

func TestEstimatedExpirationDate(t *testing.T) {
	if got := estimatedExpirationDate("2023-03-21", ""); got != "2043-03-21" {
		t.Fatalf("expected publication-based expiration, got %q", got)
	}
	if got := estimatedExpirationDate("", "2010-01-15"); got != "2030-01-15" {
		t.Fatalf("expected grant-based expiration, got %q", got)
	}
}

func TestExtractCitationEdges(t *testing.T) {
	html := `
		<div>
			<h3>Citations</h3>
			<div itemprop="backwardReferences">
				<a href="/patent/US1111111A/en">US1111111A</a>
			</div>
			<h3>Cited By</h3>
			<div itemprop="forwardReferences">
				<a href="/patent/US2222222B2/en">US2222222B2</a>
			</div>
		</div>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	edges := extractCitationEdges(doc, "US11611785B2")
	// The improved scraper might find 2 or more if it catches both the itemprop and the general link pass.
	// But they should be correctly categorized.
	if len(edges) < 2 {
		t.Fatalf("expected at least two citation edges, got %+v", edges)
	}
	
	foundCites := false
	foundCitedBy := false
	for _, edge := range edges {
		if edge.TargetPatent == "US1111111A" && edge.RelationType == "cites" {
			foundCites = true
		}
		if edge.TargetPatent == "US2222222B2" && edge.RelationType == "cited_by" {
			foundCitedBy = true
		}
	}
	
	if !foundCites {
		t.Errorf("did not find expected citation US1111111A (cites)")
	}
	if !foundCitedBy {
		t.Errorf("did not find expected citation US2222222B2 (cited_by)")
	}
}
