package pane

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/tui/render"
)

// detailDateLayout formats dates in the detail view.
const detailDateLayout = "2006-01-02"

// detailLoadedMsg delivers a finished patent.get result. state and tags are
// project-scoped and empty when the detail pane has no project.
type detailLoadedMsg struct {
	requestID uint64
	patent    domain.Patent
	state     domain.ReviewState
	tags      []domain.Tag
	err       error
}

// detailRelationsMsg delivers the family-graph edge counts for the detail view.
type detailRelationsMsg struct {
	requestID uint64
	counts    map[domain.RelationKind]int
}

// detailRelationKinds are the edge kinds the detail view counts, in display order.
var detailRelationKinds = []domain.RelationKind{
	domain.RelationCites, domain.RelationCitedBy,
	domain.RelationParent, domain.RelationChild,
}

// Detail shows one patent's full record. The record can be longer than the
// body area, so the pane scrolls — every navigation binding in the detail
// keymap layer must resolve to a handler here.
type Detail struct {
	client   *rpc.Client
	theme    render.Theme
	number   domain.PatentNumber
	project  domain.ProjectID
	handlers map[command.ID]cmdHandler

	patent    domain.Patent
	state     domain.ReviewState
	tags      []domain.Tag
	relCounts map[domain.RelationKind]int
	anchors   []render.JumpAnchor // jump targets, rebuilt on every body render
	page      render.Paginator
	loading   bool
	loadErr   string
	loadID    uint64
}

// NewDetail builds a detail pane for one patent number. project, when set,
// scopes the pane's review state and tags; pass "" for a project-independent
// view.
func NewDetail(client *rpc.Client, theme render.Theme, number domain.PatentNumber, project domain.ProjectID) *Detail {
	d := &Detail{
		client:    client,
		theme:     theme,
		number:    number,
		project:   project,
		relCounts: map[domain.RelationKind]int{},
		page:      render.NewPaginator(10),
		loading:   true,
	}
	d.handlers = map[command.ID]cmdHandler{
		command.NavDown:     func(inv Invocation) tea.Cmd { d.page.MoveDown(inv.Repeat); return nil },
		command.NavUp:       func(inv Invocation) tea.Cmd { d.page.MoveUp(inv.Repeat); return nil },
		command.NavPageDown: func(Invocation) tea.Cmd { d.page.PageDown(); return nil },
		command.NavPageUp:   func(Invocation) tea.Cmd { d.page.PageUp(); return nil },
		command.NavTop:      func(Invocation) tea.Cmd { d.page.Top(); return nil },
		command.NavBottom:   func(Invocation) tea.Cmd { d.page.Bottom(); return nil },
		command.Refresh:     func(Invocation) tea.Cmd { d.loading = true; return d.reload() },
		command.IngestFamily: func(Invocation) tea.Cmd {
			return IngestCmd(d.client, d.number, ingestFamilyDepth, domain.CrawlProfileFamily, false)
		},
		command.IngestCitations: func(Invocation) tea.Cmd {
			return IngestCmd(d.client, d.number, ingestFamilyDepth, domain.CrawlProfileCitations, false)
		},
		command.IngestCitedBy: func(Invocation) tea.Cmd {
			return IngestCmd(d.client, d.number, ingestFamilyDepth, domain.CrawlProfileCitedBy, false)
		},
		command.IngestAll: func(Invocation) tea.Cmd {
			return IngestCmd(d.client, d.number, ingestFamilyDepth, domain.CrawlProfileAll, false)
		},
		command.FetchPatent: func(Invocation) tea.Cmd {
			return IngestCmd(d.client, d.number, ingestPatentDepth, "", false)
		},
	}
	return d
}

// Context implements Pane.
func (d *Detail) Context() command.Context { return command.ContextDetail }

// Title implements Pane.
func (d *Detail) Title() string { return "Detail · " + d.number.String() }

// Init implements Pane.
func (d *Detail) Init() tea.Cmd { return d.reload() }

// reload fetches the patent record and its family-graph edge counts.
func (d *Detail) reload() tea.Cmd {
	return tea.Batch(d.load(), d.loadRelations())
}

// load fetches the patent record from the daemon, scoped to the pane's project
// so the reply carries the patent's review state and tags.
func (d *Detail) load() tea.Cmd {
	client, number, project := d.client, d.number, d.project
	requestID := nextAsyncID()
	d.loadID = requestID
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.PatentResult
		err := client.Call(ctx, proto.MethodPatentGet,
			proto.PatentGetParams{Number: number, Project: project}, &res)
		return detailLoadedMsg{
			requestID: requestID,
			patent:    res.Patent,
			state:     res.ReviewState,
			tags:      res.Tags,
			err:       err,
		}
	}
}

