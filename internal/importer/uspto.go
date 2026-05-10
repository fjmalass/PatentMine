package importer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"patentmine/internal/domain"
)

const usptoBaseURL = "https://api.uspto.gov"

// ImportUSPTO fetches patent data from the USPTO Open Data Portal API.
// patentNumber may be in any common format: US12000000B2, US12000000, 12000000.
// Requires a valid ODP API key (register at developer.uspto.gov).
func ImportUSPTO(patentNumber, apiKey string) (domain.PatentBundle, error) {
	if strings.TrimSpace(apiKey) == "" {
		return domain.PatentBundle{}, fmt.Errorf("USPTO API key is required")
	}
	number := strings.ToUpper(strings.TrimSpace(patentNumber))
	if number == "" {
		return domain.PatentBundle{}, fmt.Errorf("patent number is required")
	}

	searchNum := extractODPNumber(number)
	if searchNum == "" {
		return domain.PatentBundle{}, fmt.Errorf("could not parse patent number: %s", patentNumber)
	}

	client := &http.Client{Timeout: 20 * time.Second}

	appNum, appData, err := usptoFetchByPatentNumber(client, apiKey, searchNum)
	if err != nil {
		return domain.PatentBundle{}, err
	}

	continuity, _ := usptoFetchContinuity(client, apiKey, appNum)
	return buildUSPTOBundle(number, appData, continuity), nil
}

// --- JSON response types ---

type usptoSearchResponse struct {
	Count                    int                    `json:"count"`
	PatentFileWrapperDataBag []usptoApplicationData `json:"patentFileWrapperDataBag"`
}

type usptoApplicationData struct {
	ApplicationNumberText string            `json:"applicationNumberText"`
	ApplicationMetaData   usptoMetaData     `json:"applicationMetaData"`
	AssignmentBag         []usptoAssignment `json:"assignmentBag"`
	ParentContinuityBag   []usptoContinuity `json:"parentContinuityBag"`
	ChildContinuityBag    []usptoContinuity `json:"childContinuityBag"`
}

type usptoMetaData struct {
	PatentNumber                     string          `json:"patentNumber"`
	InventionTitle                   string          `json:"inventionTitle"`
	InventorBag                      []usptoInventor `json:"inventorBag"`
	CPCClassificationBag             []string        `json:"cpcClassificationBag"`
	GrantDate                        string          `json:"grantDate"`
	FilingDate                       string          `json:"filingDate"`
	EarliestPublicationDate          string          `json:"earliestPublicationDate"`
	ApplicationStatusDescriptionText string          `json:"applicationStatusDescriptionText"`
}

type usptoInventor struct {
	FirstName        string `json:"firstName"`
	MiddleName       string `json:"middleName"`
	LastName         string `json:"lastName"`
	InventorNameText string `json:"inventorNameText"`
}

type usptoAssignment struct {
	AssigneeBag []usptoAssignee `json:"assigneeBag"`
}

type usptoAssignee struct {
	AssigneeNameText string `json:"assigneeNameText"`
}

type usptoContinuity struct {
	ParentApplicationNumberText           string `json:"parentApplicationNumberText"`
	ChildApplicationNumberText            string `json:"childApplicationNumberText"`
	ParentPatentNumber                    string `json:"parentPatentNumber"`
	ChildPatentNumber                     string `json:"childPatentNumber"`
	ClaimParentageTypeCode                string `json:"claimParentageTypeCode"`
	ClaimParentageTypeCodeDescriptionText string `json:"claimParentageTypeCodeDescriptionText"`
}

type usptoContinuityResponse struct {
	PatentFileWrapperDataBag []struct {
		ParentContinuityBag []usptoContinuity `json:"parentContinuityBag"`
		ChildContinuityBag  []usptoContinuity `json:"childContinuityBag"`
	} `json:"patentFileWrapperDataBag"`
}

// --- API calls ---

func usptoFetchByPatentNumber(client *http.Client, apiKey, searchNum string) (string, usptoApplicationData, error) {
	searchURL := fmt.Sprintf("%s/api/v1/patent/applications/search?q=patentNumber:(%s)", usptoBaseURL, searchNum)
	var searchResp usptoSearchResponse
	if err := usptoGET(client, apiKey, searchURL, &searchResp); err != nil {
		return "", usptoApplicationData{}, fmt.Errorf("USPTO search failed: %w", err)
	}
	if searchResp.Count == 0 || len(searchResp.PatentFileWrapperDataBag) == 0 {
		return "", usptoApplicationData{}, fmt.Errorf("patent not found in USPTO ODP: %s", searchNum)
	}
	appNum := searchResp.PatentFileWrapperDataBag[0].ApplicationNumberText

	// Fetch full application data by application number
	fullURL := fmt.Sprintf("%s/api/v1/patent/applications/%s", usptoBaseURL, appNum)
	var fullResp usptoSearchResponse
	if err := usptoGET(client, apiKey, fullURL, &fullResp); err == nil && len(fullResp.PatentFileWrapperDataBag) > 0 {
		return appNum, fullResp.PatentFileWrapperDataBag[0], nil
	}
	return appNum, searchResp.PatentFileWrapperDataBag[0], nil
}

func usptoFetchContinuity(client *http.Client, apiKey, appNum string) (usptoContinuityResponse, error) {
	url := fmt.Sprintf("%s/api/v1/patent/applications/%s/continuity", usptoBaseURL, appNum)
	var resp usptoContinuityResponse
	err := usptoGET(client, apiKey, url, &resp)
	return resp, err
}

