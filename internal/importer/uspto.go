package importer

import (
	"encoding/json"
	"fmt"
	"log/slog"
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
func ImportUSPTO(patentNumber, apiKey string, logger *slog.Logger) (domain.PatentBundle, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
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

	logger.Info("uspto.import", "patent", patentNumber, "search_num", searchNum)
	client := &http.Client{Timeout: 20 * time.Second}

	appNum, appData, err := usptoFetchByPatentNumber(client, apiKey, searchNum, logger)
	if err != nil {
		logger.Error("uspto.import failed", "patent", patentNumber, "error", err)
		return domain.PatentBundle{}, err
	}

	continuity, _ := usptoFetchContinuity(client, apiKey, appNum, logger)
	forwardNums := usptoFetchForwardCitations(client, apiKey, searchNum, logger)
	bundle := buildUSPTOBundle(number, appData, continuity, forwardNums, logger)
	logger.Info("uspto.import ok", "patent", bundle.Patent.Number, "citations", len(bundle.Citations), "family_edges", len(bundle.FamilyEdges), "classifications", len(bundle.Classifications))
	return bundle, nil
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
	PatentNumber                     string                `json:"patentNumber"`
	InventionTitle                   string                `json:"inventionTitle"`
	InventorBag                      []usptoInventor       `json:"inventorBag"`
	CPCClassificationBag             []string              `json:"cpcClassificationBag"`
	GrantDate                        string                `json:"grantDate"`
	FilingDate                       string                `json:"filingDate"`
	EarliestPublicationDate          string                `json:"earliestPublicationDate"`
	ApplicationStatusDescriptionText string                `json:"applicationStatusDescriptionText"`
	ReferenceCitedBag                []usptoReferenceCited `json:"referenceCitedBag"`
}

type usptoReferenceCited struct {
	PatentNumber    string `json:"patentNumber"`
	PatentKindCode  string `json:"patentKindCode"`
	PublicationDate string `json:"publicationDate"`
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

func usptoFetchByPatentNumber(client *http.Client, apiKey, searchNum string, logger *slog.Logger) (string, usptoApplicationData, error) {
	searchURL := fmt.Sprintf("%s/api/v1/patent/applications/search?q=patentNumber:(%s)", usptoBaseURL, searchNum)
	logger.Debug("uspto.search", "url", searchURL)
	var searchResp usptoSearchResponse
	if err := usptoGET(client, apiKey, searchURL, &searchResp, logger); err != nil {
		return "", usptoApplicationData{}, fmt.Errorf("USPTO search failed: %w", err)
	}
	if searchResp.Count == 0 || len(searchResp.PatentFileWrapperDataBag) == 0 {
		return "", usptoApplicationData{}, fmt.Errorf("patent not found in USPTO ODP: %s", searchNum)
	}
	appNum := searchResp.PatentFileWrapperDataBag[0].ApplicationNumberText
	logger.Info("uspto.app_found", "search_num", searchNum, "app_num", appNum)

	// Fetch full application data by application number
	fullURL := fmt.Sprintf("%s/api/v1/patent/applications/%s", usptoBaseURL, appNum)
	logger.Debug("uspto.app_fetch", "url", fullURL)
	var fullResp usptoSearchResponse
	if err := usptoGET(client, apiKey, fullURL, &fullResp, logger); err == nil && len(fullResp.PatentFileWrapperDataBag) > 0 {
		return appNum, fullResp.PatentFileWrapperDataBag[0], nil
	}
	return appNum, searchResp.PatentFileWrapperDataBag[0], nil
}

func usptoFetchContinuity(client *http.Client, apiKey, appNum string, logger *slog.Logger) (usptoContinuityResponse, error) {
	url := fmt.Sprintf("%s/api/v1/patent/applications/%s/continuity", usptoBaseURL, appNum)
	logger.Debug("uspto.continuity", "app_num", appNum, "url", url)
	var resp usptoContinuityResponse
	err := usptoGET(client, apiKey, url, &resp, logger)
	if err != nil {
		logger.Warn("uspto.continuity failed", "app_num", appNum, "error", err)
	} else {
		var parents, children int
		if len(resp.PatentFileWrapperDataBag) > 0 {
			parents = len(resp.PatentFileWrapperDataBag[0].ParentContinuityBag)
			children = len(resp.PatentFileWrapperDataBag[0].ChildContinuityBag)
		}
		logger.Info("uspto.continuity ok", "app_num", appNum, "parents", parents, "children", children)
	}
	return resp, err
}

// usptoFetchForwardCitations searches for all patents that cite patentNum using
// the forwardReferencedPatentNumber query field. Results are paginated (25/page).
func usptoFetchForwardCitations(client *http.Client, apiKey, searchNum string, logger *slog.Logger) []string {
	const pageSize = 25
	const maxPages = 40 // cap at 1000 forward citations
	var nums []string
	seen := map[string]bool{}
	logger.Info("uspto.fwd_citations", "search_num", searchNum)
	for page := 0; page < maxPages; page++ {
		url := fmt.Sprintf("%s/api/v1/patent/applications/search?q=forwardReferencedPatentNumber:(%s)&fields=applicationMetaData.patentNumber&rows=%d&start=%d",
			usptoBaseURL, searchNum, pageSize, page*pageSize)
		var resp usptoSearchResponse
		if err := usptoGET(client, apiKey, url, &resp, logger); err != nil {
			logger.Warn("uspto.fwd_citations page failed", "search_num", searchNum, "page", page, "error", err)
			break
		}
		count := len(resp.PatentFileWrapperDataBag)
		logger.Debug("uspto.fwd_citations page", "search_num", searchNum, "page", page, "count", count)
		for _, app := range resp.PatentFileWrapperDataBag {
			if num := usptoNormalizePatentNumber(app.ApplicationMetaData.PatentNumber); num != "" && !seen[num] {
				seen[num] = true
				nums = append(nums, num)
			}
		}
		if count < pageSize {
			break
		}
	}
	logger.Info("uspto.fwd_citations done", "search_num", searchNum, "total", len(nums))
	return nums
}

func usptoGET(client *http.Client, apiKey, url string, dest any, logger *slog.Logger) error {
	logger.Debug("uspto.get", "url", url)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-KEY", apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("uspto.get request failed", "url", url, "error", err)
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	logger.Debug("uspto.get response", "url", url, "status", resp.StatusCode)
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return fmt.Errorf("USPTO API key invalid or unauthorized (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("USPTO API returned HTTP %d for %s", resp.StatusCode, url)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

// --- Bundle builder ---

func buildUSPTOBundle(originalNumber string, data usptoApplicationData, continuity usptoContinuityResponse, forwardNums []string, logger *slog.Logger) domain.PatentBundle {
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

	sourceURL, _ := GooglePatentsURL(number)

	patent := domain.Patent{
		Number:          number,
		Title:           strings.TrimSpace(meta.InventionTitle),
		Assignee:        assignee,
		Inventors:       inventors,
		PublicationDate: meta.EarliestPublicationDate,
		GrantDate:       meta.GrantDate,
		SourceGoogleURL: sourceURL,
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

	// Backward citations: references cited by this patent
	citeSeen := map[string]bool{}
	for _, ref := range meta.ReferenceCitedBag {
		num := usptoNormalizePatentNumber(ref.PatentNumber)
		if num == "" || strings.EqualFold(num, number) || citeSeen[num] {
			continue
		}
		citeSeen[num] = true
		bundle.Citations = append(bundle.Citations, domain.CitationEdge{
			SourcePatent: number,
			TargetPatent: num,
			RelationType: domain.RelationCites,
		})
	}

	// Forward citations: patents that cite this patent
	for _, num := range forwardNums {
		if strings.EqualFold(num, number) {
			continue
		}
		bundle.Citations = append(bundle.Citations, domain.CitationEdge{
			SourcePatent: number,
			TargetPatent: num,
			RelationType: domain.RelationCitedBy,
		})
	}

	bundle.References = append(bundle.References, domain.ReferenceEntry{
		PatentNumber:  number,
		CitationLabel: fmt.Sprintf("%s, %s", number, patent.Title),
	})

	logger.Debug("uspto.build_bundle", "number", number, "citations", len(bundle.Citations), "family_edges", len(bundle.FamilyEdges), "classifications", len(bundle.Classifications), "sections", len(bundle.Sections))
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
