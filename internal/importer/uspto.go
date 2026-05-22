package importer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode"

	"patentmine/internal/domain"
	"patentmine/internal/logging"
)

var usptoBaseURL = "https://api.uspto.gov"

var errUSPTONotFound = errors.New("USPTO: resource not found")

const USPTOImporterVersion = "1.5.0"

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

	if os.Getenv(logging.EnvDebug) == "1" {
		fmt.Fprintf(os.Stderr, "\n[USPTO v%s] Importing %s (Search: %s)\n", USPTOImporterVersion, patentNumber, searchNum)
	}

	logger.Info("uspto.import", "v", USPTOImporterVersion, "patent", patentNumber, "search_num", searchNum)
	client := &http.Client{Timeout: 30 * time.Second}

	// 1. Identify Application Number
	appNum, appData, err := usptoFetchByPatentNumber(client, apiKey, number, logger)
	if err != nil {
		logger.Error("uspto.import failed", "patent", patentNumber, "error", err)
		return domain.PatentBundle{}, err
	}

	// 2. Deep Fetch: Explicitly call sub-endpoints
	// Increased breather for stability
	breather := func() { time.Sleep(500 * time.Millisecond) }

	breather()
	metaData, err := usptoFetchMetadata(client, apiKey, appNum, logger)
	if err == nil && metaData.ApplicationMetaData.InventionTitle != "" {
		// Merge: preserve fields from appData if missing in metaData
		if len(metaData.ApplicationMetaData.CPCClassificationBag) == 0 && len(appData.ApplicationMetaData.CPCClassificationBag) > 0 {
			metaData.ApplicationMetaData.CPCClassificationBag = appData.ApplicationMetaData.CPCClassificationBag
		}
		if len(metaData.ApplicationMetaData.InventorBag) == 0 && len(appData.ApplicationMetaData.InventorBag) > 0 {
			metaData.ApplicationMetaData.InventorBag = appData.ApplicationMetaData.InventorBag
		}

		// Merge top-level bags
		if len(metaData.ReferenceCitedBag) > 0 {
			appData.ReferenceCitedBag = metaData.ReferenceCitedBag
		}
		if len(metaData.ReferencedCitedBag) > 0 {
			appData.ReferencedCitedBag = metaData.ReferencedCitedBag
		}
		if len(metaData.AssignmentBag) > 0 {
			appData.AssignmentBag = metaData.AssignmentBag
		}
		if len(metaData.ApplicantBag) > 0 {
			appData.ApplicantBag = metaData.ApplicantBag
		}

		appData.ApplicationMetaData = metaData.ApplicationMetaData
	}

	breather()
	assignments, err := usptoFetchAssignments(client, apiKey, appNum, logger)
	if err == nil && len(assignments) > 0 {
		appData.AssignmentBag = assignments
	}

	breather()
	pta, _ := usptoFetchAdjustments(client, apiKey, appNum, logger)
	if pta != nil {
		appData.PatentTermAdjustmentData = pta
	}

	breather()
	continuity, _ := usptoFetchContinuity(client, apiKey, appNum, logger)

	breather()
	oaCitations, _ := usptoFetchOfficeActionCitations(client, apiKey, appNum, logger)

	breather()
	// Pad searchNum for forward citations to 8 digits if it's a 7-digit utility patent
	citationSearchNum := searchNum
	if len(citationSearchNum) == 7 {
		citationSearchNum = "0" + citationSearchNum
	}
	forwardNums, forwardCount := usptoFetchForwardCitations(client, apiKey, number, citationSearchNum, logger)

	bundle := buildUSPTOBundle(number, appData, continuity, forwardNums, logger)

	// Fetch documents and add to Sections or References if relevant
	breather()
	docs, _ := usptoFetchDocuments(client, apiKey, appNum, logger)
	for _, doc := range docs {
		// Adding as reference entries for visibility
		bundle.References = append(bundle.References, domain.ReferenceEntry{
			PatentNumber:  domain.PatentNumber(number),
			CitationLabel: fmt.Sprintf("[%s] %s (%s)", doc.DocumentCode, doc.DocumentCodeDescriptionText, doc.OfficialDate),
		})
	}

	// Merge OA Citations (IDS/etc) into bundle
	if len(oaCitations) > 0 {
		seen := map[string]bool{}
		for _, c := range bundle.Citations {
			if c.RelationType == domain.RelationCites {
				seen[string(c.TargetPatent)] = true
			}
		}
		for _, num := range oaCitations {
			if !seen[num] && !strings.EqualFold(num, string(bundle.Patent.Number)) {
				seen[num] = true
				bundle.Citations = append(bundle.Citations, domain.CitationEdge{
					SourcePatent: bundle.Patent.Number,
					TargetPatent: domain.PatentNumber(num),
					RelationType: domain.RelationCites,
				})
			}
		}
	}

	if len(bundle.Citations) == 0 {
		if fallback, err := importGoogleCitationFallback(number, logger); err == nil {
			mergeCitationFallback(&bundle, fallback)
		} else {
			logger.Warn("uspto.google_citation_fallback failed", "patent", number, "error", err)
		}
	}

	// Calculate Expected Citations for logging (sum of API reported counts)
	// Backward = ReferenceCitedBag + ReferencedCitedBag (merged in buildUSPTOBundle)
	if bundle.ExpectedCitations == 0 {
		bundle.Patent.ExpectedCitations = len(appData.ReferenceCitedBag) + len(appData.ReferencedCitedBag)
		bundle.ExpectedCitations = bundle.Patent.ExpectedCitations
	}
	if bundle.ExpectedCitedBy == 0 {
		bundle.Patent.ExpectedCitedBy = forwardCount
		bundle.ExpectedCitedBy = bundle.Patent.ExpectedCitedBy
	}

	// Format counts for logging
	countStr := func(actual, expected int) string {
		eStr := "unkn"
		if expected >= 0 {
			eStr = fmt.Sprintf("%d", expected)
		}
		aStr := fmt.Sprintf("%d", actual)
		if actual == 0 && expected > 0 {
			aStr = "unkn"
		}
		if actual == 0 && expected <= 0 {
			aStr = "unkn"
			eStr = "unkn"
		}
		return fmt.Sprintf("%s/%s", aStr, eStr)
	}

	backActual, fwdActual := 0, 0
	for _, c := range bundle.Citations {
		if c.RelationType == domain.RelationCites {
			backActual++
		} else {
			fwdActual++
		}
	}

	logger.Info("uspto.import ok", "patent", bundle.Patent.Number, "citations", countStr(backActual, bundle.Patent.ExpectedCitations), "cited_by", countStr(fwdActual, bundle.Patent.ExpectedCitedBy), "family_edges", len(bundle.FamilyEdges), "classifications", len(bundle.Classifications))
	return bundle, nil
}

