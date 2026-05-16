package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/phpdave11/gofpdf"

	"patentmine/internal/domain"
)

func IDSPassagesText(e domain.IDSEntry) string {
	if e.InFull {
		return IDSIconInFull + " In full"
	}
	return e.RelevantPassages
}

type idsExportReference struct {
	Entry   domain.IDSEntry
	Patent  domain.Patent
	Section string
}

func (m *Model) idsExportCommand(args []string) (tea.Model, tea.Cmd) {
	ids, err := m.repo.ListIDSEntries(m.ctx, m.ProjectID)
	if err != nil {
		m.err = "error loading IDS: " + err.Error()
		return m, nil
	}
	npl, err := m.repo.ListIDSNPLEntries(m.ctx, m.ProjectID)
	if err != nil {
		m.err = "error loading IDS NPL: " + err.Error()
		return m, nil
	}
	meta, _ := m.repo.GetIDSMetadata(m.ctx, m.ProjectID)

	projectName := m.projectNameForExport()
	filename := fmt.Sprintf("%s_IDS_%s.md", strings.ReplaceAll(projectName, " ", "_"), time.Now().Format(dateFmtDate))
	if len(args) > 0 {
		filename = args[0]
	}

	refs := m.idsExportReferences(append(ids, npl...))
	if strings.EqualFold(filepath.Ext(filename), ".pdf") {
		if err := writeIDSPDF(filename, projectName, m.ProjectID, meta, refs, time.Now()); err != nil {
			m.err = "export failed: " + err.Error()
			return m, nil
		}
		m.message = fmt.Sprintf("IDS PDF exported to %s (%d entries)", filename, len(ids)+len(npl))
		return m, nil
	}

	content := renderIDSMarkdown(projectName, m.ProjectID, meta, refs, time.Now())
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		m.err = "export failed: " + err.Error()
		return m, nil
	}
	m.message = fmt.Sprintf("IDS exported to %s (%d entries)", filename, len(ids)+len(npl))
	return m, nil
}

func (m *Model) idsExportReferences(entries []domain.IDSEntry) []idsExportReference {
	refs := make([]idsExportReference, 0, len(entries))
	for _, entry := range entries {
		var patent domain.Patent
		if !entry.IsNPL && entry.PatentNumber != "" {
			p, err := m.repo.GetPatent(m.ctx, m.ProjectID, entry.PatentNumber)
			if err == nil {
				patent = p
			} else {
				patent = domain.Patent{Number: entry.PatentNumber}
			}
		}
		refs = append(refs, idsExportReference{
			Entry:   entry,
			Patent:  patent,
			Section: idsReferenceSection(entry),
		})
	}
	return refs
}

func idsReferenceSection(entry domain.IDSEntry) string {
	if entry.IsNPL {
		return "Non-Patent Literature"
	}
	upper := strings.ToUpper(strings.TrimSpace(entry.PatentNumber))
	cc := strings.ToUpper(strings.TrimSpace(entry.CountryCode))
	if cc == "US" || strings.HasPrefix(upper, "US") {
		return "U.S. Patent Documents"
	}
	if entry.PatentNumber != "" {
		return "Foreign Patent Documents"
	}
	return "Non-Patent Literature"
}

func patentDisplayDate(p domain.Patent) string {
	if p.PublicationDate != "" {
		return p.PublicationDate
	}
	if p.GrantDate != "" {
		return p.GrantDate
	}
	return "—"
}

func patentPatentee(p domain.Patent) string {
	if p.Assignee != "" {
		return p.Assignee
	}
	if len(p.Inventors) > 0 {
		return p.Inventors[0]
	}
	return "—"
}

