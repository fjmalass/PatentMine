package crawl

import (
	"errors"
	"testing"

	"patentmine/internal/domain"
)

const googleSamplePage = `<!doctype html><html><head>
<meta name="DC.title" content="Fallback title"/>
<meta itemprop="classification" content="CPC: G06F 16/245" />
</head><body>
<span itemprop="title">Widget apparatus and method </span>
<section itemprop="abstract"><div class="abstract">An improved widget.</div></section>
<dd itemprop="assigneeOriginal">Acme Corp</dd>
<dd itemprop="inventor">Jane Doe</dd>
<dd itemprop="inventor">John Roe</dd>
<dd itemprop="classification"><span itemprop="code">A61K31/00</span></dd>
<time itemprop="filingDate" datetime="2016-06-19">June 19, 2016</time>
<time itemprop="publicationDate" datetime="2018-03-10">March 10, 2018</time>
<section itemprop="claims">
  <div class="claim">A widget comprising a frobnicator.</div>
  <div class="claim">The widget of claim 1, wherein the frobnicator is blue.</div>
</section>
<h3>Patent Citations (2)</h3>
<div itemprop="backwardReferences">
  <tr><a href="/patent/US9000000B2/en">US9000000B2</a></tr>
  <tr><a href="/patent/US9000001B2/en">US9000001B2</a></tr>
</div>
<h3>Cited By (1)</h3>
<div itemprop="forwardReferences">
  <tr><a href="/patent/US11000000B2/en">US11000000B2</a></tr>
</div>
<div itemprop="parentApps">
  <span itemprop="representativePublication">US8000000B2</span>
</div>
</body></html>`

func TestParseGoogleExtractsBibliographicFields(t *testing.T) {
	number := domain.MustParsePatentNumber("US10000000B2")
	res, err := parseGoogle(number, []byte(googleSamplePage))
	if err != nil {
		t.Fatalf("parseGoogle: %v", err)
	}
	if res.Patent.Title != "Widget apparatus and method" {
		t.Errorf("title = %q", res.Patent.Title)
	}
	if res.Patent.Abstract != "An improved widget." {
		t.Errorf("abstract = %q", res.Patent.Abstract)
	}
	if res.Patent.Assignee != "Acme Corp" {
		t.Errorf("assignee = %q", res.Patent.Assignee)
	}
	if len(res.Patent.Inventors) != 2 || res.Patent.Inventors[0] != "Jane Doe" {
		t.Errorf("inventors = %v", res.Patent.Inventors)
	}
	if len(res.Patent.Classifications) != 2 || res.Patent.Classifications[0] != "G06F16/245" || res.Patent.Classifications[1] != "A61K31/00" {
		t.Errorf("classifications = %v", res.Patent.Classifications)
	}
	if res.Patent.ApplicationDate.Year() != 2016 || res.Patent.PublicationDate.Year() != 2018 {
		t.Errorf("dates = app %v pub %v", res.Patent.ApplicationDate, res.Patent.PublicationDate)
	}
}

func TestParseGoogleExtractsCitationsAndFamily(t *testing.T) {
	number := domain.MustParsePatentNumber("US10000000B2")
	res, err := parseGoogle(number, []byte(googleSamplePage))
	if err != nil {
		t.Fatalf("parseGoogle: %v", err)
	}
	counts := map[domain.RelationKind]int{}
	for _, rel := range res.Relations {
		counts[rel.Kind]++
		if rel.From.Normalized() != number.Normalized() {
			t.Errorf("relation From = %s, want the fetched patent", rel.From)
		}
	}
	if counts[domain.RelationCites] != 2 {
		t.Errorf("cites = %d, want 2", counts[domain.RelationCites])
	}
	if counts[domain.RelationCitedBy] != 1 {
		t.Errorf("cited_by = %d, want 1", counts[domain.RelationCitedBy])
	}
	if counts[domain.RelationParent] != 1 {
		t.Errorf("parent = %d, want 1", counts[domain.RelationParent])
	}
}

func TestParseGoogleExtractsClaimAndExpiration(t *testing.T) {
	number := domain.MustParsePatentNumber("US10000000B2")
	res, err := parseGoogle(number, []byte(googleSamplePage))
	if err != nil {
		t.Fatalf("parseGoogle: %v", err)
	}
	if res.Patent.FirstClaim != "A widget comprising a frobnicator." {
		t.Errorf("first claim = %q", res.Patent.FirstClaim)
	}
	if res.Patent.SourceURL != "https://patents.google.com/patent/US10000000B2/en" {
		t.Errorf("source URL = %q", res.Patent.SourceURL)
	}
	// ApplicationDate 2016 + 20 years.
	if res.Patent.ExpirationDate.Year() != 2036 {
		t.Errorf("expiration date = %v, want 2036", res.Patent.ExpirationDate)
	}
	if res.Patent.ExpirationSource != domain.ExpirationEstimated {
		t.Errorf("expiration source = %q, want estimated", res.Patent.ExpirationSource)
	}
}

