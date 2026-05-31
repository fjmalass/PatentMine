package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"patentmine/internal/config"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
)

const lookupUsage = `usage:
  patentmine lookup [options] <application-number>  look up a patent by USPTO application number

options:
  -zip  download and unzip associated patent documents into the configured patents (XML) cache dir
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

type wrapperData struct {
	ApplicationNumberText string        `json:"applicationNumberText"`
	GrantDocumentMetaData *documentMeta `json:"grantDocumentMetaData"`
	PGPubDocumentMetaData *documentMeta `json:"pgpubDocumentMetaData"`
}

type documentMeta struct {
	ZipFileName     string `json:"zipFileName"`
	XMLFileName     string `json:"xmlFileName"`
	FileLocationURI string `json:"fileLocationURI"`
}

type fileWrapperResponse struct {
	PatentFileWrapperDataBag []wrapperData `json:"patentFileWrapperDataBag"`
}

func runLookup(args []string) int {
	fs := flag.NewFlagSet("lookup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(fs.Output(), lookupUsage) }

	var zipFlag bool
	fs.BoolVar(&zipFlag, "zip", false, "download and unzip associated patent documents into ~/.config/patentmine/patents/")

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
		fmt.Fprintln(os.Stderr, "patentmine lookup: number is required")
		return 2
	}

	var appNum string
	if parsed, err := domain.ParsePatentNumber(rawNum); err == nil {
		appNum = parsed.Serial
	} else {
		appNum = rawNum
	}

	cfg, err := config.Load()
	if err != nil {
		return fail(err)
	}

	// Try daemon socket first (hybrid client logic)
	if client, err := rpc.Dial(string(cfg.SocketPath)); err == nil {
		defer client.Close()
		fmt.Fprintln(os.Stderr, "[Notice: Connected to active daemon, executing lookup via RPC]")
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()

		var pn domain.PatentNumber
		if parsed, err := domain.ParsePatentNumber(rawNum); err == nil {
			pn = parsed
		} else {
			pn = domain.PatentNumber{Serial: appNum}
		}

		var res proto.USPTOLookupResult
		err = client.Call(ctx, proto.MethodUSPTOLookup, proto.USPTOLookupParams{Number: pn}, &res)
		if err != nil {
			fmt.Fprintf(os.Stderr, "patentmine lookup: daemon RPC lookup failed: %v\n", err)
			return 1
		}

		var pretty json.RawMessage
		if err := json.Unmarshal([]byte(res.RawJSON), &pretty); err != nil {
			fmt.Fprintf(os.Stderr, "patentmine lookup: daemon returned invalid JSON: %v\n", err)
			return 1
		}
		formatted, _ := json.MarshalIndent(pretty, "", "  ")
		fmt.Println(string(formatted))

		if zipFlag {
			fmt.Fprintln(os.Stderr, "\n[ZIP Option Enabled] Extracting document download URLs from metadata...")
			var respData fileWrapperResponse
			if err := json.Unmarshal([]byte(res.RawJSON), &respData); err != nil {
				fmt.Fprintf(os.Stderr, "patentmine lookup: failed to parse response for ZIP download: %s\n", err)
				return 1
			}

			var metas []*documentMeta
			for _, w := range respData.PatentFileWrapperDataBag {
				if w.GrantDocumentMetaData != nil && w.GrantDocumentMetaData.FileLocationURI != "" {
					metas = append(metas, w.GrantDocumentMetaData)
				}
				if w.PGPubDocumentMetaData != nil && w.PGPubDocumentMetaData.FileLocationURI != "" {
					metas = append(metas, w.PGPubDocumentMetaData)
				}
			}

			if len(metas) == 0 {
				fmt.Fprintln(os.Stderr, "patentmine lookup: no associated document ZIP URLs found in response")
				return 1
			}

			patentsDir := string(cfg.PatentsDir)
			apiKey := strings.TrimSpace(cfg.USPTOAPIKey)
			if apiKey != "" {
				resolved, err := config.ResolveAPIKey(apiKey)
				if err == nil {
					apiKey = resolved
				}
			}

			httpClient := &http.Client{}
			for _, meta := range metas {
				fmt.Fprintf(os.Stderr, "Downloading associated document: %s\n", meta.XMLFileName)
				if err := downloadAndSaveDocument(ctx, meta.FileLocationURI, meta.ZipFileName, meta.XMLFileName, patentsDir, httpClient, apiKey); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to download/save %s: %s\n", meta.XMLFileName, err)
					return 1
				}
				fmt.Fprintf(os.Stderr, "Successfully saved %s to %s\n", meta.XMLFileName, patentsDir)
			}
		}
		return 0
	}

	fmt.Fprintln(os.Stderr, "[Notice: Daemon offline, running in standalone mode]")

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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	appNum = strings.TrimSpace(appNum)
	var query string
	if rawNum != "" && rawNum != appNum {
		query = fmt.Sprintf("applicationNumberText:%s OR applicationMetaData.patentNumber:%s OR applicationMetaData.publicationNumber:%s OR %q OR %q",
			appNum, appNum, appNum, rawNum, appNum)
	} else {
		query = fmt.Sprintf("applicationNumberText:%s OR applicationMetaData.patentNumber:%s OR applicationMetaData.publicationNumber:%s OR %q",
			appNum, appNum, appNum, appNum)
	}
	url := fmt.Sprintf("https://api.uspto.gov/api/v1/patent/applications/search?q=%s", strings.ReplaceAll(url.QueryEscape(query), "+", "%20"))
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

	if zipFlag {
		fmt.Fprintln(os.Stderr, "\n[ZIP Option Enabled] Extracting document download URLs from metadata...")
		var respData fileWrapperResponse
		if err := json.Unmarshal(body, &respData); err != nil {
			fmt.Fprintf(os.Stderr, "patentmine lookup: failed to parse response for ZIP download: %s\n", err)
			return 1
		}

		var metas []*documentMeta
		for _, w := range respData.PatentFileWrapperDataBag {
			if w.GrantDocumentMetaData != nil && w.GrantDocumentMetaData.FileLocationURI != "" {
				metas = append(metas, w.GrantDocumentMetaData)
			}
			if w.PGPubDocumentMetaData != nil && w.PGPubDocumentMetaData.FileLocationURI != "" {
				metas = append(metas, w.PGPubDocumentMetaData)
			}
		}

		if len(metas) == 0 {
			fmt.Fprintln(os.Stderr, "patentmine lookup: no associated document ZIP URLs found in response")
			return 1
		}

		patentsDir := string(cfg.PatentsDir)
		// Already created by config.Load; MkdirAll is safe but unnecessary here.

		for _, meta := range metas {
			fmt.Fprintf(os.Stderr, "Downloading associated document: %s\n", meta.XMLFileName)
			if err := downloadAndSaveDocument(ctx, meta.FileLocationURI, meta.ZipFileName, meta.XMLFileName, patentsDir, client, apiKey); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to download/save %s: %s\n", meta.XMLFileName, err)
				return 1
			}
			fmt.Fprintf(os.Stderr, "Successfully saved %s to %s\n", meta.XMLFileName, patentsDir)
		}
	}

	return 0
}

func downloadAndSaveDocument(ctx context.Context, downloadURL, zipName, xmlName, patentsDir string, client *http.Client, apiKey string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("server returned HTTP %d: %s", resp.StatusCode, string(errBody))
	}

	// patentsDir is pre-created by config; MkdirAll omitted for brevity.
	isZip := strings.HasSuffix(strings.ToLower(downloadURL), ".zip")
	if isZip {
		tmpFile, err := os.CreateTemp("", "patentmine-*.zip")
		if err != nil {
			return err
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		_, err = io.Copy(tmpFile, resp.Body)
		if err != nil {
			return err
		}
		tmpFile.Close()

		zr, err := zip.OpenReader(tmpFile.Name())
		if err != nil {
			return err
		}
		defer zr.Close()

		for _, f := range zr.File {
			cleaned := filepath.Clean(f.Name)
			if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, "/") || strings.Contains(cleaned, "\\") {
				continue
			}

			destPath := filepath.Join(patentsDir, cleaned)
			fmt.Fprintf(os.Stderr, "  -> Extracting file: %s\n", cleaned)

			rc, err := f.Open()
			if err != nil {
				return err
			}

			out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				rc.Close()
				return err
			}

			_, err = io.Copy(out, rc)
			out.Close()
			rc.Close()
			if err != nil {
				return err
			}
		}
	} else {
		filename := xmlName
		if filename == "" {
			u, err := url.Parse(downloadURL)
			if err == nil {
				parts := strings.Split(u.Path, "/")
				if len(parts) > 0 {
					filename = parts[len(parts)-1]
				}
			}
		}
		if filename == "" {
			filename = "patent.xml"
		}

		destPath := filepath.Join(patentsDir, filename)
		fmt.Fprintf(os.Stderr, "  -> Saving direct XML file: %s\n", filename)

		out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		defer out.Close()

		_, err = io.Copy(out, resp.Body)
		if err != nil {
			return err
		}
	}

	return nil
}
