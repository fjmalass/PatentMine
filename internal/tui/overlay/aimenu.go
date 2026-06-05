package overlay

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/ai"
	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

// AIAnalyzeTriggerMsg requests that the App trigger a background AI analysis.
type AIAnalyzeTriggerMsg struct {
	Patent       domain.Patent
	Provider     ai.Provider
	ActionType   string // "novelty", "claims", "risk", "custom"
	CustomPrompt string
}

// AISwitchProviderMsg requests that the App update its default AI provider in configuration.
type AISwitchProviderMsg struct {
	NewProvider string // "gemini", "ollama"
}

// PurposeAIAnalyzeCustom is the textinput purpose for entering a custom prompt.
const PurposeAIAnalyzeCustom Purpose = "ai-analyze-custom"

// AIMenu is a popup overlay for choosing patent analysis parameters and model selection.
type AIMenu struct {
	theme        render.Theme
	patent       domain.Patent
	provider     ai.Provider
	analyzer     ai.Analyzer
	configured   bool
	recoveryInfo string
}

// NewAIMenu builds a new AI menu popup.
func NewAIMenu(theme render.Theme, patent domain.Patent, provider ai.Provider, analyzer ai.Analyzer) *AIMenu {
	configured := true
	var recovery string
	if analyzer != nil {
		configured, recovery = analyzer.IsConfigured()
	} else {
		configured = false
		recovery = "AI Analyzer is not initialized. Please verify your environment configuration."
	}

	return &AIMenu{
		theme:        theme,
		patent:       patent,
		provider:     provider,
		analyzer:     analyzer,
		configured:   configured,
		recoveryInfo: recovery,
	}
}

// Patent returns the patent being analyzed.
func (a *AIMenu) Patent() domain.Patent {
	return a.patent
}

// Title implements Overlay.
func (a *AIMenu) Title() string {
	return "AI Patent Curation & Analysis"
}

// Command implements Overlay: AIMenu consumes all input via HandleKey.
func (a *AIMenu) Command(command.ID, int) (Overlay, tea.Cmd) {
	return a, nil
}

// Handles implements Overlay.
func (a *AIMenu) Handles() []command.ID {
	return nil
}

// HandleKey processes keyboard shortcuts inside the AI menu.
func (a *AIMenu) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	keyStr := strings.ToLower(msg.String())

	// Universal exit keys
	if keyStr == "q" || keyStr == "esc" {
		return a, func() tea.Msg { return CloseOverlayMsg{} }, true
	}

	// Switch providers keys (always allowed)
	if keyStr == "o" {
		return a, func() tea.Msg { return AISwitchProviderMsg{NewProvider: "ollama"} }, true
	}
	if keyStr == "g" {
		return a, func() tea.Msg { return AISwitchProviderMsg{NewProvider: "gemini"} }, true
	}
	if keyStr == "p" {
		return a, func() tea.Msg { return AISwitchProviderMsg{NewProvider: "openai"} }, true
	}

	// If the provider is not configured, we only allow switching providers or exiting
	if !a.configured {
		return a, nil, true
	}

	// Action choices
	switch keyStr {
	case "1":
		return a, func() tea.Msg {
			return AIAnalyzeTriggerMsg{
				Patent:     a.patent,
				Provider:   a.provider,
				ActionType: "novelty",
			}
		}, true
	case "2":
		return a, func() tea.Msg {
			return AIAnalyzeTriggerMsg{
				Patent:     a.patent,
				Provider:   a.provider,
				ActionType: "claims",
			}
		}, true
	case "3":
		return a, func() tea.Msg {
			return AIAnalyzeTriggerMsg{
				Patent:     a.patent,
				Provider:   a.provider,
				ActionType: "risk",
			}
		}, true
	case "4":
		// Emit text input trigger for custom prompt
		return a, func() tea.Msg {
			return TextSubmitMsg{
				Purpose: PurposeAIAnalyzeCustom,
				Value:   "", // Will prompt input overlay
			}
		}, true
	}

	return a, nil, false
}

