package ai

import (
	"context"
	"patentmine/internal/domain"
)

// PromptForAction returns the prompt text and display locator for a named
// analysis action. Known actions: "novelty", "claims", "risk". An unknown or
// empty action returns ("", "AI Analysis (<provider>)") — the caller supplies
// a custom prompt in that case.
func PromptForAction(actionType string, provider Provider) (prompt, locator string) {
	locator = "AI Analysis (" + string(provider) + ")"
	switch actionType {
	case "novelty":
		return "Summarize the core technical novelty of this patent and highlight key legal/technology design takeaways.",
			"AI Novelty Summary"
	case "claims":
		return "Perform a detailed claims and technical breakdown. Explain the scope and boundaries of the independent claims.",
			"AI Claims Breakdown"
	case "risk":
		return "Evaluate potential legal & risk factors. Identify prior art exposure, design-around vectors, or potential infringement vulnerabilities.",
			"AI Legal & Risk takeaways"
	}
	return "", locator
}

// Provider represents a supported AI platform.
type Provider string

const (
	ProviderGemini Provider = "gemini"
	ProviderOllama Provider = "ollama"
)

// Analyzer defines the technical contract for performing AI-driven patent
// analysis, summary generation, or note evaluations.
type Analyzer interface {
	// AnalyzePatent takes a patent and optional custom instruction/prompt,
	// evaluating key bibliographic details, abstract, first claim and notes,
	// returning a detailed AI text summary or report.
	AnalyzePatent(ctx context.Context, patent domain.Patent, prompt string) (string, error)

	// Provider returns the identifier for this analyzer's provider platform.
	Provider() Provider

	// IsConfigured returns true if the required API keys or endpoints are configured.
	// If false, it returns a user-facing helper/link on how to obtain or set up credentials.
	IsConfigured() (bool, string)
}
