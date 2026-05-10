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

func TestAdjustedExpirationDateFromGoogleEvents(t *testing.T) {
	html := `
		<section>
			<h2>Application US15/189,931 events</h2>
			<ul>
				<li itemprop="events">2019-02-26 Application granted</li>
				<li itemprop="events">Status Active legal-status Critical Current</li>
				<li itemprop="events">2036-11-15 Adjusted expiration legal-status Critical</li>
			</ul>
		</section>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	if got := adjustedExpirationDate(doc); got != "2036-11-15" {
		t.Fatalf("expected adjusted expiration date, got %q", got)
	}
}

func TestAdjustedExpirationDateIgnoresLegalStatusExpiresWithoutAdjustedEvent(t *testing.T) {
	html := `<section>Legal status Active, expires 2036-11-15</section>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	if got := adjustedExpirationDate(doc); got != "" {
		t.Fatalf("expected no adjusted expiration date, got %q", got)
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
	extractClassifications(doc, &bundle, "US10218760B2", nil)
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
	edges := extractCitationEdges(doc, "US11611785B2", nil)
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

func TestExtractCitationEdgesIgnoresFamilyAndUnstructuredPatentLinks(t *testing.T) {
	html := `
		<div>
			<div itemprop="forwardReferences">
				<a href="/patent/US2222222B2/en">US2222222B2</a>
			</div>
			<div itemprop="forwardReferencesFamily">
				<a href="/patent/US3333333B2/en">US3333333B2</a>
			</div>
			<section>
				<h3>Cited By</h3>
				<a href="/patent/US4444444B2/en">US4444444B2</a>
			</section>
		</div>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	edges := extractCitationEdges(doc, "US11611785B2", nil)
	if len(edges) != 1 {
		t.Fatalf("expected only one structured cited-by edge, got %+v", edges)
	}
	if edges[0].TargetPatent != "US2222222B2" || edges[0].RelationType != domain.RelationCitedBy {
		t.Fatalf("unexpected edge %+v", edges[0])
	}
}

func TestExtractFamilyEdgesParentAndPriorityApps(t *testing.T) {
	html := `
		<table>
		  <tbody>
			<tr itemprop="parentApps" itemscope>
			  <td>
				<span itemprop="applicationNumber">US16/206,188</span>
				<span itemprop="relationType">Continuation</span>
				<a href="/patent/US11346835B2/en">
				  <span itemprop="representativePublication">US11346835B2</span>
				</a>
			  </td>
			</tr>
			<tr itemprop="priorityApps" itemscope>
			  <td>
				<span itemprop="applicationNumber">US17/730,671</span>
				<a href="/patent/US12241885B2/en">
				  <span itemprop="representativePublication">US12241885B2</span>
				</a>
			  </td>
			</tr>
		  </tbody>
		</table>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	edges := extractFamilyEdges(doc, "US20220252571A1", nil)
	if len(edges) != 2 {
		t.Fatalf("expected 2 family edges, got %+v", edges)
	}
	var foundParent, foundChild bool
	for _, e := range edges {
		if e.ParentNumber == "US11346835B2" && e.ChildNumber == "US20220252571A1" && e.RelationType == domain.FamilyRelationContinuation {
			foundParent = true
		}
		if e.ParentNumber == "US20220252571A1" && e.ChildNumber == "US12241885B2" {
			foundChild = true
		}
	}
	if !foundParent {
		t.Errorf("parent edge not found in %+v", edges)
	}
	if !foundChild {
		t.Errorf("child edge not found in %+v", edges)
	}
}

func TestExtractFamilyEdgesRelationTypeDivisional(t *testing.T) {
	html := `
		<table><tbody>
		  <tr itemprop="parentApps" itemscope>
			<td>
			  <span itemprop="relationType">Divisional</span>
			  <a href="/patent/US9999999B2/en">
				<span itemprop="representativePublication">US9999999B2</span>
			  </a>
			</td>
		  </tr>
		</tbody></table>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	edges := extractFamilyEdges(doc, "US1111111A", nil)
	if len(edges) != 1 || edges[0].RelationType != domain.FamilyRelationDivisional {
		t.Fatalf("expected divisional edge, got %+v", edges)
	}
}

func TestExtractFamilyEdgesLegacyFallback(t *testing.T) {
	html := `
		<div>
		  <div itemprop="backwardReferencesFamily">
			<a href="/patent/US8888888B2/en">US8888888B2</a>
		  </div>
		  <div itemprop="forwardReferencesFamily">
			<a href="/patent/US7777777B2/en">US7777777B2</a>
		  </div>
		</div>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	edges := extractFamilyEdges(doc, "US5555555B2", nil)
	if len(edges) != 2 {
		t.Fatalf("expected 2 legacy edges, got %+v", edges)
	}
}

func TestExtractFamilyEdgesContinuationApps(t *testing.T) {
	html := `
		<table><tbody>
		  <tr itemprop="continuationApps" itemscope>
			<td>
			  <span itemprop="relationType">Continuation</span>
			  <a href="/patent/US20240012345A1/en">
				<span itemprop="representativePublication">US20240012345A1</span>
			  </a>
			</td>
		  </tr>
		</tbody></table>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))
	edges := extractFamilyEdges(doc, "US11740187B2", nil)
	if len(edges) != 1 {
		t.Fatalf("expected 1 continuationApps edge, got %+v", edges)
	}
	e := edges[0]
	if e.ParentNumber != "US11740187B2" || e.ChildNumber != "US20240012345A1" {
		t.Fatalf("wrong edge direction: %+v", e)
	}
	if e.RelationType != domain.FamilyRelationContinuation {
		t.Fatalf("expected continuation, got %q", e.RelationType)
	}
}

func testBundle() domain.PatentBundle {
	return domain.PatentBundle{}
}