func renderIDSMarkdown(projectName, projectID string, meta domain.IDSMetadata, refs []idsExportReference, now time.Time) string {
	var buf strings.Builder
	buf.WriteString("# Information Disclosure Statement\n\n")
	buf.WriteString(fmt.Sprintf("**Project:** %s (%s)  \n", projectName, projectID))
	buf.WriteString(fmt.Sprintf("**Date:** %s  \n\n", now.Format(dateFmtDate)))

	if meta.AppNumber != "" || meta.FirstInventor != "" {
		buf.WriteString("## Application Information\n\n")
		if meta.AppNumber != "" {
			buf.WriteString(fmt.Sprintf("**Application Number:** %s  \n", meta.AppNumber))
		}
		if meta.FilingDate != "" {
			buf.WriteString(fmt.Sprintf("**Filing Date:** %s  \n", meta.FilingDate))
		}
		if meta.FirstInventor != "" {
			buf.WriteString(fmt.Sprintf("**First Named Inventor:** %s  \n", meta.FirstInventor))
		}
		if meta.ArtUnit != "" {
			buf.WriteString(fmt.Sprintf("**Art Unit:** %s  \n", meta.ArtUnit))
		}
		if meta.ExaminerName != "" {
			buf.WriteString(fmt.Sprintf("**Examiner:** %s  \n", meta.ExaminerName))
		}
		if meta.AttorneyDocket != "" {
			buf.WriteString(fmt.Sprintf("**Attorney Docket:** %s  \n", meta.AttorneyDocket))
		}
		buf.WriteString("\n")
	}

	sections := []string{"U.S. Patent Documents", "Foreign Patent Documents", "Non-Patent Literature"}
	for _, section := range sections {
		sectionRefs := filterIDSReferences(refs, section)
		if len(sectionRefs) == 0 {
			continue
		}
		buf.WriteString("## " + section + "\n\n")
		switch section {
		case "U.S. Patent Documents":
			buf.WriteString("| # | Patent Number | Kind | Issue Date | Patentee/Applicant | Relevant Passages | Status |\n")
			buf.WriteString("|---|---------------|------|------------|-------------------|-------------------|--------|\n")
			for i, ref := range sectionRefs {
				kind := ref.Entry.KindCode
				if kind == "" {
					kind = "—"
				}
				patentee := patentPatentee(ref.Patent)
				passages := IDSPassagesText(ref.Entry)
				if passages == "" {
					passages = "—"
				}
				status := ref.Entry.Status
				if status == "" {
					status = "—"
				}
				buf.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | %s | %s |\n",
					i+1, ref.Entry.PatentNumber, kind, patentDisplayDate(ref.Patent),
					escapeTable(patentee), escapeTable(passages), status))
			}
		case "Foreign Patent Documents":
			buf.WriteString("| # | Country | Document Number | Kind | Pub Date | Patentee/Applicant | Relevant Passages | Status |\n")
			buf.WriteString("|---|---------|-----------------|------|----------|-------------------|-------------------|--------|\n")
			for i, ref := range sectionRefs {
				cc := ref.Entry.CountryCode
				if cc == "" {
					cc = "—"
				}
				kind := ref.Entry.KindCode
				if kind == "" {
					kind = "—"
				}
				patentee := patentPatentee(ref.Patent)
				passages := IDSPassagesText(ref.Entry)
				if passages == "" {
					passages = "—"
				}
				status := ref.Entry.Status
				if status == "" {
					status = "—"
				}
				buf.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | %s | %s | %s |\n",
					i+1, cc, ref.Entry.PatentNumber, kind, patentDisplayDate(ref.Patent),
					escapeTable(patentee), escapeTable(passages), status))
			}
		case "Non-Patent Literature":
			buf.WriteString("| # | Author | Title | Date | Publisher | Relevant Passages | Status |\n")
			buf.WriteString("|---|--------|-------|------|-----------|-------------------|--------|\n")
			for i, ref := range sectionRefs {
				author := ref.Entry.NPLAuthor
				if author == "" {
					author = "—"
				}
				title := ref.Entry.NPLTitle
				if title == "" {
					title = "—"
				}
				date := ref.Entry.NPLDate
				if date == "" {
					date = "—"
				}
				publisher := ref.Entry.NPLPublisher
				if publisher == "" {
					publisher = "—"
				}
				passages := IDSPassagesText(ref.Entry)
				if passages == "" {
					passages = "—"
				}
				status := ref.Entry.Status
				if status == "" {
					status = "—"
				}
				buf.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | %s | %s |\n",
					i+1, escapeTable(author), escapeTable(title), date,
					escapeTable(publisher), escapeTable(passages), status))
			}
		}
		buf.WriteString("\n")
	}
	buf.WriteString("---\n\n")
	buf.WriteString("*I hereby certify that each item of information listed above was first cited in any communication from a foreign patent office in a counterpart foreign application not more than three months prior to the filing of this statement.*\n\n")
	buf.WriteString(fmt.Sprintf("Signature: ________________________  Date: %s\n", now.Format("January 2, 2006")))
	return buf.String()
}

