package pane

import (
	"strings"
	"testing"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/filterexpr"
	"patentmine/internal/tui/render"
)

const (
	testPaneWidth  = 120
	testPaneHeight = 10
)

func samplePatents() []domain.PatentRow {
	return []domain.PatentRow{
		{Number: domain.MustParsePatentNumber("US16000001"), DisplayNumber: domain.MustParsePatentNumber("US1B2"), Title: "First", FetchState: domain.FetchCached},
		{Number: domain.MustParsePatentNumber("US16000002"), DisplayNumber: domain.MustParsePatentNumber("US2B2"), Title: "Second", FetchState: domain.FetchCached},
		{Number: domain.MustParsePatentNumber("US16000003"), DisplayNumber: domain.MustParsePatentNumber("US3B2"), Title: "Third", FetchState: domain.FetchStub},
	}
}

func loadedCatalog(t *testing.T) *Catalog {
	t.Helper()
	c := NewCatalog(nil, render.NewTheme())
	c.loadID = 1
	updated, _ := c.Update(catalogLoadedMsg{requestID: 1, total: 3, patents: samplePatents()})
	return updated.(*Catalog)
}

func TestCatalogSelectionFollowsCursor(t *testing.T) {
	c := loadedCatalog(t)

	sel, ok := c.Selection()
	if !ok || sel.Serial != "16000001" {
		t.Fatalf("initial selection = %v ok=%v, want US16000001", sel, ok)
	}

	// A count-prefixed motion moves two rows at once.
	c.Command(command.NavDown, Invocation{Repeat: 2})
	sel, _ = c.Selection()
	if sel.Serial != "16000003" {
		t.Fatalf("selection after 2j = %v, want US16000003", sel)
	}

	// Navigation clamps at the list bounds.
	c.Command(command.NavDown, Invocation{Repeat: 10})
	sel, _ = c.Selection()
	if sel.Serial != "16000003" {
		t.Fatalf("selection after clamp = %v, want last row", sel)
	}
	c.Command(command.NavTop, Invocation{Repeat: 1})
	sel, _ = c.Selection()
	if sel.Serial != "16000001" {
		t.Fatalf("selection after NavTop = %v, want first row", sel)
	}
}

func TestCatalogViewRendersRows(t *testing.T) {
	c := loadedCatalog(t)
	out := c.View(testPaneWidth, testPaneHeight)
	for _, want := range []string{"[1/3]", "#", "NUMBER", "US1B2", "Second", "🗃️", "🦴"} {
		if !strings.Contains(out, want) {
			t.Errorf("catalog view missing %q\n%s", want, out)
		}
	}
}

func TestCatalogIndexUsesAbsolutePositionAcrossPages(t *testing.T) {
	c := NewCatalog(nil, render.NewTheme())
	c.page.SetTotal(5)
	c.page.SetPageSize(2)
	c.page.ScrollTo(2)
	c.loadID = 1
	updated, _ := c.Update(catalogLoadedMsg{
		requestID: 1,
		offset:    2,
		total:     5,
		patents: []domain.PatentRow{
			{Number: domain.MustParsePatentNumber("US16000003"), DisplayNumber: domain.MustParsePatentNumber("US3B2"), Title: "Third", FetchState: domain.FetchStub},
			{Number: domain.MustParsePatentNumber("US16000004"), DisplayNumber: domain.MustParsePatentNumber("US4B2"), Title: "Fourth", FetchState: domain.FetchCached},
		},
	})
	c = updated.(*Catalog)
	out := c.View(testPaneWidth, testPaneHeight)
	for _, want := range []string{"3    US3B2", "4    US4B2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("catalog paged view missing %q\n%s", want, out)
		}
	}
}

func TestCatalogViewUsesReviewStateForActiveProject(t *testing.T) {
	c := loadedCatalog(t)
	project := &domain.Project{ID: "p-1", Name: "Case A"}
	c.activeProject = project
	c.patents[0].ReviewState = domain.ReviewStateUnderReview
	c.patents[0].IDSEntry = &domain.IDSEntry{Project: project.ID, Patent: c.patents[0].Number, Status: domain.IDSEntrySubmitted}

	out := c.View(testPaneWidth, testPaneHeight)
	for _, want := range []string{"[1/3]", "REVIEW ST", "IDS", "📨", "⭕", "🔍"} {
		if !strings.Contains(out, want) {
			t.Fatalf("catalog project view missing %q\n%s", want, out)
		}
	}
}

