package pane

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/command"
	"patentmine/internal/crawl"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

var fullTextSeq atomic.Uint64

func nextFullTextID() uint64 {
	return fullTextSeq.Add(1)
}

// disclosureLocator is the locator prefix for description paragraphs.
const disclosureLocator = domain.DisclosureLocator

// Quicklist split geometry. The locator column is fixed-width so line numbers
// and snippets line up; the list itself takes a fraction of the body, clamped
// to [min,max] rows, and only splits when the body is tall enough to be useful.
const (
	quickListLocatorW = 18 // section-locator column width
	quickListFraction = 3  // list takes ~1/quickListFraction of the body
	quickListMinRows  = 4  // smallest the list shrinks to
	quickListMaxRows  = 10 // largest the list grows to
	quickListMinBody  = 8  // don't split a body shorter than this
	quickListDivider  = 1  // the divider rule between body and list
)

// fullTextFetchTimeout bounds an on-demand full-text fetch from Google,
// allowing for the polite rate-limiter wait plus the HTTP request itself.
const fullTextFetchTimeout = 40 * time.Second

// bodyLine is one rendered line of the full-text body together with the
// locator of the section it belongs to ("Claim 5", "Disclosure ¶0045", or ""
// for the header/footer).
type bodyLine struct {
	text    string
	locator string
}

// FullText shows the complete claims and disclosure text of one patent. It
// fetches the full text on-demand from Google Patents (not stored in the DB)
// and supports visual line selection, clipboard yank, and note accumulation.
type FullText struct {
	client   *rpc.Client
	theme    render.Theme
	number   domain.PatentNumber
	project  domain.ProjectID
	handlers map[command.ID]cmdHandler

	// loaded data
	loading        bool
	loadErr        string
	loadID         uint64
	patent         domain.Patent
	fullText       domain.FullText
	fallbackGoogle bool
	// sourceXMLPath is the local USPTO XML file the body was generated from
	// (incl. directory); empty for the Google fallback.
	sourceXMLPath string
	// stage is the life-cycle stage currently displayed (application /
	// publication / grant). Empty until the first load resolves the latest.
	stage domain.Stage

	// rendered body
	lines []bodyLine
	bodyW int

	// scrolling
	page render.Paginator

	// visual selection (line-based)
	visualMode   bool
	visualAnchor int // line where visual mode started

	// in-document search (vim-style `/`, `n`, `N`)
	find     findBar
	matches  []int // body-line indices matching find.input, ascending
	matchIdx int   // index into matches of the current match

	// collapse ("quicklist") mode: when on, render only the matching lines.
	collapsed         bool
	renderedCollapsed bool  // collapse state baked into the current f.lines
	collapseSrc       []int // for each collapsed line, its index in the full body
	pendingCursorLine int   // full-body line to re-centre on after the next render (-1 = none)

	// open-at: seed a search and a target section when the viewer is opened from
	// the global full-text search. Consumed on the first successful load/render.
	initialQuery      string
	pendingLocator    string
	pendingOccurrence int // which match within pendingLocator to land on

	// quicklist: a persistent bottom split listing the search matches. listFocus
	// routes navigation to the list (the body follows the highlighted match).
	listOpen  bool
	listFocus bool
	listPage  render.Paginator

	// jump mode
	jump        *JumpController
	keymapBound []rune // keys reserved by the keymap, kept off jump anchors
	logger      *slog.Logger
}

// WithLogger attaches a logger so the pane can persist fetch errors.
func (f *FullText) WithLogger(l *slog.Logger) *FullText { f.logger = l; return f }

// OpenAt seeds the viewer to open with a live search (query) and land on a
// specific match — the occurrence-th hit within the section named by locator.
// Used when jumping in from the global full-text search; consumed on the first
// successful load.
func (f *FullText) OpenAt(query, locator string, occurrence int) *FullText {
	f.initialQuery = strings.TrimSpace(query)
	f.pendingLocator = locator
	f.pendingOccurrence = occurrence
	return f
}

// JumpToOccurrence re-targets an already-loaded viewer onto the occurrence-th
// match within locator, without reloading. Used by the search dock when the
// next result is in the same patent as the current preview.
func (f *FullText) JumpToOccurrence(locator string, occurrence int) {
	f.pendingLocator = locator
	f.pendingOccurrence = occurrence
}

func (f *FullText) log() *slog.Logger {
	if f.logger != nil {
		return f.logger
	}
	return slog.Default()
}

