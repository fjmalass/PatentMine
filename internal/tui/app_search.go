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
	// Dock list column widths.
	searchDockNumW = 18 // patent-number column (comma-formatted display number)
	searchDockLocW = 16 // section-locator column

	// Dock list height: a fraction of the body, clamped to [min,max] rows, and
	// never so tall it leaves no room for the preview above the divider.
	searchDockFraction    = 3  // list takes ~1/searchDockFraction of the body
	searchDockMinRows     = 5  // smallest the list shrinks to
	searchDockMaxRows     = 14 // largest the list grows to
	searchDockDividerRows = 1  // the divider rule between preview and list
	searchDockMinBodyRows = 1  // always keep at least this much preview
)

// searchSession is the persistent cross-patent full-text search: a results dock
// docked to the bottom of the screen plus a live full-text preview above it. The
// preview is a real FullText pane kept on the pane stack (so it loads and routes
// messages like any pane); baseDepth records the stack height the dock was
// opened at, so closing it pops back exactly.
type searchSession struct {
	res           proto.FullTextSearchResult
	page          render.Paginator // cursor over res.Matches
	focus         searchFocus
	preview       *pane.FullText
	previewNumber domain.PatentNumber
	baseDepth     int
	hitPatents    int // distinct patents that produced matches
}

// fullTextSearchDoneMsg delivers the result of a daemon full-text scan.
type fullTextSearchDoneMsg struct {
	res proto.FullTextSearchResult
	err error
}

// handleFullTextSearchDone opens the results dock when the daemon scan returns
// matches, or reports a status distinguishing "no hits" from "nothing was
// searchable".
func (a *App) handleFullTextSearchDone(m fullTextSearchDoneMsg) (tea.Model, tea.Cmd) {
	if m.err != nil {
		a.setErr(text.StatusFullTextSearchFailed, m.err.Error())
		return a, nil
	}
	if len(m.res.Matches) == 0 {
		if m.res.Scanned == 0 {
			a.setErr(text.StatusFullTextNoBodies, len(m.res.Missing))
			return a, nil
		}
		a.setStatus(text.StatusFullTextNoMatches, m.res.Query, m.res.Scanned)
		return a, nil
	}
	hits := map[domain.PatentNumber]bool{}
	for _, mt := range m.res.Matches {
		hits[mt.Number] = true
	}
	missingNote := ""
	if len(m.res.Missing) > 0 {
		missingNote = fmt.Sprintf(" — %d not ingested", len(m.res.Missing))
	}
	a.setStatus(text.StatusFullTextResults, len(m.res.Matches), len(hits), missingNote)
	return a, a.openSearchDock(m.res)
}

// openSearchDock starts a search session and shows the first match's preview.
func (a *App) openSearchDock(res proto.FullTextSearchResult) tea.Cmd {
	hits := map[domain.PatentNumber]bool{}
	for _, mt := range res.Matches {
		hits[mt.Number] = true
	}
	s := &searchSession{
		res:        res,
		page:       render.NewPaginator(10),
		focus:      focusDock,
		baseDepth:  len(a.panes),
		hitPatents: len(hits),
	}
	s.page.SetTotal(len(res.Matches))
	a.search = s
	return a.previewCursor()
}

