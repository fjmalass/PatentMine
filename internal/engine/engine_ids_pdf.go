package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"patentmine/internal/domain"
	exportids "patentmine/internal/export/ids"
	"patentmine/internal/observability"
	"patentmine/internal/store"
)

// IDSPDFOptions carries optional 08c fields the caller may provide for the
// signature and deposit-account sections. All fields are optional; empty
// strings leave the matching cells blank on the generated PDF.
type IDSPDFOptions struct {
	CumulativeCount int
	FeeAmount       string
	DepositAccount  string
	SignerName      string
	SignerSignature string
	SignerRegNumber string
}

// IDSPDFPreview describes what an IDS export *would* produce without writing
// anything. It is the data the export-confirm overlay shows before the user
// commits to writing PDFs to disk.
type IDSPDFPreview struct {
	BaseDir         string   // directory the timestamped subdir will be created under
	USCount         int      // number of US entries that will appear on 08a
	ForeignCount    int      // number of foreign entries that will appear on 08a
	Sheets          int      // 08a sheet count after pagination
	FeeTier         int      // 0..3 per 37 CFR 1.17(v)
	CumulativeCount int      // value that drove FeeTier
	ExistingDirs    []string // previous ids-* subdirs already on disk for this project
	MissingFields   []string // labels of required IDS header fields that are empty
}

// PreviewIDSPDF builds an IDSPDFPreview without writing anything to disk. It
// is the cheap counterpart to ExportIDSPDF so the UI can summarise the run
// (and surface missing IDS header fields) before committing.
func (e *Engine) PreviewIDSPDF(ctx context.Context, projectID domain.ProjectID, opts IDSPDFOptions) (preview IDSPDFPreview, err error) {
	defer e.observeDuration("engine.preview_ids_pdf", time.Now(), &err)
	if e.docsExportDir == "" {
		return IDSPDFPreview{}, errors.New("engine: ids export dir not configured")
	}
	project, err := e.repo.Project(ctx, projectID)
	if err != nil {
		return IDSPDFPreview{}, err
	}
	entries, err := e.repo.ListIDSEntries(ctx, projectID)
	if err != nil {
		return IDSPDFPreview{}, fmt.Errorf("engine: list ids entries: %w", err)
	}

	flattened := flattenIDSEntries(ctx, e.repo, entries)
	us, foreign := exportids.Bucket(flattened)
	sheets := exportids.Paginate(us, foreign)

	cumulative := opts.CumulativeCount
	if cumulative == 0 {
		cumulative = len(flattened)
	}

	preview = IDSPDFPreview{
		BaseDir:         filepath.Join(e.docsExportDir, sanitizeProjectID(string(projectID))),
		USCount:         len(us),
		ForeignCount:    len(foreign),
		Sheets:          len(sheets),
		FeeTier:         exportids.FeeTier(cumulative),
		CumulativeCount: cumulative,
		MissingFields:   missingIDSHeaderFields(project),
	}
	preview.ExistingDirs = listExistingExports(preview.BaseDir)
	return preview, nil
}

// missingIDSHeaderFields returns the labels of required IDS header fields the
// project does not yet have populated. The list is shown to the user via the
// export-confirm overlay so they can fix it before a PDF is generated.
func missingIDSHeaderFields(p domain.Project) []string {
	var missing []string
	if strings.TrimSpace(p.ApplicationNumber) == "" {
		missing = append(missing, "Application Number")
	}
	if p.FilingDate.IsZero() {
		missing = append(missing, "Filing Date")
	}
	if p.FirstInventor() == "" {
		missing = append(missing, "First Named Inventor")
	}
	if strings.TrimSpace(p.AttorneyDocketNumber) == "" {
		missing = append(missing, "Attorney Docket Number")
	}
	return missing
}

// idsUnknownPlaceholder is written into country / kind cells on the IDS when
// the record on file does not supply a confident value. Two visible dashes
// signal to the examiner that the field is intentionally blank rather than
// silently missing.
const idsUnknownPlaceholder = "--"

// idsKindWhitelist is the exact set of kind codes accepted on the IDS. Any
// other value is replaced with idsUnknownPlaceholder.
var idsKindWhitelist = map[string]bool{
	"A1": true,
	"A2": true,
	"B1": true,
	"B2": true,
}

// idsCountryFor returns the country code to write into an IDS row for the
// given patent. The stored country wins; the placeholder is used when it is
// missing.
func idsCountryFor(p domain.Patent) string {
	if c := strings.TrimSpace(p.Number.Country); c != "" {
		return c
	}
	return idsUnknownPlaceholder
}

// idsKindFor returns the kind code to write into an IDS row. The last two
// characters of the document's normalized number must match idsKindWhitelist;
// anything else collapses to the placeholder.
func idsKindFor(doc domain.Document) string {
	n := doc.Number.Normalized()
	if len(n) < 2 {
		return idsUnknownPlaceholder
	}
	suffix := n[len(n)-2:]
	if !idsKindWhitelist[suffix] {
		return idsUnknownPlaceholder
	}
	return suffix
}

