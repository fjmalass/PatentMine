package pane

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/tui/render"
)

// detailDateLayout formats dates in the detail view.
const detailDateLayout = "2006-01-02"

const detailClassificationLimit = 24

const (
	detailLabelShownAs         = "Shown as"
	detailLabelRecordKey       = "Record key"
	detailLabelTitle           = "Title"
	detailLabelAssignee        = "Assignee"
	detailLabelInventors       = "Inventors"
	detailLabelCountry         = "Country"
	detailLabelFetchState      = "Fetch state"
	detailLabelSource          = "Source"
	detailLabelSourceURL       = "Source URL"
	detailLabelExpiration      = "Expiration"
	detailLabelClassifications = "Classifications"
	detailLabelReviewState     = "Review state"
	detailLabelIDS             = "IDS"
	detailLabelTags            = "Tags"
	detailLabelNotes           = "Notes"
	detailLabelCitations       = "Citations"
	detailLabelCitedBy         = "Cited by"
	detailLabelParents         = "Parents"
	detailLabelChildren        = "Children"
	detailLabelDocuments       = "Documents"
	detailLabelFirstClaim      = "First claim"
	detailLabelFullClaimsText  = "Full claims text"
	detailLabelAbstract        = "Abstract"
)

// detailLoadedMsg delivers a finished patent.get result. state and tags are
// project-scoped and empty when the detail pane has no project.
type detailLoadedMsg struct {
	requestID  uint64
	patent     domain.Patent
	state      domain.ReviewState
	tags       []domain.Tag
	idsEntry   *domain.IDSEntry
	patentNote *domain.PatentNote
	usptoApp   *domain.USPTOApplication
	err        error
}

// detailRelationCountMsg delivers a single relation kind's edge count.
type detailRelationCountMsg struct {
	requestID uint64
	kind      domain.RelationKind
	count     int
	err       error
}

// detailSourceDiffsMsg delivers whether the current patent has recorded
// source comparison diffs (from compare mode + enrichment). Used for the
// conditional "C compare sources" hint in the detail view (Option A).
type detailSourceDiffsMsg struct {
	requestID uint64
	hasDiffs  bool
	err       error
}

// detailCitationSourceMsg delivers the number of a patent's citations that one
// crawler observed, for the source-coverage comparison on the Citations line.
type detailCitationSourceMsg struct {
	requestID uint64
	source    domain.Source
	count     int
	err       error
}

// detailRelationKinds are the edge kinds the detail view counts, in display order.
var detailRelationKinds = []domain.RelationKind{
	domain.RelationCites, domain.RelationCitedBy,
	domain.RelationParent, domain.RelationChild,
}

// detailCitationSources are the crawlers compared on the Citations line. A
// citation present for both is reported as the overlap; one present for only
// one source is that source's unique contribution.
var detailCitationSources = []domain.Source{domain.SourceGoogle, domain.SourceUSPTO}

// Detail shows one patent's full record. The record can be longer than the
// body area, so the pane scrolls — every navigation binding in the detail
// keymap layer must resolve to a handler here.
type Detail struct {
	client   *rpc.Client
	theme    render.Theme
	number   domain.PatentNumber
	project  domain.ProjectID
	handlers map[command.ID]cmdHandler

	patent             domain.Patent
	state              domain.ReviewState
	tags               []domain.Tag
	idsEntry           *domain.IDSEntry
	patentNote         *domain.PatentNote
	usptoApp           *domain.USPTOApplication
	relCounts          map[domain.RelationKind]int
	citeSourceCounts   map[domain.Source]int
	hasSourceDiffs     bool
	jump               *JumpController
	lineGroups         []detailLineGroup
	page               render.Paginator
	loading            bool
	loadErr            string
	loadID             uint64
	assigneeLine       int
	classificationLine int
	inventorLine       int
	pgpubURLLine       int
	pgpubURLLineEnd    int
	grantURLLine       int
	grantURLLineEnd    int
	cachedLines        []string
	lastWidth          int
	lastJumpActive     bool
	logger             *slog.Logger
}

type detailLineGroup struct {
	start int
	end   int
}

