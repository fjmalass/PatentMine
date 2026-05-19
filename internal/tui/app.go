// Package tui is the terminal client. The App is the bubbletea root: it owns
// the pane and overlay stacks and routes input, but holds no business state —
// every screen's state lives in its own Pane or Overlay. That decomposition is
// the deliberate structural defence against a god-object UI model.
//
// Input flows through one path. A key chord and a typed command both resolve
// to a command.ID and run through invoke, so the two can never diverge. Every
// command.ID is serviced by exactly one handler — an entry in appHandlers or a
// pane/overlay that lists the ID in Handles — and validateWiring fails the boot
// if any bound key or typed command would resolve to an unhandled ID.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/keys"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/keymap"
	"patentmine/internal/tui/overlay"
	"patentmine/internal/tui/pane"
	"patentmine/internal/tui/render"
	appversion "patentmine/internal/version"
)

// statusRows is the bottom status line the App always draws.
const statusRows = 1

// pingTimeout bounds the startup version probe.
const pingTimeout = 5 * time.Second

// overlay box sizing.
const (
	overlayMaxWidth  = 76
	overlayMaxHeight = 22
	overlayMargin    = 4 // free space kept around the box
	overlayChrome    = 4 // border (2) + horizontal padding (2)
)

// busEventMsg carries a daemon event into the bubbletea update loop.
type busEventMsg struct{ event proto.Event }

// eventsClosedMsg signals that the daemon event stream ended.
type eventsClosedMsg struct{}

// pingLoadedMsg carries the daemon's reported build version.
type pingLoadedMsg struct {
	version string
	err     error
}

// invocation carries the arguments of one command request: empty for a key
// chord, populated for a typed command.
type invocation struct {
	repeat int
	args   []string
}

// appHandler services one command at the App level. appHandlers is the single
// table of App-handled commands; validateWiring reads it to prove every bound
// key reaches a handler.
type appHandler func(*App, invocation) (tea.Model, tea.Cmd)

var appHandlers = map[command.ID]appHandler{
	command.Quit:               (*App).cmdQuit,
	command.Help:               (*App).cmdHelp,
	command.OpenSearch:         (*App).cmdOpenSearch,
	command.OpenCommand:        (*App).cmdOpenCommand,
	command.JumpMode:           (*App).cmdJumpMode,
	command.CloseOverlay:       (*App).cmdCloseOverlay,
	command.Back:               (*App).cmdBack,
	command.OpenDetail:         (*App).cmdOpenDetail,
	command.OpenCitations:      (*App).cmdOpenCitations,
	command.OpenCitedBy:        (*App).cmdOpenCitedBy,
	command.OpenProjects:       (*App).cmdOpenProjects,
	command.ProjectActivate:    (*App).cmdProjectActivate,
	command.ProjectClearActive: (*App).cmdProjectClear,
	command.ProjectCreate:      (*App).cmdProjectCreate,
	command.AddToProject:       (*App).cmdAddToProject,
	command.Import:             (*App).cmdImport,
	command.MarkStored:         (*App).cmdMarkStored,
	command.MarkUnderReview:    (*App).cmdMarkUnderReview,
	command.MarkIgnored:        (*App).cmdMarkIgnored,
	command.MarkDeleted:        (*App).cmdMarkDeleted,
	command.TagAdd:             (*App).cmdTagAdd,
	command.TagRemove:          (*App).cmdTagRemove,
	command.PatentDelete:       (*App).cmdPatentDelete,
}

// typedAcceptsArgs lists the commands whose typed form takes arguments. Every
// other typed command is rejected with a usage error when given any.
var typedAcceptsArgs = map[command.ID]bool{
	command.AddToProject:    true,
	command.ProjectActivate: true,
	command.ProjectCreate:   true,
	command.Import:          true,
	command.TagAdd:          true,
	command.TagRemove:       true,
}

// App is the bubbletea root model.
type App struct {
	client          *rpc.Client
	registry        *command.Registry
	keymaps         *keymap.Keymaps
	theme           render.Theme
	text            *text.Catalog
	reader          keys.Reader
	saveLastProject func(domain.ProjectID) error

	panes      []pane.Pane
	overlays   []overlay.Overlay
	confirmCmd tea.Cmd // pending action awaiting confirmation

	status        string
	statusErr     bool
	width         int
	height        int
	activeProject *domain.Project
	lastProjectID domain.ProjectID
	tuiVersion    string
	daemonVersion string
}

type Option func(*App)

func WithLastProject(id domain.ProjectID) Option {
	return func(a *App) { a.lastProjectID = id }
}

func WithLastProjectSaver(save func(domain.ProjectID) error) Option {
	return func(a *App) { a.saveLastProject = save }
}