// loadRelations counts the patent's family-graph edges by kind.
func (d *Detail) loadRelations() tea.Cmd {
	client, number, requestID := d.client, d.number, d.loadID
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		counts := make(map[domain.RelationKind]int, len(detailRelationKinds))
		for _, kind := range detailRelationKinds {
			var res proto.RelationsResult
			if err := client.Call(ctx, proto.MethodRelations,
				proto.RelationsParams{Number: number, Kind: kind, Limit: 1}, &res); err == nil {
				counts[kind] = res.Total
			}
		}
		return detailRelationsMsg{requestID: requestID, counts: counts}
	}
}

// Command implements Pane.
func (d *Detail) Command(id command.ID, inv Invocation) (Pane, tea.Cmd) {
	if handler, ok := d.handlers[id]; ok {
		return d, handler(inv)
	}
	return d, nil
}

// Handles implements Pane.
func (d *Detail) Handles() []command.ID { return handlerIDs(d.handlers) }

// Update implements Pane.
func (d *Detail) Update(msg tea.Msg) (Pane, tea.Cmd) {
	switch m := msg.(type) {
	case detailLoadedMsg:
		if m.requestID != d.loadID {
			return d, nil
		}
		d.loading = false
		if m.err != nil {
			d.loadErr = m.err.Error()
			return d, nil
		}
		d.loadErr = ""
		d.patent = m.patent
		d.state = m.state
		d.tags = m.tags
		d.page.Top()
	case detailRelationsMsg:
		if m.requestID == d.loadID {
			d.relCounts = m.counts
		}
	case ProjectChangedMsg:
		// The pane's project scopes its review state and tags; a change means
		// the project-relative fields must be re-fetched.
		var project domain.ProjectID
		if m.Project != nil {
			project = m.Project.ID
		}
		if project != d.project {
			d.project = project
			d.loading = true
			return d, d.reload()
		}
	}
	return d, nil
}

// Selection implements Pane: a detail pane's selection is its own patent.
func (d *Detail) Selection() (domain.PatentNumber, bool) {
	return d.number, true
}

// View implements Pane. Long records scroll: the body is built in full, then
// windowed to the visible height by the paginator.
func (d *Detail) View(w, h int) string {
	switch {
	case d.loading:
		return d.theme.Dim.Render("loading " + d.number.String() + "…")
	case d.loadErr != "":
		return d.theme.Error.Render("error: " + d.loadErr)
	}
	lines := strings.Split(d.body(w), "\n")
	d.page.SetTotal(len(lines))
	d.page.SetPageSize(max(h, 1))
	start, end := d.page.Window()
	return strings.Join(lines[start:end], "\n")
}

// body renders the full, unwindowed detail record. It also rebuilds the jump
// anchors, recording the line each labelled field lands on so jump mode can
// scroll straight to it.
func (d *Detail) body(w int) string {
	p := d.patent
	d.anchors = d.anchors[:0]
	var b strings.Builder
	d.field(&b, w, "Shown as", numberToShow(p).String())
	d.field(&b, w, "Record key", p.Number.String())
	d.field(&b, w, "Title", p.Title)
	d.addAnchor(&b, 'a', "Assignee", 0)
	d.field(&b, w, "Assignee", p.Assignee)
	d.addAnchor(&b, 'i', "Inventors", 0)
	var names []string
	for _, inv := range p.Inventors {
		names = append(names, string(inv))
	}
	d.field(&b, w, "Inventors", strings.Join(names, ", "))
	d.field(&b, w, "Country", countryOrDash(p.Number.Country))
	d.field(&b, w, "Fetch state", string(p.FetchState))
	d.field(&b, w, "Source", string(p.Source))
	d.field(&b, w, "Source URL", p.SourceURL)
	d.addAnchor(&b, 'e', "Expiration", 0)
	d.field(&b, w, "Expiration", expirationText(p))

	// Project-scoped fields. Review state and tags describe the patent within
	// one project, so they appear only when the pane has an active project.
	if d.project != "" {
		b.WriteByte('\n')
		d.addAnchor(&b, 'r', "Review state", 0)
		d.field(&b, w, "Review state", reviewStateText(d.state))
		d.addAnchor(&b, 't', "Tags", 0)
		d.field(&b, w, "Tags", tagsText(d.tags))
	}

	// Family-graph edge counts. The dedicated panes (c/b) list the edges.
	b.WriteByte('\n')
	d.addAnchor(&b, 'x', "Citations", 0)
	d.field(&b, w, "Citations", fmt.Sprintf("%d", d.relCounts[domain.RelationCites]))
	d.field(&b, w, "Cited by", fmt.Sprintf("%d", d.relCounts[domain.RelationCitedBy]))
	d.field(&b, w, "Parents", fmt.Sprintf("%d", d.relCounts[domain.RelationParent]))
	d.field(&b, w, "Children", fmt.Sprintf("%d", d.relCounts[domain.RelationChild]))

	// Every life-stage document — the application stays visible here even once
	// the patent has published.
	d.addAnchor(&b, 'd', "Documents", 1)
	b.WriteByte('\n')
	b.WriteString(d.theme.Header.Render("Documents"))
	b.WriteByte('\n')
	if len(p.Documents) == 0 {
		b.WriteString(d.theme.Dim.Render("  (none)"))
		b.WriteByte('\n')
	}
	for _, doc := range p.Documents {
		line := "  " + render.Pad(string(doc.Stage), 13) + " " +
			render.Pad(doc.Number.String(), 20) + " " + dateText(doc.Dated)
		b.WriteString(d.theme.Row.Render(render.Truncate(line, w)))
		b.WriteByte('\n')
	}

	d.addAnchor(&b, 'c', "First claim", 1)
	d.section(&b, w, "First claim", p.FirstClaim)
	d.addAnchor(&b, 'b', "Abstract", 1)
	d.section(&b, w, "Abstract", p.Abstract)
	return strings.TrimRight(b.String(), "\n")
}

