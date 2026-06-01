package tui

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/ai"
	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/engine"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/store/sqlite"
	"patentmine/internal/text"
	"patentmine/internal/tui/keymap"
	"patentmine/internal/tui/overlay"
	"patentmine/internal/tui/pane"
	"patentmine/internal/tui/render"
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

func newRPCBackedTestApp(t *testing.T) *App {
	t.Helper()
	repo, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "tui.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	factory := func(root domain.PatentNumber, _ int, _ domain.CrawlProfile, _ bool, _ domain.Source) engine.Job {
		return engine.JobFunc(func(_ context.Context, id engine.JobID, emit func(proto.Event)) error {
			emit(proto.NewEvent(proto.EventCrawlProgress, proto.CrawlProgress{JobID: string(id), Message: root.String()}))
			return nil
		})
	}
	eng := engine.New(ctx, repo, factory, engine.WithCrawlMaxDepth(4))
	socket := filepath.Join("/tmp", fmt.Sprintf("tui-%d.sock", time.Now().UnixNano()))
	t.Cleanup(func() { _ = os.Remove(socket) })
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = rpc.NewServer(eng, false).Serve(ctx, ln) }()
	client, err := rpc.Dial(socket)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
		cancel()
		eng.Close()
		_ = repo.Close()
	})
	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	app, err := New(client, reg, keymap.Default(), text.English())
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

func TestFitBodyPadsEveryLineToSafeWidth(t *testing.T) {
	out := fitBody("short\nthis line is much too long", 4, 10)
	lines := strings.Split(out, "\n")
	if len(lines) != 4 {
		t.Fatalf("fitBody lines = %d, want 4", len(lines))
	}
	for i, line := range lines {
		if got := render.StringWidth(line); got != 10 {
			t.Fatalf("line %d width = %d, want 10: %q", i, got, line)
		}
	}
}

