package pane

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	requestID  uint64
	patent     domain.Patent
	state      domain.ReviewState
	tags       []domain.Tag
	idsEntry   *domain.IDSEntry
	patentNote *domain.PatentNote
	err        error
}

// detailRelationCountMsg delivers a single relation kind's edge count.
type detailRelationCountMsg struct {
	requestID uint64
	kind      domain.RelationKind
	count     int
	err       error
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

	patent       domain.Patent
	state        domain.ReviewState
	tags         []domain.Tag
	idsEntry     *domain.IDSEntry
	patentNote   *domain.PatentNote
	relCounts    map[domain.RelationKind]int
	anchors      []render.JumpAnchor // jump targets, rebuilt on every body render
	jumpKeys     map[string]rune     // label -> assigned jump key, stable for pane lifetime
	lineGroups   []detailLineGroup
	page         render.Paginator
	loading      bool
	loadErr        string
	loadID         uint64
	jumpActive     bool
	inventorLine   int
	cachedLines    []string
	lastWidth      int
	lastJumpActive bool
	logger         *slog.Logger
}

type detailLineGroup struct {
	start int
	end   int
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
	"Notes",
	"Citations",
	"Cited by",
	"Parents",
	"Children",
	"Documents",
	"First claim",
	"Abstract",
}

// WithLogger attaches a logger so the pane can persist RPC errors.
func (d *Detail) WithLogger(l *slog.Logger) *Detail { d.logger = l; return d }

func (d *Detail) log() *slog.Logger {
	if d.logger != nil {
		return d.logger
	}
	return slog.Default()
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
				for range inv.Repeat {
					d.page.ScrollTo(d.nextGroupLine())
				}
			}
			return nil
		},
		command.NavUp: func(inv Invocation) tea.Cmd {
			if d.jumpActive && len(d.anchors) > 0 {
				d.page.ScrollTo(d.prevAnchorLine())
			} else {
				for range inv.Repeat {
					d.page.ScrollTo(d.prevGroupLine())
				}
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
			requestID:  requestID,
			patent:     res.Patent,
			state:      res.ReviewState,
			tags:       res.Tags,
			idsEntry:   res.IDSEntry,
			patentNote: res.PatentNote,
			err:        err,
		}
	}
}

