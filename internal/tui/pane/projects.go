package pane

import (
	"log/slog"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

// projectsLoadedMsg delivers a finished project.list result.
type projectsLoadedMsg struct {
	requestID uint64
	projects  []domain.Project
	err       error
}

// Projects lists the projects and offers project-level actions.
type Projects struct {
	client        *rpc.Client
	theme         render.Theme
	handlers      map[command.ID]cmdHandler
	splash        bool
	lastProjectID domain.ProjectID
	splashFooter  string
	emptyHint     string

	activeProject *domain.Project
	projects      []domain.Project
	page          render.Paginator
	loading       bool
	loadErr       string
	loadID        uint64
	activeAI      string
	activeSearch  string
	activeBackup  string
	activeDaemon  string
	logger        *slog.Logger
}

// WithLogger attaches a logger so the pane can persist RPC errors.
func (p *Projects) WithLogger(l *slog.Logger) *Projects { p.logger = l; return p }

func (p *Projects) log() *slog.Logger {
	if p.logger != nil {
		return p.logger
	}
	return slog.Default()
}

// NewProjects builds an empty projects pane.
func NewProjects(client *rpc.Client, theme render.Theme, lastProjectID domain.ProjectID, activeAI, activeSearch, activeBackup, activeDaemon string) *Projects {
	p := &Projects{
		client:        client,
		theme:         theme,
		page:          render.NewPaginator(10),
		loading:       true,
		lastProjectID: lastProjectID,
		activeAI:      activeAI,
		activeSearch:  activeSearch,
		activeBackup:  activeBackup,
		activeDaemon:  activeDaemon,
	}
	p.handlers = map[command.ID]cmdHandler{
		command.NavDown:     func(inv Invocation) tea.Cmd { p.page.MoveDown(inv.Repeat); return nil },
		command.NavUp:       func(inv Invocation) tea.Cmd { p.page.MoveUp(inv.Repeat); return nil },
		command.NavPageDown: func(Invocation) tea.Cmd { p.page.PageDown(); return nil },
		command.NavPageUp:   func(Invocation) tea.Cmd { p.page.PageUp(); return nil },
		command.NavTop:      func(inv Invocation) tea.Cmd { p.page.NavTop(inv.Repeat); return nil },
		command.NavBottom:   func(inv Invocation) tea.Cmd { p.page.NavBottom(inv.Repeat); return nil },
		command.Refresh:     func(Invocation) tea.Cmd { p.loading = true; return p.load() },
		command.ExportIDS:   func(Invocation) tea.Cmd { return p.exportCmd() },
	}
	return p
}

// NewSplash builds the startup project-selection screen.
func NewSplash(client *rpc.Client, theme render.Theme, lastProjectID domain.ProjectID, splashFooter, emptyHint string, activeAI, activeSearch, activeBackup, activeDaemon string) *Projects {
	p := NewProjects(client, theme, lastProjectID, activeAI, activeSearch, activeBackup, activeDaemon)
	p.splash = true
	p.splashFooter = splashFooter
	p.emptyHint = emptyHint
	return p
}

// Context implements Pane.
func (p *Projects) Scope() command.Scope { return command.ScopeProjects }

// Title implements Pane.
func (p *Projects) Title() string {
	if p.splash {
		return "PatentMine"
	}
	return "Projects"
}

// Init implements Pane.
func (p *Projects) Init() tea.Cmd { return p.load() }

func (p *Projects) load() tea.Cmd {
	client := p.client
	requestID := nextAsyncID()
	p.loadID = requestID
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.ProjectListResult
		err := client.Call(ctx, proto.MethodProjectList, nil, &res)
		return projectsLoadedMsg{requestID: requestID, projects: res.Projects, err: err}
	}
}

// Command implements Pane.
func (p *Projects) Command(id command.ID, inv Invocation) (Pane, tea.Cmd) {
	if handler, ok := p.handlers[id]; ok {
		return p, handler(inv)
	}
	return p, nil
}

// Handles implements Pane.
func (p *Projects) Handles() []command.ID { return handlerIDs(p.handlers) }