// View draws the AI Menu or the Credential Recovery dialog.
func (a *AIMenu) View(maxW, _ int) string {
	var b strings.Builder

	// Header displaying current provider
	b.WriteString(a.theme.Dim.Render("Active Provider: "))
	switch a.provider {
	case ai.ProviderOllama:
		b.WriteString(a.theme.Title.Render("Local Ollama (Mistral LLM)"))
	case ai.ProviderOpenAI:
		b.WriteString(a.theme.Title.Render("OpenAI API"))
	default:
		b.WriteString(a.theme.Title.Render("Google Gemini API"))
	}
	b.WriteString("\n\n")

	if !a.configured {
		// --- RENDER CREDENTIAL RECOVERY INTERACTIVE VIEW ---
		b.WriteString(a.theme.Error.Render("⚠️  PROVIDER IS NOT CONFIGURED / UNREACHABLE"))
		b.WriteString("\n\n")

		// Wrap and indent the recovery/onboarding guidance
		lines := strings.Split(a.recoveryInfo, "\n")
		for _, line := range lines {
			b.WriteString(a.theme.Row.Render("   " + render.Truncate(line, maxW-5)))
			b.WriteString("\n")
		}
		b.WriteString("\n")

		b.WriteString(a.theme.Dim.Render("How would you like to proceed?"))
		b.WriteString("\n\n")

		b.WriteString(a.theme.HelpKey.Render("[g]"))
		b.WriteString(a.theme.Row.Render(" Switch to Gemini Cloud API    "))
		b.WriteString(a.theme.HelpKey.Render("[o]"))
		b.WriteString(a.theme.Row.Render(" Switch to Local Ollama    "))
		b.WriteString(a.theme.HelpKey.Render("[p]"))
		b.WriteString(a.theme.Row.Render(" Switch to OpenAI API"))
		b.WriteString("\n")
		b.WriteString(a.theme.HelpKey.Render("[q/esc]"))
		b.WriteString(a.theme.Row.Render(" Close Menu"))

		return b.String()
	}

	// --- RENDER STANDARD ACTIVE MENU ---
	b.WriteString(a.theme.Header.Render("Select an AI Patent Curation Action:"))
	b.WriteString("\n\n")

	options := []struct {
		key  string
		desc string
	}{
		{"1", "Executive Novelty Summary"},
		{"2", "Detailed Claims & Technical Breakdown"},
		{"3", "Legal & Risk Factors Takeaways"},
		{"4", "Dynamic Custom Instruction Prompt..."},
	}

	for _, opt := range options {
		b.WriteString("  ")
		b.WriteString(a.theme.HelpKey.Render("[" + opt.key + "]"))
		b.WriteString(" ")
		b.WriteString(a.theme.Row.Render(opt.desc))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(a.theme.Header.Render("Settings & Providers:"))
	b.WriteString("\n\n")

	switch a.provider {
	case ai.ProviderGemini:
		b.WriteString("  ")
		b.WriteString(a.theme.HelpKey.Render("[o]"))
		b.WriteString(" ")
		b.WriteString(a.theme.Row.Render("Switch to Local LLM (Ollama)\n"))
		b.WriteString("  ")
		b.WriteString(a.theme.HelpKey.Render("[p]"))
		b.WriteString(" ")
		b.WriteString(a.theme.Row.Render("Switch to OpenAI API"))
		b.WriteString("\n")
	case ai.ProviderOllama:
		b.WriteString("  ")
		b.WriteString(a.theme.HelpKey.Render("[g]"))
		b.WriteString(" ")
		b.WriteString(a.theme.Row.Render("Switch to Google Gemini Cloud API\n"))
		b.WriteString("  ")
		b.WriteString(a.theme.HelpKey.Render("[p]"))
		b.WriteString(" ")
		b.WriteString(a.theme.Row.Render("Switch to OpenAI API"))
		b.WriteString("\n")
	case ai.ProviderOpenAI:
		b.WriteString("  ")
		b.WriteString(a.theme.HelpKey.Render("[g]"))
		b.WriteString(" ")
		b.WriteString(a.theme.Row.Render("Switch to Google Gemini Cloud API\n"))
		b.WriteString("  ")
		b.WriteString(a.theme.HelpKey.Render("[o]"))
		b.WriteString(" ")
		b.WriteString(a.theme.Row.Render("Switch to Local LLM (Ollama)"))
		b.WriteString("\n")
	}

	b.WriteString("  ")
	b.WriteString(a.theme.HelpKey.Render("[q/esc]"))
	b.WriteString(" ")
	b.WriteString(a.theme.Row.Render("Cancel & Close"))

	return b.String()
}
