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
	testCitationsPaneWidth  = 120
	testCitationsPaneHeight = 10
)

func TestProjectsPaneSelectsLastUsed(t *testing.T) {
	projects := []domain.Project{
		{ID: "p-1", Name: "Project 1", CreatedAt: time.Now().UTC()},
		{ID: "p-2", Name: "Project 2", CreatedAt: time.Now().UTC()},
	}
	p := NewSplash(nil, render.NewTheme(), "p-2", "footer", "hint", "AI: Gemini", "Search: Google, USPTO")

	updated, _ := p.Update(projectsLoadedMsg{requestID: 0, projects: projects})
	p = updated.(*Projects)
	got, ok := p.selectedProject()
	if !ok || got.ID != "p-2" {
		t.Fatalf("selected project = %v ok=%v, want p-2", got, ok)
	}
	out := p.View(testSplashPaneWidth, testSplashPaneHeight)
	for _, want := range []string{"[2/2]", "#", "SELECT PROJECT", "last used", "[p-2]", "1", "2"} {
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
	for _, want := range []string{"[2/2]", "#", "1    US0000002B2", "Second"} {
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

func TestDetailPaneJumpActive(t *testing.T) {
	num := domain.MustParsePatentNumber("US0000001B2")
	// Bound letters simulate keymap conflicts a, i, c, b — the algorithm
	// should assign non-conflicting letters for those labels.
	bound := []rune{'a', 'i', 'c', 'b'}
	d := NewDetail(nil, render.NewTheme(), num, "test-project", bound)

	// Pre-populate patent data so loading is false
	d.loading = false
	d.patent = domain.Patent{
		Number:    num,
		Title:     "Test Patent Title",
		Assignee:  "Test Assignee",
		Inventors: []domain.Inventor{"John Doe"},
	}

	// 1. When jumpActive is false, verify no inline shortcuts
	outNormal := d.body(80)
	if strings.Contains(outNormal, "[") {
		t.Errorf("expected no inline shortcuts in normal view, but found some: %s", outNormal)
	}

	// 2. Set jumpActive to true, verify inline shortcuts exist with correct keys
	d.SetJumpActive(true)
	outJump := d.body(80)

	// Assignee: 'a' is bound, try second letter 's' -> free -> 's'
	if !strings.Contains(outJump, "[s] Assignee") {
		t.Errorf("expected assignee inline shortcut '[s] Assignee' when jump mode is active, got: %s", outJump)
	}
	// Inventors: 'i' is bound, try second letter 'n' -> free -> 'n'
	if !strings.Contains(outJump, "[n] Inventors") {
		t.Errorf("expected inventors inline shortcut '[n] Inventors' when jump mode is active, got: %s", outJump)
	}
	// Expiration: 'e' is free -> 'e'
	if !strings.Contains(outJump, "[e] Expiration") {
		t.Errorf("expected expiration inline shortcut '[e] Expiration' when jump mode is active, got: %s", outJump)
	}

	// 3. Set jumpActive to false again, verify inline shortcuts are gone
	d.SetJumpActive(false)
	outNormal2 := d.body(80)
	if strings.Contains(outNormal2, "[") {
		t.Errorf("expected no inline shortcuts after deactivating jump mode, but found some: %s", outNormal2)
	}
}

func TestCitationsPaneClassificationColumn(t *testing.T) {
	root := domain.MustParsePatentNumber("US0000001B2")
	c := NewCitations(nil, render.NewTheme(), root, domain.RelationCites)

	patents := []domain.PatentRow{
		{
			Number:          domain.MustParsePatentNumber("US0000002B2"),
			Title:           "Second",
			Classifications: []string{"G06F 17/30", "H04L 29/06"},
		},
	}
	updated, _ := c.Update(citationsLoadedMsg{patents: patents, total: 1})
	c = updated.(*Citations)

	out := c.View(testCitationsPaneWidth, testCitationsPaneHeight)
	for _, want := range []string{"CLASS", "G06F 17/30, H04"} {
		if !strings.Contains(out, want) {
			t.Errorf("citations view missing expected content %q\n%s", want, out)
		}
	}
}

func TestPatentTableColumnsResponsive(t *testing.T) {
	tests := []struct {
		width       int
		wantColumns []string
	}{
		{120, []string{"#", "NUMBER", "TITLE", "INVENTOR", "CLASS", "EXPIRES", "TAGS", "IDS", "FETCH"}},
		{100, []string{"#", "NUMBER", "TITLE", "INVENTOR", "CLASS", "EXPIRES", "IDS", "FETCH"}},
		{85, []string{"#", "NUMBER", "TITLE", "INVENTOR", "CLASS", "EXPIRES", "FETCH"}},
		{70, []string{"#", "NUMBER", "TITLE", "INVENTOR", "CLASS", "FETCH"}},
		{50, []string{"#", "NUMBER", "TITLE", "FETCH"}},
	}

	for _, tt := range tests {
		cols := patentTableColumns(tt.width, "")
		if len(cols) != len(tt.wantColumns) {
			t.Errorf("width %d: got %d columns, want %d", tt.width, len(cols), len(tt.wantColumns))
			continue
		}
		for i, col := range cols {
			if col.label != tt.wantColumns[i] {
				t.Errorf("width %d column %d: got label %q, want %q", tt.width, i, col.label, tt.wantColumns[i])
			}
		}
	}
}

