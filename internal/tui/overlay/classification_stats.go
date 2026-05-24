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

type loadedClassificationStatsMsg struct {
	stats []domain.ClassificationStats
	err   error
}

type ClassificationStatsOverlay struct {
	client   *rpc.Client
	theme    render.Theme
	catalog  *text.Catalog
	patent   domain.Patent
	project  domain.ProjectID
	stats    []domain.ClassificationStats
	selected int
	loading  bool
	err      error

	focus          statsFocus
	patents        []domain.PatentRow
	patentsPage    render.Paginator
	patentsLoading bool
	patentsErr     error
	loadSeq        uint64
	loadID         uint64

	activeSort    domain.SortColumn
	sortAscending bool
	focusedColIdx int
	lastWidth     int
	preselect     map[string]bool
}

func NewClassificationStatsOverlay(client *rpc.Client, theme render.Theme, catalog *text.Catalog, patent domain.Patent, project domain.ProjectID) (*ClassificationStatsOverlay, tea.Cmd) {
	preselect := make(map[string]bool, len(patent.Classifications))
	for _, code := range patent.Classifications {
		code = strings.TrimSpace(code)
		if code != "" {
			preselect[code] = true
		}
	}
	o := &ClassificationStatsOverlay{
		client:        client,
		theme:         theme,
		catalog:       catalog,
		patent:        patent,
		project:       project,
		loading:       true,
		focus:         focusInventors,
		patentsPage:   render.NewPaginator(5),
		activeSort:    domain.SortByNumber,
		sortAscending: true,
		focusedColIdx: -1,
		lastWidth:     90,
		preselect:     preselect,
	}
	return o, o.loadStatsCmd()
}

func (o *ClassificationStatsOverlay) Title() string { return "Classification Analytics" }

func (o *ClassificationStatsOverlay) Handles() []command.ID {
	return []command.ID{command.CloseOverlay}
}

func (o *ClassificationStatsOverlay) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if id == command.CloseOverlay {
		return o, func() tea.Msg { return CloseOverlayMsg{} }
	}
	return o, nil
}

func (o *ClassificationStatsOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case loadedClassificationStatsMsg:
		o.loading = false
		if m.err != nil {
			o.err = m.err
			return o, nil
		}
		o.stats = m.stats
		if o.selected >= len(o.stats) {
			o.selected = max(0, len(o.stats)-1)
		}
		if len(o.preselect) > 0 {
			for idx, stat := range o.stats {
				if o.preselect[stat.Classification.Code] {
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
			return o, o.loadPatentsCmd(o.stats[o.selected].Classification.Code, o.loadID)
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
				cmds = append(cmds, o.loadPatentsCmd(o.stats[o.selected].Classification.Code, o.loadID))
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
				return o, o.loadPatentsCmd(o.stats[o.selected].Classification.Code, o.loadID)
			}
		}
		return o, nil
	}
	return o, nil
}

