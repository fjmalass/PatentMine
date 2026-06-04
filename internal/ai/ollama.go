package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"patentmine/internal/domain"
)

const (
	ollamaTimeout     = 60 * time.Second
	ollamaPingTimeout = 300 * time.Millisecond
)

// OllamaAnalyzer implements the Analyzer interface using a local Ollama instance.
type OllamaAnalyzer struct {
	host   string
	model  string
	client *http.Client
}

// NewOllamaAnalyzer builds a new Ollama client for local LLM patent evaluation.
func NewOllamaAnalyzer(host, model string) *OllamaAnalyzer {
	if host == "" {
		host = "http://localhost:11434"
	}
	if model == "" {
		model = "mistral"
	}
	// Strip trailing slashes
	host = strings.TrimSuffix(host, "/")

	return &OllamaAnalyzer{
		host:   host,
		model:  model,
		client: &http.Client{Timeout: ollamaTimeout},
	}
}

type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResponse struct {
	Model    string `json:"model"`
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// AnalyzePatent prompts the local Ollama model to evaluate a patent and notes.
func (o *OllamaAnalyzer) AnalyzePatent(ctx context.Context, patent domain.Patent, prompt string) (string, error) {
	var sb strings.Builder
	sb.WriteString("You are Antigravity, a professional patent attorney and technology analyst AI.\n")
	sb.WriteString(fmt.Sprintf("Analyze the patent details provided below using your %s model and return an executive-level analysis summary.\n\n", o.model))
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
// local model's text. It is the generic entry point the drafting subsystem uses
// (the caller constructs the grounded prompt via ai.BuildDraftPrompt). Running
// locally, it keeps unpublished application content off third-party services.
func (o *OllamaAnalyzer) Complete(ctx context.Context, prompt string) (string, error) {
	return o.generate(ctx, prompt)
}

// Model returns the local model name, for draft provenance.
func (o *OllamaAnalyzer) Model() string { return o.model }

// generate performs one /api/generate call with the given prompt text.
func (o *OllamaAnalyzer) generate(ctx context.Context, prompt string) (string, error) {
	reqPayload := ollamaRequest{
		Model:  o.model,
		Prompt: prompt,
		Stream: false,
	}

	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("ai/ollama: encode request: %w", err)
	}

	url := fmt.Sprintf("%s/api/generate", o.host)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("ai/ollama: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai/ollama: local request failed (is Ollama running?): %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ai/ollama: unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ai/ollama: read response: %w", err)
	}

	var respPayload ollamaResponse
	if err := json.Unmarshal(body, &respPayload); err != nil {
		return "", fmt.Errorf("ai/ollama: decode response: %w", err)
	}

	return respPayload.Response, nil
}

// Provider implements Analyzer.
func (o *OllamaAnalyzer) Provider() Provider {
	return ProviderOllama
}

// IsConfigured implements Analyzer.
func (o *OllamaAnalyzer) IsConfigured() (bool, string) {
	// Ping the Ollama API with a very short timeout to check if the daemon is alive
	pingClient := &http.Client{Timeout: ollamaPingTimeout}
	resp, err := pingClient.Get(o.host)
	if err != nil {
		return false, fmt.Sprintf("Local Ollama daemon is unreachable on %s.\n"+
			"1. Run 'cargo make ollama-setup' in your terminal to install Ollama and download the '%s' model.\n"+
			"2. Ensure the Ollama background daemon or desktop app is running.", o.host, o.model)
	}
	_ = resp.Body.Close()

	// Alternatively, verify if the mistral model is installed by hitting /api/tags
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/tags", o.host), nil)
	if err == nil {
		if tagResp, tagErr := pingClient.Do(req); tagErr == nil {
			defer func() { _ = tagResp.Body.Close() }()
			type tagsResponse struct {
				Models []struct {
					Name string `json:"name"`
				} `json:"models"`
			}
			var tr tagsResponse
			if body, readErr := io.ReadAll(tagResp.Body); readErr == nil {
				if json.Unmarshal(body, &tr) == nil {
					found := false
					for _, m := range tr.Models {
						if strings.HasPrefix(m.Name, o.model) {
							found = true
							break
						}
					}
					if !found {
						return false, fmt.Sprintf("Ollama is running, but the '%s' model is not pulled.\n"+
							"Run 'cargo make ollama-setup' or 'ollama pull %s' in your terminal to fetch it.", o.model, o.model)
					}
				}
			}
		}
	}

	return true, ""
}
