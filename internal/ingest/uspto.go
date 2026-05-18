package ingest

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"patentmine/internal/domain"
)

// usptoMinInterval keeps requests to the USPTO data endpoint polite.
const usptoMinInterval = 1 * time.Second

// usptoFields are the PatentsView fields the parser consumes.
const usptoFields = `["patent_number","patent_title","patent_abstract","patent_date",` +
	`"inventor_first_name","inventor_last_name","assignee_organization"]`

// NewUSPTOSource builds a Source backed by the PatentsView API, which serves
// USPTO grant data as JSON.
func NewUSPTOSource() Source {
	return newHTTPSource(
		domain.SourceUSPTO,
		usptoMinInterval,
		func(n domain.PatentNumber) string {
			query := `{"patent_number":"` + n.Serial + `"}`
			return "https://api.patentsview.org/patents/query?q=" +
				url.QueryEscape(query) + "&f=" + url.QueryEscape(usptoFields)
		},
		parseUSPTO,
	)
}

// usptoResponse is the PatentsView query response shape.
type usptoResponse struct {
	Patents []struct {
		Number    string `json:"patent_number"`
		Title     string `json:"patent_title"`
		Abstract  string `json:"patent_abstract"`
		Date      string `json:"patent_date"`
		Inventors []struct {
			First string `json:"inventor_first_name"`
			Last  string `json:"inventor_last_name"`
		} `json:"inventors"`
		Assignees []struct {
			Organization string `json:"assignee_organization"`
		} `json:"assignees"`
	} `json:"patents"`
}

// parseUSPTO extracts a patent from a PatentsView API response.
func parseUSPTO(number domain.PatentNumber, body []byte) (Result, error) {
	var resp usptoResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return Result{}, fmt.Errorf("ingest/uspto: decode response: %w", err)
	}
	if len(resp.Patents) == 0 {
		return Result{}, ErrNotAvailable
	}
	p := resp.Patents[0]

	var inventors []string
	for _, inv := range p.Inventors {
		if name := strings.TrimSpace(inv.First + " " + inv.Last); name != "" {
			inventors = append(inventors, name)
		}
	}
	var assignees []string
	for _, a := range p.Assignees {
		if org := strings.TrimSpace(a.Organization); org != "" {
			assignees = append(assignees, org)
		}
	}

	grantDate := parseGoogleDate(p.Date)
	patent := domain.Patent{
		Number:        number,
		DisplayNumber: number,
		Title:         strings.TrimSpace(p.Title),
		Abstract:      strings.TrimSpace(p.Abstract),
		Assignee:      strings.Join(assignees, "; "),
		Inventors:     inventors,
		FetchState:    domain.FetchCached,
		Source:        domain.SourceUSPTO,
		FetchedAt:     time.Now().UTC(),
		GrantDate:     grantDate,
	}
	doc := domain.Document{
		Number: number,
		Stage:  domain.GuessStage(number),
		Dated:  grantDate,
	}
	return Result{Patent: patent, Documents: []domain.Document{doc}}, nil
}
