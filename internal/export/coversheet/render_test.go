package coversheet

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"

	"patentmine/internal/domain"
)

// TestRenderRoundTrip fills the SB/16 and reads the AcroForm back through
// pdfcpu's exporter to assert the values we set actually land in the form.
func TestRenderRoundTrip(t *testing.T) {
	cs := domain.ProvisionalCoverSheet{
		Title: "Self-Cleaning Widget",
		Inventors: []domain.ProvisionalCoverSheetInventor{
			{GivenName: "Ada", FamilyName: "Lovelace", Residence: "London, UK"},
			{GivenName: "Grace", FamilyName: "Hopper", Residence: "Arlington, VA"},
		},
		SpecEnclosed:     true,
		SpecPages:        12,
		DrawingsEnclosed: true,
		DrawingSheets:    3,
		SmallEntity:      true,
		TotalFeeAmount:   "120",
		PayDeposit:       true,
		DepositAccount:   "50-1234",
		GovInterest:      domain.GovInterestContract,
		ContractNumber:   "W911-22-C-0001",
		SignerName:       "Pat Attorney",
		SignatureText:    "/Pat Attorney/",
		RegistrationNo:   "12345",
		SignatureDate:    time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC),
		DocketNumber:     "ACME-001",
	}

	pdf, err := RenderProvisional(cs, nil)
	if err != nil {
		t.Fatalf("RenderProvisional: %v", err)
	}
	if len(pdf) == 0 {
		t.Fatal("RenderProvisional produced no bytes")
	}

	tmp := t.TempDir()
	pdfPath := filepath.Join(tmp, "filled.pdf")
	jsonPath := filepath.Join(tmp, "filled.json")
	if err := os.WriteFile(pdfPath, pdf, 0o644); err != nil {
		t.Fatalf("write pdf: %v", err)
	}
	if err := api.ExportFormFile(pdfPath, jsonPath, nil); err != nil {
		t.Fatalf("ExportFormFile: %v", err)
	}
	got, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read exported json: %v", err)
	}
	form := string(got)

	for _, want := range []string{
		"Self-Cleaning Widget", "Ada", "Lovelace", "London, UK",
		"Grace", "Hopper", "W911-22-C-0001", "ACME-001", "06-07-2026", "50-1234",
	} {
		if !contains(form, want) {
			t.Errorf("filled form missing %q", want)
		}
	}
}

func TestRenderMissingFlags(t *testing.T) {
	cs := domain.ProvisionalCoverSheet{Title: "X"}
	if miss := cs.Missing(); len(miss) != 2 { // needs inventor + signature
		t.Errorf("expected 2 missing (inventor, signature), got %v", miss)
	}
	cs.Inventors = []domain.ProvisionalCoverSheetInventor{{FamilyName: "Doe"}}
	cs.SignerName = "Doe"
	if !cs.Complete() {
		t.Errorf("cover sheet should be complete, missing: %v", cs.Missing())
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (stringIndex(haystack, needle) >= 0)
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