func importGoogleCitationFallback(number string, logger *slog.Logger) (domain.PatentBundle, error) {
	rawURL, err := GooglePatentsURL(number)
	if err != nil {
		return domain.PatentBundle{}, err
	}
	return ImportGooglePatents(rawURL, logger)
}

func mergeCitationFallback(bundle *domain.PatentBundle, fallback domain.PatentBundle) {
	if len(bundle.Citations) == 0 {
		bundle.Citations = make([]domain.CitationEdge, 0, len(fallback.Citations))
		for _, edge := range fallback.Citations {
			edge.SourcePatent = bundle.Patent.Number
			bundle.Citations = append(bundle.Citations, edge)
		}
	}
	if bundle.ExpectedCitations == 0 && fallback.ExpectedCitations != 0 {
		bundle.ExpectedCitations = fallback.ExpectedCitations
		bundle.Patent.ExpectedCitations = fallback.ExpectedCitations
	}
	if bundle.ExpectedCitedBy == 0 && fallback.ExpectedCitedBy != 0 {
		bundle.ExpectedCitedBy = fallback.ExpectedCitedBy
		bundle.Patent.ExpectedCitedBy = fallback.ExpectedCitedBy
	}
	if len(bundle.FamilyEdges) == 0 {
		bundle.FamilyEdges = fallback.FamilyEdges
	}
	if bundle.Patent.SourceGoogleURL == "" {
		bundle.Patent.SourceGoogleURL = fallback.Patent.SourceGoogleURL
	}
}

