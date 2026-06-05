package api

import (
	"context"
	"net/http"
	"time"

	"patentmine/internal/ai"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
)

// AIConfig holds server-side AI analyzer credentials (never sent to browsers).
type AIConfig struct {
	Provider     string
	GeminiAPIKey string
	OllamaHost   string
	OllamaModel  string
	OpenAIAPIKey string
	OpenAIModel  string
}

func (s *Server) analyzer() ai.Analyzer {
	switch s.ai.Provider {
	case string(ai.ProviderOllama):
		if s.ai.OllamaHost != "" {
			return ai.NewOllamaAnalyzer(s.ai.OllamaHost, s.ai.OllamaModel)
		}
	case string(ai.ProviderOpenAI):
		if s.ai.OpenAIAPIKey != "" {
			return ai.NewOpenAIAnalyzer(s.ai.OpenAIAPIKey, s.ai.OpenAIModel)
		}
	case string(ai.ProviderGemini):
		if s.ai.GeminiAPIKey != "" {
			return ai.NewGeminiAnalyzer(s.ai.GeminiAPIKey)
		}
	}
	if s.ai.GeminiAPIKey != "" {
		return ai.NewGeminiAnalyzer(s.ai.GeminiAPIKey)
	}
	if s.ai.OpenAIAPIKey != "" {
		return ai.NewOpenAIAnalyzer(s.ai.OpenAIAPIKey, s.ai.OpenAIModel)
	}
	if s.ai.OllamaHost != "" {
		return ai.NewOllamaAnalyzer(s.ai.OllamaHost, s.ai.OllamaModel)
	}
	return ai.NewGeminiAnalyzer("")
}

// handleAIConfig reports whether server-side AI is configured.
func (s *Server) handleAIConfig(w http.ResponseWriter, r *http.Request) {
	analyzer := s.analyzer()
	ok, hint := analyzer.IsConfigured()
	writeJSON(w, http.StatusOK, map[string]any{
		"provider":    analyzer.Provider(),
		"configured":  ok,
		"setup_hint":  hint,
	})
}

// handleAIAnalyze runs patent curation on the server and returns the summary text.
func (s *Server) handleAIAnalyze(w http.ResponseWriter, r *http.Request) {
	number, err := domain.ParsePatentNumber(r.PathValue("number"))
	if err != nil {
		badRequest(w, "invalid patent number: "+err.Error())
		return
	}
	var body struct {
		Action string `json:"action"`
		Prompt string `json:"prompt"`
	}
	if r.ContentLength > 0 {
		if !decodeBody(w, r, &body) {
			return
		}
	}
	var patentRes proto.PatentResult
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if err := s.client.Call(ctx, proto.MethodPatentGet, proto.PatentGetParams{Number: number}, &patentRes); err != nil {
		writeError(w, err)
		return
	}
	analyzer := s.analyzer()
	ok, hint := analyzer.IsConfigured()
	if !ok {
		badRequest(w, hint)
		return
	}
	prompt, locator := ai.PromptForAction(body.Action, analyzer.Provider())
	if body.Prompt != "" {
		prompt = body.Prompt
	}
	if prompt == "" {
		badRequest(w, "prompt or action is required")
		return
	}
	text, err := analyzer.AnalyzePatent(ctx, patentRes.Patent, prompt)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"summary":  text,
		"locator":  locator,
		"provider": analyzer.Provider(),
	})
}
