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
		{Number: domain.MustParsePatentNumber("US16000001"), DisplayNumber: domain.MustParsePatentNumber("US0000001B2"), Title: "First", FetchState: domain.FetchCached},
		{Number: domain.MustParsePatentNumber("US16000002"), DisplayNumber: domain.MustParsePatentNumber("US0000002B2"), Title: "Second", FetchState: domain.FetchCached},
		{Number: domain.MustParsePatentNumber("US16000003"), DisplayNumber: domain.MustParsePatentNumber("US0000003B2"), Title: "Third", FetchState: domain.FetchStub},
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
	for _, want := range []string{"[1/3]", "#", "NUMBER", "US0000001B2", "Second", "⚡", "🔗"} {
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
			{Number: domain.MustParsePatentNumber("US16000003"), DisplayNumber: domain.MustParsePatentNumber("US0000003B2"), Title: "Third", FetchState: domain.FetchStub},
			{Number: domain.MustParsePatentNumber("US16000004"), DisplayNumber: domain.MustParsePatentNumber("US0000004B2"), Title: "Fourth", FetchState: domain.FetchCached},
		},
	})
	c = updated.(*Catalog)
	out := c.View(testPaneWidth, testPaneHeight)
	for _, want := range []string{"3    US0000003B2", "4    US0000004B2"} {
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
	for _, want := range []string{"[1/3]", "REVIEW ST", "IDS", "submitt", "🔍"} {
		if !strings.Contains(out, want) {
			t.Fatalf("catalog project view missing %q\n%s", want, out)
		}
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
