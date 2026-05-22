package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

func TestInventorStatsOverlayNavigationAndRendering(t *testing.T) {
	theme := render.NewTheme()
	catalog := text.English()
	patent := domain.Patent{
		Number:        domain.MustParsePatentNumber("US10000000B2"),
		DisplayNumber: domain.MustParsePatentNumber("US10000000B2"),
		Inventors:     []domain.Inventor{"John Doe", "Jane Smith"},
	}

	o := &InventorStatsOverlay{
		theme:   theme,
		catalog: catalog,
		patent:  patent,
		project: "proj-1",
		stats: []domain.InventorStats{
			{
				Inventor: "John Doe",
				Total:    12,
				States: map[string]int{
					"stored":       5,
					"under_review": 3,
					"other":        4,
				},
				Tags: map[string]int{
					"critical": 2,
					"triage":   1,
				},
			},
			{
				Inventor: "Jane Smith",
				Total:    8,
				States: map[string]int{
					"stored": 2,
					"other":  6,
				},
				Tags: nil,
			},
		},
		selected: 0,
		loading:  false,
	}

	// 1. Verify title
	if o.Title() != "Inventor Analytics" {
		t.Errorf("Expected title 'Inventor Analytics', got %q", o.Title())
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

	// 4. Test escape key returns CloseOverlayMsg
	_, cmd, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !handled {
		t.Fatal("Expected escape to be handled")
	}
	if cmd == nil {
		t.Fatal("Expected a command returned on escape")
	}
	msg := cmd()
	if _, ok := msg.(CloseOverlayMsg); !ok {
		t.Errorf("Expected CloseOverlayMsg, got: %T", msg)
	}

	// 5. Test view rendering & exact count format:
	// "[xx]: state: [y] stored, [a] under_review, [o] other, tags: [t1] tag1, [t2] tag2, .."
	viewStr := o.View(120, 20)
	if !strings.Contains(viewStr, "John Doe:") {
		t.Errorf("Expected view to render inventor John Doe")
	}
	if !strings.Contains(viewStr, "[12]:  state: [5] stored, [3] under_review, [4] other, tags: [2] critical, [1] triage") {
		t.Errorf("John Doe stats format mismatch. View:\n%s", viewStr)
	}

	if !strings.Contains(viewStr, "Jane Smith:") {
		t.Errorf("Expected view to render inventor Jane Smith")
	}
	if !strings.Contains(viewStr, "[8]:   state: [2] stored, [0] under_review, [6] other, tags: none") {
		t.Errorf("Jane Smith stats format mismatch. View:\n%s", viewStr)
	}

	// 6. Test overlay sizing
	w, h := o.OverlaySize(100, 40)
	if w != 90 || h != 22 {
		t.Errorf("Expected overlay size 90x22, got %dx%d", w, h)
	}
}
