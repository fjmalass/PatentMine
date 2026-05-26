package crawl

import (
	"errors"
	"testing"

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
      "applicantBag": [{"applicantNameText": "Acme Corp"}]
    },
    "eventDataBag": [{"eventCode":"M844","eventDescriptionText":"IDS Filed","eventDate":"2026-01-02"}],
    "parentContinuityBag": [{
      "parentApplicationNumberText": "15111111",
      "childApplicationNumberText": "16123456",
      "parentApplicationFilingDate": "2015-01-01",
      "claimParentageTypeCode": "CON",
      "claimParentageTypeCodeDescriptionText": "Continuation of"
    }]
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
}

func TestParseUSPTOEmptyResultIsNotAvailable(t *testing.T) {
	number := domain.MustParsePatentNumber("US10000000B2")
	_, err := parseUSPTO(number, []byte(`{"count":0,"patentFileWrapperDataBag":[]}`))
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("parseUSPTO on an empty result = %v, want ErrNotAvailable", err)
	}
}