// NewFullText builds a full-text viewer for one patent.
func NewFullText(client *rpc.Client, theme render.Theme, number domain.PatentNumber, project domain.ProjectID, boundLetters []rune) *FullText {
	f := &FullText{
		client:            client,
		theme:             theme,
		number:            number,
		project:           project,
		page:              render.NewPaginator(10),
		listPage:          render.NewPaginator(10),
		loading:           true,
		keymapBound:       boundLetters,
		jump:              NewJumpController(),
		pendingCursorLine: -1,
	}
	f.computeJumpKeys(boundLetters)
	f.handlers = map[command.ID]cmdHandler{
		command.NavDown: func(inv Invocation) tea.Cmd {
			switch {
			case f.listFocused():
				f.listMove(inv.Repeat, 1)
			case f.jump.Active && len(f.jump.Anchors) > 0:
				f.page.ScrollTo(f.nextAnchorLine())
			default:
				f.move(inv.Repeat, 1)
			}
			return nil
		},
		command.NavUp: func(inv Invocation) tea.Cmd {
			switch {
			case f.listFocused():
				f.listMove(inv.Repeat, -1)
			case f.jump.Active && len(f.jump.Anchors) > 0:
				f.page.ScrollTo(f.prevAnchorLine())
			default:
				f.move(inv.Repeat, -1)
			}
			return nil
		},
		command.NavPageDown: func(Invocation) tea.Cmd {
			if f.listFocused() {
				f.listPage.PageDown()
				f.syncBodyToList()
				return nil
			}
			f.page.ScrollTo(f.page.Cursor() + f.page.PageSize())
			return nil
		},
		command.NavPageUp: func(Invocation) tea.Cmd {
			if f.listFocused() {
				f.listPage.PageUp()
				f.syncBodyToList()
				return nil
			}
			f.page.ScrollTo(f.page.Cursor() - f.page.PageSize())
			return nil
		},
		command.NavTop: func(inv Invocation) tea.Cmd {
			if f.listFocused() {
				f.listPage.Top()
				f.syncBodyToList()
				return nil
			}
			f.page.NavTop(inv.Repeat)
			return nil
		},
		command.NavBottom: func(inv Invocation) tea.Cmd {
			if f.listFocused() {
				f.listPage.Bottom()
				f.syncBodyToList()
				return nil
			}
			f.page.NavBottom(inv.Repeat)
			return nil
		},
		command.SelectVisual: func(Invocation) tea.Cmd { return f.toggleVisual() },
		command.CopyYank:     func(Invocation) tea.Cmd { return f.copyYank(false) },
		command.CopyYankMeta: func(Invocation) tea.Cmd { return f.copyYank(true) },
		command.CopyAll:      func(Invocation) tea.Cmd { return f.copyAll() },
		command.FindOpen: func(Invocation) tea.Cmd {
			// Always search the full body; re-collapse with z once there are matches.
			if f.collapsed {
				f.collapsed = false
				f.lines = nil
			}
			f.find.open("")
			return nil
		},
		command.FindNext:          func(Invocation) tea.Cmd { return f.gotoMatch(1) },
		command.FindPrev:          func(Invocation) tea.Cmd { return f.gotoMatch(-1) },
		command.FullTextStageNext: func(Invocation) tea.Cmd { return f.cycleStage(1) },
		command.FullTextStagePrev: func(Invocation) tea.Cmd { return f.cycleStage(-1) },
		command.FullTextCollapse:  func(Invocation) tea.Cmd { return f.toggleCollapse() },
		command.FullTextQuickList: func(Invocation) tea.Cmd { return f.toggleQuickList() },
		command.NoteAdd:           func(Invocation) tea.Cmd { return f.noteAdd() },
		command.NoteOpen:          func(Invocation) tea.Cmd { return f.noteOpen() },
		command.Refresh:           func(Invocation) tea.Cmd { f.loading = true; return f.reload() },
		command.FetchUSPTOGrant: func(Invocation) tea.Cmd {
			if f.number.Country == "US" {
				f.loading = true
				return FetchUSPTOXMLInteractiveCmd(f.client, f.number, proto.USPTOXMLKindGrant)
			}
			return nil
		},
	}
	return f
}

func (f *FullText) Scope() command.Scope { return command.ScopeFullText }

func (f *FullText) Title() string { return "Full text · " + f.number.String() }

func (f *FullText) Init() tea.Cmd { return f.reload() }

// reload fetches both the patent record (via RPC) and full text (from Google).
func (f *FullText) reload() tea.Cmd {
	return f.load()
}

func (f *FullText) load() tea.Cmd {
	client, number, project, wantStage := f.client, f.number, f.project, f.stage
	requestID := nextFullTextID()
	f.loadID = requestID
	return func() tea.Msg {
		start := time.Now()

		// Load patent attrs from daemon
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		var res proto.PatentResult
		err := client.Call(ctx, proto.MethodPatentGet,
			proto.PatentGetParams{Number: number, Project: project}, &res)
		cancel()

		patent := res.Patent

		// Resolve which life-cycle stage to show: the requested one, else the
		// record's latest. The stage selects the document number to fetch and
		// the USPTO XML kind (grant vs pgpub).
		stage := wantStage
		if stage == "" {
			if latest, ok := patent.LatestDocument(); ok {
				stage = latest.Stage
			}
		}
		fetchNumber, kind := stageTarget(patent, stage, number)

		// Try the USPTO-parsed body first when one has been ingested; fall
		// back to Google's full-text fetch when nothing is on hand locally.
		var fullText *domain.FullText
		fallbackGoogle := false
		sourceXMLPath := ""
		if err == nil {
			fullText, sourceXMLPath = fetchUSPTOFullText(client, fetchNumber, kind)
			if fullText == nil {
				fetchCtx, fetchCancel := context.WithTimeout(context.Background(), fullTextFetchTimeout)
				fetched, fetchErr := crawl.FetchFullText(fetchCtx, fetchNumber)
				fetchCancel()
				if fetchErr == nil {
					fullText = fetched
					if fetchNumber.Country == "US" {
						fallbackGoogle = true
					}
				} else {
					err = fetchErr
				}
			}
		}

		return FullTextLoadedMsg{
			RequestID:      requestID,
			Number:         number,
			FullText:       fullText,
			Patent:         patent,
			Duration:       time.Since(start),
			Err:            err,
			FallbackGoogle: fallbackGoogle,
			SourceXMLPath:  sourceXMLPath,
			Stage:          stage,
		}
	}
}

// stageTarget returns the document number and USPTO XML kind to fetch for a
// given stage. It falls back to the record number (then the pane's own number)
// when the record has no document for that stage. Grant maps to the grant XML;
// publication and application both resolve to the pre-grant publication body.
func stageTarget(p domain.Patent, stage domain.Stage, fallback domain.PatentNumber) (domain.PatentNumber, proto.USPTOXMLKind) {
	num := fallback
	if doc, ok := p.DocumentFor(stage); ok && !doc.Number.IsZero() {
		num = doc.Number
	} else if !p.Number.IsZero() {
		num = p.Number
	}
	kind := proto.USPTOXMLKindPGPub
	if stage == domain.StageGrant {
		kind = proto.USPTOXMLKindGrant
	}
	return num, kind
}