// New builds the App with the splash/project selector as the initial pane. It
// fails when the keymap, command registry, and handlers are not consistent —
// see validateWiring — so a wiring mistake is caught at startup.
func New(client *rpc.Client, registry *command.Registry, keymaps *keymap.Keymaps, catalog *text.Catalog, opts ...Option) (*App, error) {
	if err := validateWiring(registry, keymaps, catalog); err != nil {
		return nil, err
	}
	theme := render.NewTheme()
	app := &App{
		client:        client,
		registry:      registry,
		keymaps:       keymaps,
		theme:         theme,
		text:          catalog,
		tuiVersion:    appversion.String(),
		daemonVersion: "connecting",
	}
	app.status = catalog.T(text.StatusWelcome)
	for _, opt := range opts {
		opt(app)
	}
	app.panes = []pane.Pane{pane.NewSplash(client, theme, app.lastProjectID,
		app.splashFooterHint(), app.splashEmptyHint())}
	return app, nil
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	cmds := []tea.Cmd{a.panes[0].Init(), a.fetchPing()}
	if a.client != nil {
		cmds = append(cmds, a.listen())
	}
	return tea.Batch(cmds...)
}

// listen waits for one daemon event and delivers it as a message.
func (a *App) listen() tea.Cmd {
	client := a.client
	return func() tea.Msg {
		ev, ok := <-client.Events()
		if !ok {
			return eventsClosedMsg{}
		}
		return busEventMsg{event: ev}
	}
}

// fetchPing asks the daemon which build version it is running.
func (a *App) fetchPing() tea.Cmd {
	if a.client == nil {
		return nil
	}
	client := a.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
		defer cancel()
		var res proto.PingResult
		err := client.Call(ctx, proto.MethodPing, nil, &res)
		return pingLoadedMsg{version: res.Version, err: err}
	}
}

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		return a, a.broadcast(pane.ResizeMsg{Width: m.Width, Height: max(m.Height-statusRows, 1)})
	case tea.KeyMsg:
		return a.handleKey(m)
	case overlay.ConfirmAcceptMsg:
		a.popOverlay()
		cmd := a.confirmCmd
		a.confirmCmd = nil
		return a, cmd
	case overlay.ConfirmRejectMsg:
		a.popOverlay()
		a.confirmCmd = nil
		return a, nil
	case overlay.PromptSubmitMsg:
		a.popOverlay()
		return a.executeTypedCommand(m.Input)
	case overlay.PromptCloseMsg:
		a.popOverlay()
		return a, nil
	case overlay.TextSubmitMsg:
		a.popOverlay()
		return a.handleTextSubmit(m)
	case overlay.JumpSelectMsg:
		a.popOverlay()
		if provider, ok := a.focusedPane().(pane.JumpProvider); ok {
			provider.JumpTo(m.Line)
		}
		return a, nil
	case overlay.CloseOverlayMsg:
		a.popOverlay()
		return a, nil
	case pane.StatusMsg:
		a.status, a.statusErr = a.text.Tf(m.Key, m.Args...), m.Error
		if m.Key == text.StatusIngestStarted && len(m.Args) >= 2 {
			jobID, _ := m.Args[1].(string)
			title := "Fetching " + m.Args[0].(string)
			loading := overlay.NewLoading(a.theme, jobID, title)
			a.overlays = append(a.overlays, loading)
			return a, loading.Init()
		}
		return a, nil
	case pingLoadedMsg:
		if m.err != nil {
			a.daemonVersion = "unavailable"
			return a, nil
		}
		a.daemonVersion = m.version
		return a, nil
	case busEventMsg:
		return a, tea.Batch(
			a.handleEvent(m.event),
			a.broadcastOverlays(m.event),
			a.listen())
	case eventsClosedMsg:
		a.setErr(text.StatusDaemonClosed)
		return a, nil
	default:
		// rpc results, spinner ticks and the like — let every pane/overlay consume what is theirs.
		return a, tea.Batch(a.broadcast(msg), a.broadcastOverlays(msg))
	}
}

