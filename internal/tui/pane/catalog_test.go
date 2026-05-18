package pane

import (
	"strings"
	"testing"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

const (
	testPaneWidth  = 80
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
	c.Command(command.NavDown, 2)
	sel, _ = c.Selection()
	if sel.Serial != "16000003" {
		t.Fatalf("selection after 2j = %v, want US16000003", sel)
	}

	// Navigation clamps at the list bounds.
	c.Command(command.NavDown, 10)
	sel, _ = c.Selection()
	if sel.Serial != "16000003" {
		t.Fatalf("selection after clamp = %v, want last row", sel)
	}
	c.Command(command.NavTop, 1)
	sel, _ = c.Selection()
	if sel.Serial != "16000001" {
		t.Fatalf("selection after NavTop = %v, want first row", sel)
	}
}

func TestCatalogViewRendersRows(t *testing.T) {
	c := loadedCatalog(t)
	out := c.View(testPaneWidth, testPaneHeight)
	for _, want := range []string{"NUMBER", "US0000001B2", "Second", "cached", "stub"} {
		if !strings.Contains(out, want) {
			t.Errorf("catalog view missing %q\n%s", want, out)
		}
	}
}

func TestCatalogViewUsesMembershipStateForActiveProject(t *testing.T) {
	c := loadedCatalog(t)
	project := &domain.Project{ID: "p-1", Name: "Case A"}
	c.activeProject = project
	c.patents[0].MembershipState = domain.MembershipUnderReview

	out := c.View(testPaneWidth, testPaneHeight)
	for _, want := range []string{"PROJECT STATE", "under_review"} {
		if !strings.Contains(out, want) {
			t.Fatalf("catalog project view missing %q\n%s", want, out)
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
