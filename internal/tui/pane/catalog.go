package pane

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/tui/render"
)

// catalogFetchLimit is how many patents the catalog pane pulls per load. The
// Paginator pages the display; the daemon also supports query-side paging when
// the catalog grows past this.
const catalogFetchLimit = 500

// column widths for a catalog row.
const (
	colNumber = 16
	colState  = 9
)

// catalogLoadedMsg delivers a finished patent.list result.
type catalogLoadedMsg struct {
	patents []domain.Patent
	err     error
}

// Catalog is the main patent list pane.
type Catalog struct {
	client *rpc.Client
	theme  render.Theme

	patents []domain.Patent
	page    render.Paginator
	loading bool
	loadErr string
}

// NewCatalog builds an empty catalog pane bound to a daemon client.
func NewCatalog(client *rpc.Client, theme render.Theme) *Catalog {
	return &Catalog{
		client:  client,
		theme:   theme,
		page:    render.NewPaginator(10),
		loading: true,
	}
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
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.PatentListResult
		err := client.Call(ctx, proto.MethodPatentList,
			proto.PatentListParams{Limit: catalogFetchLimit}, &res)
		return catalogLoadedMsg{patents: res.Patents, err: err}
	}
}

// Command implements Pane.
func (c *Catalog) Command(id command.ID, repeat int) (Pane, tea.Cmd) {
	switch id {
	case command.NavDown:
		c.page.MoveDown(repeat)
	case command.NavUp:
		c.page.MoveUp(repeat)
	case command.NavPageDown:
		c.page.PageDown()
	case command.NavPageUp:
		c.page.PageUp()
	case command.NavTop:
		c.page.Top()
	case command.NavBottom:
		c.page.Bottom()
	case command.Refresh:
		c.loading = true
		return c, c.load()
	case command.IngestFamily:
		number, ok := c.Selection()
		if !ok {
			return c, status("no patent selected", true)
		}
		return c, ingestFamilyCmd(c.client, number)
	case command.MarkStored, command.MarkUnderReview, command.MarkIgnored,
		command.MarkDeleted, command.AddToProject:
		return c, projectRequiredCmd()
	case command.OpenSearch:
		return c, status("search prompt is not yet wired", false)
	}
	return c, nil
}

// Update implements Pane.
func (c *Catalog) Update(msg tea.Msg) (Pane, tea.Cmd) {
	if m, ok := msg.(catalogLoadedMsg); ok {
		c.loading = false
		if m.err != nil {
			c.loadErr = m.err.Error()
			return c, nil
		}
		c.loadErr = ""
		c.patents = m.patents
		c.page.SetTotal(len(c.patents))
	}
	return c, nil
}

// Selection implements Pane.
func (c *Catalog) Selection() (domain.PatentNumber, bool) {
	cur := c.page.Cursor()
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
	case len(c.patents) == 0:
		return c.theme.Dim.Render("no patents yet — select a number and press f to ingest its family")
	}
	c.page.SetPageSize(max(h-1, 1))

	var b strings.Builder
	b.WriteString(c.theme.Header.Render(catalogRow("NUMBER", "STATE", "TITLE", w)))

	start, end := c.page.Window()
	for i := start; i < end; i++ {
		p := c.patents[i]
		line := catalogRow(p.Number.String(), string(p.FetchState), p.Title, w)
		b.WriteByte('\n')
		if i == c.page.Cursor() {
			b.WriteString(c.theme.Selected.Render(render.Pad(line, w)))
		} else {
			b.WriteString(c.theme.Row.Render(line))
		}
	}
	return b.String()
}

// catalogRow formats one fixed-width catalog line.
func catalogRow(number, state, title string, w int) string {
	titleW := max(w-colNumber-colState-2, 0)
	return render.Pad(number, colNumber) + " " +
		render.Pad(state, colState) + " " +
		render.Truncate(title, titleW)
}
