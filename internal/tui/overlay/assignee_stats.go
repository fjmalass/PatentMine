package overlay

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/pane"
	"patentmine/internal/tui/render"
)

type loadedAssigneeStatsMsg struct {
	stats []domain.AssigneeStats
	err   error
}

// AssigneeStatsOverlay presents interactive statistics for all assignees.
type AssigneeStatsOverlay struct {
	client   *rpc.Client
	theme    render.Theme
	catalog  *text.Catalog
	patent   domain.Patent
	project  domain.ProjectID
	stats    []domain.AssigneeStats
	selected int
	loading  bool
	err      error

	focus           statsFocus
	patents         []domain.PatentRow
	patentsPage     render.Paginator
	patentsVimCount int
	patentsLoading  bool
	patentsErr      error
	loadSeq         uint64
	loadID          uint64

	activeSort    domain.SortColumn
	sortAscending bool
	focusedColIdx int
	lastWidth     int
	preselect     string
}

func NewAssigneeStatsOverlay(client *rpc.Client, theme render.Theme, catalog *text.Catalog, patent domain.Patent, project domain.ProjectID) (*AssigneeStatsOverlay, tea.Cmd) {
	o := &AssigneeStatsOverlay{
		client:        client,
		theme:         theme,
		catalog:       catalog,
		patent:        patent,
		project:       project,
		loading:       true,
		focus:         focusAssignees,
		patentsPage:   render.NewPaginator(5),
		activeSort:    domain.SortByNumber,
		sortAscending: true,
		focusedColIdx: -1,
		lastWidth:     90,
		preselect:     strings.TrimSpace(patent.Assignee),
	}
	return o, o.loadStatsCmd()
}

const focusAssignees = focusInventors

func (o *AssigneeStatsOverlay) Title() string { return "Assignee Analytics" }

func (o *AssigneeStatsOverlay) Handles() []command.ID { return []command.ID{command.CloseOverlay} }

func (o *AssigneeStatsOverlay) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if id == command.CloseOverlay {
		return o, func() tea.Msg { return CloseOverlayMsg{} }
	}
	return o, nil
}

func (o *AssigneeStatsOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case loadedAssigneeStatsMsg:
		o.loading = false
		if m.err != nil {
			o.err = m.err
			return o, nil
		}
		o.stats = m.stats
		if o.selected >= len(o.stats) {
			o.selected = max(0, len(o.stats)-1)
		}
		if o.preselect != "" {
			for idx, stat := range o.stats {
				if stat.Assignee == o.preselect {
					o.selected = idx
					break
				}
			}
		}
		if len(o.stats) > 0 {
			o.loadSeq++
			o.loadID = o.loadSeq
			o.patentsLoading = true
			o.patentsErr = nil
			return o, o.loadPatentsCmd(o.stats[o.selected].Assignee, o.loadID)
		}
		return o, nil

	case loadedPatentListMsg:
		if m.requestID == o.loadID {
			o.patentsLoading = false
			o.patentsErr = m.err
			if m.err == nil {
				o.patents = m.patents
				o.patentsPage.SetTotal(m.total)
			}
		}
		return o, nil

	case proto.Event:
		if m.Method == proto.EventDBChanged {
			var cmds []tea.Cmd
			o.loading = true
			cmds = append(cmds, o.loadStatsCmd())
			if len(o.stats) > 0 && o.selected < len(o.stats) {
				o.loadSeq++
				o.loadID = o.loadSeq
				o.patentsLoading = true
				o.patentsErr = nil
				cmds = append(cmds, o.loadPatentsCmd(o.stats[o.selected].Assignee, o.loadID))
			}
			return o, tea.Batch(cmds...)
		}
		return o, nil

	case tea.WindowSizeMsg:
		w, h := o.OverlaySize(m.Width, m.Height)
		o.lastWidth = w - 4
		innerHeight := h - 4
		o.focusedColIdx = clampFocusedStatsColumn(o.currentCols(), o.focusedColIdx)
		_, patentsH := o.calcHeights(innerHeight)
		if patentsH != o.patentsPage.PageSize() {
			o.patentsPage.SetPageSize(patentsH)
			if len(o.stats) > 0 && o.selected < len(o.stats) {
				o.loadSeq++
				o.loadID = o.loadSeq
				o.patentsLoading = true
				o.patentsErr = nil
				return o, o.loadPatentsCmd(o.stats[o.selected].Assignee, o.loadID)
			}
		}
		return o, nil
	}
	return o, nil
}

