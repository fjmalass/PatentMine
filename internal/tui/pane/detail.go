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
	idsEntry  *domain.IDSEntry
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

	patent     domain.Patent
	state      domain.ReviewState
	tags       []domain.Tag
	idsEntry   *domain.IDSEntry
	relCounts  map[domain.RelationKind]int
	anchors    []render.JumpAnchor // jump targets, rebuilt on every body render
	jumpKeys   map[string]rune    // label -> assigned jump key, stable for pane lifetime
	page       render.Paginator
	loading    bool
	loadErr    string
	loadID     uint64
	jumpActive bool
}

// detailAnchorLabels are the section labels in display order. The jump key
// assignment algorithm uses this order to assign keys.
var detailAnchorLabels = []string{
	"Assignee",
	"Inventors",
	"Expiration",
	"Classifications",
	"Review state",
	"IDS",
	"Tags",
	"Citations",
	"Documents",
	"First claim",
	"Abstract",
}

// NewDetail builds a detail pane for one patent number. project, when set,
// scopes the pane's review state and tags; pass "" for a project-independent
// view. boundLetters are the single-letter/digit keys bound in the base and
// detail keymap layers, used to avoid conflicts when assigning jump keys.
func NewDetail(client *rpc.Client, theme render.Theme, number domain.PatentNumber, project domain.ProjectID, boundLetters []rune) *Detail {
	d := &Detail{
		client:    client,
		theme:     theme,
		number:    number,
		project:   project,
		relCounts: map[domain.RelationKind]int{},
		page:      render.NewPaginator(10),
		loading:   true,
	}
	d.computeJumpKeys(boundLetters)
	d.handlers = map[command.ID]cmdHandler{
		command.NavDown: func(inv Invocation) tea.Cmd {
			if d.jumpActive && len(d.anchors) > 0 {
				d.page.ScrollTo(d.nextAnchorLine())
			} else {
				d.page.ScrollTo(d.page.Cursor() + inv.Repeat)
			}
			return nil
		},
		command.NavUp: func(inv Invocation) tea.Cmd {
			if d.jumpActive && len(d.anchors) > 0 {
				d.page.ScrollTo(d.prevAnchorLine())
			} else {
				d.page.ScrollTo(d.page.Cursor() - inv.Repeat)
			}
			return nil
		},
		command.NavPageDown: func(Invocation) tea.Cmd { d.page.ScrollTo(d.page.Cursor() + d.page.PageSize()); return nil },
		command.NavPageUp:   func(Invocation) tea.Cmd { d.page.ScrollTo(d.page.Cursor() - d.page.PageSize()); return nil },
		command.NavTop:      func(Invocation) tea.Cmd { d.page.Top(); return nil },
		command.NavBottom:   func(Invocation) tea.Cmd { d.page.Bottom(); return nil },
		command.Refresh:     func(Invocation) tea.Cmd { d.loading = true; return d.reload() },
		command.CrawlFamily: func(Invocation) tea.Cmd {
			return CrawlCmd(d.client, d.number, crawlFamilyDepth, domain.CrawlProfileFamily, false)
		},
		command.CrawlCitations: func(Invocation) tea.Cmd {
			return CrawlCmd(d.client, d.number, crawlFamilyDepth, domain.CrawlProfileCitations, false)
		},
		command.CrawlCitedBy: func(Invocation) tea.Cmd {
			return CrawlCmd(d.client, d.number, crawlFamilyDepth, domain.CrawlProfileCitedBy, false)
		},
		command.CrawlAll: func(Invocation) tea.Cmd {
			return CrawlCmd(d.client, d.number, crawlFamilyDepth, domain.CrawlProfileAll, false)
		},
		command.LookupPatent: func(Invocation) tea.Cmd {
			return CrawlCmd(d.client, d.number, lookupDepth, "", false)
		},
	}
	return d
}

