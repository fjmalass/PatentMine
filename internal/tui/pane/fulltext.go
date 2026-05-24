package pane

import (
	"context"
	"fmt"
	"log/slog"
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
	loading  bool
	loadErr  string
	loadID   uint64
	patent   domain.Patent
	fullText domain.FullText

	// rendered body
	lines []bodyLine
	bodyW int

	// scrolling
	page render.Paginator

	// visual selection (line-based)
	visualMode   bool
	visualAnchor int // line where visual mode started

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
		command.NavPageDown:  func(Invocation) tea.Cmd { f.page.ScrollTo(f.page.Cursor() + f.page.PageSize()); return nil },
		command.NavPageUp:    func(Invocation) tea.Cmd { f.page.ScrollTo(f.page.Cursor() - f.page.PageSize()); return nil },
		command.NavTop:       func(Invocation) tea.Cmd { f.page.Top(); return nil },
		command.NavBottom:    func(Invocation) tea.Cmd { f.page.Bottom(); return nil },
		command.SelectVisual: func(Invocation) tea.Cmd { return f.toggleVisual() },
		command.CopyYank:     func(Invocation) tea.Cmd { return f.copyYank(false) },
		command.CopyYankMeta: func(Invocation) tea.Cmd { return f.copyYank(true) },
		command.NoteAdd:      func(Invocation) tea.Cmd { return f.noteAdd() },
		command.NoteOpen:     func(Invocation) tea.Cmd { return f.noteOpen() },
		command.Refresh:      func(Invocation) tea.Cmd { f.loading = true; return f.reload() },
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
	client, number, project := f.client, f.number, f.project
	requestID := nextFullTextID()
	f.loadID = requestID
	return func() tea.Msg {
		start := time.Now()

		// Load patent metadata from daemon
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		var res proto.PatentResult
		err := client.Call(ctx, proto.MethodPatentGet,
			proto.PatentGetParams{Number: number, Project: project}, &res)
		cancel()

		patent := res.Patent

		// If patent metadata fails, still try to load full text
		var fullText *domain.FullText
		if err == nil {
			fetchCtx, fetchCancel := context.WithTimeout(context.Background(), fullTextFetchTimeout)
			fetched, fetchErr := crawl.FetchFullText(fetchCtx, number)
			fetchCancel()
			if fetchErr == nil {
				fullText = fetched
			} else {
				err = fetchErr
			}
		}

		return FullTextLoadedMsg{
			RequestID: requestID,
			Number:    number,
			FullText:  fullText,
			Patent:    patent,
			Duration:  time.Since(start),
			Err:       err,
		}
	}
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
		if m.FullText != nil {
			f.fullText = *m.FullText
		}
		f.computeJumpKeys(f.keymapBound)
		f.lines = nil
		f.page.Top()
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
	f.page.SetTotal(len(f.lines))
	f.page.SetPageSize(max(h, 1))
	start, end := f.page.Window()
	cur := f.page.Cursor()
	maxLines := len(f.lines)

	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		line := f.lines[i].text
		gutter := render.LineGutter(f.theme.Dim, i+1, maxLines)

		isCursor := i == cur
		inVis := f.visualMode && f.inVisualRange(i)
		switch {
		case inVis, isCursor:
			plain := gutter + stripANSI(line)
			out = append(out, f.theme.Selected.Render(render.Pad(plain, w)))
		default:
			out = append(out, gutter+line)
		}
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

	// Patent metadata header.
	metaLine := fmt.Sprintf("Patent #: %s", f.number.String())
	if f.patent.Title != "" {
		metaLine += " — " + f.patent.Title
	}
	add(f.theme.Header.Render(metaLine), "")
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
		add(f.theme.Row.Render("  Publication: "+f.patent.PublicationDate.Format("2006-01-02")), "")
	}
	if !f.patent.ExpirationDate.IsZero() {
		expText := f.patent.ExpirationDate.Format("2006-01-02")
		if f.patent.ExpirationSource != "" {
			expText += " (" + f.patent.ExpirationSource + ")"
		}
		add(f.theme.Row.Render("  Expiration: "+expText), "")
	}
	add("", "")
	add(f.theme.Dim.Render("  V: select  y: copy  Y: copy+info  n: notes  N: open notes  ;: jump"), "")

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

// copyYank builds the clipboard text. When meta is true the patent metadata
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
	if !f.jump.JumpActive() {
		return f, nil, false
	}
	consumed := f.jump.HandleKey(msg, f.JumpTo)
	return f, nil, consumed
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

// patentMeta builds the patent metadata header for clipboard export.
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
				dateStr = " (" + doc.Dated.Format("2006-01-02") + ")"
			}
			b.WriteString(fmt.Sprintf("Application #: %s%s\n", doc.Number.String(), dateStr))
			break
		}
	}
	if !p.PublicationDate.IsZero() {
		b.WriteString(fmt.Sprintf("Publication:  %s\n", p.PublicationDate.Format("2006-01-02")))
	}
	if !p.ExpirationDate.IsZero() {
		src := ""
		if p.ExpirationSource != "" {
			src = " (" + p.ExpirationSource + ")"
		}
		b.WriteString(fmt.Sprintf("Expiration:   %s%s\n", p.ExpirationDate.Format("2006-01-02"), src))
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
