package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"patentmine/internal/config"
	"patentmine/internal/domain"
	"patentmine/internal/engine"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/store/sqlite"
	"patentmine/internal/uspto"
	"patentmine/internal/version"
)

const expirationDateUsage = `usage:
  patentmine expiration-date [options] <application-or-patent-number>

options:
  -source string   data source: uspto, google, or both (default "both")
  -refresh         force refresh application metadata from USPTO
`

func runExpirationDate(args []string) int {
	fs := flag.NewFlagSet("expiration-date", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(fs.Output(), expirationDateUsage) }

	var sourceFlag string
	var refreshFlag bool
	fs.StringVar(&sourceFlag, "source", "both", "data source: uspto, google, or both")
	fs.BoolVar(&refreshFlag, "refresh", false, "force refresh application metadata from USPTO")

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

	// Try daemon socket first for consistent behavior
	if client, dErr := rpc.Dial(string(cfg.SocketPath)); dErr == nil {
		defer client.Close()
		reportDaemonVersion(client)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var res proto.USPTOExpirationCalculateResult
		if err := client.Call(ctx, proto.MethodUSPTOExpirationCalculate,
			proto.USPTOExpirationCalculateParams{Number: pn, Refresh: refreshFlag}, &res); err != nil {
			fmt.Fprintf(os.Stderr, "patentmine expiration-date: daemon RPC failed: %v\n", err)
			return 1
		}

		printResult(res, sourceFlag)
		return 0
	}

	// Fall back to standalone mode — use the engine directly (same logic as daemon)
	apiKey := strings.TrimSpace(cfg.USPTOAPIKey)
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "patentmine expiration-date: USPTO API key not configured (set PATENTMINE_USPTO_API_KEY)")
		return 1
	}
	resolved, err := config.ResolveAPIKey(apiKey)
	if err == nil {
		apiKey = resolved
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	repo, err := sqlite.Open(ctx, string(cfg.DBPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open database: %v\n", err)
		return 1
	}
	defer repo.Close()

	eng := engine.New(ctx, repo, nil,
		engine.WithUSPTOAPIKey(apiKey),
		engine.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))),
	)
	defer eng.Close()

	// Use the same ComputeAndStoreUSPTOExpiration pipeline as the daemon
	app, err := eng.ComputeAndStoreUSPTOExpiration(ctx, pn, refreshFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to compute expiration: %v\n", err)
		return 1
	}

	// Build result proto (mirrors server.go:usptoExpirationCalculate)
	var googleExpStr, grantDateStr, invTitle, inventors string
	if patentRec, pErr := repo.Patent(ctx, pn); pErr == nil {
		invTitle = patentRec.Title
		var invs []string
		for _, inv := range patentRec.Inventors {
			invs = append(invs, string(inv))
		}
		inventors = strings.Join(invs, ", ")
		if !patentRec.ExpirationDate.IsZero() {
			googleExpStr = patentRec.ExpirationDate.Format("2006-01-02")
		}
		if !patentRec.GrantDate.IsZero() {
			grantDateStr = patentRec.GrantDate.Format("2006-01-02")
		}
	}
	if invTitle == "" {
		invTitle = app.InventionTitle
	}
	if inventors == "" {
		inventors = app.FirstInventorName
	}
	if grantDateStr == "" {
		grantDateStr = uspto.GrantDateFromStatus(app.ApplicationStatusText, app.ApplicationStatusDate)
	}

	res := proto.USPTOExpirationCalculateResult{
		ApplicationNumber:        app.ApplicationNumber,
		PatentNumber:             pn.String(),
		Title:                    invTitle,
		Inventors:                inventors,
		FilingDate:               app.FilingDate,
		GrantDate:                grantDateStr,
		EarliestTermFilingDate:   app.EarliestTermFilingDate,
		PatentTermAdjustmentDays: app.PatentTermAdjustmentDays,
		PatentTermExtensionDays:  app.PatentTermExtension,
		TerminalDisclaimerDate:   app.TerminalDisclaimerDate,
		ComputedExpirationDate:   app.ComputedExpirationDate,
		GoogleExpirationDate:     googleExpStr,
		ComputedAt:               app.FetchedAt,
	}

	// Populate earliest-term parent info when the filing date comes from a parent patent.
	if app.EarliestTermAppNum != "" && app.EarliestTermAppNum != app.ApplicationNumber {
		res.EarliestTermAppNum = app.EarliestTermAppNum
		if parentApp, pErr := eng.USPTOApplicationOrFetch(ctx, app.EarliestTermAppNum); pErr == nil {
			grantNum, grantDate := uspto.EarliestTermSource(parentApp)
			if grantNum == "" {
				if fresh, ferr := eng.RefreshUSPTOApplicationByAppNum(ctx, app.EarliestTermAppNum); ferr == nil {
					parentApp = fresh
					grantNum, grantDate = uspto.EarliestTermSource(parentApp)
				}
			}
			res.EarliestTermPatentNumber = grantNum
			res.EarliestTermGrantDate = grantDate
			if res.EarliestTermGrantDate == "" {
				res.EarliestTermGrantDate = uspto.GrantDateFromStatus(parentApp.ApplicationStatusText, parentApp.ApplicationStatusDate)
			}
			res.EarliestTermTitle = parentApp.InventionTitle
			res.EarliestTermInventors = parentApp.FirstInventorName
		}
	}

	printResult(res, sourceFlag)
	return 0
}

