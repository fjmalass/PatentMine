package pane

import (
	"strings"
	"testing"
	"time"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

const (
	testProjectPaneWidth    = 80
	testProjectPaneHeight   = 10
	testSplashPaneWidth     = 100
	testSplashPaneHeight    = 24
	testCitationsPaneWidth  = 100
	testCitationsPaneHeight = 10
)

func TestProjectsPaneSelectsLastUsed(t *testing.T) {
	projects := []domain.Project{
		{ID: "p-1", Name: "Project 1", CreatedAt: time.Now().UTC()},
		{ID: "p-2", Name: "Project 2", CreatedAt: time.Now().UTC()},
	}
	p := NewSplash(nil, render.NewTheme(), "p-2", "footer", "hint")

	updated, _ := p.Update(projectsLoadedMsg{requestID: 0, projects: projects})
	p = updated.(*Projects)
	got, ok := p.selectedProject()
	if !ok || got.ID != "p-2" {
		t.Fatalf("selected project = %v ok=%v, want p-2", got, ok)
	}
	out := p.View(testSplashPaneWidth, testSplashPaneHeight)
	for _, want := range []string{"#", "SELECT PROJECT", "last used", "[p-2]", "1", "2"} {
		if !strings.Contains(out, want) {
			t.Errorf("projects view missing %q\n%s", want, out)
		}
	}
}

func TestCitationsPaneSelectsNeighbour(t *testing.T) {
	root := domain.MustParsePatentNumber("US0000001B2")
	c := NewCitations(nil, render.NewTheme(), root, domain.RelationCites)

	patents := []domain.PatentRow{
		{Number: domain.MustParsePatentNumber("US0000002B2"), Title: "Second"},
		{Number: domain.MustParsePatentNumber("US0000003B2"), Title: "Third"},
	}
	updated, _ := c.Update(citationsLoadedMsg{patents: patents, total: 2})
	c = updated.(*Citations)

	// Selection is the neighbour patent, so drilling into detail works.
	sel, ok := c.Selection()
	if !ok || sel.Serial != "0000002" {
		t.Fatalf("selection = %v ok=%v, want US0000002B2", sel, ok)
	}
	c.Command(command.NavBottom, Invocation{Repeat: 1})
	sel, _ = c.Selection()
	if sel.Serial != "0000003" {
		t.Fatalf("selection after NavBottom = %v, want US0000003B2", sel)
	}

	out := c.View(testCitationsPaneWidth, testCitationsPaneHeight)
	for _, want := range []string{"#", "1    US0000002B2", "Second"} {
		if !strings.Contains(out, want) {
			t.Errorf("citations view missing expected content %q\n%s", want, out)
		}
	}
}

func TestCitationsTitleReflectsKind(t *testing.T) {
	root := domain.MustParsePatentNumber("US0000001B2")
	cited := NewCitations(nil, render.NewTheme(), root, domain.RelationCitedBy)
	if !strings.HasPrefix(cited.Title(), "Cited by") {
		t.Fatalf("cited-by pane title = %q, want a 'Cited by' prefix", cited.Title())
	}
}
