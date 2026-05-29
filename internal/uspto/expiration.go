package uspto

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

// PatentExpiration computes a rough U.S. patent expiration date.
func PatentExpiration(
	patentType string,
	filingDate time.Time,
	grantDate time.Time,
	earliestTermFilingDate time.Time,
	patentTermAdjustmentDays int,
	patentTermExtensionDays int,
	terminalDisclaimerDate time.Time,
) time.Time {
	if filingDate.IsZero() && earliestTermFilingDate.IsZero() && grantDate.IsZero() {
		return time.Time{}
	}

	var exp time.Time
	if patentType == "utility" || patentType == "plant" {
		base := filingDate
		if !earliestTermFilingDate.IsZero() {
			base = earliestTermFilingDate
		}
		// 20 years from earliest term filing date
		exp = base.AddDate(20, 0, 0)
		// Add PTA and PTE days
		exp = exp.AddDate(0, 0, patentTermAdjustmentDays+patentTermExtensionDays)
	} else if patentType == "design" {
		// Modern rule: 15 years from grant date.
		exp = grantDate.AddDate(15, 0, 0)
	} else {
		// Default to utility if unsupported type
		base := filingDate
		if !earliestTermFilingDate.IsZero() {
			base = earliestTermFilingDate
		}
		exp = base.AddDate(20, 0, 0)
		exp = exp.AddDate(0, 0, patentTermAdjustmentDays+patentTermExtensionDays)
	}

	if !terminalDisclaimerDate.IsZero() {
		if exp.IsZero() || terminalDisclaimerDate.Before(exp) {
			exp = terminalDisclaimerDate
		}
	}

	return exp
}

// FetchPTADays calls the USPTO PTA API and returns the total PTA days.
// Includes metrics and telemetry tracing.
func FetchPTADays(ctx context.Context, appNum string, apiKey string, logger *slog.Logger) (ptaDays int, err error) {
	appNum = strings.TrimSpace(appNum)
	if appNum == "" {
		return 0, fmt.Errorf("empty application number")
	}

	startTime := time.Now()
	defer func() {
		dur := time.Since(startTime)
		if logger != nil {
			logger.Info("uspto pta api fetch complete",
				slog.String("app_num", appNum),
				slog.Duration("duration", dur),
				slog.Int("pta_days", ptaDays),
				slog.Bool("success", err == nil),
			)
		}
	}()

	apiURL := fmt.Sprintf("https://api.uspto.gov/api/v1/patent/applications/%s/patent-term-adjustment", appNum)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("X-API-KEY", apiKey)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return 0, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("USPTO PTA API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Parse JSON
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		// Also try list
		var list []map[string]any
		if errList := json.Unmarshal(body, &list); errList == nil && len(list) > 0 {
			data = list[0]
		} else {
			return 0, fmt.Errorf("failed to parse PTA JSON: %w", err)
		}
	}

	// Extract the quantity
	if q, ok := data["patentTermAdjustmentQuantity"].(float64); ok {
		return int(q), nil
	}
	// Fallbacks
	for _, key := range []string{"patentTermAdjustmentDays", "totalDays", "adjustmentQuantity", "days"} {
		if q, ok := data[key].(float64); ok {
			return int(q), nil
		}
	}

	return 0, nil
}

const (
	DocCodeTerminalDisclaimerDIST      = "DIST"
	DocCodeTerminalDisclaimerDISTEFile = "DIST.E.FILE"
	DocCodeTerminalDisclaimerDISC      = "DISC"
	DocCodeTerminalDisclaimerTRMD      = "TRMD"
	DocCodeTerminalDisclaimerDISE      = "DIS.E"
)

// terminalDisclaimerCodes defines the dictionary of USPTO document codes representing terminal disclaimers.
var terminalDisclaimerCodes = map[string]bool{
	DocCodeTerminalDisclaimerDIST:      true,
	DocCodeTerminalDisclaimerDISTEFile: true,
	DocCodeTerminalDisclaimerDISC:      true,
	DocCodeTerminalDisclaimerTRMD:      true,
	DocCodeTerminalDisclaimerDISE:      true,
}