// reportDaemonVersion pings the daemon and prints its build version, warning
// when it differs from this CLI's version — a mismatch means the daemon is
// running older code and must be restarted to pick up recent changes.
func reportDaemonVersion(client *rpc.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var pong proto.PingResult
	if err := client.Call(ctx, proto.MethodPing, nil, &pong); err != nil {
		fmt.Fprintln(os.Stderr, "[Connected to active daemon (version unknown), executing expiration calculation via RPC]")
		return
	}
	fmt.Fprintf(os.Stderr, "[Connected to active daemon — version: %s]\n", pong.Version)
	if cli := version.String(); pong.Version != cli {
		fmt.Fprintf(os.Stderr, "[WARNING: CLI version %q differs from daemon %q — run `cargo make restart-daemon` to load the latest code]\n", cli, pong.Version)
	}
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
		if res.EarliestTermAppNum != "" {
			fmt.Printf("  ↳ Source Application:          %s\n", res.EarliestTermAppNum)
			if res.EarliestTermPatentNumber != "" {
				fmt.Printf("  ↳ Source Grant:                %s\n", res.EarliestTermPatentNumber)
			}
			if res.EarliestTermGrantDate != "" {
				fmt.Printf("  ↳ Source Grant Date:           %s\n", res.EarliestTermGrantDate)
			}
			if res.EarliestTermInventors != "" {
				fmt.Printf("  ↳ Source Inventors:            %s\n", res.EarliestTermInventors)
			}
			if res.EarliestTermTitle != "" {
				fmt.Printf("  ↳ Source Title:                %s\n", res.EarliestTermTitle)
			}
		}
		fmt.Printf("Patent Term Adjustment (PTA):  %d days\n", res.PatentTermAdjustmentDays)
		fmt.Printf("Patent Term Extension (PTE):   %d days\n", res.PatentTermExtensionDays)
		fmt.Printf("Terminal Disclaimer Date:      %s\n", formatStr(res.TerminalDisclaimerDate))
		fmt.Printf("Computed Expiration Date:      %s\n", formatStr(res.ComputedExpirationDate))

		computedOn := "None"
		if res.ComputedAt != "" {
			if t, err := time.Parse(time.RFC3339, res.ComputedAt); err == nil {
				computedOn = t.Local().Format("2006-01-02 15:04:05")
			} else {
				computedOn = res.ComputedAt
			}
		}
		fmt.Printf("Computed On:                   %s\n", computedOn)
	}

	if showGoogle {
		fmt.Println("\n--- Google Patents Data ---")
		fmt.Printf("Parsed Expiration Date:        %s\n", formatStr(res.GoogleExpirationDate))
	}

	if showUSPTO && showGoogle {
		if line := res.ComparisonLine(); line != "" {
			fmt.Println("\n--- Comparison ---")
			fmt.Printf("Comparison Result:             %s\n", line)
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
