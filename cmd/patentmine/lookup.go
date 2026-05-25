package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"patentmine/internal/config"
)

const lookupUsage = `usage:
  patentmine lookup <application-number>  look up a patent by USPTO application number
`

type lookupResponse struct {
	Results []lookupResult `json:"results"`
	Count   int            `json:"count"`
}

type lookupResult struct {
	PatentNumberText string `json:"patentNumberText"`
	InventionTitle   string `json:"inventionTitle"`
	AbstractText     string `json:"abstractText"`
	PatentGrantDate  string `json:"patentGrantDate"`
	FilingDate       string `json:"filingDate"`
}

func runLookup(args []string) int {
	fs := flag.NewFlagSet("lookup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(fs.Output(), lookupUsage) }
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

	appNum := strings.TrimSpace(fs.Arg(0))
	if appNum == "" {
		fmt.Fprintln(os.Stderr, "patentmine lookup: application number is required")
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}

	apiKey := strings.TrimSpace(cfg.USPTOAPIKey)
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "patentmine lookup: USPTO API key not configured (set PATENTMINE_USPTO_API_KEY)")
		return 1
	}

	resolved, err := config.ResolveAPIKey(apiKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "patentmine lookup: %s\n", err)
		return 1
	}
	apiKey = resolved

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	url := fmt.Sprintf("https://api.uspto.gov/api/v1/patent/applications/search?patentApplicationNumber=%s", appNum)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "patentmine lookup: %s\n", err)
		return 1
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "patentmine lookup: %s\n", err)
		return 1
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		fmt.Fprintf(os.Stderr, "patentmine lookup: read response: %s\n", err)
		return 1
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "patentmine lookup: USPTO returned HTTP %d: %s\n", resp.StatusCode, compactBody(body))
		return 1
	}

	var pretty json.RawMessage
	if err := json.Unmarshal(body, &pretty); err != nil {
		fmt.Fprintf(os.Stderr, "patentmine lookup: bad JSON: %s\nraw:\n%s\n", err, body)
		return 1
	}
	formatted, _ := json.MarshalIndent(pretty, "", "  ")
	fmt.Println(string(formatted))
	return 0
}