// exportCmd builds the IDS for the selected project.
func (p *Projects) exportCmd() tea.Cmd {
	project, ok := p.selectedProject()
	if !ok {
		return status(text.StatusNoProjectSelected, true)
	}
	client := p.client
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.IDSResult
		if err := client.Call(ctx, proto.MethodIDSExport,
			proto.IDSExportParams{Project: project.ID}, &res); err != nil {
			return StatusMsg{Key: text.StatusExportFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusExportDone, Args: []any{project.Name, len(res.IDS.Entries)}}
	}
}

// Update implements Pane.
func (p *Projects) Update(msg tea.Msg) (Pane, tea.Cmd) {
	if m, ok := msg.(projectsLoadedMsg); ok {
		if m.requestID != p.loadID {
			return p, nil
		}
		p.loading = false
		if m.err != nil {
			p.loadErr = m.err.Error()
			p.log().Error("project list load failed", slog.String("error", m.err.Error()))
			return p, nil
		}
		p.loadErr = ""
		p.projects = m.projects
		p.page.SetTotal(len(p.projects))
		if p.lastProjectID != "" {
			for i, project := range p.projects {
				if project.ID == p.lastProjectID {
					p.page.GotoLine(i + 1)
					break
				}
			}
		}
		return p, nil
	}
	if m, ok := msg.(ProjectChangedMsg); ok {
		p.activeProject = cloneProject(m.Project)
	}
	if m, ok := msg.(ServiceStatusChangedMsg); ok {
		p.activeAI = m.ActiveAI
		p.activeSearch = m.ActiveSearch
		p.activeBackup = m.ActiveBackup
		p.activeDaemon = m.ActiveDaemon
	}
	return p, nil
}

// Selection implements Pane: the projects pane has no patent selection.
func (p *Projects) Selection() (domain.PatentNumber, bool) {
	return domain.PatentNumber{}, false
}

func (p *Projects) selectedProject() (domain.Project, bool) {
	cur := p.page.Cursor()
	if cur < 0 || cur >= len(p.projects) {
		return domain.Project{}, false
	}
	return p.projects[cur], true
}

// SelectedProject returns the highlighted project for app-level actions.
func (p *Projects) SelectedProject() (domain.Project, bool) {
	return p.selectedProject()
}

// ProjectByArg resolves a project by id or name from the currently loaded list.
func (p *Projects) ProjectByArg(arg string) (domain.Project, bool) {
	needle := strings.TrimSpace(strings.ToLower(arg))
	for _, project := range p.projects {
		if strings.ToLower(string(project.ID)) == needle || strings.ToLower(project.Name) == needle {
			return project, true
		}
	}
	return domain.Project{}, false
}

// View implements Pane.
func (p *Projects) View(w, h int) string {
	switch {
	case p.loading && len(p.projects) == 0:
		return p.theme.Dim.Render("loading projects…")
	case p.loadErr != "":
		return p.theme.Error.Render("error: " + p.loadErr)
	case len(p.projects) == 0:
		if p.splash {
			return p.centerSplash("No projects found. "+p.emptyHint, h, w)
		}
		return p.theme.Dim.Render("no projects yet — press n to create one")
	}
	p.page.SetPageSize(max(h-headerRows, 1))

	var b strings.Builder
	if p.splash {
		b.WriteString(p.splashHeader(w))
		b.WriteString("\n\n")
	}
	b.WriteString(renderTableStatusLine(p.theme, w, p.page.Cursor(), p.page.Total(), p.activeAI, p.activeDaemon, p.activeSearch, p.activeBackup))
	b.WriteByte('\n')
	b.WriteString(p.theme.Header.Render(projectRow("#", " ", " ", "NAME", "CACHED", "ID", p.createdLabel(), "HINT", w)))

	start, end := p.page.Window()
	for i := start; i < end; i++ {
		proj := p.projects[i]
		marker := "  "
		if i == p.page.Cursor() {
			marker = "→ "
		}
		hint := ""
		if proj.ID == p.lastProjectID {
			hint = "last used"
		}
		line := projectRow(
			formatViewIndex(i),
			marker,
			activeProjectMark(p.activeProject, proj),
			proj.Name,
			strconv.Itoa(proj.CachedCount),
			string(proj.ID),
			proj.CreatedAt.Format(domain.DateLayout),
			hint,
			w,
		)
		rowStyle := tableRowStyle(p.theme, i)
		b.WriteByte('\n')
		if i == p.page.Cursor() {
			b.WriteString(p.theme.Selected.Render(render.Pad(line, w)))
		} else {
			b.WriteString(rowStyle(render.Pad(line, w)))
		}
	}
	if p.splash {
		b.WriteString("\n\n")
		b.WriteString(p.theme.Header.Render(strings.Repeat("─", max(w-2, 1))))
		b.WriteString("\n")
		b.WriteString(p.theme.Dim.Render(p.splashFooter))
		return p.centerSplash(b.String(), h, w)
	}
	return b.String()
}

