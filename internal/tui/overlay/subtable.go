package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

type subtableParams struct {
	Theme         render.Theme
	Columns       []render.TableColumn
	Page          *render.Paginator
	Total         int
	PageSize      int
	FocusActive   bool
	FocusedColIdx int
	ActiveSort    string
	SortAscending bool
	VisualMode    bool
	IsRowSelected func(absIdx int) bool
	IsRowMarked   func(absIdx int) bool
	MarkGlyph     string // override mark glyph (defaults to Theme.Glyphs.RowMark)
}

func renderSubtable(params subtableParams, maxW int, getCellValue func(absIdx, rowIdx, colIdx int) string) string {
	params.Page.SetTotal(params.Total)
	params.Page.SetPageSize(params.PageSize)
	start, end := params.Page.Window()

	cursor := params.Page.Cursor()
	focusActive := params.FocusActive
	tableParams := render.TableParams{
		Theme:         params.Theme,
		Columns:       params.Columns,
		RowCount:      end - start,
		FocusedColIdx: params.FocusedColIdx,
		ActiveSort:    params.ActiveSort,
		SortAscending: params.SortAscending,
		FocusActive:   focusActive,
		VisualMode:    params.VisualMode,
		MarkGlyph:     params.MarkGlyph,
		IsRowCursor: func(rowIdx int) bool {
			return focusActive && start+rowIdx == cursor
		},
	}
	if params.IsRowSelected != nil {
		tableParams.IsRowSelected = func(rowIdx int) bool {
			return params.IsRowSelected(start + rowIdx)
		}
	}
	if params.IsRowMarked != nil {
		tableParams.IsRowMarked = func(rowIdx int) bool {
			return params.IsRowMarked(start + rowIdx)
		}
	}

	return render.RenderTable(tableParams, maxW, func(rowIdx, colIdx int) string {
		return getCellValue(start+rowIdx, rowIdx, colIdx)
	})
}

func subtableStatus(page render.Paginator) string {
	if page.Total() == 0 {
		return "[0/0]"
	}
	return fmt.Sprintf("[%d/%d]", page.Cursor()+1, page.Total())
}

func handleSubtableMotionKey(page *render.Paginator, msg tea.KeyMsg, vimCount *int) bool {
	s := msg.String()
	if len(s) == 1 && s[0] >= '0' && s[0] <= '9' && (s != "0" || *vimCount > 0) {
		*vimCount = *vimCount*10 + int(s[0]-'0')
		return true
	}

	count := *vimCount
	*vimCount = 0

	switch s {
	case "j", "down":
		page.MoveDown(max(count, 1))
		return true
	case "k", "up":
		page.MoveUp(max(count, 1))
		return true
	case "ctrl+d", "pgdown":
		page.PageDown()
		return true
	case "ctrl+u", "pgup":
		page.PageUp()
		return true
	case "g", "home":
		page.NavTop(max(count, 1))
		return true
	case "G", "end":
		page.NavBottom(max(count, 1))
		return true
	}
	return false
}

// statsRowPrefixWidth is the display width of the prefix that RenderTable
// always adds in front of every data row (cursor glyph + mark glyph + space).
// We must reserve this when computing column widths for the stats subtables.
const statsRowPrefixWidth = 3