func (o *ClassificationStatsOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	o.err = nil
	o.patentsErr = nil
	if o.focus == focusPatents && o.patentsPage.HandleKey(msg) {
		return o, nil, true
	}
	switch msg.String() {
	case "q", "Q", "esc":
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case "tab":
		if o.focus == focusInventors {
			o.focus = focusPatents
			o.focusedColIdx = 0
		} else {
			o.focus = focusInventors
			o.focusedColIdx = -1
		}
		return o, nil, true
	}
	if o.focus == focusInventors {
		switch msg.String() {
		case "j", "down":
			if len(o.stats) > 0 {
				o.selected = (o.selected + 1) % len(o.stats)
				o.patentsPage.Top()
				o.loadSeq++
				o.loadID = o.loadSeq
				o.patentsLoading = true
				return o, o.loadPatentsCmd(o.stats[o.selected].Classification.Code, o.loadID), true
			}
		case "k", "up":
			if len(o.stats) > 0 {
				o.selected = (o.selected - 1 + len(o.stats)) % len(o.stats)
				o.patentsPage.Top()
				o.loadSeq++
				o.loadID = o.loadSeq
				o.patentsLoading = true
				return o, o.loadPatentsCmd(o.stats[o.selected].Classification.Code, o.loadID), true
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
		o.focus = focusInventors
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
				return o, o.loadPatentsCmd(o.stats[o.selected].Classification.Code, o.loadID), true
			}
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
	case "j", "down":
		before := o.patentsPage.Offset()
		o.patentsPage.MoveDown(1)
		if o.patentsPage.Offset() != before {
			o.loadSeq++
			o.loadID = o.loadSeq
			o.patentsLoading = true
			return o, o.loadPatentsCmd(o.stats[o.selected].Classification.Code, o.loadID), true
		}
		return o, nil, true
	case "k", "up":
		before := o.patentsPage.Offset()
		o.patentsPage.MoveUp(1)
		if o.patentsPage.Offset() != before {
			o.loadSeq++
			o.loadID = o.loadSeq
			o.patentsLoading = true
			return o, o.loadPatentsCmd(o.stats[o.selected].Classification.Code, o.loadID), true
		}
		return o, nil, true
	case "ctrl+d":
		before := o.patentsPage.Offset()
		o.patentsPage.PageDown()
		if o.patentsPage.Offset() != before {
			o.loadSeq++
			o.loadID = o.loadSeq
			o.patentsLoading = true
			return o, o.loadPatentsCmd(o.stats[o.selected].Classification.Code, o.loadID), true
		}
		return o, nil, true
	case "ctrl+u":
		before := o.patentsPage.Offset()
		o.patentsPage.PageUp()
		if o.patentsPage.Offset() != before {
			o.loadSeq++
			o.loadID = o.loadSeq
			o.patentsLoading = true
			return o, o.loadPatentsCmd(o.stats[o.selected].Classification.Code, o.loadID), true
		}
		return o, nil, true
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
	}
	return o, nil, true
}

func (o *ClassificationStatsOverlay) OverlaySize(termW, termH int) (w, h int) {
	return PctSize(termW, termH, 90, 90, 96, 22)
}

func (o *ClassificationStatsOverlay) currentCols() []render.TableColumn {
	return (&AssigneeStatsOverlay{lastWidth: o.lastWidth}).currentCols()
}

func (o *ClassificationStatsOverlay) View(maxW, maxH int) string {
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
		b.WriteString(o.theme.MutedItalic.Render("Loading classification statistics..."))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("[q/Esc] Close"))
		return b.String()
	}
	titleText := fmt.Sprintf("  Classification Stats (%d codes)", len(o.stats))
	if !o.patent.DisplayNumber.IsZero() {
		titleText = fmt.Sprintf("  Classification Stats for Patent %s (%d codes)", o.patent.DisplayNumber.String(), len(o.stats))
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(titleText, targetW)))
	b.WriteString("\n\n")
	if len(o.stats) == 0 {
		b.WriteString(o.theme.MutedItalic.Render("No classifications found."))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("[q/Esc] Close"))
		return b.String()
	}
	statsH, patentsH := o.calcHeights(maxH)
	o.patentsPage.SetPageSize(patentsH)
	maxNameLen := 0
	for _, s := range o.stats {
		label := classificationStatsLabel(s.Classification)
		if len(label) > maxNameLen {
			maxNameLen = len(label)
		}
	}
	startStats := max(0, o.selected-statsH/2)
	endStats := min(len(o.stats), startStats+statsH)
	if endStats-startStats < statsH && startStats > 0 {
		startStats = max(0, endStats-statsH)
	}
	for i := startStats; i < endStats; i++ {
		s := o.stats[i]
		prefix := "  "
		if i == o.selected && o.focus == focusInventors {
			prefix = "→ "
		}
		label := render.Pad(classificationStatsLabel(s.Classification), maxNameLen+2)
		line := fmt.Sprintf("%s%s%s", prefix, label, render.FormatEntityStats(s.Total, s.States, s.Tags))
		if i == o.selected && o.focus == focusInventors {
			b.WriteString(o.theme.Selected.Render(render.Truncate(line, targetW)))
		} else if i%2 == 1 {
			b.WriteString(o.theme.RowAlt.Render(render.Truncate(line, targetW)))
		} else {
			b.WriteString(o.theme.Row.Render(render.Truncate(line, targetW)))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	dividerText := fmt.Sprintf("─── Patents by Selected Classification (%s) ───", o.stats[o.selected].Classification.Code)
	if dashCount := targetW - len(dividerText); dashCount > 0 {
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
		b.WriteString(o.theme.MutedItalic.Render("No patents found for this classification."))
		b.WriteString("\n")
	} else {
		cols := o.currentCols()
		startPat, endPat := o.patentsPage.Window()
		params := render.TableParams{Theme: o.theme, Columns: cols, RowCount: endPat - startPat, Cursor: o.patentsPage.Cursor() - startPat, FocusedColIdx: o.focusedColIdx, ActiveSort: string(o.activeSort), SortAscending: o.sortAscending, FocusActive: o.focus == focusPatents, PrefixCursor: "→ ", PrefixNormal: "  ", VisualMode: o.patentsPage.VisualMode(), IsRowSelected: func(rowIdx int) bool { return o.patentsPage.IsRowSelected(startPat + rowIdx) }, IsRowMarked: func(rowIdx int) bool {
			absIdx := startPat + rowIdx
			return absIdx >= 0 && absIdx < len(o.patents) && o.patents[absIdx].Number == o.patent.Number
		}}
		tableStr := render.RenderTable(params, targetW, func(rowIdx, colIdx int) string {
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
					return o.theme.ReviewStateGlyph(string(p.ReviewState))
				}
				return o.theme.FetchStateGlyph(string(p.FetchState))
			default:
				return ""
			}
		})
		b.WriteString(tableStr)
	}
	b.WriteString("\n")
	if o.focus == focusInventors {
		status := fmt.Sprintf("[%d/%d]", o.selected+1, len(o.stats))
		b.WriteString(o.theme.Dim.Render(render.Truncate(fmt.Sprintf("%s  [Tab/l/→/Enter] Focus Patents  [j/k/↑/↓] Select Classification  [q/Q/Esc] Close", status), targetW)))
	} else {
		status := "[0/0]"
		if o.patentsPage.Total() > 0 {
			status = fmt.Sprintf("[%d/%d]", o.patentsPage.Cursor()+1, o.patentsPage.Total())
		}
		footnote := fmt.Sprintf("%s  [Tab/h/←] Focus Classifications  [j/k/↑/↓] Scroll  [l/Enter] View  [v] Visual  [←/→] Focus Col  [.] Sort  [ctrl+u/d] Page  [s/r/i/x] Review  [t] Tag  [q/Q/Esc] Close", status)
		if o.patentsPage.VisualMode() {
			footnote = fmt.Sprintf("%s VISUAL MODE  [j/k/↑/↓] Select  [s/r/i/x] Review  [t] Tag  [v/q/Q/Esc] Clear", status)
		}
		b.WriteString(o.theme.Dim.Render(render.Truncate(footnote, targetW)))
	}
	return b.String()
}

