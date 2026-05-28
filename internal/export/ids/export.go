package ids

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
)

// Input is everything Export needs to render an IDS bundle.
type Input struct {
	Project         domain.Project
	Entries         []Entry
	CumulativeCount int    // total applicant-provided items so far (incl. these), for the fee tier
	FeeAmount       string // optional: charge dollar amount on 08c
	DepositAccount  string // optional: deposit account number on 08c
	SignerName      string // optional: printed signer name
	SignerSignature string // optional: /signature/ text
	SignerRegNumber string // optional: practitioner registration number
	GeneratedAt     time.Time
}

// Result describes the files written for one IDS rendering.
type Result struct {
	Dir       string   `json:"dir"`
	Files     []string `json:"files"`
	FeeTier   int      `json:"fee_tier"`
	PageCount int      `json:"page_count"`
}

// Export renders the IDS bundle for the given input into a fresh timestamped
// directory under baseDir and returns the result. obs may be nil; when set,
// duration and page counts are recorded under the "ids.export.pdf" metric.
func Export(ctx context.Context, in Input, baseDir string, obs *observability.Runtime) (Result, error) {
	start := time.Now()
	var failed bool
	defer func() {
		if obs != nil && obs.Metrics != nil {
			obs.Metrics.ObserveDuration("ids.export.pdf", time.Since(start), failed)
		}
	}()

	if in.GeneratedAt.IsZero() {
		in.GeneratedAt = time.Now().UTC()
	}
	if in.Project.ID == "" {
		failed = true
		return Result{}, fmt.Errorf("ids: project id required")
	}

	us, foreign := Bucket(in.Entries)
	sheets := Paginate(us, foreign)
	totalSheets := len(sheets)

	stamp := in.GeneratedAt.Format("20060102-150405")
	dir := filepath.Join(baseDir, sanitizeID(string(in.Project.ID)), "ids-"+stamp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		failed = true
		return Result{}, fmt.Errorf("ids: create dir: %w", err)
	}

	var files []string

	// Render every 08a sheet (re-use the same template; header repeats per
	// USPTO convention). Sheet 1 is the unsuffixed file; overflows are
	// 08a-2.pdf, 08a-3.pdf, ...
	for i, s := range sheets {
		page := buildSB08APage(in.Project, s, i+1, totalSheets)
		out, err := fillTemplate(templateSB0008A, page)
		if err != nil {
			failed = true
			return Result{}, fmt.Errorf("ids: render 08a sheet %d: %w", i+1, err)
		}
		name := "08a.pdf"
		if i > 0 {
			name = fmt.Sprintf("08a-%d.pdf", i+1)
		}
		if err := os.WriteFile(filepath.Join(dir, name), out, 0o644); err != nil {
			failed = true
			return Result{}, fmt.Errorf("ids: write %s: %w", name, err)
		}
		files = append(files, name)
	}

	feeTier := FeeTier(in.CumulativeCount)
	feePage := buildSB08CPage(in, feeTier)
	feeOut, err := fillTemplate(templateSB0008C, feePage)
	if err != nil {
		failed = true
		return Result{}, fmt.Errorf("ids: render 08c: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "08c.pdf"), feeOut, 0o644); err != nil {
		failed = true
		return Result{}, fmt.Errorf("ids: write 08c.pdf: %w", err)
	}
	files = append(files, "08c.pdf")

	if obs != nil {
		if obs.Metrics != nil {
			obs.Metrics.IncCounter("ids.export.pages", int64(totalSheets+1))
		}
		if obs.Activity != nil {
			_ = obs.Activity.Record(ctx, observability.Record{
				Action:   observability.ActionIDSExportPDF,
				Entity:   "project",
				EntityID: string(in.Project.ID),
				Status:   "ok",
				Attributes: map[string]any{
					"sheets":           totalSheets,
					"fee_tier":         feeTier,
					"cumulative_count": in.CumulativeCount,
					"dir":              dir,
				},
			})
		}
	}

	return Result{
		Dir:       dir,
		Files:     files,
		FeeTier:   feeTier,
		PageCount: totalSheets + 1,
	}, nil
}

func buildSB08APage(project domain.Project, sheet Sheet, sheetNum, total int) formPage {
	var page formPage
	page.addText(sb08aHeaderFields.AppNumber, project.ApplicationNumber)
	page.addText(sb08aHeaderFields.FilingDate, formatDate(project.FilingDate))
	page.addText(sb08aHeaderFields.Inventor, project.FirstInventor())
	page.addText(sb08aHeaderFields.ArtUnit, project.ArtUnit)
	page.addText(sb08aHeaderFields.Examiner, project.LatestExaminer())
	page.addText(sb08aHeaderFields.DocketNumber, project.AttorneyDocketNumber)
	page.addText(sb08aHeaderFields.SheetN, fmt.Sprintf("%d", sheetNum))
	page.addText(sb08aHeaderFields.SheetOf, fmt.Sprintf("%d", total))

	for i, e := range sheet.US {
		cite, doc, date, patentee, pages := sb08aUSRowFields(i + 1)
		page.addText(cite, fmt.Sprintf("%d", i+1))
		page.addText(doc, docCellUS(e, ""))
		page.addText(date, formatDate(e.PubDate))
		page.addText(patentee, e.Patentee)
		page.addText(pages, e.Passages)
	}

	for i, e := range sheet.Foreign {
		cite, doc, date, patentee, pages, trans := sb08aForeignRowFields(i + 1)
		page.addText(cite, fmt.Sprintf("%d", i+1))
		page.addText(doc, docCellForeign(e))
		page.addText(date, formatDate(e.PubDate))
		page.addText(patentee, e.Patentee)
		page.addText(pages, e.Passages)
		if e.InFull {
			page.addBox(trans, true)
		}
	}
	return page
}

func buildSB08CPage(in Input, tier int) formPage {
	var page formPage
	page.addText(sb08cAppNumber, in.Project.ApplicationNumber)
	page.addText(sb08cInventor, in.Project.FirstInventor())
	page.addText(sb08cArtUnit, in.Project.ArtUnit)
	page.addText(sb08cFilingDate, formatDate(in.Project.FilingDate))
	page.addText(sb08cDocket, in.Project.AttorneyDocketNumber)
	page.addText(sb08cExaminer, in.Project.LatestExaminer())
	switch tier {
	case 0:
		page.addBox(sb08cBoxNoFee, true)
	case 1:
		page.addBox(sb08cBoxTier1, true)
	case 2:
		page.addBox(sb08cBoxTier2, true)
	case 3:
		page.addBox(sb08cBoxTier3, true)
	}
	page.addText(sb08cFeeAmount, in.FeeAmount)
	page.addText(sb08cDepositNum, in.DepositAccount)
	page.addText(sb08cSignDate, formatDate(in.GeneratedAt))
	page.addText(sb08cSignName, in.SignerName)
	page.addText(sb08cRegNumber, in.SignerRegNumber)
	page.addText(sb08cSignature, in.SignerSignature)
	return page
}

// sanitizeID strips path separators and unsafe characters from a project id
// so it can be used as a directory name.
func sanitizeID(id string) string {
	rep := strings.NewReplacer(string(os.PathSeparator), "_", "/", "_", "\\", "_", "..", "_")
	out := rep.Replace(strings.TrimSpace(id))
	if out == "" {
		return "project"
	}
	return out
}
