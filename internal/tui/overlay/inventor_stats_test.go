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
		theme:         theme,
		catalog:       catalog,
		patent:        patent,
		project:       "proj-1",
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
		selected:      0,
		loading:       false,
		patentsPage:   render.NewPaginator(5),
		activeSort:    domain.SortByNumber,
		sortAscending: true,
		focusedColIdx: -1,
		lastWidth:     120,
	}

	// 1. Verify title
	if o.Title() != "Inventor Analytics" {
		t.Errorf("Expected title 'Inventor Analytics', got %q", o.Title())
	}

	// 2. Test scrolling down (which triggers patents loading)
	_, cmd, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if !handled {
		t.Fatal("Expected key 'j' to be handled")
	}
	if o.selected != 1 {
		t.Fatalf("Expected selection to be 1, got %d", o.selected)
	}
	if cmd == nil {
		t.Error("Expected an asynchronous loadPatentsCmd when selecting a new inventor")
	}

	// 3. Test scrolling up
	_, cmd, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if !handled {
		t.Fatal("Expected key 'k' to be handled")
	}
	if o.selected != 0 {
		t.Fatalf("Expected selection to be 0, got %d", o.selected)
	}
	if cmd == nil {
		t.Error("Expected an asynchronous loadPatentsCmd when selecting a new inventor")
	}

	// 4. Test escape key returns CloseOverlayMsg
	_, cmd, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
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

	// 5. Test Tab key focuses patents list
	if o.focus != focusInventors {
		t.Errorf("Expected initial focus on inventors")
	}
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyTab})
	if !handled {
		t.Fatal("Expected tab to be handled")
	}
	if o.focus != focusPatents {
		t.Errorf("Expected Tab to toggle focus to patents")
	}

	// 6. Test shifting column focus and sorting
	// Move column focus right (should focus Title at idx 1)
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if !handled {
		t.Fatal("Expected key 'l' to move column focus right")
	}
	if o.focusedColIdx != 1 {
		t.Errorf("Expected focused column to be 1 (Title), got %d", o.focusedColIdx)
	}

	// Move column focus left (should focus Number at idx 0)
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if !handled {
		t.Fatal("Expected key 'h' to move column focus left")
	}
	if o.focusedColIdx != 0 {
		t.Errorf("Expected focused column to be 0 (Number), got %d", o.focusedColIdx)
	}

	// Trigger sorting on focused column with "."
	_, sortCmd, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	if !handled || sortCmd == nil {
		t.Fatal("Expected key '.' to trigger sorting on focused column")
	}

	// 7. Provide patents data to test patent list interactions
	o.patents = []domain.PatentRow{
		{
			Number:        domain.MustParsePatentNumber("US8000000B1"),
			DisplayNumber: domain.MustParsePatentNumber("US8000000B1"),
			Title:         "Awesome Method",
			ReviewState:   domain.ReviewStateStored,
			FetchState:    domain.FetchCached,
		},
		{
			Number:        domain.MustParsePatentNumber("US9000000B1"),
			DisplayNumber: domain.MustParsePatentNumber("US9000000B1"),
			Title:         "Incredible Device",
			ReviewState:   domain.ReviewStateUnderReview,
			FetchState:    domain.FetchCached,
		},
	}
	o.patentsPage.SetTotal(2)

	// Test scrolling on patents when patents list is focused
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if !handled {
		t.Fatal("Expected key 'j' on patents to be handled")
	}
	if o.patentsPage.Cursor() != 1 {
		t.Errorf("Expected patents cursor to be 1, got %d", o.patentsPage.Cursor())
	}

	// Test hotkeys for review state and tags on the selected patent row
	// Selected patent at cursor 1 is US9000000B1
	_, stateCmd, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if !handled || stateCmd == nil {
		t.Fatal("Expected state transition key 's' to be handled and command to be returned")
	}

	// Test tag manager hotkey 't'
	_, tagCmd, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if !handled || tagCmd == nil {
		t.Fatal("Expected tag hotkey 't' to be handled")
	}
	tagMsg := tagCmd()
	tagOverlayMsg, ok := tagMsg.(OpenTagPatentOverlayMsg)
	if !ok {
		t.Fatalf("Expected OpenTagPatentOverlayMsg, got %T", tagMsg)
	}
	if len(tagOverlayMsg.Patents) != 1 || tagOverlayMsg.Patents[0].String() != "US9000000B1" {
		t.Errorf("Expected tag overlay msg targeting patent US9000000B1, got %v", tagOverlayMsg.Patents)
	}

	// 8. Verify view rendering contains stats and divider
	viewStr := o.View(120, 20)
	if !strings.Contains(viewStr, "John Doe") {
		t.Errorf("Expected view to render inventor John Doe")
	}
	if !strings.Contains(viewStr, "Patents by Selected Inventor") {
		t.Errorf("Expected view to render patents divider")
	}
	if !strings.Contains(viewStr, "Awesome Method") {
		t.Errorf("Expected view to render patent row for selected inventor")
	}

	// 8. Test 90% overlay sizing responsive logic
	w, h := o.OverlaySize(100, 40)
	if w != 90 || h != 36 {
		t.Errorf("Expected overlay size 90x36 (90%% of 100x40), got %dx%d", w, h)
	}
}

