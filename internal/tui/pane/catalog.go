package pane

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

// column widths for a catalog row.
const (
	colNumber = 16
	colState  = 13
)

// catalogLoadedMsg delivers a finished patent.list result.
type catalogLoadedMsg struct {
	requestID uint64
	offset    int
	total     int
	patents   []domain.PatentRow
	err       error
}

// Catalog is the main patent list pane.
type Catalog struct {
	client   *rpc.Client
	theme    render.Theme
	handlers map[command.ID]cmdHandler

	activeProject *domain.Project

	patents    []domain.PatentRow
	page       render.Paginator
	loadedBase int
	loading    bool
	loadErr    string
	loadID     uint64
}

// NewCatalog builds an empty catalog pane bound to a daemon client.
func NewCatalog(client *rpc.Client, theme render.Theme) *Catalog {
	c := &Catalog{
		client:  client,
		theme:   theme,
		page:    render.NewPaginator(10),
		loading: true,
	}
	c.handlers = map[command.ID]cmdHandler{
		command.NavDown:     func(r int) tea.Cmd { return c.move(func() { c.page.MoveDown(r) }) },
		command.NavUp:       func(r int) tea.Cmd { return c.move(func() { c.page.MoveUp(r) }) },
		command.NavPageDown: func(int) tea.Cmd { return c.move(c.page.PageDown) },
		command.NavPageUp:   func(int) tea.Cmd { return c.move(c.page.PageUp) },
		command.NavTop:      func(int) tea.Cmd { return c.move(c.page.Top) },
		command.NavBottom:   func(int) tea.Cmd { return c.move(c.page.Bottom) },
		command.Refresh:     func(int) tea.Cmd { c.loading = true; return c.load() },
		command.IngestFamily: func(int) tea.Cmd {
			return c.ingestSelected(ingestFamilyDepth)
		},
		command.FetchPatent: func(int) tea.Cmd {
			return c.ingestSelected(ingestPatentDepth)
		},
	}
	return c
}

// Context implements Pane.
func (c *Catalog) Context() command.Context { return command.ContextCatalog }

// Title implements Pane.
func (c *Catalog) Title() string { return "Patents" }

// Init implements Pane.
func (c *Catalog) Init() tea.Cmd { return c.load() }

// load fetches the patent list from the daemon.
func (c *Catalog) load() tea.Cmd {
	client := c.client
	requestID := nextAsyncID()
	c.loadID = requestID
	offset := c.page.Offset()
	limit := c.page.PageSize()
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.PatentListResult
		var project string
		if c.activeProject != nil {
			project = string(c.activeProject.ID)
		}
		err := client.Call(ctx, proto.MethodPatentList,
			proto.PatentListParams{Project: project, Limit: limit, Offset: offset}, &res)
		return catalogLoadedMsg{
			requestID: requestID,
			offset:    offset,
			total:     res.Total,
			patents:   res.Patents,
			err:       err,
		}
	}
}

// Command implements Pane.
func (c *Catalog) Command(id command.ID, repeat int) (Pane, tea.Cmd) {
	if handler, ok := c.handlers[id]; ok {
		return c, handler(repeat)
	}
	return c, nil
}

// Handles implements Pane.
func (c *Catalog) Handles() []command.ID { return handlerIDs(c.handlers) }

// ingestSelected enqueues an ingest for the highlighted patent.
func (c *Catalog) ingestSelected(depth int) tea.Cmd {
	number, ok := c.Selection()
	if !ok {
		return status(text.StatusNoPatentSelected, true)
	}
	return IngestCmd(c.client, number, depth, false)
}

// move runs a cursor motion and reloads the page when the visible window
// scrolled to a new offset.
func (c *Catalog) move(motion func()) tea.Cmd {
	before := c.page.Offset()
	motion()
	if c.page.Offset() != before {
		c.loading = true
		return c.load()
	}
	return nil
}

// Update implements Pane.
func (c *Catalog) Update(msg tea.Msg) (Pane, tea.Cmd) {
	switch m := msg.(type) {
	case ResizeMsg:
		pageSize := max(m.Height-1, 1)
		if pageSize != c.page.PageSize() {
			before := c.page.Offset()
			c.page.SetPageSize(pageSize)
			if before != c.page.Offset() || len(c.patents) != c.page.PageSize() {
				c.loading = true
				return c, c.load()
			}
		}
	case ProjectChangedMsg:
		changed := !sameProject(c.activeProject, m.Project)
		c.activeProject = cloneProject(m.Project)
		if changed {
			c.page.Top()
			c.loadedBase = 0
			c.loading = true
			return c, c.load()
		}
	case catalogLoadedMsg:
		if m.requestID != c.loadID {
			return c, nil
		}
		c.loading = false
		if m.err != nil {
			c.loadErr = m.err.Error()
			return c, nil
		}
		c.loadErr = ""
		c.patents = m.patents
		c.loadedBase = m.offset
		c.page.SetTotal(m.total)
		if c.page.Offset() != m.offset {
			c.loading = true
			return c, c.load()
		}
	}
	return c, nil
}

// Selection implements Pane.
func (c *Catalog) Selection() (domain.PatentNumber, bool) {
	cur := c.page.Cursor() - c.loadedBase
	if cur < 0 || cur >= len(c.patents) {
		return domain.PatentNumber{}, false
	}
	return c.patents[cur].Number, true
}

// View implements Pane. It sets the page size from the available height so the
// visible window always fills the body.
func (c *Catalog) View(w, h int) string {
	switch {
	case c.loading:
		return c.theme.Dim.Render("loading patents…")
	case c.loadErr != "":
		return c.theme.Error.Render("error: " + c.loadErr)
	case c.page.Total() == 0:
		if c.activeProject != nil {
			return c.theme.Dim.Render("no patents in active project " + c.activeProject.Name)
		}
		return c.theme.Dim.Render("no patents yet — select a number and press f to ingest its family")
	}

	var b strings.Builder
	b.WriteString(c.theme.Header.Render(catalogRow("NUMBER", c.stateHeading(), "TITLE", w)))

	for i, p := range c.patents {
		absolute := c.loadedBase + i
		line := catalogRow(numberToShowRow(p).String(), c.stateText(p), p.Title, w)
		b.WriteByte('\n')
		if absolute == c.page.Cursor() {
			b.WriteString(c.theme.Selected.Render(render.Pad(line, w)))
		} else {
			b.WriteString(c.theme.Row.Render(line))
		}
	}
	return b.String()
}

func (c *Catalog) stateHeading() string {
	if c.activeProject != nil {
		return "PROJECT STATE"
	}
	return "FETCH"
}

func (c *Catalog) stateText(row domain.PatentRow) string {
	if c.activeProject != nil && row.MembershipState.Valid() {
		return string(row.MembershipState)
	}
	return string(row.FetchState)
}

func cloneProject(project *domain.Project) *domain.Project {
	if project == nil {
		return nil
	}
	copy := *project
	return &copy
}

func sameProject(a, b *domain.Project) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ID == b.ID && a.Name == b.Name && a.CreatedAt.Equal(b.CreatedAt)
}

// catalogRow formats one fixed-width catalog line.
func catalogRow(number, state, title string, w int) string {
	titleW := max(w-colNumber-colState-2, 0)
	return render.Pad(number, colNumber) + " " +
		render.Pad(state, colState) + " " +
		render.Truncate(title, titleW)
}

func numberToShowRow(row domain.PatentRow) domain.PatentNumber {
	if !row.DisplayNumber.IsZero() {
		return row.DisplayNumber
	}
	return row.Number
}
