package crawl

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"patentmine/internal/domain"
)

const usptoSampleResponse = `{
  "count": 1,
  "patentFileWrapperDataBag": [{
    "applicationNumberText": "16123456",
    "lastIngestionDateTime": "2026-05-20T00:00:00Z",
    "applicationMetaData": {
      "inventionTitle": "Widget apparatus",
      "filingDate": "2016-06-19",
      "effectiveFilingDate": "2016-06-19",
      "applicationStatusCode": 30,
      "applicationStatusDescriptionText": "Docketed New Case - Ready for Examination",
      "applicationStatusDate": "2026-05-20",
      "applicationTypeCode": "UTL",
      "applicationTypeLabelName": "Utility",
      "firstInventorToFileIndicator": "Y",
      "firstInventorName": "Jane Doe",
      "firstApplicantName": "Acme Corp",
      "customerNumber": 12345,
      "groupArtUnitNumber": "1234",
      "examinerNameText": "DOE, JANE",
      "docketNumber": "ACME-1",
      "inventorBag": [{"inventorNameText": "Jane Doe", "firstName": "Jane", "lastName": "Doe"}],
      "applicantBag": [{"applicantNameText": "Acme Corp"}],
      "patentNumber": "11611785",
      "patentNumberText": "11611785",
      "publicationNumber": "20220252571",
      "cpcClassificationBag": ["G06F 16/245", "H04L9/00"]
    },
    "eventDataBag": [{"eventCode":"M844","eventDescriptionText":"IDS Filed","eventDate":"2026-01-02"}],
    "parentContinuityBag": [{
      "parentApplicationNumberText": "15111111",
      "childApplicationNumberText": "16123456",
      "parentApplicationFilingDate": "2015-01-01",
      "claimParentageTypeCode": "CON",
      "claimParentageTypeCodeDescriptionText": "Continuation of"
    }],
    "grantDocumentMetaData": {
      "fileCreateDateTime": "2026-05-21T00:00:00Z"
    },
    "pgpubDocumentMetaData": {
      "fileCreateDateTime": "2026-05-22T00:00:00Z"
    }
  }]
}`

func TestParseUSPTOExtractsBibliographicFields(t *testing.T) {
	parseUSPTO := makeParseUSPTO("", &http.Client{})
	number := domain.MustParsePatentNumber("US16123456")
	res, err := parseUSPTO(context.Background(), number, []byte(usptoSampleResponse))
	if err != nil {
		t.Fatalf("parseUSPTO: %v", err)
	}
	if res.Patent.Title != "Widget apparatus" {
		t.Errorf("title = %q", res.Patent.Title)
	}
	if res.Patent.Assignee != "Acme Corp" {
		t.Errorf("assignee = %q", res.Patent.Assignee)
	}
	if len(res.Patent.Inventors) != 1 || res.Patent.Inventors[0] != "Jane Doe" {
		t.Errorf("inventors = %v", res.Patent.Inventors)
	}
	if len(res.Patent.Classifications) != 2 || res.Patent.Classifications[0] != "G06F16/245" || res.Patent.Classifications[1] != "H04L9/00" {
		t.Errorf("classifications from cpcClassificationBag = %v", res.Patent.Classifications)
	}
	if res.Patent.Source != domain.SourceUSPTO {
		t.Errorf("source = %q, want uspto", res.Patent.Source)
	}
	expectedURL := "https://patents.google.com/patent/US16123456/en"
	if res.Patent.SourceURL != expectedURL {
		t.Errorf("source URL = %q, want %q", res.Patent.SourceURL, expectedURL)
	}
	if res.Patent.ApplicationDate.Year() != 2016 {
		t.Errorf("application date = %v, want 2016", res.Patent.ApplicationDate)
	}
	if res.USPTOApplication == nil || res.USPTOApplication.ApplicationNumber != "16123456" {
		t.Fatalf("USPTOApplication = %+v", res.USPTOApplication)
	}
	if len(res.USPTOEvents) != 1 || res.USPTOEvents[0].EventCode != "M844" {
		t.Errorf("events = %+v", res.USPTOEvents)
	}
	counts := map[domain.RelationKind]int{}
	for _, rel := range res.Relations {
		counts[rel.Kind]++
	}
	if counts[domain.RelationParent] != 1 || counts[domain.RelationChild] != 1 {
		t.Errorf("continuity relations = %+v", res.Relations)
	}

	// Verify that the publication and grant documents were extracted
	var foundPub, foundGrant bool
	for _, d := range res.Documents {
		if d.Stage == domain.StagePublication && d.Number.Normalized() == "US20220252571" {
			foundPub = true
			if d.Dated.Year() != 2026 || d.Dated.Month() != 5 || d.Dated.Day() != 22 {
				t.Errorf("pub date = %v, want 2026-05-22", d.Dated)
			}
		}
		if d.Stage == domain.StageGrant && d.Number.Normalized() == "US11611785" {
			foundGrant = true
			if d.Dated.Year() != 2026 || d.Dated.Month() != 5 || d.Dated.Day() != 21 {
				t.Errorf("grant date = %v, want 2026-05-21", d.Dated)
			}
		}
	}
	if !foundPub {
		t.Error("missing StagePublication document in res.Documents")
	}
	if !foundGrant {
		t.Error("missing StageGrant document in res.Documents")
	}

	var foundPubID, foundGrantID bool
	for _, id := range res.AuthorityIdentifiers {
		if id.IdentifierType == "publication" && id.Identifier == "US20220252571" {
			foundPubID = true
		}
		if id.IdentifierType == "grant" && id.Identifier == "US11611785" {
			foundGrantID = true
		}
	}
	if !foundPubID {
		t.Error("missing publication AuthorityIdentifier")
	}
	if !foundGrantID {
		t.Error("missing grant AuthorityIdentifier")
	}
}

