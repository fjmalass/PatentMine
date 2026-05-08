package importer

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"

	"patentmine/internal/domain"
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

func TestExtractClassificationsReadsGooglePluralItemprop(t *testing.T) {
	html := `
		<div>
			<li itemprop="classifications">
				<span itemprop="Code">H04N21/4325</span>
				<span itemprop="Description">Content retrieval operation from a local storage medium</span>
			</li>
			<li itemprop="classifications">H04L65/601—Network streaming detail</li>
		</div>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	bundle := testBundle()
	extractClassifications(doc, &bundle, "US10218760B2")
	if len(bundle.Classifications) != 2 {
		t.Fatalf("expected two classifications, got %+v", bundle.Classifications)
	}
	if bundle.Classifications[0].Code != "H04N21/4325" {
		t.Fatalf("expected first code, got %+v", bundle.Classifications[0])
	}
	if bundle.Classifications[1].Code != "H04L65/601" || bundle.Classifications[1].Description != "Network streaming detail" {
		t.Fatalf("expected text fallback classification, got %+v", bundle.Classifications[1])
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

func testBundle() domain.PatentBundle {
	return domain.PatentBundle{}
}