// Context implements Pane.
func (d *Detail) Scope() command.Scope { return command.ScopeDetail }

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
			idsEntry:  res.IDSEntry,
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
		d.idsEntry = m.idsEntry
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
	case d.loading && d.patent.Number.Serial == "":
		return d.theme.Dim.Render("loading " + d.number.String() + "…")
	case d.loadErr != "":
		return d.theme.Error.Render("error: " + d.loadErr)
	}
	lines := strings.Split(d.body(w), "\n")
	d.page.SetTotal(len(lines))
	d.page.SetPageSize(max(h, 1))
	start, end := d.page.Window()
	cursor := d.page.Cursor()
	out := make([]string, 0, end-start)
	for i, line := range lines[start:end] {
		if start+i == cursor {
			out = append(out, d.theme.Selected.Render(render.Pad(line, w)))
		} else {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
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
	d.addAnchor(&b, d.jumpKey("Assignee"), "Assignee", 0)
	d.field(&b, w, "Assignee", p.Assignee)
	d.addAnchor(&b, d.jumpKey("Inventors"), "Inventors", 0)
	var names []string
	for _, inv := range p.Inventors {
		names = append(names, string(inv))
	}
	d.field(&b, w, "Inventors", strings.Join(names, ", "))
	d.field(&b, w, "Country", countryOrDash(p.Number.Country))
	d.field(&b, w, "Fetch state", fetchStateText(d.theme, p.FetchState))
	d.field(&b, w, "Source", string(p.Source))
	d.field(&b, w, "Source URL", p.SourceURL)
	d.addAnchor(&b, d.jumpKey("Expiration"), "Expiration", 0)
	d.field(&b, w, "Expiration", expirationText(p))
	d.addAnchor(&b, d.jumpKey("Classifications"), "Classifications", 0)
	d.field(&b, w, "Classifications", strings.Join(p.Classifications, ", "))

	// Project-scoped fields. Review state and tags describe the patent within
	// one project, so they appear only when the pane has an active project.
	if d.project != "" {
		b.WriteByte('\n')
		d.addAnchor(&b, d.jumpKey("Review state"), "Review state", 0)
		d.field(&b, w, "Review state", styledReviewStateText(d.theme, d.state))
		d.addAnchor(&b, d.jumpKey("IDS"), "IDS", 0)
		d.field(&b, w, "IDS", detailIDSText(d.idsEntry))
		d.addAnchor(&b, d.jumpKey("Tags"), "Tags", 0)
		d.field(&b, w, "Tags", tagsText(d.tags))
	}

	// Family-graph edge counts. The dedicated panes (c/b) list the edges.
	b.WriteByte('\n')
	d.addAnchor(&b, d.jumpKey("Citations"), "Citations", 0)
	d.field(&b, w, "Citations", fmt.Sprintf("%d", d.relCounts[domain.RelationCites]))
	d.field(&b, w, "Cited by", fmt.Sprintf("%d", d.relCounts[domain.RelationCitedBy]))
	d.field(&b, w, "Parents", fmt.Sprintf("%d", d.relCounts[domain.RelationParent]))
	d.field(&b, w, "Children", fmt.Sprintf("%d", d.relCounts[domain.RelationChild]))

	// Every life-stage document — the application stays visible here even once
	// the patent has published.
	d.addAnchor(&b, d.jumpKey("Documents"), "Documents", 1)
	b.WriteByte('\n')
	displayDocs := "Documents"
	if d.jumpActive {
		displayDocs = fmt.Sprintf("[%s] Documents", d.theme.Warn.Copy().Bold(true).Render(string(d.jumpKey("Documents"))))
	}
	b.WriteString(d.theme.Header.Render(displayDocs))
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

	d.addAnchor(&b, d.jumpKey("First claim"), "First claim", 1)
	d.section(&b, w, "First claim", p.FirstClaim)

	// Full claims text action hint
	b.WriteByte('\n')
	fullTextLabel := "Full claims text"
	key := d.jumpKey(fullTextLabel)
	if key != 0 {
		b.WriteString(d.theme.Warn.Render(fmt.Sprintf("[%s] %s — press '%s' to open full text viewer", string(key), fullTextLabel, string(key))))
	} else {
		b.WriteString(d.theme.Dim.Render("Full claims text — press 't' to open full text viewer"))
	}
	b.WriteByte('\n')

	d.addAnchor(&b, d.jumpKey("Abstract"), "Abstract", 1)
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

// SetJumpActive updates the jump mode state, triggering inline shortcut rendering.
func (d *Detail) SetJumpActive(active bool) {
	d.jumpActive = active
}

// JumpActive reports whether jump mode is active.
func (d *Detail) JumpActive() bool { return d.jumpActive }

// nextAnchorLine returns the line of the first anchor after the cursor, or the
// first anchor when the cursor is at or past the last anchor.
func (d *Detail) nextAnchorLine() int {
	cursor := d.page.Cursor()
	for _, a := range d.anchors {
		if a.Line > cursor {
			return a.Line
		}
	}
	return d.anchors[0].Line
}

// prevAnchorLine returns the line of the last anchor before the cursor, or the
// last anchor when the cursor is at or before the first anchor.
func (d *Detail) prevAnchorLine() int {
	cursor := d.page.Cursor()
	for i := len(d.anchors) - 1; i >= 0; i-- {
		if d.anchors[i].Line < cursor {
			return d.anchors[i].Line
		}
	}
	return d.anchors[len(d.anchors)-1].Line
}

// computeJumpKeys assigns a stable jump key to each anchor label, avoiding
// conflicts with keys already bound in the base and detail keymap layers.
// Each label's characters are tried in order (first letter, second, etc.);
// if none are free, the first free letter in a-z is used, then 0-9.
func (d *Detail) computeJumpKeys(bound []rune) {
	boundSet := make(map[rune]bool, len(bound))
	for _, r := range bound {
		boundSet[r] = true
	}
	used := make(map[rune]bool, len(detailAnchorLabels))
	d.jumpKeys = make(map[string]rune, len(detailAnchorLabels))
	for _, label := range detailAnchorLabels {
		key := d.assignKey(label, boundSet, used)
		d.jumpKeys[label] = key
		used[key] = true
	}
}

func (d *Detail) assignKey(label string, boundSet, used map[rune]bool) rune {
	for _, r := range label {
		switch {
		case r >= 'A' && r <= 'Z':
			r = r - 'A' + 'a'
			fallthrough
		case r >= 'a' && r <= 'z':
			if !boundSet[r] && !used[r] {
				return r
			}
		case r >= '0' && r <= '9':
			if !boundSet[r] && !used[r] {
				return r
			}
		}
	}
	for r := 'a'; r <= 'z'; r++ {
		if !boundSet[r] && !used[r] {
			return r
		}
	}
	for r := '0'; r <= '9'; r++ {
		if !used[r] {
			return r
		}
	}
	return '?'
}

// jumpKey returns the assigned jump key for a label, or 0 if unset.
func (d *Detail) jumpKey(label string) rune {
	if d.jumpKeys != nil {
		if key, ok := d.jumpKeys[label]; ok {
			return key
		}
	}
	return 0
}

// HandleKey implements pane.KeyHandler. When jump mode is active it intercepts
// single-letter keys that match a jump anchor, scrolling to that section and
// consuming the key so it never reaches the keymap.
func (d *Detail) HandleKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool) {
	if !d.jumpActive {
		return d, nil, false
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		r := msg.Runes[0]
		for _, a := range d.anchors {
			if a.Key == r {
				d.JumpTo(a.Line)
				return d, nil, true
			}
		}
	}
	return d, nil, false
}

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
	labelW := 14
	displayLabel := label
	if d.jumpActive {
		labelW = 18
		if key, ok := d.jumpKeys[label]; ok {
			displayLabel = fmt.Sprintf("[%s] %s", d.theme.Warn.Copy().Bold(true).Render(string(key)), label)
		} else {
			displayLabel = "    " + label
		}
	}
	if strings.TrimSpace(value) == "" {
		value = "—"
	}
	b.WriteString(d.theme.Header.Render(render.Pad(displayLabel, labelW)))
	b.WriteString(" ")
	b.WriteString(d.theme.Row.Render(render.Truncate(value, max(w-labelW-1, 0))))
	b.WriteByte('\n')
}

