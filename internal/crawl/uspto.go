package crawl

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/uspto"
)

// USPTOMinInterval keeps requests to the USPTO ODP API polite.
var USPTOMinInterval = 100 * time.Millisecond

// UpdateUSPTOMinInterval updates the USPTO request interval.
func UpdateUSPTOMinInterval(d time.Duration) {
	USPTOMinInterval = d
}

// NewUSPTOSource builds a Source backed by the USPTO Patent File Wrapper API.
func NewUSPTOSource(apiKey string) Source {
	client := &http.Client{Timeout: httpTimeout}
	return &httpSource{
		name:    domain.SourceUSPTO,
		client:  client,
		limiter: newLimiter(USPTOMinInterval),
		urlFor: func(n domain.PatentNumber) string {
			return "https://api.uspto.gov/api/v1/patent/applications/search?q=" + url.QueryEscape(usptoQuery(n))
		},
		headers: func() http.Header {
			h := make(http.Header)
			if strings.TrimSpace(apiKey) != "" {
				h.Set("x-api-key", apiKey)
			}
			h.Set("Accept", "application/json")
			return h
		},
		parse: makeParseUSPTO(apiKey, client),
	}
}

func usptoQuery(n domain.PatentNumber) string {
	serial := strings.TrimSpace(n.Serial)
	if serial == "" {
		return ""
	}
	norm := n.Normalized()
	if n.Kind != "" {
		// Serial is a grant/publication number, not an application number.
		// Excluding applicationNumberText avoids false positives where an
		// unrelated application happens to share the same serial digits.
		if norm != "" && norm != serial {
			return fmt.Sprintf("patentNumberText:%s OR publicationNumberText:%s OR publicationNumber:%s OR %q OR %q",
				serial, serial, serial, norm, serial)
		}
		return fmt.Sprintf("patentNumberText:%s OR publicationNumberText:%s OR publicationNumber:%s OR %q",
			serial, serial, serial, serial)
	}
	if norm != "" && norm != serial {
		return fmt.Sprintf("applicationNumberText:%s OR patentNumberText:%s OR publicationNumberText:%s OR publicationNumber:%s OR %q OR %q",
			serial, serial, serial, serial, norm, serial)
	}
	return fmt.Sprintf("applicationNumberText:%s OR patentNumberText:%s OR publicationNumberText:%s OR publicationNumber:%s OR %q",
		serial, serial, serial, serial, serial)
}

type usptoFileWrapperResponse struct {
	Count                    int                `json:"count"`
	RequestIdentifier        string             `json:"requestIdentifier"`
	PatentFileWrapperDataBag []usptoWrapperData `json:"patentFileWrapperDataBag"`
}

type usptoDocumentMeta struct {
	ProductIdentifier  string `json:"productIdentifier"`
	ZipFileName        string `json:"zipFileName"`
	FileCreateDateTime string `json:"fileCreateDateTime"`
	XMLFileName        string `json:"xmlFileName"`
	FileLocationURI    string `json:"fileLocationURI"`
}

type usptoWrapperData struct {
	ApplicationNumberText string               `json:"applicationNumberText"`
	ApplicationMetaData   usptoApplicationMeta `json:"applicationMetaData"`
	CPCClassificationBag  []string             `json:"cpcClassificationBag"`
	EventDataBag          []usptoEventData     `json:"eventDataBag"`
	ParentContinuityBag   []usptoContinuity    `json:"parentContinuityBag"`
	ForeignPriorityBag    []usptoForeign       `json:"foreignPriorityBag"`
	RecordAttorney        usptoRecordAttorney  `json:"recordAttorney"`
	LastIngestionDateTime string               `json:"lastIngestionDateTime"`
	GrantDocumentMetaData *usptoDocumentMeta   `json:"grantDocumentMetaData"`
	PGPubDocumentMetaData *usptoDocumentMeta   `json:"pgpubDocumentMetaData"`
}

