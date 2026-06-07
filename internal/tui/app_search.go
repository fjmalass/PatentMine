package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/text"
	"patentmine/internal/tui/pane"
	"patentmine/internal/tui/render"
)

// searchFocus selects which half of the search dock keys drive.
type searchFocus int

const (
	focusDock    searchFocus = iota // navigating the results list
	focusPreview                    // reading the full-text preview above it
)

const (
	// Dock column widths.
	searchDockNumW   = 18 // patent-number column (comma-formatted display number)
	searchDockStageW = 5  // stage tag column on a hit row ("grant"/"pub")
	searchDockLocW   = 16 // section-locator column

	// Dock list height: a fraction of the body, clamped to [min,max] rows, and
	// never so tall it leaves no room for the preview above the divider.
	searchDockFraction    = 3  // list takes ~1/searchDockFraction of the body
	searchDockMinRows     = 5  // smallest the list shrinks to
	searchDockMaxRows     = 14 // largest the list grows to
	searchDockDividerRows = 1  // the divider rule between preview and list
	searchDockMinBodyRows = 1  // always keep at least this much preview
)

// searchRow is one dock line. Rows are grouped by patent: each patent gets a
// header row, followed by its hit rows. A patent with no hit (or no ingested
// body) is a header row on its own, so every searched patent is visible. number
// is the canonical record number used to load the preview; display is the public
// (grant/pub) number shown to the user.
type searchRow struct {
	number  domain.PatentNumber
	display domain.PatentNumber
	header  bool // a patent header row
	noMatch bool // header: ingested but no hit
	missing bool // header: no ingested body
	hits    int  // header: number of hit rows under it

	kind    proto.USPTOXMLKind // hit row: stage the hit came from
	locator string
	occ     int
	snippet string
}

// searchSession is the persistent cross-patent full-text search: a results dock
// docked to the bottom of the screen plus a live full-text preview above it. The
// preview is a real FullText pane kept on the pane stack (so it loads and routes
// messages like any pane); baseDepth records the stack height the dock was
// opened at, so closing it pops back exactly.
type searchSession struct {
	query          string
	rows           []searchRow
	page           render.Paginator // cursor over rows
	focus          searchFocus
	preview        *pane.FullText
	previewNumber  domain.PatentNumber
	previewMissing bool // cursor is on a not-ingested patent: show a placeholder
	baseDepth      int

	hitRows     int                   // total hit rows (for the divider)
	hitPatents  int                   // distinct patents with a hit
	noMatch     int                   // ingested patents with no hit
	missing     int                   // patents with no ingested body
	missingNums []domain.PatentNumber // for the [f] fetch action
}

// fullTextSearchDoneMsg delivers the result of a daemon full-text scan.
type fullTextSearchDoneMsg struct {
	res proto.FullTextSearchResult
	err error
}

// matchDisplay returns the public number to show for a match, falling back to
// the canonical number when the daemon supplied no display number.
func matchDisplay(mt proto.FullTextSearchMatch) domain.PatentNumber {
	if mt.Display.IsZero() {
		return mt.Number
	}
	return mt.Display
}

// handleFullTextSearchDone opens the results dock for the daemon scan. The dock
// lists every searched patent (hits, no-hits, and not-ingested), so "only one
// result" is always explained on screen rather than left a mystery.
func (a *App) handleFullTextSearchDone(m fullTextSearchDoneMsg) (tea.Model, tea.Cmd) {
	if m.err != nil {
		a.setErr(text.StatusFullTextSearchFailed, m.err.Error())
		return a, nil
	}
	total := len(m.res.Matches) + len(m.res.NoMatch) + len(m.res.Missing)
	if total == 0 {
		a.setErr(text.StatusFullTextNoBodies, 0)
		return a, nil
	}
	if len(m.res.Matches) > 0 {
		hits := map[domain.PatentNumber]bool{}
		for _, mt := range m.res.Matches {
			hits[mt.Number] = true
		}
		missingNote := ""
		if len(m.res.Missing) > 0 {
			missingNote = fmt.Sprintf(" — %d not ingested", len(m.res.Missing))
		}
		a.setStatus(text.StatusFullTextResults, len(m.res.Matches), len(hits), missingNote)
	} else {
		a.setStatus(text.StatusFullTextNoMatches, m.res.Query, m.res.Scanned)
	}
	return a, a.openSearchDock(m.res)
}

