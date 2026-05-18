package pane

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/tui/render"
)

// projectsLoadedMsg delivers a finished project.list result.
type projectsLoadedMsg struct {
	projects []domain.Project
	err      error
}

// Projects lists the projects and offers project-level actions.
type Projects struct {
	client *rpc.Client
	theme  render.Theme

	projects []domain.Project
	page     render.Paginator
	loading  bool
	loadErr  string
}

// NewProjects builds an empty projects pane.
func NewProjects(client *rpc.Client, theme render.Theme) *Projects {
	return &Projects{
		client:  client,
		theme:   theme,
		page:    render.NewPaginator(10),
		loading: true,
	}
}

// Context implements Pane.
func (p *Projects) Context() command.Context { return command.ContextProjects }

// Title implements Pane.
func (p *Projects) Title() string { return "Projects" }

// Init implements Pane.
func (p *Projects) Init() tea.Cmd { return p.load() }

func (p *Projects) load() tea.Cmd {
	client := p.client
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.ProjectListResult
		err := client.Call(ctx, proto.MethodProjectList, nil, &res)
		return projectsLoadedMsg{projects: res.Projects, err: err}
	}
}

// Command implements Pane.
func (p *Projects) Command(id command.ID, repeat int) (Pane, tea.Cmd) {
	switch id {
	case command.NavDown:
		p.page.MoveDown(repeat)
	case command.NavUp:
		p.page.MoveUp(repeat)
	case command.NavPageDown:
		p.page.PageDown()
	case command.NavPageUp:
		p.page.PageUp()
	case command.NavTop:
		p.page.Top()
	case command.NavBottom:
		p.page.Bottom()
	case command.Refresh:
		p.loading = true
		return p, p.load()
	case command.ProjectCreate:
		return p, p.createCmd()
	case command.ExportIDS:
		return p, p.exportCmd()
	case command.OpenSearch:
		return p, status("search prompt is not yet wired", false)
	}
	return p, nil
}

// createCmd creates a project with a generated name. A name-entry overlay is
// future work; for now the project can be renamed once that lands.
func (p *Projects) createCmd() tea.Cmd {
	client := p.client
	name := fmt.Sprintf("Project %d", len(p.projects)+1)
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.ProjectResult
		if err := client.Call(ctx, proto.MethodProjectCreate,
			proto.ProjectCreateParams{Name: name}, &res); err != nil {
			return StatusMsg{Text: "create failed: " + err.Error(), Error: true}
		}
		return StatusMsg{Text: "created " + res.Project.Name}
	}
}

// exportCmd builds the IDS for the selected project.
func (p *Projects) exportCmd() tea.Cmd {
	project, ok := p.selectedProject()
	if !ok {
		return status("no project selected", true)
	}
	client := p.client
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.IDSResult
		if err := client.Call(ctx, proto.MethodIDSExport,
			proto.IDSExportParams{Project: project.ID}, &res); err != nil {
			return StatusMsg{Text: "IDS export failed: " + err.Error(), Error: true}
		}
		return StatusMsg{Text: fmt.Sprintf("IDS for %q: %d disclosed reference(s)",
			project.Name, len(res.IDS.Entries))}
	}
}

// Update implements Pane.
func (p *Projects) Update(msg tea.Msg) (Pane, tea.Cmd) {
	if m, ok := msg.(projectsLoadedMsg); ok {
		p.loading = false
		if m.err != nil {
			p.loadErr = m.err.Error()
			return p, nil
		}
		p.loadErr = ""
		p.projects = m.projects
		p.page.SetTotal(len(p.projects))
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

// View implements Pane.
func (p *Projects) View(w, h int) string {
	switch {
	case p.loading:
		return p.theme.Dim.Render("loading projects…")
	case p.loadErr != "":
		return p.theme.Error.Render("error: " + p.loadErr)
	case len(p.projects) == 0:
		return p.theme.Dim.Render("no projects yet — press n to create one")
	}
	p.page.SetPageSize(max(h-1, 1))

	var b strings.Builder
	b.WriteString(p.theme.Header.Render(projectRow("NAME", "CREATED", w)))
	start, end := p.page.Window()
	for i := start; i < end; i++ {
		proj := p.projects[i]
		line := projectRow(proj.Name, proj.CreatedAt.Format("2006-01-02"), w)
		b.WriteByte('\n')
		if i == p.page.Cursor() {
			b.WriteString(p.theme.Selected.Render(render.Pad(line, w)))
		} else {
			b.WriteString(p.theme.Row.Render(line))
		}
	}
	return b.String()
}

// projectRow formats one project line.
func projectRow(name, created string, w int) string {
	const createdW = 12
	return render.Pad(render.Truncate(name, max(w-createdW-1, 0)), max(w-createdW-1, 0)) +
		" " + render.Pad(created, createdW)
}
