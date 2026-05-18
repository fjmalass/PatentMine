// Package tui is the terminal client. The App is the bubbletea root: it owns
// the pane and overlay stacks and routes input, but holds no business state —
// every screen's state lives in its own Pane or Overlay. That decomposition is
// the deliberate structural defence against a god-object UI model.
package tui

import (
	"encoding/json"
	"fmt"
	"strings"

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
)

// reservedRows is the header + status lines the App draws around a pane body.
const reservedRows = 2

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

// App is the bubbletea root model.
type App struct {
	client   *rpc.Client
	registry *command.Registry
	keymaps  *keymap.Keymaps
	theme    render.Theme
	reader   keys.Reader

	panes    []pane.Pane
	overlays []overlay.Overlay

	status    string
	statusErr bool
	width     int
	height    int
}

// New builds the App with the catalog as the initial pane.
func New(client *rpc.Client, registry *command.Registry, keymaps *keymap.Keymaps) *App {
	theme := render.NewTheme()
	return &App{
		client:   client,
		registry: registry,
		keymaps:  keymaps,
		theme:    theme,
		panes:    []pane.Pane{pane.NewCatalog(client, theme)},
		status:   "ready — press ? for help",
	}
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	return tea.Batch(a.panes[0].Init(), a.listen())
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

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		return a, a.broadcast(pane.ResizeMsg{Width: m.Width, Height: max(m.Height-reservedRows, 1)})
	case tea.KeyMsg:
		return a.handleKey(m)
	case pane.StatusMsg:
		a.status, a.statusErr = m.Text, m.Error
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
	return a, p.Init()
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
		return a.refreshFocused()
	case proto.EventDBChanged:
		return a.refreshFocused()
	}
	return nil
}

// refreshFocused asks the focused pane to reload from the daemon.
func (a *App) refreshFocused() tea.Cmd {
	updated, cmd := a.focusedPane().Command(command.Refresh, 1)
	a.panes[len(a.panes)-1] = updated
	return cmd
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
	bodyHeight := max(a.height-reservedRows, 1)
	focused := a.focusedPane()

	header := a.theme.Title.Render(render.Pad(" "+focused.Title(), a.width))
	body := fitBody(focused.View(a.width, bodyHeight), bodyHeight)

	statusStyle := a.theme.Status
	if a.statusErr {
		statusStyle = a.theme.Error
	}
	status := statusStyle.Render(render.Pad(" "+a.statusText(), a.width))

	screen := header + "\n" + body + "\n" + status
	if len(a.overlays) > 0 {
		screen = a.compositeOverlay(screen)
	}
	return screen
}

// statusText appends the chord reader's pending input, Vim-style.
func (a *App) statusText() string {
	if pending := a.reader.Pending(); pending != "" {
		return a.status + "   [" + pending + "]"
	}
	return a.status
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
