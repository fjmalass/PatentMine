package crawl

import (
	"errors"
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
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
      "publicationNumber": "20220252571"
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
	number := domain.MustParsePatentNumber("US16123456")
	res, err := parseUSPTO(number, []byte(usptoSampleResponse))
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
	if res.Patent.Source != domain.SourceUSPTO {
		t.Errorf("source = %q, want uspto", res.Patent.Source)
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
	number := domain.MustParsePatentNumber("US10000000B2")
	_, err := parseUSPTO(number, []byte(`{"count":0,"patentFileWrapperDataBag":[]}`))
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

	// 3. Search for Application US20140283408A1
	wC := usptoWrapperData{
		ApplicationNumberText: "14283408",
	}
	wC.ApplicationMetaData.PublicationNumber = "US20140283408A1"
	matched3, ok := matchingUSPTOWrapper(domain.MustParsePatentNumber("US20140283408A1"), []usptoWrapperData{wC})
	if !ok {
		t.Fatalf("matchingUSPTOWrapper failed to match application US20140283408A1")
	}
	if matched3.ApplicationNumberText != "14283408" {
		t.Errorf("matched application %q, want 14283408 (Wrapper C)", matched3.ApplicationNumberText)
	}

	// 4. Search for Application with slashes and special formatting
	wD := usptoWrapperData{
		ApplicationNumberText: "14/283,408",
	}
	wD.ApplicationMetaData.PublicationNumber = "US-2014/0283408-A1"
	matched4, ok := matchingUSPTOWrapper(domain.MustParsePatentNumber("US20140283408A1"), []usptoWrapperData{wD})
	if !ok {
		t.Fatalf("matchingUSPTOWrapper failed to match application with slashes US-2014/0283408-A1")
	}
	if matched4.ApplicationNumberText != "14/283,408" {
		t.Errorf("matched application %q, want 14/283,408 (Wrapper D)", matched4.ApplicationNumberText)
	}
}

func TestMatchesPatentTelemetry(t *testing.T) {
	oldMetrics := Metrics
	Metrics = observability.NewMetrics()
	defer func() { Metrics = oldMetrics }()

	t1 := domain.MustParsePatentNumber("US20140283408A1")
	if !matchesPatent("US20140283408A1", t1) {
		t.Error("expected US20140283408A1 to match")
	}

	t2 := domain.MustParsePatentNumber("US11611785B2")
	if matchesPatent("US20140283408A1", t2) {
		t.Error("expected US20140283408A1 to not match US11611785B2")
	}

	t3 := domain.MustParsePatentNumber("US123")
	if !matchesPatent("abc-123", t3) {
		t.Error("expected abc-123 to match US123 via fallback")
	}

	snap := Metrics.Snapshot()
	
	if snap.Counters["crawl.uspto.matches_patent.calls_total"] != 3 {
		t.Errorf("calls_total = %d, want 3", snap.Counters["crawl.uspto.matches_patent.calls_total"])
	}

	if snap.Counters["crawl.uspto.matches_patent.parse_success_total"] != 2 {
		t.Errorf("parse_success_total = %d, want 2", snap.Counters["crawl.uspto.matches_patent.parse_success_total"])
	}

	if snap.Counters["crawl.uspto.matches_patent.parse_fallback_total"] != 1 {
		t.Errorf("parse_fallback_total = %d, want 1", snap.Counters["crawl.uspto.matches_patent.parse_fallback_total"])
	}

	if snap.Counters["crawl.uspto.matches_patent.matched_total"] != 2 {
		t.Errorf("matched_total = %d, want 2", snap.Counters["crawl.uspto.matches_patent.matched_total"])
	}
}
