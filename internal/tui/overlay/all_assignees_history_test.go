package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

func TestAllAssigneesHistoryOverlaySorting(t *testing.T) {
	theme := render.NewTheme()
	catalog := text.English()
	patent := domain.Patent{
		Number:        domain.MustParsePatentNumber("US10000000B2"),
		DisplayNumber: domain.MustParsePatentNumber("US10000000B2"),
		Assignee:      "IBM",
	}

	o := &AllAssigneesHistoryOverlay{
		theme:              theme,
		catalog:            catalog,
		patent:             patent,
		project:            "proj-1",
		stats: []domain.AllAssigneesHistory{
			{
				Assignee: "Apple",
				Total:    5,
				States:   map[string]int{"active": 5},
			},
			{
				Assignee: "IBM",
				Total:    12,
				States:   map[string]int{"active": 12},
			},
			{
				Assignee: "Google",
				Total:    8,
				States:   map[string]int{"active": 8},
			},
		},
		selected:           1, // originally IBM (index 1)
		loading:            false,
		focus:              focusStats,
		patentsPage:        render.NewPaginator(5),
		activeSort:         domain.SortByNumber,
		sortAscending:      true,
		focusedColIdx:      -1,
		lastWidth:          120,
		statsSortCol:       "patents",
		statsSortAsc:       false,
		statsFocusedColIdx: 1, // Total/Patents column
	}

	// 1. Initial manual sorting call to verify patents desc (default) behavior.
	// stats: Apple (5), IBM (12), Google (8)
	// After patents desc sort: IBM (12) [idx 0], Google (8) [idx 1], Apple (5) [idx 2]
	// Since selected was IBM (which was at index 1 before sorting), o.selected should become 0!
	o.sortStats()

	if o.stats[0].Assignee != "IBM" || o.stats[1].Assignee != "Google" || o.stats[2].Assignee != "Apple" {
		t.Errorf("Expected patents desc sort order: IBM, Google, Apple. Got: %s, %s, %s",
			o.stats[0].Assignee, o.stats[1].Assignee, o.stats[2].Assignee)
	}

	if o.selected != 0 {
		t.Errorf("Expected selection to preserve IBM at index 0, got selection index %d", o.selected)
	}

	// 2. Test '.' keypress cycling to 'patents' asc.
	// Cycle 1: patents desc -> patents asc
	// After patents asc sort: Apple (5) [idx 0], Google (8) [idx 1], IBM (12) [idx 2]
	// selected should preserve IBM at index 2.
	_, _, handled := o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	if !handled {
		t.Fatal("Expected '.' to be handled in assignee focus mode")
	}

	if o.statsSortCol != "patents" || !o.statsSortAsc {
		t.Errorf("Expected statsSortCol='patents' and statsSortAsc=true. Got: col=%s, asc=%t", o.statsSortCol, o.statsSortAsc)
	}

	if o.stats[0].Assignee != "Apple" || o.stats[1].Assignee != "Google" || o.stats[2].Assignee != "IBM" {
		t.Errorf("Expected patents asc sort order: Apple, Google, IBM. Got: %s, %s, %s",
			o.stats[0].Assignee, o.stats[1].Assignee, o.stats[2].Assignee)
	}

	if o.selected != 2 {
		t.Errorf("Expected selection to preserve IBM at index 2, got selection index %d", o.selected)
	}

	// 3. Test '.' keypress cycling to 'name' asc.
	// Cycle 2: patents asc -> name asc
	// Switch column focus to Name (column 0)
	o.HandleKey(tea.KeyMsg{Type: tea.KeyLeft})
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	if !handled {
		t.Fatal("Expected '.' to be handled")
	}

	if o.statsSortCol != "name" || !o.statsSortAsc {
		t.Errorf("Expected statsSortCol='name' and statsSortAsc=true. Got: col=%s, asc=%t", o.statsSortCol, o.statsSortAsc)
	}

	if o.stats[0].Assignee != "Apple" || o.stats[1].Assignee != "Google" || o.stats[2].Assignee != "IBM" {
		t.Errorf("Expected name asc sort order: Apple, Google, IBM. Got: %s, %s, %s",
			o.stats[0].Assignee, o.stats[1].Assignee, o.stats[2].Assignee)
	}

	if o.selected != 2 {
		t.Errorf("Expected selection to preserve IBM at index 2, got selection index %d", o.selected)
	}

	// 4. Test '.' keypress cycling to 'name' desc.
	// Cycle 3: name asc -> name desc
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	if !handled {
		t.Fatal("Expected '.' to be handled")
	}

	if o.statsSortCol != "name" || o.statsSortAsc {
		t.Errorf("Expected statsSortCol='name' and statsSortAsc=false. Got: col=%s, asc=%t", o.statsSortCol, o.statsSortAsc)
	}

	if o.stats[0].Assignee != "IBM" || o.stats[1].Assignee != "Google" || o.stats[2].Assignee != "Apple" {
		t.Errorf("Expected name desc sort order: IBM, Google, Apple. Got: %s, %s, %s",
			o.stats[0].Assignee, o.stats[1].Assignee, o.stats[2].Assignee)
	}

	if o.selected != 0 {
		t.Errorf("Expected selection to preserve IBM at index 0, got selection index %d", o.selected)
	}

	// 5. Test '.' keypress cycling back to 'patents' desc.
	// Cycle 4: name desc -> patents desc
	// Switch column focus back to Patents (column 1)
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRight})
	// Toggle to patents (defaults to asc first)
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	// Toggle to patents desc
	_, _, handled = o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	if !handled {
		t.Fatal("Expected '.' to be handled")
	}

	if o.statsSortCol != "patents" || o.statsSortAsc {
		t.Errorf("Expected statsSortCol='patents' and statsSortAsc=false. Got: col=%s, asc=%t", o.statsSortCol, o.statsSortAsc)
	}

	// 6. Test Header Rendering with sort arrow indicator
	viewStr := o.View(100, 30)

	// Since statsSortCol is "patents" and statsSortAsc is false,
	// the patents header part should contain "Total ▾"
	if !strings.Contains(viewStr, "Total ▾") {
		t.Errorf("Expected view to contain 'Total ▾' arrow indicator, got:\n%s", viewStr)
	}

	// Toggle back to "name" asc by moving focus to Name column and pressing '.'
	o.HandleKey(tea.KeyMsg{Type: tea.KeyLeft})
	o.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})

	viewStr = o.View(100, 30)
	if !strings.Contains(viewStr, "Assignee ▴") {
		t.Errorf("Expected view to contain 'Assignee ▴' arrow indicator, got:\n%s", viewStr)
	}
}