// availableStages returns the record's documents in life-cycle order
// (application → publication → grant), giving the stage switcher and header a
// stable sequence.
func (f *FullText) availableStages() []domain.Document {
	order := []domain.Stage{domain.StageApplication, domain.StagePublication, domain.StageGrant}
	out := make([]domain.Document, 0, len(order))
	for _, st := range order {
		if doc, ok := f.patent.DocumentFor(st); ok {
			out = append(out, doc)
		}
	}
	return out
}

// stageNumber returns the document number for the active stage, for display.
func (f *FullText) stageNumber() domain.PatentNumber {
	if doc, ok := f.patent.DocumentFor(f.stage); ok {
		return doc.Number
	}
	return f.number
}

// stageLabel renders a human-readable stage label with its document number.
func stageLabel(stage domain.Stage, number domain.PatentNumber) string {
	name := "Latest"
	switch stage {
	case domain.StageApplication:
		name = "Application"
	case domain.StagePublication:
		name = "Publication"
	case domain.StageGrant:
		name = "Grant"
	}
	if number.IsZero() {
		return name
	}
	return name + " (" + number.String() + ")"
}

// cycleStage moves the active stage by dir among the record's available stages
// (wrapping) and reloads the body for the new stage. It is a no-op when the
// record carries fewer than two stages.
func (f *FullText) cycleStage(dir int) tea.Cmd {
	stages := f.availableStages()
	if len(stages) < 2 {
		return nil
	}
	cur := 0
	for i, d := range stages {
		if d.Stage == f.stage {
			cur = i
			break
		}
	}
	next := (cur + dir + len(stages)) % len(stages)
	if stages[next].Stage == f.stage {
		return nil
	}
	f.stage = stages[next].Stage
	f.loading = true
	f.clearMatches()
	return f.reload()
}

// toggleCollapse switches the body between the full document and a collapsed
// "quicklist" of only the matching lines. Entering collapse needs an active
// search with at least one match; the cursor is carried onto the nearest match
// and carried back to that match's real line when the body is expanded again.
func (f *FullText) toggleCollapse() tea.Cmd {
	if !f.collapsed {
		if len(f.matches) == 0 {
			f.recomputeMatches()
		}
		if len(f.matches) == 0 {
			if strings.TrimSpace(f.find.input) == "" {
				return status(text.StatusNoPatentSelected, false, "press / to search, then z to collapse")
			}
			return status(text.StatusNoPatentSelected, false, "no matches for "+f.find.input)
		}
		f.pendingCursorLine = f.page.Cursor() // expanded: cursor is a full-body line
		f.collapsed = true
	} else {
		f.pendingCursorLine = f.cursorFullLine()
		f.collapsed = false
	}
	f.lines = nil // force render to rebuild for the new mode
	return nil
}

// cursorFullLine maps the cursor to its line index in the full (uncollapsed)
// body, so the position survives a collapse toggle.
func (f *FullText) cursorFullLine() int {
	cur := f.page.Cursor()
	if f.collapsed && cur >= 0 && cur < len(f.collapseSrc) {
		return f.collapseSrc[cur]
	}
	return cur
}

// applyPendingCursor re-centres the cursor after a render that swapped the
// display list (a collapse toggle). The stored target is a full-body line; in
// collapse mode it resolves to the first match at or after that line. It also
// re-syncs matchIdx so n/N continue from the cursor.
func (f *FullText) applyPendingCursor() {
	if f.pendingCursorLine < 0 {
		return
	}
	target := f.pendingCursorLine
	if f.collapsed {
		target = 0
		for i, src := range f.collapseSrc {
			if src >= f.pendingCursorLine {
				target = i
				break
			}
		}
	}
	f.page.ScrollTo(target)
	f.matchIdx = 0
	for j, m := range f.matches {
		if m >= f.page.Cursor() {
			f.matchIdx = j
			break
		}
	}
	f.pendingCursorLine = -1
}

// listFocused reports whether keyboard navigation is currently routed to the
// quicklist split rather than the body.
func (f *FullText) listFocused() bool { return f.listOpen && f.listFocus }

// toggleQuickList opens or closes the bottom quicklist split. Opening requires
// an active search with matches; the list takes focus and the body follows the
// current match.
func (f *FullText) toggleQuickList() tea.Cmd {
	if f.listOpen {
		f.listOpen = false
		f.listFocus = false
		return nil
	}
	if len(f.matches) == 0 {
		f.recomputeMatches()
	}
	if len(f.matches) == 0 {
		if strings.TrimSpace(f.find.input) == "" {
			return status(text.StatusNoPatentSelected, false, "press / to search, then ctrl+q for the match list")
		}
		return status(text.StatusNoPatentSelected, false, "no matches for "+f.find.input)
	}
	f.listOpen = true
	f.listFocus = true
	f.listPage.SetTotal(len(f.matches))
	f.listPage.ScrollTo(f.matchIdx)
	f.syncBodyToList()
	return nil
}

// listMove moves the quicklist cursor by repeat in dir and scrolls the body to
// the newly-focused match.
func (f *FullText) listMove(repeat, dir int) {
	for i := 0; i < max(repeat, 1); i++ {
		if dir > 0 {
			f.listPage.MoveDown(1)
		} else {
			f.listPage.MoveUp(1)
		}
	}
	f.syncBodyToList()
}

// syncBodyToList scrolls the body to the match the quicklist cursor points at,
// keeping matchIdx (the active-match highlight) in step.
func (f *FullText) syncBodyToList() {
	if len(f.matches) == 0 {
		return
	}
	i := f.listPage.Cursor()
	if i < 0 || i >= len(f.matches) {
		return
	}
	f.matchIdx = i
	f.page.ScrollTo(f.matches[i])
}

func (f *FullText) Command(id command.ID, inv Invocation) (Pane, tea.Cmd) {
	if handler, ok := f.handlers[id]; ok {
		return f, handler(inv)
	}
	return f, nil
}

func (f *FullText) Handles() []command.ID { return handlerIDs(f.handlers) }