// openSearchDock builds the grouped dock rows from the scan result and shows the
// first row's preview.
func (a *App) openSearchDock(res proto.FullTextSearchResult) tea.Cmd {
	hits := map[domain.PatentNumber]bool{}
	rows := make([]searchRow, 0, len(res.Matches)+len(res.NoMatch)+len(res.Missing))

	// Matches arrive grouped by patent (the daemon scans one patent fully before
	// the next), so consecutive runs share a number. Emit a header per run.
	for i := 0; i < len(res.Matches); {
		n := res.Matches[i].Number
		j := i
		for j < len(res.Matches) && res.Matches[j].Number == n {
			j++
		}
		group := res.Matches[i:j]
		hits[n] = true
		rows = append(rows, searchRow{
			number: n, display: matchDisplay(group[0]), header: true, hits: len(group),
			locator: group[0].Locator, occ: group[0].Occurrence,
		})
		for _, mt := range group {
			rows = append(rows, searchRow{
				number: n, display: matchDisplay(mt), kind: mt.Kind,
				locator: mt.Locator, occ: mt.Occurrence, snippet: mt.Snippet,
			})
		}
		i = j
	}
	for _, n := range res.NoMatch {
		rows = append(rows, searchRow{number: n, display: n, header: true, noMatch: true})
	}
	for _, n := range res.Missing {
		rows = append(rows, searchRow{number: n, display: n, header: true, missing: true})
	}

	s := &searchSession{
		query:       res.Query,
		rows:        rows,
		page:        render.NewPaginator(10),
		focus:       focusDock,
		baseDepth:   len(a.panes),
		hitRows:     len(res.Matches),
		hitPatents:  len(hits),
		noMatch:     len(res.NoMatch),
		missing:     len(res.Missing),
		missingNums: res.Missing,
	}
	s.page.SetTotal(len(rows))
	a.search = s
	return a.previewCursor()
}

// previewCursor updates the preview to the patent under the dock cursor. A
// not-ingested row shows a placeholder (no network fetch); a same-patent move
// jumps within the current preview; a different patent loads a fresh preview.
func (a *App) previewCursor() tea.Cmd {
	s := a.search
	if s == nil || len(s.rows) == 0 {
		return nil
	}
	i := s.page.Cursor()
	if i < 0 || i >= len(s.rows) {
		return nil
	}
	row := s.rows[i]
	if row.missing {
		// No ingested body to preview — keep the pane stack as-is and show a
		// placeholder rather than triggering a live fetch.
		s.previewMissing = true
		return nil
	}
	s.previewMissing = false
	if s.preview != nil && s.previewNumber == row.number {
		s.preview.JumpToOccurrence(row.locator, row.occ)
		return nil
	}
	return a.pushPreview(row)
}

// pushPreview replaces the preview pane with a fresh full-text viewer for row's
// patent (popping back to the dock's base depth first).
func (a *App) pushPreview(row searchRow) tea.Cmd {
	s := a.search
	if s.baseDepth <= len(a.panes) {
		a.panes = a.panes[:s.baseDepth]
	}
	var project domain.ProjectID
	if a.activeProject != nil {
		project = a.activeProject.ID
	}
	bound := a.keymaps.BoundLetters(command.ScopeFullText)
	p := pane.NewFullText(a.client, a.theme, row.number, project, bound).
		WithLogger(a.log()).
		OpenAt(s.query, row.locator, row.occ)
	a.panes = append(a.panes, p)
	s.preview = p
	s.previewNumber = row.number
	return p.Init()
}

// previewViaGoogle force-opens a preview for the row under the cursor. The
// full-text viewer falls back to Google Patents when the patent has no ingested
// USPTO body, so this is the on-demand way to read a not-ingested patent's text
// (one at a time — Google rate-limits bulk requests).
func (a *App) previewViaGoogle() tea.Cmd {
	s := a.search
	i := s.page.Cursor()
	if i < 0 || i >= len(s.rows) {
		return nil
	}
	s.previewMissing = false
	return a.pushPreview(s.rows[i])
}

// handleDockKey services keys while the dock has focus. It reports whether the
// key was consumed.
func (a *App) handleDockKey(m tea.KeyMsg) (tea.Cmd, bool) {
	s := a.search
	switch m.String() {
	case "j", "down":
		s.page.MoveDown(1)
		return a.previewCursor(), true
	case "k", "up":
		s.page.MoveUp(1)
		return a.previewCursor(), true
	case "ctrl+d", "pgdown":
		s.page.PageDown()
		return a.previewCursor(), true
	case "ctrl+u", "pgup":
		s.page.PageUp()
		return a.previewCursor(), true
	case "g", "home":
		s.page.Top()
		return a.previewCursor(), true
	case "G", "end":
		s.page.Bottom()
		return a.previewCursor(), true
	case "enter", "l":
		a.promoteSearch()
		return nil, true
	case "f":
		// Fetch the granted XML for the not-ingested patents so a re-run can
		// search them too. Pending applications without a grant are reported as
		// failures by the batch; re-run the search once it finishes.
		if len(s.missingNums) > 0 {
			_, cmd := a.fetchAndOpenUSPTOXML(s.missingNums, proto.USPTOXMLKindGrant)
			return cmd, true
		}
		return nil, true
	case "w":
		// View the patent under the cursor via Google Patents (on-demand, one at
		// a time) — works for not-ingested patents the search couldn't cover.
		return a.previewViaGoogle(), true
	case "esc", "q":
		a.closeSearch()
		return nil, true
	}
	return nil, false
}

// promoteSearch keeps the current preview as a normal pane and dismisses the
// dock, so the user lands in that patent's full-text viewer.
func (a *App) promoteSearch() {
	if a.search == nil {
		return
	}
	if a.search.preview != nil && !a.search.previewMissing {
		a.recordHistory(a.search.previewNumber)
		a.search = nil
		return
	}
	// On a not-ingested row there is no preview to keep — just close.
	a.closeSearch()
}

