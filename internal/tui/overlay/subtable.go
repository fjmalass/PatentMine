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
//   - Title gets the bulk of the flexible space (after reserving prefix + gaps).
//   - The last column ("state", which holds the icon/glyph) is the
//     explicit "catch-up" column. After title is assigned, we give the
//     state column whatever width is needed so that
//       prefix + sum(all widths) + gaps == exact target row width.
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

	// Minimum widths for columns. The last column ("state", which holds the
	// icon/glyph) acts as the "catch-up" column: after we give the title
	// the bulk of the flexible space, we assign whatever is left to the
	// last column. This guarantees that every rendered table row has a
	// deterministic total width (prefix + sum(widths) + gaps), so the
	// right edge of the subtable lines up perfectly with the rest of the
	// overlay content even when the icon is double-width.
	const (
		defaultNumWidth = 16
		yearWidth       = 4
		tagsWidth       = 15
		invWidth        = 16
		// state has no fixed min here — it will be computed as the catch-up
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
	cols = append(cols, spec{"state", "State", string(domain.SortByReviewState), 0}) // catch-up column for icon alignment

	// Sum the fixed (non-title, non-catch-up) columns.
	fixed := 0
	for _, c := range cols {
		if c.key != "title" && c.key != "state" {
			fixed += c.width
		}
	}

	gaps := len(cols) - 1

	// Give the title most of the remaining space (after reserving for prefix).
	titleW := max(10, w-fixed-gaps-statsRowPrefixWidth)

	// Now compute what the last column (state/icon) must be to make the
	// total row width exact: prefix + all widths + gaps == target content width.
	// This is the "catch-up" that makes the right edge line up reliably.
	nonCatchup := fixed + titleW + gaps + statsRowPrefixWidth
	catchUpWidth := max(4, w-nonCatchup) // at least 4 so the icon has breathing room

	out := make([]render.TableColumn, len(cols))
	for i, c := range cols {
		width := c.width
		if c.key == "title" {
			width = titleW
		} else if c.key == "state" {
			width = catchUpWidth
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