// previewCursor updates the preview to the match under the dock cursor: a jump
// within the current pane when it is the same patent, else a fresh preview pane
// (returning its load command).
func (a *App) previewCursor() tea.Cmd {
	s := a.search
	if s == nil || len(s.res.Matches) == 0 {
		return nil
	}
	i := s.page.Cursor()
	if i < 0 || i >= len(s.res.Matches) {
		return nil
	}
	mt := s.res.Matches[i]
	if s.preview != nil && s.previewNumber == mt.Number {
		s.preview.JumpToOccurrence(mt.Locator, mt.Occurrence)
		return nil
	}
	// Different patent: replace the preview pane (pop back to base, push fresh).
	if s.baseDepth <= len(a.panes) {
		a.panes = a.panes[:s.baseDepth]
	}
	var project domain.ProjectID
	if a.activeProject != nil {
		project = a.activeProject.ID
	}
	bound := a.keymaps.BoundLetters(command.ScopeFullText)
	p := pane.NewFullText(a.client, a.theme, mt.Number, project, bound).
		WithLogger(a.log()).
		OpenAt(s.res.Query, mt.Locator, mt.Occurrence)
	a.panes = append(a.panes, p)
	s.preview = p
	s.previewNumber = mt.Number
	return p.Init()
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
		if len(s.res.Missing) > 0 {
			_, cmd := a.fetchAndOpenUSPTOXML(s.res.Missing, proto.USPTOXMLKindGrant)
			return cmd, true
		}
		return nil, true
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
	if a.search.preview != nil {
		a.recordHistory(a.search.previewNumber)
	}
	a.search = nil
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

// syncSearchDock dismisses the dock if the preview is no longer the focused pane
// (the user drilled away or pressed Back). Called before each render.
func (a *App) syncSearchDock() {
	s := a.search
	if s == nil || s.preview == nil {
		return
	}
	if len(a.panes) == 0 || a.panes[len(a.panes)-1] != pane.Pane(s.preview) {
		a.search = nil
	}
}

// searchView renders the search body: the full-text preview on top, a divider,
// and the results dock below.
func (a *App) searchView(focused pane.Pane, w, h int) string {
	listH := min(max(h/searchDockFraction, searchDockMinRows), searchDockMaxRows)
	if maxList := h - searchDockDividerRows - searchDockMinBodyRows; listH > maxList {
		listH = max(maxList, 1)
	}
	topH := max(h-listH-searchDockDividerRows, 1)
	top := fitBody(focused.View(w, topH), topH, w)
	return top + "\n" + a.searchDivider(w) + "\n" + a.searchList(w, listH)
}

// searchDivider renders the dock's header rule with the match count and the
// focus-dependent key hints.
func (a *App) searchDivider(w int) string {
	s := a.search
	label := fmt.Sprintf("Full-text search %d/%d · %d patent(s)", s.page.Cursor()+1, len(s.res.Matches), s.hitPatents)
	if len(s.res.NoMatch) > 0 {
		label += fmt.Sprintf(" · %d no match", len(s.res.NoMatch))
	}
	if len(s.res.Missing) > 0 {
		label += fmt.Sprintf(" · %d not ingested", len(s.res.Missing))
	}
	if s.res.Query != "" {
		label += " · /" + s.res.Query
	}
	if s.focus == focusPreview {
		label += "  [tab] list  [esc] back"
	} else {
		label += "  [tab] read  [enter] keep"
		if len(s.res.Missing) > 0 {
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

// searchList renders the results dock rows (patent · section · snippet).
func (a *App) searchList(w, h int) string {
	s := a.search
	s.page.SetPageSize(h)
	start, end := s.page.Window()
	cur := s.page.Cursor()
	rows := make([]string, 0, h)
	for i := start; i < end; i++ {
		mt := s.res.Matches[i]
		locator := mt.Locator
		if locator == "" {
			locator = "—"
		}
		row := fmt.Sprintf("%-*s  %-*s  %s",
			searchDockNumW, render.Truncate(mt.Number.DisplayString(), searchDockNumW),
			searchDockLocW, render.Truncate(locator, searchDockLocW),
			mt.Snippet)
		switch {
		case i == cur && s.focus == focusDock:
			rows = append(rows, a.theme.Selected.Render(render.Pad(row, w)))
		case i == cur:
			rows = append(rows, a.theme.Visual.Render(render.Pad(row, w)))
		default:
			rows = append(rows, a.theme.Row.Render(render.Truncate(row, w)))
		}
	}
	for len(rows) < h {
		rows = append(rows, "")
	}
	return strings.Join(rows, "\n")
}