// closeSearch dismisses the dock and pops the preview pane, returning to where
// the search was launched.
func (a *App) closeSearch() {
	if a.search == nil {
		return
	}
	if a.search.baseDepth <= len(a.panes) {
		a.panes = a.panes[:a.search.baseDepth]
	}
	a.search = nil
}

// syncSearchDock dismisses the dock if a live preview is no longer the focused
// pane (the user drilled away or pressed Back). Called before each render.
func (a *App) syncSearchDock() {
	s := a.search
	if s == nil || s.preview == nil {
		return
	}
	if len(a.panes) == 0 || a.panes[len(a.panes)-1] != pane.Pane(s.preview) {
		a.search = nil
	}
}

// searchView renders the search body: the full-text preview (or a placeholder)
// on top, a divider, and the grouped results dock below.
func (a *App) searchView(w, h int) string {
	s := a.search
	listH := min(max(h/searchDockFraction, searchDockMinRows), searchDockMaxRows)
	if maxList := h - searchDockDividerRows - searchDockMinBodyRows; listH > maxList {
		listH = max(maxList, 1)
	}
	topH := max(h-listH-searchDockDividerRows, 1)

	var top string
	switch {
	case s.previewMissing:
		top = fitBody(a.theme.Dim.Render("  no ingested full text for this patent — press f to fetch its USPTO XML"), topH, w)
	case s.preview != nil:
		top = fitBody(s.preview.View(w, topH), topH, w)
	default:
		top = fitBody(a.theme.Dim.Render("  select a result to preview"), topH, w)
	}
	return top + "\n" + a.searchDivider(w) + "\n" + a.searchList(w, listH)
}

// searchDivider renders the dock's header rule with the coverage breakdown and
// the focus-dependent key hints.
func (a *App) searchDivider(w int) string {
	s := a.search
	label := fmt.Sprintf("Full-text search · %d hit(s) in %d patent(s)", s.hitRows, s.hitPatents)
	if s.noMatch > 0 {
		label += fmt.Sprintf(" · %d no match", s.noMatch)
	}
	if s.missing > 0 {
		label += fmt.Sprintf(" · %d not ingested", s.missing)
	}
	if s.query != "" {
		label += " · /" + s.query
	}
	if s.focus == focusPreview {
		label += "  [tab] list  [esc] back"
	} else {
		label += "  [tab] read  [enter] keep  [w] google"
		if s.missing > 0 {
			label += "  [f] fetch missing"
		}
		label += "  [esc] close"
	}
	line := "─ " + label + " "
	if pad := w - render.StringWidth(line); pad > 0 {
		line += strings.Repeat("─", pad)
	}
	return a.theme.Header.Render(render.Truncate(line, max(w, 1)))
}

// searchList renders the grouped dock rows: a patent header per patent followed
// by its indented hits (or a no-match / not-ingested note).
func (a *App) searchList(w, h int) string {
	s := a.search
	s.page.SetPageSize(h)
	start, end := s.page.Window()
	cur := s.page.Cursor()
	rows := make([]string, 0, h)
	for i := start; i < end; i++ {
		r := s.rows[i]
		rows = append(rows, a.searchRowString(r, w, i == cur))
	}
	for len(rows) < h {
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n")
}

// searchRowString renders one dock row (header or hit) with cursor/kind styling.
func (a *App) searchRowString(r searchRow, w int, cursor bool) string {
	var rowStr string
	if r.header {
		var summary string
		switch {
		case r.missing:
			summary = "not ingested — press f to fetch"
		case r.noMatch:
			summary = "no match"
		default:
			summary = fmt.Sprintf("%d hit(s)", r.hits)
		}
		rowStr = fmt.Sprintf("%-*s  %s", searchDockNumW, render.Truncate(r.display.DisplayString(), searchDockNumW), summary)
	} else {
		locator := r.locator
		if locator == "" {
			locator = "—"
		}
		rowStr = fmt.Sprintf("    %-*s %-*s  %s",
			searchDockStageW, stageTag(r.kind),
			searchDockLocW, render.Truncate(locator, searchDockLocW),
			r.snippet)
	}
	switch {
	case cursor && a.search.focus == focusDock:
		return a.theme.Selected.Render(render.Pad(rowStr, w))
	case cursor:
		return a.theme.Visual.Render(render.Pad(rowStr, w))
	case r.header && (r.missing || r.noMatch):
		return a.theme.Dim.Render(render.Truncate(rowStr, w))
	case r.header:
		return a.theme.Header.Render(render.Truncate(rowStr, w))
	default:
		return a.theme.Row.Render(render.Truncate(rowStr, w))
	}
}

// stageTag is the short stage label shown on a hit row.
func stageTag(kind proto.USPTOXMLKind) string {
	switch kind {
	case proto.USPTOXMLKindGrant:
		return "grant"
	case proto.USPTOXMLKindPGPub:
		return "pub"
	}
	return ""
}