func TestCatalogAppliesReviewStateChangeImmediately(t *testing.T) {
	c := loadedCatalog(t)
	c.activeProject = &domain.Project{ID: "p-1", Name: "Case A"}

	updated, _ := c.Update(ReviewStateChangedMsg{
		Project: "p-1",
		Patents: []domain.PatentNumber{c.patents[1].Number},
		State:   domain.ReviewStateUnderReview,
	})
	c = updated.(*Catalog)

	if got := c.patents[1].ReviewState; got != domain.ReviewStateUnderReview {
		t.Fatalf("catalog review state = %q, want %q", got, domain.ReviewStateUnderReview)
	}
}

func TestCatalogAppliesIDSEntryChangeImmediately(t *testing.T) {
	c := loadedCatalog(t)
	c.activeProject = &domain.Project{ID: "p-1", Name: "Case A"}
	entry := &domain.IDSEntry{
		Project: "p-1",
		Patent:  c.patents[1].Number,
		Status:  domain.IDSEntryAccepted,
		InFull:  true,
	}

	updated, _ := c.Update(IDSEntryChangedMsg{
		Project: "p-1",
		Patent:  c.patents[1].Number,
		Entry:   entry,
	})
	c = updated.(*Catalog)

	if got := c.patents[1].IDSEntry; got != entry {
		t.Fatalf("catalog IDS entry = %+v, want %+v", got, entry)
	}
	out := c.View(testPaneWidth, testPaneHeight)
	if !strings.Contains(out, "✅") {
		t.Fatalf("catalog view should include updated IDS status, got:\n%s", out)
	}

	updated, _ = c.Update(IDSEntryChangedMsg{
		Project: "p-2",
		Patent:  c.patents[1].Number,
		Entry:   nil,
	})
	c = updated.(*Catalog)
	if got := c.patents[1].IDSEntry; got != entry {
		t.Fatalf("catalog IDS entry changed for another project: %+v", got)
	}
}

func TestCatalogAppliesIDSEntryChangeByDisplayNumber(t *testing.T) {
	c := loadedCatalog(t)
	c.activeProject = &domain.Project{ID: "p-1", Name: "Case A"}
	display := domain.MustParsePatentNumber("US20080011946A1")
	c.patents[1].DisplayNumber = display
	entry := &domain.IDSEntry{
		Project: "p-1",
		Patent:  display,
		Status:  domain.IDSEntryPending,
	}

	updated, cmd := c.Update(IDSEntryChangedMsg{Project: "p-1", Patent: display, Entry: entry})
	if cmd != nil {
		t.Fatal("display-number IDS update should not need a reload")
	}
	c = updated.(*Catalog)
	if got := c.patents[1].IDSEntry; got != entry {
		t.Fatalf("catalog IDS entry = %+v, want %+v", got, entry)
	}
}

func TestCatalogStatusLineShowsFilters(t *testing.T) {
	c := loadedCatalog(t)
	c.filter.Search = "widget"
	c.filter.setExpression(filterexpr.TermExpr{Field: filterexpr.FieldState, Value: string(domain.ReviewStateUnderReview), State: domain.ReviewStateUnderReview})
	out := c.View(testPaneWidth, testPaneHeight)
	for _, want := range []string{"[1/3]", "filters:", "state:under_review", "search:widget"} {
		if !strings.Contains(out, want) {
			t.Fatalf("catalog status line missing %q\n%s", want, out)
		}
	}
}