type usptoApplicationMeta struct {
	InventionTitle                string           `json:"inventionTitle"`
	FilingDate                    string           `json:"filingDate"`
	EffectiveFilingDate           string           `json:"effectiveFilingDate"`
	ApplicationStatusCode         any              `json:"applicationStatusCode"`
	ApplicationStatusDescription  string           `json:"applicationStatusDescriptionText"`
	ApplicationStatusDate         string           `json:"applicationStatusDate"`
	ApplicationTypeCode           string           `json:"applicationTypeCode"`
	ApplicationTypeLabelName      string           `json:"applicationTypeLabelName"`
	ApplicationTypeCategory       string           `json:"applicationTypeCategory"`
	FirstInventorToFileIndicator  string           `json:"firstInventorToFileIndicator"`
	NationalStageIndicator        bool             `json:"nationalStageIndicator"`
	FirstInventorName             string           `json:"firstInventorName"`
	FirstApplicantName            string           `json:"firstApplicantName"`
	CustomerNumber                any              `json:"customerNumber"`
	GroupArtUnitNumber            string           `json:"groupArtUnitNumber"`
	ExaminerNameText              string           `json:"examinerNameText"`
	DocketNumber                  string           `json:"docketNumber"`
	ApplicationConfirmationNumber any              `json:"applicationConfirmationNumber"`
	USPCSymbolText                string           `json:"uspcSymbolText"`
	Class                         string           `json:"class"`
	Subclass                      string           `json:"subclass"`
	CPCClassificationBag          []string         `json:"cpcClassificationBag"`
	EntityStatusData              usptoEntity      `json:"entityStatusData"`
	PublicationCategoryBag        json.RawMessage  `json:"publicationCategoryBag"`
	InventorBag                   []usptoInventor  `json:"inventorBag"`
	ApplicantBag                  []usptoApplicant `json:"applicantBag"`
	PatentNumberText              string           `json:"patentNumberText"`
	PatentNumber                  string           `json:"patentNumber"`
	PublicationNumber             string           `json:"publicationNumber"`
	EarliestPublicationNumber     string           `json:"earliestPublicationNumber"`
}

type usptoEntity struct {
	SmallEntityStatusIndicator   bool   `json:"smallEntityStatusIndicator"`
	BusinessEntityStatusCategory string `json:"businessEntityStatusCategory"`
}

type usptoInventor struct {
	FirstName                string          `json:"firstName"`
	MiddleName               string          `json:"middleName"`
	LastName                 string          `json:"lastName"`
	NameSuffix               string          `json:"nameSuffix"`
	InventorNameText         string          `json:"inventorNameText"`
	CorrespondenceAddressBag json.RawMessage `json:"correspondenceAddressBag"`
}

type usptoApplicant struct {
	ApplicantNameText        string          `json:"applicantNameText"`
	CorrespondenceAddressBag json.RawMessage `json:"correspondenceAddressBag"`
}

type usptoEventData struct {
	EventCode            string `json:"eventCode"`
	EventDescriptionText string `json:"eventDescriptionText"`
	EventDate            string `json:"eventDate"`
}

type usptoContinuity struct {
	ParentApplicationNumberText        string `json:"parentApplicationNumberText"`
	ChildApplicationNumberText         string `json:"childApplicationNumberText"`
	ParentApplicationFilingDate        string `json:"parentApplicationFilingDate"`
	ParentApplicationStatusCode        any    `json:"parentApplicationStatusCode"`
	ParentApplicationStatusDescription string `json:"parentApplicationStatusDescriptionText"`
	ClaimParentageTypeCode             string `json:"claimParentageTypeCode"`
	ClaimParentageTypeDescriptionText  string `json:"claimParentageTypeCodeDescriptionText"`
}

type usptoForeign struct {
	ApplicationNumberText string `json:"applicationNumberText"`
	FilingDate            string `json:"filingDate"`
	IPOfficeName          string `json:"ipOfficeName"`
}

type usptoRecordAttorney struct {
	AttorneyBag []usptoAttorney `json:"attorneyBag"`
}

type usptoAttorney struct {
	ActiveIndicator                string          `json:"activeIndicator"`
	FirstName                      string          `json:"firstName"`
	MiddleName                     string          `json:"middleName"`
	LastName                       string          `json:"lastName"`
	RegistrationNumber             string          `json:"registrationNumber"`
	RegisteredPractitionerCategory string          `json:"registeredPractitionerCategory"`
	AttorneyAddressBag             json.RawMessage `json:"attorneyAddressBag"`
	TelecommunicationAddressBag    json.RawMessage `json:"telecommunicationAddressBag"`
}