// FetchTerminalDisclaimerDate calls the USPTO Documents API and checks for terminal disclaimers.
// Returns the date of the earliest terminal disclaimer, whether a disclaimer was found, and error.
// Includes metrics and telemetry tracing.
func FetchTerminalDisclaimerDate(ctx context.Context, appNum string, apiKey string, logger *slog.Logger) (tdDate time.Time, found bool, err error) {
	appNum = strings.TrimSpace(appNum)
	if appNum == "" {
		return time.Time{}, false, fmt.Errorf("empty application number")
	}

	startTime := time.Now()
	defer func() {
		dur := time.Since(startTime)
		if logger != nil {
			logger.Info("uspto documents api fetch complete",
				slog.String("app_num", appNum),
				slog.Duration("duration", dur),
				slog.Bool("found_td", found),
				slog.String("td_date", tdDate.Format("2006-01-02")),
				slog.Bool("success", err == nil),
			)
		}
	}()

	apiURL := fmt.Sprintf("https://api.uspto.gov/api/v1/patent/applications/%s/documents", appNum)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return time.Time{}, false, err
	}
	req.Header.Set("X-API-KEY", apiKey)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return time.Time{}, false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return time.Time{}, false, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return time.Time{}, false, fmt.Errorf("USPTO Documents API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Parse JSON
	var docResponse struct {
		DocumentBag []struct {
			DocumentCode                string `json:"documentCode"`
			DocumentCodeDescriptionText string `json:"documentCodeDescriptionText"`
			MailDate                    string `json:"mailDate"`
			FilingDate                  string `json:"filingDate"`
			DocumentDate                string `json:"documentDate"`
			OfficialDate                string `json:"officialDate"`
			MailRoomDate                string `json:"mailRoomDate"`
		} `json:"documentBag"`
	}

	if err := json.Unmarshal(body, &docResponse); err != nil {
		return time.Time{}, false, fmt.Errorf("failed to parse Documents JSON: %w", err)
	}

	var earliestDate time.Time

	for _, doc := range docResponse.DocumentBag {
		code := strings.ToUpper(strings.TrimSpace(doc.DocumentCode))
		desc := strings.ToLower(doc.DocumentCodeDescriptionText)

		// Check if it's a terminal disclaimer
		isTD := terminalDisclaimerCodes[code] || strings.Contains(desc, "terminal disclaimer")

		if isTD {
			found = true
			// Try to parse the date from MailDate, OfficialDate, MailRoomDate, FilingDate, then DocumentDate
			for _, dateStr := range []string{doc.MailDate, doc.OfficialDate, doc.MailRoomDate, doc.FilingDate, doc.DocumentDate} {
				if t, err := parseUSPTODate(dateStr); err == nil && !t.IsZero() {
					if earliestDate.IsZero() || t.Before(earliestDate) {
						earliestDate = t
					}
					break
				}
			}
		}
	}

	return earliestDate, found, nil
}

// ContinuityWalkStats summarizes a continuity chain walk for telemetry.
type ContinuityWalkStats struct {
	// MaxDepth is the deepest recursion level reached (0 = no parents found).
	MaxDepth int
	// TotalSteps is the total number of recursive calls made, including revisits
	// that were short-circuited by cycle detection.
	TotalSteps int
	// UniqueApps is the number of distinct application numbers processed.
	UniqueApps int
	// EarliestApp is the normalized application number that contributed the
	// earliest term filing date. Equals the root appNum when no parent is earlier.
	EarliestApp string
}

// ComputeEarliestTermFilingDate finds the earliest term filing date under 35 U.S.C. 154
// by recursively walking the non-provisional parent continuity chain.
// Returns the earliest filing date, walk telemetry stats, and error.
func ComputeEarliestTermFilingDate(
	ctx context.Context,
	repo store.Repository,
	appNum string,
	filingDate time.Time,
	logger *slog.Logger,
) (time.Time, ContinuityWalkStats, error) {
	startTime := time.Now()
	visited := make(map[string]bool) // cycle detection: marks nodes currently on the DFS stack
	seen := make(map[string]bool)    // telemetry: all-time unique app numbers encountered
	stats := ContinuityWalkStats{EarliestApp: normalizeAppNum(appNum)}
	earliest, err := walkContinuityChain(ctx, repo, appNum, filingDate, visited, seen, 0, &stats, logger)
	dur := time.Since(startTime)

	if logger != nil {
		logger.Info("continuity walk completed",
			slog.String("app_num", appNum),
			slog.Duration("duration", dur),
			slog.String("earliest_date", earliest.Format("2006-01-02")),
			slog.Int("max_depth", stats.MaxDepth),
			slog.Int("total_steps", stats.TotalSteps),
			slog.Int("unique_apps", stats.UniqueApps),
			slog.Bool("success", err == nil),
		)
	}
	return earliest, stats, err
}