// --- JSON response types ---

type usptoSearchResponse struct {
	Count                    int                    `json:"count"`
	PatentFileWrapperDataBag []usptoApplicationData `json:"patentFileWrapperDataBag"`
}

type usptoApplicationData struct {
	ApplicationNumberText    string                `json:"applicationNumberText"`
	ApplicationMetaData      usptoMetaData         `json:"applicationMetaData"`
	AssignmentBag            []usptoAssignment     `json:"assignmentBag"`
	ApplicantBag             []usptoApplicant      `json:"applicantBag"`
	ReferenceCitedBag        []usptoReferenceCited `json:"referenceCitedBag"`
	ReferencedCitedBag       []usptoReferenceCited `json:"referencedCitedBag"`
	ParentContinuityBag      []usptoContinuity     `json:"parentContinuityBag"`
	ChildContinuityBag       []usptoContinuity     `json:"childContinuityBag"`
	PatentTermAdjustmentData *usptoPTA             `json:"patentTermAdjustmentData"`
}

type usptoPTA struct {
	AdjustmentTotalQuantity int `json:"adjustmentTotalQuantity"`
}

type usptoMetaData struct {
	PatentNumber                     string          `json:"patentNumber"`
	InventionTitle                   string          `json:"inventionTitle"`
	FirstApplicantName               string          `json:"firstApplicantName"`
	InventorBag                      []usptoInventor `json:"inventorBag"`
	CPCClassificationBag             []string        `json:"cpcClassificationBag"`
	GrantDate                        string          `json:"grantDate"`
	FilingDate                       string          `json:"filingDate"`
	EarliestPublicationDate          string          `json:"earliestPublicationDate"`
	ApplicationStatusDescriptionText string          `json:"applicationStatusDescriptionText"`
}

type usptoReferenceCited struct {
	PatentNumber     string `json:"patentNumber"`
	PatentNumberText string `json:"patentNumberText"`
	PatentKindCode   string `json:"patentKindCode"`
	PublicationDate  string `json:"publicationDate"`
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

type usptoApplicant struct {
	ApplicantNameText string `json:"applicantNameText"`
}

type usptoContinuity struct {
	ParentApplicationNumberText           string `json:"parentApplicationNumberText"`
	ChildApplicationNumberText            string `json:"childApplicationNumberText"`
	ParentPatentNumber                    string `json:"parentPatentNumber"`
	ChildPatentNumber                     string `json:"childPatentNumber"`
	ParentDocumentNumber                  string `json:"parentDocumentNumber"`
	ChildDocumentNumber                   string `json:"childDocumentNumber"`
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

func usptoFetchByPatentNumber(client *http.Client, apiKey, originalNumber string, logger *slog.Logger) (string, usptoApplicationData, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	rawSearchNum := extractODPNumber(originalNumber)
	paddedSearchNum := rawSearchNum
	// USPTO ODP search often expects 8 digits (zero-padded) for utility patent citations.
	if len(paddedSearchNum) == 7 {
		paddedSearchNum = "0" + paddedSearchNum
	}

	pubNum := strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(originalNumber)), "US")

	// Multi-stage search strategy to handle USPTO ODP API quirks
	// We explicitly request citation bags in all search calls.
	fields := "applicationNumberText,applicationMetaData,referenceCitedBag,referencedCitedBag,assignmentBag,applicantBag"

	// Stage 1: Simple numeric keyword search (try both formats)
	for _, sn := range []string{rawSearchNum, paddedSearchNum} {
		logger.Debug("uspto.search_stage1", "num", sn)
		if appNum, data, err := usptoExecuteSearch(client, apiKey, url.QueryEscape(sn), fields, logger); err == nil {
			return appNum, data, nil
		}
		if sn == paddedSearchNum {
			break // avoid redundant search if already identical
		}
	}

	// Stage 2: Prefixed metadata search
	for _, sn := range []string{rawSearchNum, paddedSearchNum} {
		q2 := fmt.Sprintf("applicationMetaData.patentNumber:%s OR applicationMetaData.earliestPublicationNumber:*%s* OR applicationMetaData.publicationSequenceNumberBag:*%s*",
			sn, sn, pubNum)
		logger.Debug("uspto.search_stage2", "query", q2)
		if appNum, data, err := usptoExecuteSearch(client, apiKey, url.QueryEscape(q2), fields, logger); err == nil {
			return appNum, data, nil
		}
		if sn == paddedSearchNum {
			break
		}
	}

	// Stage 3: Non-prefixed raw field search
	for _, sn := range []string{rawSearchNum, paddedSearchNum} {
		q3 := fmt.Sprintf("patentNumber:%s OR earliestPublicationNumber:%s OR publicationSequenceNumberBag:%s",
			sn, originalNumber, pubNum)
		logger.Debug("uspto.search_stage3", "query", q3)
		if appNum, data, err := usptoExecuteSearch(client, apiKey, url.QueryEscape(q3), fields, logger); err == nil {
			return appNum, data, nil
		}
		if sn == paddedSearchNum {
			break
		}
	}

	return "", usptoApplicationData{}, fmt.Errorf("patent not found in USPTO ODP: %s (Tried numeric keyword, prefixed fields, and raw fields)", originalNumber)
}

