package ai

import (
	"context"
	"fmt"
	"strings"

	"patentmine/internal/domain"
)

// Drafter is the generic completion capability the drafting subsystem needs: a
// single grounded prompt in, model text out, plus provenance. Both the cloud
// (Gemini) and local (Ollama) analyzers satisfy it, so the engine can be wired
// with either — local for unpublished/work-product matters, cloud for published
// ones — without depending on a concrete provider.
type Drafter interface {
	Complete(ctx context.Context, prompt string) (string, error)
	Provider() Provider
	Model() string
}

// Extractor pulls plain text out of a binary document (a scanned, image-only
// PDF; an image) with a multimodal model — OCR for documents that carry no text
// layer, which the pure-Go pdf extractor cannot read. A provider that cannot do
// this (a text-only local model) simply does not implement it, and callers fall
// back to "no extractable text" via a type assertion.
type Extractor interface {
	ExtractText(ctx context.Context, data []byte, mimeType string) (string, error)
	Provider() Provider
	Model() string
}

// DrafterConfig selects and configures the drafting provider. It is the same
// shape as the analyzer credentials so the daemon can build a Drafter from
// config without duplicating provider logic.
type DrafterConfig struct {
	Provider     string // "ollama" (local, privacy-preserving), "gemini" (cloud), or "openai"
	GeminiAPIKey string
	GeminiModel  string
	OllamaHost   string
	OllamaModel  string
	OpenAIAPIKey string
	OpenAIModel  string
}

// NewDrafter builds a Drafter honoring the chosen provider, falling back to
// whichever is configured (cloud key if present, else the local Ollama host).
// It returns nil only when neither path is available, so the engine can report
// an actionable setup error rather than panicking.
func NewDrafter(c DrafterConfig) Drafter {
	switch c.Provider {
	case string(ProviderOllama):
		if c.OllamaHost != "" {
			return NewOllamaAnalyzer(c.OllamaHost, c.OllamaModel)
		}
	case string(ProviderGemini):
		if c.GeminiAPIKey != "" {
			return NewGeminiAnalyzer(c.GeminiAPIKey, c.GeminiModel)
		}
	case string(ProviderOpenAI):
		if c.OpenAIAPIKey != "" {
			return NewOpenAIAnalyzer(c.OpenAIAPIKey, c.OpenAIModel)
		}
	}
	if c.GeminiAPIKey != "" {
		return NewGeminiAnalyzer(c.GeminiAPIKey, c.GeminiModel)
	}
	if c.OpenAIAPIKey != "" {
		return NewOpenAIAnalyzer(c.OpenAIAPIKey, c.OpenAIModel)
	}
	if c.OllamaHost != "" {
		return NewOllamaAnalyzer(c.OllamaHost, c.OllamaModel)
	}
	return nil
}

// PriorArtRef is a cited reference offered to the model as grounding when
// drafting an argument or a background. Only fields the corpus actually holds
// are passed; the model is told to rely on them and nothing else.
type PriorArtRef struct {
	Number   string
	Title    string
	Assignee string
	Abstract string
	Relevant string // attorney note on why this reference matters
}

// GroundingInput is the verified material a draft is generated from. Everything
// the model is allowed to rely on is here; the prompt forbids going beyond it.
type GroundingInput struct {
	Kind             domain.DraftKind
	Section          domain.SectionKind
	SectionHeading   string
	Title            string
	Project          domain.Project
	Claims           []domain.DraftClaim
	PriorArt         []PriorArtRef
	OfficeActionText string   // extracted examiner text, for a response
	Notes            string   // attorney notes / focus
	Pinned           []string // spans that MUST appear verbatim in the output
	Instruction      string   // the specific drafting ask
}

// needsAttorneyInput is the literal token the model is told to emit wherever it
// cannot support a statement from the supplied sources. Callers (and VerifyDraft)
// scan for it to flag spots that require a human before filing.
const needsAttorneyInput = "‹NEEDS ATTORNEY INPUT›"

