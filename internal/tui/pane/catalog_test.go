package pane

import (
	"strings"
	"testing"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

func samplePatents() []domain.Patent {
	return []domain.Patent{
		{Number: domain.MustParsePatentNumber("US0000001B2"), Title: "First", FetchState: domain.FetchCached},
		{Number: domain.MustParsePatentNumber("US0000002B2"), Title: "Second", FetchState: domain.FetchCached},
		{Number: domain.MustParsePatentNumber("US0000003B2"), Title: "Third", FetchState: domain.FetchStub},
	}
}

func loadedCatalog(t *testing.T) *Catalog {
	t.Helper()
	c := NewCatalog(nil, render.NewTheme())
	updated, _ := c.Update(catalogLoadedMsg{patents: samplePatents()})
	return updated.(*Catalog)
}

func TestCatalogSelectionFollowsCursor(t *testing.T) {
	c := loadedCatalog(t)

	sel, ok := c.Selection()
	if !ok || sel.Serial != "0000001" {
		t.Fatalf("initial selection = %v ok=%v, want US0000001B2", sel, ok)
	}

	// A count-prefixed motion moves two rows at once.
	c.Command(command.NavDown, 2)
	sel, _ = c.Selection()
	if sel.Serial != "0000003" {
		t.Fatalf("selection after 2j = %v, want US0000003B2", sel)
	}

	// Navigation clamps at the list bounds.
	c.Command(command.NavDown, 10)
	sel, _ = c.Selection()
	if sel.Serial != "0000003" {
		t.Fatalf("selection after clamp = %v, want last row", sel)
	}
	c.Command(command.NavTop, 1)
	sel, _ = c.Selection()
	if sel.Serial != "0000001" {
		t.Fatalf("selection after NavTop = %v, want first row", sel)
	}
}

func TestCatalogViewRendersRows(t *testing.T) {
	c := loadedCatalog(t)
	out := c.View(80, 10)
	for _, want := range []string{"NUMBER", "US0000001B2", "Second", "cached", "stub"} {
		if !strings.Contains(out, want) {
			t.Errorf("catalog view missing %q\n%s", want, out)
		}
	}
}

func TestCatalogEmptyAndErrorStates(t *testing.T) {
	c := NewCatalog(nil, render.NewTheme())
	if got := c.View(80, 10); !strings.Contains(got, "loading") {
		t.Errorf("fresh catalog should show a loading state, got %q", got)
	}
	loaded, _ := c.Update(catalogLoadedMsg{patents: nil})
	if got := loaded.(*Catalog).View(80, 10); !strings.Contains(got, "no patents") {
		t.Errorf("empty catalog should show an empty state, got %q", got)
	}
}
