package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/store"
	"patentmine/internal/uspto"
)

func parseUSPTODateHelper(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, format := range []string{domain.DateLayout, domain.CompactDateLayout, domain.USDateLayout} {
		if t, err := time.Parse(format, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// ingestUSPTOXML opens the saved XML, parses it, and persists the parsed
// bundle. It is best-effort from the caller's perspective: failures are
// logged but do not fail the fetch — the XML file itself is still on disk.
// Three phase timings are recorded so slow stages can be located:
//
//	uspto.xml.parse  — read + decode
//	uspto.xml.map    — XML doc to ingest bundle
//	uspto.xml.save   — sqlite transaction
//
// plus the wrapping uspto.xml.ingest timing covering all three.

func (e *Engine) usptoLookupByAppNum(ctx context.Context, appNum string) (string, error) {
	appNum = strings.TrimSpace(appNum)
	if appNum == "" {
		return "", fmt.Errorf("engine: empty application number")
	}
	return e.usptoSearchRaw(ctx, "applicationNumberText:"+appNum)
}

// usptoSearchRaw runs one USPTO ODP applications search and returns the raw JSON
// body. Callers supply the fully-formed query string.
func (e *Engine) usptoSearchRaw(ctx context.Context, query string) (string, error) {
	apiKey := strings.TrimSpace(e.usptoAPIKey)
	if apiKey == "" {
		return "", fmt.Errorf("engine: USPTO API key not configured")
	}

	apiURL := "https://api.uspto.gov/api/v1/patent/applications/search?q=" + url.QueryEscape(query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("engine: USPTO returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return string(body), nil
}

// ClearPatentCache clears cached parsed XML bodies in uspto_grant_body for the given patents.
// If the patents slice is empty, it clears all records in the table.
// It returns the number of cleared records, estimated database bytes saved, and any error.

func (e *Engine) ComputeAndStoreUSPTOExpiration(ctx context.Context, n domain.PatentNumber, refresh bool) (app domain.USPTOApplication, err error) {
	start := time.Now()
	defer func() {
		dur := time.Since(start)
		if e.metrics != nil {
			e.metrics.ObserveDuration("engine.uspto.compute_expiration", dur, err != nil)
			e.metrics.IncCounter("engine.uspto.compute_expiration.count", 1)
		}
	}()

	// 1. Fetch/load the USPTOApplication from store
	app, err = e.repo.USPTOApplication(ctx, n)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Try to check if a local XML file exists in patentsDir first and ingest it
			if e.patentsDir != "" {
				pattern := filepath.Join(e.patentsDir, "*_"+n.Serial+".xml")
				matches, _ := filepath.Glob(pattern)
				if len(matches) > 0 {
					localPath := matches[0]
					baseName := filepath.Base(localPath)
					parts := strings.Split(baseName, "_")
					if len(parts) == 2 {
						appNum := parts[0]
						pRec := domain.Patent{
							Number:     n,
							FetchState: domain.FetchCached,
							Source:     domain.SourceUSPTO,
						}
						appRec := domain.USPTOApplication{
							ApplicationNumber: appNum,
							RecordNumber:      n,
						}
						batch := store.NodeBatch{
							Patent:           pRec,
							USPTOApplication: &appRec,
						}
						if sErr := e.repo.SaveNode(ctx, batch); sErr == nil {
							_ = e.ingestUSPTOXML(ctx, n, proto.USPTOXMLKindGrant, appNum, localPath)
						}
					}
				}
			}

			// Reload to check if successfully ingested from local XML cache
			app, err = e.repo.USPTOApplication(ctx, n)
			if err != nil {
				// Try a synchronous USPTO/ODP crawl/lookup to fetch, parse, and persist metadata
				if e.crawl != nil {
					job := e.crawl(n, 0, domain.CrawlProfileAll, false, domain.SourceUSPTO)
					_ = job.Run(ctx, "sync-uspto-lookup", func(proto.Event) {})
				}
				// Retry loading the USPTOApplication after crawl
				app, err = e.repo.USPTOApplication(ctx, n)
			}
		}
		// Third fallback: direct USPTO ODP API call when store and crawl both fail
		if err != nil {
			app, err = e.resolveUSPTOApplicationFromAPI(ctx, n)
		}
		if err != nil {
			return domain.USPTOApplication{}, fmt.Errorf("engine: failed to resolve USPTO application: %w", err)
		}
	}

	// When refresh is requested, re-fetch application metadata from the USPTO
	// ODP API even when a stored copy exists, then save the fresh data.
	if refresh && err == nil {
		fresh, apiErr := e.resolveUSPTOApplicationFromAPI(ctx, n)
		if apiErr == nil {
			fresh.RecordNumber = app.RecordNumber
			fresh.PGPubXMLURL = app.PGPubXMLURL
			fresh.PGPubXMLName = app.PGPubXMLName
			fresh.PatentGrantXMLURL = app.PatentGrantXMLURL
			fresh.PatentGrantXMLName = app.PatentGrantXMLName
			// Preserve PTE — the ODP API never includes it; it only comes from grant XML ingestion.
			fresh.PatentTermExtension = app.PatentTermExtension
			batch := store.NodeBatch{Patent: domain.Patent{Number: n, FetchState: domain.FetchCached, Source: domain.SourceUSPTO}, USPTOApplication: &fresh}
			_ = e.repo.SaveNode(ctx, batch)
			app = fresh
		}
	}

	// If PTE is still 0 and a grant XML URL is available, try to download the grant
	// XML and extract the patent term extension days. The ODP application search
	// API never returns PTE — it comes exclusively from the grant XML.
	if app.PatentTermExtension == 0 && strings.TrimSpace(app.PatentGrantXMLURL) != "" && strings.TrimSpace(e.usptoAPIKey) != "" {
		if pte, fetchErr := fetchPTEFromGrantURL(ctx, app.PatentGrantXMLURL, e.usptoAPIKey); fetchErr == nil && pte > 0 {
			app.PatentTermExtension = pte
			e.log(ctx, slog.LevelInfo, "extracted PTE from grant XML",
				slog.String("app_num", app.ApplicationNumber),
				slog.Int("pte", pte))
		}
	}

	appNum := app.ApplicationNumber
	apiKey := strings.TrimSpace(e.usptoAPIKey)

	// 2. Compute earliest term filing date by walking the continuity chain.
	// First run DB-only to establish a baseline, then enrich from the USPTO API
	// if continuity data is absent, and re-run to detect any change.
	filingDate := parseUSPTODateHelper(app.FilingDate)

	dbEarliest, dbStats, err := uspto.ComputeEarliestTermFilingDate(ctx, e.repo, appNum, filingDate, e.logger)
	if err != nil {
		e.log(ctx, slog.LevelWarn, "failed to compute earliest term filing date (db)", slog.String("app_num", appNum), slog.String("error", err.Error()))
	}

	earliestTermFilingDate := dbEarliest
	walkStats := dbStats

	// API enrichment: only on explicit refresh to avoid adding latency to normal calls.
	if refresh {
		fetchedCount, fetchErr := e.fetchMissingContinuities(ctx, n, app)
		if fetchErr != nil {
			e.log(ctx, slog.LevelWarn, "failed to fetch missing continuities from USPTO API", slog.String("app_num", appNum), slog.String("error", fetchErr.Error()))
		}
		if fetchedCount > 0 {
			apiEarliest, apiStats, apiErr := uspto.ComputeEarliestTermFilingDate(ctx, e.repo, appNum, filingDate, e.logger)
			if apiErr == nil {
				changed := !apiEarliest.Equal(dbEarliest)
				e.log(ctx, slog.LevelInfo, "continuity walk comparison: db vs api-enriched",
					slog.String("app_num", appNum),
					slog.String("db_earliest", dbEarliest.Format(domain.DateLayout)),
					slog.String("api_earliest", apiEarliest.Format(domain.DateLayout)),
					slog.Int("db_unique_apps", dbStats.UniqueApps),
					slog.Int("api_unique_apps", apiStats.UniqueApps),
					slog.Int("continuities_fetched", fetchedCount),
					slog.Bool("date_changed", changed),
				)
				if changed {
					e.incCounter("uspto.expiration.continuity.date_corrected_by_api", 1)
				}
				earliestTermFilingDate = apiEarliest
				walkStats = apiStats
			}
		}
	}

	e.incCounter("uspto.expiration.continuity.total_steps", int64(walkStats.TotalSteps))
	e.incCounter("uspto.expiration.continuity.unique_apps", int64(walkStats.UniqueApps))
	e.setGauge("uspto.expiration.continuity.max_depth", int64(walkStats.MaxDepth))
	app.EarliestTermAppNum = walkStats.EarliestApp

	// On refresh, prefetch and cache the parent application that contributed the earliest
	// filing date so that subsequent non-refresh calls can resolve its patent number and title.
	if refresh && app.EarliestTermAppNum != "" && app.EarliestTermAppNum != appNum {
		if _, dbErr := e.repo.USPTOApplicationByAppNum(ctx, app.EarliestTermAppNum); dbErr != nil {
			if _, apiErr := e.resolveUSPTOApplicationByAppNum(ctx, app.EarliestTermAppNum); apiErr != nil {
				e.log(ctx, slog.LevelDebug, "could not prefetch parent application metadata",
					slog.String("parent_app", app.EarliestTermAppNum), slog.String("error", apiErr.Error()))
			}
		}
	}

	// 3. Fetch live PTA and Documents (Terminal Disclaimers) if apiKey is present
	var ptaDays int
	var tdDate time.Time
	var hasTD bool
	var tdErr error
	if apiKey != "" {
		ptaDays, err = uspto.FetchPTADays(ctx, appNum, apiKey, e.logger)
		if err != nil {
			e.log(ctx, slog.LevelWarn, "failed to fetch PTA days from live USPTO API", slog.String("app_num", appNum), slog.String("error", err.Error()))
		}
		tdDate, hasTD, tdErr = uspto.FetchTerminalDisclaimerDate(ctx, appNum, apiKey, e.logger)
		if tdErr != nil {
			e.log(ctx, slog.LevelWarn, "failed to fetch terminal disclaimers from live USPTO API", slog.String("app_num", appNum), slog.String("error", tdErr.Error()))
		}
	}

	// Determine if we have parents (continuity)
	hasParents := !earliestTermFilingDate.IsZero() && !filingDate.IsZero() && earliestTermFilingDate != filingDate

	// Determine what to show for terminal disclaimer
	var tdDateStr string
	switch {
	case !tdDate.IsZero():
		tdDateStr = tdDate.Format(domain.DateLayout)
	case apiKey == "":
		// No API key configured — USPTO was never contacted.
		tdDateStr = "USPTO Not Reachable"
	case tdErr != nil:
		// API key present but the Documents call failed (auth, network, rate limit).
		tdDateStr = "USPTO Not Reachable"
	case hasParents:
		// API queried, no TD found, but patent has a continuity chain — explicitly checked, none found.
		tdDateStr = "None"
	default:
		tdDateStr = "Not Applicable"
	}

	// 4. Determine patent type
	grantKind := app.PatentGrantXMLName
	if grantKind == "" {
		grantKind = n.Kind
	}
	patentType := uspto.DeterminePatentType(n, app.ApplicationTypeLabel, app.ApplicationTypeCategory, grantKind)

	// 5. Get GrantDate — fall back to ApplicationStatusDate when the patent
	//    record does not have a stored grant date but the USPTO application
	//    status text indicates a granted patent.
	var grantDate time.Time
	patentRec, pErr := e.repo.Patent(ctx, n)
	if pErr == nil && !patentRec.GrantDate.IsZero() {
		grantDate = patentRec.GrantDate
	}
	if grantDate.IsZero() {
		if t, err := uspto.ParseGrantDateFromStatus(app.ApplicationStatusText, app.ApplicationStatusDate); err == nil {
			grantDate = t
		}
	}

	// 6. Compute statutory expiration
	var tdDateForCalc time.Time
	if !tdDate.IsZero() {
		tdDateForCalc = tdDate
	}
	computedExp := uspto.PatentExpiration(patentType, filingDate, grantDate, earliestTermFilingDate, ptaDays, app.PatentTermExtension, tdDateForCalc)

	// 7. Store computed expiration fields back in uspto_application table
	earliestTermFilingDateStr := ""
	if !earliestTermFilingDate.IsZero() {
		earliestTermFilingDateStr = earliestTermFilingDate.Format(domain.DateLayout)
	}
	computedExpStr := ""
	if !computedExp.IsZero() {
		computedExpStr = computedExp.Format(domain.DateLayout)
	}

	app.PatentTermAdjustmentDays = ptaDays
	app.TerminalDisclaimerDate = tdDateStr
	app.EarliestTermFilingDate = earliestTermFilingDateStr
	app.ComputedExpirationDate = computedExpStr

	nowStr := time.Now().UTC().Format(time.RFC3339)
	app.FetchedAt = nowStr
	app.LastIngestionDateTime = nowStr

	pRec := patentRec
	if pErr != nil {
		pRec = domain.Patent{
			Number:     n,
			FetchState: domain.FetchCached,
			Source:     domain.SourceUSPTO,
		}
	}
	batch := store.NodeBatch{
		Patent:           pRec,
		USPTOApplication: &app,
	}
	if err := e.repo.SaveNode(ctx, batch); err != nil {
		e.log(ctx, slog.LevelError, "failed to save computed expiration to database", slog.String("app_num", appNum), slog.String("error", err.Error()))
	}

	// 8. Fetch current Patent record to compare and/or update its ExpirationDate if needed
	p, err := e.repo.Patent(ctx, n)
	if err == nil {
		var googleExp time.Time
		if p.Source == domain.SourceGoogle {
			googleExp = p.ExpirationDate
		} else {
			googleExp = p.ExpirationDate
		}

		if !googleExp.IsZero() && !computedExp.IsZero() {
			diffDays := int(computedExp.Sub(googleExp).Hours() / 24)
			e.log(ctx, slog.LevelInfo, "patent expiration comparison",
				slog.String("number", n.String()),
				slog.String("computed_uspto_date", computedExpStr),
				slog.String("google_date", googleExp.Format(domain.DateLayout)),
				slog.Int("diff_days", diffDays),
				slog.Bool("has_terminal_disclaimer", hasTD),
			)
		}

		// Stamp the computed USPTO statutory expiration onto the record projection
		// (the value the detail panel renders) whenever we actually computed one.
		// This runs on an explicit :patent.expiration-date request, which has
		// already fetched the USPTO application data, so the computed date is the
		// authoritative expiration regardless of which source first populated the
		// record — many records (citation stubs, Google-primary) have a non-USPTO
		// or empty source and would otherwise never reflect the computed date. The
		// Google +20yr estimate, when present, is preserved in the source_bib
		// Google row for side-by-side comparison.
		if !computedExp.IsZero() && (!p.ExpirationDate.Equal(computedExp) || p.ExpirationSource != string(domain.SourceUSPTO)) {
			p.ExpirationDate = computedExp
			p.ExpirationSource = string(domain.SourceUSPTO)
			_ = e.repo.SavePatent(ctx, p)
		}
	}

	return app, nil
}

// stringify converts an arbitrary value to its trimmed string form, mirroring
// the same helper in crawl/uspto.go.
func stringify(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case float64:
		return fmt.Sprintf("%.0f", x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

// resolveUSPTOApplicationFromAPI fetches the USPTO application metadata directly from the
// USPTO ODP API when the local store has no record. This allows expiration-date and similar
// commands to work without pre-existing store data.
func (e *Engine) resolveUSPTOApplicationFromAPI(ctx context.Context, n domain.PatentNumber) (app domain.USPTOApplication, err error) {
	raw, apiErr := e.USPTOLookup(ctx, n)
	if apiErr != nil {
		return domain.USPTOApplication{}, apiErr
	}
	return e.parseAndStoreUSPTOApplication(ctx, raw, n)
}

// grantNumberFromXMLName extracts the grant number from a USPTO grant XML file
// name of the form "<appNum>_<grantNum>.xml" (e.g. "10269843_06915216.xml" →
// "6915216"). Returns "" when the name does not match or the grant token is not
// all digits. Leading zeros are stripped.
func grantNumberFromXMLName(name string) string {
	base := strings.TrimSuffix(strings.TrimSpace(name), ".xml")
	_, grant, ok := strings.Cut(base, "_")
	if !ok {
		return ""
	}
	grant = strings.TrimLeft(grant, "0")
	if grant == "" {
		return ""
	}
	for _, r := range grant {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return grant
}

// resolveUSPTOApplicationByAppNum fetches USPTO application data strictly by
// application number, avoiding the patent-number collision that the OR-query in
// resolveUSPTOApplicationFromAPI can hit (see usptoLookupByAppNum).
func (e *Engine) resolveUSPTOApplicationByAppNum(ctx context.Context, appNum string) (domain.USPTOApplication, error) {
	raw, apiErr := e.usptoLookupByAppNum(ctx, appNum)
	if apiErr != nil {
		return domain.USPTOApplication{}, apiErr
	}
	return e.parseAndStoreUSPTOApplication(ctx, raw, domain.PatentNumber{Serial: appNum})
}

// parseAndStoreUSPTOApplication decodes a USPTO ODP applications-search response
// into a USPTOApplication and persists it. fallbackNum supplies the record
// number when the response omits a granted patent number.
func (e *Engine) parseAndStoreUSPTOApplication(ctx context.Context, raw string, fallbackNum domain.PatentNumber) (app domain.USPTOApplication, err error) {
	var resp struct {
		Count                    int `json:"count"`
		PatentFileWrapperDataBag []struct {
			ApplicationNumberText string `json:"applicationNumberText"`
			ApplicationMetaData   struct {
				InventionTitle                string `json:"inventionTitle"`
				FilingDate                    string `json:"filingDate"`
				EffectiveFilingDate           string `json:"effectiveFilingDate"`
				ApplicationTypeCode           string `json:"applicationTypeCode"`
				ApplicationTypeLabelName      string `json:"applicationTypeLabelName"`
				ApplicationTypeCategory       string `json:"applicationTypeCategory"`
				FirstInventorName             string `json:"firstInventorName"`
				FirstApplicantName            string `json:"firstApplicantName"`
				ApplicationStatusCode         any    `json:"applicationStatusCode"`
				ApplicationStatusText         string `json:"applicationStatusDescriptionText"`
				ApplicationStatusDate         string `json:"applicationStatusDate"`
				GroupArtUnitNumber            string `json:"groupArtUnitNumber"`
				ExaminerNameText              string `json:"examinerNameText"`
				DocketNumber                  string `json:"docketNumber"`
				CustomerNumber                any    `json:"customerNumber"`
				ApplicationConfirmationNumber any    `json:"applicationConfirmationNumber"`
				FirstInventorToFileIndicator  string `json:"firstInventorToFileIndicator"`
				NationalStageIndicator        bool   `json:"nationalStageIndicator"`
				USPCSymbolText                string `json:"uspcSymbolText"`
				Class                         string `json:"class"`
				Subclass                      string `json:"subclass"`
				PatentNumberText              string `json:"patentNumberText"`
			} `json:"applicationMetaData"`
			GrantDocumentMetaData *struct {
				FileLocationURI string `json:"fileLocationURI"`
				XMLFileName     string `json:"xmlFileName"`
			} `json:"grantDocumentMetaData"`
			PGPubDocumentMetaData *struct {
				FileLocationURI string `json:"fileLocationURI"`
				XMLFileName     string `json:"xmlFileName"`
			} `json:"pgpubDocumentMetaData"`
		} `json:"patentFileWrapperDataBag"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return domain.USPTOApplication{}, fmt.Errorf("engine: parse uspto lookup response: %w", err)
	}
	if resp.Count == 0 || len(resp.PatentFileWrapperDataBag) == 0 {
		return domain.USPTOApplication{}, store.ErrNotFound
	}

	w := resp.PatentFileWrapperDataBag[0]
	appNum := strings.TrimSpace(w.ApplicationNumberText)
	if appNum == "" {
		return domain.USPTOApplication{}, store.ErrNotFound
	}

	// Resolve the granted patent number from the API response when available.
	// fallbackNum may be an application-style PatentNumber; patentNumberText gives
	// the actual grant. Older grants omit patentNumberText, so fall back to the
	// grant XML file name, which is "<appNum>_<grantNum>.xml".
	ptxt := strings.TrimSpace(w.ApplicationMetaData.PatentNumberText)
	if ptxt == "" && w.GrantDocumentMetaData != nil {
		ptxt = grantNumberFromXMLName(w.GrantDocumentMetaData.XMLFileName)
	}
	patentRecordNum := fallbackNum
	if ptxt != "" {
		if parsed, pErr := domain.ParsePatentNumber(ptxt); pErr == nil {
			patentRecordNum = parsed
		} else if parsed, pErr := domain.ParsePatentNumber("US" + ptxt + "B1"); pErr == nil {
			patentRecordNum = parsed
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	app = domain.USPTOApplication{
		ApplicationNumber:             appNum,
		RecordNumber:                  patentRecordNum,
		InventionTitle:                strings.TrimSpace(w.ApplicationMetaData.InventionTitle),
		FilingDate:                    w.ApplicationMetaData.FilingDate,
		EffectiveFilingDate:           w.ApplicationMetaData.EffectiveFilingDate,
		ApplicationStatusCode:         stringify(w.ApplicationMetaData.ApplicationStatusCode),
		ApplicationStatusText:         strings.TrimSpace(w.ApplicationMetaData.ApplicationStatusText),
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
		LastIngestionDateTime:         now,
		FetchedAt:                     now,
	}
	if w.GrantDocumentMetaData != nil {
		app.PatentGrantXMLURL = w.GrantDocumentMetaData.FileLocationURI
		app.PatentGrantXMLName = w.GrantDocumentMetaData.XMLFileName
	}
	if w.PGPubDocumentMetaData != nil {
		app.PGPubXMLURL = w.PGPubDocumentMetaData.FileLocationURI
		app.PGPubXMLName = w.PGPubDocumentMetaData.XMLFileName
	}

	// Persist the fetched application data so subsequent lookups hit the store
	batch := store.NodeBatch{
		Patent: domain.Patent{
			Number:     patentRecordNum,
			FetchState: domain.FetchCached,
			Source:     domain.SourceUSPTO,
		},
		USPTOApplication: &app,
	}
	if saveErr := e.repo.SaveNode(ctx, batch); saveErr != nil {
		e.log(ctx, slog.LevelWarn, "failed to persist fetched USPTO application",
			slog.String("number", patentRecordNum.String()),
			slog.String("error", saveErr.Error()))
	}

	return app, nil
}

// USPTOApplicationOrFetch returns USPTO application data for the given application number string.
// Checks the local store first. If absent and an API key is configured, fetches from the
// USPTO ODP API and caches the result so subsequent calls hit the store.
func (e *Engine) USPTOApplicationOrFetch(ctx context.Context, appNum string) (domain.USPTOApplication, error) {
	if app, err := e.repo.USPTOApplicationByAppNum(ctx, appNum); err == nil {
		return app, nil
	}
	if strings.TrimSpace(e.usptoAPIKey) == "" {
		return domain.USPTOApplication{}, store.ErrNotFound
	}
	return e.resolveUSPTOApplicationByAppNum(ctx, appNum)
}

// RefreshUSPTOApplicationByAppNum fetches application metadata fresh from the
// USPTO API strictly by application number, bypassing the store, and persists
// the enriched record. Use it to backfill data a cached stub is missing (e.g.
// the granted patent number of an earliest-term parent).
func (e *Engine) RefreshUSPTOApplicationByAppNum(ctx context.Context, appNum string) (domain.USPTOApplication, error) {
	if strings.TrimSpace(e.usptoAPIKey) == "" {
		return domain.USPTOApplication{}, store.ErrNotFound
	}
	return e.resolveUSPTOApplicationByAppNum(ctx, appNum)
}

// fetchMissingContinuities checks the store for continuity records for the given
// application. If none exist and an API key is configured, it fetches parent
// continuity data from the USPTO ODP API and persists the results so that
// subsequent DB-based walks benefit from the enriched data.
//
// Returns the number of continuity rows fetched from the API.
// Returns 0 if the store already had data or no API key is configured.
func (e *Engine) fetchMissingContinuities(ctx context.Context, n domain.PatentNumber, app domain.USPTOApplication) (int, error) {
	appNum := app.ApplicationNumber

	existing, err := e.repo.USPTOContinuities(ctx, appNum)
	if err == nil && len(existing) > 0 {
		e.log(ctx, slog.LevelDebug, "continuity data present in store, skipping API fetch",
			slog.String("app_num", appNum),
			slog.Int("count", len(existing)))
		return 0, nil
	}

	apiKey := strings.TrimSpace(e.usptoAPIKey)
	if apiKey == "" {
		return 0, nil
	}

	e.log(ctx, slog.LevelDebug, "continuity data absent from store, fetching from USPTO API",
		slog.String("app_num", appNum))

	raw, err := e.USPTOLookup(ctx, n)
	if err != nil {
		return 0, fmt.Errorf("engine: fetch continuities from USPTO API: %w", err)
	}

	var resp struct {
		Count                    int `json:"count"`
		PatentFileWrapperDataBag []struct {
			ApplicationNumberText string `json:"applicationNumberText"`
			ParentContinuityBag   []struct {
				ParentApplicationNumberText        string `json:"parentApplicationNumberText"`
				ChildApplicationNumberText         string `json:"childApplicationNumberText"`
				ParentApplicationFilingDate        string `json:"parentApplicationFilingDate"`
				ParentApplicationStatusCode        any    `json:"parentApplicationStatusCode"`
				ParentApplicationStatusDescription string `json:"parentApplicationStatusDescriptionText"`
				ClaimParentageTypeCode             string `json:"claimParentageTypeCode"`
				ClaimParentageTypeDescriptionText  string `json:"claimParentageTypeCodeDescriptionText"`
			} `json:"parentContinuityBag"`
		} `json:"patentFileWrapperDataBag"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return 0, fmt.Errorf("engine: parse continuities response: %w", err)
	}
	if resp.Count == 0 || len(resp.PatentFileWrapperDataBag) == 0 {
		return 0, nil
	}

	w := resp.PatentFileWrapperDataBag[0]
	if len(w.ParentContinuityBag) == 0 {
		e.log(ctx, slog.LevelDebug, "USPTO API returned no continuity records",
			slog.String("app_num", appNum))
		return 0, nil
	}

	continuities := make([]domain.USPTOContinuity, 0, len(w.ParentContinuityBag))
	for i, c := range w.ParentContinuityBag {
		continuities = append(continuities, domain.USPTOContinuity{
			ApplicationNumber:                 appNum,
			Ordinal:                           i,
			ParentApplicationNumberText:       c.ParentApplicationNumberText,
			ChildApplicationNumberText:        c.ChildApplicationNumberText,
			ParentApplicationFilingDate:       c.ParentApplicationFilingDate,
			ParentApplicationStatusCode:       stringify(c.ParentApplicationStatusCode),
			ParentApplicationStatusText:       c.ParentApplicationStatusDescription,
			ClaimParentageTypeCode:            c.ClaimParentageTypeCode,
			ClaimParentageTypeDescriptionText: c.ClaimParentageTypeDescriptionText,
		})
	}

	batch := store.NodeBatch{
		Patent: domain.Patent{
			Number:     n,
			FetchState: domain.FetchCached,
			Source:     domain.SourceUSPTO,
		},
		USPTOApplication:  &app,
		USPTOContinuities: continuities,
	}
	if saveErr := e.repo.SaveNode(ctx, batch); saveErr != nil {
		e.log(ctx, slog.LevelWarn, "failed to persist fetched continuities",
			slog.String("app_num", appNum),
			slog.String("error", saveErr.Error()))
	} else {
		e.log(ctx, slog.LevelInfo, "fetched and stored continuities from USPTO API",
			slog.String("app_num", appNum),
			slog.Int("count", len(continuities)))
		e.incCounter("uspto.expiration.continuity.api_fetched", int64(len(continuities)))
	}

	return len(continuities), nil
}

// fetchPTEFromGrantURL downloads a USPTO grant XML and extracts the
// us-term-extension days (PTE). Returns 0 when the element is absent or empty.
func fetchPTEFromGrantURL(ctx context.Context, grantURL, apiKey string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, grantURL, nil)
	if err != nil {
		return 0, fmt.Errorf("engine: create grant xml request: %w", err)
	}
	req.Header.Set("x-api-key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("engine: fetch grant xml: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return 0, fmt.Errorf("engine: read grant xml body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("engine: grant xml http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Extract <us-term-extension>NNN</us-term-extension> via string search.
	s := string(body)
	tag := "<us-term-extension>"
	start := strings.Index(s, tag)
	if start == -1 {
		return 0, nil // element absent — not an error
	}
	start += len(tag)
	end := strings.Index(s[start:], "</us-term-extension>")
	if end == -1 {
		return 0, nil // unclosed — treat as absent
	}
	val := strings.TrimSpace(s[start : start+end])
	if val == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("engine: parse term extension %q: %w", val, err)
	}
	return n, nil
}
