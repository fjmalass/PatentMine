package crawl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"patentmine/internal/domain"
)

// usptoMinInterval keeps requests to the USPTO ODP API polite.
const usptoMinInterval = 1 * time.Second

// NewUSPTOSource builds a Source backed by the USPTO Open Data Portal (ODP) API.
func NewUSPTOSource(apiKey string) Source {
	return &httpSource{
		name:    domain.SourceUSPTO,
		client:  &http.Client{Timeout: httpTimeout},
		limiter: newLimiter(usptoMinInterval),
		urlFor: func(n domain.PatentNumber) string {
			return "https://api.uspto.gov/api/v1/patent/applications/search?q=patentNumberText:" + n.Serial
		},
		headers: func() http.Header {
			h := make(http.Header)
			h.Set("x-api-key", apiKey)
			h.Set("Accept", "application/json")
			return h
		},
		parse: parseUSPTO,
	}
}

// usptoResponse is the USPTO ODP search query response shape.
type usptoResponse struct {
	Results []struct {
		PatentNumberText      string `json:"patentNumberText"`
		InventionTitle        string `json:"inventionTitle"`
		AbstractText          string `json:"abstractText"`
		PatentGrantDate       string `json:"patentGrantDate"`
		GrantDate             string `json:"grantDate"`
		FilingDate            string `json:"filingDate"`
		PublicationDate       string `json:"publicationDate"`
		InventorNameBag       []struct {
			NameText string `json:"nameText"`
		} `json:"inventorNameBag"`
		AssigneeBag           []struct {
			OrganizationName string `json:"organizationName"`
		} `json:"assigneeBag"`
	} `json:"results"`
}

// parseUSPTO extracts a patent from a USPTO ODP API response.
func parseUSPTO(number domain.PatentNumber, body []byte) (Result, error) {
	var resp usptoResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return Result{}, fmt.Errorf("crawl/uspto: decode response: %w", err)
	}
	if len(resp.Results) == 0 {
		return Result{}, ErrNotAvailable
	}
	p := resp.Results[0]

	var inventors []domain.Inventor
	for _, inv := range p.InventorNameBag {
		if name := strings.TrimSpace(inv.NameText); name != "" {
			inventors = append(inventors, domain.Inventor(name))
		}
	}
	var assignees []string
	for _, a := range p.AssigneeBag {
		if org := strings.TrimSpace(a.OrganizationName); org != "" {
			assignees = append(assignees, org)
		}
	}

	dateStr := p.PatentGrantDate
	if dateStr == "" {
		dateStr = p.GrantDate
	}
	if dateStr == "" {
		dateStr = p.PublicationDate
	}
	grantDate := parseISODate(dateStr)
	filingDate := parseISODate(p.FilingDate)

	patent := domain.Patent{
		Number:          number,
		DisplayNumber:   number,
		Title:           strings.TrimSpace(p.InventionTitle),
		Abstract:        strings.TrimSpace(p.AbstractText),
		Assignee:        strings.Join(assignees, "; "),
		Inventors:       inventors,
		FetchState:      domain.FetchCached,
		Source:          domain.SourceUSPTO,
		FetchedAt:       time.Now().UTC(),
		ApplicationDate: filingDate,
		PublicationDate: grantDate,
		GrantDate:       grantDate,
	}
	doc := domain.Document{
		Number: number,
		Stage:  domain.GuessStage(number),
		Dated:  grantDate,
	}
	return Result{Patent: patent, Documents: []domain.Document{doc}}, nil
}

// parseISODate converts an ISO date string to a time.Time object.
func parseISODate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02", "2006/01/02", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