// loadRelations counts the patent's family-graph edges by kind.
func (d *Detail) loadRelations() tea.Cmd {
	client, number, requestID := d.client, d.number, d.loadID
	var cmds []tea.Cmd
	for _, kind := range detailRelationKinds {
		k := kind
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := callContext()
			defer cancel()
			var res proto.RelationsResult
			err := client.Call(ctx, proto.MethodRelations,
				proto.RelationsParams{Number: number, Kind: k, Limit: 1}, &res)
			return detailRelationCountMsg{requestID: requestID, kind: k, count: res.Total, err: err}
		})
	}
	return tea.Batch(cmds...)
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
			d.log().Error("patent detail load failed", slog.String("number", d.number.String()), slog.String("error", m.err.Error()))
			return d, nil
		}
		d.loadErr = ""
		d.patent = m.patent
		d.state = m.state
		d.tags = m.tags
		d.idsEntry = m.idsEntry
		d.patentNote = m.patentNote
		d.page.Top()
		d.cachedLines = nil
	case detailRelationCountMsg:
		if m.requestID == d.loadID && m.err == nil {
			d.relCounts[m.kind] = m.count
			d.cachedLines = nil
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
	if d.cachedLines == nil || w != d.lastWidth || d.jumpActive != d.lastJumpActive {
		d.cachedLines = strings.Split(d.body(w), "\n")
		d.lastWidth = w
		d.lastJumpActive = d.jumpActive
	}
	lines := d.cachedLines
	d.page.SetTotal(len(lines))
	d.page.SetPageSize(max(h, 1))
	start, end := d.page.Window()
	cursor := d.page.Cursor()
	highlight := d.highlightGroup(cursor)
	out := make([]string, 0, end-start)
	for i, line := range lines[start:end] {
		lineIndex := start + i
		if lineIndex == cursor || (highlight.start >= 0 && lineIndex >= highlight.start && lineIndex <= highlight.end) {
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
	d.lineGroups = d.lineGroups[:0]
	var b strings.Builder
	d.field(&b, w, "Shown as", numberToShow(p).String())
	d.field(&b, w, "Record key", p.Number.String())
	d.field(&b, w, "Title", p.Title)
	
	// Assignee
	d.addAnchor(&b, d.jumpKey("Assignee"), "Assignee", p.Assignee, false, 0)
	d.field(&b, w, "Assignee", p.Assignee)
	
	// Inventors
	var names []string
	for _, inv := range p.Inventors {
		names = append(names, string(inv))
	}
	invsVal := strings.Join(names, ", ")
	d.addAnchor(&b, d.jumpKey("Inventors"), "Inventors", invsVal, false, 0)
	d.inventorLine = strings.Count(b.String(), "\n")
	d.field(&b, w, "Inventors", invsVal)
	
	d.field(&b, w, "Country", countryOrDash(p.Number.Country))
	d.field(&b, w, "Fetch state", fetchStateText(d.theme, p.FetchState))
	d.field(&b, w, "Source", string(p.Source))
	d.field(&b, w, "Source URL", p.SourceURL)
	
	// Expiration
	expVal := expirationText(p)
	d.addAnchor(&b, d.jumpKey("Expiration"), "Expiration", expVal, false, 0)
	d.field(&b, w, "Expiration", expVal)
	
	// Classifications: comma-joined codes wrapped across the value column.
	// Cached descriptions live in the dedicated popup (K), so the detail field
	// stays compact regardless of how many codes a record carries.
	classVal := strings.Join(p.Classifications, ", ")
	d.addAnchor(&b, d.jumpKey("Classifications"), "Classifications", classVal, false, 0)
	d.wrappedField(&b, w, "Classifications", classVal)

	// Project-scoped fields. Review state and tags describe the patent within
	// one project, so they appear only when the pane has an active project.
	if d.project != "" {
		b.WriteByte('\n')
		revVal := reviewStateText(d.theme, d.state)
		d.addAnchor(&b, d.jumpKey("Review state"), "Review state", revVal, true, 0)
		d.field(&b, w, "Review state", styledReviewStateText(d.theme, d.state))
		
		idsVal := detailIDSText(d.idsEntry)
		d.addAnchor(&b, d.jumpKey("IDS"), "IDS", idsVal, true, 0)
		d.field(&b, w, "IDS", idsVal)
		
		tagsVal := tagsText(d.tags)
		d.addAnchor(&b, d.jumpKey("Tags"), "Tags", tagsVal, true, 0)
		d.field(&b, w, "Tags", tagsVal)
		
		var notesVal string
		if d.patentNote == nil || strings.TrimSpace(d.patentNote.Markdown) == "" {
			notesVal = "—"
		} else {
			notesVal = render.MarkdownHeadings(d.patentNote.Markdown)
		}
		d.addAnchor(&b, d.jumpKey("Notes"), "Notes", notesVal, true, 0)
		d.field(&b, w, "Notes", notesVal)
	}

	// Family-graph edge counts. The dedicated panes (c/b) list the edges.
	b.WriteByte('\n')
	citeVal := fmt.Sprintf("%d", d.relCounts[domain.RelationCites])
	d.addAnchor(&b, d.jumpKey("Citations"), "Citations", citeVal, false, 0)
	d.field(&b, w, "Citations", citeVal)
	
	citedByVal := fmt.Sprintf("%d", d.relCounts[domain.RelationCitedBy])
	d.addAnchor(&b, d.jumpKey("Cited by"), "Cited by", citedByVal, false, 0)
	d.field(&b, w, "Cited by", citedByVal)
	
	parentsVal := fmt.Sprintf("%d", d.relCounts[domain.RelationParent])
	d.addAnchor(&b, d.jumpKey("Parents"), "Parents", parentsVal, false, 0)
	d.field(&b, w, "Parents", parentsVal)
	
	childVal := fmt.Sprintf("%d", d.relCounts[domain.RelationChild])
	d.addAnchor(&b, d.jumpKey("Children"), "Children", childVal, false, 0)
	d.field(&b, w, "Children", childVal)

	// Every life-stage document — the application stays visible here even once
	// the patent has published.
	docVal := fmt.Sprintf("%d documents", len(p.Documents))
	d.addAnchor(&b, d.jumpKey("Documents"), "Documents", docVal, false, 1)
	docStart := strings.Count(b.String(), "\n") + 1
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
		d.lineGroups = append(d.lineGroups, detailLineGroup{start: docStart, end: docStart + 1})
	}
	for _, doc := range p.Documents {
		line := "  " + render.Pad(string(doc.Stage), 13) + " " +
			render.Pad(doc.Number.String(), 20) + " " + dateText(doc.Dated)
		b.WriteString(d.theme.Row.Render(render.Truncate(line, w)))
		b.WriteByte('\n')
	}
	if len(p.Documents) > 0 {
		docEnd := strings.Count(b.String(), "\n") - 1
		d.lineGroups = append(d.lineGroups, detailLineGroup{start: docStart, end: docEnd})
	}

	claimVal := render.Truncate(p.FirstClaim, 60)
	d.addAnchor(&b, d.jumpKey("First claim"), "First claim", claimVal, false, 1)
	d.section(&b, w, "First claim", p.FirstClaim)

	// Full claims text action hint
	b.WriteByte('\n')
	fullTextLabel := "Full claims text"
	key := d.jumpKey(fullTextLabel)
	if key != 0 {
		b.WriteString(d.theme.Warn.Render(fmt.Sprintf("[%s] %s — press '%s' to open full text viewer", string(key), fullTextLabel, string(key))))
	} else {
		b.WriteString(d.theme.Dim.Render("Full claims text — press 'T' to open full text viewer"))
	}
	b.WriteByte('\n')

	abstractVal := render.Truncate(p.Abstract, 60)
	d.addAnchor(&b, d.jumpKey("Abstract"), "Abstract", abstractVal, false, 1)
	d.section(&b, w, "Abstract", p.Abstract)
	return strings.TrimRight(b.String(), "\n")
}

func (d *Detail) highlightGroup(cursor int) detailLineGroup {
	for _, group := range d.lineGroups {
		if cursor >= group.start && cursor <= group.end {
			return group
		}
	}
	return detailLineGroup{start: -1, end: -1}
}

// addAnchor records a jump anchor for the next labelled line b will write.
// lineDelta offsets the recorded line past a leading blank or heading: 0 for a
// plain field, 1 for a section whose heading follows a spacer line.
func (d *Detail) addAnchor(b *strings.Builder, key rune, label, value string, local bool, lineDelta int) {
	d.anchors = append(d.anchors, render.JumpAnchor{
		Key:   key,
		Label: label,
		Line:  strings.Count(b.String(), "\n") + lineDelta,
		Value: value,
		Local: local,
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

func (d *Detail) nextGroupLine() int {
	cursor := d.page.Cursor()
	for _, group := range d.lineGroups {
		if group.start > cursor {
			return group.start
		}
		if cursor >= group.start && cursor < group.end {
			continue
		}
	}
	if len(d.lineGroups) == 0 {
		return cursor + 1
	}
	return d.lineGroups[len(d.lineGroups)-1].start
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

func (d *Detail) prevGroupLine() int {
	cursor := d.page.Cursor()
	for i := len(d.lineGroups) - 1; i >= 0; i-- {
		group := d.lineGroups[i]
		if group.end < cursor {
			return group.start
		}
		if cursor > group.start && cursor <= group.end {
			continue
		}
	}
	if len(d.lineGroups) == 0 {
		return cursor - 1
	}
	return d.lineGroups[0].start
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

	// Pass 1: Satisfy manual overrides first to reserve their keys
	for _, label := range detailAnchorLabels {
		if label == "Citations" || label == "Cited by" || label == "Parents" || label == "Children" {
			key := d.assignKey(label, boundSet, used)
			d.jumpKeys[label] = key
			used[key] = true
		}
	}

	// Pass 2: Dynamically assign keys to all other fields
	for _, label := range detailAnchorLabels {
		if label != "Citations" && label != "Cited by" && label != "Parents" && label != "Children" {
			key := d.assignKey(label, boundSet, used)
			d.jumpKeys[label] = key
			used[key] = true
		}
	}
}

func (d *Detail) assignKey(label string, boundSet, used map[rune]bool) rune {
	switch label {
	case "Citations":
		if !used['c'] {
			return 'c'
		}
	case "Cited by":
		if !used['b'] {
			return 'b'
		}
	case "Parents":
		if !used['p'] {
			return 'p'
		}
	case "Children":
		if !used['C'] {
			return 'C'
		}
	}
	return render.AssignKey(label, used, false)
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
				d.jumpActive = false // Automatically turn off jump mode upon a successful jump
				return d, nil, true
			}
		}
	}
	// Any other key (including esc) exits jump mode and is safely consumed so it doesn't leak normal-mode commands
	d.jumpActive = false
	return d, nil, true
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
	start := strings.Count(b.String(), "\n")
	labelW := 14
	displayLabel := label
	if d.jumpActive {
		labelW = 18
		if key, ok := d.jumpKeys[label]; ok {
			var style lipgloss.Style
			if isLocalField(label) {
				style = d.theme.JumpLocalLabel
			} else {
				// Global field: use a visible accent style or JumpGlobalLabel
				style = d.theme.JumpGlobalLabel
			}
			displayLabel = fmt.Sprintf("%s %s", style.Render(fmt.Sprintf("[%s]", string(key))), label)
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
	d.lineGroups = append(d.lineGroups, detailLineGroup{start: start, end: start})
}

// wrappedField writes one "Label  value" entry, word-wrapping the value
// across as many lines as needed and aligning continuation lines under the
// value column. Used for fields whose contents (like a long classifications
// list) would not fit on a single truncated row.
func (d *Detail) wrappedField(b *strings.Builder, w int, label, value string) {
	labelW := 14
	displayLabel := label
	if d.jumpActive {
		labelW = 18
		if key, ok := d.jumpKeys[label]; ok {
			var style lipgloss.Style
			if isLocalField(label) {
				style = d.theme.JumpLocalLabel
			} else {
				style = d.theme.JumpGlobalLabel
			}
			displayLabel = fmt.Sprintf("%s %s", style.Render(fmt.Sprintf("[%s]", string(key))), label)
		} else {
			displayLabel = "    " + label
		}
	}
	if strings.TrimSpace(value) == "" {
		value = "—"
	}
	valueW := max(w-labelW-1, 0)
	lines := wrapText(value, valueW)
	if len(lines) == 0 {
		lines = []string{""}
	}
	indent := strings.Repeat(" ", labelW+1)
	for i, line := range lines {
		start := strings.Count(b.String(), "\n")
		if i == 0 {
			b.WriteString(d.theme.Header.Render(render.Pad(displayLabel, labelW)))
			b.WriteString(" ")
		} else {
			b.WriteString(indent)
		}
		b.WriteString(d.theme.Row.Render(line))
		b.WriteByte('\n')
		d.lineGroups = append(d.lineGroups, detailLineGroup{start: start, end: start})
	}
}

// section writes a heading followed by word-wrapped body text, so long fields
// like the first claim and abstract are readable in the scrolling view.
func (d *Detail) section(b *strings.Builder, w int, heading, text string) {
	start := strings.Count(b.String(), "\n") + 1
	b.WriteByte('\n')
	displayHeading := heading
	if d.jumpActive {
		if key, ok := d.jumpKeys[heading]; ok {
			var style lipgloss.Style
			if isLocalField(heading) {
				style = d.theme.JumpLocalLabel
			} else {
				style = d.theme.JumpGlobalLabel
			}
			displayHeading = fmt.Sprintf("%s %s", style.Render(fmt.Sprintf("[%s]", string(key))), heading)
		}
	}
	b.WriteString(d.theme.Header.Render(displayHeading))
	b.WriteByte('\n')
	if strings.TrimSpace(text) == "" {
		b.WriteString(d.theme.Dim.Render("  (none)"))
		b.WriteByte('\n')
		d.lineGroups = append(d.lineGroups, detailLineGroup{start: start, end: start + 1})
		return
	}
	for _, line := range wrapText(text, max(w-2, 1)) {
		b.WriteString(d.theme.Row.Render("  " + line))
		b.WriteByte('\n')
	}
	end := strings.Count(b.String(), "\n") - 1
	d.lineGroups = append(d.lineGroups, detailLineGroup{start: start, end: end})
}

func isLocalField(label string) bool {
	switch label {
	case "Review state", "IDS", "Tags", "Notes":
		return true
	default:
		return false
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
func reviewStateText(theme render.Theme, state domain.ReviewState) string {
	if state == "" {
		return "not in project"
	}
	return theme.ReviewStateGlyph(string(state))
}

func styledReviewStateText(theme render.Theme, state domain.ReviewState) string {
	text := reviewStateText(theme, state)
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
	text := theme.FetchStateGlyph(string(state))
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
	if entry.Project == "" || entry.Patent.IsZero() {
		return "—"
	}
	parts := []string{string(entry.Status)}
	if !entry.Status.Valid() {
		parts[0] = string(domain.IDSEntryPending)
	}
	switch {
	case entry.InFull:
		parts = append(parts, "full")
	case strings.TrimSpace(entry.RelevantPassages) != "":
		parts = append(parts, strings.TrimSpace(entry.RelevantPassages))
	}
	if note := strings.TrimSpace(entry.Notes); note != "" {
		parts = append(parts, render.MarkdownHeadings(note))
	}
	return strings.Join(parts, " | ")
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

func detailDateTimeText(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format("2006-01-02 15:04:05")
}

func (d *Detail) Patent() domain.Patent     { return d.patent }
func (d *Detail) IsCursorOnInventors() bool { return d.page.Cursor() == d.inventorLine }

// ResolveCursorRelation checks if the cursor is currently hovering over a relation count line
// and returns the corresponding RelationKind if it is.
func (d *Detail) ResolveCursorRelation() (domain.RelationKind, bool) {
	cursor := d.page.Cursor()
	for _, a := range d.anchors {
		if a.Line == cursor {
			switch a.Label {
			case "Citations":
				return domain.RelationCites, true
			case "Cited by":
				return domain.RelationCitedBy, true
			case "Parents":
				return domain.RelationParent, true
			case "Children":
				return domain.RelationChild, true
			}
		}
	}
	return "", false
}

func (d *Detail) PatentNumber() domain.PatentNumber { return d.number }