func TestRaceConditionMitigation(t *testing.T) {
	theme := render.NewTheme()
	catalog := text.English()
	patent := domain.Patent{
		Number: domain.MustParsePatentNumber("US10000000B2"),
	}

	o := &InventorStatsOverlay{
		theme:   theme,
		catalog: catalog,
		patent:  patent,
		loadID:  2,
	}

	// Receive loadedPatentListMsg with a matching requestID
	o.Update(loadedPatentListMsg{
		requestID: 2,
		patents: []domain.PatentRow{
			{
				Number:      domain.MustParsePatentNumber("US8000000B1"),
				ReviewState: domain.ReviewStateStored,
				FetchState:  domain.FetchCached,
			},
		},
		total: 1,
	})

	if len(o.patents) != 1 {
		t.Errorf("Expected patents to load when requestID matches")
	}

	// Receive loadedPatentListMsg with an obsolete requestID (1 < 2)
	o.Update(loadedPatentListMsg{
		requestID: 1,
		patents: []domain.PatentRow{
			{
				Number:      domain.MustParsePatentNumber("US9000000B1"),
				ReviewState: domain.ReviewStateStored,
				FetchState:  domain.FetchCached,
			},
		},
		total: 10,
	})

	if len(o.patents) != 1 || o.patents[0].Number.String() != "US8000000B1" {
		t.Errorf("Race condition mitigation failed: obsolete request loaded")
	}
}

func TestAdaptiveColumnHidingAndClamping(t *testing.T) {
	theme := render.NewTheme()
	catalog := text.English()
	patent := domain.Patent{
		Number: domain.MustParsePatentNumber("US10000000B2"),
	}

	o := &InventorStatsOverlay{
		theme:         theme,
		catalog:       catalog,
		patent:        patent,
		focusedColIdx: 4, // Focused on "Tags" (index 4) originally
		lastWidth:     100,
	}

	// 1. Verify 6 columns are visible at width 100
	cols := o.currentCols()
	if len(cols) != 6 {
		t.Errorf("Expected 6 columns at width 100, got %d", len(cols))
	}

	// 2. Perform resize to terminal width 76 (which results in inner width 70, hiding "Tags")
	// "Tags" at idx 4 is now hidden, the new columns set has length 5.
	// Clamping should adjust focusedColIdx to the nearest sortable column.
	o.Update(tea.WindowSizeMsg{Width: 76, Height: 40})
	
	cols = o.currentCols()
	if len(cols) != 5 {
		t.Errorf("Expected 5 columns at inner width 70 (terminal width 76), got %d", len(cols))
	}
	if o.focusedColIdx >= 5 {
		t.Errorf("Expected focused column to be clamped to a valid index, got %d", o.focusedColIdx)
	}
}

