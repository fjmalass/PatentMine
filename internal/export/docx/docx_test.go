package docx

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"patentmine/internal/domain"
)

// readPart unzips a generated .docx and returns the named part's content.
func readPart(t *testing.T, data []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open docx zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open part %s: %v", name, err)
			}
			defer rc.Close()
			b, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read part %s: %v", name, err)
			}
			return string(b)
		}
	}
	t.Fatalf("part %s not found in archive", name)
	return ""
}

func TestDocumentBytesHasRequiredParts(t *testing.T) {
	data, err := New().Title("Hello").Para("World").Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	for _, part := range []string{"[Content_Types].xml", "_rels/.rels", "word/document.xml", "word/styles.xml", "word/_rels/document.xml.rels"} {
		_ = readPart(t, data, part) // fails the test if missing
	}
	doc := readPart(t, data, "word/document.xml")
	if !strings.Contains(doc, "Hello") || !strings.Contains(doc, "World") {
		t.Errorf("document.xml missing text: %s", doc)
	}
}

func TestEscapingPreventsBrokenXML(t *testing.T) {
	data, err := New().Para(`A & B < C > D "q" 'a'`).Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	doc := readPart(t, data, "word/document.xml")
	if strings.Contains(doc, "A & B") {
		t.Error("ampersand was not escaped")
	}
	if !strings.Contains(doc, "&amp;") || !strings.Contains(doc, "&lt;") || !strings.Contains(doc, "&gt;") {
		t.Errorf("expected escaped entities, got: %s", doc)
	}
}

func TestRenderProvisionalHasNoClaims(t *testing.T) {
	d := domain.Draft{
		Kind:     domain.DraftProvisional,
		Title:    "A Better Mousetrap",
		Sections: domain.DefaultSections(domain.DraftProvisional),
		// Claims present in the struct must be ignored for a provisional.
		Claims: []domain.DraftClaim{{Number: 1, Text: "A mousetrap."}},
	}
	d.Sections[0].Body = "This invention relates to rodent control."
	data, err := RenderDraft(d, domain.Project{Inventors: []string{"Ada Lovelace"}}, nil)
	if err != nil {
		t.Fatalf("RenderDraft: %v", err)
	}
	doc := readPart(t, data, "word/document.xml")
	if !strings.Contains(doc, "A Better Mousetrap") {
		t.Error("title missing")
	}
	if !strings.Contains(doc, "Ada Lovelace") {
		t.Error("inventor missing")
	}
	if strings.Contains(doc, "What is claimed is") {
		t.Error("provisional must not render a claim listing")
	}
	if !strings.Contains(doc, "rodent control") {
		t.Error("section body missing")
	}
}

func TestRenderNonprovisionalRendersClaims(t *testing.T) {
	d := domain.Draft{
		Kind:  domain.DraftNonprovisional,
		Title: "Widget",
		Claims: []domain.DraftClaim{
			{Number: 1, Type: domain.ClaimIndependent, Status: domain.AmendOriginal, Text: "A widget comprising a frame."},
		},
	}
	data, err := RenderDraft(d, domain.Project{}, nil)
	if err != nil {
		t.Fatalf("RenderDraft: %v", err)
	}
	doc := readPart(t, data, "word/document.xml")
	if !strings.Contains(doc, "What is claimed is") {
		t.Error("non-provisional must render the claim lead-in")
	}
	if !strings.Contains(doc, "A widget comprising a frame.") {
		t.Error("claim text missing")
	}
}

func TestRenderResponseClaimMarkup(t *testing.T) {
	oaDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	d := domain.Draft{
		Kind: domain.DraftOAResponse,
		Sections: []domain.DraftSection{
			{Ordinal: 0, Kind: domain.SectionRemarks, Heading: "REMARKS", Body: "Claim 1 is patentable over Smith."},
		},
		Claims: []domain.DraftClaim{
			{
				Number:   1,
				Status:   domain.AmendCurrentlyAmended,
				BaseText: "A widget comprising a frame.",
				Text:     "A widget comprising a metal frame.",
			},
			{Number: 2, Status: domain.AmendCancelled},
		},
	}
	oa := domain.OfficeAction{MailDate: oaDate, Examiner: "Jane Doe", ArtUnit: "2151"}
	data, err := RenderDraft(d, domain.Project{ApplicationNumber: "16/123,456"}, &oa)
	if err != nil {
		t.Fatalf("RenderDraft: %v", err)
	}
	doc := readPart(t, data, "word/document.xml")

	if !strings.Contains(doc, "AMENDMENTS TO THE CLAIMS") {
		t.Error("amendments heading missing")
	}
	if !strings.Contains(doc, "(Currently Amended)") {
		t.Error("status identifier missing")
	}
	// The inserted word "metal" must be underlined.
	if !strings.Contains(doc, `<w:u w:val="single"/>`) {
		t.Error("expected an underlined insertion run for the added word")
	}
	if !strings.Contains(doc, "metal") {
		t.Error("inserted word missing")
	}
	if !strings.Contains(doc, "RESPONSE TO OFFICE ACTION DATED") {
		t.Error("response heading/date missing")
	}
	if !strings.Contains(doc, "Jane Doe") {
		t.Error("examiner caption missing")
	}
	if !strings.Contains(doc, "(Cancelled)") {
		t.Error("cancelled claim status missing")
	}
	if !strings.Contains(doc, "patentable over Smith") {
		t.Error("remarks body missing")
	}
}

func TestAmendmentRunsDiff(t *testing.T) {
	runs := amendmentRuns("the quick brown fox", "the slow brown fox")
	var struck, underlined, plain []string
	for _, r := range runs {
		switch {
		case r.Strike:
			struck = append(struck, strings.TrimSpace(r.Text))
		case r.Underline:
			underlined = append(underlined, strings.TrimSpace(r.Text))
		default:
			plain = append(plain, strings.TrimSpace(r.Text))
		}
	}
	if strings.Join(struck, " ") != "quick" {
		t.Errorf("deleted = %v, want [quick]", struck)
	}
	if strings.Join(underlined, " ") != "slow" {
		t.Errorf("inserted = %v, want [slow]", underlined)
	}
	// Reassembling every run reproduces the amended text exactly (additions +
	// equal words), with deletions removed.
	var assembled strings.Builder
	for _, r := range runs {
		if !r.Strike {
			assembled.WriteString(r.Text)
		}
	}
	if got := strings.Join(strings.Fields(assembled.String()), " "); got != "the slow brown fox" {
		t.Errorf("assembled amended text = %q", got)
	}
}

func TestAmendmentRunsNewClaimNoMarkup(t *testing.T) {
	runs := amendmentRuns("", "A brand new claim.")
	if len(runs) != 1 || runs[0].Underline || runs[0].Strike {
		t.Errorf("a new claim must be a single plain run, got %+v", runs)
	}
}
