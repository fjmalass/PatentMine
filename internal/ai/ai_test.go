package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"patentmine/internal/domain"
)

func TestGeminiAnalyzerSuccess(t *testing.T) {
	// Start a local mock server to simulate the Gemini API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check headers and URL
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Return simulated successful response
		resp := geminiResponse{}
		resp.Candidates = append(resp.Candidates, struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		}{})
		resp.Candidates[0].Content.Parts = append(resp.Candidates[0].Content.Parts, struct {
			Text string `json:"text"`
		}{Text: "Simulated AI Analysis output for patent."})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Modify target base URL in the analyzer to use the mock server
	analyzer := &GeminiAnalyzer{
		apiKey: "test-api-key",
		client: &http.Client{Timeout: 5 * time.Second},
	}

	// Create test patent
	patent := domain.Patent{
		Number:    domain.MustParsePatentNumber("US10000000B2"),
		Title:     "Widget apparatus",
		Abstract:  "An improved widget.",
		Assignee:  "Acme Corp",
		Inventors: []domain.Inventor{"Jane Doe"},
	}

	// Make HTTP call (override BaseURL path using server.URL)
	// To avoid changing global const, we temporarily wrap g.client using a Transport that redirects all traffic to the server.URL.
	analyzer.client.Transport = &mockTransport{mockServerURL: server.URL}

	res, err := analyzer.AnalyzePatent(context.Background(), patent, "Analyze novelty")
	if err != nil {
		t.Fatalf("AnalyzePatent failed: %v", err)
	}

	if res != "Simulated AI Analysis output for patent." {
		t.Errorf("unexpected analysis result: %q", res)
	}
}

func TestGeminiAnalyzerMissingAPIKey(t *testing.T) {
	analyzer := NewGeminiAnalyzer("")
	patent := domain.Patent{Number: domain.MustParsePatentNumber("US10000000B2")}
	_, err := analyzer.AnalyzePatent(context.Background(), patent, "")
	if err == nil || !strings.Contains(err.Error(), "API Key is required") {
		t.Errorf("expected missing API key error, got: %v", err)
	}
}

type mockTransport struct {
	mockServerURL string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Re-route request to the mock server URL, preserving query parameters and path
	newReq, err := http.NewRequest(req.Method, m.mockServerURL, req.Body)
	if err != nil {
		return nil, err
	}
	newReq.Header = req.Header
	return http.DefaultClient.Do(newReq)
}

func TestOllamaAnalyzerSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models":[{"name":"mistral:latest"}]}`))
			return
		}

		if r.URL.Path == "/api/generate" {
			var reqBody ollamaRequest
			if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
				t.Errorf("failed to decode request body: %v", err)
			}
			if reqBody.Model != "mistral" {
				t.Errorf("expected model mistral, got %s", reqBody.Model)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			resp := ollamaResponse{
				Model:    "mistral",
				Response: "Simulated local Mistral AI summary.",
				Done:     true,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	analyzer := NewOllamaAnalyzer(server.URL, "mistral")
	configured, errStr := analyzer.IsConfigured()
	if !configured {
		t.Fatalf("expected IsConfigured to pass, got: %s", errStr)
	}

	patent := domain.Patent{
		Number:   domain.MustParsePatentNumber("US10000000B2"),
		Title:    "Widget apparatus",
		Abstract: "An improved widget.",
	}

	res, err := analyzer.AnalyzePatent(context.Background(), patent, "Analyze novelty")
	if err != nil {
		t.Fatalf("AnalyzePatent failed: %v", err)
	}

	if res != "Simulated local Mistral AI summary." {
		t.Errorf("unexpected analysis result: %q", res)
	}
}

func TestRedactedRoundTripper(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ensure headers are scrubbed
		if val := r.Header.Get("x-api-key"); val != "[REDACTED]" {
			t.Errorf("expected x-api-key [REDACTED], got %q", val)
		}
		if val := r.Header.Get("Authorization"); val != "[REDACTED]" {
			t.Errorf("expected Authorization [REDACTED], got %q", val)
		}
		// Ensure query parameters are scrubbed
		if val := r.URL.Query().Get("key"); val != "[REDACTED]" {
			t.Errorf("expected ?key=[REDACTED], got %q", val)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{
		Transport: NewRedactedRoundTripper(nil),
	}

	reqUrl := server.URL + "?key=SECRET-VALUE-ABC"
	req, err := http.NewRequest(http.MethodGet, reqUrl, nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}
	req.Header.Set("x-api-key", "SECRET-KEY-123")
	req.Header.Set("Authorization", "Bearer SECRET-TOKEN")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = resp.Body.Close()

	// Verify ScrubURL
	scrubbed := ScrubURL("https://api.uspto.gov/search?key=SECRET_TOKEN&q=something")
	if !strings.Contains(scrubbed, "key=%5BREDACTED%5D") && !strings.Contains(scrubbed, "key=[REDACTED]") {
		t.Errorf("ScrubURL did not scrub the key parameter: %s", scrubbed)
	}
}