func filterIDSReferences(refs []idsExportReference, section string) []idsExportReference {
	var filtered []idsExportReference
	for _, ref := range refs {
		if ref.Section == section {
			filtered = append(filtered, ref)
		}
	}
	return filtered
}

func escapeTable(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

// drawRow draws a row of cells with MultiCell and returns the Y position after the tallest cell.
func drawRow(pdf *gofpdf.Fpdf, startX, startY float64, colW []float64, cells []string, lineH float64, border, align string) float64 {
	x := startX
	maxY := startY
	for i, cell := range cells {
		pdf.SetXY(x, startY)
		pdf.MultiCell(colW[i], lineH, cell, border, align, false)
		if pdf.GetY() > maxY {
			maxY = pdf.GetY()
		}
		x += colW[i]
	}
	return maxY
}

func writeIDSPDF(filename, projectName, projectID string, meta domain.IDSMetadata, refs []idsExportReference, now time.Time) error {
	pdf := gofpdf.New("P", "mm", "Letter", "")
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	// === HEADER ===
	pageW, _ := pdf.GetPageSize()
	margins := 15.0 * 2
	usableW := pageW - margins

	pdf.SetFont("Helvetica", "B", 13)
	pdf.CellFormat(usableW*0.55, 7, "INFORMATION DISCLOSURE STATEMENT", "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 9)
	pdf.CellFormat(usableW*0.45, 7, fmt.Sprintf("Application Number: %s", orDash(meta.AppNumber)), "", 1, "L", false, 0, "")

	pdf.SetFont("Helvetica", "", 9)
	pdf.CellFormat(usableW*0.55, 5, "BY APPLICANT", "", 0, "L", false, 0, "")
	pdf.CellFormat(usableW*0.45, 5, fmt.Sprintf("Filing Date: %s", orDash(meta.FilingDate)), "", 1, "L", false, 0, "")

	pdf.CellFormat(usableW*0.55, 5, "(Not for submission under 37 CFR 1.99)", "", 0, "L", false, 0, "")
	pdf.CellFormat(usableW*0.45, 5, fmt.Sprintf("First Named Inventor: %s", orDash(meta.FirstInventor)), "", 1, "L", false, 0, "")

	pdf.CellFormat(usableW*0.55, 5, fmt.Sprintf("Project: %s (%s)", projectName, projectID), "", 0, "L", false, 0, "")
	pdf.CellFormat(usableW*0.45, 5, fmt.Sprintf("Art Unit: %s", orDash(meta.ArtUnit)), "", 1, "L", false, 0, "")

	pdf.CellFormat(usableW*0.55, 5, "", "", 0, "L", false, 0, "")
	pdf.CellFormat(usableW*0.45, 5, fmt.Sprintf("Examiner Name: %s", orDash(meta.ExaminerName)), "", 1, "L", false, 0, "")

	pdf.CellFormat(usableW*0.55, 5, "", "", 0, "L", false, 0, "")
	pdf.CellFormat(usableW*0.45, 5, fmt.Sprintf("Attorney Docket No.: %s", orDash(meta.AttorneyDocket)), "", 1, "L", false, 0, "")

	pdf.Ln(3)
	pdf.SetDrawColor(0, 0, 0)
	pdf.Line(15, pdf.GetY(), pageW-15, pdf.GetY())
	pdf.Ln(3)

	// === U.S. PATENT DOCUMENTS ===
	usRefs := filterIDSReferences(refs, "U.S. Patent Documents")
	if len(usRefs) > 0 {
		writeIDSUSSection(pdf, usRefs, usableW)
		pdf.Ln(4)
	}

	// === FOREIGN PATENT DOCUMENTS ===
	foreignRefs := filterIDSReferences(refs, "Foreign Patent Documents")
	if len(foreignRefs) > 0 {
		writeIDSForeignSection(pdf, foreignRefs, usableW)
		pdf.Ln(4)
	}

	// === NON-PATENT LITERATURE ===
	nplRefs := filterIDSReferences(refs, "Non-Patent Literature")
	if len(nplRefs) > 0 {
		writeIDSNPLSection(pdf, nplRefs, usableW)
		pdf.Ln(4)
	}

	// === CERTIFICATION ===
	pdf.SetFont("Helvetica", "", 8)
	pdf.MultiCell(usableW, 4,
		"I hereby certify that each item of information listed in this Information Disclosure Statement was first cited in any communication from a foreign patent office in a counterpart foreign application not more than three months prior to the filing of this statement, or that no item of information contained in this statement was cited in a communication from a foreign patent office in a counterpart foreign application, and that each item of information contained in this statement is in conformance with the requirements of 37 C.F.R. §§ 1.97 and 1.98.",
		"", "L", false)
	pdf.Ln(4)
	pdf.SetFont("Helvetica", "", 9)
	pdf.CellFormat(usableW*0.5, 6, "Signature: ______________________________", "", 0, "L", false, 0, "")
	pdf.CellFormat(usableW*0.5, 6, fmt.Sprintf("Date: %s", now.Format("01-02-2006")), "", 1, "L", false, 0, "")
	pdf.CellFormat(usableW*0.5, 6, "Registration No.: ________________________", "", 0, "L", false, 0, "")
	pdf.CellFormat(usableW*0.5, 6, "", "", 1, "L", false, 0, "")

	return pdf.OutputFileAndClose(filename)
}

func orDash(s string) string {
	if s == "" {
		return "___________"
	}
	return s
}

func writeIDSUSSection(pdf *gofpdf.Fpdf, refs []idsExportReference, usableW float64) {
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(0, 6, "U.S. PATENT DOCUMENTS", "", 1, "L", false, 0, "")

	// Column widths: Exam Init, #, Patent Number, Kind, Pub Date, Patentee, Passages
	colW := []float64{12, 10, 34, 12, 26, 52, usableW - 12 - 10 - 34 - 12 - 26 - 52}
	headers := []string{"Examiner\nInitials", "Cite\nNo.", "Patent Number", "Kind\nCode", "Publication\nDate MM-DD-YYYY", "Name of Patentee or\nApplicant", "Pages, Columns, Lines\nWhere Relevant Passages\nor Figures Appear"}

	pdf.SetFont("Helvetica", "B", 7)
	startY := pdf.GetY()
	maxY := drawRow(pdf, 15, startY, colW, headers, 3.5, "1", "C")
	pdf.SetY(maxY)

	pdf.SetFont("Helvetica", "", 8)
	for i, ref := range refs {
		startY = pdf.GetY()
		cells := []string{
			"",
			fmt.Sprintf("%d", i+1),
			ref.Entry.PatentNumber,
			ref.Entry.KindCode,
			formatIDSDate(patentDisplayDate(ref.Patent)),
			patentPatentee(ref.Patent),
			IDSPassagesText(ref.Entry),
		}
		maxY = drawRow(pdf, 15, startY, colW, cells, 4.5, "1", "L")
		pdf.SetY(maxY)
	}
}

func writeIDSForeignSection(pdf *gofpdf.Fpdf, refs []idsExportReference, usableW float64) {
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(0, 6, "FOREIGN PATENT DOCUMENTS", "", 1, "L", false, 0, "")

	colW := []float64{12, 10, 16, 32, 12, 26, 48, usableW - 12 - 10 - 16 - 32 - 12 - 26 - 48}
	headers := []string{"Examiner\nInitials", "Cite\nNo.", "Country\nCode", "Document Number", "Kind\nCode", "Publication\nDate", "Name of Patentee\nor Applicant", "Pages/Relevant\nPassages"}

	pdf.SetFont("Helvetica", "B", 7)
	startY := pdf.GetY()
	maxY := drawRow(pdf, 15, startY, colW, headers, 3.5, "1", "C")
	pdf.SetY(maxY)

	pdf.SetFont("Helvetica", "", 8)
	for i, ref := range refs {
		startY = pdf.GetY()
		cc := ref.Entry.CountryCode
		if cc == "" {
			// try to parse from patent number prefix
			pn := strings.ToUpper(ref.Entry.PatentNumber)
			if len(pn) >= 2 && !('0' <= pn[0] && pn[0] <= '9') {
				cc = pn[:2]
			}
		}
		cells := []string{
			"",
			fmt.Sprintf("%d", i+1),
			cc,
			ref.Entry.PatentNumber,
			ref.Entry.KindCode,
			formatIDSDate(patentDisplayDate(ref.Patent)),
			patentPatentee(ref.Patent),
			IDSPassagesText(ref.Entry),
		}
		maxY = drawRow(pdf, 15, startY, colW, cells, 4.5, "1", "L")
		pdf.SetY(maxY)
	}
}

func writeIDSNPLSection(pdf *gofpdf.Fpdf, refs []idsExportReference, usableW float64) {
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(0, 6, "NON-PATENT LITERATURE DOCUMENTS", "", 1, "L", false, 0, "")

	colW := []float64{12, 10, usableW - 12 - 10 - 40, 40}
	headers := []string{"Examiner\nInitials", "Cite\nNo.", "Author, Title of Article, Journal, Book, Volume, Date, Page(s)", "Pages/Relevant\nPassages"}

	pdf.SetFont("Helvetica", "B", 7)
	startY := pdf.GetY()
	maxY := drawRow(pdf, 15, startY, colW, headers, 3.5, "1", "C")
	pdf.SetY(maxY)

	pdf.SetFont("Helvetica", "", 8)
	for i, ref := range refs {
		startY = pdf.GetY()
		var citation strings.Builder
		if ref.Entry.NPLAuthor != "" {
			citation.WriteString(ref.Entry.NPLAuthor + ". ")
		}
		if ref.Entry.NPLTitle != "" {
			citation.WriteString("\"" + ref.Entry.NPLTitle + "\". ")
		}
		if ref.Entry.NPLPublisher != "" {
			citation.WriteString(ref.Entry.NPLPublisher + ". ")
		}
		if ref.Entry.NPLDate != "" {
			citation.WriteString(ref.Entry.NPLDate + ".")
		}

		cells := []string{
			"",
			fmt.Sprintf("%d", i+1),
			strings.TrimSpace(citation.String()),
			IDSPassagesText(ref.Entry),
		}
		maxY = drawRow(pdf, 15, startY, colW, cells, 4.5, "1", "L")
		pdf.SetY(maxY)
	}
}

func formatIDSDate(d string) string {
	if d == "" || d == "—" {
		return "—"
	}
	// Convert YYYY-MM-DD to MM-DD-YYYY (USPTO format)
	if len(d) == 10 && d[4] == '-' {
		return d[5:7] + "-" + d[8:10] + "-" + d[0:4]
	}
	return d
}
