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

func TestDetailPaneMultilineSectionsHighlightAsOneGroup(t *testing.T) {
	num := domain.MustParsePatentNumber("US0000001B2")
	d := NewDetail(nil, render.NewTheme(), num, "", nil)
	d.loading = false
	d.patent = domain.Patent{
		Number:     num,
		Title:      "Test Patent Title",
		FirstClaim: "A method for coordinating a distributed processing system across multiple independent compute nodes with replicated state synchronization.",
		Abstract:   "A system and method for coordinating a distributed processing system across multiple independent compute nodes with replicated state synchronization.",
	}

	body := d.body(36)
	lines := strings.Split(body, "\n")
	d.page.SetTotal(len(lines))
	d.page.SetPageSize(10)
	firstClaimLine := -1
	abstractLine := -1
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case "First claim":
			firstClaimLine = i
		case "Abstract":
			abstractLine = i
		}
	}
	if firstClaimLine < 0 || abstractLine < 0 {
		t.Fatalf("expected section headings in detail body, got:\n%s", body)
	}

	firstClaimGroup := d.highlightGroup(firstClaimLine)
	if firstClaimGroup.start != firstClaimLine || firstClaimGroup.end <= firstClaimLine {
		t.Fatalf("first claim group = %+v, want heading plus wrapped content", firstClaimGroup)
	}
	if got := d.highlightGroup(firstClaimGroup.end); got != firstClaimGroup {
		t.Fatalf("wrapped first claim line group = %+v, want %+v", got, firstClaimGroup)
	}

	abstractGroup := d.highlightGroup(abstractLine)
	if abstractGroup.start != abstractLine || abstractGroup.end <= abstractLine {
		t.Fatalf("abstract group = %+v, want heading plus wrapped content", abstractGroup)
	}
	if got := d.highlightGroup(abstractGroup.end); got != abstractGroup {
		t.Fatalf("wrapped abstract line group = %+v, want %+v", got, abstractGroup)
	}
}

func TestDetailPaneDocumentsHighlightAsOneGroup(t *testing.T) {
	num := domain.MustParsePatentNumber("US0000001B2")
	d := NewDetail(nil, render.NewTheme(), num, "", nil)
	d.loading = false
	d.patent = domain.Patent{
		Number: num,
		Title:  "Test Patent Title",
		Documents: []domain.Document{
			{Stage: domain.StageApplication, Number: domain.MustParsePatentNumber("US1234567A1")},
			{Stage: domain.StageGrant, Number: domain.MustParsePatentNumber("US1234567B2")},
		},
	}

	body := d.body(80)
	lines := strings.Split(body, "\n")
	docLine := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "Documents" {
			docLine = i
			break
		}
	}
	if docLine < 0 {
		t.Fatalf("expected documents heading in detail body, got:\n%s", body)
	}

	docGroup := d.highlightGroup(docLine)
	if docGroup.start != docLine || docGroup.end <= docLine {
		t.Fatalf("documents group = %+v, want heading plus document rows", docGroup)
	}
	if got := d.highlightGroup(docGroup.end); got != docGroup {
		t.Fatalf("document row group = %+v, want %+v", got, docGroup)
	}
}

func TestDetailPaneNavMovesBetweenGroups(t *testing.T) {
	num := domain.MustParsePatentNumber("US0000001B2")
	d := NewDetail(nil, render.NewTheme(), num, "", nil)
	d.loading = false
	d.patent = domain.Patent{
		Number:     num,
		Title:      "Test Patent Title",
		FirstClaim: "A method for coordinating a distributed processing system across multiple independent compute nodes with replicated state synchronization.",
		Abstract:   "A system and method for coordinating a distributed processing system across multiple independent compute nodes with replicated state synchronization.",
	}

	body := d.body(36)
	lines := strings.Split(body, "\n")
	d.page.SetTotal(len(lines))
	d.page.SetPageSize(10)
	firstClaimLine := -1
	abstractLine := -1
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case "First claim":
			firstClaimLine = i
		case "Abstract":
			abstractLine = i
		}
	}
	if firstClaimLine < 0 || abstractLine < 0 {
		t.Fatalf("expected section headings in detail body, got:\n%s", body)
	}

	d.page.ScrollTo(firstClaimLine)
	d.Command(command.NavDown, Invocation{Repeat: 1})
	if got := d.page.Cursor(); got != abstractLine {
		t.Fatalf("cursor after NavDown from first claim = %d, want %d", got, abstractLine)
	}

	d.Command(command.NavUp, Invocation{Repeat: 1})
	if got := d.page.Cursor(); got != firstClaimLine {
		t.Fatalf("cursor after NavUp from abstract = %d, want %d", got, firstClaimLine)
	}
}