func (f *FullText) Selection() (domain.PatentNumber, bool) {
	return f.number, true
}

func (f *FullText) Update(msg tea.Msg) (Pane, tea.Cmd) {
	switch m := msg.(type) {
	case FullTextLoadedMsg:
		if m.RequestID != f.loadID {
			return f, nil
		}
		f.loading = false
		if m.Err != nil {
			f.loadErr = m.Err.Error()
			f.log().Error("full text load failed", slog.String("number", f.number.String()), slog.String("error", m.Err.Error()))
			return f, nil
		}
		f.loadErr = ""
		f.patent = m.Patent
		f.fallbackGoogle = m.FallbackGoogle
		f.sourceXMLPath = m.SourceXMLPath
		f.stage = m.Stage
		if m.FullText != nil {
			f.fullText = *m.FullText
		} else {
			f.fullText = domain.FullText{}
		}
		f.computeJumpKeys(f.keymapBound)
		f.clearMatches()
		f.pendingCursorLine = -1
		f.listOpen = false
		f.listFocus = false
		if f.initialQuery != "" {
			// Opened from the global search: run the same query so the body
			// highlights matches and n/N work; pendingLocator lands the cursor.
			f.find.input = f.initialQuery
			f.initialQuery = ""
		}
		f.lines = nil
		f.page.Top()
	case USPTOXMLFetchedMsg:
		// Automatically reload the pane if the active USPTO patent XML was fetched successfully
		if m.Number == f.number && m.Err == "" {
			f.loading = true
			return f, f.reload()
		}
	}
	return f, nil
}

// View implements Pane.
func (f *FullText) View(w, h int) string {
	switch {
	case f.loading && f.patent.Number.Serial == "":
		return f.theme.Dim.Render("loading full text for " + f.number.String() + "…")
	case f.loadErr != "":
		return f.theme.Error.Render("error: " + f.loadErr)
	}

	// ── line-number gutter ──────────────────────────────────────
	// Use a generous fixed estimate for gutter width so we never need
	// a second render pass.  LineGutter computes the exact width so
	// numbers are right-aligned; the small gap on the right is harmless.
	const gutterEstimate = 6 // " 99999 " → up to 99_999 lines
	contentW := w - gutterEstimate
	if contentW < 10 {
		contentW = w
	}
	f.render(contentW)

	// Resolve a pending open-at target now that the body (and its line locators)
	// exist: land on the occurrence-th matching line within the section, falling
	// back to the section header, then the first match anywhere.
	if f.pendingLocator != "" {
		want, occ := f.pendingLocator, f.pendingOccurrence
		f.pendingLocator = ""
		f.pendingOccurrence = 0
		target, seen := -1, 0
		for _, idx := range f.matches {
			if f.lines[idx].locator == want {
				if seen == occ {
					target = idx
					break
				}
				seen++
			}
		}
		if target < 0 {
			for i, ln := range f.lines {
				if ln.locator == want {
					target = i
					break
				}
			}
		}
		if target < 0 && len(f.matches) > 0 {
			target = f.matches[0]
		}
		f.pendingCursorLine = target
	}

	// Lay out the rows: an optional find bar on the very last row, an optional
	// bottom quicklist split above it, and the body filling the rest.
	availH := max(h, 1)
	if f.find.active {
		availH = max(h-1, 1)
	}
	listH := 0
	if f.listOpen && len(f.matches) > 0 && availH >= quickListMinBody {
		listH = min(max(availH/quickListFraction, quickListMinRows), quickListMaxRows)
	}
	bodyH := availH
	if listH > 0 {
		bodyH = max(availH-listH-quickListDivider, 1)
	}

	f.page.SetTotal(len(f.lines))
	f.page.SetPageSize(bodyH)
	f.applyPendingCursor()
	start, end := f.page.Window()
	cur := f.page.Cursor()
	maxLines := len(f.lines)

	out := make([]string, 0, availH+1)
	query := strings.TrimSpace(f.find.input)
	for i := start; i < end; i++ {
		line := f.lines[i].text
		gutter := render.LineGutter(f.theme.Dim, i+1, maxLines)

		isCursor := i == cur
		inVis := f.visualMode && f.inVisualRange(i)
		switch {
		case f.isMatchLine(i) && query != "":
			// Paint the row as selected and the matched text within it; the
			// active match (always the cursor row after n/N) gets the brighter
			// highlight so it stands out from the other matches.
			hl := f.theme.Match
			if isCursor {
				hl = f.theme.MatchCurrent
			}
			out = append(out, highlightRow(gutter, stripANSI(line), query, f.theme.Selected, hl, w))
		case inVis, isCursor:
			plain := gutter + stripANSI(line)
			out = append(out, f.theme.Selected.Render(render.Pad(plain, w)))
		default:
			out = append(out, gutter+line)
		}
	}
	if listH > 0 {
		// Pad the body to a fixed height so the split sits flush at the bottom.
		for len(out) < bodyH {
			out = append(out, "")
		}
		out = append(out, f.listDivider(w))
		out = append(out, f.listRows(w, listH)...)
	}
	if f.find.active {
		out = append(out, f.find.view(w, f.theme, len(f.matches)))
	}
	return strings.Join(out, "\n")
}

// listDivider renders the quicklist split's header rule with the match count
// and the focus-dependent key hints.
func (f *FullText) listDivider(w int) string {
	label := fmt.Sprintf("Matches %d/%d", f.matchIdx+1, len(f.matches))
	if q := strings.TrimSpace(f.find.input); q != "" {
		label += " · /" + q
	}
	if f.listFocus {
		label += "  [tab] body  [enter] read  [q] close"
	} else {
		label += "  [tab] list  [esc] close"
	}
	line := "─ " + label + " "
	if pad := w - render.StringWidth(line); pad > 0 {
		line += strings.Repeat("─", pad)
	}
	return f.theme.Header.Render(render.Truncate(line, max(w, 1)))
}