func usptoExecuteSearch(client *http.Client, apiKey, encodedQuery, fields string, logger *slog.Logger) (string, usptoApplicationData, error) {
	searchURL := fmt.Sprintf("%s/api/v1/patent/applications/search?q=%s", usptoBaseURL, encodedQuery)
	if fields != "" {
		searchURL += "&fields=" + fields
	}
	var resp usptoSearchResponse
	if err := usptoGET(client, apiKey, searchURL, &resp, logger); err != nil {
		if errors.Is(err, errUSPTONotFound) {
			return "", usptoApplicationData{}, fmt.Errorf("no results (404)")
		}
		return "", usptoApplicationData{}, err
	}
	if resp.Count == 0 || len(resp.PatentFileWrapperDataBag) == 0 {
		return "", usptoApplicationData{}, fmt.Errorf("no results")
	}
	return resp.PatentFileWrapperDataBag[0].ApplicationNumberText, resp.PatentFileWrapperDataBag[0], nil
}

func usptoFetchContinuity(client *http.Client, apiKey, appNum string, logger *slog.Logger) (usptoContinuityResponse, error) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	url := fmt.Sprintf("%s/api/v1/patent/applications/%s/continuity", usptoBaseURL, appNum)
	logger.Debug("uspto.continuity", "app_num", appNum, "url", url)
	var resp usptoContinuityResponse
	err := usptoGET(client, apiKey, url, &resp, logger)
	if err != nil {
		if errors.Is(err, errUSPTONotFound) {
			logger.Debug("uspto.continuity none", "app_num", appNum)
			return resp, nil
		}
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

func usptoFetchMetadata(client *http.Client, apiKey, appNum string, logger *slog.Logger) (usptoApplicationData, error) {
	fields := "applicationNumberText,applicationMetaData,referenceCitedBag,referencedCitedBag,assignmentBag,applicantBag"
	url := fmt.Sprintf("%s/api/v1/patent/applications/%s/meta-data?fields=%s", usptoBaseURL, appNum, fields)
	var resp usptoSearchResponse
	if err := usptoGET(client, apiKey, url, &resp, logger); err != nil {
		if errors.Is(err, errUSPTONotFound) {
			return usptoApplicationData{}, nil
		}
		return usptoApplicationData{}, err
	}
	if len(resp.PatentFileWrapperDataBag) > 0 {
		return resp.PatentFileWrapperDataBag[0], nil
	}
	return usptoApplicationData{}, nil
}

func usptoFetchAssignments(client *http.Client, apiKey, appNum string, logger *slog.Logger) ([]usptoAssignment, error) {
	url := fmt.Sprintf("%s/api/v1/patent/applications/%s/assignment", usptoBaseURL, appNum)
	var resp usptoSearchResponse
	if err := usptoGET(client, apiKey, url, &resp, logger); err != nil {
		if errors.Is(err, errUSPTONotFound) {
			return nil, nil
		}
		return nil, err
	}
	if len(resp.PatentFileWrapperDataBag) > 0 {
		return resp.PatentFileWrapperDataBag[0].AssignmentBag, nil
	}
	return nil, nil
}

func usptoFetchAdjustments(client *http.Client, apiKey, appNum string, logger *slog.Logger) (*usptoPTA, error) {
	url := fmt.Sprintf("%s/api/v1/patent/applications/%s/adjustment", usptoBaseURL, appNum)
	var resp usptoSearchResponse
	if err := usptoGET(client, apiKey, url, &resp, logger); err != nil {
		if errors.Is(err, errUSPTONotFound) {
			return nil, nil
		}
		return nil, err
	}
	if len(resp.PatentFileWrapperDataBag) > 0 {
		return resp.PatentFileWrapperDataBag[0].PatentTermAdjustmentData, nil
	}
	return nil, nil
}

type usptoDocumentsResponse struct {
	Count       int             `json:"count"`
	DocumentBag []usptoDocument `json:"documentBag"`
}

type usptoDocument struct {
	ApplicationNumberText       string `json:"applicationNumberText"`
	OfficialDate                string `json:"officialDate"`
	DocumentIdentifier          string `json:"documentIdentifier"`
	DocumentCode                string `json:"documentCode"`
	DocumentCodeDescriptionText string `json:"documentCodeDescriptionText"`
	DirectionCategory           string `json:"directionCategory"`
}

func usptoFetchDocuments(client *http.Client, apiKey, appNum string, logger *slog.Logger) ([]usptoDocument, error) {
	url := fmt.Sprintf("%s/api/v1/patent/applications/%s/documents", usptoBaseURL, appNum)
	var resp usptoDocumentsResponse
	if err := usptoGET(client, apiKey, url, &resp, logger); err != nil {
		if errors.Is(err, errUSPTONotFound) {
			return nil, nil
		}
		return nil, err
	}
	return resp.DocumentBag, nil
}

type usptoOACitationResponse struct {
	Count   int `json:"count"`
	Records []struct {
		ReferenceIdentifier                      string `json:"referenceIdentifier"`
		Patent_PGPub                             string `json:"Patent_PGPub"`
		ApplicantCitedExaminerReferenceIndicator bool   `json:"applicantCitedExaminerReferenceIndicator"`
	} `json:"records"`
}

func usptoFetchOfficeActionCitations(client *http.Client, apiKey, appNum string, logger *slog.Logger) ([]string, error) {
	// Research suggests this endpoint for structured IDS/OA citations.
	// Note: Using POST as recommended by USPTO documentation for search-like behavior on records.
	url := "https://api.uspto.gov/api/v1/patent/oa/oa_citations/v2/records"

	payload := fmt.Sprintf(`{"searchText": "patentApplicationNumber:%s", "rows": 500}`, appNum)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-KEY", apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("OA Citations API returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if os.Getenv(logging.EnvDebug) == "1" {
		fmt.Fprintf(os.Stderr, "<<< USPTO OA RESP (%d): %s\n\n", resp.StatusCode, string(body))
		logger.Debug("uspto.oa_response", "status", resp.StatusCode, "body", string(body))
	}

	var oaResp usptoOACitationResponse
	if err := json.Unmarshal(body, &oaResp); err != nil {
		return nil, err
	}

	var nums []string
	seen := map[string]bool{}
	for _, rec := range oaResp.Records {
		raw := rec.ReferenceIdentifier
		if raw == "" {
			raw = rec.Patent_PGPub
		}
		if num := usptoNormalizePatentNumber(raw); num != "" && !seen[num] {
			seen[num] = true
			nums = append(nums, num)
		}
	}
	return nums, nil
}

// usptoFetchForwardCitations searches for all patents that cite patentNum using
// the applicationMetaData.referenceCitedBag.patentNumber query field. Results are paginated (25/page).
func usptoFetchForwardCitations(client *http.Client, apiKey, originalNumber, searchNum string, logger *slog.Logger) ([]string, int) {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	const pageSize = 25
	const maxPages = 40 // cap at 1000 forward citations
	var nums []string
	seen := map[string]bool{}
	totalCount := -1

	// Broad forward citation query: check multiple identifier formats and field names.
	// We check both prefixed and non-prefixed fields, and both 'reference' and 'referenced' spellings.
	// We also include 'forwardReferencedPatentNumber' which is sometimes used for this purpose.
	query := fmt.Sprintf("applicationMetaData.referenceCitedBag.patentNumber:%s OR referenceCitedBag.patentNumber:%s OR applicationMetaData.referencedCitedBag.patentNumber:%s OR referencedCitedBag.patentNumber:%s OR forwardReferencedPatentNumber:%s",
		searchNum, searchNum, searchNum, searchNum, searchNum)

	logger.Info("uspto.fwd_citations", "num", originalNumber, "query", query)
	for page := 0; page < maxPages; page++ {
		url := fmt.Sprintf("%s/api/v1/patent/applications/search?q=%s&fields=applicationNumberText,applicationMetaData.patentNumber&rows=%d&start=%d",
			usptoBaseURL, url.QueryEscape(query), pageSize, page*pageSize)
		var resp usptoSearchResponse
		if err := usptoGET(client, apiKey, url, &resp, logger); err != nil {
			if errors.Is(err, errUSPTONotFound) {
				if page == 0 {
					totalCount = 0
				}
				break // No (more) forward citations
			}
			logger.Warn("uspto.fwd_citations page failed", "num", originalNumber, "page", page, "error", err)
			break
		}
		if page == 0 {
			totalCount = resp.Count
		}
		count := len(resp.PatentFileWrapperDataBag)
		logger.Debug("uspto.fwd_citations page", "num", originalNumber, "page", page, "count", count)
		for _, app := range resp.PatentFileWrapperDataBag {
			// Try patentNumber first, then fallback to applicationNumberText
			num := app.ApplicationMetaData.PatentNumber
			if num == "" {
				num = app.ApplicationNumberText
			}
			if num := usptoNormalizePatentNumber(num); num != "" && !seen[num] {
				seen[num] = true
				nums = append(nums, num)
			}
		}
		if count < pageSize || resp.Count <= (page+1)*pageSize {
			break
		}
	}
	logger.Info("uspto.fwd_citations done", "num", originalNumber, "total", len(nums))
	return nums, totalCount
}

func usptoGET(client *http.Client, apiKey, url string, dest any, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}

	const maxRetries = 3
	var body []byte
	var lastStatus int

	for i := 0; i <= maxRetries; i++ {
		if i > 0 {
			logger.Warn("uspto.retry", "url", url, "attempt", i, "status", lastStatus)
			if os.Getenv(logging.EnvDebug) == "1" {
				fmt.Fprintf(os.Stderr, "[USPTO] Rate limited (HTTP %d), retrying in 2s (Attempt %d/%d)...\n", lastStatus, i, maxRetries)
			}
			time.Sleep(2 * time.Second)
		}

		if os.Getenv(logging.EnvDebug) == "1" {
			fmt.Fprintf(os.Stderr, ">>> USPTO REQ: %s\n", url)
		}

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
		lastStatus = resp.StatusCode

		var readErr error
		body, readErr = io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("failed to read response body: %w", readErr)
		}

		if os.Getenv(logging.EnvDebug) == "1" {
			fmt.Fprintf(os.Stderr, "<<< USPTO RESP (%d): %s\n\n", resp.StatusCode, string(body))
			logger.Debug("uspto.response", "status", resp.StatusCode, "body", string(body))
		}

		if resp.StatusCode == 429 {
			continue // Retry on rate limit
		}

		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return fmt.Errorf("USPTO API key invalid or unauthorized (HTTP %d). Body: %s", resp.StatusCode, string(body))
		}
		if resp.StatusCode == 404 {
			return errUSPTONotFound
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return fmt.Errorf("USPTO API returned HTTP %d for %s. Body: %s", resp.StatusCode, url, string(body))
		}

		if err := json.Unmarshal(body, dest); err != nil {
			return fmt.Errorf("failed to unmarshal JSON: %w. Body: %s", err, string(body))
		}
		return nil
	}

	return fmt.Errorf("USPTO API rate limited after %d retries", maxRetries)
}