func makeParseUSPTO(apiKey string, client *http.Client) parseFunc {
	return func(ctx context.Context, number domain.PatentNumber, body []byte) (Result, error) {
		var resp usptoFileWrapperResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return Result{}, fmt.Errorf("crawl/uspto: decode response: %w", err)
		}
		if len(resp.PatentFileWrapperDataBag) == 0 {
			return Result{}, ErrUSPTOApplicationNotFound
		}

		w, ok := matchingUSPTOWrapper(number, resp.PatentFileWrapperDataBag)
		if !ok {
			return Result{}, ErrUSPTOApplicationNotFound
		}
		appNumber := strings.TrimSpace(w.ApplicationNumberText)
		if appNumber == "" {
			return Result{}, ErrUSPTOApplicationNotFound
		}

		recordNumber, err := domain.ParsePatentNumber("US" + appNumber)
		if err != nil {
			recordNumber = number
		}
		filingDate := parseISODate(w.ApplicationMetaData.FilingDate)
		now := time.Now().UTC()
		nowText := encodeRFC3339(now)

		inventors := usptoInventors(w.ApplicationMetaData)
		assignees := usptoApplicants(w.ApplicationMetaData)
		patent := domain.Patent{
			Number:          recordNumber,
			DisplayNumber:   recordNumber,
			Title:           strings.TrimSpace(w.ApplicationMetaData.InventionTitle),
			Assignee:        strings.Join(assignees, "; "),
			Inventors:       inventors,
			Classifications: usptoClassifications(w.ApplicationMetaData, w.CPCClassificationBag),
			FetchState:      domain.FetchCached,
			Source:          domain.SourceUSPTO,
			FetchedAt:       now,
			ApplicationDate: filingDate,
			SourceURL:       googlePatentURL(recordNumber),
		}

		res := Result{
			Patent: patent,
			Documents: []domain.Document{{
				Number: recordNumber,
				Stage:  domain.StageApplication,
				Dated:  filingDate,
			}},
			AuthorityIdentifiers: []domain.AuthorityIdentifier{{
				Authority:      "US",
				IdentifierType: "application",
				Identifier:     appNumber,
				RawIdentifier:  w.ApplicationNumberText,
				RecordNumber:   recordNumber,
				DocumentNumber: recordNumber.Normalized(),
				Country:        "US",
				Dated:          w.ApplicationMetaData.FilingDate,
				Source:         string(domain.SourceUSPTO),
				Confidence:     100,
			}},
		}

		extraDocs, extraIds := extractAdditionalUSPTODocuments(recordNumber, w)
		res.Documents = append(res.Documents, extraDocs...)
		res.AuthorityIdentifiers = append(res.AuthorityIdentifiers, extraIds...)

		res.USPTOApplication = &domain.USPTOApplication{
			ApplicationNumber:             appNumber,
			RecordNumber:                  recordNumber,
			InventionTitle:                patent.Title,
			FilingDate:                    w.ApplicationMetaData.FilingDate,
			EffectiveFilingDate:           w.ApplicationMetaData.EffectiveFilingDate,
			ApplicationStatusCode:         stringify(w.ApplicationMetaData.ApplicationStatusCode),
			ApplicationStatusText:         strings.TrimSpace(w.ApplicationMetaData.ApplicationStatusDescription),
			ApplicationStatusDate:         w.ApplicationMetaData.ApplicationStatusDate,
			ApplicationTypeCode:           w.ApplicationMetaData.ApplicationTypeCode,
			ApplicationTypeLabel:          w.ApplicationMetaData.ApplicationTypeLabelName,
			ApplicationTypeCategory:       w.ApplicationMetaData.ApplicationTypeCategory,
			FirstInventorToFile:           strings.EqualFold(w.ApplicationMetaData.FirstInventorToFileIndicator, "Y"),
			NationalStage:                 w.ApplicationMetaData.NationalStageIndicator,
			FirstInventorName:             w.ApplicationMetaData.FirstInventorName,
			FirstApplicantName:            w.ApplicationMetaData.FirstApplicantName,
			CustomerNumber:                stringify(w.ApplicationMetaData.CustomerNumber),
			GroupArtUnitNumber:            w.ApplicationMetaData.GroupArtUnitNumber,
			ExaminerName:                  w.ApplicationMetaData.ExaminerNameText,
			DocketNumber:                  w.ApplicationMetaData.DocketNumber,
			ApplicationConfirmationNumber: stringify(w.ApplicationMetaData.ApplicationConfirmationNumber),
			USPCSymbolText:                w.ApplicationMetaData.USPCSymbolText,
			USPCClass:                     w.ApplicationMetaData.Class,
			USPCSubclass:                  w.ApplicationMetaData.Subclass,
			SmallEntityStatus:             w.ApplicationMetaData.EntityStatusData.SmallEntityStatusIndicator,
			BusinessEntityStatus:          w.ApplicationMetaData.EntityStatusData.BusinessEntityStatusCategory,
			PublicationCategoryJSON:       rawJSONOrDefault(w.ApplicationMetaData.PublicationCategoryBag, "[]"),
			LastIngestionDateTime:         w.LastIngestionDateTime,
			FetchedAt:                     nowText,
			PGPubXMLURL: func() string {
				if w.PGPubDocumentMetaData != nil {
					return w.PGPubDocumentMetaData.FileLocationURI
				}
				return ""
			}(),
			PGPubXMLName: func() string {
				if w.PGPubDocumentMetaData != nil {
					return w.PGPubDocumentMetaData.XMLFileName
				}
				return ""
			}(),
			PatentGrantXMLURL: func() string {
				if w.GrantDocumentMetaData != nil {
					return w.GrantDocumentMetaData.FileLocationURI
				}
				return ""
			}(),
			PatentGrantXMLName: func() string {
				if w.GrantDocumentMetaData != nil {
					return w.GrantDocumentMetaData.XMLFileName
				}
				return ""
			}(),
		}
		res.USPTOParties = usptoParties(appNumber, w)
		res.USPTOEvents = usptoEvents(appNumber, w.EventDataBag)
		res.USPTOContinuities = usptoContinuities(appNumber, recordNumber, w.ParentContinuityBag)
		res.USPTOForeignPriority = usptoForeignPriorities(appNumber, w.ForeignPriorityBag)
		res.SourceSnapshots = []domain.SourceSnapshot{{
			ID:             snapshotID(string(domain.SourceUSPTO), appNumber, body),
			PatentNumber:   recordNumber,
			Source:         "uspto_file_wrapper",
			SourceRecordID: appNumber,
			SourceURL:      patent.SourceURL,
			FetchedAt:      nowText,
			PayloadKind:    "json",
			PayloadHash:    payloadHash(body),
			ResponseBytes:  int64(len(body)),
			HTTPStatus:     http.StatusOK,
			SummaryJSON:    `{"parser":"file_wrapper"}`,
		}}
		res.AuthorityIdentifiers = append(res.AuthorityIdentifiers, identifiersFromUSPTO(recordNumber, appNumber, w)...)
		res.Relations = append(res.Relations, relationsFromUSPTOContinuity(recordNumber, w.ParentContinuityBag)...)

		// Automatically fetch and parse citations from USPTO XML if available!
		xmlURL := ""
		if w.GrantDocumentMetaData != nil && w.GrantDocumentMetaData.FileLocationURI != "" {
			xmlURL = w.GrantDocumentMetaData.FileLocationURI
		} else if w.PGPubDocumentMetaData != nil && w.PGPubDocumentMetaData.FileLocationURI != "" {
			xmlURL = w.PGPubDocumentMetaData.FileLocationURI
		}

		if xmlURL != "" {
			req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, xmlURL, nil)
			if rerr == nil {
				if strings.TrimSpace(apiKey) != "" {
					req.Header.Set("x-api-key", apiKey)
				}
				req.Header.Set("Accept", "*/*")
				resp, derr := client.Do(req)
				if derr == nil {
					defer resp.Body.Close()
					if resp.StatusCode >= 200 && resp.StatusCode < 300 {
						xmlData, rerr := io.ReadAll(resp.Body)
						if rerr == nil {
							var rawCites []domain.USPTOGrantCitation
							if strings.HasSuffix(strings.ToLower(xmlURL), ".zip") {
								rawCites, _ = extractAndParseCitationsFromZip(xmlData)
							} else {
								parsed, _ := uspto.ParseCitations(bytes.NewReader(xmlData))
								rawCites = parsed.Citations
							}
							for _, c := range rawCites {
								cited, ok := patentNumberFromUSPTOCitation(c)
								if ok && !cited.IsZero() && cited.Normalized() != recordNumber.Normalized() {
									res.Relations = append(res.Relations, domain.Relation{
										From:   recordNumber,
										To:     cited,
										Kind:   domain.RelationCites,
										Source: domain.SourceUSPTO,
									})
								}
							}
						}
					}
				}
			}
		}

		return res, nil
	}
}

