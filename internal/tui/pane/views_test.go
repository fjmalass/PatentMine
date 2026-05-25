package pane

import (
	"strings"
	"testing"
	"time"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
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
	p := NewSplash(nil, render.NewTheme(), "p-2", "footer", "hint", "AI: Gemini", "Search: Google, USPTO", "Backup: B2")

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

func TestCitationsAppliesReviewStateChangeImmediately(t *testing.T) {
	root := domain.MustParsePatentNumber("US0000001B2")
	c := NewCitations(nil, render.NewTheme(), root, domain.RelationCites)
	c.activeProject = &domain.Project{ID: "p-1", Name: "Case A"}
	patents := []domain.PatentRow{
		{Number: domain.MustParsePatentNumber("US0000002B2"), Title: "Second"},
		{Number: domain.MustParsePatentNumber("US0000003B2"), Title: "Third"},
	}
	updated, _ := c.Update(citationsLoadedMsg{patents: patents, total: 2})
	c = updated.(*Citations)

	updated, _ = c.Update(ReviewStateChangedMsg{
		Project: "p-1",
		Patents: []domain.PatentNumber{patents[0].Number},
		State:   domain.ReviewStateUnderReview,
	})
	c = updated.(*Citations)

	if got := c.patents[0].ReviewState; got != domain.ReviewStateUnderReview {
		t.Fatalf("citations review state = %q, want %q", got, domain.ReviewStateUnderReview)
	}
}

func TestFamilyGraphViewGroupsByDepthAndSkipsHeaders(t *testing.T) {
	root := domain.MustParsePatentNumber("US0000001B2")
	parent := domain.MustParsePatentNumber("EP0000002A1")
	child := domain.MustParsePatentNumber("JP0000003A1")
	g := NewFamilyGraph(nil, render.NewTheme(), root, 2, nil)
	g.loading = false
	g.nodes = []proto.FamilyGraphNode{
		{Patent: domain.PatentRow{Number: root, DisplayNumber: root, Title: "Root", FetchState: domain.FetchCached}, Depth: 0, Parents: []domain.PatentNumber{parent}},
		{Patent: domain.PatentRow{Number: parent, DisplayNumber: parent, Title: "Parent", FetchState: domain.FetchCached}, Depth: 1, Children: []domain.PatentNumber{root}},
		{Patent: domain.PatentRow{Number: child, DisplayNumber: child, Title: "Child", FetchState: domain.FetchStub}, Depth: 1},
	}
	g.rebuildRows()
	g.page.SetTotal(len(g.rows))
	g.jumpToFirstNode()

	out := g.View(120, 12)
	for _, want := range []string{"Depth 0  ·  1 node(s)  root", "Depth 1  ·  2 node(s)", "up:EP0000002A1", "dn:US0000001B2", "🦴  Child", "y: copy Mermaid"} {
		if !strings.Contains(out, want) {
			t.Fatalf("family graph view missing %q\n%s", want, out)
		}
	}
	if !strings.Contains(out, "depth:2/6") {
		t.Fatalf("family graph summary missing current/max depth\n%s", out)
	}

	if sel, ok := g.Selection(); !ok || sel != root {
		t.Fatalf("initial selection = %v ok=%v, want root", sel, ok)
	}
	g.Command(command.NavDown, Invocation{Repeat: 1})
	if sel, ok := g.Selection(); !ok || sel != parent {
		t.Fatalf("selection after NavDown = %v ok=%v, want parent", sel, ok)
	}
	g.Command(command.NavDown, Invocation{Repeat: 1})
	if sel, ok := g.Selection(); !ok || sel != child {
		t.Fatalf("selection after second NavDown = %v ok=%v, want child", sel, ok)
	}

	g.Command(command.NavTop, Invocation{Repeat: 1})
	g.Command(command.SelectVisual, Invocation{Repeat: 1})
	g.Command(command.NavDown, Invocation{Repeat: 2})
	selected := g.Selections()
	if len(selected) != 3 {
		t.Fatalf("visual selections len = %d, want 3", len(selected))
	}
	for i, want := range []domain.PatentNumber{root, parent, child} {
		if selected[i] != want {
			t.Fatalf("selection[%d] = %v, want %v", i, selected[i], want)
		}
	}

	g.Command(command.FamilyDepthMore, Invocation{Repeat: 1})
	if g.depth != 3 {
		t.Fatalf("depth after FamilyDepthMore = %d, want 3", g.depth)
	}
	g.Command(command.FamilyDepthLess, Invocation{Repeat: 1})
	if g.depth != 2 {
		t.Fatalf("depth after FamilyDepthLess = %d, want 2", g.depth)
	}
}

func TestFamilyGraphMermaidExportIsGroupedByDepth(t *testing.T) {
	root := domain.MustParsePatentNumber("US0000001B2")
	parent := domain.MustParsePatentNumber("EP0000002A1")
	child := domain.MustParsePatentNumber("JP0000003A1")
	g := NewFamilyGraph(nil, render.NewTheme(), root, 2, nil)
	g.loading = false
	g.nodes = []proto.FamilyGraphNode{
		{Patent: domain.PatentRow{Number: root, DisplayNumber: root, Title: "Root patent", FetchState: domain.FetchCached}, Depth: 0},
		{Patent: domain.PatentRow{Number: parent, DisplayNumber: parent, Title: "Parent patent", FetchState: domain.FetchCached}, Depth: 1},
		{Patent: domain.PatentRow{Number: child, DisplayNumber: child, Title: "", FetchState: domain.FetchStub}, Depth: 1},
	}
	g.edges = []proto.FamilyGraphEdge{{Parent: parent, Child: root}, {Parent: root, Child: child, Inconsistent: true}}
	g.rebuildRows()

	out := g.mermaidGraph()
	for _, want := range []string{
		"flowchart TD",
		"p_us_0000001_b2((\"US0000001B2<br/>Root patent\"))",
		"p_ep_0000002_a1[\"EP0000002A1<br/>Parent patent\"]",
		"p_jp_0000003_a1[\"JP0000003A1\"]",
		"p_ep_0000002_a1 --> p_us_0000001_b2",
		"p_us_0000001_b2 -.-> p_jp_0000003_a1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("mermaid export missing %q\n%s", want, out)
		}
	}
}