const googleDescriptionPage = `<!doctype html><html><head></head><body>
<section itemprop="description">
  <div class="description-paragraph" num="0001">The present invention relates to widgets.</div>
  <div class="description-paragraph" num="0045">In one embodiment, the frobnicator is blue.</div>
  <div class="description-paragraph">An unnumbered paragraph of disclosure.</div>
</section>
</body></html>`

func TestParseAllClaimsNumbersEveryClaim(t *testing.T) {
	claims, err := ParseAllClaims([]byte(googleSamplePage))
	if err != nil {
		t.Fatalf("ParseAllClaims: %v", err)
	}
	if len(claims) != 2 {
		t.Fatalf("claims = %d, want 2", len(claims))
	}
	if claims[0].Number != 1 || claims[1].Number != 2 {
		t.Errorf("claim numbers = %d, %d", claims[0].Number, claims[1].Number)
	}
	if claims[0].Text != "A widget comprising a frobnicator." {
		t.Errorf("claim 1 text = %q", claims[0].Text)
	}
}

func TestParseDescriptionExtractsNumberedParagraphs(t *testing.T) {
	paragraphs, err := ParseDescription([]byte(googleDescriptionPage))
	if err != nil {
		t.Fatalf("ParseDescription: %v", err)
	}
	if len(paragraphs) != 3 {
		t.Fatalf("paragraphs = %d, want 3", len(paragraphs))
	}
	if paragraphs[0].Number != "0001" || paragraphs[1].Number != "0045" {
		t.Errorf("paragraph numbers = %q, %q", paragraphs[0].Number, paragraphs[1].Number)
	}
	if paragraphs[2].Number != "" {
		t.Errorf("unnumbered paragraph number = %q, want empty", paragraphs[2].Number)
	}
	if paragraphs[1].Text != "In one embodiment, the frobnicator is blue." {
		t.Errorf("paragraph 0045 text = %q", paragraphs[1].Text)
	}
}

func TestParseDescriptionAbsentReturnsEmpty(t *testing.T) {
	paragraphs, err := ParseDescription([]byte(googleSamplePage))
	if err != nil {
		t.Fatalf("ParseDescription on a page with no description = %v, want nil", err)
	}
	if len(paragraphs) != 0 {
		t.Errorf("paragraphs = %d, want 0", len(paragraphs))
	}
}

func TestParseGoogleUnknownPageIsNotAvailable(t *testing.T) {
	number := domain.MustParsePatentNumber("US10000000B2")
	_, err := parseGoogle(number, []byte("<html><head></head><body>nothing here</body></html>"))
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("parseGoogle on a non-patent page = %v, want ErrNotAvailable", err)
	}
}

func TestParseGoogleExtractsMetaTags(t *testing.T) {
	googleMetaPage := `<!doctype html><html><head>
<meta name="DC.title" content="Meta title"/>
<meta name="citation_patent_application_number" content="US17696256" />
<meta name="citation_patent_publication_number" content="US20220205933A1" />
</head><body>
<span itemprop="title">Meta title</span>
</body></html>`

	number := domain.MustParsePatentNumber("US11714053B2")
	res, err := parseGoogle(number, []byte(googleMetaPage))
	if err != nil {
		t.Fatalf("parseGoogle: %v", err)
	}

	var foundApp, foundPub bool
	for _, d := range res.Documents {
		if d.Stage == domain.StageApplication && d.Number.Normalized() == "US17696256" {
			foundApp = true
		}
		if d.Stage == domain.StagePublication && d.Number.Normalized() == "US20220205933A1" {
			foundPub = true
		}
	}
	if !foundApp {
		t.Error("missing StageApplication document in Google parsed Documents")
	}
	if !foundPub {
		t.Error("missing StagePublication document in Google parsed Documents")
	}

	var foundAppID, foundPubID bool
	for _, id := range res.AuthorityIdentifiers {
		if id.Authority == "US" && id.IdentifierType == "application" && id.Identifier == "17696256" {
			foundAppID = true
		}
		if id.IdentifierType == "publication" && id.Identifier == "US20220205933A1" {
			foundPubID = true
		}
	}
	if !foundAppID {
		t.Error("missing application AuthorityIdentifier in Google result")
	}
	if !foundPubID {
		t.Error("missing publication AuthorityIdentifier in Google result")
	}
}

func TestParseDescription_Hierarchical(t *testing.T) {
	googleHTML := `<!doctype html><html><body>
<section itemprop="description">
  <div class="description-paragraph" num="0001">
    Here is a list of features:
    <ul>
      <li>first feature</li>
      <li>second feature</li>
    </ul>
    And a line break.<br/>Done.
  </div>
</section>
</body></html>`

	paragraphs, err := ParseDescription([]byte(googleHTML))
	if err != nil {
		t.Fatalf("ParseDescription failed: %v", err)
	}

	if len(paragraphs) != 1 {
		t.Fatalf("expected 1 paragraph, got %d", len(paragraphs))
	}

	expectedText := "Here is a list of features:\n" +
		"    first feature\n" +
		"    second feature\n\n" +
		"And a line break.\n" +
		"Done."

	if paragraphs[0].Text != expectedText {
		t.Errorf("Formatted Google description mismatch:\nwant:\n%q\ngot:\n%q", expectedText, paragraphs[0].Text)
	}
}