func patentNumberFromUSPTOCitation(c domain.USPTOGrantCitation) (domain.PatentNumber, bool) {
	if c.CitationType != "" && !strings.EqualFold(c.CitationType, "patent") {
		return domain.PatentNumber{}, false
	}
	doc := strings.TrimSpace(c.CitedDocNumber)
	if doc == "" {
		return domain.PatentNumber{}, false
	}
	country := strings.ToUpper(strings.TrimSpace(c.CitedCountry))
	kind := strings.ToUpper(strings.TrimSpace(c.CitedKind))

	for _, raw := range []string{
		country + doc + kind,
		country + doc,
		doc + kind,
		doc,
	} {
		n, err := domain.ParsePatentNumber(raw)
		if err != nil {
			continue
		}
		if n.Country == "" {
			n.Country = country
		}
		if n.Kind == "" {
			n.Kind = kind
		}
		return n, true
	}
	return domain.PatentNumber{}, false
}

func extractAndParseCitationsFromZip(zipBytes []byte) ([]domain.USPTOGrantCitation, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".xml") {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			res, err := uspto.ParseCitations(rc)
			if err != nil {
				return nil, err
			}
			return res.Citations, nil
		}
	}
	return nil, fmt.Errorf("zip contained no xml file")
}

func matchingUSPTOWrapper(number domain.PatentNumber, bags []usptoWrapperData) (usptoWrapperData, bool) {
	serial := strings.TrimLeft(number.Serial, "0")
	if serial == "" {
		serial = number.Serial
	}

	var bestW usptoWrapperData
	bestScore := -1

	stage := domain.GuessStage(number)

	for _, w := range bags {
		score := 0
		matchesApp := sameDigits(w.ApplicationNumberText, serial)
		matchesGrant := sameDigits(w.ApplicationMetaData.PatentNumber, serial) || sameDigits(w.ApplicationMetaData.PatentNumberText, serial)
		matchesPub := sameDigits(w.ApplicationMetaData.PublicationNumber, serial) ||
			sameDigits(w.ApplicationMetaData.EarliestPublicationNumber, serial)

		if !matchesApp && !matchesGrant && !matchesPub {
			continue
		}

		if stage == domain.StageGrant {
			if matchesGrant {
				score = 10
			} else if matchesPub {
				score = 5
			} else if matchesApp {
				score = 1
			}
		} else if stage == domain.StagePublication {
			if matchesPub {
				score = 10
			} else if matchesApp {
				score = 5
			} else if matchesGrant {
				score = 1
			}
		} else {
			if matchesApp {
				score = 10
			} else if matchesGrant {
				score = 5
			} else if matchesPub {
				score = 5
			}
		}

		if score > bestScore {
			bestScore = score
			bestW = w
		}
	}

	if bestScore >= 0 {
		return bestW, true
	}
	return usptoWrapperData{}, false
}

