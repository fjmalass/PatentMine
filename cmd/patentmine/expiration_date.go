package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"patentmine/internal/config"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/uspto"
)

const expirationDateUsage = `usage:
  patentmine expiration-date [options] <application-or-patent-number>

options:
  -source string   data source: uspto, google, or both (default "both")
`

func runExpirationDate(args []string) int {
	fs := flag.NewFlagSet("expiration-date", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(fs.Output(), expirationDateUsage) }

	var sourceFlag string
	fs.StringVar(&sourceFlag, "source", "both", "data source: uspto, google, or both")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return 2
	}

	rawNum := strings.TrimSpace(fs.Arg(0))
	if rawNum == "" {
		fmt.Fprintln(os.Stderr, "patentmine expiration-date: application or patent number is required")
		return 2
	}

	pn, err := domain.ParsePatentNumber(rawNum)
	if err != nil {
		pn = domain.PatentNumber{Serial: rawNum}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		return 1
	}

	apiKey := strings.TrimSpace(cfg.USPTOAPIKey)
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "patentmine expiration-date: USPTO API key not configured (set PATENTMINE_USPTO_API_KEY)")
		return 1
	}
	resolved, err := config.ResolveAPIKey(apiKey)
	if err == nil {
		apiKey = resolved
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Fprintf(os.Stderr, "Querying live USPTO API for: %s\n", pn.String())

	// Step 1: Resolve patent/application number via USPTO ODP search API
	appNum, filingDateStr, grantDateStr, title, inventors, appTypeLabel, appTypeCategory, err := lookupApplication(ctx, pn, apiKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to resolve application: %v\n", err)
		return 1
	}

	fmt.Fprintf(os.Stderr, "Resolved application: %s (filed %s)\n", appNum, filingDateStr)
	if grantDateStr != "" {
		fmt.Fprintf(os.Stderr, "Grant date: %s\n", grantDateStr)
	}
	if inventors != "" {
		fmt.Fprintf(os.Stderr, "Inventors: %s\n", inventors)
	}

	// Step 2: Fetch PTA days
	ptaDays, _ := uspto.FetchPTADays(ctx, appNum, apiKey, nil)
	if ptaDays > 0 {
		fmt.Fprintf(os.Stderr, "PTA days: %d\n", ptaDays)
	}

	// Step 3: Fetch Terminal Disclaimer date
	tdDate, hasTD, _ := uspto.FetchTerminalDisclaimerDate(ctx, appNum, apiKey, nil)
	if hasTD && !tdDate.IsZero() {
		fmt.Fprintf(os.Stderr, "Terminal disclaimer: %s\n", tdDate.Format("2006-01-02"))
	}

	// Step 4: Compute expiration
	filingDate, _ := time.Parse("2006-01-02", filingDateStr)
	var grantDate time.Time
	if grantDateStr != "" {
		grantDate, _ = time.Parse("2006-01-02", grantDateStr)
	}
	patentType := uspto.DeterminePatentType(pn, appTypeLabel, appTypeCategory, pn.Kind)

	expirationDate := uspto.PatentExpiration(patentType, filingDate, grantDate, time.Time{}, ptaDays, 0, tdDate)

	tdDateStr := ""
	if hasTD && !tdDate.IsZero() {
		tdDateStr = tdDate.Format("2006-01-02")
	}
	expirationStr := ""
	if !expirationDate.IsZero() {
		expirationStr = expirationDate.Format("2006-01-02")
	}

	res := proto.USPTOExpirationCalculateResult{
		ApplicationNumber:        appNum,
		PatentNumber:             pn.String(),
		Title:                    title,
		Inventors:                inventors,
		FilingDate:               filingDateStr,
		GrantDate:                grantDateStr,
		PatentTermAdjustmentDays: ptaDays,
		TerminalDisclaimerDate:   tdDateStr,
		ComputedExpirationDate:   expirationStr,
	}

	printResult(res, sourceFlag)
	return 0
}

