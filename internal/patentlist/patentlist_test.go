package patentlist

import (
	"strings"
	"testing"

	"patentmine/internal/domain"
)

func TestParseValid(t *testing.T) {
	content := Header + "\n" +
		"# project: Acme\n" +
		"# exported: 2026-05-31\n" +
		"\n" +
		"US11611785B2\n" +
		"  EP1234567A1  \n" +
		"# a trailing comment\n"
	numbers, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(numbers) != 2 {
		t.Fatalf("got %d numbers, want 2: %v", len(numbers), numbers)
	}
	if numbers[0].String() != "US11611785B2" {
		t.Errorf("first number = %q, want US11611785B2", numbers[0].String())
	}
}

func TestParseBlankLinesBeforeHeaderAllowed(t *testing.T) {
	content := "\n\n" + Header + "\nUS11611785B2\n"
	if _, err := Parse(content); err != nil {
		t.Fatalf("Parse with leading blanks: %v", err)
	}
}

func TestParseMissingHeader(t *testing.T) {
	if _, err := Parse("US11611785B2\nEP1234567A1\n"); err == nil {
		t.Fatal("expected error for missing header, got nil")
	}
}

func TestParseEmpty(t *testing.T) {
	if _, err := Parse(""); err == nil {
		t.Fatal("expected error for empty content, got nil")
	}
}

func TestParseBadNumber(t *testing.T) {
	content := Header + "\nnot-a-patent!!\n"
	if _, err := Parse(content); err == nil {
		t.Fatal("expected error for bad number, got nil")
	}
}

func TestFormatHasHeaderAndSortedNumbers(t *testing.T) {
	a, _ := domain.ParsePatentNumber("US11611785B2")
	b, _ := domain.ParsePatentNumber("EP1234567A1")
	out := Format([]domain.PatentNumber{a, b}, "Acme", "2026-05-31")
	if !strings.HasPrefix(out, Header+"\n") {
		t.Fatalf("output missing header:\n%s", out)
	}
	if !strings.Contains(out, "# project: Acme") || !strings.Contains(out, "# exported: 2026-05-31") {
		t.Errorf("output missing metadata comments:\n%s", out)
	}
	// EP sorts before US, so the EP line must come first among numbers.
	epIdx := strings.Index(out, "EP1234567A1")
	usIdx := strings.Index(out, "US11611785B2")
	if epIdx < 0 || usIdx < 0 || epIdx > usIdx {
		t.Errorf("numbers not sorted as expected:\n%s", out)
	}
}

func TestFormatOmitsEmptyMetadata(t *testing.T) {
	a, _ := domain.ParsePatentNumber("US11611785B2")
	out := Format([]domain.PatentNumber{a}, "", "")
	if strings.Contains(out, "# project:") || strings.Contains(out, "# exported:") {
		t.Errorf("expected no metadata comment lines:\n%s", out)
	}
}

func TestRoundTrip(t *testing.T) {
	a, _ := domain.ParsePatentNumber("US11611785B2")
	b, _ := domain.ParsePatentNumber("EP1234567A1")
	out := Format([]domain.PatentNumber{a, b}, "Acme", "2026-05-31")
	parsed, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse formatted output: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("round trip lost numbers: got %d, want 2", len(parsed))
	}
}
