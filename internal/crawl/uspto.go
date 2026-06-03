package crawl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
)

// usptoMinInterval keeps requests to the USPTO ODP API polite.
const usptoMinInterval = 1 * time.Second

type usptoSource struct {
	strictSource *httpSource
	broadSource  *httpSource
	apiKey       string
}

func (s *usptoSource) Name() domain.Source {
	return domain.SourceUSPTO
}

func (s *usptoSource) Fetch(ctx context.Context, number domain.PatentNumber) (Result, error) {
	if note := usptoCoverageNote(number); note != "" {
		return Result{}, fmt.Errorf("%w: USPTO has no record for %s — %s; use uspto-first or compare mode to fall back to Google", ErrNotAvailable, number, note)
	}

	res, err := s.strictSource.Fetch(ctx, number)
	if err == nil {
		return res, nil
	}

	if errors.Is(err, ErrUSPTOApplicationNotFound) || errors.Is(err, ErrNotAvailable) {
		slog.Info("crawl/uspto: strict query failed, attempting broad query fallback",
			slog.String("raw_number", number.String()),
			slog.String("error", err.Error()))
		res, err = s.broadSource.Fetch(ctx, number)
		if err == nil {
			return res, nil
		}
		// A genuine miss is often a coverage gap, not a query problem: the USPTO
		// Open Data Portal indexes the *published-application* file-wrapper
		// dataset, so a document whose application was never published has no
		// record there. The kind code says which (see usptoCoverageNote). Make
		// the failure actionable (switch to uspto-first/compare so Google can
		// serve it) rather than a bare "not available". %w keeps ErrNotAvailable
		// so the registry still falls through to the next source.
		if errors.Is(err, ErrNotAvailable) {
			if note := usptoCoverageNote(number); note != "" {
				return Result{}, fmt.Errorf("%w: USPTO has no record for %s — %s; use uspto-first or compare mode to fall back to Google", ErrNotAvailable, number, note)
			}
		}
		return res, err
	}

	return Result{}, err
}

// usptoCoverageNote explains why the USPTO Open Data Portal has no record for a
// number when that is attributable to ODP coverage rather than a transient miss.
// The ODP indexes the published-application file-wrapper dataset, so the grant
// kind code is the signal: a US grant published as "B2" had a pre-grant
// publication and is present; "B1" was granted without one; and a bare "A"
// predates the pre-grant-publication system (pre-2001) — neither of the latter
// two has a file wrapper to find. Returns "" when no specific note applies.
func usptoCoverageNote(n domain.PatentNumber) string {
	if n.Country != "US" {
		return ""
	}
	switch {
	case n.Kind == "A":
		return "it is a pre-2001 grant, which predates the USPTO pre-grant-publication system, so the Open Data Portal (a published-application dataset) holds no file wrapper for it"
	case n.Kind == "B1":
		return "it was granted without a pre-grant publication (kind B1), so its application is not in the Open Data Portal's published-application dataset"
	}
	return ""
}

func (s *usptoSource) WithMetrics(metrics *observability.Metrics) {
	s.strictSource.WithMetrics(metrics)
	s.broadSource.WithMetrics(metrics)
}

func (s *usptoSource) WithLogger(logger *slog.Logger) {
	s.strictSource.WithLogger(logger)
	s.broadSource.WithLogger(logger)
}