// SearchUSPTO queries the USPTO ODP API using a broad query across multiple fields
// and returns candidate lightweight attrs rows.
func SearchUSPTO(ctx context.Context, apiKey string, number domain.PatentNumber) ([]domain.USPTOCandidate, error) {
	serial := strings.TrimSpace(number.Serial)
	if serial == "" {
		return nil, nil
	}
	query := usptoQuery(number)
	apiURL := "https://api.uspto.gov/api/v1/patent/applications/search?q=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("uspto: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var wrapperResp usptoFileWrapperResponse
	if err := json.Unmarshal(body, &wrapperResp); err != nil {
		return nil, err
	}

	if len(wrapperResp.PatentFileWrapperDataBag) == 0 {
		return nil, ErrUSPTOApplicationNotFound
	}

	var candidates []domain.USPTOCandidate
	for _, w := range wrapperResp.PatentFileWrapperDataBag {
		pub := w.ApplicationMetaData.PublicationNumber
		if pub == "" {
			pub = w.ApplicationMetaData.EarliestPublicationNumber
		}
		candidates = append(candidates, domain.USPTOCandidate{
			ApplicationNumber: w.ApplicationNumberText,
			Title:             w.ApplicationMetaData.InventionTitle,
			FilingDate:        w.ApplicationMetaData.FilingDate,
			FirstInventorName: w.ApplicationMetaData.FirstInventorName,
			GrantNumber:       w.ApplicationMetaData.PatentNumberText,
			PublicationNumber: pub,
		})
	}
	return candidates, nil
}