func (o *AssigneeStatsOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	o.err = nil
	o.patentsErr = nil

	if o.focus == focusPatents {
		if o.patentsPage.HandleKey(msg) {
			return o, nil, true
		}
	}

	switch msg.String() {
	case "q", "Q", "esc":
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case "tab":
		if o.focus == focusAssignees {
			o.focus = focusPatents
			o.focusedColIdx = 0
		} else {
			o.focus = focusAssignees
			o.focusedColIdx = -1
		}
		return o, nil, true
	}

	if o.focus == focusAssignees {
		switch msg.String() {
		case "j", "down":
			if len(o.stats) > 0 {
				o.selected = (o.selected + 1) % len(o.stats)
				o.patentsPage.Top()
				o.loadSeq++
				o.loadID = o.loadSeq
				o.patentsLoading = true
				return o, o.loadPatentsCmd(o.stats[o.selected].Assignee, o.loadID), true
			}
		case "k", "up":
			if len(o.stats) > 0 {
				o.selected = (o.selected - 1 + len(o.stats)) % len(o.stats)
				o.patentsPage.Top()
				o.loadSeq++
				o.loadID = o.loadSeq
				o.patentsLoading = true
				return o, o.loadPatentsCmd(o.stats[o.selected].Assignee, o.loadID), true
			}
		case "l", "right", "enter":
			o.focus = focusPatents
			o.focusedColIdx = 0
			return o, nil, true
		}
		return o, nil, true
	}

	switch msg.String() {
	case "left":
		o.focusedColIdx = moveStatsColumn(o.currentCols(), o.focusedColIdx, -1)
		return o, nil, true
	case "right":
		o.focusedColIdx = moveStatsColumn(o.currentCols(), o.focusedColIdx, 1)
		return o, nil, true
	case "h":
		o.focus = focusAssignees
		o.focusedColIdx = -1
		return o, nil, true
	case ".":
		if o.focusedColIdx >= 0 {
			cols := o.currentCols()
			col := cols[o.focusedColIdx]
			if col.SortKey != "" {
				if string(o.activeSort) == col.SortKey {
					o.sortAscending = !o.sortAscending
				} else {
					o.activeSort = domain.SortColumn(col.SortKey)
					o.sortAscending = true
				}
				o.patentsPage.Top()
				o.loadSeq++
				o.loadID = o.loadSeq
				o.patentsLoading = true
				assignee := ""
				if len(o.stats) > 0 && o.selected >= 0 && o.selected < len(o.stats) {
					assignee = o.stats[o.selected].Assignee
				}
				return o, o.loadPatentsCmd(assignee, o.loadID), true
			}
		}
		return o, nil, true
	}

	before := o.patentsPage.Offset()
	if handleSubtableMotionKey(&o.patentsPage, msg, &o.patentsVimCount) {
		if o.patentsPage.Offset() != before {
			return o, o.reloadPatentsCmd(), true
		}
		return o, nil, true
	}

	if len(o.patents) == 0 {
		return o, nil, true
	}
	cursor := o.patentsPage.CursorInPage()
	if cursor < 0 || cursor >= len(o.patents) {
		return o, nil, true
	}
	selectedPatent := o.patents[cursor]

	switch msg.String() {
	case "l", "enter":
		return o, func() tea.Msg { return OpenPatentDetailMsg{Number: selectedPatent.Number} }, true
	case "s":
		numbers := o.selections()
		if len(numbers) == 0 {
			numbers = []domain.PatentNumber{selectedPatent.Number}
		}
		cmd := pane.SetReviewStateCmd(o.client, o.project, numbers, domain.ReviewStateActive)
		o.clearVisual()
		return o, cmd, true
	case "r":
		numbers := o.selections()
		if len(numbers) == 0 {
			numbers = []domain.PatentNumber{selectedPatent.Number}
		}
		cmd := pane.SetReviewStateCmd(o.client, o.project, numbers, domain.ReviewStateUnderReview)
		o.clearVisual()
		return o, cmd, true
	case "i":
		numbers := o.selections()
		if len(numbers) == 0 {
			numbers = []domain.PatentNumber{selectedPatent.Number}
		}
		cmd := pane.SetReviewStateCmd(o.client, o.project, numbers, domain.ReviewStateIgnored)
		o.clearVisual()
		return o, cmd, true
	case "x":
		numbers := o.selections()
		if len(numbers) == 0 {
			numbers = []domain.PatentNumber{selectedPatent.Number}
		}
		cmd := pane.SetReviewStateCmd(o.client, o.project, numbers, domain.ReviewStateDeleted)
		o.clearVisual()
		return o, cmd, true
	case "t":
		numbers := o.selections()
		if len(numbers) == 0 {
			numbers = []domain.PatentNumber{selectedPatent.Number}
		}
		o.clearVisual()
		return o, func() tea.Msg { return OpenTagPatentOverlayMsg{Patents: numbers} }, true
	case "I":
		numbers := o.selections()
		if len(numbers) == 0 {
			numbers = []domain.PatentNumber{selectedPatent.Number}
		}
		o.clearVisual()
		return o, pane.CycleIDSEntryStatusesCmd(o.client, o.project, numbers), true
	}

	return o, nil, true
}

