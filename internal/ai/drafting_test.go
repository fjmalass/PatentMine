package ai

import (
	"strings"
	"testing"

	"patentmine/internal/domain"
)

func TestBuildDraftPromptGroundingAndRules(t *testing.T) {
	g := GroundingInput{
		Kind:           domain.DraftNonprovisional,
		Section:        domain.SectionBackground,
		SectionHeading: "BACKGROUND",
		Title:          "Self-Cleaning Widget",
		Project:        domain.Project{Inventors: []string{"Ada Lovelace"}, ApplicationNumber: "16/123,456"},
		PriorArt: []PriorArtRef{
			{Number: "US9999999B2", Title: "Old Widget", Abstract: "A widget.", Relevant: "Does not teach self-cleaning."},
		},
		Notes:       "Emphasize the self-cleaning mechanism.",
		Pinned:      []string{"a self-cleaning surface coating"},
		Instruction: "Draft the background.",
	}
	p := BuildDraftPrompt(g)

	for _, want := range []string{
		"HARD RULES",
		needsAttorneyInput,
		"PINNED VERBATIM",
		"a self-cleaning surface coating",
		"US9999999B2",
		"Does not teach self-cleaning.",
		"Ada Lovelace",
		"16/123,456",
		"Draft the background.",
		"Non-Provisional",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q\n---\n%s", want, p)
		}
	}
}

func TestBuildDraftPromptOfficeActionText(t *testing.T) {
	g := GroundingInput{
		Kind:             domain.DraftOAResponse,
		Section:          domain.SectionRemarks,
		OfficeActionText: "Claims 1-3 are rejected under 35 U.S.C. 103 over Smith.",
	}
	p := BuildDraftPrompt(g)
	if !strings.Contains(p, "Office Action text") || !strings.Contains(p, "rejected under 35 U.S.C. 103 over Smith") {
		t.Errorf("office action text not grounded into prompt:\n%s", p)
	}
}

func TestVerifyVerbatim(t *testing.T) {
	draft := "The invention provides a self-cleaning surface coating that resists fouling."
	// Pinned text present but with different whitespace must still match.
	missing := VerifyVerbatim(draft, []string{"a self-cleaning   surface coating", "a hydrophobic membrane"})
	if len(missing) != 1 || missing[0] != "a hydrophobic membrane" {
		t.Fatalf("VerifyVerbatim = %v, want [a hydrophobic membrane]", missing)
	}
}

func TestVerifyDraftFlagsNeedsInput(t *testing.T) {
	draft := "The reference discloses " + needsAttorneyInput + " which is unsupported."
	problems := VerifyDraft(draft, []string{"required exact phrase"})
	if len(problems) != 2 {
		t.Fatalf("expected 2 problems (missing pinned + needs-input), got %d: %v", len(problems), problems)
	}
}

func TestVerifyDraftClean(t *testing.T) {
	draft := "It contains the required exact phrase verbatim."
	if problems := VerifyDraft(draft, []string{"required exact phrase"}); len(problems) != 0 {
		t.Fatalf("expected no problems, got %v", problems)
	}
}

// Compile-time proof that both providers satisfy Drafter, so the engine can be
// wired with either.
var (
	_ Drafter = (*GeminiAnalyzer)(nil)
	_ Drafter = (*OllamaAnalyzer)(nil)
)
