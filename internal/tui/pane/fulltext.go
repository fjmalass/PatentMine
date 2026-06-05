package pane

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
const disclosureLocator = "Disclosure"

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

	// jump mode
	jump        *JumpController
	keymapBound []rune // keys reserved by the keymap, kept off jump anchors
	logger      *slog.Logger
}

// WithLogger attaches a logger so the pane can persist fetch errors.
func (f *FullText) WithLogger(l *slog.Logger) *FullText { f.logger = l; return f }

func (f *FullText) log() *slog.Logger {
	if f.logger != nil {
		return f.logger
	}
	return slog.Default()
}

// NewFullText builds a full-text viewer for one patent.
func NewFullText(client *rpc.Client, theme render.Theme, number domain.PatentNumber, project domain.ProjectID, boundLetters []rune) *FullText {
	f := &FullText{
		client:      client,
		theme:       theme,
		number:      number,
		project:     project,
		page:        render.NewPaginator(10),
		loading:     true,
		keymapBound: boundLetters,
		jump:        NewJumpController(),
	}
	f.computeJumpKeys(boundLetters)
	f.handlers = map[command.ID]cmdHandler{
		command.NavDown: func(inv Invocation) tea.Cmd {
			if f.jump.Active && len(f.jump.Anchors) > 0 {
				f.page.ScrollTo(f.nextAnchorLine())
			} else {
				f.move(inv.Repeat, 1)
			}
			return nil
		},
		command.NavUp: func(inv Invocation) tea.Cmd {
			if f.jump.Active && len(f.jump.Anchors) > 0 {
				f.page.ScrollTo(f.prevAnchorLine())
			} else {
				f.move(inv.Repeat, -1)
			}
			return nil
		},
		command.NavPageDown:       func(Invocation) tea.Cmd { f.page.ScrollTo(f.page.Cursor() + f.page.PageSize()); return nil },
		command.NavPageUp:         func(Invocation) tea.Cmd { f.page.ScrollTo(f.page.Cursor() - f.page.PageSize()); return nil },
		command.NavTop:            func(inv Invocation) tea.Cmd { f.page.NavTop(inv.Repeat); return nil },
		command.NavBottom:         func(inv Invocation) tea.Cmd { f.page.NavBottom(inv.Repeat); return nil },
		command.SelectVisual:      func(Invocation) tea.Cmd { return f.toggleVisual() },
		command.CopyYank:          func(Invocation) tea.Cmd { return f.copyYank(false) },
		command.CopyYankMeta:      func(Invocation) tea.Cmd { return f.copyYank(true) },
		command.CopyAll:           func(Invocation) tea.Cmd { return f.copyAll() },
		command.FindOpen:          func(Invocation) tea.Cmd { f.find.open(""); return nil },
		command.FindNext:          func(Invocation) tea.Cmd { return f.gotoMatch(1) },
		command.FindPrev:          func(Invocation) tea.Cmd { return f.gotoMatch(-1) },
		command.FullTextStageNext: func(Invocation) tea.Cmd { return f.cycleStage(1) },
		command.FullTextStagePrev: func(Invocation) tea.Cmd { return f.cycleStage(-1) },
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

	// Reserve the last row for the find bar while a search is open.
	bodyH := max(h, 1)
	if f.find.active {
		bodyH = max(h-1, 1)
	}
	f.page.SetTotal(len(f.lines))
	f.page.SetPageSize(bodyH)
	start, end := f.page.Window()
	cur := f.page.Cursor()
	maxLines := len(f.lines)

	out := make([]string, 0, end-start+1)
	for i := start; i < end; i++ {
		line := f.lines[i].text
		gutter := render.LineGutter(f.theme.Dim, i+1, maxLines)

		isCursor := i == cur
		inVis := f.visualMode && f.inVisualRange(i)
		switch {
		case inVis, isCursor, f.isMatchLine(i):
			plain := gutter + stripANSI(line)
			out = append(out, f.theme.Selected.Render(render.Pad(plain, w)))
		default:
			out = append(out, gutter+line)
		}
	}
	if f.find.active {
		out = append(out, f.find.view(w, f.theme, len(f.matches)))
	}
	return strings.Join(out, "\n")
}

// render builds f.lines and f.anchors for body width w. It is idempotent for a
// given width, so View and the copy/note helpers all see the same layout.
func (f *FullText) render(w int) {
	if w < 1 {
		w = 1
	}
	if f.lines != nil && f.bodyW == w {
		return
	}
	f.bodyW = w
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
	add(f.theme.Dim.Render("  V: select  y: copy  Y: copy+info  g y: copy all  /: find  n/N: match  a/A: notes  ;: jump"), "")

	// Claims.
	for i, claim := range f.fullText.Claims {
		label := fmt.Sprintf("Claim %d", claim.Number)
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
			locator := paragraphLocator(para, i)
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

// paragraphLocator returns the locator string for a disclosure paragraph.
func paragraphLocator(p domain.DescriptionParagraph, index int) string {
	if p.Number != "" {
		return disclosureLocator + " ¶" + p.Number
	}
	return fmt.Sprintf("%s ¶%d", disclosureLocator, index+1)
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
		if fmt.Sprintf("Claim %d", c.Number) == locator {
			return c.Text
		}
	}
	for i, p := range f.fullText.Paragraphs {
		if paragraphLocator(p, i) == locator {
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

// clearMatches drops the search highlight and navigation state.
func (f *FullText) clearMatches() {
	f.matches = nil
	f.matchIdx = 0
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
		labels = append(labels, fmt.Sprintf("Claim %d", c.Number))
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
	full := &domain.FullText{Number: number}
	for _, c := range res.Body.Claims {
		num := 0
		if n, err := strconv.Atoi(strings.TrimLeft(c.Number, "0")); err == nil {
			num = n
		}
		full.Claims = append(full.Claims, domain.ClaimSection{Number: num, Text: c.Text})
	}
	if res.Body.AbstractText != "" {
		full.Paragraphs = append(full.Paragraphs, domain.DescriptionParagraph{
			Number: "Abstract",
			Text:   strings.TrimSpace(res.Body.AbstractText),
		})
	}
	if d := strings.TrimSpace(res.Body.DescriptionText); d != "" {
		// Split by double newlines to populate separate paragraphs and headings
		for _, part := range strings.Split(d, "\n\n") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			num := ""
			text := part
			// If it starts with a paragraph number like "[0001] ", extract it
			if strings.HasPrefix(part, "[") {
				idx := strings.Index(part, "]")
				if idx > 1 {
					num = part[1:idx]
					text = strings.TrimSpace(part[idx+1:])
				}
			}
			full.Paragraphs = append(full.Paragraphs, domain.DescriptionParagraph{
				Number: num,
				Text:   text,
			})
		}
	}
	return full, res.SourcePath
}
