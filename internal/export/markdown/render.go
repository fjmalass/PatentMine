// Package markdown renders a Draft to the two human-readable Markdown artifacts
// an office-action response is split into: new_claims.md (the marked-up claim
// listing) and response.md (the caption and remarks). They are projections of
// the structured Draft — the same source the .docx exporter renders — so the
// claim-status labels and MPEP 714 insertion/deletion markup are computed from
// DraftClaim rows (status + base→amended diff) rather than hand-typed, and never
// drift from the .docx. Insertions render as <ins> (underline) and deletions as
// <del> (strikethrough), the semantic HTML every Markdown viewer styles the way
// the USPTO expects.
package markdown

import (
	"fmt"
	"strings"

	"patentmine/internal/domain"
	"patentmine/internal/textdiff"
)

// ClaimsFilename and ResponseFilename are the stable, user-facing names of the
// two rendered artifacts. They are stable on purpose: the working copy always
// holds the latest version, while history lives in the draft_revision table and
// the git mirror (see engine.SnapshotDraftResponse).
const (
	ClaimsFilename   = "new_claims.md"
	ResponseFilename = "response.md"
)

// RenderClaims renders the claim listing to Markdown — the contents of
// new_claims.md. For an office-action response it is the "Amendments to the
// Claims" listing with status labels and markup; for a first application it is a
// plain numbered listing.
func RenderClaims(d domain.Draft, project domain.Project, oa *domain.OfficeAction) string {
	var b strings.Builder
	response := d.Kind == domain.DraftOAResponse
	if response {
		b.WriteString("# Amendments to the Claims\n\n")
		b.WriteString("*The following listing of claims will replace all prior versions, and listings, of claims in the application:*\n\n")
	} else {
		b.WriteString("# Claims\n\n")
		b.WriteString("*What is claimed is:*\n\n")
	}
	if len(d.Claims) == 0 {
		b.WriteString("_[No claims drafted yet.]_\n")
		return b.String()
	}
	for _, c := range d.Claims {
		b.WriteString(claimLine(c, response))
		b.WriteString("\n\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// RenderResponse renders the response caption and remarks to Markdown — the
// contents of response.md. The claim listing is kept in its own file
// (new_claims.md); response.md carries the USPTO caption followed by every prose
// section (the REMARKS arguments traversing the rejections).
func RenderResponse(d domain.Draft, project domain.Project, oa *domain.OfficeAction) string {
	var b strings.Builder

	if d.Kind == domain.DraftOAResponse {
		b.WriteString("# In the United States Patent and Trademark Office\n\n")
		writeField(&b, "Application No.", firstNonEmpty(oaAppNumber(oa), project.ApplicationNumber))
		writeField(&b, "Applicant / First Inventor", project.FirstInventor())
		writeField(&b, "Examiner", firstNonEmpty(oaExaminer(oa), project.LatestExaminer()))
		writeField(&b, "Art Unit", firstNonEmpty(oaArtUnit(oa), project.ArtUnit))
		writeField(&b, "Attorney Docket No.", strings.TrimSpace(project.AttorneyDocketNumber))
		b.WriteString("\n")
		heading := "Response to Office Action"
		if oa != nil && !oa.MailDate.IsZero() {
			heading += " Dated " + oa.MailDate.Format(domain.DateLayout)
		}
		fmt.Fprintf(&b, "# %s\n\n", heading)
	} else if t := strings.TrimSpace(d.Title); t != "" {
		fmt.Fprintf(&b, "# %s\n\n", t)
	}

	for _, s := range d.Sections {
		b.WriteString(sectionMarkdown(s))
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// claimLine renders one claim as a Markdown list-free paragraph. showStatus adds
// the MPEP 714 status label (shown in an amendment, omitted for a first
// application); a currently-amended claim carries inline ins/del markup.
func claimLine(c domain.DraftClaim, showStatus bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d. ", c.Number)
	if showStatus {
		if label := c.Status.Label(); label != "" {
			b.WriteString(label)
			b.WriteString(" ")
		}
	}
	if c.Status == domain.AmendCancelled {
		return strings.TrimRight(b.String(), " ")
	}
	if showStatus && c.Status.ShowsMarkup() {
		b.WriteString(markupInline(c.BaseText, c.Text))
	} else {
		b.WriteString(strings.TrimSpace(c.Text))
	}
	return b.String()
}

// markupInline renders the base→amended transition with <ins>/<del> tags. When
// base is empty the amended text stands alone (a new or original claim carries
// no markup). Trailing whitespace on each span is kept outside the tags so the
// underline/strikethrough hugs the words, not the gaps between them.
func markupInline(base, amended string) string {
	if strings.TrimSpace(base) == "" {
		return strings.TrimSpace(amended)
	}
	var b strings.Builder
	for _, op := range coalesce(textdiff.Words(base, amended)) {
		core, trail := splitTrailingSpace(op.Text)
		if strings.TrimSpace(core) == "" {
			b.WriteString(op.Text)
			continue
		}
		switch op.Kind {
		case textdiff.OpEqual:
			b.WriteString(core)
		case textdiff.OpInsert:
			fmt.Fprintf(&b, "<ins>%s</ins>", core)
		case textdiff.OpDelete:
			fmt.Fprintf(&b, "<del>%s</del>", core)
		}
		b.WriteString(trail)
	}
	return strings.TrimSpace(b.String())
}

// coalesce merges adjacent ops of the same kind so the markup reads as whole
// inserted/deleted phrases rather than word-by-word.
func coalesce(in []textdiff.Op) []textdiff.Op {
	if len(in) == 0 {
		return in
	}
	out := []textdiff.Op{in[0]}
	for _, op := range in[1:] {
		last := &out[len(out)-1]
		if last.Kind == op.Kind {
			last.Text += op.Text
			continue
		}
		out = append(out, op)
	}
	return out
}

func splitTrailingSpace(s string) (core, trail string) {
	core = strings.TrimRight(s, " \t\n")
	return core, s[len(core):]
}

// sectionMarkdown writes a section as a level-2 heading and its body, splitting
// on blank lines so multi-paragraph prose keeps its shape. An empty section
// prints a bracketed placeholder so the author sees the skeleton.
func sectionMarkdown(s domain.DraftSection) string {
	heading := strings.TrimSpace(s.Heading)
	if heading == "" {
		heading = s.Kind.Heading()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", titleCase(heading))
	body := strings.TrimSpace(s.Body)
	if body == "" {
		fmt.Fprintf(&b, "_[%s — to be drafted]_\n\n", heading)
		return b.String()
	}
	for _, para := range splitParagraphs(body) {
		b.WriteString(para)
		b.WriteString("\n\n")
	}
	return b.String()
}

// titleCase lowercases a heading and capitalizes the first letter of each word,
// turning the all-caps section headings (e.g. "BRIEF DESCRIPTION OF THE
// DRAWINGS") into title case for a calmer Markdown reading experience.
func titleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		r := []rune(w)
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

func writeField(b *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(b, "- **%s:** %s\n", label, value)
}

// splitParagraphs breaks body text into paragraphs on blank lines, flattening
// single newlines within a paragraph to spaces (mirrors the .docx exporter).
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
