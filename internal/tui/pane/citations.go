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

// citationsLoadedMsg delivers a finished patent.relations result.
type citationsLoadedMsg struct {
	requestID uint64
	relations []domain.Relation
	err       error
}

// Citations lists one slice of a patent's family graph — the edges of a single
// RelationKind (citations, cited-by, parents, or children) — so the same pane
// type serves every family view.
type Citations struct {
	client *rpc.Client
	theme  render.Theme
	root   domain.PatentNumber
	kind   domain.RelationKind

	relations []domain.Relation
	page      render.Paginator
	loading   bool
	loadErr   string
	loadID    uint64
}

// NewCitations builds a family-edge pane for one patent and relation kind.
func NewCitations(client *rpc.Client, theme render.Theme, root domain.PatentNumber, kind domain.RelationKind) *Citations {
	return &Citations{
		client:  client,
		theme:   theme,
		root:    root,
		kind:    kind,
		page:    render.NewPaginator(10),
		loading: true,
	}
}

// Context implements Pane.
func (c *Citations) Context() command.Context { return command.ContextCitations }

// Title implements Pane.
func (c *Citations) Title() string {
	return relationLabel(c.kind) + " · " + c.root.String()
}

// Init implements Pane.
func (c *Citations) Init() tea.Cmd { return c.load() }

func (c *Citations) load() tea.Cmd {
	client, root, kind := c.client, c.root, c.kind
	requestID := nextAsyncID()
	c.loadID = requestID
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.RelationsResult
		err := client.Call(ctx, proto.MethodRelations,
			proto.RelationsParams{Number: root, Kind: string(kind)}, &res)
		return citationsLoadedMsg{requestID: requestID, relations: res.Relations, err: err}
	}
}

// Command implements Pane.
func (c *Citations) Command(id command.ID, repeat int) (Pane, tea.Cmd) {
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
	}
	return c, nil
}

// Update implements Pane.
func (c *Citations) Update(msg tea.Msg) (Pane, tea.Cmd) {
	if m, ok := msg.(citationsLoadedMsg); ok {
		if m.requestID != c.loadID {
			return c, nil
		}
		c.loading = false
		if m.err != nil {
			c.loadErr = m.err.Error()
			return c, nil
		}
		c.loadErr = ""
		c.relations = m.relations
		c.page.SetTotal(len(c.relations))
	}
	return c, nil
}

// Selection implements Pane: the highlighted neighbour patent.
func (c *Citations) Selection() (domain.PatentNumber, bool) {
	cur := c.page.Cursor()
	if cur < 0 || cur >= len(c.relations) {
		return domain.PatentNumber{}, false
	}
	return c.relations[cur].To, true
}

// View implements Pane.
func (c *Citations) View(w, h int) string {
	switch {
	case c.loading:
		return c.theme.Dim.Render("loading family edges…")
	case c.loadErr != "":
		return c.theme.Error.Render("error: " + c.loadErr)
	case len(c.relations) == 0:
		return c.theme.Dim.Render("no " + relationLabel(c.kind) + " edges recorded")
	}
	c.page.SetPageSize(max(h-1, 1))

	var b strings.Builder
	b.WriteString(c.theme.Header.Render(relationLabel(c.kind)))
	start, end := c.page.Window()
	for i := start; i < end; i++ {
		line := "  " + c.relations[i].To.String()
		b.WriteByte('\n')
		if i == c.page.Cursor() {
			b.WriteString(c.theme.Selected.Render(render.Pad(line, w)))
		} else {
			b.WriteString(c.theme.Row.Render(line))
		}
	}
	return b.String()
}

// relationLabel is the human heading for a relation kind.
func relationLabel(k domain.RelationKind) string {
	switch k {
	case domain.RelationCites:
		return "Citations"
	case domain.RelationCitedBy:
		return "Cited by"
	case domain.RelationParent:
		return "Parents"
	case domain.RelationChild:
		return "Children"
	default:
		return "Relations"
	}
}
