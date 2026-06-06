package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/tui/overlay"
	"patentmine/internal/tui/pane"
	"patentmine/internal/tui/render"
)

const (
	quickListWidthPct  = 70
	quickListHeightPct = 70
	quickListMinWidth  = 60
	quickListMinHeight = 14

	// quickListLocatorW is the fixed column width for the section locator
	// ("Claim 5", "Disclosure ¶0045") so line numbers and text line up.
	quickListLocatorW = 18
)

// QuickListOverlay lists every search match in a full-text pane as a navigable,
// vim-quickfix-style popup. Selecting an entry jumps the underlying pane to that
// line (expanding it first if it is collapsed) and closes the overlay.
type QuickListOverlay struct {
	theme   render.Theme
	number  domain.PatentNumber
	query   string
	matches []pane.FullTextMatch
	page    render.Paginator
}

func newQuickListOverlay(theme render.Theme, number domain.PatentNumber, query string, matches []pane.FullTextMatch) *QuickListOverlay {
	return &QuickListOverlay{
		theme:   theme,
		number:  number,
		query:   query,
		matches: matches,
		page:    render.NewPaginator(10),
	}
}

func (o *QuickListOverlay) Title() string {
	label := "1 match"
	if n := len(o.matches); n != 1 {
		label = fmt.Sprintf("%d matches", n)
	}
	title := "Matches · " + o.number.String() + " · " + label
	if o.query != "" {
		title += " · /" + o.query
	}
	return title
}

// OverlaySize implements DynamicSize so the quicklist gets a roomy popup.
func (o *QuickListOverlay) OverlaySize(termW, termH int) (int, int) {
	return overlay.PctSize(termW, termH, quickListWidthPct, quickListHeightPct, quickListMinWidth, quickListMinHeight)
}

// Handles returns no command IDs: the overlay consumes its keys directly as a
// KeyHandler, so there are no overlay-scoped keymap bindings to validate.
func (o *QuickListOverlay) Handles() []command.ID { return nil }

func (o *QuickListOverlay) Command(command.ID, int) (overlay.Overlay, tea.Cmd) { return o, nil }

// HandleKey drives navigation and selection. Every key is consumed so the list
// behaves as a modal layer; Enter jumps to the focused match, Esc/q close.
func (o *QuickListOverlay) HandleKey(msg tea.KeyMsg) (overlay.Overlay, tea.Cmd, bool) {
	switch msg.String() {
	case "enter", "l":
		return o, o.jump(), true
	case "j", "down":
		o.page.MoveDown(1)
	case "k", "up":
		o.page.MoveUp(1)
	case "ctrl+d", "pgdown":
		o.page.PageDown()
	case "ctrl+u", "pgup":
		o.page.PageUp()
	case "g", "home":
		o.page.Top()
	case "G", "end":
		o.page.Bottom()
	case "esc", "q", "Q":
		return o, func() tea.Msg { return overlay.CloseOverlayMsg{} }, true
	}
	return o, nil, true
}

// jump asks the source full-text pane to scroll to the focused match, then
// closes the overlay.
func (o *QuickListOverlay) jump() tea.Cmd {
	if len(o.matches) == 0 {
		return func() tea.Msg { return overlay.CloseOverlayMsg{} }
	}
	target := o.matches[o.page.Cursor()]
	number := o.number
	return tea.Batch(
		func() tea.Msg { return pane.FullTextJumpMsg{Number: number, Line: target.Line} },
		func() tea.Msg { return overlay.CloseOverlayMsg{} },
	)
}

func (o *QuickListOverlay) View(maxW, maxH int) string {
	if len(o.matches) == 0 {
		return o.theme.Dim.Render("  (no matches)")
	}
	// Reserve the last two rows for spacing and the footer hint.
	bodyH := max(maxH-2, 1)
	o.page.SetTotal(len(o.matches))
	o.page.SetPageSize(bodyH)
	start, end := o.page.Window()
	cur := o.page.Cursor()

	var b strings.Builder
	for i := start; i < end; i++ {
		m := o.matches[i]
		locator := m.Locator
		if locator == "" {
			locator = "—"
		}
		row := fmt.Sprintf("%-*s  L%-5d  %s", quickListLocatorW, render.Truncate(locator, quickListLocatorW), m.Line+1, m.Text)
		if i == cur {
			b.WriteString(o.theme.Selected.Render(render.Pad(row, maxW)))
		} else {
			b.WriteString(o.theme.Row.Render(render.Truncate(row, maxW)))
		}
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(o.theme.Dim.Render("  [enter] jump  [j/k] move  [ctrl+u/d] page  [g/G] top/bottom  [esc/q] close"))
	return b.String()
}