// listRows renders the quicklist split's rows (one per match). When the body
// has focus the list tracks the active match; otherwise its own cursor leads.
func (f *FullText) listRows(w, h int) []string {
	f.listPage.SetTotal(len(f.matches))
	f.listPage.SetPageSize(h)
	// The active match (matchIdx) is the single source of truth — list focus
	// moves it via listMove, n/N move it directly — so the list cursor always
	// tracks it.
	f.listPage.ScrollTo(f.matchIdx)
	start, end := f.listPage.Window()
	cur := f.listPage.Cursor()
	rows := make([]string, 0, h)
	for i := start; i < end; i++ {
		idx := f.matches[i]
		locator := f.lines[idx].locator
		if locator == "" {
			locator = "—"
		}
		body := strings.TrimSpace(stripANSI(f.lines[idx].text))
		row := fmt.Sprintf("%-*s  L%-5d  %s", quickListLocatorW, render.Truncate(locator, quickListLocatorW), idx+1, body)
		switch {
		case i == cur && f.listFocus:
			rows = append(rows, f.theme.Selected.Render(render.Pad(row, w)))
		case i == cur:
			rows = append(rows, f.theme.Visual.Render(render.Pad(row, w)))
		default:
			rows = append(rows, f.theme.Row.Render(render.Truncate(row, w)))
		}
	}
	for len(rows) < h {
		rows = append(rows, "")
	}
	return rows
}

// render builds f.lines and f.anchors for body width w. It is idempotent for a
// given width, so View and the copy/note helpers all see the same layout.
func (f *FullText) render(w int) {
	if w < 1 {
		w = 1
	}
	if f.lines != nil && f.bodyW == w && f.renderedCollapsed == f.collapsed {
		return
	}
	f.bodyW = w
	f.renderedCollapsed = f.collapsed
	f.collapseSrc = nil
	f.lines = f.lines[:0]
	f.jump.ClearAnchors()

	add := func(rendered, locator string) {
		f.lines = append(f.lines, bodyLine{text: rendered, locator: locator})
	}

	// Patent attrs header.
	if f.fallbackGoogle {
		add(f.theme.Warn.Render("  [Showing Google Patents fallback version. Press . to fetch high-quality USPTO XML]"), "")
		add("", "")
	}

	metaLine := fmt.Sprintf("Patent #: %s", f.number.String())
	if f.patent.Title != "" {
		metaLine += " — " + f.patent.Title
	}
	add(f.theme.Header.Render(metaLine), "")
	if stages := f.availableStages(); len(stages) > 0 {
		stageLine := "  Stage: " + stageLabel(f.stage, f.stageNumber())
		if len(stages) > 1 {
			stageLine += "   [ / ] switch stage"
		}
		add(f.theme.Info.Render(stageLine), "")
	}
	if len(f.patent.Inventors) > 0 {
		names := make([]string, 0, len(f.patent.Inventors))
		for _, inv := range f.patent.Inventors {
			names = append(names, string(inv))
		}
		add(f.theme.Row.Render("  Inventors: "+strings.Join(names, ", ")), "")
	}
	if f.patent.Assignee != "" {
		add(f.theme.Row.Render("  Assignee: "+f.patent.Assignee), "")
	}
	if !f.patent.PublicationDate.IsZero() {
		add(f.theme.Row.Render("  Publication: "+f.patent.PublicationDate.Format(domain.DateLayout)), "")
	}
	if !f.patent.ExpirationDate.IsZero() {
		expText := f.patent.ExpirationDate.Format(domain.DateLayout)
		if f.patent.ExpirationSource != "" {
			expText += " (" + f.patent.ExpirationSource + ")"
		}
		add(f.theme.Row.Render("  Expiration: "+expText), "")
	}
	if !f.fallbackGoogle && f.sourceXMLPath != "" {
		add(f.theme.Dim.Render("  Source XML: "+f.sourceXMLPath), "")
	}
	add("", "")
	rule := f.theme.Dim.Render(strings.Repeat("─", max(w, 1)))
	add(rule, "")
	add(f.theme.Dim.Render("  V: select  y: copy  Y: copy+info  g y: copy all  /: find  n/N: match  z: collapse  ^q: match list  ^n: notes  ;: jump"), "")
	add(rule, "")

	// Claims.
	for i, claim := range f.fullText.Claims {
		label := claim.Locator()
		f.addSectionHeader(add, label, label)
		for _, line := range wrapText(claim.Text, max(w-2, 1)) {
			add(f.theme.Row.Render("  "+line), label)
		}
		if i < len(f.fullText.Claims)-1 {
			add("", label)
		}
	}

	// Disclosure paragraphs.
	if len(f.fullText.Paragraphs) > 0 {
		add("", "")
		f.addSectionHeader(add, disclosureLocator, disclosureLocator)
		for i, para := range f.fullText.Paragraphs {
			locator := para.Locator(i)
			tag := "¶ " + para.Number
			if para.Number == "" {
				tag = fmt.Sprintf("¶ %d", i+1)
			}
			add(f.theme.Warn.Render("  "+tag), locator)
			for _, line := range wrapText(para.Text, max(w-4, 1)) {
				add(f.theme.Row.Render("    "+line), locator)
			}
			if i < len(f.fullText.Paragraphs)-1 {
				add("", locator)
			}
		}
	}

	// Keep match indices in sync with the freshly-built lines.
	f.recomputeMatches()

	// In collapse mode, drop everything except the matching lines so the body
	// reads as a quicklist. Falls back to the full body when nothing matches.
	if f.collapsed && len(f.matches) > 0 {
		f.applyCollapse()
	}
}

