package ai

import (
	"context"
	"patentmine/internal/domain"
)

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
