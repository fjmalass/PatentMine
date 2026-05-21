package crawl

import (
	"errors"
	"testing"

	"patentmine/internal/domain"
)

const googleSamplePage = `<!doctype html><html><head>
<meta name="DC.title" content="Fallback title"/>
</head><body>
<span itemprop="title">Widget apparatus and method </span>
<section itemprop="abstract"><div class="abstract">An improved widget.</div></section>
<dd itemprop="assigneeOriginal">Acme Corp</dd>
<dd itemprop="inventor">Jane Doe</dd>
<dd itemprop="inventor">John Roe</dd>
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
	// Publication 2018 + 20 years.
	if res.Patent.ExpirationDate.Year() != 2038 {
		t.Errorf("expiration date = %v, want 2038", res.Patent.ExpirationDate)
	}
	if res.Patent.ExpirationSource != domain.ExpirationEstimated {
		t.Errorf("expiration source = %q, want estimated", res.Patent.ExpirationSource)
	}
}

func TestParseGoogleUnknownPageIsNotAvailable(t *testing.T) {
	number := domain.MustParsePatentNumber("US10000000B2")
	_, err := parseGoogle(number, []byte("<html><head></head><body>nothing here</body></html>"))
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("parseGoogle on a non-patent page = %v, want ErrNotAvailable", err)
	}
}
