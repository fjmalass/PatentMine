// Package tui is the terminal client. The App is the bubbletea root: it owns
// the pane and overlay stacks and routes input, but holds no business state —
// every screen's state lives in its own Pane or Overlay. That decomposition is
// the deliberate structural defence against a god-object UI model.
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

// App is the bubbletea root model.
type App struct {
	client          *rpc.Client
	registry        *command.Registry
	keymaps         *keymap.Keymaps
	theme           render.Theme
	reader          keys.Reader
	saveLastProject func(domain.ProjectID) error

	panes    []pane.Pane
	overlays []overlay.Overlay

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

// New builds the App with the splash/project selector as the initial pane.
func New(client *rpc.Client, registry *command.Registry, keymaps *keymap.Keymaps, opts ...Option) *App {
	theme := render.NewTheme()
	app := &App{
		client:        client,
		registry:      registry,
		keymaps:       keymaps,
		theme:         theme,
		status:        "select a project to begin — press ? for help",
		tuiVersion:    appversion.String(),
		daemonVersion: "connecting",
	}
	for _, opt := range opts {
		opt(app)
	}
	app.panes = []pane.Pane{pane.NewSplash(client, theme, app.lastProjectID,
		app.splashFooterHint(), app.splashEmptyHint())}
	return app
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
	case overlay.PromptSubmitMsg:
		a.popOverlay()
		return a.executeTypedCommand(m.Input)
	case overlay.PromptCloseMsg:
		a.popOverlay()
		return a, nil
	case pane.StatusMsg:
		a.status, a.statusErr = m.Text, m.Error
		return a, nil
	case pingLoadedMsg:
		if m.err != nil {
			a.daemonVersion = "unavailable"
			return a, nil
		}
		a.daemonVersion = m.version
		return a, nil
	case busEventMsg:
		return a, tea.Batch(a.handleEvent(m.event), a.listen())
	case eventsClosedMsg:
		a.status, a.statusErr = "daemon connection closed", true
		return a, nil
	default:
		// rpc results and the like — let every pane consume what is theirs.
		return a, a.broadcast(msg)
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
	return a.dispatch(id, chord.Repeat())
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

// dispatch carries out a resolved command. Stack-changing commands are handled
// here; everything else is forwarded to the focused pane or overlay.
func (a *App) dispatch(id command.ID, repeat int) (tea.Model, tea.Cmd) {
	switch id {
	case command.Quit:
		return a, tea.Quit
	case command.Help:
		if len(a.overlays) == 0 {
			a.overlays = append(a.overlays, overlay.NewHelp(a.registry, a.keymaps, a.theme))
		}
		return a, nil
	case command.OpenSearch:
		return a.openPrompt(overlay.PromptPalette)
	case command.OpenCommand:
		return a.openPrompt(overlay.PromptDirect)
	case command.CloseOverlay:
		a.popOverlay()
		return a, nil
	case command.Back:
		if len(a.overlays) > 0 {
			a.popOverlay()
		} else if len(a.panes) > 1 {
			a.panes = a.panes[:len(a.panes)-1]
		}
		return a, nil
	case command.OpenDetail:
		return a.openDetail()
	case command.OpenCitations:
		return a.openCitations(domain.RelationCites)
	case command.OpenCitedBy:
		return a.openCitations(domain.RelationCitedBy)
	case command.OpenProjects:
		return a.pushPane(pane.NewProjects(a.client, a.theme))
	case command.ProjectActivate:
		return a.activateProject()
	case command.ProjectClearActive:
		a.activeProject = nil
		a.status, a.statusErr = "cleared active project", false
		return a, a.broadcast(pane.ProjectChangedMsg{})
	case command.AddToProject:
		return a.runProjectAction(func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
			return pane.AddToProjectCmd(a.client, project, patent)
		})
	case command.MarkStored:
		return a.runMembershipState(domain.MembershipStored)
	case command.MarkUnderReview:
		return a.runMembershipState(domain.MembershipUnderReview)
	case command.MarkIgnored:
		return a.runMembershipState(domain.MembershipIgnored)
	case command.MarkDeleted:
		return a.runMembershipState(domain.MembershipDeleted)
	}

	if len(a.overlays) > 0 {
		updated, cmd := a.focusedOverlay().Command(id, repeat)
		a.overlays[len(a.overlays)-1] = updated
		return a, cmd
	}
	updated, cmd := a.focusedPane().Command(id, repeat)
	a.panes[len(a.panes)-1] = updated
	return a, cmd
}

// pushPane adds a pane to the stack and returns its init command.
func (a *App) pushPane(p pane.Pane) (tea.Model, tea.Cmd) {
	a.panes = append(a.panes, p)
	return a, tea.Batch(p.Init(), a.syncPaneProject(p))
}

// openDetail pushes a detail pane for the focused pane's selected patent.
func (a *App) openDetail() (tea.Model, tea.Cmd) {
	number, ok := a.focusedPane().Selection()
	if !ok {
		a.status, a.statusErr = "no patent selected", true
		return a, nil
	}
	return a.pushPane(pane.NewDetail(a.client, a.theme, number))
}

// openCitations pushes a family-edge pane for the focused pane's selected
// patent, showing edges of the given kind.
func (a *App) openCitations(kind domain.RelationKind) (tea.Model, tea.Cmd) {
	number, ok := a.focusedPane().Selection()
	if !ok {
		a.status, a.statusErr = "no patent selected", true
		return a, nil
	}
	return a.pushPane(pane.NewCitations(a.client, a.theme, number, kind))
}

func (a *App) activateProject() (tea.Model, tea.Cmd) {
	selector, ok := a.focusedPane().(interface{ SelectedProject() (domain.Project, bool) })
	if !ok {
		a.status, a.statusErr = "focused pane has no project selection", true
		return a, nil
	}
	project, ok := selector.SelectedProject()
	if !ok {
		a.status, a.statusErr = "no project selected", true
		return a, nil
	}
	a.activeProject = &project
	a.lastProjectID = project.ID
	if a.saveLastProject != nil {
		if err := a.saveLastProject(project.ID); err != nil {
			a.status, a.statusErr = "active project: "+project.Name+" (save failed: "+err.Error()+")", true
		} else {
			a.status, a.statusErr = "active project: "+project.Name, false
		}
	} else {
		a.status, a.statusErr = "active project: "+project.Name, false
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

func (a *App) activateProjectByArg(arg string) (tea.Model, tea.Cmd) {
	project, ok := a.resolveProjectArg(arg)
	if !ok {
		a.status, a.statusErr = "project not found: "+arg, true
		return a, nil
	}
	a.activeProject = &project
	a.lastProjectID = project.ID
	if a.saveLastProject != nil {
		if err := a.saveLastProject(project.ID); err != nil {
			a.status, a.statusErr = "active project: "+project.Name+" (save failed: "+err.Error()+")", true
		} else {
			a.status, a.statusErr = "active project: "+project.Name, false
		}
	} else {
		a.status, a.statusErr = "active project: "+project.Name, false
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

func (a *App) runMembershipState(target domain.MembershipState) (tea.Model, tea.Cmd) {
	return a.runProjectAction(func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
		return pane.SetMembershipStateCmd(a.client, project, patent, target)
	})
}

func (a *App) runProjectAction(action func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd) (tea.Model, tea.Cmd) {
	if a.activeProject == nil {
		a.status, a.statusErr = "select an active project first", true
		return a, nil
	}
	number, ok := a.focusedPane().Selection()
	if !ok {
		a.status, a.statusErr = "no patent selected", true
		return a, nil
	}
	if a.client == nil {
		a.status, a.statusErr = "daemon connection unavailable", true
		return a, nil
	}
	return a, action(a.activeProject.ID, number)
}

func (a *App) syncPaneProject(_ pane.Pane) tea.Cmd {
	return a.broadcast(pane.ProjectChangedMsg{Project: a.activeProject})
}

func (a *App) openPrompt(mode overlay.PromptMode) (tea.Model, tea.Cmd) {
	a.overlays = append(a.overlays, overlay.NewPrompt(a.registry, a.keymaps, a.theme, a.commandContext(), mode))
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

func (a *App) executeTypedCommand(input string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(strings.TrimSpace(input))
	if len(parts) == 0 {
		return a, nil
	}
	cmd, ok := a.registry.LookupName(parts[0])
	if !ok {
		a.status, a.statusErr = "unknown command: "+parts[0], true
		return a, nil
	}
	ctx := a.commandContext()
	if !cmd.AvailableIn(ctx) {
		a.status, a.statusErr = cmd.Name+" is not available here", true
		return a, nil
	}
	args := parts[1:]
	switch cmd.ID {
	case command.AddToProject:
		if len(args) == 0 {
			return a.runProjectAction(func(project domain.ProjectID, patent domain.PatentNumber) tea.Cmd {
				return pane.AddToProjectCmd(a.client, project, patent)
			})
		}
		if len(args) != 1 {
			return a.typedUsageError(cmd)
		}
		number, err := domain.ParsePatentNumber(args[0])
		if err != nil {
			a.status, a.statusErr = "invalid patent number: "+err.Error(), true
			return a, nil
		}
		if a.activeProject == nil {
			a.status, a.statusErr = "select an active project first", true
			return a, nil
		}
		if a.client == nil {
			a.status, a.statusErr = "daemon connection unavailable", true
			return a, nil
		}
		return a, pane.AddToProjectCmd(a.client, a.activeProject.ID, number)
	case command.ProjectActivate:
		if len(args) == 0 {
			return a.activateProject()
		}
		if len(args) != 1 {
			return a.typedUsageError(cmd)
		}
		return a.activateProjectByArg(args[0])
	case command.ProjectClearActive:
		if len(args) != 0 {
			return a.typedUsageError(cmd)
		}
		a.activeProject = nil
		a.status, a.statusErr = "cleared active project", false
		return a, a.broadcast(pane.ProjectChangedMsg{})
	case command.MarkStored, command.MarkUnderReview, command.MarkIgnored, command.MarkDeleted,
		command.OpenProjects, command.OpenDetail, command.OpenCitations, command.OpenCitedBy,
		command.Refresh, command.Help, command.Quit:
		if len(args) != 0 {
			return a.typedUsageError(cmd)
		}
		return a.dispatch(cmd.ID, 1)
	default:
		a.status, a.statusErr = "typed command not implemented: "+cmd.Name, true
		return a, nil
	}
}

func (a *App) typedUsageError(cmd command.Command) (tea.Model, tea.Cmd) {
	usage := cmd.Usage
	if usage == "" {
		usage = ":" + cmd.Name
	}
	a.status, a.statusErr = "usage: "+usage, true
	return a, nil
}

// handleEvent reflects a daemon event into the status line and refreshes data.
func (a *App) handleEvent(ev proto.Event) tea.Cmd {
	switch ev.Method {
	case proto.EventIngestProgress:
		var p proto.IngestProgress
		if json.Unmarshal(ev.Params, &p) == nil {
			a.status = fmt.Sprintf("ingest %s — fetched %d, found %d: %s",
				p.JobID, p.Fetched, p.Found, p.Message)
			a.statusErr = false
		}
		return nil
	case proto.EventIngestDone:
		var d proto.IngestDone
		_ = json.Unmarshal(ev.Params, &d)
		if d.Error != "" {
			a.status, a.statusErr = "ingest "+d.JobID+" failed: "+d.Error, true
		} else {
			a.status, a.statusErr = "ingest "+d.JobID+" complete", false
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
			a.shortcutHint(ctx, command.OpenSearch, "commands"),
			a.shortcutHint(ctx, command.OpenCommand, "command"),
			a.shortcutHint(ctx, command.OpenDetail, "detail"),
			a.shortcutHint(ctx, command.OpenCitations, "citations"),
			a.shortcutHint(ctx, command.OpenCitedBy, "cited by"),
			a.shortcutHint(ctx, command.OpenProjects, "projects"),
			a.multiShortcutHint(ctx, []command.ID{command.AddToProject, command.MarkStored, command.MarkUnderReview, command.MarkIgnored, command.MarkDeleted}, "project actions"),
			a.shortcutHint(ctx, command.Back, "back"),
		)
	case command.ContextDetail:
		return a.joinHints(
			a.shortcutHint(ctx, command.OpenSearch, "commands"),
			a.shortcutHint(ctx, command.OpenCommand, "command"),
			a.shortcutHint(ctx, command.OpenCitations, "citations"),
			a.shortcutHint(ctx, command.OpenCitedBy, "cited by"),
			a.shortcutHint(ctx, command.OpenProjects, "projects"),
			a.multiShortcutHint(ctx, []command.ID{command.AddToProject, command.MarkStored, command.MarkUnderReview, command.MarkIgnored, command.MarkDeleted}, "project actions"),
			a.shortcutHint(ctx, command.Back, "back"),
		)
	case command.ContextCitations:
		return a.joinHints(
			a.shortcutHint(ctx, command.OpenSearch, "commands"),
			a.shortcutHint(ctx, command.OpenCommand, "command"),
			a.shortcutHint(ctx, command.OpenDetail, "detail"),
			a.shortcutHint(ctx, command.OpenProjects, "projects"),
			a.shortcutHint(ctx, command.IngestFamily, "ingest"),
			a.multiShortcutHint(ctx, []command.ID{command.AddToProject, command.MarkStored, command.MarkUnderReview, command.MarkIgnored, command.MarkDeleted}, "project actions"),
			a.shortcutHint(ctx, command.Back, "back"),
		)
	case command.ContextProjects:
		return a.joinHints(
			a.shortcutHint(ctx, command.OpenSearch, "commands"),
			a.shortcutHint(ctx, command.OpenCommand, "command"),
			a.shortcutHint(ctx, command.ProjectActivate, "select project"),
			a.shortcutHint(ctx, command.ProjectClearActive, "clear active"),
			a.shortcutHint(ctx, command.ProjectCreate, "new project"),
			a.shortcutHint(ctx, command.ExportIDS, "export IDS"),
			a.shortcutHint(ctx, command.Back, "back"),
		)
	default:
		return a.joinHints(
			a.shortcutHint(ctx, command.OpenSearch, "commands"),
			a.shortcutHint(ctx, command.OpenCommand, "command"),
			a.shortcutHint(ctx, command.Help, "help"),
			a.shortcutHint(ctx, command.Quit, "quit"),
		)
	}
}

func (a *App) splashFooterHint() string {
	ctx := command.ContextProjects
	return a.joinHints(
		a.navigationHint(ctx),
		a.shortcutHint(ctx, command.ProjectActivate, "select"),
		"/ commands",
		a.shortcutHint(ctx, command.OpenCommand, "command"),
		a.shortcutHint(ctx, command.ProjectCreate, "new project"),
		a.shortcutHint(ctx, command.Quit, "quit"),
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
		return "Create one with " + createUsage + "."
	}
	return "Create one with " + createUsage + " or " + shortcut + "."
}

func (a *App) navigationHint(ctx command.Context) string {
	down := a.shortcutKeys(ctx, command.NavDown)
	up := a.shortcutKeys(ctx, command.NavUp)
	if down == "" && up == "" {
		return "move"
	}
	if down == "" {
		return up + " move"
	}
	if up == "" {
		return down + " move"
	}
	return down + "/" + up + " move"
}

func (a *App) shortcutHint(ctx command.Context, id command.ID, label string) string {
	keys := a.shortcutKeys(ctx, id)
	if keys == "" {
		return label
	}
	return keys + " " + label
}

func (a *App) multiShortcutHint(ctx command.Context, ids []command.ID, label string) string {
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
	if pending := a.reader.Pending(); pending != "" {
		return a.status + versionText + "   [" + pending + "]"
	}
	return a.status + versionText
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