// broadcast forwards a message to every pane.
func (a *App) broadcast(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for i, p := range a.panes {
		updated, cmd := p.Update(msg)
		a.panes[i] = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// broadcastOverlays forwards a message to every overlay.
func (a *App) broadcastOverlays(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd
	for i, ov := range a.overlays {
		if u, ok := ov.(interface {
			Update(tea.Msg) (overlay.Overlay, tea.Cmd)
		}); ok {
			updated, cmd := u.Update(msg)
			a.overlays[i] = updated
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}
	return tea.Batch(cmds...)
}

// handleKey feeds a key press through the chord reader and keymap.
func (a *App) handleKey(m tea.KeyMsg) (tea.Model, tea.Cmd) {
	if len(a.overlays) > 0 {
		if handler, ok := a.focusedOverlay().(overlay.KeyHandler); ok {
			updated, cmd, consumed := handler.HandleKey(m)
			a.overlays[len(a.overlays)-1] = updated
			if consumed {
				return a, cmd
			}
		}
	}
	stack := a.keyStack()
	chord, ok := a.reader.Feed(keys.Key(m.String()), stack.Match)
	if !ok {
		return a, nil // buffered as part of a longer sequence
	}
	id, ok := stack.Resolve(chord.Sequence())
	if !ok {
		return a, nil // unbound sequence
	}
	return a.invoke(id, invocation{repeat: chord.Repeat()})
}

// keyStack composes the active keymap. With an overlay open the pane layer is
// deliberately left out so only the overlay and global bindings apply.
func (a *App) keyStack() *keymap.Stack {
	stack := keymap.NewStack(a.keymaps.Base())
	var ctx command.Context
	if len(a.overlays) > 0 {
		ctx = command.ContextOverlay
	} else {
		ctx = a.focusedPane().Context()
	}
	if layer := a.keymaps.Context(ctx); layer != nil {
		stack.Push(layer)
	}
	return stack
}

// invoke carries out a resolved command. App-level commands run from the
// appHandlers table; everything else is forwarded to the focused overlay or
// pane — but only when that overlay or pane lists the command in Handles, so a
// command can never be silently dropped.
func (a *App) invoke(id command.ID, inv invocation) (tea.Model, tea.Cmd) {
	if handler, ok := appHandlers[id]; ok {
		return handler(a, inv)
	}
	if len(a.overlays) > 0 {
		ov := a.focusedOverlay()
		if !slices.Contains(ov.Handles(), id) {
			return a.unhandled(id)
		}
		updated, cmd := ov.Command(id, inv.repeat)
		a.overlays[len(a.overlays)-1] = updated
		return a, cmd
	}
	p := a.focusedPane()
	if !slices.Contains(p.Handles(), id) {
		return a.unhandled(id)
	}
	updated, cmd := p.Command(id, inv.repeat)
	a.panes[len(a.panes)-1] = updated
	return a, cmd
}

// unhandled reports a command that reached invoke with no handler. validateWiring
// makes this unreachable for bound keys and typed commands; it stays as a
// visible last line of defence rather than a silent no-op.
func (a *App) unhandled(id command.ID) (tea.Model, tea.Cmd) {
	a.setErr(text.StatusUnhandledCommand, string(id))
	return a, nil
}

// --- App-level command handlers ---------------------------------------------

func (a *App) cmdQuit(invocation) (tea.Model, tea.Cmd) { return a, tea.Quit }

func (a *App) cmdHelp(invocation) (tea.Model, tea.Cmd) {
	if len(a.overlays) == 0 {
		a.overlays = append(a.overlays, overlay.NewHelp(a.registry, a.keymaps, a.theme, a.text))
	}
	return a, nil
}

func (a *App) cmdOpenSearch(invocation) (tea.Model, tea.Cmd) {
	return a.openPrompt(overlay.PromptPalette)
}

func (a *App) cmdOpenCommand(invocation) (tea.Model, tea.Cmd) {
	return a.openPrompt(overlay.PromptDirect)
}

// cmdJumpMode opens the jump overlay for the focused pane, when that pane
// offers jump anchors. It is a no-op when an overlay is already open, the pane
// does not support jump mode, or the pane has not yet rendered any anchors.
func (a *App) cmdJumpMode(invocation) (tea.Model, tea.Cmd) {
	if len(a.overlays) > 0 {
		return a, nil
	}
	provider, ok := a.focusedPane().(pane.JumpProvider)
	if !ok {
		return a, nil
	}
	anchors := provider.JumpAnchors()
	if len(anchors) == 0 {
		return a, nil
	}
	a.overlays = append(a.overlays, overlay.NewJump(a.theme, anchors))
	return a, nil
}

func (a *App) cmdCloseOverlay(invocation) (tea.Model, tea.Cmd) {
	a.popOverlay()
	return a, nil
}

func (a *App) cmdBack(invocation) (tea.Model, tea.Cmd) {
	if len(a.overlays) > 0 {
		a.popOverlay()
	} else if len(a.panes) > 1 {
		a.panes = a.panes[:len(a.panes)-1]
	}
	return a, nil
}

func (a *App) cmdOpenDetail(invocation) (tea.Model, tea.Cmd) { return a.openDetail() }
func (a *App) cmdOpenCitations(invocation) (tea.Model, tea.Cmd) {
	return a.openCitations(domain.RelationCites)
}
func (a *App) cmdOpenCitedBy(invocation) (tea.Model, tea.Cmd) {
	return a.openCitations(domain.RelationCitedBy)
}

func (a *App) cmdOpenProjects(invocation) (tea.Model, tea.Cmd) {
	return a.pushPane(pane.NewProjects(a.client, a.theme))
}

func (a *App) cmdProjectClear(invocation) (tea.Model, tea.Cmd) {
	a.activeProject = nil
	a.setStatus(text.StatusClearedProject)
	return a, a.broadcast(pane.ProjectChangedMsg{})
}

func (a *App) cmdMarkStored(invocation) (tea.Model, tea.Cmd) {
	return a.runReviewState(domain.ReviewStateLoad)
}
func (a *App) cmdMarkUnderReview(invocation) (tea.Model, tea.Cmd) {
	return a.runReviewState(domain.ReviewStateUnderReview)
}
func (a *App) cmdMarkIgnored(invocation) (tea.Model, tea.Cmd) {
	return a.runReviewState(domain.ReviewStateIgnored)
}
func (a *App) cmdMarkDeleted(invocation) (tea.Model, tea.Cmd) {
	return a.runReviewState(domain.ReviewStateDeleted)
}

func (a *App) cmdProjectActivate(inv invocation) (tea.Model, tea.Cmd) {
	switch len(inv.args) {
	case 0:
		return a.activateProject()
	case 1:
		return a.activateProjectByArg(inv.args[0])
	default:
		return a.usageError(command.ProjectActivate)
	}
}

func (a *App) cmdAddToProject(inv invocation) (tea.Model, tea.Cmd) {
	switch len(inv.args) {
	case 0:
		return a.runProjectAction(func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
			return pane.AddToProjectCmd(a.client, project, patent)
		})
	case 1:
		number, err := domain.ParsePatentNumber(inv.args[0])
		if err != nil {
			a.setErr(text.StatusInvalidPatentNumber, err.Error())
			return a, nil
		}
		if a.activeProject == nil {
			a.setErr(text.StatusNoActiveProject)
			return a, nil
		}
		if a.client == nil {
			a.setErr(text.StatusDaemonUnavailable)
			return a, nil
		}
		return a, pane.AddToProjectCmd(a.client, a.activeProject.ID, number)
	default:
		return a.usageError(command.AddToProject)
	}
}

// cmdTagAdd tags the selected patent within the active project. The tag name
// is the typed argument; it may contain spaces.
func (a *App) cmdTagAdd(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) == 0 {
		return a.usageError(command.TagAdd)
	}
	name := strings.Join(inv.args, " ")
	return a.runProjectAction(func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
		return pane.AssignTagCmd(a.client, project, patent, name)
	})
}