// applyCollapse filters f.lines down to the matched lines, remembering each
// line's original index in collapseSrc so the cursor can be mapped back to the
// full body when collapse is turned off. After collapsing, every visible line
// is a match, so f.matches becomes the full display range and jump anchors
// (which point at now-hidden section headers) are cleared.
func (f *FullText) applyCollapse() {
	full := f.lines
	matched := f.matches
	lines := make([]bodyLine, len(matched))
	src := make([]int, len(matched))
	idents := make([]int, len(matched))
	for k, idx := range matched {
		lines[k] = full[idx]
		src[k] = idx
		idents[k] = k
	}
	f.lines = lines
	f.collapseSrc = src
	f.matches = idents
	if f.matchIdx >= len(idents) {
		f.matchIdx = 0
	}
	f.jump.ClearAnchors()
}

// addSectionHeader appends a section header line and registers a jump anchor
// for it when a jump key is assigned.
func (f *FullText) addSectionHeader(add func(string, string), label, locator string) {
	key := f.jumpKey(label)
	if key != 0 {
		f.jump.AddAnchor(label, "", len(f.lines), false, key)
		add(f.theme.Warn.Render(fmt.Sprintf("[%s] %s", string(key), label)), locator)
		return
	}
	add(f.theme.Header.Render(label), locator)
}

// move scrolls and updates visual selection.
func (f *FullText) move(repeat int, dir int) {
	for i := 0; i < max(repeat, 1); i++ {
		if dir > 0 {
			f.page.MoveDown(1)
		} else {
			f.page.MoveUp(1)
		}
	}
}

// toggleVisual toggles visual line selection mode.
func (f *FullText) toggleVisual() tea.Cmd {
	if f.visualMode {
		f.visualMode = false
		return nil
	}
	f.visualMode = true
	f.visualAnchor = f.page.Cursor()
	return nil
}

// inVisualRange reports whether abs is within the visual selection.
func (f *FullText) inVisualRange(abs int) bool {
	lo := min(f.visualAnchor, f.page.Cursor())
	hi := max(f.visualAnchor, f.page.Cursor())
	return abs >= lo && abs <= hi
}

// selection returns the plain text of the current selection (or the current
// section when nothing is visually selected) and the locator where it was
// captured.
func (f *FullText) selection() (body string, locator string) {
	f.render(f.bodyWidth())
	if len(f.lines) == 0 {
		return "", ""
	}
	if f.visualMode {
		lo := max(min(f.visualAnchor, f.page.Cursor()), 0)
		hi := min(max(f.visualAnchor, f.page.Cursor()), len(f.lines)-1)
		if lo > hi {
			return "", ""
		}
		plain := make([]string, 0, hi-lo+1)
		for i := lo; i <= hi; i++ {
			plain = append(plain, stripANSI(f.lines[i].text))
		}
		return strings.TrimRight(strings.Join(plain, "\n"), "\n"), f.lines[lo].locator
	}
	// No visual selection — fall back to the section under the cursor.
	locator = f.locatorAt(f.page.Cursor())
	return f.sectionText(locator), locator
}

// locatorAt returns the section locator for the body line at index.
func (f *FullText) locatorAt(index int) string {
	if index < 0 || index >= len(f.lines) {
		return ""
	}
	return f.lines[index].locator
}

// sectionText returns the source text of the claim or paragraph named by locator.
func (f *FullText) sectionText(locator string) string {
	for _, c := range f.fullText.Claims {
		if c.Locator() == locator {
			return c.Text
		}
	}
	for i, p := range f.fullText.Paragraphs {
		if p.Locator(i) == locator {
			return p.Text
		}
	}
	return ""
}

// bodyWidth returns the last width render was called with, defaulting to 80.
func (f *FullText) bodyWidth() int {
	if f.bodyW > 0 {
		return f.bodyW
	}
	return 80
}

// copyYank builds the clipboard text. When meta is true the patent attrs
// header is prepended ("copy with patent info").
func (f *FullText) copyYank(meta bool) tea.Cmd {
	body, locator := f.selection()
	if strings.TrimSpace(body) == "" {
		return status(text.StatusNoPatentSelected, false, "nothing to copy")
	}
	var b strings.Builder
	if meta {
		b.WriteString(patentMeta(f.patent, f.number))
	}
	if locator != "" {
		b.WriteString(locator + "\n")
	}
	b.WriteString("captured " + time.Now().Format(time.RFC3339) + "\n")
	b.WriteString(strings.Repeat("─", 48) + "\n")
	b.WriteString(body)
	out := b.String()
	return func() tea.Msg { return CopyToClipboardMsg{Text: out} }
}

// copyAll builds the clipboard text for the entire document — all claims and
// disclosure paragraphs, with the patent attrs header — built from the source
// text rather than the wrapped/paginated view (vim `:%y`).
func (f *FullText) copyAll() tea.Cmd {
	doc := f.documentText()
	if strings.TrimSpace(doc) == "" {
		return status(text.StatusNoPatentSelected, false, "nothing to copy")
	}
	var b strings.Builder
	b.WriteString(patentMeta(f.patent, f.number))
	b.WriteString("captured " + time.Now().Format(time.RFC3339) + "\n")
	b.WriteString(strings.Repeat("─", 48) + "\n")
	b.WriteString(doc)
	out := b.String()
	return func() tea.Msg { return CopyToClipboardMsg{Text: out} }
}