func (o *AssigneeStatsOverlay) OverlaySize(termW, termH int) (w, h int) {
	return PctSize(termW, termH, 90, 90, 90, 22)
}

func (o *AssigneeStatsOverlay) currentCols() []render.TableColumn {
	avail := o.lastWidth - statsTableMargin

	// For assignee stats we keep the inventor and tags columns visible
	// in more width buckets than the inventor stats view does.
	var includeInventor, includeTags bool
	switch {
	case avail >= 65:
		includeInventor, includeTags = true, true
	case avail >= 55:
		includeInventor, includeTags = true, false
	default:
		includeInventor, includeTags = false, false
	}

	return StatsPatentsColumns(avail, includeInventor, includeTags, 0)
}

func (o *AssigneeStatsOverlay) View(maxW, maxH int) string {
	o.lastWidth = maxW
	targetW := maxW - statsTableMargin
	if targetW < 40 {
		targetW = 40
	}
	var b strings.Builder

	if o.err != nil {
		b.WriteString(o.theme.Error.Render(render.Truncate("Error: "+o.err.Error(), targetW)))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("[q/Esc] Close"))
		return b.String()
	}
	if o.loading {
		b.WriteString(o.theme.MutedItalic.Render("Loading assignee statistics..."))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("[q/Esc] Close"))
		return b.String()
	}

	titleText := fmt.Sprintf("  Assignee Stats (%d assignees)", len(o.stats))
	if o.patent.DisplayNumber.String() != "" && !o.patent.DisplayNumber.IsZero() {
		titleText = fmt.Sprintf("  Assignee Stats for Patent %s (%d assignees)", o.patent.DisplayNumber.String(), len(o.stats))
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(titleText, targetW)))
	b.WriteString("\n\n")

	if len(o.stats) == 0 {
		b.WriteString(o.theme.MutedItalic.Render("No assignees found."))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("[q/Esc] Close"))
		return b.String()
	}

	statsH, patentsH := o.calcHeights(maxH)
	o.patentsPage.SetPageSize(patentsH)

	maxNameLen := 0
	for _, s := range o.stats {
		if len(s.Assignee) > maxNameLen {
			maxNameLen = len(s.Assignee)
		}
	}
	startStats := max(0, o.selected-statsH/2)
	endStats := min(len(o.stats), startStats+statsH)
	if endStats-startStats < statsH && startStats > 0 {
		startStats = max(0, endStats-statsH)
	}
	for i := startStats; i < endStats; i++ {
		s := o.stats[i]
		isSelectedRow := i == o.selected
		cursorPart := o.theme.Glyphs.RowNoCursor
		if isSelectedRow && o.focus == focusAssignees {
			cursorPart = o.theme.Glyphs.RowCursor
		}
		prefix := cursorPart + o.theme.Glyphs.RowNoMark + " "
		paddedName := render.Pad(s.Assignee, maxNameLen+2)
		statsStr := render.FormatEntityStats(s.Total, s.States, s.Tags)
		line := fmt.Sprintf("%s%s%s", prefix, paddedName, statsStr)
		if isSelectedRow && o.focus == focusAssignees {
			b.WriteString(o.theme.Selected.Render(render.Truncate(line, targetW)))
		} else if i%2 == 1 {
			b.WriteString(o.theme.RowAlt.Render(render.Truncate(line, targetW)))
		} else {
			b.WriteString(o.theme.Row.Render(render.Truncate(line, targetW)))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	dividerText := fmt.Sprintf("─── Patents by Selected Assignee (%s) ───", o.stats[o.selected].Assignee)
	dashCount := targetW - render.StringWidth(dividerText)
	if dashCount > 0 {
		dividerText += strings.Repeat("─", dashCount)
	}
	b.WriteString(o.theme.Header.Render(render.Truncate(dividerText, targetW)))
	b.WriteString("\n")

	if o.patentsErr != nil {
		b.WriteString(o.theme.Error.Render(render.Truncate("Error loading patents: "+o.patentsErr.Error(), targetW)))
		b.WriteString("\n")
	} else if o.patentsLoading && len(o.patents) == 0 {
		b.WriteString(o.theme.MutedItalic.Render("Loading patents..."))
		b.WriteString("\n")
	} else if len(o.patents) == 0 {
		b.WriteString(o.theme.MutedItalic.Render("No patents found for this assignee."))
		b.WriteString("\n")
	} else {
		avail := o.lastWidth - statsTableMargin
		if avail < 40 {
			avail = 40
		}

		// Decide optional columns (same logic as currentCols)
		var includeInventor, includeTags bool
		switch {
		case avail >= 65:
			includeInventor, includeTags = true, true
		case avail >= 55:
			includeInventor, includeTags = true, false
		default:
			includeInventor, includeTags = false, false
		}

		// Compute dynamic width for Number column from visible data
		numColWidth := 16
		if len(o.patents) > 0 {
			start, cnt := o.patentsPage.Window()
			if maxNum := maxVisiblePatentNumberWidth(o.patents, start, cnt); maxNum > 0 {
				numColWidth = max(maxNum+1, 8)
			}
		}

		// Use ForceExactWidth so every subtable row is forced to exactly
		// targetW. This avoids brittle catch-up math on the icon column.
		cols := StatsPatentsColumns(avail, includeInventor, includeTags, numColWidth)

		offset := o.patentsPage.Offset()
		tableStr := renderSubtable(subtableParams{
			ForceExactWidth: true,
			TargetWidth:     targetW,
			Theme:           o.theme,
			Columns:         cols,
			Page:            &o.patentsPage,
			Total:           o.patentsPage.Total(),
			PageSize:        patentsH,
			FocusedColIdx:   o.focusedColIdx,
			ActiveSort:      string(o.activeSort),
			SortAscending:   o.sortAscending,
			FocusActive:     o.focus == focusPatents,
			VisualMode:      o.patentsPage.VisualMode(),
			IsRowSelected: func(absIdx int) bool {
				return o.patentsPage.IsRowSelected(absIdx)
			},
			IsRowMarked: func(absIdx int) bool {
				idx := absIdx - offset
				return idx >= 0 && idx < len(o.patents) && o.patents[idx].Number == o.patent.Number
			},
		}, targetW, func(_ int, rowIdx, colIdx int) string {
			if rowIdx < 0 || rowIdx >= len(o.patents) {
				return ""
			}
			p := o.patents[rowIdx]
			switch cols[colIdx].Key {
			case "number":
				if !p.DisplayNumber.IsZero() {
					return p.DisplayNumber.String()
				}
				return p.Number.String()
			case "title":
				return strings.Join(strings.Fields(p.Title), " ")
			case "inventor":
				return formatInventorsShort(p.Inventors)
			case "year":
				if !p.PublicationDate.IsZero() {
					return strconv.Itoa(p.PublicationDate.Year())
				}
				return "-"
			case "tags":
				if len(p.Tags) > 0 {
					return strings.Join(p.Tags, " ")
				}
				return "-"
			case "state":
				if o.project != "" {
					return o.theme.ReviewStateGlyph(p.ReviewState)
				}
				return o.theme.FetchStateGlyph(p.FetchState)
			default:
				return ""
			}
		})
		b.WriteString(normalizeOverlayContent(tableStr, targetW))
	}

	b.WriteString("\n")
	if o.focus == focusAssignees {
		status := fmt.Sprintf("[%d/%d]", o.selected+1, len(o.stats))
		b.WriteString(o.theme.Dim.Render(render.Truncate(fmt.Sprintf("%s  [Tab/l/→/Enter] Focus Patents  [j/k/↑/↓] Select Assignee  [q/Q/Esc] Close", status), targetW)))
	} else {
		status := subtableStatus(o.patentsPage)
		footnote := fmt.Sprintf("%s  [Tab/h/←] Focus Assignees  [j/k/↑/↓] Scroll  [l/Enter] View  [v] Visual  [ga] All  [←/→] Focus Col  [.] Sort  [ctrl+u/d] Page  [s/r/i/x] Review  [t] Tag  [I] IDS  [q/Q/Esc] Close", status)
		if o.patentsPage.VisualMode() {
			footnote = fmt.Sprintf("%s VISUAL MODE  [j/k/↑/↓] Select  [ga] All  [s/r/i/x] Review  [t] Tag  [I] IDS  [v/q/Q/Esc] Clear", status)
		}
		b.WriteString(o.theme.Dim.Render(render.Truncate(footnote, targetW)))
	}
	return normalizeOverlayContent(b.String(), targetW)
}

func (o *AssigneeStatsOverlay) loadStatsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var res proto.PatentAssigneeStatsResult
		if err := o.client.Call(ctx, proto.MethodPatentAssigneeStats, proto.PatentAssigneeStatsParams{Project: o.project}, &res); err != nil {
			return loadedAssigneeStatsMsg{err: err}
		}
		return loadedAssigneeStatsMsg{stats: res.Stats}
	}
}