// --- Bundle builder ---

func buildUSPTOBundle(originalNumber string, data usptoApplicationData, continuity usptoContinuityResponse, forwardNums []string, logger *slog.Logger) domain.PatentBundle {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	meta := data.ApplicationMetaData

	// Canonical number: use ODP patentNumber with US prefix, else fall back to original
	number := strings.ToUpper(strings.TrimSpace(meta.PatentNumber))
	if number == "" {
		number = strings.ToUpper(strings.TrimSpace(originalNumber))
	} else if !strings.HasPrefix(number, "US") {
		number = "US" + number
	}

	// First assignee from most recent assignment
	assignee := strings.TrimSpace(meta.FirstApplicantName)
	if assignee == "" {
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
	}
	if assignee == "" {
		for _, a := range data.ApplicantBag {
			if a.ApplicantNameText != "" {
				assignee = a.ApplicantNameText
				break
			}
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
		Number:            domain.PatentNumber(number),
		Title:             toTitleCase(meta.InventionTitle),
		Assignee:          assignee,
		Inventors:         inventors,
		PublicationDate:   meta.EarliestPublicationDate,
		GrantDate:         meta.GrantDate,
		SourceGoogleURL:   sourceURL,
		ApplicationNumber: data.ApplicationNumberText,
		ApplicationDate:   meta.FilingDate,
		PublicationNumber: meta.EarliestPublicationDate, // Often same field in metadata index
		GrantNumber:       meta.PatentNumber,
	}
	ptaDays := 0
	if data.PatentTermAdjustmentData != nil {
		ptaDays = data.PatentTermAdjustmentData.AdjustmentTotalQuantity
	}

	if exp := estimatedExpirationDate(meta.EarliestPublicationDate, meta.GrantDate); exp != "" {
		patent.ExpirationDate = exp
		patent.ExpirationSource = domain.ExpirationSourceEstimated
		// Refine with Patent Term Adjustment (PTA) if available
		if ptaDays > 0 {
			if t, err := time.Parse("2006-01-02", exp); err == nil {
				refined := t.AddDate(0, 0, ptaDays)
				patent.ExpirationDate = refined.Format("2006-01-02")
				patent.ExpirationSource = domain.ExpirationSourceImported // More precise with PTA
			}
		}
	}
	if patent.Title == "" {
		patent.Title = number
	}

	bundle := domain.PatentBundle{Patent: patent}

	for _, code := range meta.CPCClassificationBag {
		cls := domain.ParseClassification(code)
		cls.PatentNumber = domain.PatentNumber(number)
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
		if strings.EqualFold(string(e.ParentNumber), string(e.ChildNumber)) {
			return
		}
		key := string(e.ParentNumber) + "\x00" + string(e.ChildNumber)
		if !edgeSeen[key] {
			edgeSeen[key] = true
			bundle.FamilyEdges = append(bundle.FamilyEdges, e)
		}
	}
	for _, p := range parentBag {
		pNum := usptoNormalizePatentNumber(firstNonEmpty(p.ParentPatentNumber, p.ParentDocumentNumber, p.ParentApplicationNumberText))
		cNum := usptoNormalizePatentNumber(firstNonEmpty(p.ChildPatentNumber, p.ChildDocumentNumber, p.ChildApplicationNumberText))
		// If child number in record is missing or looks like an application but we are importing the issued patent,
		// we should still link to the issued patent 'number' if the record is a parent of our current context.
		if cNum == "" {
			cNum = number
		}
		if pNum != "" {
			addEdge(domain.FamilyEdge{
				ParentNumber: domain.PatentNumber(pNum),
				ChildNumber:  domain.PatentNumber(cNum),
				RelationType: mapContinuityType(p.ClaimParentageTypeCode),
			})
		}
	}
	for _, c := range childBag {
		pNum := usptoNormalizePatentNumber(firstNonEmpty(c.ParentPatentNumber, c.ParentDocumentNumber, c.ParentApplicationNumberText))
		cNum := usptoNormalizePatentNumber(firstNonEmpty(c.ChildPatentNumber, c.ChildDocumentNumber, c.ChildApplicationNumberText))
		if pNum == "" {
			pNum = number
		}
		if cNum != "" {
			addEdge(domain.FamilyEdge{
				ParentNumber: domain.PatentNumber(pNum),
				ChildNumber:  domain.PatentNumber(cNum),
				RelationType: mapContinuityType(c.ClaimParentageTypeCode),
			})
		}
	}

	// Backward citations: references cited by this patent
	citeSeen := map[string]bool{}
	processRefs := func(bag []usptoReferenceCited) {
		for _, ref := range bag {
			rawNum := ref.PatentNumber
			if rawNum == "" {
				rawNum = ref.PatentNumberText
			}
			num := usptoNormalizePatentNumber(rawNum)
			if num == "" || strings.EqualFold(num, number) || citeSeen[num] {
				continue
			}
			citeSeen[num] = true
			bundle.Citations = append(bundle.Citations, domain.CitationEdge{
				SourcePatent: domain.PatentNumber(number),
				TargetPatent: domain.PatentNumber(num),
				RelationType: domain.RelationCites,
			})
		}
	}
	processRefs(data.ReferenceCitedBag)
	processRefs(data.ReferencedCitedBag)

	// Forward citations: patents that cite this patent
	for _, num := range forwardNums {
		if strings.EqualFold(num, number) {
			continue
		}
		bundle.Citations = append(bundle.Citations, domain.CitationEdge{
			SourcePatent: domain.PatentNumber(number),
			TargetPatent: domain.PatentNumber(num),
			RelationType: domain.RelationCitedBy,
		})
	}

	bundle.References = append(bundle.References, domain.ReferenceEntry{
		PatentNumber:  domain.PatentNumber(number),
		CitationLabel: fmt.Sprintf("%s, %s", number, patent.Title),
	})

	logger.Debug("uspto.build_bundle", "number", number, "citations", len(bundle.Citations), "family_edges", len(bundle.FamilyEdges), "classifications", len(bundle.Classifications), "sections", len(bundle.Sections), "pta_days", ptaDays)
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

func toTitleCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Small words that should stay lowercase unless they are the first or last word.
	// User requested "except adverbs" - adverbs like "highly", "easily", "quickly" usually remain capitalized.
	// Standard title case exceptions: articles, conjunctions, short prepositions.
	exceptions := map[string]bool{
		"a": true, "an": true, "the": true,
		"and": true, "but": true, "or": true, "nor": true, "for": true, "yet": true, "so": true,
		"at": true, "by": true, "in": true, "of": true, "on": true, "to": true, "up": true, "as": true,
		"with": true, "from": true, "into": true, "upon": true,
	}

	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if i > 0 && i < len(words)-1 && exceptions[w] {
			continue
		}
		// Capitalize first letter
		r := []rune(w)
		if len(r) > 0 {
			r[0] = unicode.ToUpper(r[0])
			words[i] = string(r)
		}
	}
	return strings.Join(words, " ")
}
