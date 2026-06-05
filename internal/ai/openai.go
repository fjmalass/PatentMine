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

	"patentmine/internal/domain"
)

const (
	openAIBaseURL = "https://api.openai.com/v1/chat/completions"
)

// OpenAIAnalyzer implements the Analyzer and Drafter interfaces using OpenAI's API.
type OpenAIAnalyzer struct {
	apiKey string
	model  string
	client *http.Client
}

// NewOpenAIAnalyzer builds a new OpenAI API-backed patent analyzer.
func NewOpenAIAnalyzer(apiKey, model string) *OpenAIAnalyzer {
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &OpenAIAnalyzer{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{
			Timeout:   httpTimeout,
			Transport: NewRedactedRoundTripper(nil),
		},
	}
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiRequest struct {
	Model    string          `json:"model"`
	Messages []openaiMessage `json:"messages"`
}

type openaiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// AnalyzePatent formats a patent's bibliographic data, abstract, first claim,
// and optional notes/prompts, evaluates them, and retrieves a detailed AI analysis.
func (o *OpenAIAnalyzer) AnalyzePatent(ctx context.Context, patent domain.Patent, prompt string) (string, error) {
	if o.apiKey == "" {
		return "", errors.New("ai/openai: API Key is required")
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

	return o.generate(ctx, sb.String())
}

// Complete runs a single completion against the provided prompt and returns the
// model's text.
func (o *OpenAIAnalyzer) Complete(ctx context.Context, prompt string) (string, error) {
	if o.apiKey == "" {
		return "", errors.New("ai/openai: API Key is required")
	}
	return o.generate(ctx, prompt)
}

// Model returns the identifier of the underlying model.
func (o *OpenAIAnalyzer) Model() string { return o.model }

// Provider implements Analyzer.
func (o *OpenAIAnalyzer) Provider() Provider {
	return ProviderOpenAI
}

// IsConfigured implements Analyzer.
func (o *OpenAIAnalyzer) IsConfigured() (bool, string) {
	if o.apiKey == "" {
		return false, "Set OpenAI API key as OPENAI_API_KEY in ~/.ssh/patentmine/.env or your local configuration"
	}
	return true, ""
}

// generate performs one chat completion call with the given prompt text.
func (o *OpenAIAnalyzer) generate(ctx context.Context, prompt string) (string, error) {
	payload := openaiRequest{
		Model: o.model,
		Messages: []openaiMessage{
			{Role: "user", Content: prompt},
		},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("ai/openai: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIBaseURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("ai/openai: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", o.apiKey))

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai/openai: POST request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ai/openai: unexpected HTTP status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ai/openai: read response: %w", err)
	}

	var respPayload openaiResponse
	if err := json.Unmarshal(body, &respPayload); err != nil {
		return "", fmt.Errorf("ai/openai: decode response: %w", err)
	}

	if len(respPayload.Choices) == 0 {
		return "", fmt.Errorf("ai/openai: empty choices response from model")
	}
	return respPayload.Choices[0].Message.Content, nil
}