func (o *ClassificationStatsOverlay) loadStatsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var res proto.PatentClassificationStatsResult
		if err := o.client.Call(ctx, proto.MethodPatentClassificationStats, proto.PatentClassificationStatsParams{Project: o.project}, &res); err != nil {
			return loadedClassificationStatsMsg{err: err}
		}
		return loadedClassificationStatsMsg{stats: res.Stats}
	}
}

func (o *ClassificationStatsOverlay) loadPatentsCmd(code string, requestID uint64) tea.Cmd {
	offset := o.patentsPage.Offset()
	limit := o.patentsPage.PageSize()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var res proto.PatentListResult
		err := o.client.Call(ctx, proto.MethodPatentList, proto.PatentListParams{Project: o.project, ClassificationCode: code, Limit: limit, Offset: offset, SortColumn: o.activeSort, SortAscending: o.sortAscending}, &res)
		return loadedPatentListMsg{requestID: requestID, patents: res.Patents, total: res.Total, err: err}
	}
}

func (o *ClassificationStatsOverlay) calcHeights(innerHeight int) (statsH, patentsH int) {
	availH := innerHeight - 10
	if availH < 2 {
		availH = 2
	}
	statsH = max(1, availH/3)
	patentsH = max(1, availH-statsH)
	return statsH, patentsH
}

func (o *ClassificationStatsOverlay) clearVisual() {
	o.patentsPage.SaveVisual()
	o.patentsPage.ClearVisual()
}

func (o *ClassificationStatsOverlay) selections() []domain.PatentNumber {
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

func classificationStatsLabel(c domain.Classification) string {
	if c.Description == "" {
		return c.Code
	}
	return c.Code + " - " + c.Description
}