// lookupApplication resolves a patent/application number to its application
// number, filing date, title, and type info via the USPTO ODP search API.
func lookupApplication(ctx context.Context, n domain.PatentNumber, apiKey string) (appNum, filingDate, grantDate, title, inventors, appTypeLabel, appTypeCategory string, err error) {
	serial := strings.TrimSpace(n.Serial)
	if serial == "" {
		return "", "", "", "", "", "", "", fmt.Errorf("empty patent number")
	}

	norm := n.Normalized()
	var query string
	if norm != "" && norm != serial {
		query = fmt.Sprintf("applicationNumberText:%s OR patentNumberText:%s OR publicationNumberText:%s OR publicationNumber:%s OR %q OR %q",
			serial, serial, serial, serial, norm, serial)
	} else {
		query = fmt.Sprintf("applicationNumberText:%s OR patentNumberText:%s OR publicationNumberText:%s OR publicationNumber:%s OR %q",
			serial, serial, serial, serial, serial)
	}

	apiURL := "https://api.uspto.gov/api/v1/patent/applications/search?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", "", "", "", "", "", "", err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", "", "", "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", "", "", "", "", "", "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", "", "", "", "", "", fmt.Errorf("USPTO search API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var searchResp struct {
		Count                    int `json:"count"`
		PatentFileWrapperDataBag []struct {
			ApplicationNumberText string `json:"applicationNumberText"`
			ApplicationMetaData   struct {
				InventionTitle            string `json:"inventionTitle"`
				FilingDate                string `json:"filingDate"`
				ApplicationTypeLabelName  string `json:"applicationTypeLabelName"`
				ApplicationTypeCategory   string `json:"applicationTypeCategory"`
				FirstInventorName         string `json:"firstInventorName"`
				ApplicationStatusCode     any    `json:"applicationStatusCode"`
				ApplicationStatusText     string `json:"applicationStatusDescriptionText"`
				ApplicationStatusDate     string `json:"applicationStatusDate"`
				InventorBag              []struct {
					InventorNameText string `json:"inventorNameText"`
					FirstName        string `json:"firstName"`
					MiddleName       string `json:"middleName"`
					LastName         string `json:"lastName"`
					NameSuffix       string `json:"nameSuffix"`
				} `json:"inventorBag"`
			} `json:"applicationMetaData"`
			GrantDocumentMetaData *struct {
				FileCreateDateTime string `json:"fileCreateDateTime"`
			} `json:"grantDocumentMetaData"`
		} `json:"patentFileWrapperDataBag"`
	}
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return "", "", "", "", "", "", "", fmt.Errorf("parse USPTO response: %w", err)
	}
	if searchResp.Count == 0 || len(searchResp.PatentFileWrapperDataBag) == 0 {
		return "", "", "", "", "", "", "", fmt.Errorf("no results found for %s", n.String())
	}

	w := searchResp.PatentFileWrapperDataBag[0]
	appNum = strings.TrimSpace(w.ApplicationNumberText)
	if appNum == "" {
		return "", "", "", "", "", "", "", fmt.Errorf("empty application number in response")
	}

	meta := w.ApplicationMetaData

	filingDate = meta.FilingDate
	title = strings.TrimSpace(meta.InventionTitle)
	appTypeLabel = meta.ApplicationTypeLabelName
	appTypeCategory = meta.ApplicationTypeCategory

	// Extract grant date: prefer ApplicationStatusDate for granted patents,
	// fall back to GrantDocumentMetaData.FileCreateDateTime
	grantDate = uspto.GrantDateFromStatus(meta.ApplicationStatusText, meta.ApplicationStatusDate)
	if grantDate == "" && w.GrantDocumentMetaData != nil {
		grantDate = w.GrantDocumentMetaData.FileCreateDateTime
	}

	// Extract inventors from inventorBag, fall back to FirstInventorName
	var invs []string
	for _, inv := range meta.InventorBag {
		name := strings.TrimSpace(inv.InventorNameText)
		if name == "" {
			parts := strings.TrimSpace(strings.Join([]string{inv.FirstName, inv.MiddleName, inv.LastName, inv.NameSuffix}, " "))
			if parts != "" {
				name = parts
			}
		}
		if name != "" {
			invs = append(invs, name)
		}
	}
	if len(invs) == 0 && strings.TrimSpace(meta.FirstInventorName) != "" {
		invs = append(invs, strings.TrimSpace(meta.FirstInventorName))
	}
	inventors = strings.Join(invs, "; ")

	// Normalize filing date to YYYY-MM-DD
	if t, err := time.Parse("2006-01-02", filingDate); err == nil {
		filingDate = t.Format("2006-01-02")
	} else if t, err := time.Parse("20060102", filingDate); err == nil {
		filingDate = t.Format("2006-01-02")
	} else if t, err := time.Parse("01-02-2006", filingDate); err == nil {
		filingDate = t.Format("2006-01-02")
	}

	// Normalize grant date to YYYY-MM-DD
	if grantDate != "" {
		if t, err := time.Parse("2006-01-02", grantDate); err == nil {
			grantDate = t.Format("2006-01-02")
		} else if t, err := time.Parse("20060102", grantDate); err == nil {
			grantDate = t.Format("2006-01-02")
		} else if t, err := time.Parse(time.RFC3339, grantDate); err == nil {
			grantDate = t.Format("2006-01-02")
		} else if len(grantDate) >= 10 {
			grantDate = grantDate[:10]
		}
	}

	return appNum, filingDate, grantDate, title, inventors, appTypeLabel, appTypeCategory, nil
}

