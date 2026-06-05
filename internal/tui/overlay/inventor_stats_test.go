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
					"unknown":      5,
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
					"unknown": 2,
					"other":   6,
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
	if o.focus != focusStats {
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
	// Move column focus right (should focus Kind at idx 1)
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
	if !handled {
		t.Fatal("Expected key 'right' to move column focus right")
	}
	if o.focusedColIdx != 1 {
		t.Errorf("Expected focused column to be 1 (Kind), got %d", o.focusedColIdx)
	}

	// Move column focus right again (should focus Title at idx 2)
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
	if !handled {
		t.Fatal("Expected key 'right' to move column focus right")
	}
	if o.focusedColIdx != 2 {
		t.Errorf("Expected focused column to be 2 (Title), got %d", o.focusedColIdx)
	}

	// Move column focus left (should focus Kind at idx 1)
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyLeft})
	if !handled {
		t.Fatal("Expected key 'left' to move column focus left")
	}
	if o.focusedColIdx != 1 {
		t.Errorf("Expected focused column to be 1 (Kind), got %d", o.focusedColIdx)
	}

	// Move column focus left again (should focus Number at idx 0)
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyLeft})
	if !handled {
		t.Fatal("Expected key 'left' to move column focus left")
	}
	if o.focusedColIdx != 0 {
		t.Errorf("Expected focused column to be 0 (Number), got %d", o.focusedColIdx)
	}

	// Trigger sorting on focused column with "."
	_, sortCmd, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	if !handled || sortCmd == nil {
		t.Fatal("Expected key '.' to trigger sorting on focused column")
	}

	// Test structural pane side-by-side navigation using 'h'
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if !handled || o.focus != focusStats {
		t.Fatal("Expected key 'h' to move focus back to inventors list")
	}

	// Focus back to patents using 'l'
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if !handled || o.focus != focusPatents {
		t.Fatal("Expected key 'l' on inventors to focus patents list")
	}

	// 7. Provide patents data to test patent list interactions
	o.patents = []domain.PatentRow{
		{
			Number:        domain.MustParsePatentNumber("US8000000B1"),
			DisplayNumber: domain.MustParsePatentNumber("US8000000B1"),
			Title:         "Awesome Method",
			ReviewState:   domain.ReviewStateUnknown,
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
				ReviewState: domain.ReviewStateUnknown,
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
				ReviewState: domain.ReviewStateUnknown,
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
		focusedColIdx: 5, // Focused on "Tags" (index 5) originally
		lastWidth:     100,
	}

	// 1. Verify 7 columns are visible at width 100
	cols := o.currentCols()
	if len(cols) != 7 {
		t.Errorf("Expected 7 columns at width 100, got %d", len(cols))
	}

	// 2. Perform resize to terminal width 76 (which results in inner width 70, hiding "Tags")
	// "Tags" at idx 5 is now hidden, the new columns set has length 6.
	// Clamping should adjust focusedColIdx to the nearest sortable column.
	o.Update(tea.WindowSizeMsg{Width: 76, Height: 40})

	cols = o.currentCols()
	if len(cols) != 6 {
		t.Errorf("Expected 6 columns at inner width 70 (terminal width 76), got %d", len(cols))
	}
	if o.focusedColIdx >= 6 {
		t.Errorf("Expected focused column to be clamped to a valid index, got %d", o.focusedColIdx)
	}
}

