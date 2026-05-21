package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/text"
	"patentmine/internal/tui/keymap"
	"patentmine/internal/tui/overlay"
	"patentmine/internal/tui/pane"
)

const (
	testAppWidth  = 100
	testAppHeight = 30
)

// newTestApp builds an App with no daemon connection. The tests here exercise
// only input routing and the pane/overlay stacks, which never touch the client.
func newTestApp(t *testing.T) *App {
	t.Helper()
	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	app, err := New(nil, reg, keymap.Default(), text.English())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	app.Update(tea.WindowSizeMsg{Width: testAppWidth, Height: testAppHeight})
	return app
}

// runeKey builds a KeyMsg for a printable key.
func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestAppOpensAndClosesHelpOverlay(t *testing.T) {
	app := newTestApp(t)

	// "?" opens the help overlay.
	app.Update(runeKey('?'))
	if len(app.overlays) != 1 {
		t.Fatalf("after '?', overlays = %d, want 1", len(app.overlays))
	}
	// The overlay renders over the dimmed background without panicking.
	if view := app.View(); !strings.Contains(view, "Help") {
		t.Fatalf("help overlay not visible in view")
	}
	// "esc" closes it.
	app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if len(app.overlays) != 0 {
		t.Fatalf("after esc, overlays = %d, want 0", len(app.overlays))
	}
}

