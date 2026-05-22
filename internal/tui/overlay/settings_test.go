package overlay

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/ai"
	"patentmine/internal/tui/render"
)

func TestSettingsOverlay_HandleKey(t *testing.T) {
	theme := render.NewTheme()
	s := NewSettingsOverlay(theme, ai.ProviderGemini, "fake-gemini-key", "http://localhost:11434", "mistral", true)

	if s.activeAI != ai.ProviderGemini {
		t.Errorf("expected initial provider to be gemini, got %s", s.activeAI)
	}

	// Press 'o' to switch to Ollama
	updated, cmd, consumed := s.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	if !consumed {
		t.Error("expected key press to be consumed")
	}
	sUpdated, ok := updated.(*SettingsOverlay)
	if !ok {
		t.Fatalf("expected updated overlay to be *SettingsOverlay, got %T", updated)
	}
	if sUpdated.activeAI != ai.ProviderOllama {
		t.Errorf("expected provider to be ollama after pressing 'o', got %s", sUpdated.activeAI)
	}
	if cmd == nil {
		t.Fatal("expected non-nil tea.Cmd command")
	}
	msg := cmd()
	switch m := msg.(type) {
	case AISwitchProviderMsg:
		if m.NewProvider != "ollama" {
			t.Errorf("expected switch message provider to be ollama, got %s", m.NewProvider)
		}
	default:
		t.Fatalf("expected AISwitchProviderMsg, got %T", msg)
	}

	// Press 'g' to switch to Gemini
	updated, cmd, consumed = sUpdated.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if !consumed {
		t.Error("expected key press to be consumed")
	}
	sUpdated, ok = updated.(*SettingsOverlay)
	if !ok {
		t.Fatalf("expected updated overlay to be *SettingsOverlay, got %T", updated)
	}
	if sUpdated.activeAI != ai.ProviderGemini {
		t.Errorf("expected provider to be gemini after pressing 'g', got %s", sUpdated.activeAI)
	}
	if cmd == nil {
		t.Fatal("expected non-nil tea.Cmd command")
	}
	msg = cmd()
	switch m := msg.(type) {
	case AISwitchProviderMsg:
		if m.NewProvider != "gemini" {
			t.Errorf("expected switch message provider to be gemini, got %s", m.NewProvider)
		}
	default:
		t.Fatalf("expected AISwitchProviderMsg, got %T", msg)
	}
}