// NewUSPTOSource builds a Source backed by the USPTO Patent File Wrapper API.
func NewUSPTOSource(apiKey string) Source {
	limiter := newLimiter(usptoMinInterval)
	client := &http.Client{Timeout: httpTimeout}
	headers := func() http.Header {
		h := make(http.Header)
		if strings.TrimSpace(apiKey) != "" {
			h.Set("x-api-key", apiKey)
		}
		h.Set("Accept", "application/json")
		return h
	}

	strict := &httpSource{
		name:    domain.SourceUSPTO,
		client:  client,
		limiter: limiter,
		urlFor: func(n domain.PatentNumber) string {
			q := usptoStrictQuery(n)
			slog.Info("crawl/uspto: query formulation (strict)",
				slog.String("raw_number", n.String()),
				slog.String("serial", n.Serial),
				slog.String("kind", n.Kind),
				slog.String("query", q))
			return "https://api.uspto.gov/api/v1/patent/applications/search?q=" + url.QueryEscape(q)
		},
		headers: headers,
		parse:   parseUSPTO,
	}

	broad := &httpSource{
		name:    domain.SourceUSPTO,
		client:  client,
		limiter: limiter,
		urlFor: func(n domain.PatentNumber) string {
			q := usptoBroadQuery(n)
			slog.Info("crawl/uspto: query formulation (broad)",
				slog.String("raw_number", n.String()),
				slog.String("serial", n.Serial),
				slog.String("kind", n.Kind),
				slog.String("query", q))
			return "https://api.uspto.gov/api/v1/patent/applications/search?q=" + url.QueryEscape(q)
		},
		headers: headers,
		parse:   parseUSPTO,
	}

	return &usptoSource{
		strictSource: strict,
		broadSource:  broad,
		apiKey:       apiKey,
	}
}

func usptoStrictQuery(n domain.PatentNumber) string {
	serial := strings.TrimSpace(n.Serial)
	if serial == "" {
		return ""
	}
	switch {
	case n.HasExplicitGrantKind():
		return fmt.Sprintf("applicationMetaData.patentNumber:%s", serial)
	case n.HasExplicitPublicationKind():
		return fmt.Sprintf("applicationMetaData.publicationNumber:%s", serial)
	}
	return fmt.Sprintf("applicationNumberText:%s OR applicationMetaData.patentNumber:%s OR applicationMetaData.publicationNumber:%s",
		serial, serial, serial)
}

func usptoBroadQuery(n domain.PatentNumber) string {
	serial := strings.TrimSpace(n.Serial)
	if serial == "" {
		return ""
	}
	norm := n.Normalized()
	switch {
	case n.HasExplicitGrantKind():
		if norm != "" && norm != serial {
			return fmt.Sprintf("applicationMetaData.patentNumber:%s OR %q OR %q",
				serial, norm, serial)
		}
		return fmt.Sprintf("applicationMetaData.patentNumber:%s OR %q",
			serial, serial)
	case n.HasExplicitPublicationKind():
		if norm != "" && norm != serial {
			return fmt.Sprintf("applicationMetaData.publicationNumber:%s OR %q OR %q",
				serial, norm, serial)
		}
		return fmt.Sprintf("applicationMetaData.publicationNumber:%s OR %q",
			serial, serial)
	}
	if norm != "" && norm != serial {
		return fmt.Sprintf("applicationNumberText:%s OR applicationMetaData.patentNumber:%s OR applicationMetaData.publicationNumber:%s OR %q OR %q",
			serial, serial, serial, norm, serial)
	}
	return fmt.Sprintf("applicationNumberText:%s OR applicationMetaData.patentNumber:%s OR applicationMetaData.publicationNumber:%s OR %q",
		serial, serial, serial, serial)
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
	ApplicationNumberText string                `json:"applicationNumberText"`
	ApplicationMetaData   usptoApplicationMeta  `json:"applicationMetaData"`
	EventDataBag          []usptoEventData      `json:"eventDataBag"`
	ParentContinuityBag   []usptoContinuity     `json:"parentContinuityBag"`
	ForeignPriorityBag    []usptoForeign        `json:"foreignPriorityBag"`
	RecordAttorney        usptoRecordAttorney   `json:"recordAttorney"`
	LastIngestionDateTime string                `json:"lastIngestionDateTime"`
	GrantDocumentMetaData *usptoDocumentMeta    `json:"grantDocumentMetaData"`
	PGPubDocumentMetaData *usptoDocumentMeta    `json:"pgpubDocumentMetaData"`
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
	EntityStatusData              usptoEntity      `json:"entityStatusData"`
	PublicationCategoryBag        json.RawMessage  `json:"publicationCategoryBag"`
	InventorBag                   []usptoInventor  `json:"inventorBag"`
	ApplicantBag                  []usptoApplicant `json:"applicantBag"`
	PatentNumberText              string           `json:"patentNumberText"`
	PatentNumber                  string           `json:"patentNumber"`
	PublicationNumber             string           `json:"publicationNumber"`
	EarliestPublicationNumber     string           `json:"earliestPublicationNumber"`
}