func TestInventorStatsOverlayPaging(t *testing.T) {
	theme := render.NewTheme()
	catalog := text.English()
	patent := domain.Patent{
		Number: domain.MustParsePatentNumber("US10000000B2"),
	}

	o := &InventorStatsOverlay{
		theme:   theme,
		catalog: catalog,
		patent:  patent,
		project: "proj-1",
		stats: []domain.InventorStats{
			{Inventor: "John Doe", Total: 12},
		},
		selected:      0,
		loading:       false,
		focus:         focusPatents,
		patentsPage:   render.NewPaginator(5),
		activeSort:    domain.SortByNumber,
		sortAscending: true,
		focusedColIdx: -1,
		lastWidth:     120,
	}

	// Suppose total matching is 7, page size is 5.
	o.patentsPage.SetTotal(7)

	// Set patents to hold the second page (indices 5, 6)
	// We simulate page 2 being loaded. In Paginator, offset is 5, page size is 5.
	// So we scroll to index 5
	o.patentsPage.ScrollTo(5)

	// Now o.patents holds the 5 patents for the page starting at offset 2 (indices 2 to 6)
	o.patents = []domain.PatentRow{
		{Number: domain.MustParsePatentNumber("US1000000B1")},
		{Number: domain.MustParsePatentNumber("US2000000B1")},
		{Number: domain.MustParsePatentNumber("US3000000B1")},
		{
			Number:      domain.MustParsePatentNumber("US6000000B1"),
			Title:       "Patent Page 2 Item 1",
			ReviewState: domain.ReviewStateUnknown,
			FetchState:  domain.FetchCached,
		},
		{
			Number:      domain.MustParsePatentNumber("US7000000B1"),
			Title:       "Patent Page 2 Item 2",
			ReviewState: domain.ReviewStateUnknown,
			FetchState:  domain.FetchCached,
		},
	}

	// Verify that o.patentsPage.CursorInPage() is 3 (cursor is 5, offset is 2)
	if o.patentsPage.CursorInPage() != 3 {
		t.Errorf("Expected CursorInPage to be 3, got %d", o.patentsPage.CursorInPage())
	}

	// Pressing 's' on the first item of page 2 should succeed and not early-exit!
	_, stateCmd, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if !handled || stateCmd == nil {
		t.Fatal("Expected state transition key 's' to be handled when cursor is on page 2")
	}

	// Move cursor down inside page 2 (index 6, which is CursorInPage = 4)
	o.patentsPage.MoveDown(1)
	if o.patentsPage.CursorInPage() != 4 {
		t.Errorf("Expected CursorInPage to be 4, got %d", o.patentsPage.CursorInPage())
	}

	// Pressing 's' on the second item should also succeed
	_, stateCmd, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if !handled || stateCmd == nil {
		t.Fatal("Expected state transition key 's' to be handled on second item of page 2")
	}

	// View rendering check for page 2
	viewStr := o.View(120, 20)
	if !strings.Contains(viewStr, "Patent Page 2 Item 1") {
		t.Errorf("Expected view to render 'Patent Page 2 Item 1' when page 2 is active")
	}
	if !strings.Contains(viewStr, "Patent Page 2 Item 2") {
		t.Errorf("Expected view to render 'Patent Page 2 Item 2' when page 2 is active")
	}
}

func TestNewInventorStatsOverlayDirectFocus(t *testing.T) {
	theme := render.NewTheme()
	catalog := text.English()
	patent := domain.Patent{
		Number: domain.MustParsePatentNumber("US10000000B2"),
	}

	o, _ := NewInventorStatsOverlay(nil, theme, catalog, patent, "proj-1", true)

	if o.focus != focusPatents {
		t.Errorf("Expected focus to be focusPatents, got %v", o.focus)
	}
	if o.focusedColIdx != 0 {
		t.Errorf("Expected initial focusedColIdx to be 0, got %d", o.focusedColIdx)
	}
}

