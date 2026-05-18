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
	testCitationsPaneWidth  = 80
	testCitationsPaneHeight = 10
)

func TestProjectsPaneNavigationAndView(t *testing.T) {
	p := NewProjects(nil, render.NewTheme())
	projects := []domain.Project{
		{ID: "p-1", Name: "Litigation A", CreatedAt: time.Now()},
		{ID: "p-2", Name: "Filing B", CreatedAt: time.Now()},
	}
	updated, _ := p.Update(projectsLoadedMsg{projects: projects})
	p = updated.(*Projects)
	active := projects[1]
	updated, _ = p.Update(ProjectChangedMsg{Project: &active})
	p = updated.(*Projects)

	if got, ok := p.selectedProject(); !ok || got.ID != "p-1" {
		t.Fatalf("initial selected project = %v ok=%v, want p-1", got, ok)
	}
	p.Command(command.NavDown, 1)
	if got, _ := p.selectedProject(); got.ID != "p-2" {
		t.Fatalf("selected project after NavDown = %v, want p-2", got)
	}

	out := p.View(testProjectPaneWidth, testProjectPaneHeight)
	for _, want := range []string{"ACTIVE", "NAME", "Litigation A", "Filing B", "*"} {
		if !strings.Contains(out, want) {
			t.Errorf("projects view missing %q\n%s", want, out)
		}
	}
	// The projects pane exposes no patent selection.
	if _, ok := p.Selection(); ok {
		t.Error("projects pane should report no patent selection")
	}
}

func TestSplashPreselectsLastUsedProject(t *testing.T) {
	p := NewSplash(nil, render.NewTheme(), "p-2", "footer", "empty")
	projects := []domain.Project{
		{ID: "p-1", Name: "Litigation A", CreatedAt: time.Now()},
		{ID: "p-2", Name: "Filing B", CreatedAt: time.Now()},
	}
	updated, _ := p.Update(projectsLoadedMsg{requestID: 0, projects: projects})
	p = updated.(*Projects)
	got, ok := p.selectedProject()
	if !ok || got.ID != "p-2" {
		t.Fatalf("selected project = %v ok=%v, want p-2", got, ok)
	}
	out := p.View(testSplashPaneWidth, testSplashPaneHeight)
	for _, want := range []string{"SELECT PROJECT", "last used", "[p-2]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("splash view missing %q\n%s", want, out)
		}
	}
}

func TestCitationsPaneSelectsNeighbour(t *testing.T) {
	root := domain.MustParsePatentNumber("US0000001B2")
	c := NewCitations(nil, render.NewTheme(), root, domain.RelationCites)

	relations := []domain.Relation{
		{From: root, To: domain.MustParsePatentNumber("US0000002B2"), Kind: domain.RelationCites},
		{From: root, To: domain.MustParsePatentNumber("US0000003B2"), Kind: domain.RelationCites},
	}
	updated, _ := c.Update(citationsLoadedMsg{relations: relations})
	c = updated.(*Citations)

	// Selection is the neighbour patent, so drilling into detail works.
	sel, ok := c.Selection()
	if !ok || sel.Serial != "0000002" {
		t.Fatalf("selection = %v ok=%v, want US0000002B2", sel, ok)
	}
	c.Command(command.NavBottom, 1)
	sel, _ = c.Selection()
	if sel.Serial != "0000003" {
		t.Fatalf("selection after NavBottom = %v, want US0000003B2", sel)
	}

	out := c.View(testCitationsPaneWidth, testCitationsPaneHeight)
	if !strings.Contains(out, "Citations") || !strings.Contains(out, "US0000002B2") {
		t.Errorf("citations view missing expected content\n%s", out)
	}
}

func TestCitationsTitleReflectsKind(t *testing.T) {
	root := domain.MustParsePatentNumber("US0000001B2")
	cited := NewCitations(nil, render.NewTheme(), root, domain.RelationCitedBy)
	if !strings.HasPrefix(cited.Title(), "Cited by") {
		t.Fatalf("cited-by pane title = %q, want a 'Cited by' prefix", cited.Title())
	}
}