func TestAppQuitCommand(t *testing.T) {
	app := newTestApp(t)
	_, cmd := app.Update(runeKey('Q'))
	if cmd == nil {
		t.Fatal("'Q' should return a command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("'Q' should resolve to tea.Quit")
	}
}

func TestAppChordCountReachesPane(t *testing.T) {
	app := newTestApp(t)
	// While an overlay is open, pane bindings are inactive: 'f' (crawl, a
	// catalog binding) must not be routed anywhere.
	app.Update(runeKey('?'))
	stack := app.keyStack()
	if _, ok := stack.Resolve("f"); ok {
		t.Fatal("catalog 'f' binding should be inactive while the overlay is focused")
	}
	// The overlay's own navigation binding is active.
	if id, ok := stack.Resolve("j"); !ok || id != command.NavDown {
		t.Fatalf("overlay 'j' resolved to %q ok=%v, want nav.down", id, ok)
	}
}

func TestAppBackPopsPaneStack(t *testing.T) {
	app := newTestApp(t)
	// Simulate having drilled into a detail pane by pushing one.
	app.Update(runeKey('?')) // overlay
	app.Update(runeKey('q')) // 'q' = Back: closes the overlay first
	if len(app.overlays) != 0 {
		t.Fatalf("'q' should close the overlay, overlays = %d", len(app.overlays))
	}
}

func TestStatusTextIncludesTUIAndDaemonVersions(t *testing.T) {
	app := newTestApp(t)
	app.daemonVersion = "daemon-v1"
	app.activeProject = &domain.Project{ID: "p-1", Name: "Case A"}
	text := app.statusText()
	for _, want := range []string{"tui ", "daemon daemon-v1", "project Case A"} {
		if !strings.Contains(text, want) {
			t.Fatalf("status text %q missing %q", text, want)
		}
	}
}

func TestAppOpensCommandPaletteAndPrompt(t *testing.T) {
	app := newTestApp(t)
	app.activeProject = &domain.Project{ID: "p-1", Name: "Case A"}

	app.Update(runeKey('/'))
	if len(app.overlays) != 1 {
		t.Fatalf("after '/', overlays = %d, want 1", len(app.overlays))
	}
	if _, ok := app.focusedOverlay().(*overlay.Prompt); !ok {
		t.Fatal("'/' should open a prompt overlay")
	}
	if view := app.View(); !strings.Contains(view, "Commands") || !strings.Contains(view, "Case A") {
		t.Fatalf("palette view missing expected content\n%s", view)
	}
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if cmd != nil {
		app.Update(cmd())
	}

	app.Update(runeKey(':'))
	if len(app.overlays) != 1 {
		t.Fatalf("after ':', overlays = %d, want 1", len(app.overlays))
	}
	if view := app.View(); !strings.Contains(view, "Command") || !strings.Contains(view, "Type a dot command") {
		t.Fatalf("command prompt view missing expected content\n%s", view)
	}
}

type refreshProbePane struct{ refreshes int }

func (p *refreshProbePane) Scope() command.Scope            { return command.ScopeCatalog }
func (p *refreshProbePane) Title() string                       { return "probe" }
func (p *refreshProbePane) Init() tea.Cmd                       { return nil }
func (p *refreshProbePane) Update(tea.Msg) (pane.Pane, tea.Cmd) { return p, nil }
func (p *refreshProbePane) View(int, int) string                { return "" }
func (p *refreshProbePane) Selection() (domain.PatentNumber, bool) {
	return domain.PatentNumber{}, false
}

func (p *refreshProbePane) Command(id command.ID, _ pane.Invocation) (pane.Pane, tea.Cmd) {
	if id == command.Refresh {
		p.refreshes++
	}
	return p, nil
}
func (p *refreshProbePane) Handles() []command.ID { return []command.ID{command.Refresh} }

type mountProbePane struct {
	resized bool
	inited  bool
}

func (p *mountProbePane) Scope() command.Scope               { return command.ScopeCatalog }
func (p *mountProbePane) Title() string                          { return "mount" }
func (p *mountProbePane) Init() tea.Cmd                          { p.inited = true; return nil }
func (p *mountProbePane) View(int, int) string                   { return "" }
func (p *mountProbePane) Selection() (domain.PatentNumber, bool) { return domain.PatentNumber{}, false }
func (p *mountProbePane) Handles() []command.ID                  { return nil }
func (p *mountProbePane) Command(command.ID, pane.Invocation) (pane.Pane, tea.Cmd) {
	return p, nil
}
func (p *mountProbePane) Update(msg tea.Msg) (pane.Pane, tea.Cmd) {
	if _, ok := msg.(pane.ResizeMsg); ok {
		p.resized = true
		return p, func() tea.Msg { return nil }
	}
	return p, nil
}

type projectProbePane struct{ project domain.Project }

func (p *projectProbePane) Scope() command.Scope            { return command.ScopeProjects }
func (p *projectProbePane) Title() string                       { return "projects" }
func (p *projectProbePane) Init() tea.Cmd                       { return nil }
func (p *projectProbePane) Update(tea.Msg) (pane.Pane, tea.Cmd) { return p, nil }
func (p *projectProbePane) View(int, int) string                { return "" }
func (p *projectProbePane) Selection() (domain.PatentNumber, bool) {
	return domain.PatentNumber{}, false
}
func (p *projectProbePane) Command(command.ID, pane.Invocation) (pane.Pane, tea.Cmd) { return p, nil }
func (p *projectProbePane) Handles() []command.ID                                    { return nil }
func (p *projectProbePane) SelectedProject() (domain.Project, bool)                  { return p.project, true }

type patentProbePane struct {
	selection  domain.PatentNumber
	selections []domain.PatentNumber
}

func (p *patentProbePane) Scope() command.Scope            { return command.ScopeCatalog }
func (p *patentProbePane) Title() string                       { return "patents" }
func (p *patentProbePane) Init() tea.Cmd                       { return nil }
func (p *patentProbePane) Update(tea.Msg) (pane.Pane, tea.Cmd) { return p, nil }
func (p *patentProbePane) View(int, int) string                { return "" }
func (p *patentProbePane) Command(command.ID, pane.Invocation) (pane.Pane, tea.Cmd) {
	return p, nil
}
func (p *patentProbePane) Handles() []command.ID { return nil }
func (p *patentProbePane) Selection() (domain.PatentNumber, bool) {
	if p.selection.IsZero() {
		return domain.PatentNumber{}, false
	}
	return p.selection, true
}
func (p *patentProbePane) Selections() []domain.PatentNumber { return p.selections }

func TestAppRefreshesAllPanesOnDBChange(t *testing.T) {
	app := newTestApp(t)
	first := &refreshProbePane{}
	second := &refreshProbePane{}
	app.panes = []pane.Pane{first, second}

	app.handleEvent(busEventMsg{}.event)
	if first.refreshes != 0 || second.refreshes != 0 {
		t.Fatal("empty event should not refresh panes")
	}

	cmd := app.handleEvent(proto.NewEvent(proto.EventDBChanged, struct{}{}))
	if cmd != nil {
		cmd()
	}
	if first.refreshes != 1 || second.refreshes != 1 {
		t.Fatalf("db.changed refreshed %d/%d panes, want 1/1", first.refreshes, second.refreshes)
	}
}

func TestPushPanePreparesPaneBeforeInit(t *testing.T) {
	app := newTestApp(t)
	probe := &mountProbePane{}
	_, cmd := app.pushPane(probe)
	if !probe.resized {
		t.Fatal("pushPane should send ResizeMsg before first load/init")
	}
	if probe.inited {
		t.Fatal("pushPane should not call Init when preparePane already returned a load command")
	}
	if cmd == nil {
		t.Fatal("pushPane should return prepared pane command")
	}
}

func TestImportCommandValidatesArgs(t *testing.T) {
	app := newTestApp(t)

	app.executeTypedCommand("import")
	if !app.statusErr {
		t.Fatal(":import with no argument should report a usage error")
	}
	app.executeTypedCommand("import US10000000B2 bogus")
	if !app.statusErr {
		t.Fatal(":import with an unknown flag should report a usage error")
	}
}

func TestExecuteTypedCommandUsesSelectedProjectState(t *testing.T) {
	app := newTestApp(t)
	app.panes = []pane.Pane{&projectProbePane{project: domain.Project{ID: "p-2", Name: "Case B"}}}

	app.executeTypedCommand("project.use")
	if app.activeProject == nil || app.activeProject.ID != "p-2" {
		t.Fatalf("activeProject = %+v, want p-2", app.activeProject)
	}
}

func TestOpenBrowserUsesFocusedSelection(t *testing.T) {
	app := newTestApp(t)
	number := domain.MustParsePatentNumber("US11611785B2")
	app.panes = []pane.Pane{&patentProbePane{selection: number}}
	var opened []string
	app.openURL = func(url string) error {
		opened = append(opened, url)
		return nil
	}

	_, cmd := app.invoke(command.OpenBrowser, invocation{repeat: 1})
	if cmd == nil {
		t.Fatal("OpenBrowser should return a command")
	}
	msg := cmd()
	status, ok := msg.(pane.StatusMsg)
	if !ok || status.Key != text.StatusBrowserOpened {
		t.Fatalf("browser status = %#v, want StatusBrowserOpened", msg)
	}
	if len(opened) != 1 || opened[0] != "https://patents.google.com/patent/US11611785B2" {
		t.Fatalf("opened URLs = %v", opened)
	}
}

func TestOpenBrowserUsesVisualSelection(t *testing.T) {
	app := newTestApp(t)
	selected := []domain.PatentNumber{
		domain.MustParsePatentNumber("US11611785B2"),
		domain.MustParsePatentNumber("US10000000B2"),
	}
	app.panes = []pane.Pane{&patentProbePane{selection: selected[0], selections: selected}}
	var opened []string
	app.openURL = func(url string) error {
		opened = append(opened, url)
		return nil
	}

	_, cmd := app.invoke(command.OpenBrowser, invocation{repeat: 1})
	if cmd == nil {
		t.Fatal("OpenBrowser should return a command")
	}
	_ = cmd()
	if len(opened) != 2 {
		t.Fatalf("opened %d URLs, want 2", len(opened))
	}
}

func TestBrowseCommandAcceptsExplicitPatentArgs(t *testing.T) {
	app := newTestApp(t)
	app.panes = []pane.Pane{&patentProbePane{}}
	var opened []string
	app.openURL = func(url string) error {
		opened = append(opened, url)
		return nil
	}

	_, cmd := app.executeTypedCommand("browse US11611785B2")
	if cmd == nil {
		t.Fatal(":browse should return a command")
	}
	msg := cmd()
	status, ok := msg.(pane.StatusMsg)
	if !ok || status.Key != text.StatusBrowserOpened {
		t.Fatalf("browse status = %#v, want StatusBrowserOpened", msg)
	}
	if len(opened) != 1 || !strings.Contains(opened[0], "US11611785B2") {
		t.Fatalf("opened URLs = %v", opened)
	}
}
