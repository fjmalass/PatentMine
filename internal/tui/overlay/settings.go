package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/ai"
	"patentmine/internal/command"
	"patentmine/internal/tui/render"
)

// SettingsOverlay is an interactive popup overlay showing AI and crawl/search config.
// AIEditConfigMsg requests that the App open a TextInput overlay to edit a configuration field.
type AIEditConfigMsg struct {
	Field string
}

// settingsRowCount is the number of selectable config rows, kept in one place
// so cursor wrap, jump navigation, and the Enter field map never drift.
const settingsRowCount = 7

// SettingsOverlay is an interactive popup overlay showing AI and crawl/search config.
type SettingsOverlay struct {
	theme           render.Theme
	activeAI        ai.Provider
	geminiKey       string
	geminiModel     string
	ollamaHost      string
	ollamaModel     string
	openaiKey       string
	openaiModel     string
	usptoConfigured bool
	cursor          int
	jump            JumpNavigator
}

// NewSettingsOverlay builds a settings overlay screen.
func NewSettingsOverlay(theme render.Theme, activeAI ai.Provider, geminiKey, geminiModel, ollamaHost, ollamaModel, openaiKey, openaiModel string, usptoConfigured bool) *SettingsOverlay {
	return &SettingsOverlay{
		theme:           theme,
		activeAI:        activeAI,
		geminiKey:       geminiKey,
		geminiModel:     geminiModel,
		ollamaHost:      ollamaHost,
		ollamaModel:     ollamaModel,
		openaiKey:       openaiKey,
		openaiModel:     openaiModel,
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

// UpdateValues updates the config values rendered by this overlay in-place.
func (s *SettingsOverlay) UpdateValues(geminiKey, geminiModel, ollamaHost, ollamaModel, openaiKey, openaiModel string, usptoConfigured bool) {
	s.geminiKey = geminiKey
	s.geminiModel = geminiModel
	s.ollamaHost = ollamaHost
	s.ollamaModel = ollamaModel
	s.openaiKey = openaiKey
	s.openaiModel = openaiModel
	s.usptoConfigured = usptoConfigured
}

// HandleKey processes keyboard toggles inside the settings overlay.
func (s *SettingsOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if newCursor, handled := s.jump.HandleKey(msg, s.cursor, settingsRowCount); handled {
		s.cursor = newCursor
		return s, nil, true
	}
	rawKey := msg.String()
	switch rawKey {
	case "K":
		return s, func() tea.Msg { return AIEditConfigMsg{Field: "gemini_key"} }, true
	case "G":
		return s, func() tea.Msg { return AIEditConfigMsg{Field: "gemini_model"} }, true
	case "H":
		return s, func() tea.Msg { return AIEditConfigMsg{Field: "ollama_host"} }, true
	case "M":
		return s, func() tea.Msg { return AIEditConfigMsg{Field: "ollama_model"} }, true
	case "O":
		return s, func() tea.Msg { return AIEditConfigMsg{Field: "openai_key"} }, true
	case "N":
		return s, func() tea.Msg { return AIEditConfigMsg{Field: "openai_model"} }, true
	case "U":
		return s, func() tea.Msg { return AIEditConfigMsg{Field: "uspto_key"} }, true
	}

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
	if keyStr == "p" {
		s.activeAI = ai.ProviderOpenAI
		return s, func() tea.Msg { return AISwitchProviderMsg{NewProvider: "openai"} }, true
	}
	switch msg.Type {
	case tea.KeyUp:
		s.cursor = (s.cursor - 1 + settingsRowCount) % settingsRowCount
		return s, nil, true
	case tea.KeyDown:
		s.cursor = (s.cursor + 1) % settingsRowCount
		return s, nil, true
	case tea.KeyEnter:
		fields := []string{"gemini_key", "gemini_model", "ollama_host", "ollama_model", "openai_key", "openai_model", "uspto_key"}
		return s, func() tea.Msg { return AIEditConfigMsg{Field: fields[s.cursor]} }, true
	case tea.KeyRunes:
		switch msg.String() {
		case ";":
			s.jump.Active = true
			s.jump.PendingCount = 0
			s.jump.PendingG = false
			return s, nil, true
		case "k":
			s.cursor = (s.cursor - 1 + settingsRowCount) % settingsRowCount
			return s, nil, true
		case "j":
			s.cursor = (s.cursor + 1) % settingsRowCount
			return s, nil, true
		}
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

	renderRow := func(index int, label, val string) string {
		marker := "  "
		if index == s.cursor {
			marker = "▸ "
		}
		lineNum := index + 1
		gutter := s.jump.GutterPrefix(lineNum)
		rowText := fmt.Sprintf("%s%s%-18s : %s", marker, gutter, label, val)
		if index == s.cursor {
			return s.theme.Selected.Render(render.Truncate(rowText, maxW))
		}
		return s.theme.Row.Render(render.Truncate(rowText, maxW))
	}

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
	} else if s.activeAI == ai.ProviderOpenAI {
		b.WriteString(s.theme.Title.Render("OpenAI API"))
	} else {
		b.WriteString(s.theme.Error.Render("None"))
	}
	b.WriteString("\n")

	b.WriteString(renderRow(0, "Gemini API Key", maskKey(s.geminiKey)))
	b.WriteString("\n")
	b.WriteString(renderRow(1, "Gemini Model", s.geminiModel))
	b.WriteString("\n")
	b.WriteString(renderRow(2, "Ollama Host", s.ollamaHost))
	b.WriteString("\n")
	b.WriteString(renderRow(3, "Ollama Model", s.ollamaModel))
	b.WriteString("\n")
	b.WriteString(renderRow(4, "OpenAI API Key", maskKey(s.openaiKey)))
	b.WriteString("\n")
	b.WriteString(renderRow(5, "OpenAI Model", s.openaiModel))
	b.WriteString("\n\n")

	// Crawl / Search Registry Panel
	b.WriteString(s.theme.Title.Render("2. Crawl & Search Services (Daemon-side)"))
	b.WriteString("\n")
	b.WriteString(s.theme.Row.Render("   Base Patent Crawler (Google) : "))
	b.WriteString(s.theme.OK.Render("Enabled (Default)"))
	b.WriteString("\n")
	usptoVal := "Not Configured (Google Fallback)"
	if s.usptoConfigured {
		usptoVal = "Configured (Full Access)"
	}
	b.WriteString(renderRow(6, "USPTO API Key", usptoVal))
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
	b.WriteString(s.theme.HelpKey.Render("[p]"))
	b.WriteString(s.theme.Row.Render(" Switch active AI to OpenAI API"))
	b.WriteString("\n")

	b.WriteString("  ")
	b.WriteString(s.theme.HelpKey.Render("[Shift+Letter]"))
	b.WriteString(s.theme.Row.Render(" Edit configuration field in-place (e.g. Shift+K, Shift+H)"))
	b.WriteString("\n")

	b.WriteString("  ")
	var hint string
	if s.jump.Active {
		hint = s.jump.HintSuffix(s.cursor, -1, false)
	} else {
		hint = "[q/esc] Close settings and return · [;] jump mode"
	}
	b.WriteString(s.theme.Dim.Render(render.Truncate(hint, maxW)))

	return b.String()
}