// cmdTagRemove removes a tag from the selected patent within the active project.
func (a *App) cmdTagRemove(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) == 0 {
		return a.usageError(command.TagRemove)
	}
	name := strings.Join(inv.args, " ")
	return a.runProjectAction(func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
		return pane.RemoveTagCmd(a.client, project, patent, name)
	})
}

func (a *App) cmdPatentDelete(invocation) (tea.Model, tea.Cmd) {
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	numbers := a.focusedSelections()
	if len(numbers) == 0 {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	var msg string
	var confirmCmd tea.Cmd
	if len(numbers) == 1 {
		msg = "Delete " + numbers[0].String() + "? This cannot be undone."
		confirmCmd = pane.DeletePatentCmd(a.client, numbers[0])
	} else {
		msg = fmt.Sprintf("Delete %d patents? This cannot be undone.", len(numbers))
		cmds := make([]tea.Cmd, 0, len(numbers))
		for _, n := range numbers {
			cmds = append(cmds, pane.DeletePatentCmd(a.client, n))
		}
		confirmCmd = tea.Batch(cmds...)
	}
	a.confirmCmd = confirmCmd
	a.overlays = append(a.overlays, overlay.NewConfirm(a.theme, msg))
	return a, nil
}

// cmdProjectCreate opens a name-entry overlay, or — given a typed name — creates
// the project directly.
func (a *App) cmdProjectCreate(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) > 0 {
		return a.createProject(strings.Join(inv.args, " "))
	}
	a.overlays = append(a.overlays, overlay.NewTextInput(
		a.theme, a.text, overlay.PurposeCreateProject, text.NewProjectTitle, text.NewProjectCaption))
	return a, nil
}