// flattenIDSEntries resolves each IDS entry to its patent record and projects
// the data needed by the renderer. Patents that no longer exist or have no
// publishable document are silently dropped (the same rule the export uses).
func flattenIDSEntries(ctx context.Context, repo idsRepo, entries []domain.IDSEntry) []exportids.Entry {
	out := make([]exportids.Entry, 0, len(entries))
	for _, entry := range entries {
		patent, err := repo.Patent(ctx, entry.Patent)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			continue
		}
		doc, ok := idsDocument(patent)
		if !ok {
			continue
		}
		number := doc.Number
		number.Country = idsCountryFor(patent)
		number.Kind = idsKindFor(doc)
		out = append(out, exportids.Entry{
			Number:   number,
			Patentee: patent.Assignee,
			PubDate:  pubDateFor(patent, doc),
			Passages: entry.RelevantPassages,
			InFull:   entry.InFull,
		})
	}
	return out
}

// idsRepo is the subset of the store needed by flattenIDSEntries.
type idsRepo interface {
	Patent(ctx context.Context, n domain.PatentNumber) (domain.Patent, error)
}

// sanitizeProjectID strips path separators and unsafe characters from a
// project id so it can be used as a directory name.
func sanitizeProjectID(id string) string {
	rep := strings.NewReplacer(string(os.PathSeparator), "_", "/", "_", "\\", "_", "..", "_")
	out := rep.Replace(strings.TrimSpace(id))
	if out == "" {
		return "project"
	}
	return out
}

// listExistingExports returns the basenames of every "ids-*" directory
// already present at base. Missing directories are not an error: the export
// has simply never run for this project yet.
func listExistingExports(base string) []string {
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "ids-") {
			out = append(out, e.Name())
		}
	}
	return out
}

// ExportIDSPDF renders the curated IDS for a project to PDF (PTO/SB/08a +
// 08c, with overflow handled by re-filling 08a per USPTO convention).
//
// The bundle is written to a fresh timestamped directory under the configured
// IDS export base; the absolute paths are returned so the caller can present
// or stream them. CumulativeCount selects the 37 CFR 1.17(v) fee tier; pass
// the total number of applicant-provided items for the application (or just
// the entries' length when no prior IDS has been filed).
func (e *Engine) ExportIDSPDF(ctx context.Context, projectID domain.ProjectID, opts IDSPDFOptions) (res exportids.Result, err error) {
	defer e.observeDuration("engine.export_ids_pdf", time.Now(), &err)
	if e.docsExportDir == "" {
		return exportids.Result{}, errors.New("engine: ids export dir not configured")
	}
	project, err := e.repo.Project(ctx, projectID)
	if err != nil {
		return exportids.Result{}, err
	}
	entries, err := e.repo.ListIDSEntries(ctx, projectID)
	if err != nil {
		return exportids.Result{}, fmt.Errorf("engine: list ids entries: %w", err)
	}

	flattened := flattenIDSEntries(ctx, e.repo, entries)

	cumulative := opts.CumulativeCount
	if cumulative == 0 {
		cumulative = len(flattened)
	}

	in := exportids.Input{
		Project:         project,
		Entries:         flattened,
		CumulativeCount: cumulative,
		FeeAmount:       opts.FeeAmount,
		DepositAccount:  opts.DepositAccount,
		SignerName:      opts.SignerName,
		SignerSignature: opts.SignerSignature,
		SignerRegNumber: opts.SignerRegNumber,
	}
	obs := &observability.Runtime{Metrics: e.metrics, Activity: e.activities}
	res, err = exportids.Export(ctx, in, e.docsExportDir, obs)
	if err != nil {
		return exportids.Result{}, err
	}
	return res, nil
}

// pubDateFor picks the right date for an IDS row: the document's own date
// when set, otherwise the patent's grant or publication date.
func pubDateFor(p domain.Patent, doc domain.Document) time.Time {
	if !doc.Dated.IsZero() {
		return doc.Dated
	}
	if !p.GrantDate.IsZero() {
		return p.GrantDate
	}
	return p.PublicationDate
}

// UpdateProject persists changes to a project's mutable fields (name and IDS
// attrs). The project's id and created_at are taken from the existing
// record; everything else from the supplied value.
func (e *Engine) UpdateProject(ctx context.Context, p domain.Project) (saved domain.Project, err error) {
	defer e.observeDuration("engine.update_project", time.Now(), &err)
	if p.ID == "" {
		return domain.Project{}, errors.New("engine: project id required")
	}
	existing, err := e.repo.Project(ctx, p.ID)
	if err != nil {
		return domain.Project{}, err
	}
	existing.Name = p.Name
	existing.ApplicationNumber = p.ApplicationNumber
	existing.FilingDate = p.FilingDate
	existing.Inventors = p.Inventors
	existing.ArtUnit = p.ArtUnit
	existing.AttorneyDocketNumber = p.AttorneyDocketNumber
	// Examiners: merge new entries on top of the existing history. An entry
	// from the caller without a RecordedAt is treated as a brand-new
	// assignment recorded at now; otherwise the caller-supplied timestamp is
	// kept so historical entries can be backfilled too. Duplicates by
	// (name, recorded_at) collapse at the DB layer.
	for _, ex := range p.Examiners {
		entry := ex
		if entry.Name == "" {
			continue
		}
		if entry.RecordedAt.IsZero() {
			entry.RecordedAt = time.Now().UTC()
		}
		existing.Examiners = append(existing.Examiners, entry)
	}
	if err := e.repo.SaveProject(ctx, existing); err != nil {
		return domain.Project{}, err
	}
	e.announceChange()
	return existing, nil
}
