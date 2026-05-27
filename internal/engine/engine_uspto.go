package engine

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/store"
	"patentmine/internal/uspto"
)

const usptoXMLFetchTimeout = 60 * time.Second

// FetchUSPTOXML returns the local XML file for the given patent, downloading
// it only when no cached copy exists on disk. The per-document download
// counter in uspto_xml_download is bumped on every call, so callers can see
// how often each document has been requested even when it was served from
// cache. The URL and zip/xml filenames come from the saved uspto_application
// row.
func (e *Engine) FetchUSPTOXML(ctx context.Context, n domain.PatentNumber, kind proto.USPTOXMLKind) (res proto.USPTOFetchXMLResult, err error) {
	start := time.Now()
	defer e.observeDuration("uspto.fetch_xml", start, &err)

	if e.patentsDir == "" {
		return proto.USPTOFetchXMLResult{}, errors.New("engine: patents dir not configured")
	}

	app, err := e.repo.USPTOApplication(ctx, n)
	if err != nil {
		return proto.USPTOFetchXMLResult{}, fmt.Errorf("engine: load uspto application: %w", err)
	}

	downloadURL, xmlName := xmlTarget(app, kind)
	if downloadURL == "" {
		return proto.USPTOFetchXMLResult{}, fmt.Errorf("engine: no %s XML url for %s", kind, n.Normalized())
	}

	prior, priorErr := e.repo.USPTOXMLDownload(ctx, app.ApplicationNumber, string(kind))
	if priorErr != nil && !errors.Is(priorErr, store.ErrNotFound) {
		return proto.USPTOFetchXMLResult{}, fmt.Errorf("engine: lookup xml record: %w", priorErr)
	}

	if priorErr == nil && prior.LocalPath != "" {
		if info, statErr := os.Stat(prior.LocalPath); statErr == nil && info.Size() > 0 {
			// Cached on disk. If we have never parsed it (or the body row
			// has been wiped), parse now so the patent body reflects the XML.
			if _, bodyErr := e.repo.USPTOGrantBody(ctx, app.ApplicationNumber, string(kind)); errors.Is(bodyErr, store.ErrNotFound) {
				if ingestErr := e.ingestUSPTOXML(ctx, n, kind, app.ApplicationNumber, prior.LocalPath); ingestErr != nil {
					e.log(ctx, slog.LevelWarn, "uspto xml ingest (cache) failed",
						slog.String("patent", n.Normalized()),
						slog.String("kind", string(kind)),
						slog.String("path", prior.LocalPath),
						slog.String("error", ingestErr.Error()))
				}
			}
			return e.recordXMLAccess(ctx, prior, n, kind, true)
		}
	}

	apiKey := strings.TrimSpace(e.usptoAPIKey)
	if apiKey == "" {
		return proto.USPTOFetchXMLResult{}, errors.New("engine: USPTO API key not configured")
	}

	if err := os.MkdirAll(e.patentsDir, 0o755); err != nil {
		return proto.USPTOFetchXMLResult{}, fmt.Errorf("engine: create patents dir: %w", err)
	}

	dlCtx, cancel := context.WithTimeout(ctx, usptoXMLFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return proto.USPTOFetchXMLResult{}, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Accept", "*/*")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return proto.USPTOFetchXMLResult{}, fmt.Errorf("engine: download xml: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return proto.USPTOFetchXMLResult{}, fmt.Errorf("engine: download xml: HTTP %d: %s", resp.StatusCode, string(body))
	}

	localPath, bytes, err := saveUSPTOXML(resp.Body, downloadURL, xmlName, e.patentsDir)
	if err != nil {
		return proto.USPTOFetchXMLResult{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	rec := prior
	rec.ApplicationNumber = app.ApplicationNumber
	rec.Kind = string(kind)
	rec.LocalPath = localPath
	rec.Bytes = bytes
	rec.DownloadCount++
	if rec.FirstDownloadedAt == "" {
		rec.FirstDownloadedAt = now
	}
	rec.LastDownloadedAt = now
	rec.LastAccessedAt = now
	if recErr := e.repo.RecordUSPTOXMLDownload(ctx, rec); recErr != nil {
		e.log(ctx, slog.LevelWarn, "uspto xml record write failed",
			slog.String("patent", n.Normalized()),
			slog.String("kind", string(kind)),
			slog.String("error", recErr.Error()))
	}

	if ingestErr := e.ingestUSPTOXML(ctx, n, kind, app.ApplicationNumber, localPath); ingestErr != nil {
		e.log(ctx, slog.LevelWarn, "uspto xml ingest failed",
			slog.String("patent", n.Normalized()),
			slog.String("kind", string(kind)),
			slog.String("path", localPath),
			slog.String("error", ingestErr.Error()))
	}

	e.incCounter("uspto.fetch_xml.bytes", bytes)
	e.incCounter("uspto.fetch_xml.count", 1)
	e.incCounter("uspto.fetch_xml.cache_miss", 1)
	e.log(ctx, slog.LevelInfo, "uspto xml fetched",
		slog.String("patent", n.Normalized()),
		slog.String("kind", string(kind)),
		slog.String("path", localPath),
		slog.Int64("bytes", bytes),
		slog.Int64("download_count", rec.DownloadCount),
		slog.Bool("cached", false))
	e.recordActivity(ctx, observability.Record{
		Component: "engine",
		Action:    "uspto.fetch_xml",
		Entity:    n.Normalized(),
		Status:    "ok",
		Attributes: map[string]any{
			"kind":           string(kind),
			"path":           localPath,
			"bytes":          bytes,
			"download_count": rec.DownloadCount,
			"cached":         false,
		},
	})

	return proto.USPTOFetchXMLResult{
		LocalPath:     localPath,
		Bytes:         bytes,
		Cached:        false,
		DownloadCount: rec.DownloadCount,
	}, nil
}

// ViewUSPTOXML returns a human-readable TOML rendering of the raw grant or
// pgpub XML for the given patent. It ensures the document is present on the
// server (via the normal fetch/cache/ingest path), then parses and converts
// it using the same logic the TUI popup uses. This removes any need for the
// client to open server-side file paths directly.
func (e *Engine) ViewUSPTOXML(ctx context.Context, n domain.PatentNumber, kind proto.USPTOXMLKind) (proto.USPTOXMLViewResult, error) {
	start := time.Now()
	var viewErr error
	defer e.observeDuration("uspto.xml.view", start, &viewErr)

	// Ensure the XML exists on the server (updates download counters, may
	// trigger ingest into the grant body table, etc.).
	fetched, err := e.FetchUSPTOXML(ctx, n, kind)
	if err != nil {
		viewErr = err
		return proto.USPTOXMLViewResult{}, err
	}

	if fetched.LocalPath == "" {
		viewErr = fmt.Errorf("engine: fetch returned no local path for %s %s", kind, n)
		return proto.USPTOXMLViewResult{}, viewErr
	}

	f, err := os.Open(fetched.LocalPath)
	if err != nil {
		viewErr = fmt.Errorf("engine: open cached xml for view: %w", err)
		return proto.USPTOXMLViewResult{}, viewErr
	}
	defer f.Close()

	convertStart := time.Now()
	doc, parseErr := uspto.ParseUSPTOGrantXML(f)
	if parseErr != nil {
		viewErr = fmt.Errorf("engine: parse xml for view: %w", parseErr)
		return proto.USPTOXMLViewResult{}, viewErr
	}

	tomlStr, convErr := uspto.StructToTOML(doc)
	convertDur := time.Since(convertStart)
	if convErr != nil {
		viewErr = fmt.Errorf("engine: convert xml to toml for view: %w", convErr)
		return proto.USPTOXMLViewResult{}, viewErr
	}

	title := fmt.Sprintf("%s XML · %s", strings.ToUpper(string(kind)), n.String())

	e.log(ctx, slog.LevelInfo, "uspto xml view rendered",
		slog.String("patent", n.Normalized()),
		slog.String("kind", string(kind)),
		slog.String("path", fetched.LocalPath),
		slog.Bool("cached", fetched.Cached),
		slog.Int64("bytes", fetched.Bytes))

	return proto.USPTOXMLViewResult{
		TOML:                  tomlStr,
		Title:                 title,
		Kind:                  string(kind),
		Bytes:                 fetched.Bytes,
		Cached:                fetched.Cached,
		DownloadCount:         fetched.DownloadCount,
		ConvertDurationMillis: convertDur.Milliseconds(),
	}, nil
}

// USPTOGrantBody returns the parsed body of the given patent. When kind is
// empty, the engine tries grant first, then pgpub.
func (e *Engine) USPTOGrantBody(ctx context.Context, n domain.PatentNumber, kind proto.USPTOXMLKind) (domain.USPTOGrantBody, bool, error) {
	app, err := e.repo.USPTOApplication(ctx, n)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return domain.USPTOGrantBody{}, false, nil
		}
		return domain.USPTOGrantBody{}, false, err
	}
	tryKinds := []proto.USPTOXMLKind{kind}
	if kind == "" {
		tryKinds = []proto.USPTOXMLKind{proto.USPTOXMLKindGrant, proto.USPTOXMLKindPGPub}
	}
	for _, k := range tryKinds {
		body, err := e.repo.USPTOGrantBody(ctx, app.ApplicationNumber, string(k))
		if err == nil {
			return body, true, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return domain.USPTOGrantBody{}, false, err
		}

		// If not found in SQLite but XML has been downloaded on disk, ingest it now on-demand
		prior, priorErr := e.repo.USPTOXMLDownload(ctx, app.ApplicationNumber, string(k))
		if priorErr == nil && prior.LocalPath != "" {
			if info, statErr := os.Stat(prior.LocalPath); statErr == nil && info.Size() > 0 {
				if ingestErr := e.ingestUSPTOXML(ctx, n, k, app.ApplicationNumber, prior.LocalPath); ingestErr == nil {
					if secondBody, secondErr := e.repo.USPTOGrantBody(ctx, app.ApplicationNumber, string(k)); secondErr == nil {
						return secondBody, true, nil
					}
				}
			}
		}
	}
	return domain.USPTOGrantBody{}, false, nil
}

// recordXMLAccess updates the access timestamp + counter for a cached XML
// and emits the cache-hit observability signals.
func (e *Engine) recordXMLAccess(ctx context.Context, prior domain.USPTOXMLDownload, n domain.PatentNumber, kind proto.USPTOXMLKind, cached bool) (proto.USPTOFetchXMLResult, error) {
	rec := prior
	now := time.Now().UTC().Format(time.RFC3339)
	rec.DownloadCount++
	rec.LastAccessedAt = now
	if recErr := e.repo.RecordUSPTOXMLDownload(ctx, rec); recErr != nil {
		e.log(ctx, slog.LevelWarn, "uspto xml record write failed",
			slog.String("patent", n.Normalized()),
			slog.String("kind", string(kind)),
			slog.String("error", recErr.Error()))
	}
	e.incCounter("uspto.fetch_xml.count", 1)
	e.incCounter("uspto.fetch_xml.cache_hit", 1)
	e.log(ctx, slog.LevelInfo, "uspto xml served from cache",
		slog.String("patent", n.Normalized()),
		slog.String("kind", string(kind)),
		slog.String("path", rec.LocalPath),
		slog.Int64("bytes", rec.Bytes),
		slog.Int64("download_count", rec.DownloadCount),
		slog.Bool("cached", cached))
	e.recordActivity(ctx, observability.Record{
		Component: "engine",
		Action:    "uspto.fetch_xml",
		Entity:    n.Normalized(),
		Status:    "ok",
		Attributes: map[string]any{
			"kind":           string(kind),
			"path":           rec.LocalPath,
			"bytes":          rec.Bytes,
			"download_count": rec.DownloadCount,
			"cached":         cached,
		},
	})
	return proto.USPTOFetchXMLResult{
		LocalPath:     rec.LocalPath,
		Bytes:         rec.Bytes,
		Cached:        cached,
		DownloadCount: rec.DownloadCount,
	}, nil
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
func (e *Engine) ingestUSPTOXML(ctx context.Context, n domain.PatentNumber, kind proto.USPTOXMLKind, applicationNumber, localPath string) error {
	start := time.Now()
	var ingestErr error
	defer e.observeDuration("uspto.xml.ingest", start, &ingestErr)

	e.log(ctx, slog.LevelInfo, "uspto xml parse start",
		slog.String("patent", n.Normalized()),
		slog.String("kind", string(kind)),
		slog.String("path", localPath))

	f, err := os.Open(localPath)
	if err != nil {
		ingestErr = err
		e.incCounter("uspto.xml.parse.error", 1)
		return fmt.Errorf("engine: open xml: %w", err)
	}
	defer f.Close()

	parseStart := time.Now()
	var parseErr error
	doc, parseErr := uspto.ParseUSPTOGrantXML(f)
	e.observeDuration("uspto.xml.parse", parseStart, &parseErr)
	if parseErr != nil {
		ingestErr = parseErr
		e.incCounter("uspto.xml.parse.error", 1)
		e.log(ctx, slog.LevelError, "uspto xml parse failed",
			slog.String("patent", n.Normalized()),
			slog.String("kind", string(kind)),
			slog.String("path", localPath),
			slog.String("error", parseErr.Error()))
		e.recordActivity(ctx, observability.Record{
			Component:  "engine",
			Action:     "uspto.xml.parse",
			Entity:     n.Normalized(),
			Status:     "error",
			Attributes: map[string]any{"kind": string(kind), "error": parseErr.Error()},
		})
		return parseErr
	}
	e.incCounter("uspto.xml.parse.count", 1)

	mapStart := time.Now()
	bundle := uspto.USPTOGrantToIngest(doc, applicationNumber, string(kind))
	e.observeDuration("uspto.xml.map", mapStart, nil)

	saveStart := time.Now()
	var saveErr error
	saveErr = e.repo.SaveUSPTOGrantIngest(ctx, bundle)
	e.observeDuration("uspto.xml.save", saveStart, &saveErr)
	if saveErr != nil {
		ingestErr = saveErr
		e.incCounter("uspto.xml.save.error", 1)
		e.log(ctx, slog.LevelError, "uspto xml save failed",
			slog.String("patent", n.Normalized()),
			slog.String("kind", string(kind)),
			slog.String("error", saveErr.Error()))
		return fmt.Errorf("engine: save grant ingest: %w", saveErr)
	}
	graphCitations, graphErr := e.saveUSPTOCitationGraph(ctx, n, bundle.Citations)
	if graphErr != nil {
		ingestErr = graphErr
		e.incCounter("uspto.xml.save.error", 1)
		e.log(ctx, slog.LevelError, "uspto citation graph save failed",
			slog.String("patent", n.Normalized()),
			slog.String("kind", string(kind)),
			slog.String("error", graphErr.Error()))
		return fmt.Errorf("engine: save citation graph: %w", graphErr)
	}

	// Best-effort enrichment: if this patent record has no classifications yet
	// (common for pure-USPTO source or USPTOOnly mode), lift the IPCR/CPC codes
	// we just parsed from the grant/pgpub XML. They are already normalized by
	// the Code() helpers + the store reader to the same compact form Google
	// uses (e.g. "G06F21/14"), so filters, detail view, K overlay, FTS etc.
	// will all see them immediately.
	if len(bundle.Classifications) > 0 {
		if codes, cerr := e.repo.USPTOGrantClassifications(ctx, applicationNumber, string(kind)); cerr == nil && len(codes) > 0 {
			if p, perr := e.repo.Patent(ctx, n); perr == nil && len(p.Classifications) == 0 {
				p.Classifications = codes
				if serr := e.repo.SavePatent(ctx, p); serr == nil {
					e.incCounter("uspto.xml.ingest.classifications_backfilled", int64(len(codes)))
					e.log(ctx, slog.LevelInfo, "uspto xml classifications backfilled to patent",
						slog.String("patent", n.Normalized()),
						slog.Int("count", len(codes)))

					// Also upgrade any still-unreconciled source_diff rows for
					// "classifications" so that the Source Comparison overlay (which
					// the user is often looking at) will now show the real USPTO
					// values from the XML instead of the empty snapshot from crawl time.
					_ = e.repo.UpdateUnreconciledSourceDiffUSPTOValue(ctx, n, "classifications", strings.Join(codes, ";"))
				}
			}
		}
	}

	e.incCounter("uspto.xml.ingest.count", 1)
	e.incCounter("uspto.xml.ingest.claims", int64(len(bundle.Body.Claims)))
	e.incCounter("uspto.xml.ingest.citations", int64(len(bundle.Citations)))
	e.incCounter("uspto.xml.ingest.citation_relations", int64(graphCitations))
	e.incCounter("uspto.xml.ingest.classifications", int64(len(bundle.Classifications)))
	e.incCounter("uspto.xml.ingest.drawings", int64(len(bundle.Drawings)))
	e.incCounter("uspto.xml.ingest.relations", int64(len(bundle.Relations)))
	e.incCounter("uspto.xml.ingest.description_bytes", int64(len(bundle.Body.DescriptionText)))
	e.incCounter("uspto.xml.ingest.abstract_bytes", int64(len(bundle.Body.AbstractText)))
	e.incCounter("uspto.xml.ingest.claims_bytes", int64(len(bundle.Body.ClaimsText)))
	e.log(ctx, slog.LevelInfo, "uspto xml ingested",
		slog.String("patent", n.Normalized()),
		slog.String("application", applicationNumber),
		slog.String("kind", string(kind)),
		slog.Int("claims", len(bundle.Body.Claims)),
		slog.Int("citations", len(bundle.Citations)),
		slog.Int("citation_relations", graphCitations),
		slog.Int("classifications", len(bundle.Classifications)),
		slog.Int("drawings", len(bundle.Drawings)),
		slog.Int("relations", len(bundle.Relations)),
		slog.Int("description_chars", len(bundle.Body.DescriptionText)),
		slog.Int("abstract_chars", len(bundle.Body.AbstractText)),
		slog.Int("claims_chars", len(bundle.Body.ClaimsText)),
		slog.Duration("parse_ms", time.Since(parseStart)),
		slog.Duration("total_ms", time.Since(start)))
	e.recordActivity(ctx, observability.Record{
		Component: "engine",
		Action:    "uspto.xml.ingest",
		Entity:    n.Normalized(),
		Status:    "ok",
		Attributes: map[string]any{
			"kind":               string(kind),
			"claims":             len(bundle.Body.Claims),
			"citations":          len(bundle.Citations),
			"citation_relations": graphCitations,
			"classifications":    len(bundle.Classifications),
			"drawings":           len(bundle.Drawings),
			"relations":          len(bundle.Relations),
			"abstract_chars":     len(bundle.Body.AbstractText),
			"description_chars":  len(bundle.Body.DescriptionText),
			"claims_chars":       len(bundle.Body.ClaimsText),
		},
	})
	return nil
}

func (e *Engine) saveUSPTOCitationGraph(ctx context.Context, record domain.PatentNumber, citations []domain.USPTOGrantCitation) (int, error) {
	if record.IsZero() || len(citations) == 0 {
		return 0, nil
	}
	batch := store.NodeBatch{}
	seenRelations := map[string]bool{}
	seenStubs := map[string]bool{}

	for _, citation := range citations {
		cited, ok := patentNumberFromUSPTOCitation(citation)
		if !ok || cited.IsZero() {
			continue
		}
		if cited == record {
			continue
		}

		citedRecord, err := e.repo.RecordOf(ctx, cited)
		switch {
		case errors.Is(err, store.ErrNotFound):
			citedRecord = cited
			key := citedRecord.Normalized()
			if key != "" && !seenStubs[key] {
				seenStubs[key] = true
				batch.Stubs = append(batch.Stubs, store.StubRecord{
					Number: citedRecord,
					Stage:  domain.GuessStage(citedRecord),
				})
			}
		case err != nil:
			return 0, err
		}

		if citedRecord.IsZero() || citedRecord == record {
			continue
		}
		key := record.Normalized() + "\x00" + citedRecord.Normalized()
		if seenRelations[key] {
			continue
		}
		seenRelations[key] = true
		batch.Relations = append(batch.Relations, domain.Relation{
			From: record,
			To:   citedRecord,
			Kind: domain.RelationCites,
		})
	}

	if len(batch.Relations) == 0 && len(batch.Stubs) == 0 {
		return 0, nil
	}
	if err := e.repo.SaveNode(ctx, batch); err != nil {
		return 0, err
	}
	return len(batch.Relations), nil
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

func xmlTarget(app domain.USPTOApplication, kind proto.USPTOXMLKind) (string, string) {
	switch kind {
	case proto.USPTOXMLKindPGPub:
		return app.PGPubXMLURL, app.PGPubXMLName
	case proto.USPTOXMLKindGrant:
		return app.PatentGrantXMLURL, app.PatentGrantXMLName
	}
	return "", ""
}

// saveUSPTOXML writes the upstream body to disk under patentsDir. A .zip URL is
// extracted; a direct .xml URL is streamed under xmlName (falling back to the
// URL's basename, then patent.xml). Returns the first written XML file path.
func saveUSPTOXML(body io.Reader, downloadURL, xmlName, patentsDir string) (string, int64, error) {
	if strings.HasSuffix(strings.ToLower(downloadURL), ".zip") {
		return extractZipXML(body, patentsDir)
	}

	filename := xmlName
	if filename == "" {
		if u, perr := url.Parse(downloadURL); perr == nil {
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
	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", 0, fmt.Errorf("engine: write xml: %w", err)
	}
	defer out.Close()
	n, err := io.Copy(out, body)
	if err != nil {
		return "", 0, fmt.Errorf("engine: write xml: %w", err)
	}
	return destPath, n, nil
}

func extractZipXML(body io.Reader, patentsDir string) (string, int64, error) {
	tmp, err := os.CreateTemp("", "patentmine-*.zip")
	if err != nil {
		return "", 0, fmt.Errorf("engine: temp zip: %w", err)
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := io.Copy(tmp, body); err != nil {
		return "", 0, fmt.Errorf("engine: buffer zip: %w", err)
	}
	tmp.Close()

	zr, err := zip.OpenReader(tmp.Name())
	if err != nil {
		return "", 0, fmt.Errorf("engine: open zip: %w", err)
	}
	defer zr.Close()

	var firstXML string
	var total int64
	for _, f := range zr.File {
		cleaned := filepath.Clean(f.Name)
		if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, "/") || strings.Contains(cleaned, "\\") {
			continue
		}
		destPath := filepath.Join(patentsDir, cleaned)
		rc, oerr := f.Open()
		if oerr != nil {
			return "", 0, fmt.Errorf("engine: zip entry %s: %w", cleaned, oerr)
		}
		out, oerr := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if oerr != nil {
			rc.Close()
			return "", 0, fmt.Errorf("engine: write zip entry %s: %w", cleaned, oerr)
		}
		n, cerr := io.Copy(out, rc)
		out.Close()
		rc.Close()
		if cerr != nil {
			return "", 0, fmt.Errorf("engine: copy zip entry %s: %w", cleaned, cerr)
		}
		total += n
		if firstXML == "" && strings.HasSuffix(strings.ToLower(cleaned), ".xml") {
			firstXML = destPath
		}
	}
	if firstXML == "" {
		return "", total, errors.New("engine: zip contained no xml")
	}
	return firstXML, total, nil
}

// autoFetchUSPTOXMLAfterCrawl waits for the source-specific USPTO crawl job
// to finish, then auto-fetches the grant XML when one is available, falling
// back to the pre-grant publication XML otherwise. Runs in its own goroutine
// — failures are logged but never propagated, since this is a courtesy on
// top of a successful add.
func (e *Engine) autoFetchUSPTOXMLAfterCrawl(record domain.PatentNumber, id JobID) {
	ch, unsub := e.pool.bus.Subscribe()
	defer unsub()
	for ev := range ch {
		if proto.EventKind(ev.Method) != proto.EventCrawlDone {
			continue
		}
		var d proto.CrawlDone
		if err := json.Unmarshal(ev.Params, &d); err != nil || d.JobID != string(id) {
			continue
		}
		if d.Error != "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*usptoXMLFetchTimeout)
		defer cancel()
		app, err := e.repo.USPTOApplication(ctx, record)
		if err != nil {
			e.log(ctx, slog.LevelDebug, "auto xml fetch skipped: no uspto app row",
				slog.String("record", record.String()), slog.String("error", err.Error()))
			return
		}
		kind := pickAutoFetchKind(app)
		if kind == "" {
			e.log(ctx, slog.LevelDebug, "auto xml fetch skipped: no xml urls",
				slog.String("record", record.String()))
			return
		}
		e.incCounter("uspto.auto_fetch_xml.attempt", 1)
		e.log(ctx, slog.LevelInfo, "auto xml fetch starting after add",
			slog.String("record", record.String()),
			slog.String("kind", string(kind)))
		if _, ferr := e.FetchUSPTOXML(ctx, record, kind); ferr != nil {
			e.incCounter("uspto.auto_fetch_xml.error", 1)
			e.log(ctx, slog.LevelWarn, "auto xml fetch failed",
				slog.String("record", record.String()),
				slog.String("kind", string(kind)),
				slog.String("error", ferr.Error()))
			return
		}
		e.incCounter("uspto.auto_fetch_xml.ok", 1)
		return
	}
}

// pickAutoFetchKind returns "grant" when a granted XML URL is on record,
// "pgpub" when only the pre-grant publication has one, and "" when neither
// is set.
func pickAutoFetchKind(app domain.USPTOApplication) proto.USPTOXMLKind {
	if strings.TrimSpace(app.PatentGrantXMLURL) != "" {
		return proto.USPTOXMLKindGrant
	}
	if strings.TrimSpace(app.PGPubXMLURL) != "" {
		return proto.USPTOXMLKindPGPub
	}
	return ""
}

// USPTOApplicationFor returns the saved USPTO application row for one patent
// record. Thin wrapper over repo.USPTOApplication that the RPC layer uses to
// resolve a patent to its application_number before calling assignment
// list/save endpoints.
func (e *Engine) USPTOApplicationFor(ctx context.Context, n domain.PatentNumber) (domain.USPTOApplication, error) {
	return e.repo.USPTOApplication(ctx, n)
}

// USPTOGrantClassifications returns the normalized CPC/IPC codes extracted
// from a previously ingested grant or pgpub XML (if any). It is the same data
// that gets automatically back-filled into the patent record on ingest when
// the patent had none. Exposed for future UI "refresh from XML" actions and
// for observability.
func (e *Engine) USPTOGrantClassifications(ctx context.Context, applicationNumber, kind string) ([]string, error) {
	return e.repo.USPTOGrantClassifications(ctx, applicationNumber, kind)
}

// USPTOAssignments returns the persisted assignment chain for one
// application number.
func (e *Engine) USPTOAssignments(ctx context.Context, applicationNumber string) ([]domain.USPTOAssignment, error) {
	return e.repo.USPTOAssignments(ctx, applicationNumber)
}

// FetchUSPTOAssignments calls the USPTO Patent Assignment Search API for the
// patent's application number and persists every recorded assignment plus its
// parties. Existing rows for the application are replaced (a re-fetch
// reflects the current chain). Returns the saved count and the patent's
// canonical record number.
func (e *Engine) FetchUSPTOAssignments(ctx context.Context, n domain.PatentNumber) (res proto.USPTOFetchAssignmentsResult, err error) {
	defer e.observeDuration("uspto.fetch_assignments", time.Now(), &err)
	apiKey := strings.TrimSpace(e.usptoAPIKey)
	if apiKey == "" {
		return proto.USPTOFetchAssignmentsResult{}, errors.New("engine: USPTO API key not configured")
	}
	app, err := e.repo.USPTOApplication(ctx, n)
	if err != nil {
		return proto.USPTOFetchAssignmentsResult{}, fmt.Errorf("engine: load uspto application: %w", err)
	}
	if app.ApplicationNumber == "" {
		return proto.USPTOFetchAssignmentsResult{}, errors.New("engine: no application number on record")
	}
	if e.usptoAssignments == nil {
		return proto.USPTOFetchAssignmentsResult{}, errors.New("engine: USPTO assignment fetcher not configured")
	}
	assignments, err := e.usptoAssignments(ctx, apiKey, app.ApplicationNumber)
	if err != nil {
		e.incCounter("uspto.fetch_assignments.error", 1)
		return proto.USPTOFetchAssignmentsResult{}, err
	}
	if saveErr := e.repo.SaveUSPTOAssignments(ctx, app.ApplicationNumber, assignments); saveErr != nil {
		return proto.USPTOFetchAssignmentsResult{}, fmt.Errorf("engine: save uspto assignments: %w", saveErr)
	}
	parties := 0
	for _, a := range assignments {
		parties += len(a.Parties)
	}
	e.incCounter("uspto.fetch_assignments.ok", 1)
	e.incCounter("uspto.fetch_assignments.records", int64(len(assignments)))
	e.incCounter("uspto.fetch_assignments.parties", int64(parties))
	e.log(ctx, slog.LevelInfo, "uspto assignments fetched",
		slog.String("patent", n.Normalized()),
		slog.String("application", app.ApplicationNumber),
		slog.Int("assignments", len(assignments)),
		slog.Int("parties", parties))
	e.recordActivity(ctx, observability.Record{
		Component: "engine",
		Action:    "uspto.fetch_assignments",
		Entity:    n.Normalized(),
		Status:    "ok",
		Attributes: map[string]any{
			"application_number": app.ApplicationNumber,
			"assignments":        len(assignments),
			"parties":            parties,
		},
	})
	e.announceChange()
	return proto.USPTOFetchAssignmentsResult{
		ApplicationNumber: app.ApplicationNumber,
		Assignments:       len(assignments),
		Parties:           parties,
	}, nil
}

// USPTOLookup queries the USPTO ODP API using the configured API key and returns the raw JSON response as a string.
func (e *Engine) USPTOLookup(ctx context.Context, number domain.PatentNumber) (string, error) {
	apiKey := strings.TrimSpace(e.usptoAPIKey)
	if apiKey == "" {
		return "", fmt.Errorf("engine: USPTO API key not configured")
	}

	serial := strings.TrimSpace(number.Serial)
	if serial == "" {
		return "", fmt.Errorf("engine: empty application number")
	}

	// Query formulation matching crawl/uspto.go / cmd/patentmine/lookup.go
	norm := number.Normalized()
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
func (e *Engine) ClearPatentCache(ctx context.Context, patents []domain.PatentNumber) (int64, int64, error) {
	start := time.Now()
	var cleared, bytesSaved int64
	var opErr error
	defer func() {
		// Log telemetry & metrics
		if e.metrics != nil {
			e.metrics.ObserveDuration("engine.uspto.clear_cache", time.Since(start), opErr != nil)
			e.metrics.IncCounter("engine.uspto.clear_cache.count", 1)
			if opErr == nil {
				e.metrics.IncCounter("engine.uspto.clear_cache.records", cleared)
				e.metrics.IncCounter("engine.uspto.clear_cache.bytes_saved", bytesSaved)
			}
		}
		e.log(ctx, slog.LevelInfo, "uspto clear cache completed",
			slog.Int("requested_patents", len(patents)),
			slog.Int64("cleared_records", cleared),
			slog.Int64("bytes_saved", bytesSaved),
			slog.Bool("success", opErr == nil))
	}()

	var appNums []string
	if len(patents) > 0 {
		for _, n := range patents {
			app, err := e.repo.USPTOApplication(ctx, n)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					// If the application doesn't exist, it has no cached body either. Skip.
					continue
				}
				opErr = fmt.Errorf("engine: resolve application number for %s: %w", n.Normalized(), err)
				return 0, 0, opErr
			}
			if strings.TrimSpace(app.ApplicationNumber) != "" {
				appNums = append(appNums, app.ApplicationNumber)
			}
		}
		// If we had patents but resolved none of them to an application, nothing to clear.
		if len(appNums) == 0 {
			return 0, 0, nil
		}
	}

	cleared, bytesSaved, opErr = e.repo.ClearUSPTOGrantBodies(ctx, appNums)
	return cleared, bytesSaved, opErr
}