// cmdImport fetches a patent by number — optionally forcing past the file
// cache — or loads a fixture file when the argument is a path.
func (a *App) cmdImport(inv invocation) (tea.Model, tea.Cmd) {
	if len(inv.args) == 0 {
		return a.usageError(command.Import)
	}
	force := false
	for _, arg := range inv.args[1:] {
		if !strings.EqualFold(arg, "force") {
			return a.usageError(command.Import)
		}
		force = true
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	target := inv.args[0]
	if isFixturePath(target) {
		return a, pane.ImportFileCmd(a.client, target)
	}
	number, err := domain.ParsePatentNumber(target)
	if err != nil {
		a.setErr(text.StatusInvalidPatentNumber, err.Error())
		return a, nil
	}
	return a, pane.IngestCmd(a.client, number, 0, force)
}

// isFixturePath reports whether arg names a fixture file rather than a patent.
func isFixturePath(arg string) bool {
	return strings.ContainsAny(arg, `/\`) || strings.HasSuffix(strings.ToLower(arg), ".json")
}

// handleTextSubmit routes a value entered in a TextInput overlay to its action.
func (a *App) handleTextSubmit(m overlay.TextSubmitMsg) (tea.Model, tea.Cmd) {
	switch m.Purpose {
	case overlay.PurposeCreateProject:
		return a.createProject(m.Value)
	default:
		return a, nil
	}
}

// createProject sends a project.create request for name.
func (a *App) createProject(name string) (tea.Model, tea.Cmd) {
	name = strings.TrimSpace(name)
	if name == "" {
		a.setErr(text.StatusProjectNameEmpty)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	return a, pane.CreateProjectCmd(a.client, name)
}

// usageError reports the correct invocation of a command.
func (a *App) usageError(id command.ID) (tea.Model, tea.Cmd) {
	cmd, _ := a.registry.Lookup(id)
	usage := cmd.Usage
	if usage == "" {
		usage = ":" + cmd.Name
	}
	a.setErr(text.StatusUsage, usage)
	return a, nil
}

// --- pane stack --------------------------------------------------------------

// pushPane adds a pane to the stack and returns its init command.
func (a *App) pushPane(p pane.Pane) (tea.Model, tea.Cmd) {
	a.panes = append(a.panes, p)
	return a, tea.Batch(p.Init(), a.syncPaneProject(p))
}

// openDetail pushes a detail pane for the focused pane's selected patent. The
// active project, when set, scopes the detail's review state and tags.
func (a *App) openDetail() (tea.Model, tea.Cmd) {
	number, ok := a.focusedPane().Selection()
	if !ok {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	var project domain.ProjectID
	if a.activeProject != nil {
		project = a.activeProject.ID
	}
	return a.pushPane(pane.NewDetail(a.client, a.theme, number, project))
}

// openCitations pushes a family-edge pane for the focused pane's selected
// patent, showing edges of the given kind.
func (a *App) openCitations(kind domain.RelationKind) (tea.Model, tea.Cmd) {
	number, ok := a.focusedPane().Selection()
	if !ok {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	return a.pushPane(pane.NewCitations(a.client, a.theme, number, kind))
}

func (a *App) activateProject() (tea.Model, tea.Cmd) {
	selector, ok := a.focusedPane().(interface{ SelectedProject() (domain.Project, bool) })
	if !ok {
		a.setErr(text.StatusNoProjectSelection)
		return a, nil
	}
	project, ok := selector.SelectedProject()
	if !ok {
		a.setErr(text.StatusNoProjectSelected)
		return a, nil
	}
	return a.useProject(project)
}

func (a *App) activateProjectByArg(arg string) (tea.Model, tea.Cmd) {
	project, ok := a.resolveProjectArg(arg)
	if !ok {
		a.setErr(text.StatusProjectNotFound, arg)
		return a, nil
	}
	return a.useProject(project)
}

// useProject makes project the active project and updates every pane.
func (a *App) useProject(project domain.Project) (tea.Model, tea.Cmd) {
	a.activeProject = &project
	a.lastProjectID = project.ID
	if a.saveLastProject != nil {
		if err := a.saveLastProject(project.ID); err != nil {
			a.setErr(text.StatusActiveProjectSaveErr, project.Name, err.Error())
		} else {
			a.setStatus(text.StatusActiveProject, project.Name)
		}
	} else {
		a.setStatus(text.StatusActiveProject, project.Name)
	}
	if splash, ok := a.focusedPane().(interface{ IsSplash() bool }); ok && splash.IsSplash() && len(a.panes) == 1 {
		catalog := pane.NewCatalog(a.client, a.theme)
		a.panes[0] = catalog
		return a, tea.Batch(catalog.Init(), a.broadcast(pane.ProjectChangedMsg{Project: &project}))
	}
	if len(a.panes) > 1 {
		a.panes = a.panes[:len(a.panes)-1]
	}
	return a, a.broadcast(pane.ProjectChangedMsg{Project: &project})
}

func (a *App) resolveProjectArg(arg string) (domain.Project, bool) {
	if selector, ok := a.focusedPane().(*pane.Projects); ok {
		if project, found := selector.ProjectByArg(arg); found {
			return project, true
		}
	}
	if a.client == nil {
		return domain.Project{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	var res proto.ProjectListResult
	if err := a.client.Call(ctx, proto.MethodProjectList, nil, &res); err != nil {
		return domain.Project{}, false
	}
	needle := strings.TrimSpace(strings.ToLower(arg))
	for _, project := range res.Projects {
		if strings.ToLower(string(project.ID)) == needle || strings.ToLower(project.Name) == needle {
			return project, true
		}
	}
	return domain.Project{}, false
}

func (a *App) runReviewState(target domain.ReviewState) (tea.Model, tea.Cmd) {
	return a.runProjectAction(func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
		return pane.SetReviewStateCmd(a.client, project, patent, target)
	})
}

func (a *App) focusedSelections() []domain.PatentNumber {
	if ms, ok := a.focusedPane().(pane.MultiSelector); ok {
		if sels := ms.Selections(); len(sels) >= 2 {
			return sels
		}
	}
	if number, ok := a.focusedPane().Selection(); ok {
		return []domain.PatentNumber{number}
	}
	return nil
}

func (a *App) runProjectAction(action func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.setErr(text.StatusNoActiveProject)
		return a, nil
	}
	if a.client == nil {
		a.setErr(text.StatusDaemonUnavailable)
		return a, nil
	}
	numbers := a.focusedSelections()
	if len(numbers) == 0 {
		a.setErr(text.StatusNoPatentSelected)
		return a, nil
	}
	cmds := make([]tea.Cmd, 0, len(numbers))
	for _, n := range numbers {
		cmds = append(cmds, action(a.activeProject.ID, n))
	}
	return a, tea.Batch(cmds...)
}

func (a *App) syncPaneProject(_ pane.Pane) tea.Cmd {
	return a.broadcast(pane.ProjectChangedMsg{Project: a.activeProject})
}

func (a *App) openPrompt(mode overlay.PromptMode) (tea.Model, tea.Cmd) {
	a.overlays = append(a.overlays, overlay.NewPrompt(a.registry, a.keymaps, a.theme, a.text, a.commandContext(), mode))
	return a, nil
}

func (a *App) commandContext() command.Context {
	if len(a.overlays) > 0 {
		if source, ok := a.focusedOverlay().(overlay.ContextSource); ok {
			return source.SourceContext()
		}
		return command.ContextOverlay
	}
	return a.focusedPane().Context()
}

// executeTypedCommand parses a typed command and routes it through invoke, the
// same path a key chord takes — so a command can never work one way and not the
// other.
func (a *App) executeTypedCommand(input string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(strings.TrimSpace(input))
	if len(parts) == 0 {
		return a, nil
	}
	cmd, ok := a.registry.LookupName(parts[0])
	if !ok {
		a.setErr(text.StatusUnknownCommand, parts[0])
		return a, nil
	}
	if !cmd.AvailableIn(a.commandContext()) {
		a.setErr(text.StatusCommandNotHere, cmd.Name)
		return a, nil
	}
	args := parts[1:]
	if len(args) > 0 && !typedAcceptsArgs[cmd.ID] {
		return a.usageError(cmd.ID)
	}
	return a.invoke(cmd.ID, invocation{repeat: 1, args: args})
}

// handleEvent reflects a daemon event into the status line and refreshes data.
func (a *App) handleEvent(ev proto.Event) tea.Cmd {
	switch ev.Method {
	case proto.EventIngestProgress:
		var p proto.IngestProgress
		if json.Unmarshal(ev.Params, &p) == nil {
			a.setStatus(text.StatusIngestProgress, p.JobID, p.Fetched, p.Found, p.Message)
		}
		return nil
	case proto.EventIngestDone:
		var d proto.IngestDone
		_ = json.Unmarshal(ev.Params, &d)
		if d.Error != "" {
			a.setErr(text.StatusIngestFailed, d.JobID, d.Error)
		} else {
			a.setStatus(text.StatusIngestComplete, d.JobID)
		}
		return a.refreshPanes()
	case proto.EventDBChanged:
		return a.refreshPanes()
	}
	return nil
}

// refreshPanes asks every pane to reload from the daemon. Panes cache their own
// view state, so a store mutation must invalidate the whole stack rather than
// only the currently visible pane.
func (a *App) refreshPanes() tea.Cmd {
	var cmds []tea.Cmd
	for i, p := range a.panes {
		updated, cmd := p.Command(command.Refresh, 1)
		a.panes[i] = updated
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

func (a *App) focusedPane() pane.Pane          { return a.panes[len(a.panes)-1] }
func (a *App) focusedOverlay() overlay.Overlay { return a.overlays[len(a.overlays)-1] }

func (a *App) popOverlay() {
	if n := len(a.overlays); n > 0 {
		a.overlays = a.overlays[:n-1]
	}
}

// setStatus and setErr resolve a catalog key into the status line.
func (a *App) setStatus(key text.Key, args ...any) {
	a.status, a.statusErr = a.text.Tf(key, args...), false
}

func (a *App) setErr(key text.Key, args ...any) {
	a.status, a.statusErr = a.text.Tf(key, args...), true
}

// View implements tea.Model.
func (a *App) View() string {
	if a.width == 0 || a.height == 0 {
		return "starting…"
	}
	focused := a.focusedPane()
	header := a.headerBlock(focused)
	headerLines := 0
	if header != "" {
		headerLines = strings.Count(header, "\n") + 1
	}
	bodyHeight := max(a.height-headerLines-statusRows, 1)

	body := fitBody(focused.View(a.width, bodyHeight), bodyHeight)

	statusStyle := a.theme.Status
	if a.statusErr {
		statusStyle = a.theme.Error
	}
	status := statusStyle.Render(render.Pad(" "+a.statusText(), a.width))

	screen := body + "\n" + status
	if header != "" {
		screen = header + "\n" + screen
	}
	if len(a.overlays) > 0 {
		screen = a.compositeOverlay(screen)
	}
	return screen
}

func (a *App) headerBlock(focused pane.Pane) string {
	if splash, ok := focused.(interface{ IsSplash() bool }); ok && splash.IsSplash() {
		return ""
	}
	line1 := a.renderScreenHeader(focused)
	line2 := a.theme.Dim.Render(render.Pad(" "+a.helperLine(focused.Context()), a.width))
	line3 := a.theme.Header.Render(strings.Repeat("─", a.width))
	return line1 + "\n" + line2 + "\n" + line3
}

func (a *App) renderScreenHeader(focused pane.Pane) string {
	var b strings.Builder
	b.WriteString(a.theme.Title.Render("PatentMine"))
	if a.activeProject != nil {
		b.WriteString(" ")
		project := a.activeProject.Name + " [" + string(a.activeProject.ID) + "]"
		b.WriteString(a.theme.Row.Render(project))
	}
	b.WriteString(" ")
	b.WriteString(a.theme.Header.Render("· "))
	b.WriteString(a.theme.Row.Bold(true).Render(focused.Title()))
	return render.Pad(" "+b.String(), a.width)
}

func (a *App) helperLine(ctx command.Context) string {
	switch ctx {
	case command.ContextCatalog:
		return a.joinHints(
			a.shortcutHint(ctx, command.OpenSearch, text.HintCommands),
			a.shortcutHint(ctx, command.OpenCommand, text.HintCommand),
			a.shortcutHint(ctx, command.OpenDetail, text.HintDetail),
			a.shortcutHint(ctx, command.OpenCitations, text.HintCitations),
			a.shortcutHint(ctx, command.OpenCitedBy, text.HintCitedBy),
			a.shortcutHint(ctx, command.OpenProjects, text.HintProjects),
			a.multiShortcutHint(ctx, []command.ID{command.AddToProject, command.MarkStored, command.MarkUnderReview, command.MarkIgnored, command.MarkDeleted}, text.HintProjectActions),
			a.shortcutHint(ctx, command.Back, text.HintBack),
		)
	case command.ContextDetail:
		return a.joinHints(
			a.shortcutHint(ctx, command.OpenSearch, text.HintCommands),
			a.shortcutHint(ctx, command.OpenCommand, text.HintCommand),
			a.shortcutHint(ctx, command.JumpMode, text.HintJump),
			a.shortcutHint(ctx, command.OpenCitations, text.HintCitations),
			a.shortcutHint(ctx, command.OpenCitedBy, text.HintCitedBy),
			a.shortcutHint(ctx, command.OpenProjects, text.HintProjects),
			a.multiShortcutHint(ctx, []command.ID{command.AddToProject, command.MarkStored, command.MarkUnderReview, command.MarkIgnored, command.MarkDeleted}, text.HintProjectActions),
			a.shortcutHint(ctx, command.Back, text.HintBack),
		)
	case command.ContextCitations:
		return a.joinHints(
			a.shortcutHint(ctx, command.OpenSearch, text.HintCommands),
			a.shortcutHint(ctx, command.OpenCommand, text.HintCommand),
			a.shortcutHint(ctx, command.OpenDetail, text.HintDetail),
			a.shortcutHint(ctx, command.OpenProjects, text.HintProjects),
			a.shortcutHint(ctx, command.IngestFamily, text.HintIngest),
			a.multiShortcutHint(ctx, []command.ID{command.AddToProject, command.MarkStored, command.MarkUnderReview, command.MarkIgnored, command.MarkDeleted}, text.HintProjectActions),
			a.shortcutHint(ctx, command.Back, text.HintBack),
		)
	case command.ContextProjects:
		return a.joinHints(
			a.shortcutHint(ctx, command.OpenSearch, text.HintCommands),
			a.shortcutHint(ctx, command.OpenCommand, text.HintCommand),
			a.shortcutHint(ctx, command.ProjectActivate, text.HintSelectProject),
			a.shortcutHint(ctx, command.ProjectClearActive, text.HintClearActive),
			a.shortcutHint(ctx, command.ProjectCreate, text.HintNewProject),
			a.shortcutHint(ctx, command.ExportIDS, text.HintExportIDS),
			a.shortcutHint(ctx, command.Back, text.HintBack),
		)
	default:
		return a.joinHints(
			a.shortcutHint(ctx, command.OpenSearch, text.HintCommands),
			a.shortcutHint(ctx, command.OpenCommand, text.HintCommand),
			a.shortcutHint(ctx, command.Help, text.HintHelp),
			a.shortcutHint(ctx, command.Quit, text.HintQuit),
		)
	}
}

func (a *App) splashFooterHint() string {
	ctx := command.ContextProjects
	return a.joinHints(
		a.navigationHint(ctx),
		a.shortcutHint(ctx, command.ProjectActivate, text.HintSelect),
		a.text.T(text.HintSlashCommands),
		a.shortcutHint(ctx, command.OpenCommand, text.HintCommand),
		a.shortcutHint(ctx, command.ProjectCreate, text.HintNewProject),
		a.shortcutHint(ctx, command.Quit, text.HintQuit),
	)
}

func (a *App) splashEmptyHint() string {
	ctx := command.ContextProjects
	createUsage := ":project.create"
	if cmd, ok := a.registry.Lookup(command.ProjectCreate); ok && cmd.Usage != "" {
		createUsage = cmd.Usage
	}
	shortcut := a.shortcutKeys(ctx, command.ProjectCreate)
	if shortcut == "" {
		return a.text.Tf(text.SplashCreateHint, createUsage)
	}
	return a.text.Tf(text.SplashCreateKeyHint, createUsage, shortcut)
}

func (a *App) navigationHint(ctx command.Context) string {
	down := a.shortcutKeys(ctx, command.NavDown)
	up := a.shortcutKeys(ctx, command.NavUp)
	move := a.text.T(text.HintMove)
	if down == "" && up == "" {
		return move
	}
	if down == "" {
		return up + " " + move
	}
	if up == "" {
		return down + " " + move
	}
	return down + "/" + up + " " + move
}

func (a *App) shortcutHint(ctx command.Context, id command.ID, labelKey text.Key) string {
	label := a.text.T(labelKey)
	keys := a.shortcutKeys(ctx, id)
	if keys == "" {
		return label
	}
	return keys + " " + label
}

func (a *App) multiShortcutHint(ctx command.Context, ids []command.ID, labelKey text.Key) string {
	label := a.text.T(labelKey)
	var parts []string
	for _, id := range ids {
		for _, seq := range a.keymaps.Shortcuts(ctx, id) {
			if !slices.Contains(parts, seq) {
				parts = append(parts, seq)
			}
		}
	}
	if len(parts) == 0 {
		return label
	}
	slices.Sort(parts)
	return strings.Join(parts, "/") + " " + label
}

func (a *App) shortcutKeys(ctx command.Context, id command.ID) string {
	shortcuts := a.keymaps.Shortcuts(ctx, id)
	if len(shortcuts) == 0 && ctx != command.ContextOverlay {
		shortcuts = a.keymaps.Shortcuts(command.ContextOverlay, id)
	}
	if len(shortcuts) == 0 {
		return ""
	}
	return strings.Join(shortcuts, "/")
}

func (a *App) joinHints(parts ...string) string {
	filtered := parts[:0]
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, " · ")
}

// statusText appends the chord reader's pending input, Vim-style.
func (a *App) statusText() string {
	versionText := "   [tui " + a.tuiVersion + " | daemon " + a.daemonVersion + "]"
	if a.activeProject != nil {
		versionText += "   [project " + a.activeProject.Name + "]"
	}
	visual := ""
	if ms, ok := a.focusedPane().(pane.MultiSelector); ok {
		if sels := ms.Selections(); len(sels) > 0 {
			visual = fmt.Sprintf("   [VISUAL %d]", len(sels))
		}
	}
	if pending := a.reader.Pending(); pending != "" {
		return a.status + versionText + visual + "   [" + pending + "]"
	}
	return a.status + versionText + visual
}

// compositeOverlay draws the focused overlay centred over the dimmed screen.
func (a *App) compositeOverlay(screen string) string {
	boxWidth := min(a.width-overlayMargin, overlayMaxWidth)
	boxHeight := min(a.height-overlayMargin, overlayMaxHeight)
	if boxWidth < 16 || boxHeight < 6 {
		return screen // terminal too small to frame an overlay
	}
	ov := a.focusedOverlay()
	innerWidth := boxWidth - overlayChrome
	content := a.theme.Title.Render(ov.Title()) + "\n\n" +
		ov.View(innerWidth, boxHeight-overlayChrome)
	box := a.theme.Box.Width(innerWidth).Height(boxHeight - 2).Render(content)

	dimmed := render.Dim(screen)
	x, y := render.CenterOffset(a.width, a.height, lipgloss.Width(box), lipgloss.Height(box))
	return render.Composite(dimmed, box, x, y)
}

// fitBody pads or trims rendered pane output to exactly height lines so the
// status line always sits on the bottom row.
func fitBody(body string, height int) string {
	lines := strings.Split(body, "\n")
	for len(lines) < height {
		lines = append(lines, "")
	}
	return strings.Join(lines[:height], "\n")
}