// section writes a heading followed by word-wrapped body text, so long fields
// like the first claim and abstract are readable in the scrolling view.
func (d *Detail) section(b *strings.Builder, w int, heading, text string) {
	b.WriteByte('\n')
	displayHeading := heading
	if d.jumpActive {
		if key, ok := d.jumpKeys[heading]; ok {
			displayHeading = fmt.Sprintf("[%s] %s", d.theme.Warn.Copy().Bold(true).Render(string(key)), heading)
		}
	}
	b.WriteString(d.theme.Header.Render(displayHeading))
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

func styledReviewStateText(theme render.Theme, state domain.ReviewState) string {
	text := reviewStateText(state)
	switch state {
	case domain.ReviewStateUnderReview:
		return theme.Warn.Render(text)
	case domain.ReviewStateCached:
		return theme.Dim.Render(text)
	case domain.ReviewStateDeleted:
		return theme.Error.Render(text)
	default:
		return text
	}
}

func fetchStateText(theme render.Theme, state domain.FetchState) string {
	text := string(state)
	switch state {
	case domain.FetchCached:
		return theme.Dim.Render(text)
	case domain.FetchStub:
		return theme.MutedItalic.Render(text)
	default:
		return text
	}
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

func detailIDSText(entry *domain.IDSEntry) string {
	if entry == nil {
		return "not on IDS"
	}
	return entry.SummaryText()
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