func TestCatalogEmptyAndErrorStates(t *testing.T) {
	c := NewCatalog(nil, render.NewTheme())
	c.filter = PatentFilter{} // Clear default filter for testing raw empty states
	if got := c.View(testPaneWidth, testPaneHeight); !strings.Contains(got, "loading") {
		t.Errorf("fresh catalog should show a loading state, got %q", got)
	}
	c.loadID = 1
	loaded, _ := c.Update(catalogLoadedMsg{requestID: 1, patents: nil})
	if got := loaded.(*Catalog).View(testPaneWidth, testPaneHeight); !strings.Contains(got, "no patents") {
		t.Errorf("empty catalog should show an empty state, got %q", got)
	}
}

func TestCatalogRelationHighlightsAndCache(t *testing.T) {
	c := loadedCatalog(t)

	// Initially no highlights are present
	if c.highlights.Len() != 0 {
		t.Fatalf("expected no highlights initially, got %d", c.highlights.Len())
	}

	// Pressing highlight family (g h) toggles loading state
	c.Command(command.HighlightFamily, Invocation{})
	if c.activeHighlight.Group != HighlightGroupFamily || !c.activeHighlight.Loading {
		t.Fatalf("expected family highlight loading, got %+v", c.activeHighlight)
	}

	// Simulate relations load message callback
	anchor := c.activeHighlight.Anchor
	loadID := c.activeHighlight.LoadID
	msg := relationHighlightLoadedMsg{
		requestID: loadID,
		group:     HighlightGroupFamily,
		anchor:    anchor,
		forward:   []domain.PatentNumber{domain.MustParsePatentNumber("US16000002")},
		reverse:   []domain.PatentNumber{domain.MustParsePatentNumber("US16000003")},
	}
	updated, _ := c.Update(msg)
	c = updated.(*Catalog)

	// Highlights should be updated and loading finished
	if c.activeHighlight.Loading {
		t.Fatalf("expected activeHighlight loading to be false")
	}
	if c.highlights.Kind(anchor) != HighlightFamilyAnchor {
		t.Fatalf("expected anchor %s to be HighlightFamilyAnchor, got %v", anchor, c.highlights.Kind(anchor))
	}
	if c.highlights.Kind(domain.MustParsePatentNumber("US16000002")) != HighlightFamilyParent {
		t.Fatalf("expected US16000002 to be HighlightFamilyParent, got %v", c.highlights.Kind(domain.MustParsePatentNumber("US16000002")))
	}
	if c.highlights.Kind(domain.MustParsePatentNumber("US16000003")) != HighlightFamilyChild {
		t.Fatalf("expected US16000003 to be HighlightFamilyChild, got %v", c.highlights.Kind(domain.MustParsePatentNumber("US16000003")))
	}

	// Cache should be populated
	if len(c.relationCache) != 1 {
		t.Fatalf("expected relation cache to have 1 entry, got %d", len(c.relationCache))
	}

	// Pressing highlight citations (g c) should clear previous highlights and toggle citations highlight
	c.Command(command.HighlightCitations, Invocation{})
	if c.activeHighlight.Group != HighlightGroupCitations || !c.activeHighlight.Loading {
		t.Fatalf("expected citations highlight loading")
	}
	if c.highlights.Len() != 0 {
		t.Fatalf("expected previous family highlights cleared on toggle citation")
	}
}

func TestCatalogRelationFilterCollapsesList(t *testing.T) {
	c := loadedCatalog(t)

	// Toggling filter when no highlight is active should return status/error.
	_, cmd := c.Command(command.RelationFilterCollapse, Invocation{})
	if cmd == nil {
		t.Fatalf("expected collapse relation filter command to return tea.Cmd when not active")
	}

	// Make a family relation highlight active.
	anchor := domain.MustParsePatentNumber("US16000001")
	c.activeHighlight = HighlightState{
		Group:   HighlightGroupFamily,
		Anchor:  anchor,
		Loading: false,
	}
	c.highlights.Upgrade(domain.MustParsePatentNumber("US16000002"), HighlightFamilyParent)
	c.highlights.Upgrade(domain.MustParsePatentNumber("US16000003"), HighlightFamilyChild)
	c.highlights.Upgrade(anchor, HighlightFamilyAnchor)

	// RelationNumbers helper should return the 3 patent numbers.
	relNums := c.highlights.RelationNumbers()
	if len(relNums) != 3 {
		t.Fatalf("expected 3 relation numbers, got %d", len(relNums))
	}

	// Pressing [ or z c collapses
	c.Command(command.RelationFilterCollapse, Invocation{})
	if !c.activeHighlight.FilterToRelations {
		t.Fatalf("expected FilterToRelations to be true after collapse")
	}

	// Pressing ] or z o expands
	c.Command(command.RelationFilterExpand, Invocation{})
	if c.activeHighlight.FilterToRelations {
		t.Fatalf("expected FilterToRelations to be false after expand")
	}

	// Collapse again to check clean toggle-off of highlight
	c.Command(command.RelationFilterCollapse, Invocation{})
	c.Command(command.HighlightFamily, Invocation{})
	if c.activeHighlight.FilterToRelations {
		t.Fatalf("expected FilterToRelations to be cleared when highlight is toggled off")
	}
}