func TestInventorStatsOverlayVisualMode(t *testing.T) {
	theme := render.NewTheme()
	catalog := text.English()
	patent := domain.Patent{
		Number: domain.MustParsePatentNumber("US10000000B2"),
	}

	o := &InventorStatsOverlay{
		theme:   theme,
		catalog: catalog,
		patent:  patent,
		project: "proj-1",
		stats: []domain.InventorStats{
			{Inventor: "John Doe", Total: 12},
		},
		selected:      0,
		loading:       false,
		focus:         focusPatents,
		patentsPage:   render.NewPaginator(5),
		activeSort:    domain.SortByNumber,
		sortAscending: true,
		focusedColIdx: -1,
		lastWidth:     120,
	}

	o.patents = []domain.PatentRow{
		{Number: domain.MustParsePatentNumber("US1000000B1")},
		{Number: domain.MustParsePatentNumber("US2000000B1")},
		{Number: domain.MustParsePatentNumber("US3000000B1")},
	}
	o.patentsPage.SetTotal(3)

	// Pressing 'v' should enter visual mode
	_, _, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if !handled || !o.patentsPage.VisualMode() {
		t.Fatal("Expected key 'v' to enter visual mode")
	}

	// Verify visual range starts at 0 (current cursor)
	start, _, active := o.patentsPage.VisualRange()
	if !active || start != 0 {
		t.Errorf("Expected visual range start to be 0, got %d (active=%t)", start, active)
	}

	// Move cursor down to index 1
	o.patentsPage.MoveDown(1)

	// Verify selections contains indices 0 and 1
	selections := o.selections()
	if len(selections) != 2 {
		t.Fatalf("Expected 2 selected patents, got %d", len(selections))
	}
	if selections[0].String() != "US1000000B1" || selections[1].String() != "US2000000B1" {
		t.Errorf("Unexpected selections: %v", selections)
	}

	// Pressing 'esc' should clear visual mode instead of closing overlay
	_, cmd, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyEsc})
	if !handled || o.patentsPage.VisualMode() {
		t.Fatal("Expected Esc to clear visual mode")
	}
	if cmd != nil {
		t.Error("Expected no command to be returned when clearing visual mode")
	}

	// Verify chord 'g v' restores the last visual selection
	// Pressing 'g'
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if !handled || !o.patentsPage.GPending() {
		t.Fatal("Expected key 'g' to set gPending")
	}

	// Pressing 'v' to restore visual selection
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	if !handled || !o.patentsPage.VisualMode() {
		t.Fatal("Expected key 'v' with gPending to restore visual mode")
	}

	// Check selections are restored (which was indices 0 and 1)
	restoredSelections := o.selections()
	if len(restoredSelections) != 2 {
		t.Errorf("Expected restored selections to have 2 items, got %d", len(restoredSelections))
	}
}

func TestInventorStatsOverlaySourcePatentHighlight(t *testing.T) {
	theme := render.NewTheme()
	catalog := text.English()
	sourcePatentNo := domain.MustParsePatentNumber("US10000000B2")
	patent := domain.Patent{
		Number:        sourcePatentNo,
		DisplayNumber: sourcePatentNo,
		Inventors:     []domain.Inventor{"John Doe"},
	}

	o := &InventorStatsOverlay{
		theme:   theme,
		catalog: catalog,
		patent:  patent,
		project: "proj-1",
		stats: []domain.InventorStats{
			{Inventor: "John Doe", Total: 1},
		},
		selected:      0,
		loading:       false,
		focus:         focusPatents,
		patentsPage:   render.NewPaginator(5),
		activeSort:    domain.SortByNumber,
		sortAscending: true,
		focusedColIdx: -1,
		lastWidth:     120,
	}

	// 1. Setup patents list where one patent matches the source patent
	o.patents = []domain.PatentRow{
		{
			Number:        domain.MustParsePatentNumber("US8000000B1"),
			DisplayNumber: domain.MustParsePatentNumber("US8000000B1"),
			Title:         "Other Patent",
		},
		{
			Number:        sourcePatentNo,
			DisplayNumber: sourcePatentNo,
			Title:         "Source Patent Highlighted",
		},
	}
	o.patentsPage.SetTotal(2)

	// Cursor is at index 0 initially. Row 1 is the marked source patent.
	// Render view and check if the marked icon '⚑' is present in the rendered output.
	viewStr := o.View(120, 20)
	if !strings.Contains(viewStr, "⚑") {
		t.Errorf("Expected marked icon '⚑' to be rendered on the source patent row")
	}

	// 2. Test sorting: when sorted, the row indices change, but the marker should still be correct.
	// We reverse order or move cursor to index 1 (which is the source patent).
	o.patentsPage.MoveDown(1) // Move cursor to the source patent (index 1)

	// Cursor now on source patent row. Prefix composes cursor glyph + mark glyph: ">⚑".
	viewStrCursor := o.View(120, 20)
	if !strings.Contains(viewStrCursor, ">⚑") {
		t.Errorf("Expected combined cursor + marked icon '>⚑' to be rendered when cursor is on the source patent row")
	}
}
