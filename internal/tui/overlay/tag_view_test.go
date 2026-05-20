package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

func TestTagTaxonomyOverlayNavigationAndAdding(t *testing.T) {
	theme := render.NewTheme()
	catalog := text.English()
	projectID := domain.ProjectID("proj-1")

	// Create overlay without a live client since we will test local interactive keyboard logic
	o := &TagTaxonomyOverlay{
		theme:   theme,
		catalog: catalog,
		project: projectID,
		tags: []domain.Tag{
			{ID: 1, Name: "alpha"},
			{ID: 2, Name: "beta"},
			{ID: 3, Name: "gamma"},
		},
	}

	// 1. Test scrolling down
	_, _, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if !handled {
		t.Fatal("Expected key 'j' to be handled")
	}
	if o.selected != 1 {
		t.Fatalf("Expected selection to be 1, got %d", o.selected)
	}

	// 2. Test scrolling up
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if !handled {
		t.Fatal("Expected key 'k' to be handled")
	}
	if o.selected != 0 {
		t.Fatalf("Expected selection to be 0, got %d", o.selected)
	}

	// 3. Test enter add-mode
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !handled {
		t.Fatal("Expected key 'a' to be handled")
	}
	if !o.adding {
		t.Fatal("Expected overlay to be in adding mode")
	}

	// 4. Test typing characters in add-mode
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if !handled {
		t.Fatal("Expected key 'n' to be handled")
	}
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if o.inputValue != "new" {
		t.Fatalf("Expected input value to be 'new', got %q", o.inputValue)
	}

	// 5. Test snake_case validation on invalid tags
	o.inputValue = "Invalid-Tag"
	o.inputCursor = len("Invalid-Tag")
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Fatal("Expected enter to be handled")
	}
	if o.err == nil {
		t.Fatal("Expected error on invalid snake_case tag name")
	}
	if !strings.Contains(o.err.Error(), "snake_case") {
		t.Fatalf("Expected snake_case error, got: %v", o.err)
	}

	// 6. Test escape cancel add-mode
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !handled {
		t.Fatal("Expected escape to be handled")
	}
	if o.adding {
		t.Fatal("Expected adding mode to be cancelled")
	}
}

func TestTagPatentOverlayTogglingAndMultiSelection(t *testing.T) {
	theme := render.NewTheme()
	catalog := text.English()
	projectID := domain.ProjectID("proj-1")
	patents := []domain.PatentNumber{
		domain.MustParsePatentNumber("US10000000B2"),
		domain.MustParsePatentNumber("US10000001B2"),
	}

	o := &TagPatentOverlay{
		theme:   theme,
		catalog: catalog,
		project: projectID,
		patents: patents,
		available: []domain.Tag{
			{ID: 1, Name: "tag_one"},
			{ID: 2, Name: "tag_two"},
		},
		checked: map[string]bool{
			"tag_one": true,
		},
	}

	// 1. Verify title shows plural for multi-selection
	title := o.Title()
	if !strings.Contains(title, "2 Patents") {
		t.Fatalf("Expected title to mention multi-selection, got %q", title)
	}

	// 2. Toggle the checked state of tag_one (currently selected: 0, checked: true)
	_, _, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	if !handled {
		t.Fatal("Expected spacebar to be handled")
	}
	if o.checked["tag_one"] {
		t.Fatal("Expected tag_one to be unchecked after space toggle")
	}

	// 3. Move down and toggle tag_two
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if o.selected != 1 {
		t.Fatalf("Expected selection to be 1, got %d", o.selected)
	}
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")})
	if !handled {
		t.Fatal("Expected spacebar to be handled")
	}
	if !o.checked["tag_two"] {
		t.Fatal("Expected tag_two to be checked after space toggle")
	}
}