func TestParseUSPTOEmptyResultIsNotAvailable(t *testing.T) {
	parseUSPTO := makeParseUSPTO("", &http.Client{})
	number := domain.MustParsePatentNumber("US10000000B2")
	_, err := parseUSPTO(context.Background(), number, []byte(`{"count":0,"patentFileWrapperDataBag":[]}`))
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("parseUSPTO on an empty result = %v, want ErrNotAvailable", err)
	}
}

func TestMatchingUSPTOWrapperScoring(t *testing.T) {
	// Wrapper A: Application 11714053 (with patent number 7561063)
	wA := usptoWrapperData{
		ApplicationNumberText: "11714053",
	}
	wA.ApplicationMetaData.PatentNumber = "7561063"

	// Wrapper B: Application 17696256 (with patent number 11714053)
	wB := usptoWrapperData{
		ApplicationNumberText: "17696256",
	}
	wB.ApplicationMetaData.PatentNumber = "11714053"

	bags := []usptoWrapperData{wA, wB}

	// 1. Search for Grant US11714053B2 (serial 11714053, StageGrant)
	grantNum := domain.MustParsePatentNumber("US11714053B2")
	matched, ok := matchingUSPTOWrapper(grantNum, bags)
	if !ok {
		t.Fatalf("matchingUSPTOWrapper failed to match")
	}
	if matched.ApplicationNumberText != "17696256" {
		t.Errorf("matched application %q, want 17696256 (Wrapper B)", matched.ApplicationNumberText)
	}

	// 2. Search for Application US11714053 (serial 11714053, StagePublication by GuessStage)
	appNum := domain.MustParsePatentNumber("US11714053")
	matched2, ok := matchingUSPTOWrapper(appNum, bags)
	if !ok {
		t.Fatalf("matchingUSPTOWrapper failed to match application")
	}
	if matched2.ApplicationNumberText != "11714053" {
		t.Errorf("matched application %q, want 11714053 (Wrapper A)", matched2.ApplicationNumberText)
	}

	// 3. Search for Publication US20090222331A1 (serial 20090222331, StagePublication)
	wC := usptoWrapperData{
		ApplicationNumberText: "12388910",
	}
	wC.ApplicationMetaData.PublicationNumber = "US20090222331A1"

	bags3 := []usptoWrapperData{wC}
	pubNum := domain.MustParsePatentNumber("US20090222331A1")
	matched3, ok := matchingUSPTOWrapper(pubNum, bags3)
	if !ok {
		t.Fatalf("matchingUSPTOWrapper failed to match publication US20090222331A1")
	}
	if matched3.ApplicationNumberText != "12388910" {
		t.Errorf("matched application %q, want 12388910 (Wrapper C)", matched3.ApplicationNumberText)
	}
}

func TestUSPTORateLimitScalesWithSourceMode(t *testing.T) {
	// Ensure baseline values are set.
	USPTOMinInterval = 100 * time.Millisecond
	GoogleMinInterval = 3 * time.Second

	usptoSrc := NewUSPTOSource("")
	r := NewRegistry(usptoSrc)

	httpSrc := usptoSrc.(*httpSource)

	largest := GoogleMinInterval

	// Default mode (compare) should use the largest (3s)
	if got := httpSrc.limiter.interval; got != largest {
		t.Errorf("default interval = %v, want %v (largest)", got, largest)
	}

	// Change to uspto-only should select the appropriate time (100ms)
	if err := r.SetSourceMode(string(SourceModeUSPTOOnly)); err != nil {
		t.Fatalf("SetSourceMode: %v", err)
	}
	if got := httpSrc.limiter.interval; got != USPTOMinInterval {
		t.Errorf("uspto-only interval = %v, want %v (usptoMinInterval)", got, USPTOMinInterval)
	}

	// Change back to compare should use the largest again (3s)
	if err := r.SetSourceMode(string(SourceModeCompare)); err != nil {
		t.Fatalf("SetSourceMode: %v", err)
	}
	if got := httpSrc.limiter.interval; got != largest {
		t.Errorf("compare interval = %v, want %v (largest)", got, largest)
	}
}

func TestUSPTOQuery(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{
			in:   "US16123456",
			want: `applicationNumberText:16123456 OR patentNumberText:16123456 OR publicationNumberText:16123456 OR publicationNumber:16123456 OR "US16123456" OR "16123456"`,
		},
		{
			in:   "US20040260504",
			want: `applicationNumberText:20040260504 OR patentNumberText:20040260504 OR publicationNumberText:20040260504 OR publicationNumber:20040260504 OR "US20040260504" OR "20040260504" OR publicationNumberText:20040260504A1 OR publicationNumber:20040260504A1 OR "US20040260504A1"`,
		},
	}

	for _, tt := range tests {
		n := domain.MustParsePatentNumber(tt.in)
		got := usptoQuery(n)
		if got != tt.want {
			t.Errorf("usptoQuery(%s) =\n%q\nwant:\n%q", tt.in, got, tt.want)
		}
	}
}