func TestFamilyGraphViewUsesReviewStateForActiveProject(t *testing.T) {
	root := domain.MustParsePatentNumber("US0000001B2")
	g := NewFamilyGraph(nil, render.NewTheme(), root, 2, nil)
	g.loading = false
	g.project = "p1"
	g.nodes = []proto.FamilyGraphNode{{
		Patent: domain.PatentRow{
			Number:        root,
			DisplayNumber: root,
			Title:         "Root",
			FetchState:    domain.FetchCached,
			ReviewState:   domain.ReviewStateUnderReview,
		},
		Depth: 0,
	}}
	g.rebuildRows()
	out := g.View(120, 8)
	if !strings.Contains(out, "🔍") {
		t.Fatalf("family graph view should show project review state, got:\n%s", out)
	}
}

func TestFamilyGraphAppliesReviewStateChangeImmediately(t *testing.T) {
	root := domain.MustParsePatentNumber("US0000001B2")
	g := NewFamilyGraph(nil, render.NewTheme(), root, 2, nil)
	g.project = "p1"
	g.nodes = []proto.FamilyGraphNode{{Patent: domain.PatentRow{Number: root, DisplayNumber: root, Title: "Root"}, Depth: 0}}
	g.rebuildRows()

	updated, _ := g.Update(ReviewStateChangedMsg{
		Project: "p1",
		Patents: []domain.PatentNumber{root},
		State:   domain.ReviewStateUnderReview,
	})
	g = updated.(*FamilyGraph)

	if got := g.nodes[0].Patent.ReviewState; got != domain.ReviewStateUnderReview {
		t.Fatalf("family graph review state = %q, want %q", got, domain.ReviewStateUnderReview)
	}
}

func TestDetailAppliesReviewStateChangeImmediately(t *testing.T) {
	num := domain.MustParsePatentNumber("US0000001B2")
	d := NewDetail(nil, render.NewTheme(), num, "p1", nil)
	d.loading = false

	updated, _ := d.Update(ReviewStateChangedMsg{
		Project: "p1",
		Patents: []domain.PatentNumber{num},
		State:   domain.ReviewStateUnderReview,
	})
	d = updated.(*Detail)

	if got := d.state; got != domain.ReviewStateUnderReview {
		t.Fatalf("detail review state = %q, want %q", got, domain.ReviewStateUnderReview)
	}
}