func (p *Projects) IsSplash() bool { return p.splash }

func (p *Projects) splashHeader(w int) string {
	lines := []string{
		"██████╗  █████╗ ████████╗███████╗███╗   ██╗████████╗ ███╗   ███╗██╗███╗   ██╗███████╗",
		"██╔══██╗██╔══██╗╚══██╔══╝██╔════╝████╗  ██║╚══██╔══╝ ████╗ ████║██║████╗  ██║██╔════╝",
		"██████╔╝███████║   ██║   █████╗  ██╔██╗ ██║   ██║    ██╔████╔██║██║██╔██╗ ██║█████╗  ",
		"██╔═══╝ ██╔══██║   ██║   ██╔══╝  ██║╚██╗██║   ██║    ██║╚██╔╝██║██║██║╚██╗██║██╔══╝  ",
		"██║     ██║  ██║   ██║   ███████╗██║ ╚████║   ██║    ██║ ╚═╝ ██║██║██║ ╚████║███████╗",
		"╚═╝     ╚═╝  ╚═╝   ╚═╝   ╚══════╝╚═╝  ╚═══╝   ╚═╝    ╚═╝     ╚═╝╚═╝╚═╝  ╚═══╝╚══════╝",
	}
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(p.theme.Title.Render(render.Truncate(line, w)))
		b.WriteByte('\n')
	}
	b.WriteString(p.theme.Dim.Render("Local Patent Research & Intelligence"))
	b.WriteByte('\n')
	b.WriteString(p.theme.Dim.Render("Select a project to enter the catalog"))
	b.WriteByte('\n')
	b.WriteString(p.theme.Header.Render(strings.Repeat("─", max(w-2, 1))))
	b.WriteByte('\n')
	b.WriteString(p.theme.Title.Render("SELECT PROJECT"))
	return strings.TrimRight(b.String(), "\n")
}

func (p *Projects) centerSplash(body string, height, width int) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines[i] = lipgloss.PlaceHorizontal(width, lipgloss.Center, line)
	}
	body = strings.Join(lines, "\n")
	used := len(lines)
	if pad := max((height-used)/2, 0); pad > 0 {
		body = strings.Repeat("\n", pad) + body
	}
	return body
}

func (p *Projects) createdLabel() string {
	if p.splash {
		return "UPDATED"
	}
	return "CREATED"
}

func projectRow(index, marker, active, name, cached, id, date, hint string, w int) string {
	const indexW = 4
	const markerW = 2
	const activeW = 2
	const nameW = 24
	const cachedW = 8
	const idW = 16
	const dateW = 12
	hintW := max(w-indexW-markerW-activeW-nameW-cachedW-idW-dateW-7, 0)
	displayID := id
	if id != "" && id != "ID" {
		displayID = "[" + id + "]"
	}
	return render.Pad(index, indexW) + " " +
		render.Pad(marker, markerW) + " " +
		render.Pad(active, activeW) + " " +
		render.Pad(render.Truncate(name, nameW), nameW) + " " +
		render.Pad(cached, cachedW) + " " +
		render.Pad(render.Truncate(displayID, idW), idW) + " " +
		render.Pad(date, dateW) + " " +
		render.Pad(render.Truncate(hint, hintW), hintW)
}

func activeProjectMark(active *domain.Project, candidate domain.Project) string {
	if active != nil && active.ID == candidate.ID {
		return "*"
	}
	return ""
}