func usptoGET(client *http.Client, apiKey, url string, dest interface{}) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("USPTO API key invalid or unauthorized (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("USPTO API returned HTTP %d for %s", resp.StatusCode, url)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

// --- Bundle builder ---

func buildUSPTOBundle(originalNumber string, data usptoApplicationData, continuity usptoContinuityResponse) domain.PatentBundle {
	meta := data.ApplicationMetaData

	// Canonical number: use ODP patentNumber with US prefix, else fall back to original
	number := strings.ToUpper(strings.TrimSpace(meta.PatentNumber))
	if number == "" {
		number = strings.ToUpper(strings.TrimSpace(originalNumber))
	} else if !strings.HasPrefix(number, "US") {
		number = "US" + number
	}

	// First assignee from most recent assignment
	assignee := ""
	for _, a := range data.AssignmentBag {
		for _, e := range a.AssigneeBag {
			if e.AssigneeNameText != "" {
				assignee = e.AssigneeNameText
				break
			}
		}
		if assignee != "" {
			break
		}
	}

	// Inventors
	var inventors []string
	seen := map[string]bool{}
	for _, inv := range meta.InventorBag {
		name := strings.TrimSpace(inv.InventorNameText)
		if name == "" {
			parts := []string{}
			if inv.FirstName != "" {
				parts = append(parts, inv.FirstName)
			}
			if inv.MiddleName != "" {
				parts = append(parts, inv.MiddleName)
			}
			if inv.LastName != "" {
				parts = append(parts, inv.LastName)
			}
			name = strings.Join(parts, " ")
		}
		name = strings.TrimSpace(name)
		if name != "" && !seen[name] {
			seen[name] = true
			inventors = append(inventors, name)
		}
	}

	// Google Patents URL as SourceURL for browser convenience
	sourceURL, _ := GooglePatentsURL(number)

	patent := domain.Patent{
		Number:          number,
		Title:           strings.TrimSpace(meta.InventionTitle),
		Assignee:        assignee,
		Inventors:       inventors,
		PublicationDate: meta.EarliestPublicationDate,
		GrantDate:       meta.GrantDate,
		SourceURL:       sourceURL,
	}
	if exp := estimatedExpirationDate(meta.EarliestPublicationDate, meta.GrantDate); exp != "" {
		patent.ExpirationDate = exp
		patent.ExpirationEstimated = true
	}
	if patent.Title == "" {
		patent.Title = number
	}

	bundle := domain.PatentBundle{Patent: patent}

	for _, code := range meta.CPCClassificationBag {
		cls := domain.ParseClassification(code)
		cls.PatentNumber = number
		bundle.Classifications = append(bundle.Classifications, cls)
	}

	// Family edges from dedicated continuity endpoint, or inline if not available
	var parentBag, childBag []usptoContinuity
	if len(continuity.PatentFileWrapperDataBag) > 0 {
		parentBag = continuity.PatentFileWrapperDataBag[0].ParentContinuityBag
		childBag = continuity.PatentFileWrapperDataBag[0].ChildContinuityBag
	} else {
		parentBag = data.ParentContinuityBag
		childBag = data.ChildContinuityBag
	}

	edgeSeen := map[string]bool{}
	addEdge := func(e domain.FamilyEdge) {
		if e.ParentNumber == "" || e.ChildNumber == "" {
			return
		}
		if strings.EqualFold(e.ParentNumber, e.ChildNumber) {
			return
		}
		key := e.ParentNumber + "\x00" + e.ChildNumber
		if !edgeSeen[key] {
			edgeSeen[key] = true
			bundle.FamilyEdges = append(bundle.FamilyEdges, e)
		}
	}
	for _, p := range parentBag {
		if parentNum := usptoNormalizePatentNumber(p.ParentPatentNumber); parentNum != "" {
			addEdge(domain.FamilyEdge{
				ParentNumber: parentNum,
				ChildNumber:  number,
				RelationType: mapContinuityType(p.ClaimParentageTypeCode),
			})
		}
	}
	for _, c := range childBag {
		if childNum := usptoNormalizePatentNumber(c.ChildPatentNumber); childNum != "" {
			addEdge(domain.FamilyEdge{
				ParentNumber: number,
				ChildNumber:  childNum,
				RelationType: mapContinuityType(c.ClaimParentageTypeCode),
			})
		}
	}

	bundle.References = append(bundle.References, domain.ReferenceEntry{
		PatentNumber:  number,
		CitationLabel: fmt.Sprintf("%s, %s", number, patent.Title),
	})

	return bundle
}

func usptoNormalizePatentNumber(n string) string {
	n = strings.TrimSpace(n)
	if n == "" {
		return ""
	}
	n = strings.ToUpper(n)
	if !strings.HasPrefix(n, "US") {
		n = "US" + n
	}
	return n
}

// extractODPNumber strips country code prefix and kind code suffix,
// returning just the numeric portion for use in ODP search queries.
func extractODPNumber(number string) string {
	// Skip leading non-digit characters (country code: US, EP, WO, etc.)
	i := 0
	for i < len(number) && !unicode.IsDigit(rune(number[i])) {
		i++
	}
	number = number[i:]
	// Strip trailing kind code: one or two letters followed by a digit (B2, A1, B1, E1, etc.)
	for len(number) >= 2 {
		last := number[len(number)-1]
		prev := number[len(number)-2]
		if unicode.IsDigit(rune(last)) && !unicode.IsDigit(rune(prev)) {
			number = number[:len(number)-2]
		} else {
			break
		}
	}
	return number
}

func mapContinuityType(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "DIV":
		return domain.FamilyRelationDivisional
	case "CIP":
		return domain.FamilyRelationCIP
	case "PCT":
		return domain.FamilyRelationPCT
	default:
		return domain.FamilyRelationContinuation
	}
}
