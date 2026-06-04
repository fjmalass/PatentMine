package docx

import (
	"fmt"
	"strings"
	"time"

	"patentmine/internal/domain"
)

// RenderDraft renders a Draft to .docx bytes. project supplies the caption /
// header fields (the same prosecution data the IDS exporter reads); oa is the
// answered Office Action for a response draft and may be nil otherwise.
//
// The structure follows the draft kind: a first application leads with a title
// and specification sections (plus a claim listing for a non-provisional),
// while an office-action response leads with the USPTO caption and an
// "Amendments to the Claims" listing carrying MPEP 714 markup, then the remarks.
func RenderDraft(d domain.Draft, project domain.Project, oa *domain.OfficeAction) ([]byte, error) {
	doc := New()
	switch d.Kind {
	case domain.DraftOAResponse:
		renderResponse(doc, d, project, oa)
	default:
		renderApplication(doc, d, project)
	}
	return doc.Bytes()
}

func renderApplication(doc *Document, d domain.Draft, project domain.Project) {
	title := strings.TrimSpace(d.Title)
	if title == "" {
		title = "PATENT APPLICATION"
	}
	doc.Title(title)

	switch d.Kind {
	case domain.DraftProvisional:
		doc.Para("PROVISIONAL APPLICATION FOR PATENT")
	case domain.DraftNonprovisional:
		doc.Para("NON-PROVISIONAL APPLICATION FOR PATENT")
	}
	if inv := strings.Join(project.Inventors, "; "); inv != "" {
		doc.Para("Inventor(s): " + inv)
	}
	if dn := strings.TrimSpace(project.AttorneyDocketNumber); dn != "" {
		doc.Para("Attorney Docket No.: " + dn)
	}
	doc.Spacer()

	for _, s := range d.Sections {
		renderSection(doc, s)
	}

	// A non-provisional must claim; render the structured claim listing under
	// the conventional lead-in. A provisional carries no claims.
	if d.Kind.NeedsClaims() && len(d.Claims) > 0 {
		doc.Heading1("CLAIMS")
		doc.Para("What is claimed is:")
		for _, c := range d.Claims {
			renderClaim(doc, c, false)
		}
	}
}

func renderResponse(doc *Document, d domain.Draft, project domain.Project, oa *domain.OfficeAction) {
	doc.Title("IN THE UNITED STATES PATENT AND TRADEMARK OFFICE")

	appNo := firstNonEmpty(oaAppNumber(oa), project.ApplicationNumber)
	if appNo != "" {
		doc.Para("Application No.: " + appNo)
	}
	if inv := project.FirstInventor(); inv != "" {
		doc.Para("Applicant / First Inventor: " + inv)
	}
	if ex := firstNonEmpty(oaExaminer(oa), project.LatestExaminer()); ex != "" {
		doc.Para("Examiner: " + ex)
	}
	if au := firstNonEmpty(oaArtUnit(oa), project.ArtUnit); au != "" {
		doc.Para("Art Unit: " + au)
	}
	if dn := strings.TrimSpace(project.AttorneyDocketNumber); dn != "" {
		doc.Para("Attorney Docket No.: " + dn)
	}
	doc.Spacer()

	heading := "RESPONSE TO OFFICE ACTION"
	if oa != nil && !oa.MailDate.IsZero() {
		heading += " DATED " + oa.MailDate.Format(domain.DateLayout)
	}
	doc.Heading1(heading)

	// Amendments to the claims come first, in their own marked-up listing, per
	// USPTO amendment practice.
	if len(d.Claims) > 0 {
		doc.Heading1("AMENDMENTS TO THE CLAIMS")
		doc.Para("The following listing of claims will replace all prior versions, and listings, of claims in the application:")
		for _, c := range d.Claims {
			renderClaim(doc, c, true)
		}
	}

	for _, s := range d.Sections {
		renderSection(doc, s)
	}
}

// renderSection writes a heading and the section body, splitting the body into
// paragraphs on blank lines so multi-paragraph prose keeps its shape.
func renderSection(doc *Document, s domain.DraftSection) {
	heading := strings.TrimSpace(s.Heading)
	if heading == "" {
		heading = s.Kind.Heading()
	}
	doc.Heading1(heading)
	body := strings.TrimSpace(s.Body)
	if body == "" {
		// An empty section still prints its heading so the author sees the
		// skeleton; a bracketed placeholder marks what remains to be written.
		doc.IndentedPara("[" + heading + " — to be drafted]")
		return
	}
	for _, para := range splitParagraphs(body) {
		doc.IndentedPara(para)
	}
}

// renderClaim writes one claim. showStatus controls whether the MPEP 714 status
// identifier is printed next to the number — it is shown in an amendment
// (office-action response) and omitted for a first application's original claims.
func renderClaim(doc *Document, c domain.DraftClaim, showStatus bool) {
	var prefix strings.Builder
	fmt.Fprintf(&prefix, "%d. ", c.Number)
	if showStatus {
		if label := c.Status.Label(); label != "" {
			prefix.WriteString(label)
			prefix.WriteString(" ")
		}
	}

	if c.Status == domain.AmendCancelled {
		doc.RunsPara([]Run{{Text: strings.TrimRight(prefix.String(), " ")}}, true)
		return
	}

	runs := []Run{{Text: prefix.String()}}
	if showStatus && c.Status.ShowsMarkup() {
		runs = append(runs, amendmentRuns(c.BaseText, c.Text)...)
	} else {
		runs = append(runs, Run{Text: c.Text})
	}
	doc.RunsPara(runs, true)
}

// splitParagraphs breaks body text into paragraphs on blank lines, trimming
// each. Single newlines within a paragraph are flattened to spaces.
func splitParagraphs(body string) []string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	chunks := strings.Split(body, "\n\n")
	var out []string
	for _, chunk := range chunks {
		joined := strings.Join(strings.Fields(chunk), " ")
		if joined != "" {
			out = append(out, joined)
		}
	}
	if len(out) == 0 {
		return []string{strings.TrimSpace(body)}
	}
	return out
}

// DraftFilename returns a stable, filesystem-safe base name for a rendered
// draft, e.g. "provisional-2026-06-03-150405.docx".
func DraftFilename(d domain.Draft, generatedAt time.Time) string {
	kind := string(d.Kind)
	if kind == "" {
		kind = "draft"
	}
	return fmt.Sprintf("%s-%s.docx", kind, generatedAt.Format(domain.FileTimestampLayout))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func oaAppNumber(oa *domain.OfficeAction) string {
	if oa == nil {
		return ""
	}
	return oa.ApplicationNumber
}

func oaExaminer(oa *domain.OfficeAction) string {
	if oa == nil {
		return ""
	}
	return oa.Examiner
}

func oaArtUnit(oa *domain.OfficeAction) string {
	if oa == nil {
		return ""
	}
	return oa.ArtUnit
}