// addAnchor records a jump anchor for the next labelled line b will write.
// lineDelta offsets the recorded line past a leading blank or heading: 0 for a
// plain field, 1 for a section whose heading follows a spacer line.
func (d *Detail) addAnchor(b *strings.Builder, key rune, label string, lineDelta int) {
	d.anchors = append(d.anchors, render.JumpAnchor{
		Key:   key,
		Label: label,
		Line:  strings.Count(b.String(), "\n") + lineDelta,
	})
}

// JumpAnchors implements pane.JumpProvider: the jump targets of the last render.
func (d *Detail) JumpAnchors() []render.JumpAnchor { return d.anchors }

// JumpTo implements pane.JumpProvider, scrolling the body so line leads the
// visible window.
func (d *Detail) JumpTo(line int) { d.page.ScrollTo(line) }

// numberToShow returns the record's display number, falling back to the
// record key when no documents set one.
func numberToShow(p domain.Patent) domain.PatentNumber {
	if !p.DisplayNumber.IsZero() {
		return p.DisplayNumber
	}
	return p.Number
}

// field writes one "Label: value" line, truncated to the body width.
func (d *Detail) field(b *strings.Builder, w int, label, value string) {
	const labelW = 14
	if strings.TrimSpace(value) == "" {
		value = "—"
	}
	b.WriteString(d.theme.Header.Render(render.Pad(label, labelW)))
	b.WriteString(" ")
	b.WriteString(d.theme.Row.Render(render.Truncate(value, max(w-labelW-1, 0))))
	b.WriteByte('\n')
}

// section writes a heading followed by word-wrapped body text, so long fields
// like the first claim and abstract are readable in the scrolling view.
func (d *Detail) section(b *strings.Builder, w int, heading, text string) {
	b.WriteByte('\n')
	b.WriteString(d.theme.Header.Render(heading))
	b.WriteByte('\n')
	if strings.TrimSpace(text) == "" {
		b.WriteString(d.theme.Dim.Render("  (none)"))
		b.WriteByte('\n')
		return
	}
	for _, line := range wrapText(text, max(w-2, 1)) {
		b.WriteString(d.theme.Row.Render("  " + line))
		b.WriteByte('\n')
	}
}

// wrapText greedily word-wraps s to lines no wider than width.
func wrapText(s string, width int) []string {
	var lines []string
	var line strings.Builder
	for _, word := range strings.Fields(s) {
		switch {
		case line.Len() == 0:
			line.WriteString(word)
		case line.Len()+1+len(word) <= width:
			line.WriteByte(' ')
			line.WriteString(word)
		default:
			lines = append(lines, line.String())
			line.Reset()
			line.WriteString(word)
		}
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}
	return lines
}

// reviewStateText renders a patent's review state within the pane's project,
// or a note when the patent is not a member of that project.
func reviewStateText(state domain.ReviewState) string {
	if state == "" {
		return "not in project"
	}
	return string(state)
}

// tagsText renders a patent's tags as a comma-separated list, or a dash when
// it carries none.
func tagsText(tags []domain.Tag) string {
	if len(tags) == 0 {
		return "—"
	}
	names := make([]string, len(tags))
	for i, t := range tags {
		names[i] = t.Name
	}
	return strings.Join(names, ", ")
}

// countryOrDash returns the country code, or a dash when it is blank.
func countryOrDash(code string) string {
	if strings.TrimSpace(code) == "" {
		return "—"
	}
	return code
}

// expirationText renders the expiration date and how it was determined.
func expirationText(p domain.Patent) string {
	if p.ExpirationDate.IsZero() {
		return "—"
	}
	text := p.ExpirationDate.Format(detailDateLayout)
	if p.ExpirationSource != "" {
		text += " (" + p.ExpirationSource + ")"
	}
	return text
}

// dateText renders a date, or a dash when it is unset.
func dateText(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format(detailDateLayout)
}