// StatsPatentsColumns returns a well-balanced set of columns for the
// "Patents by Selected ..." subtable that appears inside the inventor,
// assignee, and classification stats overlays.
//
// The only remaining "configuration" numbers are the *minimum* widths
// for the non-flexible columns (number, inventor, year, tags).
// Everything else is computed:
//
//   - Title gets most of the flexible space, but we deliberately reserve
//     at least 20 columns of the flexible space. This makes the title
//     column noticeably smaller.
//   - The state column has a small fixed width. Final right-edge alignment
//     for the entire overlay (including subtable rows with icons) is
//     enforced uniformly by normalizeOverlayContent after the table is
//     generated. This approach is much less brittle than trying to make
//     any single column (especially the one with the emoji) absorb the
//     exact remainder through column math.
//
// When numberColWidth > 0 it overrides the default minimum for the
// Number column (allowing it to dynamically size to the longest
// visible patent number in the current page).
//
// This guarantees that every rendered subtable row has a deterministic
// total width, so the right edge of the box lines up perfectly on rows
// that contain double-width icons (✅, ◆, etc.) and rows that don't.
//
// The include* flags let callers drop optional columns on narrow terminals.
func StatsPatentsColumns(availWidth int, includeInventorCol, includeTagsCol bool, numberColWidth int) []render.TableColumn {
	w := availWidth
	if w < 40 {
		w = 40
	}

	// Minimum widths for columns.
	//
	// The state column (last one) is deliberately kept small and fixed.
	// The final right-edge alignment for the whole overlay is handled by
	// normalizeOverlayContent after the subtable is generated. This is much
	// less brittle than trying to make the icon column itself be the
	// variable-width catch-up column.
	const (
		defaultNumWidth = 16
		yearWidth       = 4
		tagsWidth       = 15
		invWidth        = 24
		stateWidth      = 6 // small fixed width for the icon column
	)

	numWidth := defaultNumWidth
	if numberColWidth > 0 {
		numWidth = numberColWidth
	}

	type spec struct {
		key     string
		label   string
		sortKey string
		width   int // 0 means "flexible / computed later"
	}

	var cols []spec

	cols = append(cols, spec{"number", "Number", string(domain.SortByNumber), numWidth})
	cols = append(cols, spec{"title", "Title", string(domain.SortByTitle), 0}) // main flexible

	if includeInventorCol {
		cols = append(cols, spec{"inventor", "Inventor", string(domain.SortByInventor), invWidth})
	}
	cols = append(cols, spec{"year", "Year", string(domain.SortByExpires), yearWidth})

	if includeTagsCol {
		cols = append(cols, spec{"tags", "Tags", string(domain.SortByTags), tagsWidth})
	}
	cols = append(cols, spec{"state", "State", string(domain.SortByReviewState), stateWidth})

	// Sum everything except title (title is the main flexible column).
	fixed := 0
	for _, c := range cols {
		if c.key != "title" {
			fixed += c.width
		}
	}

	gaps := len(cols) - 1

	flex := w - fixed - gaps - statsRowPrefixWidth

	// Title is the main flexible column.
	// We deliberately give it less space (reserve at least 20 columns of flex)
	// so the overall layout feels better balanced.
	titleW := max(10, flex-20)

	out := make([]render.TableColumn, len(cols))
	for i, c := range cols {
		width := c.width
		if c.key == "title" {
			width = titleW
		}
		out[i] = render.TableColumn{
			Key:     c.key,
			Label:   c.label,
			SortKey: c.sortKey,
			Width:   width,
		}
	}
	return out
}

// normalizeOverlayContent forces every logical line in the content returned
// by a stats overlay View to exactly the same display width (targetW).
// This guarantees that the right edge of the lipgloss box is perfectly
// straight, even on rows that contain double-width icons (state glyphs,
// mark glyphs, etc.) in the subtable or elsewhere.
//
// We do this as a final pass because the subtable (RenderTable) and the
// various list/divider lines can have tiny measurement differences once
// all the lipgloss styling and emojis are in the string.
func normalizeOverlayContent(content string, w int) string {
	if w <= 0 {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = render.Pad(line, w)
	}
	return strings.Join(lines, "\n")
}

// maxVisiblePatentNumberWidth returns the display width of the longest
// patent number (using DisplayNumber when present) among the rows
// currently visible in the subtable. This lets the Number column
// dynamically "catch up" to actual content instead of always using
// the old hardcoded 16.
func maxVisiblePatentNumberWidth(patents []domain.PatentRow, start, count int) int {
	maxW := 0
	for i := 0; i < count && start+i < len(patents); i++ {
		p := patents[start+i]
		s := p.Number.String()
		if !p.DisplayNumber.IsZero() {
			s = p.DisplayNumber.String()
		}
		if w := render.StringWidth(s); w > maxW {
			maxW = w
		}
	}
	return maxW
}
