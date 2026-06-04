package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"patentmine/internal/domain"
)

const (
	geminiModel   = "gemini-2.5-flash"
	geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models/" + geminiModel + ":generateContent"
	httpTimeout   = 30 * time.Second
)

// GeminiAnalyzer implements the Analyzer interface using Google's Gemini API.
type GeminiAnalyzer struct {
	apiKey string
	client *http.Client
}

// NewGeminiAnalyzer builds a new Gemini API-backed patent analyzer.
func NewGeminiAnalyzer(apiKey string) *GeminiAnalyzer {
	return &GeminiAnalyzer{
		apiKey: apiKey,
		client: &http.Client{
			Timeout:   httpTimeout,
			Transport: NewRedactedRoundTripper(nil),
		},
	}
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// AnalyzePatent formats a patent's bibliographic data, abstract, first claim,
// and optional notes/prompts, evaluates them, and retrieves a detailed AI analysis.
func (g *GeminiAnalyzer) AnalyzePatent(ctx context.Context, patent domain.Patent, prompt string) (string, error) {
	if g.apiKey == "" {
		return "", errors.New("ai/gemini: API Key is required")
	}

	var sb strings.Builder
	sb.WriteString("You are Antigravity, a professional patent attorney and technology analyst AI.\n")
	sb.WriteString("Analyze the patent details provided below and return an executive-level analysis summary.\n\n")
	sb.WriteString(fmt.Sprintf("Patent Number: %s\n", patent.Number.String()))
	sb.WriteString(fmt.Sprintf("Title: %s\n", patent.Title))
	sb.WriteString(fmt.Sprintf("Assignee: %s\n", patent.Assignee))
	if len(patent.Inventors) > 0 {
		var invs []string
		for _, inv := range patent.Inventors {
			invs = append(invs, string(inv))
		}
		sb.WriteString(fmt.Sprintf("Inventors: %s\n", strings.Join(invs, ", ")))
	}
	if !patent.GrantDate.IsZero() {
		sb.WriteString(fmt.Sprintf("Grant Date: %s\n", patent.GrantDate.Format(domain.DateLayout)))
	}
	sb.WriteString(fmt.Sprintf("\nAbstract:\n%s\n", patent.Abstract))
	if patent.FirstClaim != "" {
		sb.WriteString(fmt.Sprintf("\nFirst Claim:\n%s\n", patent.FirstClaim))
	}
	if prompt != "" {
		sb.WriteString(fmt.Sprintf("\nCustom Analysis Focus / Notes:\n%s\n", prompt))
	} else {
		sb.WriteString("\nPlease summarize the core technical novelty of this patent and highlight key legal/technology design takeaways.\n")
	}

	return g.generate(ctx, sb.String())
}

// Complete runs a single completion against the provided prompt and returns the
// model's text. It is the generic, grounding-agnostic entry point used by the
// drafting subsystem; the caller owns prompt construction (see ai.BuildDraftPrompt).
func (g *GeminiAnalyzer) Complete(ctx context.Context, prompt string) (string, error) {
	if g.apiKey == "" {
		return "", errors.New("ai/gemini: API Key is required")
	}
	return g.generate(ctx, prompt)
}

// Model returns the identifier of the underlying model, for draft provenance.
func (g *GeminiAnalyzer) Model() string { return geminiModel }

// generate performs one generateContent call with the given prompt text.
func (g *GeminiAnalyzer) generate(ctx context.Context, prompt string) (string, error) {
	reqPayload := geminiRequest{
		Contents: []geminiContent{{Parts: []geminiPart{{Text: prompt}}}},
	}

	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("ai/gemini: encode request: %w", err)
	}

	url := fmt.Sprintf("%s?key=%s", geminiBaseURL, g.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("ai/gemini: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai/gemini: POST request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ai/gemini: unexpected HTTP status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ai/gemini: read response: %w", err)
	}

	var respPayload geminiResponse
	if err := json.Unmarshal(body, &respPayload); err != nil {
		return "", fmt.Errorf("ai/gemini: decode response: %w", err)
	}

	if len(respPayload.Candidates) == 0 || len(respPayload.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("ai/gemini: empty response from model")
	}
	return respPayload.Candidates[0].Content.Parts[0].Text, nil
}

// Provider implements Analyzer.
func (g *GeminiAnalyzer) Provider() Provider {
	return ProviderGemini
}

// IsConfigured implements Analyzer.
func (g *GeminiAnalyzer) IsConfigured() (bool, string) {
	if g.apiKey == "" {
		return false, "Get a free Google Gemini Developer API key at: https://aistudio.google.com/ and set it as GEMINI_API_KEY in ~/.ssh/patentmine/.env"
	}
	return true, ""
}
