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
	"strconv"
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

func parseUSPTODateHelper(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, format := range []string{"2006-01-02", "20060102", "01-02-2006"} {
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

	// Best-effort enrichment: if this patent record has no classifications or other fields yet
	// (common for pure-USPTO source or USPTOOnly mode), lift the parsed metadata we just got.
	if p, perr := e.repo.Patent(ctx, n); perr == nil {
		changed := false
		if len(p.Classifications) == 0 && len(bundle.Classifications) > 0 {
			if codes, cerr := e.repo.USPTOGrantClassifications(ctx, applicationNumber, string(kind)); cerr == nil && len(codes) > 0 {
				p.Classifications = codes
				changed = true
				e.incCounter("uspto.xml.ingest.classifications_backfilled", int64(len(codes)))
				_ = e.repo.UpdateUnreconciledSourceDiffUSPTOValue(ctx, n, "classifications", strings.Join(codes, ";"))
			}
		}
		if p.Title == "" && bundle.Summary.InventionTitle != "" {
			p.Title = bundle.Summary.InventionTitle
			changed = true
			_ = e.repo.UpdateUnreconciledSourceDiffUSPTOValue(ctx, n, "title", bundle.Summary.InventionTitle)
		}
		if p.Abstract == "" && bundle.Body.AbstractText != "" {
			p.Abstract = bundle.Body.AbstractText
			changed = true
			_ = e.repo.UpdateUnreconciledSourceDiffUSPTOValue(ctx, n, "abstract", bundle.Body.AbstractText)
		}
		if p.FirstClaim == "" && len(bundle.Body.Claims) > 0 {
			p.FirstClaim = bundle.Body.Claims[0].Text
			changed = true
			_ = e.repo.UpdateUnreconciledSourceDiffUSPTOValue(ctx, n, "first_claim", bundle.Body.Claims[0].Text)
		}
		if p.ApplicationDate.IsZero() && bundle.Summary.FilingDate != "" {
			if t := parseUSPTODateHelper(bundle.Summary.FilingDate); !t.IsZero() {
				p.ApplicationDate = t
				changed = true
				_ = e.repo.UpdateUnreconciledSourceDiffUSPTOValue(ctx, n, "application_date", t.Format("2006-01-02"))
			}
		}
		pubDateStr := bundle.Summary.GrantDate
		if p.PublicationDate.IsZero() && pubDateStr != "" {
			if t := parseUSPTODateHelper(pubDateStr); !t.IsZero() {
				p.PublicationDate = t
				changed = true
				_ = e.repo.UpdateUnreconciledSourceDiffUSPTOValue(ctx, n, "publication_date", t.Format("2006-01-02"))
			}
		}
		if p.GrantDate.IsZero() && string(kind) == string(proto.USPTOXMLKindGrant) && pubDateStr != "" {
			if t := parseUSPTODateHelper(pubDateStr); !t.IsZero() {
				p.GrantDate = t
				changed = true
				_ = e.repo.UpdateUnreconciledSourceDiffUSPTOValue(ctx, n, "grant_date", t.Format("2006-01-02"))
			}
		}
		if p.Assignee == "" {
			var assigneeName string
			for _, party := range bundle.Parties {
				if party.Role == "assignee" {
					if party.OrgName != "" {
						assigneeName = party.OrgName
					} else {
						assigneeName = strings.TrimSpace(party.FirstName + " " + party.LastName)
					}
					break
				}
			}
			if assigneeName != "" {
				p.Assignee = assigneeName
				changed = true
				_ = e.repo.UpdateUnreconciledSourceDiffUSPTOValue(ctx, n, "assignee", assigneeName)
			}
		}
		if len(p.Inventors) == 0 {
			var inventors []domain.Inventor
			var inventorNames []string
			for _, party := range bundle.Parties {
				if party.Role == "inventor" {
					name := strings.TrimSpace(party.FirstName + " " + party.LastName)
					if name != "" {
						inventors = append(inventors, domain.Inventor(name))
						inventorNames = append(inventorNames, name)
					}
				}
			}
			if len(inventors) > 0 {
				p.Inventors = inventors
				changed = true
				_ = e.repo.UpdateUnreconciledSourceDiffUSPTOValue(ctx, n, "inventors", strings.Join(inventorNames, ";"))
			}
		}

		if changed {
			if serr := e.repo.SavePatent(ctx, p); serr == nil {
				e.log(ctx, slog.LevelInfo, "uspto xml metadata enriched and saved to patent table",
					slog.String("patent", n.Normalized()))
			} else {
				e.log(ctx, slog.LevelError, "failed to save enriched patent record",
					slog.String("patent", n.Normalized()),
					slog.String("error", serr.Error()))
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

// ComputeAndStoreUSPTOExpiration computes the statutory patent expiration date from the USPTO source,
// stores it in the uspto_application table, and writes comparison logs with Google Patents.
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
					slog.String("db_earliest", dbEarliest.Format("2006-01-02")),
					slog.String("api_earliest", apiEarliest.Format("2006-01-02")),
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
			parentN := domain.PatentNumber{Serial: app.EarliestTermAppNum}
			if _, apiErr := e.resolveUSPTOApplicationFromAPI(ctx, parentN); apiErr != nil {
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
		tdDateStr = tdDate.Format("2006-01-02")
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
		earliestTermFilingDateStr = earliestTermFilingDate.Format("2006-01-02")
	}
	computedExpStr := ""
	if !computedExp.IsZero() {
		computedExpStr = computedExp.Format("2006-01-02")
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
				slog.String("google_date", googleExp.Format("2006-01-02")),
				slog.Int("diff_days", diffDays),
				slog.Bool("has_terminal_disclaimer", hasTD),
			)
		}

		// Update the patent's own expiration date and source if it's a USPTO patent
		if p.Source == domain.SourceUSPTO && !computedExp.IsZero() {
			p.ExpirationDate = computedExp
			p.ExpirationSource = "uspto"
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

	var resp struct {
		Count                    int              `json:"count"`
		PatentFileWrapperDataBag []struct {
			ApplicationNumberText string `json:"applicationNumberText"`
			ApplicationMetaData   struct {
				InventionTitle            string `json:"inventionTitle"`
				FilingDate                string `json:"filingDate"`
				EffectiveFilingDate       string `json:"effectiveFilingDate"`
				ApplicationTypeCode       string `json:"applicationTypeCode"`
				ApplicationTypeLabelName  string `json:"applicationTypeLabelName"`
				ApplicationTypeCategory   string `json:"applicationTypeCategory"`
				FirstInventorName         string `json:"firstInventorName"`
				FirstApplicantName        string `json:"firstApplicantName"`
				ApplicationStatusCode     any    `json:"applicationStatusCode"`
				ApplicationStatusText     string `json:"applicationStatusDescriptionText"`
				ApplicationStatusDate     string `json:"applicationStatusDate"`
				GroupArtUnitNumber        string `json:"groupArtUnitNumber"`
				ExaminerNameText          string `json:"examinerNameText"`
				DocketNumber              string `json:"docketNumber"`
				CustomerNumber            any    `json:"customerNumber"`
				ApplicationConfirmationNumber any `json:"applicationConfirmationNumber"`
				FirstInventorToFileIndicator string `json:"firstInventorToFileIndicator"`
				NationalStageIndicator    bool   `json:"nationalStageIndicator"`
				USPCSymbolText            string `json:"uspcSymbolText"`
				Class                     string `json:"class"`
				Subclass                  string `json:"subclass"`
				PatentNumberText          string `json:"patentNumberText"`
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
	// The input n may be an application-style PatentNumber; patentNumberText gives the actual grant.
	patentRecordNum := n
	if ptxt := strings.TrimSpace(w.ApplicationMetaData.PatentNumberText); ptxt != "" {
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
	return e.resolveUSPTOApplicationFromAPI(ctx, domain.PatentNumber{Serial: appNum})
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
