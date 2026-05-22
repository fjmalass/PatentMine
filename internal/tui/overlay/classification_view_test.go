package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

func TestClassificationListOverlayNavigationAndAdding(t *testing.T) {
	theme := render.NewTheme()
	catalog := text.English()

	o := &ClassificationListOverlay{
		theme:   theme,
		catalog: catalog,
		classifications: []domain.Classification{
			{System: "CPC", Code: "G06F16/00", Section: "G", Class: "06", Subclass: "F", MainGroup: "16", Subgroup: "00", Description: "Information retrieval"},
			{System: "CPC", Code: "G06F16/24", Section: "G", Class: "06", Subclass: "F", MainGroup: "16", Subgroup: "24", Description: "Query processing"},
		},
	}

	// 1. Verify Title
	if o.Title() != "Patent Classification Registry" {
		t.Fatalf("Expected Title 'Patent Classification Registry', got %q", o.Title())
	}

	// 2. Test scrolling down
	_, _, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if !handled {
		t.Fatal("Expected key 'j' to be handled")
	}
	if o.selected != 1 {
		t.Fatalf("Expected selection to be 1, got %d", o.selected)
	}

	// 3. Test scrolling up
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if !handled {
		t.Fatal("Expected key 'k' to be handled")
	}
	if o.selected != 0 {
		t.Fatalf("Expected selection to be 0, got %d", o.selected)
	}

	// 4. Test enter add-mode
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !handled {
		t.Fatal("Expected key 'a' to be handled")
	}
	if !o.adding {
		t.Fatal("Expected overlay to be in adding mode")
	}

	// 5. Test typing characters in add-mode
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	if !handled {
		t.Fatal("Expected key 'G' to be handled")
	}
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("0")})
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")})
	if o.inputValue != "G06" {
		t.Fatalf("Expected input value to be 'G06', got %q", o.inputValue)
	}

	// 6. Test escape cancels add-mode
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !handled {
		t.Fatal("Expected escape to be handled")
	}
	if o.adding {
		t.Fatal("Expected adding mode to be cancelled")
	}

	// 7. Test rendering of the dual columns
	viewStr := o.View(80, 24)
	if !strings.Contains(viewStr, "G06F16/00") {
		t.Fatal("Expected view string to contain G06F16/00 code")
	}
	if !strings.Contains(viewStr, "Information retrieval") {
		t.Fatal("Expected view string to contain classification description")
	}
	if !strings.Contains(viewStr, "Metadata Definition Card") {
		t.Fatal("Expected view string to contain sidebar title")
	}
}