func printResult(res proto.USPTOExpirationCalculateResult, sourceMode string) {
	fmt.Println("\n=======================================================")
	fmt.Println("         Patent Expiration Analysis Summary            ")
	fmt.Println("=======================================================")
	fmt.Printf("Patent Number:                 %s\n", res.PatentNumber)
	fmt.Printf("Application Number:            %s\n", res.ApplicationNumber)
	if res.Title != "" {
		fmt.Printf("Title:                         %s\n", res.Title)
	}
	if res.Inventors != "" {
		fmt.Printf("Inventors:                     %s\n", res.Inventors)
	}

	showUSPTO := sourceMode == "uspto" || sourceMode == "both"
	showGoogle := sourceMode == "google" || sourceMode == "both"

	if showUSPTO {
		fmt.Println("\n--- USPTO Intermediate Data ---")
		fmt.Printf("Filing Date:                   %s\n", formatStr(res.FilingDate))
		fmt.Printf("Grant Date:                    %s\n", formatStr(res.GrantDate))
		fmt.Printf("Earliest Term Filing Date:     %s\n", formatStr(res.EarliestTermFilingDate))
		fmt.Printf("Patent Term Adjustment (PTA):  %d days\n", res.PatentTermAdjustmentDays)
		fmt.Printf("Patent Term Extension (PTE):   %d days\n", res.PatentTermExtensionDays)
		fmt.Printf("Terminal Disclaimer Date:      %s\n", formatStr(res.TerminalDisclaimerDate))
		fmt.Printf("Computed Expiration Date:      %s\n", formatStr(res.ComputedExpirationDate))
	}

	if showGoogle {
		fmt.Println("\n--- Google Patents Data ---")
		fmt.Printf("Parsed Expiration Date:        %s\n", formatStr(res.GoogleExpirationDate))
	}

	if showUSPTO && showGoogle && res.ComputedExpirationDate != "" && res.GoogleExpirationDate != "" {
		fmt.Println("\n--- Comparison ---")
		tUSPTO, errU := time.Parse("2006-01-02", res.ComputedExpirationDate)
		tGoogle, errG := time.Parse("2006-01-02", res.GoogleExpirationDate)
		if errU == nil && errG == nil {
			diff := tUSPTO.Sub(tGoogle)
			days := int(diff.Hours() / 24)
			if days == 0 {
				fmt.Println("Comparison Result:             Computed USPTO date matches Google date.")
			} else {
				fmt.Printf("Difference (USPTO - Google):   %d days\n", days)
			}
		}
	}
	fmt.Println("=======================================================")
}

func formatStr(s string) string {
	if s == "" {
		return "None"
	}
	return s
}