func sameDigits(a, b string) bool {
	aClean := a
	if p, err := domain.ParsePatentNumber(a); err == nil {
		aClean = p.Serial
	} else {
		aClean = onlyDigits(a)
	}
	bClean := b
	if p, err := domain.ParsePatentNumber(b); err == nil {
		bClean = p.Serial
	} else {
		bClean = onlyDigits(b)
	}
	return strings.TrimLeft(aClean, "0") == strings.TrimLeft(bClean, "0")
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func usptoInventors(m usptoApplicationMeta) []domain.Inventor {
	var out []domain.Inventor
	for _, inv := range m.InventorBag {
		name := strings.TrimSpace(inv.InventorNameText)
		if name == "" {
			name = strings.TrimSpace(strings.Join([]string{inv.FirstName, inv.MiddleName, inv.LastName, inv.NameSuffix}, " "))
		}
		if name != "" {
			out = append(out, domain.Inventor(clean(name)))
		}
	}
	if len(out) == 0 && strings.TrimSpace(m.FirstInventorName) != "" {
		out = append(out, domain.Inventor(strings.TrimSpace(m.FirstInventorName)))
	}
	return out
}

func usptoApplicants(m usptoApplicationMeta) []string {
	var out []string
	for _, a := range m.ApplicantBag {
		if name := strings.TrimSpace(a.ApplicantNameText); name != "" {
			out = append(out, name)
		}
	}
	if len(out) == 0 && strings.TrimSpace(m.FirstApplicantName) != "" {
		out = append(out, strings.TrimSpace(m.FirstApplicantName))
	}
	return out
}

// usptoClassifications extracts and normalizes CPC codes from the file-wrapper
// response's cpcClassificationBag (when present). It produces the same compact
// upper-case no-space form that googleClassifications + cleanClassification
// use, so :add.uspto immediately populates Patent.Classifications with the full
// comma-delimited (in UI) set instead of waiting for grant XML.
func usptoClassifications(m usptoApplicationMeta, fallback []string) []string {
	bag := m.CPCClassificationBag
	if len(bag) == 0 {
		bag = fallback
	}
	seen := map[string]bool{}
	var out []string
	for _, raw := range bag {
		c := cleanClassification(raw)
		if c != "" && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

func extractAdditionalUSPTODocuments(recordNumber domain.PatentNumber, w usptoWrapperData) ([]domain.Document, []domain.AuthorityIdentifier) {
	var docs []domain.Document
	var ids []domain.AuthorityIdentifier

	pubStr := strings.TrimSpace(w.ApplicationMetaData.PublicationNumber)
	if pubStr == "" {
		pubStr = strings.TrimSpace(w.ApplicationMetaData.EarliestPublicationNumber)
	}
	if pubStr != "" {
		if !strings.HasPrefix(strings.ToUpper(pubStr), "US") {
			pubStr = "US" + pubStr
		}
		if pubNum, err := domain.ParsePatentNumber(pubStr); err == nil {
			pubDate := time.Time{}
			if w.PGPubDocumentMetaData != nil {
				pubDate = parseISODate(w.PGPubDocumentMetaData.FileCreateDateTime)
			}
			docs = append(docs, domain.Document{
				Number: pubNum,
				Stage:  domain.StagePublication,
				Dated:  pubDate,
			})
			rawPub := w.ApplicationMetaData.PublicationNumber
			if rawPub == "" {
				rawPub = w.ApplicationMetaData.EarliestPublicationNumber
			}
			ids = append(ids, domain.AuthorityIdentifier{
				Authority:      "US",
				IdentifierType: "publication",
				Identifier:     pubNum.Normalized(),
				RawIdentifier:  rawPub,
				RecordNumber:   recordNumber,
				DocumentNumber: pubNum.Normalized(),
				Country:        "US",
				Kind:           pubNum.Kind,
				Dated: func() string {
					if w.PGPubDocumentMetaData != nil {
						return w.PGPubDocumentMetaData.FileCreateDateTime
					}
					return ""
				}(),
				Source:     string(domain.SourceUSPTO),
				Confidence: 100,
			})
		}
	}

	// Extract grant numbers if present
	grantDate := time.Time{}
	if w.GrantDocumentMetaData != nil {
		grantDate = parseISODate(w.GrantDocumentMetaData.FileCreateDateTime)
	}
	addedGrants := make(map[domain.PatentNumber]bool)
	for _, rawGrant := range []string{w.ApplicationMetaData.PatentNumber, w.ApplicationMetaData.PatentNumberText} {
		grantStr := strings.TrimSpace(rawGrant)
		if grantStr == "" {
			continue
		}
		if !strings.HasPrefix(strings.ToUpper(grantStr), "US") {
			grantStr = "US" + grantStr
		}
		if grantNum, err := domain.ParsePatentNumber(grantStr); err == nil {
			if !addedGrants[grantNum] {
				addedGrants[grantNum] = true
				docs = append(docs, domain.Document{
					Number: grantNum,
					Stage:  domain.StageGrant,
					Dated:  grantDate,
				})
				ids = append(ids, domain.AuthorityIdentifier{
					Authority:      "US",
					IdentifierType: "grant",
					Identifier:     grantNum.Normalized(),
					RawIdentifier:  rawGrant,
					RecordNumber:   recordNumber,
					DocumentNumber: grantNum.Normalized(),
					Country:        "US",
					Kind:           grantNum.Kind,
					Dated: func() string {
						if w.GrantDocumentMetaData != nil {
							return w.GrantDocumentMetaData.FileCreateDateTime
						}
						return ""
					}(),
					Source:     string(domain.SourceUSPTO),
					Confidence: 100,
				})
			}
		}
	}

	return docs, ids
}

func usptoParties(appNumber string, w usptoWrapperData) []domain.USPTOParty {
	var out []domain.USPTOParty
	for i, inv := range w.ApplicationMetaData.InventorBag {
		out = append(out, domain.USPTOParty{
			ApplicationNumber: appNumber,
			Role:              "inventor",
			Ordinal:           i,
			NameText:          firstNonEmpty(inv.InventorNameText, strings.TrimSpace(strings.Join([]string{inv.FirstName, inv.MiddleName, inv.LastName, inv.NameSuffix}, " "))),
			FirstName:         inv.FirstName,
			MiddleName:        inv.MiddleName,
			LastName:          inv.LastName,
			NameSuffix:        inv.NameSuffix,
			AddressJSON:       rawJSONOrDefault(inv.CorrespondenceAddressBag, "[]"),
		})
	}
	for i, a := range w.ApplicationMetaData.ApplicantBag {
		out = append(out, domain.USPTOParty{
			ApplicationNumber: appNumber,
			Role:              "applicant",
			Ordinal:           i,
			NameText:          a.ApplicantNameText,
			OrganizationName:  a.ApplicantNameText,
			AddressJSON:       rawJSONOrDefault(a.CorrespondenceAddressBag, "[]"),
		})
	}
	for i, a := range w.RecordAttorney.AttorneyBag {
		out = append(out, domain.USPTOParty{
			ApplicationNumber:    appNumber,
			Role:                 "attorney",
			Ordinal:              i,
			NameText:             strings.TrimSpace(strings.Join([]string{a.FirstName, a.MiddleName, a.LastName}, " ")),
			FirstName:            a.FirstName,
			MiddleName:           a.MiddleName,
			LastName:             a.LastName,
			RegistrationNumber:   a.RegistrationNumber,
			ActiveIndicator:      a.ActiveIndicator,
			PractitionerCategory: a.RegisteredPractitionerCategory,
			AddressJSON:          rawJSONOrDefault(a.AttorneyAddressBag, "[]"),
			TelecomJSON:          rawJSONOrDefault(a.TelecommunicationAddressBag, "[]"),
		})
	}
	return out
}

func usptoEvents(appNumber string, events []usptoEventData) []domain.USPTOEvent {
	out := make([]domain.USPTOEvent, 0, len(events))
	for i, e := range events {
		out = append(out, domain.USPTOEvent{
			ApplicationNumber:    appNumber,
			Ordinal:              i,
			EventCode:            e.EventCode,
			EventDescriptionText: e.EventDescriptionText,
			EventDate:            e.EventDate,
		})
	}
	return out
}

func usptoContinuities(appNumber string, childRecord domain.PatentNumber, rows []usptoContinuity) []domain.USPTOContinuity {
	out := make([]domain.USPTOContinuity, 0, len(rows))
	for i, c := range rows {
		parent, _ := parseUSApplicationNumber(c.ParentApplicationNumberText)
		out = append(out, domain.USPTOContinuity{
			ApplicationNumber:                 appNumber,
			Ordinal:                           i,
			ParentApplicationNumberText:       c.ParentApplicationNumberText,
			ChildApplicationNumberText:        c.ChildApplicationNumberText,
			ParentApplicationFilingDate:       c.ParentApplicationFilingDate,
			ParentApplicationStatusCode:       stringify(c.ParentApplicationStatusCode),
			ParentApplicationStatusText:       c.ParentApplicationStatusDescription,
			ClaimParentageTypeCode:            c.ClaimParentageTypeCode,
			ClaimParentageTypeDescriptionText: c.ClaimParentageTypeDescriptionText,
			ParentRecordNumber:                parent,
			ChildRecordNumber:                 childRecord,
		})
	}
	return out
}

func usptoForeignPriorities(appNumber string, rows []usptoForeign) []domain.USPTOForeignPriority {
	out := make([]domain.USPTOForeignPriority, 0, len(rows))
	for i, f := range rows {
		out = append(out, domain.USPTOForeignPriority{
			ApplicationNumber:        appNumber,
			Ordinal:                  i,
			ForeignApplicationNumber: strings.TrimSpace(f.ApplicationNumberText),
			FilingDate:               f.FilingDate,
			IPOfficeName:             f.IPOfficeName,
			Authority:                strings.ToUpper(strings.TrimSpace(f.IPOfficeName)),
		})
	}
	return out
}

func identifiersFromUSPTO(record domain.PatentNumber, appNumber string, w usptoWrapperData) []domain.AuthorityIdentifier {
	var out []domain.AuthorityIdentifier
	for _, c := range w.ParentContinuityBag {
		if raw := strings.TrimSpace(c.ParentApplicationNumberText); raw != "" {
			out = append(out, domain.AuthorityIdentifier{
				Authority:      authorityFromRaw(raw),
				IdentifierType: "application",
				Identifier:     raw,
				RawIdentifier:  raw,
				RecordNumber:   record,
				Dated:          c.ParentApplicationFilingDate,
				Source:         string(domain.SourceUSPTO),
				Confidence:     60,
			})
		}
	}
	for _, f := range w.ForeignPriorityBag {
		if raw := strings.TrimSpace(f.ApplicationNumberText); raw != "" {
			out = append(out, domain.AuthorityIdentifier{
				Authority:      firstNonEmpty(strings.ToUpper(strings.TrimSpace(f.IPOfficeName)), "FOREIGN"),
				IdentifierType: "priority",
				Identifier:     raw,
				RawIdentifier:  raw,
				RecordNumber:   record,
				Dated:          f.FilingDate,
				Source:         string(domain.SourceUSPTO),
				Confidence:     70,
			})
		}
	}
	return out
}

func relationsFromUSPTOContinuity(child domain.PatentNumber, rows []usptoContinuity) []domain.Relation {
	var out []domain.Relation
	for _, c := range rows {
		parent, ok := parseUSApplicationNumber(c.ParentApplicationNumberText)
		if !ok || parent.IsZero() || parent == child {
			continue
		}
		out = append(out,
			domain.Relation{From: child, To: parent, Kind: domain.RelationParent},
			domain.Relation{From: parent, To: child, Kind: domain.RelationChild})
	}
	return out
}

func parseUSApplicationNumber(raw string) (domain.PatentNumber, bool) {
	digits := onlyDigits(raw)
	if digits == "" || len(digits) > 9 || strings.Contains(strings.ToUpper(raw), "PCT") {
		return domain.PatentNumber{}, false
	}
	n, err := domain.ParsePatentNumber("US" + digits)
	return n, err == nil
}

func stringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatInt(int64(x), 10)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

func rawJSONOrDefault(raw json.RawMessage, fallback string) string {
	if len(raw) == 0 || string(raw) == "null" {
		return fallback
	}
	return string(raw)
}

func authorityFromRaw(raw string) string {
	upper := strings.ToUpper(strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(upper, "PCT"):
		return "PCT"
	case strings.HasPrefix(upper, "WO"):
		return "WO"
	default:
		return "US"
	}
}

func payloadHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func snapshotID(source, record string, body []byte) string {
	return source + ":" + record + ":" + payloadHash(body)[:16]
}

func encodeRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func parseISODate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02", "2006/01/02", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
