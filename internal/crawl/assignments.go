package crawl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"patentmine/internal/domain"
)

// usptoAssignmentSearchURL is the USPTO Open Data Portal assignments search
// endpoint. The query parameter mirrors patent search: a Lucene-style expression
// over assignment fields (applicationNumberText, patentNumberText, etc.).
const usptoAssignmentSearchURL = "https://api.uspto.gov/api/v1/patent/assignments/search"

// usptoAssignmentResponse is the search-response envelope. Field shapes mirror
// the patent search response to stay consistent with the rest of the package.
type usptoAssignmentResponse struct {
	Count                  int                       `json:"count"`
	RequestIdentifier      string                    `json:"requestIdentifier"`
	PatentAssignmentBag    []usptoAssignmentDataItem `json:"patentAssignmentBag"`
}

// usptoAssignmentDataItem is one recorded assignment plus its parties. Field
// names follow the USPTO ODP JSON casing. Any field the API omits decodes as
// zero — the ingest layer treats every column as optional.
type usptoAssignmentDataItem struct {
	ReelNumber                    string                     `json:"reelNumber"`
	FrameNumber                   string                     `json:"frameNumber"`
	ReelAndFrameNumber            string                     `json:"reelAndFrameNumber"`
	ApplicationNumberText         string                     `json:"applicationNumberText"`
	PatentNumberText              string                     `json:"patentNumberText"`
	ConveyanceText                string                     `json:"conveyanceText"`
	AssignmentReceivedDate        string                     `json:"assignmentReceivedDate"`
	AssignmentRecordedDate        string                     `json:"assignmentRecordedDate"`
	AssignmentMailedDate          string                     `json:"assignmentMailedDate"`
	AssignmentDocumentLocationURI string                     `json:"assignmentDocumentLocationURI"`
	AttorneyDocketNumber          string                     `json:"attorneyDocketNumber"`
	PageTotalQuantity             int                        `json:"pageTotalQuantity"`
	ImageAvailable                bool                       `json:"imageAvailable"`
	CorrespondenceAddress         map[string]any             `json:"correspondenceAddress"`
	AssignorBag                   []usptoAssignmentPartyJSON `json:"assignorBag"`
	AssigneeBag                   []usptoAssignmentPartyJSON `json:"assigneeBag"`
}

type usptoAssignmentPartyJSON struct {
	NameLineOneText string         `json:"nameLineOneText"`
	ExecutionDate   string         `json:"executionDate"`
	Address         map[string]any `json:"address"`
}

// FetchUSPTOAssignments calls the assignment search endpoint for one
// application number and returns the recorded chain in document order. An
// HTTP 404 returns nil, nil so a patent with no recorded assignment is not a
// hard error. An empty applicationNumber returns nil, nil.
func FetchUSPTOAssignments(ctx context.Context, apiKey, applicationNumber string) ([]domain.USPTOAssignment, error) {
	appNum := strings.TrimSpace(applicationNumber)
	if appNum == "" {
		return nil, nil
	}

	q := fmt.Sprintf("applicationNumberText:%s", appNum)
	apiURL := usptoAssignmentSearchURL + "?q=" + url.QueryEscape(q)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("uspto assignments: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("uspto assignments: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("uspto assignments: read: %w", err)
	}

	var env usptoAssignmentResponse
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("uspto assignments: decode: %w", err)
	}

	out := make([]domain.USPTOAssignment, 0, len(env.PatentAssignmentBag))
	for i, raw := range env.PatentAssignmentBag {
		assignment := domain.USPTOAssignment{
			ApplicationNumber:             appNum,
			Ordinal:                       i,
			ReelAndFrameNumber:            firstNonEmpty(raw.ReelAndFrameNumber, reelFrame(raw.ReelNumber, raw.FrameNumber)),
			ReelNumber:                    raw.ReelNumber,
			FrameNumber:                   raw.FrameNumber,
			ConveyanceText:                raw.ConveyanceText,
			AssignmentReceivedDate:        raw.AssignmentReceivedDate,
			AssignmentRecordedDate:        raw.AssignmentRecordedDate,
			AssignmentMailedDate:          raw.AssignmentMailedDate,
			AssignmentDocumentLocationURI: raw.AssignmentDocumentLocationURI,
			AttorneyDocketNumber:          raw.AttorneyDocketNumber,
			PageTotalQuantity:             raw.PageTotalQuantity,
			ImageAvailable:                raw.ImageAvailable,
			CorrespondenceAddressJSON:     marshalJSON(raw.CorrespondenceAddress),
		}
		for j, p := range raw.AssignorBag {
			assignment.Parties = append(assignment.Parties, domain.USPTOAssignmentParty{
				ApplicationNumber: appNum,
				AssignmentOrdinal: i,
				Role:              "assignor",
				Ordinal:           j,
				NameText:          p.NameLineOneText,
				ExecutionDate:     p.ExecutionDate,
				AddressJSON:       marshalJSON(p.Address),
			})
		}
		for j, p := range raw.AssigneeBag {
			assignment.Parties = append(assignment.Parties, domain.USPTOAssignmentParty{
				ApplicationNumber: appNum,
				AssignmentOrdinal: i,
				Role:              "assignee",
				Ordinal:           j,
				NameText:          p.NameLineOneText,
				ExecutionDate:     p.ExecutionDate,
				AddressJSON:       marshalJSON(p.Address),
			})
		}
		out = append(out, assignment)
	}
	return out, nil
}

func reelFrame(reel, frame string) string {
	r := strings.TrimSpace(reel)
	f := strings.TrimSpace(frame)
	if r == "" && f == "" {
		return ""
	}
	return r + "/" + f
}

func marshalJSON(v map[string]any) string {
	if len(v) == 0 {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
