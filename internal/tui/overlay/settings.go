package overlay

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/ai"
	"patentmine/internal/command"
	"patentmine/internal/tui/render"
)

// SettingsOverlay is an interactive popup overlay showing AI and crawl/search config.
type SettingsOverlay struct {
	theme           render.Theme
	activeAI        ai.Provider
	geminiKey       string
	ollamaHost      string
	ollamaModel     string
	usptoConfigured bool
}

// NewSettingsOverlay builds a settings overlay screen.
func NewSettingsOverlay(theme render.Theme, activeAI ai.Provider, geminiKey, ollamaHost, ollamaModel string, usptoConfigured bool) *SettingsOverlay {
	return &SettingsOverlay{
		theme:           theme,
		activeAI:        activeAI,
		geminiKey:       geminiKey,
		ollamaHost:      ollamaHost,
		ollamaModel:     ollamaModel,
		usptoConfigured: usptoConfigured,
	}
}

// Title implements Overlay.
func (s *SettingsOverlay) Title() string { return "AI & Search Settings" }

// Command implements Overlay.
func (s *SettingsOverlay) Command(command.ID, int) (Overlay, tea.Cmd) { return s, nil }

// Handles implements Overlay.
func (s *SettingsOverlay) Handles() []command.ID { return nil }

// SetActiveAI updates the active AI provider.
func (s *SettingsOverlay) SetActiveAI(provider ai.Provider) {
	s.activeAI = provider
}

// HandleKey processes keyboard toggles inside the settings overlay.
func (s *SettingsOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	keyStr := strings.ToLower(msg.String())
	if keyStr == "q" || keyStr == "esc" {
		return s, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	if keyStr == "o" {
		s.activeAI = ai.ProviderOllama
		return s, func() tea.Msg { return AISwitchProviderMsg{NewProvider: "ollama"} }, true
	}
	if keyStr == "g" {
		s.activeAI = ai.ProviderGemini
		return s, func() tea.Msg { return AISwitchProviderMsg{NewProvider: "gemini"} }, true
	}
	return s, nil, true
}

// maskKey helper masks the API key for premium aesthetics.
func maskKey(key string) string {
	if key == "" {
		return "Not Configured (Missing Key)"
	}
	if len(key) <= 8 {
		return "••••••••"
	}
	return key[:4] + "••••••••" + key[len(key)-4:]
}

// View renders the beautiful settings screen.
func (s *SettingsOverlay) View(maxW, _ int) string {
	var b strings.Builder

	b.WriteString(s.theme.Header.Render("Active Capabilities & Registries"))
	b.WriteString("\n\n")

	// AI Engine Panel
	b.WriteString(s.theme.Title.Render("1. AI Curation Engines"))
	b.WriteString("\n")
	b.WriteString(s.theme.Dim.Render("   Active provider: "))
	if s.activeAI == ai.ProviderGemini {
		b.WriteString(s.theme.Title.Render("Google Gemini API"))
	} else if s.activeAI == ai.ProviderOllama {
		b.WriteString(s.theme.Title.Render("Local Ollama"))
	} else {
		b.WriteString(s.theme.Error.Render("None"))
	}
	b.WriteString("\n")

	b.WriteString(s.theme.Row.Render("   Gemini API Key : " + maskKey(s.geminiKey)))
	b.WriteString("\n")
	b.WriteString(s.theme.Row.Render("   Ollama Host    : " + s.ollamaHost + " (" + s.ollamaModel + ")"))
	b.WriteString("\n\n")

	// Crawl / Search Registry Panel
	b.WriteString(s.theme.Title.Render("2. Crawl & Search Services (Daemon-side)"))
	b.WriteString("\n")
	b.WriteString(s.theme.Row.Render("   Base Patent Crawler (Google) : "))
	b.WriteString(s.theme.OK.Render("Enabled (Default)"))
	b.WriteString("\n")
	b.WriteString(s.theme.Row.Render("   USPTO Official API Key       : "))
	if s.usptoConfigured {
		b.WriteString(s.theme.OK.Render("Configured (Full Access)"))
	} else {
		b.WriteString(s.theme.Dim.Render("Not Configured (Google Fallback)"))
	}
	b.WriteString("\n\n")

	// Toggle controls
	b.WriteString(s.theme.Header.Render("Controls & Hotkeys:"))
	b.WriteString("\n\n")

	b.WriteString("  ")
	b.WriteString(s.theme.HelpKey.Render("[g]"))
	b.WriteString(s.theme.Row.Render(" Switch active AI to Google Gemini Cloud API"))
	b.WriteString("\n")

	b.WriteString("  ")
	b.WriteString(s.theme.HelpKey.Render("[o]"))
	b.WriteString(s.theme.Row.Render(" Switch active AI to Local Ollama Server"))
	b.WriteString("\n")

	b.WriteString("  ")
	b.WriteString(s.theme.HelpKey.Render("[q/esc]"))
	b.WriteString(s.theme.Row.Render(" Close settings and return"))

	return b.String()
}