func TestDetailPaneShowsProjectPatentNote(t *testing.T) {
	num := domain.MustParsePatentNumber("US0000001B2")
	d := NewDetail(nil, render.NewTheme(), num, "p1", nil)
	d.loading = false
	d.patent = domain.Patent{Number: num, Title: "Test Patent Title"}
	d.patentNote = &domain.PatentNote{
		Project:   "p1",
		Patent:    num,
		Markdown:  "# Overview\n\nImportant note.",
		AddedAt:   time.Date(2026, 5, 22, 10, 30, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 5, 22, 11, 0, 0, 0, time.UTC),
	}

	body := d.body(80)
	if !strings.Contains(body, "# Overview") {
		t.Fatalf("detail body missing heading summary, got:\n%s", body)
	}
	if !strings.Contains(body, "Notes") {
		t.Fatalf("detail body missing Notes field, got:\n%s", body)
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
	for _, want := range []string{"CLASS", "G06F 17/30"} {
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
		{150, []string{"#", "NUMBER", "TITLE", "INVENTOR", "CLASS", "EXPIRES", "CITES", "CITED", "PARENTS", "TAGS", "IDS", "FETCH"}},
		{135, []string{"#", "NUMBER", "TITLE", "INVENTOR", "CLASS", "EXPIRES", "CITES", "CITED", "PARENTS", "IDS", "FETCH"}},
		{120, []string{"#", "NUMBER", "TITLE", "INVENTOR", "CLASS", "EXPIRES", "TAGS", "IDS", "FETCH"}},
		{85, []string{"#", "NUMBER", "TITLE", "INVENTOR", "CLASS", "EXPIRES", "FETCH"}},
		{70, []string{"#", "NUMBER", "TITLE", "INVENTOR", "CLASS", "FETCH"}},
		{50, []string{"#", "NUMBER", "TITLE", "FETCH"}},
	}

	for _, tt := range tests {
		cols := patentTableColumns(tt.width, domain.PatentTableColumns(""))
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

func TestMoveSortableColumnSkipsUnsortableVisibleColumns(t *testing.T) {
	cols := patentTableColumns(120, domain.PatentTableColumns(""))
	if got := moveSortableColumn(cols, -1, 1); got != 1 {
		t.Fatalf("moveSortableColumn next from none = %d, want 1 for NUMBER", got)
	}
	if got := moveSortableColumn(cols, 5, 1); got != 8 {
		t.Fatalf("moveSortableColumn next from EXPIRES = %d, want 8 for IDS", got)
	}
	if got := moveSortableColumn(cols, 8, 1); got != 1 {
		t.Fatalf("moveSortableColumn should wrap to NUMBER, got %d", got)
	}
	if got := moveSortableColumn(cols, 8, -1); got != 5 {
		t.Fatalf("moveSortableColumn prev from FETCH = %d, want 5 for EXPIRES", got)
	}
}

func TestClampFocusedSortableColumnSkipsUnsortableCurrentColumn(t *testing.T) {
	cols := patentTableColumns(120, domain.PatentTableColumns(""))
	if got := clampFocusedSortableColumn(cols, 0); got != 1 {
		t.Fatalf("clampFocusedSortableColumn from index column = %d, want NUMBER column index 1", got)
	}
	projectCols := patentTableColumns(120, domain.PatentTableColumns("p1"))
	if got := clampFocusedSortableColumn(projectCols, 6); got != 6 {
		t.Fatalf("clampFocusedSortableColumn should keep sortable TAGS focus, got %d", got)
	}
}