// detailAnchorLabels are the section labels in display order. The jump key
// assignment algorithm uses this order to assign keys.
var detailAnchorLabels = []string{
	detailLabelAssignee,
	detailLabelInventors,
	detailLabelExpiration,
	detailLabelClassifications,
	detailLabelReviewState,
	detailLabelIDS,
	detailLabelTags,
	detailLabelNotes,
	detailLabelCitations,
	detailLabelCitedBy,
	detailLabelParents,
	detailLabelChildren,
	detailLabelDocuments,
	detailLabelFirstClaim,
	detailLabelAbstract,
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
		client:           client,
		theme:            theme,
		number:           number,
		project:          project,
		relCounts:        map[domain.RelationKind]int{},
		citeSourceCounts: map[domain.Source]int{},
		page:             render.NewPaginator(10),
		loading:          true,
		jump:             NewJumpController(),
	}
	override := func(label string, used map[rune]bool) rune {
		switch label {
		case detailLabelCitations:
			if !used['c'] {
				return 'c'
			}
		case detailLabelCitedBy:
			if !used['b'] {
				return 'b'
			}
		case detailLabelParents:
			if !used['p'] {
				return 'p'
			}
		case detailLabelChildren:
			if !used['C'] {
				return 'C'
			}
		}
		return 0
	}
	d.jump.Compute(detailAnchorLabels, boundLetters, override, false)
	d.handlers = map[command.ID]cmdHandler{
		command.NavDown: func(inv Invocation) tea.Cmd {
			if d.jump.Active && len(d.jump.Anchors) > 0 {
				d.page.ScrollTo(d.nextAnchorLine())
			} else {
				for range inv.Repeat {
					d.gotoGroupLine(d.nextGroupLine())
				}
			}
			return nil
		},
		command.NavUp: func(inv Invocation) tea.Cmd {
			if d.jump.Active && len(d.jump.Anchors) > 0 {
				d.page.ScrollTo(d.prevAnchorLine())
			} else {
				for range inv.Repeat {
					d.gotoGroupLine(d.prevGroupLine())
				}
			}
			return nil
		},
		command.NavPageDown: func(Invocation) tea.Cmd { d.page.ScrollTo(d.page.Cursor() + d.page.PageSize()); return nil },
		command.NavPageUp:   func(Invocation) tea.Cmd { d.page.ScrollTo(d.page.Cursor() - d.page.PageSize()); return nil },
		command.NavTop:      func(inv Invocation) tea.Cmd { d.page.NavTop(inv.Repeat); return nil },
		command.NavBottom:   func(inv Invocation) tea.Cmd { d.page.NavBottom(inv.Repeat); return nil },
		command.Refresh:     func(Invocation) tea.Cmd { d.loading = true; return d.reload() },
		command.CrawlFamily: func(Invocation) tea.Cmd {
			return CrawlCmd(d.client, d.number, crawlFamilyDepth, domain.CrawlProfileFamily, false, "")
		},
		command.CrawlCitations: func(Invocation) tea.Cmd {
			return CrawlCmd(d.client, d.number, crawlFamilyDepth, domain.CrawlProfileCitations, false, "")
		},
		command.CrawlCitedBy: func(Invocation) tea.Cmd {
			return CrawlCmd(d.client, d.number, crawlFamilyDepth, domain.CrawlProfileCitedBy, false, "")
		},
		command.CrawlAll: func(Invocation) tea.Cmd {
			return CrawlCmd(d.client, d.number, crawlFamilyDepth, domain.CrawlProfileAll, false, "")
		},
		command.LookupPatent: func(Invocation) tea.Cmd {
			// Force a fresh fetch when the user explicitly hits L on a stub.
			// The plain :add / depth-0 path can leave a near-empty stub; L must
			// actually pull data instead of repeating the same weak lookup.
			return CrawlCmd(d.client, d.number, lookupDepth, "", true, "")
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
	return tea.Batch(d.load(), d.loadRelations(), d.loadSourceDiffs())
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
			usptoApp:   res.USPTOApplication,
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
	// Per-source citation totals for the coverage comparison. Each call counts
	// the citations one crawler observed; the render derives the overlap.
	for _, src := range detailCitationSources {
		s := src
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := callContext()
			defer cancel()
			var res proto.RelationsResult
			err := client.Call(ctx, proto.MethodRelations,
				proto.RelationsParams{Number: number, Kind: domain.RelationCites, Source: s, Limit: 1}, &res)
			return detailCitationSourceMsg{requestID: requestID, source: s, count: res.Total, err: err}
		})
	}
	return tea.Batch(cmds...)
}

