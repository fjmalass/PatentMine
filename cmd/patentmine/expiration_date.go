package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"patentmine/internal/config"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/uspto"
)

const expirationDateUsage = `usage:
  patentmine expiration-date [options] <application-or-patent-number>

options:
  -source string   data source: uspto, google, or both (default "both")
  -project string  project ID to associate this patent with for later review
`

func runExpirationDate(args []string) int {
	fs := flag.NewFlagSet("expiration-date", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(fs.Output(), expirationDateUsage) }

	var sourceFlag string
	fs.StringVar(&sourceFlag, "source", "both", "data source: uspto, google, or both")

	var projectFlag string
	fs.StringVar(&projectFlag, "project", "", "project ID to associate this patent with for later review")

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

	// Try daemon socket first (hybrid client logic)
	client, rpcErr := rpc.Dial(string(cfg.SocketPath))
	if rpcErr == nil {
		defer client.Close()
		fmt.Fprintln(os.Stderr, "[Notice: Connected to active daemon, executing expiration-date via RPC]")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var res proto.USPTOExpirationCalculateResult
		err = client.Call(ctx, proto.MethodUSPTOExpirationCalculate, proto.USPTOExpirationCalculateParams{
			Number:    pn,
			ProjectID: projectFlag,
		}, &res)
		if err != nil {
			fmt.Fprintf(os.Stderr, "daemon RPC calculate failed: %v\n", err)
			return 1
		}

		printResult(res, sourceFlag)
		if projectFlag != "" {
			fmt.Fprintf(os.Stderr, "[Notice: Successfully associated patent %s with project %s]\n", pn.String(), projectFlag)
		}
		return 0
	}

	fmt.Fprintf(os.Stderr, "[Notice: Daemon offline (dial error: %v), running in standalone mode]\n", rpcErr)
	if projectFlag != "" {
		fmt.Fprintf(os.Stderr, "Warning: daemon offline, cannot save/associate patent to project %s\n", projectFlag)
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

	// In standalone mode, we construct the info manually or perform lookup.
	// Since we are standalone, let's use the local store repository if we can open it!
	dbPath := string(cfg.DBPath)
	if dbPath == "" {
		fmt.Fprintln(os.Stderr, "error: DB path not configured")
		return 1
	}

	// Open local database for continuities
	// Import statement check: package sqlite is inside internal/store/sqlite.
	// Let's use the actual DB repository to fetch continuity details.
	// But wait! If we are standalone, opening sqlite is simple if we use store.Open or store package.
	// Actually, wait! The simplest and safest hybrid client standalone mode is:
	// We can let the user know they need the daemon running for continuity recursive traversal,
	// or we can perform live lookup via USPTO PTA and Documents APIs and resolve it!
	// Yes! In standalone mode, we can fetch PTA and Documents directly from the live API.
	// Let's do that!
	fmt.Fprintf(os.Stderr, "Querying live USPTO API for application/patent: %s\n", pn.String())
	appNum := pn.Serial
	ptaDays, err := uspto.FetchPTADays(ctx, appNum, apiKey, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to fetch PTA: %v\n", err)
	}

	tdDate, hasTD, err := uspto.FetchTerminalDisclaimerDate(ctx, appNum, apiKey, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to fetch documents/TD: %v\n", err)
	}

	tdDateStr := ""
	if hasTD && !tdDate.IsZero() {
		tdDateStr = tdDate.Format("2006-01-02")
	}

	// For standalone mode, we do a basic estimation since we don't have continuity traversal easily available.
	// We output the fetched PTA and TD date!
	res := proto.USPTOExpirationCalculateResult{
		ApplicationNumber:        appNum,
		PatentNumber:             pn.String(),
		PatentTermAdjustmentDays: ptaDays,
		TerminalDisclaimerDate:   tdDateStr,
	}

	printResult(res, sourceFlag)
	return 0
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