func (m *usptoApplicationMeta) UnmarshalJSON(data []byte) error {
	type Alias usptoApplicationMeta
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if m.PublicationNumber == "" && m.EarliestPublicationNumber != "" {
		m.PublicationNumber = m.EarliestPublicationNumber
	}
	return nil
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

func parseUSPTO(number domain.PatentNumber, body []byte) (Result, error) {
	var resp usptoFileWrapperResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return Result{}, fmt.Errorf("crawl/uspto: decode response: %w", err)
	}
	if len(resp.PatentFileWrapperDataBag) == 0 {
		return Result{}, ErrUSPTOApplicationNotFound
	}

	if len(resp.PatentFileWrapperDataBag) > 1 {
		slog.Warn("crawl/uspto: multiple wrappers returned for serial digits",
			slog.String("requested", number.String()),
			slog.Int("count", len(resp.PatentFileWrapperDataBag)),
			slog.String("detail", formatWrapperDetails(resp.PatentFileWrapperDataBag)))
		if Metrics != nil {
			Metrics.IncCounter("crawl.uspto.multiple_candidates_total", 1)
		}
	}

	w, ok := matchingUSPTOWrapper(number, resp.PatentFileWrapperDataBag)
	if !ok {
		if len(resp.PatentFileWrapperDataBag) > 0 {
			slog.Warn("crawl/uspto: candidates found but rejected due to low match score (score < 5)",
				slog.String("requested", number.String()),
				slog.Int("candidate_count", len(resp.PatentFileWrapperDataBag)),
				slog.String("detail", formatWrapperDetails(resp.PatentFileWrapperDataBag)))
			if Metrics != nil {
				Metrics.IncCounter("crawl.uspto.match_rejected_total", 1)
			}
		}
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
	var pubDate time.Time
	if w.PGPubDocumentMetaData != nil {
		pubDate = parseISODate(w.PGPubDocumentMetaData.FileCreateDateTime)
	}
	var grantDate time.Time
	if w.GrantDocumentMetaData != nil {
		grantDate = parseISODate(w.GrantDocumentMetaData.FileCreateDateTime)
	}
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
		FetchState:      domain.FetchCached,
		Source:          domain.SourceUSPTO,
		FetchedAt:       now,
		ApplicationDate: filingDate,
		PublicationDate: pubDate,
		GrantDate:       grantDate,
		SourceURL:       "https://api.uspto.gov/api/v1/patent/applications/search?q=" + url.QueryEscape("applicationNumberText:"+appNumber),
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

	extraDocs, extraIds := extractAdditionalUSPTODocuments(number, recordNumber, w)
	res.Documents = append(res.Documents, extraDocs...)
	res.AuthorityIdentifiers = append(res.AuthorityIdentifiers, extraIds...)

	// The matched wrapper does not always carry the grant/publication document
	// metadata for the number the caller asked for — ODP often returns only an
	// application file wrapper, even for a granted patent (see USPTO ODP
	// coverage). Without a document bearing the requested number, resolveRecord
	// cannot tie this fetch to the stub that already exists for it (e.g. a
	// citation neighbour discovered earlier by Google), so the data is orphaned
	// under the application number and the stub stays empty. matchingUSPTOWrapper
	// already confirmed this wrapper corresponds to the requested number, so bind
	// that number to the record explicitly. (See the record-number identity
	// divergence.)
	res.Documents, res.AuthorityIdentifiers = ensureRequestedDocument(number, recordNumber, res.Documents, res.AuthorityIdentifiers)

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
		PGPubXMLURL:                   func() string { if w.PGPubDocumentMetaData != nil { return w.PGPubDocumentMetaData.FileLocationURI }; return "" }(),
		PGPubXMLName:                  func() string { if w.PGPubDocumentMetaData != nil { return w.PGPubDocumentMetaData.XMLFileName }; return "" }(),
		PatentGrantXMLURL:             func() string { if w.GrantDocumentMetaData != nil { return w.GrantDocumentMetaData.FileLocationURI }; return "" }(),
		PatentGrantXMLName:            func() string { if w.GrantDocumentMetaData != nil { return w.GrantDocumentMetaData.XMLFileName }; return "" }(),
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
	res.SourceBibs = []domain.SourceBib{domain.SourceBibFromPatent(patent)}
	return res, nil
}

func matchingUSPTOWrapper(number domain.PatentNumber, bags []usptoWrapperData) (usptoWrapperData, bool) {
	var bestW usptoWrapperData
	bestScore := -1

	stage := domain.GuessStage(number)

	for _, w := range bags {
		score := 0
		matchesApp := matchesPatent(w.ApplicationNumberText, number)
		matchesGrant := matchesPatent(w.ApplicationMetaData.PatentNumber, number) || matchesPatent(w.ApplicationMetaData.PatentNumberText, number)
		matchesPub := matchesPatent(w.ApplicationMetaData.PublicationNumber, number)

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

	if bestScore >= 5 {
		return bestW, true
	}
	return usptoWrapperData{}, false
}

// SearchUSPTO queries the USPTO ODP API using a strict query, falling back to a broad query
// across multiple fields only if no candidates are found, and returns candidate lightweight rows.
func SearchUSPTO(ctx context.Context, apiKey string, number domain.PatentNumber) ([]domain.USPTOCandidate, error) {
	if note := usptoCoverageNote(number); note != "" {
		return nil, fmt.Errorf("%w: USPTO has no record for %s — %s; use uspto-first or compare mode to fall back to Google", ErrUSPTOApplicationNotFound, number, note)
	}

	serial := strings.TrimSpace(number.Serial)
	if serial == "" {
		return nil, nil
	}

	// 1. Try strict search
	candidates, err := searchUSPTOWithQuery(ctx, apiKey, number, usptoStrictQuery(number))
	if err == nil && len(candidates) > 0 {
		return candidates, nil
	}

	// 2. Try broad fallback search
	slog.Info("crawl/uspto: SearchUSPTO strict query yielded no candidates, attempting broad fallback",
		slog.String("raw_number", number.String()))

	candidates, err = searchUSPTOWithQuery(ctx, apiKey, number, usptoBroadQuery(number))
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, ErrUSPTOApplicationNotFound
	}
	return candidates, nil
}

func searchUSPTOWithQuery(ctx context.Context, apiKey string, number domain.PatentNumber, query string) ([]domain.USPTOCandidate, error) {
	slog.Info("crawl/uspto: SearchUSPTO query formulation",
		slog.String("raw_number", number.String()),
		slog.String("serial", number.Serial),
		slog.String("kind", number.Kind),
		slog.String("query", query))
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

	var candidates []domain.USPTOCandidate
	for _, w := range wrapperResp.PatentFileWrapperDataBag {
		candidates = append(candidates, domain.USPTOCandidate{
			ApplicationNumber: w.ApplicationNumberText,
			Title:             w.ApplicationMetaData.InventionTitle,
			FilingDate:        w.ApplicationMetaData.FilingDate,
			FirstInventorName: w.ApplicationMetaData.FirstInventorName,
			GrantNumber:       w.ApplicationMetaData.PatentNumberText,
			PublicationNumber: w.ApplicationMetaData.PublicationNumber,
		})
	}
	return candidates, nil
}

func matchesPatent(raw string, target domain.PatentNumber) bool {
	parsed, err := domain.ParsePatentNumber(raw)
	var matched bool
	var parsedSerial string
	var parseSuccess bool

	if err == nil {
		parseSuccess = true
		parsedSerial = parsed.Serial
		matched = (strings.TrimLeft(parsed.Serial, "0") == strings.TrimLeft(target.Serial, "0"))
	} else {
		// Fallback to naive digit-only stripping if ParsePatentNumber fails
		parsedSerial = onlyDigits(raw)
		matched = (strings.TrimLeft(parsedSerial, "0") == strings.TrimLeft(onlyDigits(target.Serial), "0"))
	}

	// Telemetry/metrics
	if Metrics != nil {
		Metrics.IncCounter("crawl.uspto.matches_patent.calls_total", 1)
		if parseSuccess {
			Metrics.IncCounter("crawl.uspto.matches_patent.parse_success_total", 1)
		} else {
			Metrics.IncCounter("crawl.uspto.matches_patent.parse_fallback_total", 1)
		}
		if matched {
			Metrics.IncCounter("crawl.uspto.matches_patent.matched_total", 1)
		}
	}

	// Logging for debugging/conversion tracking purposes
	slog.Info("matchesPatent comparison",
		slog.String("raw_input", raw),
		slog.String("target_number", target.String()),
		slog.Bool("parse_success", parseSuccess),
		slog.String("parsed_serial", parsedSerial),
		slog.Bool("matched", matched))

	return matched
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

func extractAdditionalUSPTODocuments(requested, recordNumber domain.PatentNumber, w usptoWrapperData) ([]domain.Document, []domain.AuthorityIdentifier) {
	var docs []domain.Document
	var ids []domain.AuthorityIdentifier

	// Extract publication number if present
	pubStr := strings.TrimSpace(w.ApplicationMetaData.PublicationNumber)
	if w.PGPubDocumentMetaData != nil && strings.TrimSpace(w.PGPubDocumentMetaData.ProductIdentifier) != "" {
		pubStr = strings.TrimSpace(w.PGPubDocumentMetaData.ProductIdentifier)
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
			ids = append(ids, domain.AuthorityIdentifier{
				Authority:      "US",
				IdentifierType: "publication",
				Identifier:     pubNum.Normalized(),
				RawIdentifier:  pubStr,
				RecordNumber:   recordNumber,
				DocumentNumber: pubNum.Normalized(),
				Country:        "US",
				Kind:           pubNum.Kind,
				Dated:          func() string { if w.PGPubDocumentMetaData != nil { return w.PGPubDocumentMetaData.FileCreateDateTime }; return "" }(),
				Source:         string(domain.SourceUSPTO),
				Confidence:     100,
			})
		}
	}

	// Extract grant numbers if present
	grantDate := time.Time{}
	if w.GrantDocumentMetaData != nil {
		grantDate = parseISODate(w.GrantDocumentMetaData.FileCreateDateTime)
	}

	var grantStr string
	if w.GrantDocumentMetaData != nil && strings.TrimSpace(w.GrantDocumentMetaData.ProductIdentifier) != "" {
		grantStr = strings.TrimSpace(w.GrantDocumentMetaData.ProductIdentifier)
	}
	if grantStr == "" {
		grantStr = strings.TrimSpace(w.ApplicationMetaData.PatentNumber)
		if grantStr == "" {
			grantStr = strings.TrimSpace(w.ApplicationMetaData.PatentNumberText)
		}
	}

	if grantStr != "" {
		if !strings.HasPrefix(strings.ToUpper(grantStr), "US") {
			grantStr = "US" + grantStr
		}
		if grantNum, err := domain.ParsePatentNumber(grantStr); err == nil {
			// The ODP grant identifier is a bare serial (e.g. "6541975") with no
			// kind code, but the user requested — and the crawler stubbed — the
			// grant with its kind ("US6541975B2"). RecordOf resolves by exact
			// document number, so without the kind this grant document never
			// matches that stub and the fetched bibliographic data is orphaned
			// under the application number instead. Carry the requested kind onto
			// the grant document so it binds to the record the user added.
			// (See the record-number identity divergence.)
			if requested.Kind != "" && requested.Country == grantNum.Country && requested.Serial == grantNum.Serial {
				grantNum = requested
			}
			docs = append(docs, domain.Document{
				Number: grantNum,
				Stage:  domain.StageGrant,
				Dated:  grantDate,
			})
			ids = append(ids, domain.AuthorityIdentifier{
				Authority:      "US",
				IdentifierType: "grant",
				Identifier:     grantNum.Normalized(),
				RawIdentifier:  grantStr,
				RecordNumber:   recordNumber,
				DocumentNumber: grantNum.Normalized(),
				Country:        "US",
				Kind:           grantNum.Kind,
				Dated:          func() string { if w.GrantDocumentMetaData != nil { return w.GrantDocumentMetaData.FileCreateDateTime }; return "" }(),
				Source:         string(domain.SourceUSPTO),
				Confidence:     100,
			})
		}
	}

	return docs, ids
}

// ensureRequestedDocument guarantees the caller-requested number is represented
// as a document of the resolved record, so record resolution (resolveRecord)
// unifies this fetch with any stub already keyed by that number rather than
// orphaning the data under the application number. It is a no-op when the
// requested number is the record (application) number itself or is already
// among the documents (e.g. extractAdditionalUSPTODocuments recovered it from
// grant/publication metadata).
func ensureRequestedDocument(requested, recordNumber domain.PatentNumber, docs []domain.Document, ids []domain.AuthorityIdentifier) ([]domain.Document, []domain.AuthorityIdentifier) {
	if requested.IsZero() || requested == recordNumber {
		return docs, ids
	}
	for _, d := range docs {
		if d.Number == requested {
			return docs, ids
		}
	}
	stage := domain.GuessStage(requested)
	identifierType := "grant"
	if stage == domain.StagePublication {
		identifierType = "publication"
	}
	docs = append(docs, domain.Document{Number: requested, Stage: stage})
	ids = append(ids, domain.AuthorityIdentifier{
		Authority:      "US",
		IdentifierType: identifierType,
		Identifier:     requested.Normalized(),
		RawIdentifier:  requested.String(),
		RecordNumber:   recordNumber,
		DocumentNumber: requested.Normalized(),
		Country:        requested.Country,
		Kind:           requested.Kind,
		Source:         string(domain.SourceUSPTO),
		Confidence:     100,
	})
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
			domain.Relation{From: child, To: parent, Kind: domain.RelationParent})
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
	// USPTO dates come as YYYY-MM-DD, the slash variant YYYY/MM/DD, or RFC3339.
	// Normalizing the separator lets the canonical domain.DateLayout cover the
	// first two, so there is no second date literal to keep in sync.
	s = strings.ReplaceAll(strings.TrimSpace(s), "/", "-")
	for _, layout := range []string{domain.DateLayout, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func formatWrapperDetails(bags []usptoWrapperData) string {
	var details []string
	for i, w := range bags {
		details = append(details, fmt.Sprintf(
			"[%d] AppNum: %q, PatentNum: %q, PatentNumText: %q, PubNum: %q, Title: %q",
			i,
			w.ApplicationNumberText,
			w.ApplicationMetaData.PatentNumber,
			w.ApplicationMetaData.PatentNumberText,
			w.ApplicationMetaData.PublicationNumber,
			w.ApplicationMetaData.InventionTitle,
		))
	}
	return "[" + strings.Join(details, "; ") + "]"
}