// citationSourceCoverage renders the per-source split of the citation count,
// e.g. "(google 30, uspto 38, both 26)". It lets the user compare what each
// crawler observed and see the overlap. Returns "" until at least one source
// count has loaded, so a graph with no per-source data shows just the total.
func (d *Detail) citationSourceCoverage() string {
	var parts []string
	sum := 0
	for _, src := range detailCitationSources {
		c := d.citeSourceCounts[src]
		sum += c
		if c > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", src, c))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	// Inclusion-exclusion: an edge counted by both crawlers appears in each
	// source total, so the overlap is their sum minus the distinct total.
	if both := sum - d.relCounts[domain.RelationCites]; both > 0 {
		parts = append(parts, fmt.Sprintf("both %d", both))
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

// loadSourceDiffs checks (lightweight) whether source comparison diffs exist
// for the current patent. Used to drive the conditional hint + enable the
// comparison overlay (Option A reconciliation flow).
func (d *Detail) loadSourceDiffs() tea.Cmd {
	client, number, requestID := d.client, d.number, d.loadID
	if client == nil || number.IsZero() {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.SourceDiffsListResult
		err := client.Call(ctx, proto.MethodSourceDiffsList,
			proto.SourceDiffsListParams{Number: number}, &res)
		has := err == nil && len(res.Diffs) > 0
		return detailSourceDiffsMsg{requestID: requestID, hasDiffs: has, err: err}
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
			d.log().Error("patent detail load failed", slog.String("number", d.number.String()), slog.String("error", m.err.Error()))
			return d, nil
		}
		d.loadErr = ""
		d.patent = m.patent
		d.state = m.state
		d.tags = m.tags
		d.idsEntry = m.idsEntry
		d.patentNote = m.patentNote
		d.usptoApp = m.usptoApp
		d.hasSourceDiffs = false // will be set by the async loadSourceDiffs
		d.citeSourceCounts = map[domain.Source]int{}
		d.page.Top()
		d.cachedLines = nil
	case detailRelationCountMsg:
		if m.requestID == d.loadID && m.err == nil {
			d.relCounts[m.kind] = m.count
			d.cachedLines = nil
		}
	case detailCitationSourceMsg:
		if m.requestID == d.loadID && m.err == nil {
			d.citeSourceCounts[m.source] = m.count
			d.cachedLines = nil
		}
	case detailSourceDiffsMsg:
		if m.requestID == d.loadID {
			d.hasSourceDiffs = m.hasDiffs && m.err == nil
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
	case ReviewStateChangedMsg:
		if d.project != m.Project {
			return d, nil
		}
		for _, patent := range m.Patents {
			if patentNumberMatches(d.number, d.patent.DisplayNumber, patent) {
				d.state = m.State
				d.cachedLines = nil
				break
			}
		}
	case IDSEntryChangedMsg:
		if d.project != m.Project || !patentNumberMatches(d.number, d.patent.DisplayNumber, m.Patent) {
			return d, nil
		}
		d.idsEntry = m.Entry
		d.cachedLines = nil
	case IDSEntriesChangedMsg:
		for i := range m.Entries {
			entry := &m.Entries[i]
			if d.project != entry.Project || !patentNumberMatches(d.number, d.patent.DisplayNumber, entry.Patent) {
				continue
			}
			copied := *entry
			d.idsEntry = &copied
			d.cachedLines = nil
			break
		}
	}
	return d, nil
}

// Selection implements Pane: a detail pane's selection is its own patent.
func (d *Detail) Selection() (domain.PatentNumber, bool) {
	return d.number, true
}

// ActivityFocus implements ActivityFocusProvider.
func (d *Detail) ActivityFocus() (ActivityFocus, bool) {
	attrs := map[string]any{
		"scope":           "detail",
		"display_number":  numberToShow(d.patent).String(),
		"title":           d.patent.Title,
		"review_state":    d.state,
		"tags":            d.tags,
		"classifications": d.patent.Classifications,
		"line":            d.page.Cursor(),
		"inventors_short": formatInventorsShort(d.patent.Inventors),
	}
	if !d.patent.PublicationDate.IsZero() {
		attrs["publication_date"] = d.patent.PublicationDate.Format("2006-01-02")
	}
	if d.project != "" {
		attrs["project"] = d.project
	}
	return ActivityFocus{Entity: "patent", EntityID: d.number.String(), Label: d.Title(), Attributes: attrs}, true
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
	// Leave one column unused. Exact-width ANSI-styled rows can leave some
	// terminals in wrap-pending state, causing stale rows to look duplicated.
	renderW := max(w-1, 1)
	if d.cachedLines == nil || renderW != d.lastWidth || d.jump.Active != d.lastJumpActive {
		d.cachedLines = strings.Split(d.body(renderW), "\n")
		d.lastWidth = renderW
		d.lastJumpActive = d.jump.Active
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
		line = render.Truncate(line, renderW)
		if lineIndex == cursor || (highlight.start >= 0 && lineIndex >= highlight.start && lineIndex <= highlight.end) {
			out = append(out, d.theme.Selected.Render(render.Pad(line, renderW)))
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
	d.jump.ClearAnchors()
	d.lineGroups = d.lineGroups[:0]
	d.pgpubURLLine, d.pgpubURLLineEnd = -1, -1
	d.grantURLLine, d.grantURLLineEnd = -1, -1
	var b strings.Builder
	d.field(&b, w, detailLabelShownAs, numberToShow(p).String())
	d.field(&b, w, detailLabelRecordKey, p.Number.String())
	d.field(&b, w, detailLabelTitle, p.Title)

	// Assignee
	d.addAnchor(&b, d.jumpKey(detailLabelAssignee), detailLabelAssignee, p.Assignee, false, 0)
	d.assigneeLine = strings.Count(b.String(), "\n")
	d.field(&b, w, detailLabelAssignee, p.Assignee)

	// Inventors
	var names []string
	for _, inv := range p.Inventors {
		names = append(names, string(inv))
	}
	invsVal := strings.Join(names, ", ")
	d.addAnchor(&b, d.jumpKey(detailLabelInventors), detailLabelInventors, invsVal, false, 0)
	d.inventorLine = strings.Count(b.String(), "\n")
	d.field(&b, w, detailLabelInventors, invsVal)

	d.field(&b, w, detailLabelCountry, countryOrDash(p.Number.Country))
	d.field(&b, w, detailLabelFetchState, detailFetchStateText(d.theme, p.FetchState))
	d.field(&b, w, detailLabelSource, string(p.Source))
	d.field(&b, w, detailLabelSourceURL, p.SourceURL)

	// Conditional source comparison hint (Option A) — shown only when
	// source_diff rows exist for this patent (from compare mode + enrichment).
	// Use "g c" (while detail focused) or :source-compare to open the split
	// review & choose (defaults to USPTO).
	if d.hasSourceDiffs {
		b.WriteString(d.theme.Warn.Render("  compare sources — differences from other providers (g c / :source-compare)"))
		b.WriteByte('\n')
	}

	// USPTO Application details
	if d.usptoApp != nil && d.usptoApp.ApplicationNumber != "" {
		b.WriteByte('\n')
		b.WriteString(d.theme.Dim.Render("—— USPTO Application File Wrapper ——"))
		b.WriteByte('\n')
		d.field(&b, w, "USPTO App", d.usptoApp.ApplicationNumber)
		d.field(&b, w, "Filing Date", d.usptoApp.FilingDate)
		if d.usptoApp.EffectiveFilingDate != "" && d.usptoApp.EffectiveFilingDate != d.usptoApp.FilingDate {
			d.field(&b, w, "Eff. Filing", d.usptoApp.EffectiveFilingDate)
		}
		statusVal := d.usptoApp.ApplicationStatusText
		if d.usptoApp.ApplicationStatusCode != "" {
			statusVal = fmt.Sprintf("[%s] %s", d.usptoApp.ApplicationStatusCode, d.usptoApp.ApplicationStatusText)
		}
		d.field(&b, w, "Status", statusVal)
		if d.usptoApp.ApplicationStatusDate != "" {
			d.field(&b, w, "Status Date", d.usptoApp.ApplicationStatusDate)
		}
		if d.usptoApp.GroupArtUnitNumber != "" {
			d.field(&b, w, "Art Unit", d.usptoApp.GroupArtUnitNumber)
		}
		if d.usptoApp.ExaminerName != "" {
			d.field(&b, w, "Examiner", d.usptoApp.ExaminerName)
		}
		if d.usptoApp.DocketNumber != "" {
			d.field(&b, w, "Docket Num", d.usptoApp.DocketNumber)
		}
		entityVal := d.usptoApp.BusinessEntityStatus
		if d.usptoApp.SmallEntityStatus {
			if entityVal != "" {
				entityVal += " (Small)"
			} else {
				entityVal = "Small"
			}
		}
		if entityVal != "" {
			d.field(&b, w, "Entity", entityVal)
		}
		if d.usptoApp.PGPubXMLURL != "" {
			d.pgpubURLLine = strings.Count(b.String(), "\n")
			d.field(&b, w, "PGPub XML", d.usptoApp.PGPubXMLName)
			d.wrappedField(&b, w, "PGPub URL", d.usptoApp.PGPubXMLURL)
			d.pgpubURLLineEnd = strings.Count(b.String(), "\n") - 1
		}
		if d.usptoApp.PatentGrantXMLURL != "" {
			d.grantURLLine = strings.Count(b.String(), "\n")
			d.field(&b, w, "Grant XML", d.usptoApp.PatentGrantXMLName)
			d.wrappedField(&b, w, "Grant URL", d.usptoApp.PatentGrantXMLURL)
			d.grantURLLineEnd = strings.Count(b.String(), "\n") - 1
		}
	}

	// Expiration
	expVal := expirationText(p)
	d.addAnchor(&b, d.jumpKey(detailLabelExpiration), detailLabelExpiration, expVal, false, 0)
	d.field(&b, w, detailLabelExpiration, expVal)

	// Classifications: comma-joined codes wrapped across the value column.
	// Cached descriptions live in the dedicated popup (K), so the detail field
	// stays compact regardless of how many codes a record carries.
	classVal := detailClassificationsText(p.Classifications)
	d.addAnchor(&b, d.jumpKey(detailLabelClassifications), detailLabelClassifications, classVal, false, 0)
	d.classificationLine = strings.Count(b.String(), "\n")
	d.wrappedField(&b, w, detailLabelClassifications, classVal)

	// Project-scoped fields. Review state and tags describe the patent within
	// one project, so they appear only when the pane has an active project.
	if d.project != "" {
		b.WriteByte('\n')
		revVal := detailReviewStateText(d.state)
		d.addAnchor(&b, d.jumpKey(detailLabelReviewState), detailLabelReviewState, revVal, true, 0)
		d.field(&b, w, detailLabelReviewState, detailStyledReviewStateText(d.theme, d.state))

		idsVal := detailIDSText(d.theme, d.idsEntry)
		d.addAnchor(&b, d.jumpKey(detailLabelIDS), detailLabelIDS, idsVal, true, 0)
		d.field(&b, w, detailLabelIDS, idsVal)

		tagsVal := tagsText(d.tags)
		d.addAnchor(&b, d.jumpKey(detailLabelTags), detailLabelTags, tagsVal, true, 0)
		d.field(&b, w, detailLabelTags, tagsVal)

		var notesVal string
		if d.patentNote == nil || strings.TrimSpace(d.patentNote.Markdown) == "" {
			notesVal = "—"
		} else {
			notesVal = render.MarkdownHeadings(d.patentNote.Markdown)
		}
		d.addAnchor(&b, d.jumpKey(detailLabelNotes), detailLabelNotes, notesVal, true, 0)
		d.field(&b, w, detailLabelNotes, notesVal)
	}

	// Family-graph edge counts. The dedicated panes (c/b) list the edges.
	b.WriteByte('\n')
	citeVal := fmt.Sprintf("%d", d.relCounts[domain.RelationCites])
	if cov := d.citationSourceCoverage(); cov != "" {
		citeVal += " " + cov
	}
	d.addAnchor(&b, d.jumpKey(detailLabelCitations), detailLabelCitations, citeVal, false, 0)
	d.field(&b, w, detailLabelCitations, citeVal)

	citedByVal := fmt.Sprintf("%d", d.relCounts[domain.RelationCitedBy])
	d.addAnchor(&b, d.jumpKey(detailLabelCitedBy), detailLabelCitedBy, citedByVal, false, 0)
	d.field(&b, w, detailLabelCitedBy, citedByVal)

	parentsVal := fmt.Sprintf("%d", d.relCounts[domain.RelationParent])
	d.addAnchor(&b, d.jumpKey(detailLabelParents), detailLabelParents, parentsVal, false, 0)
	d.field(&b, w, detailLabelParents, parentsVal)

	childVal := fmt.Sprintf("%d", d.relCounts[domain.RelationChild])
	d.addAnchor(&b, d.jumpKey(detailLabelChildren), detailLabelChildren, childVal, false, 0)
	d.field(&b, w, detailLabelChildren, childVal)

	// Every life-stage document — the application stays visible here even once
	// the patent has published.
	docVal := fmt.Sprintf("%d documents", len(p.Documents))
	d.addAnchor(&b, d.jumpKey(detailLabelDocuments), detailLabelDocuments, docVal, false, 1)
	docStart := strings.Count(b.String(), "\n") + 1
	b.WriteByte('\n')
	displayDocs := detailLabelDocuments
	if d.jump.Active {
		displayDocs = fmt.Sprintf("[%s] %s", d.theme.Warn.Copy().Bold(true).Render(string(d.jumpKey(detailLabelDocuments))), detailLabelDocuments)
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
	d.addAnchor(&b, d.jumpKey(detailLabelFirstClaim), detailLabelFirstClaim, claimVal, false, 1)
	d.section(&b, w, detailLabelFirstClaim, p.FirstClaim)

	// Full claims text action hint
	b.WriteByte('\n')
	fullTextLabel := detailLabelFullClaimsText
	key := d.jumpKey(fullTextLabel)
	if key != 0 {
		b.WriteString(d.theme.Warn.Render(fmt.Sprintf("[%s] %s — press '%s' to open full text viewer", string(key), fullTextLabel, string(key))))
	} else {
		b.WriteString(d.theme.Dim.Render("Full claims text — press 'T' to open full text viewer"))
	}
	b.WriteByte('\n')

	abstractVal := render.Truncate(p.Abstract, 60)
	d.addAnchor(&b, d.jumpKey(detailLabelAbstract), detailLabelAbstract, abstractVal, false, 1)
	d.section(&b, w, detailLabelAbstract, p.Abstract)
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
	d.jump.AddAnchor(label, value, strings.Count(b.String(), "\n")+lineDelta, local, key)
}

// JumpAnchors implements pane.JumpProvider: the jump targets of the last render.
func (d *Detail) JumpAnchors() []render.JumpAnchor { return d.jump.JumpAnchors() }

// JumpTo implements pane.JumpProvider, scrolling the body so line leads the
// visible window.
func (d *Detail) JumpTo(line int) { d.page.ScrollTo(line) }

// SetJumpActive updates the jump mode state, triggering inline shortcut rendering.
func (d *Detail) SetJumpActive(active bool) {
	d.jump.SetJumpActive(active)
}

// JumpActive reports whether jump mode is active.
func (d *Detail) JumpActive() bool { return d.jump.JumpActive() }

// nextAnchorLine returns the line of the first anchor after the cursor, or the
// first anchor when the cursor is at or past the last anchor.
func (d *Detail) nextAnchorLine() int {
	cursor := d.page.Cursor()
	anchors := d.jump.JumpAnchors()
	for _, a := range anchors {
		if a.Line > cursor {
			return a.Line
		}
	}
	return anchors[0].Line
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
	anchors := d.jump.JumpAnchors()
	for i := len(anchors) - 1; i >= 0; i-- {
		if anchors[i].Line < cursor {
			return anchors[i].Line
		}
	}
	return anchors[len(anchors)-1].Line
}

func (d *Detail) prevGroupLine() int {
	cursor := d.page.Cursor()
	for i := len(d.lineGroups) - 1; i >= 0; i-- {
		group := d.lineGroups[i]
		if group.end < cursor {
			return group.start
		}
		if cursor > group.start && cursor <= group.end {
			return group.start
		}
	}
	if len(d.lineGroups) == 0 {
		return cursor - 1
	}
	return d.lineGroups[0].start
}

func (d *Detail) gotoGroupLine(line int) {
	d.page.GotoLine(line + 1)
}

// jumpKey returns the assigned jump key for a label, or 0 if unset.
func (d *Detail) jumpKey(label string) rune {
	return d.jump.JumpKey(label)
}

// HandleKey implements pane.KeyHandler. When jump mode is active it intercepts
// single-letter keys that match a jump anchor, scrolling to that section and
// consuming the key so it never reaches the keymap.
func (d *Detail) HandleKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool) {
	if !d.jump.JumpActive() {
		return d, nil, false
	}
	consumed := d.jump.HandleKey(msg, d.JumpTo)
	return d, nil, consumed
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
	if d.jump.Active {
		labelW = 18
		if key := d.jump.JumpKey(label); key != 0 {
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
	if d.jump.Active {
		labelW = 18
		if key := d.jump.JumpKey(label); key != 0 {
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
	start := strings.Count(b.String(), "\n")
	for i, line := range lines {
		if i == 0 {
			b.WriteString(d.theme.Header.Render(render.Pad(displayLabel, labelW)))
			b.WriteString(" ")
		} else {
			b.WriteString(indent)
		}
		b.WriteString(d.theme.Row.Render(line))
		b.WriteByte('\n')
	}
	d.lineGroups = append(d.lineGroups, detailLineGroup{start: start, end: strings.Count(b.String(), "\n") - 1})
}

// section writes a heading followed by word-wrapped body text, so long fields
// like the first claim and abstract are readable in the scrolling view.
func (d *Detail) section(b *strings.Builder, w int, heading, text string) {
	start := strings.Count(b.String(), "\n") + 1
	b.WriteByte('\n')
	displayHeading := heading
	if d.jump.Active {
		if key := d.jump.JumpKey(heading); key != 0 {
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
	case detailLabelReviewState, detailLabelIDS, detailLabelTags, detailLabelNotes:
		return true
	default:
		return false
	}
}

// wrapText greedily word-wraps s to lines no wider than width, hard-splitting
// single tokens that would otherwise force the terminal to wrap physical rows.
// Pre-existing newlines and leading spaces are preserved to maintain hierarchical structures.
func wrapText(s string, width int) []string {
	if width <= 0 {
		return nil
	}
	var lines []string

	// Split by newlines to respect pre-existing line breaks and indentation
	rawLines := strings.Split(s, "\n")
	for _, rawLine := range rawLines {
		// Detect leading spaces for indentation preservation
		var indent strings.Builder
		for _, r := range rawLine {
			if r == ' ' {
				indent.WriteRune(r)
			} else {
				break
			}
		}
		indentStr := indent.String()
		indentWidth := render.StringWidth(indentStr)

		// Trim whitespace so we can wrap words
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" {
			lines = append(lines, "")
			continue
		}

		// If the indent width is greater than or equal to the target width,
		// wrap without prepending the indent to avoid infinite loops/overflows.
		effectiveIndent := indentStr
		effectiveWidth := width - indentWidth
		if effectiveWidth <= 5 { // Require a minimum of 5 characters for wrapped text
			effectiveIndent = ""
			effectiveWidth = width
		}

		var line strings.Builder
		flush := func() {
			if line.Len() == 0 {
				return
			}
			lines = append(lines, effectiveIndent+line.String())
			line.Reset()
		}

		for _, word := range strings.Fields(trimmed) {
			for render.StringWidth(word) > effectiveWidth {
				flush()
				lines = append(lines, effectiveIndent+ansi.Cut(word, 0, effectiveWidth))
				word = ansi.Cut(word, effectiveWidth, render.StringWidth(word))
			}
			if word == "" {
				continue
			}
			switch {
			case line.Len() == 0:
				line.WriteString(word)
			case render.StringWidth(line.String())+1+render.StringWidth(word) <= effectiveWidth:
				line.WriteByte(' ')
				line.WriteString(word)
			default:
				flush()
				line.WriteString(word)
			}
		}
		flush()
	}

	return lines
}

// reviewStateText renders a patent's review state within the pane's project,
// or a note when the patent is not a member of that project.
func detailReviewStateText(state domain.ReviewState) string {
	if state == "" {
		return "not in project"
	}
	return string(state)
}

func detailStyledReviewStateText(theme render.Theme, state domain.ReviewState) string {
	if state == "" {
		return theme.Dim.Render("not in project")
	}
	glyph := theme.ReviewStateGlyph(state)
	name := string(state)
	var styledName string
	switch state {
	case domain.ReviewStateUnknown:
		styledName = theme.Dim.Render(name)
	case domain.ReviewStateUnderReview:
		styledName = theme.Warn.Render(name)
	case domain.ReviewStateActive:
		styledName = theme.OK.Render(name)
	case domain.ReviewStateIgnored:
		styledName = theme.Dim.Render(name)
	case domain.ReviewStateDeleted:
		styledName = theme.Error.Render(name)
	default:
		styledName = name
	}
	return glyph + "  " + styledName
}

func detailFetchStateText(theme render.Theme, state domain.FetchState) string {
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

func reviewStateText(theme render.Theme, state domain.ReviewState) string {
	if state == "" {
		return "not in project"
	}
	return theme.ReviewStateGlyph(state)
}

func styledReviewStateText(theme render.Theme, state domain.ReviewState) string {
	text := reviewStateText(theme, state)
	switch state {
	case domain.ReviewStateUnknown:
		return theme.Dim.Render(text)
	case domain.ReviewStateUnderReview:
		return theme.Warn.Render(text)
	case domain.ReviewStateActive:
		return theme.OK.Render(text)
	case domain.ReviewStateIgnored:
		return theme.Dim.Render(text)
	case domain.ReviewStateDeleted:
		return theme.Error.Render(text)
	default:
		return text
	}
}

func fetchStateText(theme render.Theme, state domain.FetchState) string {
	text := theme.FetchStateGlyph(state)
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

func detailClassificationsText(classifications []string) string {
	if len(classifications) <= detailClassificationLimit {
		return strings.Join(classifications, ", ")
	}
	shown := strings.Join(classifications[:detailClassificationLimit], ", ")
	return fmt.Sprintf("%s (+%d more; press K)", shown, len(classifications)-detailClassificationLimit)
}

func detailIDSText(theme render.Theme, entry *domain.IDSEntry) string {
	if entry == nil {
		return theme.IDSEntryNoneGlyph() + " none"
	}
	if entry.Project == "" || entry.Patent.IsZero() {
		return "—"
	}
	parts := []string{idsStatusDisplayText(theme, entry.Status)}
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

func (d *Detail) Patent() domain.Patent           { return d.patent }
func (d *Detail) IsCursorOnAssignee() bool        { return d.page.Cursor() == d.assigneeLine }
func (d *Detail) IsCursorOnClassifications() bool { return d.page.Cursor() == d.classificationLine }
func (d *Detail) IsCursorOnInventors() bool       { return d.page.Cursor() == d.inventorLine }

// ResolveCursorRelation checks if the cursor is currently hovering over a relation count line
// and returns the corresponding RelationKind if it is.
func (d *Detail) ResolveCursorRelation() (domain.RelationKind, bool) {
	cursor := d.page.Cursor()
	for _, a := range d.jump.JumpAnchors() {
		if a.Line == cursor {
			switch a.Label {
			case detailLabelCitations:
				return domain.RelationCites, true
			case detailLabelCitedBy:
				return domain.RelationCitedBy, true
			case detailLabelParents:
				return domain.RelationParent, true
			case detailLabelChildren:
				return domain.RelationChild, true
			}
		}
	}
	return "", false
}

func (d *Detail) PatentNumber() domain.PatentNumber { return d.number }

// ResolveCursorUSPTOXML reports the USPTO XML kind whose URL row the cursor
// is currently hovering, or false when the cursor is not on a PGPub/Grant URL
// line. The label rows (e.g. "PGPub XML") are intentionally not matched —
// only the URL value (which may wrap across multiple lines) triggers a fetch.
func (d *Detail) ResolveCursorUSPTOXML() (proto.USPTOXMLKind, bool) {
	cursor := d.page.Cursor()
	if d.pgpubURLLine >= 0 && cursor >= d.pgpubURLLine && cursor <= d.pgpubURLLineEnd {
		return proto.USPTOXMLKindPGPub, true
	}
	if d.grantURLLine >= 0 && cursor >= d.grantURLLine && cursor <= d.grantURLLineEnd {
		return proto.USPTOXMLKindGrant, true
	}
	return "", false
}