func walkContinuityChain(
	ctx context.Context,
	repo store.Repository,
	appNum string,
	currentFilingDate time.Time,
	visited map[string]bool,
	seen map[string]bool,
	depth int,
	stats *ContinuityWalkStats,
	logger *slog.Logger,
) (time.Time, error) {
	appNum = normalizeAppNum(appNum)
	if appNum == "" || visited[appNum] {
		return currentFilingDate, nil
	}

	stats.TotalSteps++
	if depth > stats.MaxDepth {
		stats.MaxDepth = depth
	}
	if !seen[appNum] {
		seen[appNum] = true
		stats.UniqueApps++
	}

	if logger != nil {
		logger.Debug("continuity walk step",
			slog.String("app", appNum),
			slog.Int("depth", depth),
		)
	}

	visited[appNum] = true
	defer delete(visited, appNum)

	// Query continuities for this appNum
	continuities, err := repo.USPTOContinuities(ctx, appNum)
	if err != nil {
		// Non-fatal, return what we have so far
		return currentFilingDate, nil
	}

	earliest := currentFilingDate

	for _, c := range continuities {
		parentApp := normalizeAppNum(c.ParentApplicationNumberText)
		if parentApp == "" {
			continue
		}

		// Skip provisional applications
		if isProvisional(parentApp, c.ClaimParentageTypeCode, c.ClaimParentageTypeDescriptionText) {
			continue
		}

		parentFilingDate, err := parseUSPTODate(c.ParentApplicationFilingDate)
		if err != nil || parentFilingDate.IsZero() {
			continue
		}

		if earliest.IsZero() || parentFilingDate.Before(earliest) {
			earliest = parentFilingDate
			stats.EarliestApp = parentApp
		}

		// Recursively walk parent; deeper calls update stats.EarliestApp via shared pointer
		// when they find a date earlier than this one.
		parentEarliest, err := walkContinuityChain(ctx, repo, parentApp, parentFilingDate, visited, seen, depth+1, stats, logger)
		if err == nil && !parentEarliest.IsZero() && (earliest.IsZero() || parentEarliest.Before(earliest)) {
			earliest = parentEarliest
		}
	}

	return earliest, nil
}

func normalizeAppNum(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	return strings.ReplaceAll(strings.ReplaceAll(s, "/", ""), ",", "")
}

func isProvisional(parentApp, code, desc string) bool {
	appClean := normalizeAppNum(parentApp)
	if strings.HasPrefix(appClean, "60") || strings.HasPrefix(appClean, "61") || strings.HasPrefix(appClean, "62") {
		return true
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	desc = strings.ToLower(desc)
	if code == "PROV" || strings.Contains(desc, "provisional") {
		return true
	}
	return false
}

// GrantDateFromStatus returns the application status date when the status
// text indicates a granted patent, or the empty string otherwise.
func GrantDateFromStatus(statusText, statusDate string) string {
	st := strings.ToLower(strings.TrimSpace(statusText))
	sd := strings.TrimSpace(statusDate)
	if (strings.Contains(st, "patent") || strings.Contains(st, "grant")) && sd != "" {
		return sd
	}
	return ""
}

// ParseGrantDateFromStatus returns the application status date parsed as a
// time.Time when the status text indicates a granted patent, or a zero
// time.Time when the status does not indicate a grant.
func ParseGrantDateFromStatus(statusText, statusDate string) (time.Time, error) {
	s := GrantDateFromStatus(statusText, statusDate)
	if s == "" {
		return time.Time{}, fmt.Errorf("not a granted patent")
	}
	return parseUSPTODate(s)
}

func parseUSPTODate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date string")
	}
	// Try YYYY-MM-DD
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	// Try YYYYMMDD
	if t, err := time.Parse("20060102", s); err == nil {
		return t, nil
	}
	// Try MM-DD-YYYY
	if t, err := time.Parse("01-02-2006", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("unknown date format: %q", s)
}

// DeterminePatentType classifies a patent as utility, design, or plant.
func DeterminePatentType(number domain.PatentNumber, appLabel, appCategory, grantKind string) string {
	label := strings.ToLower(appLabel)
	cat := strings.ToLower(appCategory)
	if strings.Contains(label, "design") || strings.Contains(cat, "design") {
		return "design"
	}
	if strings.Contains(label, "plant") || strings.Contains(cat, "plant") {
		return "plant"
	}

	kind := strings.ToUpper(grantKind)
	if kind == "" {
		kind = strings.ToUpper(number.Kind)
	}
	if strings.HasPrefix(kind, "S") {
		return "design"
	}
	if strings.HasPrefix(kind, "P") && kind != "PL" {
		return "plant"
	}

	return "utility"
}