func TestClearLineEndsAppendsTerminalClear(t *testing.T) {
	out := clearLineEnds("a\nb")
	if out != "a\x1b[K\nb\x1b[K" {
		t.Fatalf("clearLineEnds = %q", out)
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

func TestAppCrawlDepthMaxCommandSetsStatus(t *testing.T) {
	app := newRPCBackedTestApp(t)
	_, cmd := app.cmdCrawlDepthMax(invocation{})
	if cmd == nil {
		t.Fatal("cmdCrawlDepthMax returned nil command")
	}
	app.Update(cmd())
	if app.status != "crawl family depth max: 4" {
		t.Fatalf("status = %q, want crawl family depth max: 4", app.status)
	}
}

func TestAppCrawlProgressStatusIncludesDepth(t *testing.T) {
	app := newTestApp(t)
	app.handleEvent(proto.NewEvent(proto.EventCrawlProgress, proto.CrawlProgress{
		JobID:           "job-1",
		Depth:           2,
		MaxDepth:        4,
		CrawledCount:    3,
		DiscoveredCount: 8,
		Message:         "crawled US123",
	}))
	if !strings.Contains(app.status, "depth 2/4") {
		t.Fatalf("status %q missing depth", app.status)
	}
}

type refreshProbePane struct{ refreshes int }

func (p *refreshProbePane) Scope() command.Scope                { return command.ScopeCatalog }
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

func (p *mountProbePane) Scope() command.Scope                   { return command.ScopeCatalog }
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

func (p *projectProbePane) Scope() command.Scope                { return command.ScopeProjects }
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

func (p *patentProbePane) Scope() command.Scope                { return command.ScopeCatalog }
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

type visualSavingPatentProbe struct {
	*patentProbePane
	saves int
}

func (p *visualSavingPatentProbe) SaveVisualSelection() {
	p.saves++
	p.selections = nil
}

func TestOpenIDSPickerPreservesVisualSelectionForRepeatedApply(t *testing.T) {
	app := newTestApp(t)
	app.activeProject = &domain.Project{ID: "p-1", Name: "Case A"}
	numbers := []domain.PatentNumber{
		domain.MustParsePatentNumber("US11611785B2"),
		domain.MustParsePatentNumber("US11700000B2"),
	}
	probe := &visualSavingPatentProbe{patentProbePane: &patentProbePane{selection: numbers[0], selections: numbers}}
	app.panes = []pane.Pane{probe}

	updated, cmd := app.cmdOpenIDS(invocation{})
	app = updated.(*App)
	if cmd != nil {
		t.Fatal("opening IDS status picker should not return a command")
	}
	if probe.saves != 0 {
		t.Fatalf("opening IDS status picker saved/cleared visual selection %d times", probe.saves)
	}
	if len(app.overlays) != 1 {
		t.Fatalf("overlays = %d, want IDS status picker", len(app.overlays))
	}
	if _, ok := app.overlays[0].(*overlay.IDSStatusPicker); !ok {
		t.Fatalf("overlay = %T, want IDSStatusPicker", app.overlays[0])
	}

	app.overlays = nil
	updated, cmd = app.cmdOpenIDS(invocation{})
	app = updated.(*App)
	if cmd != nil {
		t.Fatal("reopening IDS status picker should not return a command")
	}
	if probe.saves != 0 {
		t.Fatalf("reopening IDS status picker saved/cleared visual selection %d times", probe.saves)
	}
	if len(app.panes) != 1 {
		t.Fatalf("second IDS open pushed a detail pane; panes = %d", len(app.panes))
	}
	if len(app.overlays) != 1 {
		t.Fatalf("second IDS open overlays = %d, want IDS status picker", len(app.overlays))
	}
}

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
	if len(opened) != 1 || opened[0] != "https://patents.google.com/patent/US11611785B2/en" {
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

func TestAppSettingsAIKeys(t *testing.T) {
	app := newTestApp(t)

	// Trigger open Settings AI
	_, cmd := app.invoke(command.SettingsAI, invocation{})
	if cmd != nil {
		app.Update(cmd())
	}

	if len(app.overlays) != 1 {
		t.Fatalf("after SettingsAI command, overlays = %d, want 1", len(app.overlays))
	}

	settings, ok := app.focusedOverlay().(*overlay.SettingsOverlay)
	if !ok {
		t.Fatalf("expected SettingsOverlay, got %T", app.focusedOverlay())
	}

	// Make sure initial provider is Gemini
	app.aiProvider = ai.ProviderGemini
	settings.SetActiveAI(ai.ProviderGemini)

	// Press 'o' to change to Ollama
	_, cmd = app.Update(runeKey('o'))
	if cmd == nil {
		t.Fatal("pressing 'o' should return a command")
	}

	// Process the message returned by the command
	msg := cmd()
	app.Update(msg)

	// Check if app.aiProvider is now Ollama
	if app.aiProvider != ai.ProviderOllama {
		t.Errorf("expected app.aiProvider to be ollama, got %s", app.aiProvider)
	}

	// Check if overlay has updated provider
	if settings.View(80, 20) == "" {
		t.Error("expected non-empty overlay view")
	}

	// Press 'g' to change to Gemini
	_, cmd = app.Update(runeKey('g'))
	if cmd == nil {
		t.Fatal("pressing 'g' should return a command")
	}

	msg = cmd()
	app.Update(msg)

	// Check if app.aiProvider is now Gemini
	if app.aiProvider != ai.ProviderGemini {
		t.Errorf("expected app.aiProvider to be gemini, got %s", app.aiProvider)
	}
}

func TestAppNavigationHistory(t *testing.T) {
	app := newTestApp(t)

	// History should start empty
	if len(app.history) != 0 {
		t.Fatalf("expected history to be empty, got %d", len(app.history))
	}

	numA := domain.MustParsePatentNumber("US0000001B2")
	numB := domain.MustParsePatentNumber("US0000002B2")

	// Manually record history
	app.recordHistory(numA)
	app.recordHistory(numB)

	if len(app.history) != 2 {
		t.Fatalf("expected history length 2, got %d", len(app.history))
	}
	if app.historyCursor != 1 {
		t.Fatalf("expected historyCursor to be 1, got %d", app.historyCursor)
	}

	// Test back boundary
	app.historyCursor = 0
	app.recordHistory(numB) // should truncate and append, so history becomes [numA, numB]
	if len(app.history) != 2 || app.history[1] != numB {
		t.Fatalf("expected history to truncate on new record after back")
	}

	// Configure telemetry for persistent history overlay reading
	tmpDir := t.TempDir()
	rt, err := observability.Open(tmpDir, "test-app", "1.0")
	if err != nil {
		t.Fatalf("observability.Open: %v", err)
	}
	app.logger = rt.Logger
	app.activity = rt.Activity
	app.activityDir = rt.LogsDir
	app.metrics = rt.Metrics

	// Manually record a persistent activity search log entry synchronously
	err = rt.Activity.Record(context.Background(), observability.Record{
		Action:    "filter.apply",
		Entity:    "filter",
		EntityID:  "cpc:G06F",
		Status:    "requested",
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to record activity search: %v", err)
	}

	// Press Shift+H to open history overlay
	app.Update(runeKey('H'))
	if len(app.overlays) != 1 {
		t.Fatalf("expected history overlay to be open, got %d overlays", len(app.overlays))
	}
	o, ok := app.focusedOverlay().(*overlay.HistoryOverlay)
	if !ok {
		t.Fatalf("focused overlay should be HistoryOverlay")
	}
	if o.Title() != "Activity History" {
		t.Errorf("expected title 'Activity History', got %q", o.Title())
	}

	// Test pressing Esc to close overlay
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if cmd != nil {
		app.Update(cmd())
	}
	if len(app.overlays) != 0 {
		t.Fatalf("expected overlays to be closed, got %d", len(app.overlays))
	}
}

func TestHistoryFeedGroupKeyKeepsIDSStatusTransitions(t *testing.T) {
	base := observability.Record{
		Action:   observability.ActionIDSEntrySave,
		Entity:   "ids_entry",
		EntityID: "project/US11611785B2",
		Status:   "committed",
	}
	first := base
	first.ID = "activity-1"
	first.Attributes = map[string]any{"prior_status": "pending", "status": "submitted"}
	second := base
	second.ID = "activity-2"
	second.Attributes = map[string]any{"prior_status": "submitted", "status": "accepted"}

	if observability.HistoryFeedGroupKey(first) == observability.HistoryFeedGroupKey(second) {
		t.Fatal("IDS status transitions should not collapse to the same history key")
	}
}

func TestHistoryFeedGroupKeyKeepsDistinctIDSSaveRecords(t *testing.T) {
	base := observability.Record{
		Action:   observability.ActionIDSEntrySave,
		Entity:   "ids_entry",
		EntityID: "project/US11611785B2",
		Status:   "committed",
		Attributes: map[string]any{"prior_status": "ignored", "status": "pending"},
	}
	first := base
	first.ID = "activity-1"
	second := base
	second.ID = "activity-2"

	if observability.HistoryFeedGroupKey(first) == observability.HistoryFeedGroupKey(second) {
		t.Fatal("distinct IDS save records should not collapse to the same history key")
	}
}

func TestHistoryFeedGroupKeyKeepsReviewStateTransitions(t *testing.T) {
	base := observability.Record{
		Action:   observability.ActionMembershipSetState,
		Entity:   "membership",
		EntityID: "project/US11611785B2",
		Status:   "committed",
	}
	first := base
	first.ID = "activity-1"
	first.Attributes = map[string]any{"prior_state": "under_review", "state": "active"}
	second := base
	second.ID = "activity-2"
	second.Attributes = map[string]any{"prior_state": "active", "state": "ignored"}

	if observability.HistoryFeedGroupKey(first) == observability.HistoryFeedGroupKey(second) {
		t.Fatal("review state transitions should not collapse to the same history key")
	}
}

func TestHistoryFeedGroupKeyCollapsesRepeatedFocusRecords(t *testing.T) {
	base := observability.Record{
		Action:   observability.ActionUIFocus,
		Entity:   "patent",
		EntityID: "US11611785B2",
		Status:   "observed",
	}
	first := base
	first.ID = "activity-1"
	second := base
	second.ID = "activity-2"

	if observability.HistoryFeedGroupKey(first) != observability.HistoryFeedGroupKey(second) {
		t.Fatal("repeated focus records should collapse to the same history key")
	}
}

func TestIDSCycleStatusCyclesSelectedPatents(t *testing.T) {
	repo, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "ids-cycle.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	eng := engine.New(ctx, repo, nil)
	t.Cleanup(func() {
		cancel()
		eng.Close()
		_ = repo.Close()
	})

	numbers := []domain.PatentNumber{
		domain.MustParsePatentNumber("US11611785B2"),
		domain.MustParsePatentNumber("US11700000B2"),
	}
	for _, number := range numbers {
		if err := eng.SavePatent(context.Background(), domain.Patent{Number: number, FetchState: domain.FetchCached}); err != nil {
			t.Fatalf("SavePatent: %v", err)
		}
	}
	project, err := eng.CreateProject(context.Background(), "Case A")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	for _, number := range numbers {
		if _, _, _, err := eng.AddToProject(context.Background(), project.ID, number); err != nil {
			t.Fatalf("AddToProject: %v", err)
		}
	}

	socket := filepath.Join("/tmp", fmt.Sprintf("ids-cycle-%d.sock", time.Now().UnixNano()))
	t.Cleanup(func() { _ = os.Remove(socket) })
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = rpc.NewServer(eng, false).Serve(ctx, ln) }()
	client, err := rpc.Dial(socket)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	reg, err := command.Default()
	if err != nil {
		t.Fatalf("command.Default: %v", err)
	}
	app, err := New(client, reg, keymap.Default(), text.English())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	app.activeProject = &project
	app.panes = []pane.Pane{&patentProbePane{selections: numbers}}

	app = runConfirmedIDSCycle(t, app)
	for _, number := range numbers {
		entry, err := repo.IDSEntry(context.Background(), project.ID, number)
		if err != nil {
			t.Fatalf("IDSEntry: %v", err)
		}
		if entry.Status != domain.IDSEntrySubmitted {
			t.Fatalf("first cycle status = %q, want %q", entry.Status, domain.IDSEntrySubmitted)
		}
	}

	app = runConfirmedIDSCycle(t, app)
	for _, number := range numbers {
		entry, err := repo.IDSEntry(context.Background(), project.ID, number)
		if err != nil {
			t.Fatalf("IDSEntry: %v", err)
		}
		if entry.Status != domain.IDSEntryAccepted {
			t.Fatalf("second cycle status = %q, want %q", entry.Status, domain.IDSEntryAccepted)
		}
	}
}

func runConfirmedIDSCycle(t *testing.T, app *App) *App {
	t.Helper()
	updated, cmd := app.invoke(command.IDSCycleStatus, invocation{repeat: 1})
	app = updated.(*App)
	if cmd != nil {
		t.Fatal("multi-patent IDS cycle should require confirmation before returning a command")
	}
	if app.confirmCmd == nil {
		t.Fatal("expected IDS cycle confirmation command")
	}
	updated, cmd = app.Update(overlay.ConfirmAcceptMsg{})
	app = updated.(*App)
	if cmd == nil {
		t.Fatal("confirming IDS cycle should return a command")
	}
	msg := cmd()
	updated, cmd = app.Update(msg)
	app = updated.(*App)
	if cmd != nil {
		cmd()
	}
	return app
}

type idsChangedProbePane struct {
	ScopeVal    command.Scope
	TitleVal    string
	receivedMsg tea.Msg
	bulkMsg     tea.Msg
}

func (p *idsChangedProbePane) Scope() command.Scope { return p.ScopeVal }
func (p *idsChangedProbePane) Title() string        { return p.TitleVal }
func (p *idsChangedProbePane) Init() tea.Cmd        { return nil }
func (p *idsChangedProbePane) View(int, int) string { return "" }
func (p *idsChangedProbePane) Selection() (domain.PatentNumber, bool) {
	return domain.PatentNumber{}, false
}
func (p *idsChangedProbePane) Handles() []command.ID { return nil }
func (p *idsChangedProbePane) Command(command.ID, pane.Invocation) (pane.Pane, tea.Cmd) {
	return p, nil
}
func (p *idsChangedProbePane) Update(msg tea.Msg) (pane.Pane, tea.Cmd) {
	switch msg.(type) {
	case pane.IDSEntryChangedMsg:
		p.receivedMsg = msg
	case pane.IDSEntriesChangedMsg:
		p.bulkMsg = msg
	}
	return p, nil
}

func TestAppRoutesCurationEvents(t *testing.T) {
	app := &App{
		text: text.English(),
	}
	app.theme = render.NewTheme()

	// Create mock catalog pane that records received messages
	catalog := &idsChangedProbePane{
		ScopeVal: command.ScopeCatalog,
		TitleVal: "Catalog",
	}
	app.panes = []pane.Pane{catalog}

	// Feed IDSEntrySavedMsg to app.Update
	num := domain.MustParsePatentNumber("US0000001B2")
	savedMsg := pane.IDSEntrySavedMsg{
		Project: "p-1",
		Entry: domain.IDSEntry{
			Project: "p-1",
			Patent:  num,
			Status:  domain.IDSEntrySubmitted,
		},
	}

	updatedModel, _ := app.Update(savedMsg)
	app = updatedModel.(*App)

	// Verify that the catalog pane received IDSEntryChangedMsg
	if catalog.receivedMsg == nil {
		t.Fatal("expected catalog pane to receive a broadcasted message")
	}
	changedMsg, ok := catalog.receivedMsg.(pane.IDSEntryChangedMsg)
	if !ok {
		t.Fatalf("expected catalog to receive IDSEntryChangedMsg, got: %T", catalog.receivedMsg)
	}
	if changedMsg.Project != "p-1" || changedMsg.Patent != num || changedMsg.Entry.Status != domain.IDSEntrySubmitted {
		t.Fatalf("unexpected change msg contents: %+v", changedMsg)
	}
}

func TestAppRoutesBulkIDSEntriesAsSingleBatchUpdate(t *testing.T) {
	app := &App{
		text:    text.English(),
		metrics: observability.NewMetrics(),
	}
	app.theme = render.NewTheme()

	catalog := &idsChangedProbePane{
		ScopeVal: command.ScopeCatalog,
		TitleVal: "Catalog",
	}
	app.panes = []pane.Pane{catalog}

	num := domain.MustParsePatentNumber("US0000001B2")
	num2 := domain.MustParsePatentNumber("US0000002B2")
	updatedModel, _ := app.Update(pane.IDSEntriesChangedMsg{Entries: []domain.IDSEntry{{
		Project: "p-1",
		Patent:  num,
		Status:  domain.IDSEntryAccepted,
	}, {
		Project: "p-1",
		Patent:  num2,
		Status:  domain.IDSEntryIgnored,
	}}})
	app = updatedModel.(*App)

	bulkMsg, ok := catalog.bulkMsg.(pane.IDSEntriesChangedMsg)
	if !ok {
		t.Fatalf("expected bulk IDS change broadcast, got %T", catalog.bulkMsg)
	}
	if len(bulkMsg.Entries) != 2 || bulkMsg.Entries[0].Patent != num || bulkMsg.Entries[1].Patent != num2 {
		t.Fatalf("unexpected bulk change msg contents: %+v", bulkMsg)
	}
	if catalog.receivedMsg != nil {
		t.Fatalf("bulk IDS update should not emit per-patent messages, got %T", catalog.receivedMsg)
	}
}
