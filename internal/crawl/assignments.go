package crawl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"patentmine/internal/domain"
)

// usptoAssignmentAppURL is the working USPTO ODP endpoint for pulling a specific application's assignments.
const usptoAssignmentAppURL = "https://api.uspto.gov/api/v1/patent/applications/%s/assignment"

// usptoAssignmentResponse represents the response structure returned by the ODP application assignment endpoint.
type usptoAssignmentResponse struct {
	Count                    int                              `json:"count"`
	RequestIdentifier        string                           `json:"requestIdentifier"`
	PatentFileWrapperDataBag []usptoPatentFileWrapperDataItem `json:"patentFileWrapperDataBag"`
}

type usptoPatentFileWrapperDataItem struct {
	AssignmentBag         []usptoAssignmentDataItem `json:"assignmentBag"`
	ApplicationNumberText string                    `json:"applicationNumberText"`
}

// usptoAssignmentDataItem holds the transaction details of one assignment event.
type usptoAssignmentDataItem struct {
	ReelNumber                    any                        `json:"reelNumber"`
	FrameNumber                   any                        `json:"frameNumber"`
	ReelAndFrameNumber            string                     `json:"reelAndFrameNumber"`
	ConveyanceText                string                     `json:"conveyanceText"`
	AssignmentReceivedDate        string                     `json:"assignmentReceivedDate"`
	AssignmentRecordedDate        string                     `json:"assignmentRecordedDate"`
	AssignmentMailedDate          string                     `json:"assignmentMailedDate"`
	AssignmentDocumentLocationURI string                     `json:"assignmentDocumentLocationURI"`
	AttorneyDocketNumber          string                     `json:"attorneyDocketNumber"`
	PageTotalQuantity             int                        `json:"pageTotalQuantity"`
	ImageAvailableStatusCode      bool                       `json:"imageAvailableStatusCode"`
	CorrespondenceAddress         map[string]any             `json:"correspondenceAddress"`
	AssignorBag                   []usptoAssignmentPartyJSON `json:"assignorBag"`
	AssigneeBag                   []usptoAssignmentPartyJSON `json:"assigneeBag"`
}

type usptoAssignmentPartyJSON struct {
	NameLineOneText  string         `json:"nameLineOneText"`
	AssigneeNameText string         `json:"assigneeNameText"`
	AssignorName     string         `json:"assignorName"`
	ExecutionDate    string         `json:"executionDate"`
	Address          map[string]any `json:"address"`
	AssigneeAddress  map[string]any `json:"assigneeAddress"`
}

// FetchUSPTOAssignments calls the USPTO ODP applications/assignment endpoint for the given
// application number and returns the recorded assignments in document order.
func FetchUSPTOAssignments(ctx context.Context, apiKey, applicationNumber string) ([]domain.USPTOAssignment, error) {
	appNum := strings.TrimSpace(applicationNumber)
	if appNum == "" {
		return nil, nil
	}

	apiURL := fmt.Sprintf(usptoAssignmentAppURL, appNum)

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

	var out []domain.USPTOAssignment
	ordinal := 0
	for _, wrapper := range env.PatentFileWrapperDataBag {
		for _, raw := range wrapper.AssignmentBag {
			reelStr := parseStringOrInt(raw.ReelNumber)
			frameStr := parseStringOrInt(raw.FrameNumber)

			assignment := domain.USPTOAssignment{
				ApplicationNumber:             appNum,
				Ordinal:                       ordinal,
				ReelAndFrameNumber:            firstNonEmpty(raw.ReelAndFrameNumber, reelFrame(reelStr, frameStr)),
				ReelNumber:                    reelStr,
				FrameNumber:                   frameStr,
				ConveyanceText:                raw.ConveyanceText,
				AssignmentReceivedDate:        raw.AssignmentReceivedDate,
				AssignmentRecordedDate:        raw.AssignmentRecordedDate,
				AssignmentMailedDate:          raw.AssignmentMailedDate,
				AssignmentDocumentLocationURI: raw.AssignmentDocumentLocationURI,
				AttorneyDocketNumber:          raw.AttorneyDocketNumber,
				PageTotalQuantity:             raw.PageTotalQuantity,
				ImageAvailable:                raw.ImageAvailableStatusCode,
				CorrespondenceAddressJSON:     marshalJSON(raw.CorrespondenceAddress),
			}

			for j, p := range raw.AssignorBag {
				name := firstNonEmpty(p.AssignorName, p.NameLineOneText)
				assignment.Parties = append(assignment.Parties, domain.USPTOAssignmentParty{
					ApplicationNumber: appNum,
					AssignmentOrdinal: ordinal,
					Role:              "assignor",
					Ordinal:           j,
					NameText:          name,
					ExecutionDate:     p.ExecutionDate,
					AddressJSON:       marshalJSON(p.Address),
				})
			}

			for j, p := range raw.AssigneeBag {
				name := firstNonEmpty(p.AssigneeNameText, p.NameLineOneText)
				addr := p.AssigneeAddress
				if len(addr) == 0 {
					addr = p.Address
				}
				assignment.Parties = append(assignment.Parties, domain.USPTOAssignmentParty{
					ApplicationNumber: appNum,
					AssignmentOrdinal: ordinal,
					Role:              "assignee",
					Ordinal:           j,
					NameText:          name,
					ExecutionDate:     p.ExecutionDate,
					AddressJSON:       marshalJSON(addr),
				})
			}

			out = append(out, assignment)
			ordinal++
		}
	}
	return out, nil
}

func parseStringOrInt(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return fmt.Sprintf("%.0f", val)
	case int:
		return fmt.Sprintf("%d", val)
	default:
		return fmt.Sprintf("%v", v)
	}
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