func TestCatalogHandlesIDSEntryChangedMsg(t *testing.T) {
	c := loadedCatalog(t)
	project := &domain.Project{ID: "p-1", Name: "Case A"}
	c.activeProject = project

	// Verify initial IDS state displays the 'none' glyph (⭕)
	outInit := c.View(testPaneWidth, testPaneHeight)
	if !strings.Contains(outInit, "⭕") {
		t.Fatalf("expected initial catalog view to display 'none' glyph (⭕):\n%s", outInit)
	}

	// Emit IDSEntryChangedMsg
	entry := &domain.IDSEntry{
		Project: project.ID,
		Patent:  c.patents[0].Number,
		Status:  domain.IDSEntrySubmitted,
	}
	updated, _ := c.Update(IDSEntryChangedMsg{
		Project: project.ID,
		Patent:  c.patents[0].Number,
		Entry:   entry,
	})
	c = updated.(*Catalog)

	// Verify updated IDS state
	if c.patents[0].IDSEntry == nil || c.patents[0].IDSEntry.Status != domain.IDSEntrySubmitted {
		t.Fatalf("expected IDSEntry status to be submitted, got %v", c.patents[0].IDSEntry)
	}

	// Verify that View renders the new status glyph (📨)
	outUpdated := c.View(testPaneWidth, testPaneHeight)
	if !strings.Contains(outUpdated, "📨") {
		t.Fatalf("catalog view did not render updated IDS status glyph '📨':\n%s", outUpdated)
	}
}

func TestCatalogHandlesIDSEntriesChangedMsgBulk(t *testing.T) {
	c := loadedCatalog(t)
	project := &domain.Project{ID: "p-1", Name: "Case A"}
	c.activeProject = project

	if len(c.patents) < 2 {
		t.Fatalf("test fixture needs at least 2 patents, got %d", len(c.patents))
	}

	entries := []domain.IDSEntry{
		{Project: project.ID, Patent: c.patents[0].Number, Status: domain.IDSEntryAccepted, InFull: true},
		{Project: project.ID, Patent: c.patents[1].Number, Status: domain.IDSEntryIgnored, InFull: true},
	}
	updated, cmd := c.Update(IDSEntriesChangedMsg{Entries: entries})
	c = updated.(*Catalog)
	if cmd != nil {
		t.Fatal("visible bulk IDS updates should apply in Update without forcing a reload")
	}

	if c.patents[0].IDSEntry == nil || c.patents[0].IDSEntry.Status != domain.IDSEntryAccepted {
		t.Fatalf("row 0 IDS not applied: %+v", c.patents[0].IDSEntry)
	}
	if c.patents[1].IDSEntry == nil || c.patents[1].IDSEntry.Status != domain.IDSEntryIgnored {
		t.Fatalf("row 1 IDS not applied: %+v", c.patents[1].IDSEntry)
	}

	out := c.View(testPaneWidth, testPaneHeight)
	if !strings.Contains(out, "✅") {
		t.Fatalf("expected accepted glyph in view:\n%s", out)
	}
	if !strings.Contains(out, "👻") {
		t.Fatalf("expected ignored glyph in view:\n%s", out)
	}
}