func (o *AssigneeStatsOverlay) reloadPatentsCmd() tea.Cmd {
	o.loadSeq++
	o.loadID = o.loadSeq
	o.patentsLoading = true
	o.patentsErr = nil

	assignee := ""
	if len(o.stats) > 0 && o.selected >= 0 && o.selected < len(o.stats) {
		assignee = o.stats[o.selected].Assignee
	}
	return o.loadPatentsCmd(assignee, o.loadID)
}

func (o *AssigneeStatsOverlay) loadPatentsCmd(assignee string, requestID uint64) tea.Cmd {
	offset := o.patentsPage.Offset()
	limit := o.patentsPage.PageSize()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var res proto.PatentListResult
		err := o.client.Call(ctx, proto.MethodPatentList, proto.PatentListParams{Project: o.project, Assignee: assignee, Limit: limit, Offset: offset, SortColumn: o.activeSort, SortAscending: o.sortAscending}, &res)
		return loadedPatentListMsg{requestID: requestID, patents: res.Patents, total: res.Total, err: err}
	}
}

func (o *AssigneeStatsOverlay) calcHeights(innerHeight int) (statsH, patentsH int) {
	availH := innerHeight - 10
	if availH < 2 {
		availH = 2
	}
	statsH = max(1, availH/3)
	patentsH = max(1, availH-statsH)
	return statsH, patentsH
}

func (o *AssigneeStatsOverlay) clearVisual() {
	o.patentsPage.SaveVisual()
	o.patentsPage.ClearVisual()
}

func (o *AssigneeStatsOverlay) selections() []domain.PatentNumber {
	start, end, active := o.patentsPage.VisualRange()
	if !active || len(o.patents) == 0 {
		return nil
	}
	offset := o.patentsPage.Offset()
	lo := max(start, offset)
	hi := min(end, offset+len(o.patents)-1)
	if lo > hi {
		return nil
	}
	out := make([]domain.PatentNumber, 0, hi-lo+1)
	for abs := lo; abs <= hi; abs++ {
		out = append(out, o.patents[abs-offset].Number)
	}
	return out
}
