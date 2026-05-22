package crawl

import (
	"errors"
	"testing"

	"patentmine/internal/domain"
)

const usptoSampleResponse = `{"results":[{
	"patentNumberText":"10000000",
	"inventionTitle":"Widget apparatus",
	"abstractText":"An improved widget.",
	"patentGrantDate":"2018-06-19",
	"inventorNameBag":[{"nameText":"Jane Doe"}],
	"assigneeBag":[{"organizationName":"Acme Corp"}]
}]}`

func TestParseUSPTOExtractsBibliographicFields(t *testing.T) {
	number := domain.MustParsePatentNumber("US10000000B2")
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
	if res.Patent.GrantDate.Year() != 2018 {
		t.Errorf("grant date = %v, want 2018", res.Patent.GrantDate)
	}
}

func TestParseUSPTOEmptyResultIsNotAvailable(t *testing.T) {
	number := domain.MustParsePatentNumber("US10000000B2")
	_, err := parseUSPTO(number, []byte(`{"results":[]}`))
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("parseUSPTO on an empty result = %v, want ErrNotAvailable", err)
	}
}