// BuildDraftPrompt assembles the grounded, anti-hallucination prompt for one
// section. The structure is deliberate: hard rules first, then only the verified
// source blocks, then any pinned-verbatim spans, then the specific task. The
// model is instructed to use nothing beyond the provided material, to reproduce
// pinned text exactly, and to mark anything it cannot support with
// needsAttorneyInput rather than inventing it.
func BuildDraftPrompt(g GroundingInput) string {
	var b strings.Builder

	b.WriteString("You are a patent attorney's drafting assistant. You draft one section of a U.S. patent document.\n\n")
	b.WriteString("HARD RULES — follow exactly:\n")
	b.WriteString("1. Use ONLY the SOURCE MATERIAL below. Do not introduce any technical fact, prior-art reference, case citation, statute, date, name, or number that is not present in the sources.\n")
	fmt.Fprintf(&b, "2. If a statement cannot be supported by the sources, write the token %s in its place. Never guess or fabricate.\n", needsAttorneyInput)
	b.WriteString("3. Reproduce any text under PINNED VERBATIM exactly, character for character. Do not paraphrase pinned text.\n")
	b.WriteString("4. Do not draft or amend claim language; claims are handled separately. Write prose only.\n")
	b.WriteString("5. Return only the section body text, with no preamble, no markdown headings, and no commentary.\n\n")

	fmt.Fprintf(&b, "DOCUMENT TYPE: %s\n", draftKindLabel(g.Kind))
	heading := g.SectionHeading
	if heading == "" {
		heading = g.Section.Heading()
	}
	fmt.Fprintf(&b, "SECTION TO DRAFT: %s\n\n", heading)

	b.WriteString("SOURCE MATERIAL:\n")
	if t := strings.TrimSpace(g.Title); t != "" {
		fmt.Fprintf(&b, "- Invention title: %s\n", t)
	}
	if inv := strings.Join(g.Project.Inventors, "; "); inv != "" {
		fmt.Fprintf(&b, "- Inventor(s): %s\n", inv)
	}
	if app := strings.TrimSpace(g.Project.ApplicationNumber); app != "" {
		fmt.Fprintf(&b, "- Application No.: %s\n", app)
	}
	for _, c := range g.Claims {
		if strings.TrimSpace(c.Text) != "" {
			fmt.Fprintf(&b, "- Claim %d: %s\n", c.Number, strings.TrimSpace(c.Text))
		}
	}
	for i, ref := range g.PriorArt {
		fmt.Fprintf(&b, "- Reference [%d] %s", i+1, strings.TrimSpace(ref.Number))
		if ref.Title != "" {
			fmt.Fprintf(&b, " — %s", strings.TrimSpace(ref.Title))
		}
		b.WriteString("\n")
		if ref.Abstract != "" {
			fmt.Fprintf(&b, "    Abstract: %s\n", strings.TrimSpace(ref.Abstract))
		}
		if ref.Relevant != "" {
			fmt.Fprintf(&b, "    Attorney note: %s\n", strings.TrimSpace(ref.Relevant))
		}
	}
	if oa := strings.TrimSpace(g.OfficeActionText); oa != "" {
		fmt.Fprintf(&b, "- Office Action text (verbatim, examiner's words):\n%s\n", oa)
	}
	if notes := strings.TrimSpace(g.Notes); notes != "" {
		fmt.Fprintf(&b, "- Attorney notes / focus:\n%s\n", notes)
	}

	if pinned := nonEmpty(g.Pinned); len(pinned) > 0 {
		b.WriteString("\nPINNED VERBATIM — reproduce each of these exactly where relevant:\n")
		for _, p := range pinned {
			fmt.Fprintf(&b, "<<<\n%s\n>>>\n", strings.TrimSpace(p))
		}
	}

	b.WriteString("\nTASK: ")
	if instr := strings.TrimSpace(g.Instruction); instr != "" {
		b.WriteString(instr)
	} else {
		fmt.Fprintf(&b, "Draft the %s section from the source material above.", heading)
	}
	b.WriteString("\n")
	return b.String()
}

// VerifyDraft checks a generated section against its grounding and returns the
// problems a reviewer must resolve before the text can be trusted: pinned spans
// the model failed to reproduce verbatim, and any needsAttorneyInput markers it
// emitted. An empty result means the draft cleared both checks (it does NOT mean
// the draft is correct — only that the mechanical guardrails passed).
func VerifyDraft(draft string, pinned []string) []string {
	var problems []string
	missing := VerifyVerbatim(draft, pinned)
	for _, m := range missing {
		problems = append(problems, fmt.Sprintf("pinned text not reproduced verbatim: %q", truncate(m, 80)))
	}
	if n := strings.Count(draft, needsAttorneyInput); n > 0 {
		problems = append(problems, fmt.Sprintf("%d spot(s) marked %s need attorney input", n, needsAttorneyInput))
	}
	return problems
}

// VerifyVerbatim returns the pinned spans that do not appear verbatim in the
// draft, comparing on whitespace-normalized text so reflowing alone does not
// count as a mismatch. This is the deterministic counterpart to the prompt's
// "reproduce pinned text exactly" instruction: the instruction asks, this
// verifies, so a hallucinated paraphrase of a pinned quote is caught mechanically.
func VerifyVerbatim(draft string, pinned []string) []string {
	normDraft := normalizeWS(draft)
	var missing []string
	for _, p := range pinned {
		want := normalizeWS(p)
		if want == "" {
			continue
		}
		if !strings.Contains(normDraft, want) {
			missing = append(missing, strings.TrimSpace(p))
		}
	}
	return missing
}

func normalizeWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func nonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func draftKindLabel(k domain.DraftKind) string {
	switch k {
	case domain.DraftProvisional:
		return "Provisional Patent Application"
	case domain.DraftNonprovisional:
		return "Non-Provisional (Utility) Patent Application"
	case domain.DraftOAResponse:
		return "Response to Office Action"
	default:
		return "Patent Document"
	}
}