// documentText assembles the full claims + disclosure body from the source
// text (unwrapped), the way it should read when pasted elsewhere.
func (f *FullText) documentText() string {
	var b strings.Builder
	for _, c := range f.fullText.Claims {
		fmt.Fprintf(&b, "Claim %d\n%s\n\n", c.Number, strings.TrimSpace(c.Text))
	}
	if len(f.fullText.Paragraphs) > 0 {
		b.WriteString(disclosureLocator + "\n\n")
		for i, p := range f.fullText.Paragraphs {
			tag := "¶ " + p.Number
			if p.Number == "" {
				tag = fmt.Sprintf("¶ %d", i+1)
			}
			fmt.Fprintf(&b, "%s\n%s\n\n", tag, strings.TrimSpace(p.Text))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// noteAdd adds the current selection (or section) to the notes buffer.
func (f *FullText) noteAdd() tea.Cmd {
	body, locator := f.noteSelection()
	if locator == "" {
		return status(text.StatusNoPatentSelected, false, "move cursor to a claim or paragraph first")
	}
	number := f.number
	captured := time.Now()
	return func() tea.Msg {
		return NoteAddMsg{Number: number, Locator: locator, Text: body, CapturedAt: captured}
	}
}

// noteSelection resolves the current note target. Disclosure notes use the
// active paragraph locator rather than the top-level Disclosure header.
func (f *FullText) noteSelection() (body string, locator string) {
	body, locator = f.selection()
	if f.visualMode || locator != disclosureLocator {
		return body, locator
	}
	if paraLocator := f.nearestParagraphLocator(f.page.Cursor()); paraLocator != "" {
		return f.sectionText(paraLocator), paraLocator
	}
	return body, locator
}

func (f *FullText) nearestParagraphLocator(index int) string {
	for i := index + 1; i < len(f.lines); i++ {
		locator := f.lines[i].locator
		if strings.HasPrefix(locator, disclosureLocator+" ") {
			return locator
		}
	}
	for i := index - 1; i >= 0; i-- {
		locator := f.lines[i].locator
		if strings.HasPrefix(locator, disclosureLocator+" ") {
			return locator
		}
	}
	return ""
}

// noteOpen shows the notes buffer overlay.
func (f *FullText) noteOpen() tea.Cmd {
	number, patent := f.number, f.patent
	return func() tea.Msg {
		return NoteOpenMsg{Number: number, Patent: patent}
	}
}

// --- Jump mode support ---

func (f *FullText) JumpAnchors() []render.JumpAnchor { return f.jump.JumpAnchors() }

func (f *FullText) JumpTo(line int) { f.page.ScrollTo(line) }

func (f *FullText) SetJumpActive(active bool) { f.jump.SetJumpActive(active) }

func (f *FullText) JumpActive() bool { return f.jump.JumpActive() }

func (f *FullText) HandleKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool) {
	if f.find.active {
		return f.handleFindKey(msg)
	}
	// While the quicklist split is open, Tab toggles focus body↔list, Esc
	// dismisses the panel, and (with the list focused) q closes / Enter drops
	// into the body. Navigation keys fall through to the keymap handlers, which
	// route to the list when it has focus.
	if f.listOpen {
		switch msg.String() {
		case "tab":
			f.listFocus = !f.listFocus
			if f.listFocus {
				f.listPage.SetTotal(len(f.matches))
				f.listPage.ScrollTo(f.matchIdx)
			}
			return f, nil, true
		case "esc":
			f.listOpen, f.listFocus = false, false
			return f, nil, true
		case "q":
			if f.listFocus {
				f.listOpen, f.listFocus = false, false
				return f, nil, true
			}
		case "enter":
			if f.listFocus {
				f.listFocus = false
				return f, nil, true
			}
		}
	}
	if !f.jump.JumpActive() {
		return f, nil, false
	}
	consumed := f.jump.HandleKey(msg, f.JumpTo)
	return f, nil, consumed
}

// handleFindKey routes raw keys into the find bar while it is open and keeps the
// match set in sync as the query changes.
func (f *FullText) handleFindKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool) {
	_, action := f.find.handleKey(msg)
	switch action {
	case "reload", "confirm":
		f.recomputeMatches()
		f.jumpToNearestMatch()
	case "cancel":
		f.clearMatches()
	}
	return f, nil, true
}

// recomputeMatches rebuilds the set of body lines containing the search query
// (case-insensitive), preserving the current match across width changes.
func (f *FullText) recomputeMatches() {
	query := strings.ToLower(strings.TrimSpace(f.find.input))
	if query == "" {
		f.clearMatches()
		return
	}
	prev := -1
	if f.matchIdx < len(f.matches) {
		prev = f.matches[f.matchIdx]
	}
	f.matches = f.matches[:0]
	for i, ln := range f.lines {
		if strings.Contains(strings.ToLower(stripANSI(ln.text)), query) {
			f.matches = append(f.matches, i)
		}
	}
	f.matchIdx = 0
	for j, line := range f.matches {
		if line >= prev {
			f.matchIdx = j
			break
		}
	}
}

// clearMatches drops the search highlight and navigation state. Collapse mode
// depends on having matches, so clearing them also expands the body (the next
// render rebuilds the full document because renderedCollapsed no longer agrees).
func (f *FullText) clearMatches() {
	f.matches = nil
	f.matchIdx = 0
	f.collapsed = false
	f.listOpen = false
	f.listFocus = false
}

// isMatchLine reports whether body line i is a current search match.
func (f *FullText) isMatchLine(i int) bool {
	return slices.Contains(f.matches, i)
}

// jumpToNearestMatch scrolls to the first match at or after the cursor (else the
// first match), used while typing in the find bar.
func (f *FullText) jumpToNearestMatch() {
	if len(f.matches) == 0 {
		return
	}
	cur := f.page.Cursor()
	for j, line := range f.matches {
		if line >= cur {
			f.matchIdx = j
			f.page.ScrollTo(line)
			return
		}
	}
	f.matchIdx = 0
	f.page.ScrollTo(f.matches[0])
}

// gotoMatch advances the current match by dir (+1 next, -1 previous), wrapping,
// and scrolls to it. With no matches it reports a status.
func (f *FullText) gotoMatch(dir int) tea.Cmd {
	if len(f.matches) == 0 {
		f.recomputeMatches()
	}
	if len(f.matches) == 0 {
		if strings.TrimSpace(f.find.input) == "" {
			return status(text.StatusNoPatentSelected, false, "press / to search")
		}
		return status(text.StatusNoPatentSelected, false, "no matches for "+f.find.input)
	}
	f.matchIdx = (f.matchIdx + dir + len(f.matches)) % len(f.matches)
	f.page.ScrollTo(f.matches[f.matchIdx])
	return nil
}

func (f *FullText) nextAnchorLine() int {
	cur := f.page.Cursor()
	anchors := f.jump.JumpAnchors()
	for _, a := range anchors {
		if a.Line > cur {
			return a.Line
		}
	}
	if len(anchors) > 0 {
		return anchors[0].Line
	}
	return 0
}

func (f *FullText) prevAnchorLine() int {
	cur := f.page.Cursor()
	anchors := f.jump.JumpAnchors()
	for i := len(anchors) - 1; i >= 0; i-- {
		if anchors[i].Line < cur {
			return anchors[i].Line
		}
	}
	if len(anchors) > 0 {
		return anchors[len(anchors)-1].Line
	}
	return 0
}

// computeJumpKeys assigns a stable single-key jump target to each claim and to
// the disclosure section header, avoiding conflict with the bound keymap.
func (f *FullText) computeJumpKeys(bound []rune) {
	labels := make([]string, 0, len(f.fullText.Claims)+1)
	for _, c := range f.fullText.Claims {
		labels = append(labels, c.Locator())
	}
	if len(f.fullText.Paragraphs) > 0 {
		labels = append(labels, disclosureLocator)
	}
	f.jump.Compute(labels, bound, nil, true)
}

func (f *FullText) jumpKey(label string) rune {
	return f.jump.JumpKey(label)
}

// --- Helpers ---

// patentMeta builds the patent attrs header for clipboard export.
func patentMeta(p domain.Patent, number domain.PatentNumber) string {
	var b strings.Builder
	sep := strings.Repeat("═", 48)
	b.WriteString(sep + "\n")
	b.WriteString(fmt.Sprintf("Patent #:     %s\n", number.String()))
	if p.Title != "" {
		b.WriteString(fmt.Sprintf("Title:        %s\n", p.Title))
	}
	if len(p.Inventors) > 0 {
		inventorStr := string(p.Inventors[0])
		if len(p.Inventors) > 1 {
			inventorStr += " et al. (" + fmt.Sprintf("%d", len(p.Inventors)) + ")"
		}
		b.WriteString(fmt.Sprintf("Inventors:    %s\n", inventorStr))
	}
	if p.Assignee != "" {
		b.WriteString(fmt.Sprintf("Assignee:     %s\n", p.Assignee))
	}
	for _, doc := range p.Documents {
		if doc.Stage == domain.StageApplication {
			dateStr := ""
			if !doc.Dated.IsZero() {
				dateStr = " (" + doc.Dated.Format(domain.DateLayout) + ")"
			}
			b.WriteString(fmt.Sprintf("Application #: %s%s\n", doc.Number.String(), dateStr))
			break
		}
	}
	if !p.PublicationDate.IsZero() {
		b.WriteString(fmt.Sprintf("Publication:  %s\n", p.PublicationDate.Format(domain.DateLayout)))
	}
	if !p.ExpirationDate.IsZero() {
		src := ""
		if p.ExpirationSource != "" {
			src = " (" + p.ExpirationSource + ")"
		}
		b.WriteString(fmt.Sprintf("Expiration:   %s%s\n", p.ExpirationDate.Format(domain.DateLayout), src))
	}
	b.WriteString(sep + "\n")
	return b.String()
}

// highlightRow renders one matched body line: the line-number gutter and text
// are painted with base (the selected-row background) while every
// case-insensitive occurrence of query is painted with hl so the matched text
// stands out within the row. The result is padded with base to fill width so
// the row background runs the full pane width.
func highlightRow(gutter, body, query string, base, hl lipgloss.Style, width int) string {
	if query == "" {
		return base.Render(render.Pad(gutter+body, width))
	}
	var b strings.Builder
	b.WriteString(base.Render(gutter))
	used := render.StringWidth(gutter)

	lowerBody := strings.ToLower(body)
	lowerQuery := strings.ToLower(query)
	for i := 0; i < len(body); {
		rel := strings.Index(lowerBody[i:], lowerQuery)
		if rel < 0 {
			seg := body[i:]
			b.WriteString(base.Render(seg))
			used += render.StringWidth(seg)
			break
		}
		if rel > 0 {
			seg := body[i : i+rel]
			b.WriteString(base.Render(seg))
			used += render.StringWidth(seg)
		}
		matched := body[i+rel : i+rel+len(lowerQuery)]
		b.WriteString(hl.Render(matched))
		used += render.StringWidth(matched)
		i += rel + len(lowerQuery)
	}

	if pad := width - used; pad > 0 {
		b.WriteString(base.Render(strings.Repeat(" ", pad)))
	}
	return b.String()
}

// stripANSI removes ANSI escape codes from a string.
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
				j++
			}
			if j < len(s) {
				i = j + 1
			} else {
				i = j
			}
		} else {
			out.WriteByte(s[i])
			i++
		}
	}
	return out.String()
}

func (f *FullText) PatentNumber() domain.PatentNumber { return f.number }

// fetchUSPTOFullText queries the daemon for a parsed USPTO body of the given
// kind and, when one is present, projects it into the FullText shape the pane
// already renders. It also returns the local XML file the body was generated
// from (incl. directory). A nil return means no XML of that kind has been
// ingested for this patent and the caller should fall back to the live Google
// fetch.
func fetchUSPTOFullText(client *rpc.Client, number domain.PatentNumber, kind proto.USPTOXMLKind) (*domain.FullText, string) {
	if client == nil {
		return nil, ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	var res proto.USPTOGrantBodyResult
	if err := client.Call(ctx, proto.MethodUSPTOGrantBody,
		proto.USPTOGrantBodyParams{Number: number, Kind: kind}, &res); err != nil {
		return nil, ""
	}
	if !res.Present {
		return nil, ""
	}
	full := domain.FullTextFromGrantBody(number, res.Body)
	return &full, res.SourcePath
}