func TestDetailAppliesIDSEntryChangeImmediately(t *testing.T) {
	num := domain.MustParsePatentNumber("US0000001B2")
	d := NewDetail(nil, render.NewTheme(), num, "p1", nil)
	d.loading = false
	d.patent = domain.Patent{Number: num, Title: "Test Patent Title"}
	if out := d.View(80, 20); !strings.Contains(out, "⭕ none") {
		t.Fatalf("detail view should start without IDS entry, got:\n%s", out)
	}
	if d.cachedLines == nil {
		t.Fatalf("detail view should populate cached lines")
	}
	entry := &domain.IDSEntry{
		Project: "p1",
		Patent:  num,
		Status:  domain.IDSEntrySubmitted,
		InFull:  true,
	}

	updated, _ := d.Update(IDSEntryChangedMsg{
		Project: "p1",
		Patent:  num,
		Entry:   entry,
	})
	d = updated.(*Detail)

	if got := d.idsEntry; got != entry {
		t.Fatalf("detail IDS entry = %+v, want %+v", got, entry)
	}
	if d.cachedLines != nil {
		t.Fatalf("detail cached lines should be invalidated after IDS entry change")
	}
	out := d.View(80, 20)
	if !strings.Contains(out, "📨 submitted | full") {
		t.Fatalf("detail view should include updated IDS entry, got:\n%s", out)
	}

	updated, _ = d.Update(IDSEntryChangedMsg{
		Project: "p1",
		Patent:  domain.MustParsePatentNumber("US0000002B2"),
		Entry:   nil,
	})
	d = updated.(*Detail)
	if got := d.idsEntry; got != entry {
		t.Fatalf("detail IDS entry changed for another patent: %+v", got)
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

	// Assignee: 'a' is bound, but bypasses constraints to use the most intuitive key -> 'a'
	if !strings.Contains(outJump, "[a] Assignee") {
		t.Errorf("expected assignee inline shortcut '[a] Assignee' when jump mode is active, got: %s", outJump)
	}
	// Inventors: 'i' is bound, but bypasses constraints to use the most intuitive key -> 'i'
	if !strings.Contains(outJump, "[i] Inventors") {
		t.Errorf("expected inventors inline shortcut '[i] Inventors' when jump mode is active, got: %s", outJump)
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
		{150, []string{"#", "NUMBER", "TITLE", "INVENTOR", "CLASS", "EXPIRES", "CITES", "CITED", "PARENTS", "TAGS", "IDS", "FETCH STATE"}},
		{135, []string{"#", "NUMBER", "TITLE", "INVENTOR", "CLASS", "EXPIRES", "CITES", "CITED", "PARENTS", "IDS", "FETCH STATE"}},
		{120, []string{"#", "NUMBER", "TITLE", "INVENTOR", "CLASS", "EXPIRES", "TAGS", "IDS", "FETCH STATE"}},
		{85, []string{"#", "NUMBER", "TITLE", "INVENTOR", "CLASS", "EXPIRES", "FETCH STATE"}},
		{70, []string{"#", "NUMBER", "TITLE", "INVENTOR", "CLASS", "FETCH STATE"}},
		{50, []string{"#", "NUMBER", "TITLE", "FETCH STATE"}},
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

func TestWrapTextSplitsLongTokens(t *testing.T) {
	lines := wrapText("alpha supercalifragilisticexpialidocious omega", 8)
	if len(lines) == 0 {
		t.Fatal("wrapText returned no lines")
	}
	for i, line := range lines {
		if got := render.StringWidth(line); got > 8 {
			t.Fatalf("line %d width = %d, want <= 8: %q", i, got, line)
		}
	}
}

func TestDetailViewDoesNotEmitOverwideWrappedLines(t *testing.T) {
	const width = 36
	num := domain.MustParsePatentNumber("US0000001B2")
	d := NewDetail(nil, render.NewTheme(), num, "", nil)
	d.loading = false
	d.patent = domain.Patent{
		Number:          num,
		Title:           "Long token repaint regression",
		Assignee:        "Acme",
		Classifications: []string{"A01B123456789012345678901234567890"},
		FirstClaim:      "A method comprising supercalifragilisticexpialidociouswithoutbreaks and output.",
		Abstract:        "Abstract contains pneumonoultramicroscopicsilicovolcanoconiosiswithoutbreaks.",
	}

	out := d.View(width, 20)
	for i, line := range strings.Split(out, "\n") {
		if got := render.StringWidth(line); got >= width {
			t.Fatalf("detail line %d width = %d, want < %d: %q", i, got, width, line)
		}
	}
}

func TestDetailSelectedFetchStateDoesNotPadEmojiRow(t *testing.T) {
	const width = 36
	num := domain.MustParsePatentNumber("US0000001B2")
	d := NewDetail(nil, render.NewTheme(), num, "", nil)
	d.loading = false
	d.patent = domain.Patent{Number: num, Title: "Fetch state", FetchState: domain.FetchCached}

	d.View(width, 8)
	d.JumpTo(6)
	out := d.View(width, 8)
	if strings.Contains(out, "⚡") {
		t.Fatalf("detail fetch state should render stable text, got:\n%s", out)
	}
	for i, line := range strings.Split(out, "\n") {
		if got := render.StringWidth(line); got >= width {
			t.Fatalf("selected detail line %d width = %d, want < %d: %q", i, got, width, line)
		}
	}
}

func TestDetailNavigationWithinViewportDoesNotForceTargetToTop(t *testing.T) {
	const width = 80
	num := domain.MustParsePatentNumber("US0000001B2")
	d := NewDetail(nil, render.NewTheme(), num, "", nil)
	d.loading = false
	d.patent = domain.Patent{Number: num, Title: "Fetch state", FetchState: domain.FetchCached}

	d.View(width, 8)
	d.JumpTo(5) // Country at top, Fetch state still visible below it.
	if got := d.page.Offset(); got != 5 {
		t.Fatalf("offset before nav = %d, want 5", got)
	}
	d.Command(command.NavDown, Invocation{Repeat: 1})
	if got := d.page.Cursor(); got != 6 {
		t.Fatalf("cursor after nav = %d, want Fetch state line 6", got)
	}
	if got := d.page.Offset(); got != 5 {
		t.Fatalf("offset after visible-row nav = %d, want unchanged 5", got)
	}
}

func TestDetailClassificationsAreSummarized(t *testing.T) {
	classifications := make([]string, 0, detailClassificationLimit+3)
	for i := 0; i < detailClassificationLimit+3; i++ {
		classifications = append(classifications, "H04N"+formatViewIndex(i))
	}

	got := detailClassificationsText(classifications)
	if !strings.Contains(got, "(+3 more; press K)") {
		t.Fatalf("classification summary missing overflow hint: %q", got)
	}
	if strings.Contains(got, "H04N"+formatViewIndex(detailClassificationLimit)) {
		t.Fatalf("classification summary should not include entries past limit: %q", got)
	}
}

func TestDetailNavigationSkipsWrappedFieldContinuations(t *testing.T) {
	const width = 36
	num := domain.MustParsePatentNumber("US0000001B2")
	d := NewDetail(nil, render.NewTheme(), num, "", nil)
	d.loading = false
	for i := 0; i < detailClassificationLimit; i++ {
		d.patent.Classifications = append(d.patent.Classifications, "A01B "+formatViewIndex(i)+"/00")
	}
	d.patent.Number = num
	d.patent.Title = "Classification navigation"

	d.View(width, 8)
	classification := d.highlightGroup(d.classificationLine)
	if classification.end <= classification.start {
		t.Fatalf("classification group should span wrapped lines, got %+v", classification)
	}

	d.JumpTo(d.classificationLine)
	d.Command(command.NavDown, Invocation{Repeat: 1})
	if got := d.page.Cursor(); got <= classification.end {
		t.Fatalf("NavDown from classification landed inside wrapped group at %d, group %+v", got, classification)
	}

	d.JumpTo(d.classificationLine + 1)
	d.Command(command.NavUp, Invocation{Repeat: 1})
	if got := d.page.Cursor(); got != d.classificationLine {
		t.Fatalf("NavUp from classification continuation = %d, want label line %d", got, d.classificationLine)
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

func TestDetailJumpKeysDump(t *testing.T) {
	num := domain.MustParsePatentNumber("US0000001B2")
	d := NewDetail(nil, render.NewTheme(), num, "test-project", nil)
	t.Log("JUMP KEYS:")
	for label, key := range d.jump.JumpKeys {
		t.Logf("  %s -> %q", label, string(key))
	}
	if got := d.jump.JumpKeys["Children"]; got != 'C' {
		t.Errorf("jump key for Children = %q, want 'C'", string(got))
	}
}
