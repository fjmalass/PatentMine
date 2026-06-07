package markdown

import (
	"strings"
	"testing"
	"time"

	"patentmine/internal/domain"
)

func TestRenderClaimsAmendmentMarkup(t *testing.T) {
	d := domain.Draft{
		Kind: domain.DraftOAResponse,
		Claims: []domain.DraftClaim{
			{Number: 1, Status: domain.AmendCurrentlyAmended,
				BaseText: "A widget comprising a first arm.",
				Text:     "A widget comprising a second arm."},
			{Number: 2, Status: domain.AmendOriginal, Text: "The widget of claim 1."},
			{Number: 3, Status: domain.AmendCancelled},
			{Number: 4, Status: domain.AmendNew, Text: "A method of using the widget."},
		},
	}
	out := RenderClaims(d, domain.Project{}, nil)

	if !strings.Contains(out, "# Amendments to the Claims") {
		t.Errorf("missing amendments heading:\n%s", out)
	}
	if !strings.Contains(out, "<del>first</del>") {
		t.Errorf("deleted word not struck through:\n%s", out)
	}
	if !strings.Contains(out, "<ins>second</ins>") {
		t.Errorf("inserted word not marked:\n%s", out)
	}
	if !strings.Contains(out, "1. (Currently Amended)") {
		t.Errorf("missing currently-amended label:\n%s", out)
	}
	if !strings.Contains(out, "2. (Original) The widget of claim 1.") {
		t.Errorf("original claim mis-rendered:\n%s", out)
	}
	if !strings.Contains(out, "3. (Cancelled)") {
		t.Errorf("cancelled claim should carry only its label:\n%s", out)
	}
	// An amended claim's unchanged words must not be wrapped in markup.
	if strings.Contains(out, "<ins>A widget") || strings.Contains(out, "<del>A widget") {
		t.Errorf("unchanged words wrongly marked:\n%s", out)
	}
}

func TestRenderResponseCaptionAndRemarks(t *testing.T) {
	d := domain.Draft{
		Kind: domain.DraftOAResponse,
		Sections: []domain.DraftSection{
			{Kind: domain.SectionRemarks, Heading: domain.SectionRemarks.Heading(),
				Body: "Claim 1 is allowable.\n\nThe rejection is respectfully traversed."},
		},
	}
	oa := &domain.OfficeAction{
		ApplicationNumber: "16/123,456",
		Examiner:          "John Smith",
		MailDate:          time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}
	out := RenderResponse(d, domain.Project{}, oa)

	for _, want := range []string{
		"# In the United States Patent and Trademark Office",
		"- **Application No.:** 16/123,456",
		"- **Examiner:** John Smith",
		"# Response to Office Action Dated 2026-01-15",
		"## Remarks",
		"Claim 1 is allowable.",
		"The rejection is respectfully traversed.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("response.md missing %q:\n%s", want, out)
		}
	}
}
