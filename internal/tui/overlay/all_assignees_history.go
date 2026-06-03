package overlay

import (
	"context"
	"fmt"
	"sort"
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

type loadedAllAssigneesHistoryMsg struct {
	stats []domain.AllAssigneesHistory
	err   error
}

// AllAssigneesHistoryOverlay presents interactive statistics for all assignees.
type AllAssigneesHistoryOverlay struct {
	client   *rpc.Client
	theme    render.Theme
	catalog  *text.Catalog
	patent   domain.Patent
	project  domain.ProjectID
	stats    []domain.AllAssigneesHistory
	allStats []domain.AllAssigneesHistory
	selected int
	loading  bool
	err      error

	searchActive bool
	searchQuery  string
	searchScope  int

	patentsSearchActive bool
	patentsSearchQuery  string

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

	statsSortCol       string
	statsSortAsc       bool
	statsFocusedColIdx int
	filterToPatent     bool
}

func NewAllAssigneesHistoryOverlay(client *rpc.Client, theme render.Theme, catalog *text.Catalog, patent domain.Patent, project domain.ProjectID, filterToPatent bool) (*AllAssigneesHistoryOverlay, tea.Cmd) {
	o := &AllAssigneesHistoryOverlay{
		client:         client,
		theme:          theme,
		catalog:        catalog,
		patent:         patent,
		project:        project,
		loading:        true,
		focus:          focusAssignees,
		patentsPage:    render.NewPaginator(5),
		activeSort:     domain.SortByNumber,
		sortAscending:  true,
		focusedColIdx:  -1,
		lastWidth:      90,
		preselect:      strings.TrimSpace(patent.Assignee),
		statsSortCol:   "patents",
		statsSortAsc:   false,
		statsFocusedColIdx: 1,
		filterToPatent:     filterToPatent,
	}
	return o, o.loadStatsCmd()
}

const focusAssignees = focusInventors

func (o *AllAssigneesHistoryOverlay) Title() string { return "Assignee Analytics" }

func (o *AllAssigneesHistoryOverlay) Handles() []command.ID { return []command.ID{command.CloseOverlay} }

func (o *AllAssigneesHistoryOverlay) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if id == command.CloseOverlay {
		return o, func() tea.Msg { return CloseOverlayMsg{} }
	}
	return o, nil
}

func (o *AllAssigneesHistoryOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case loadedAllAssigneesHistoryMsg:
		o.loading = false
		if m.err != nil {
			o.err = m.err
			return o, nil
		}
		o.allStats = m.stats
		o.applyFilter()
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
		o.statsFocusedColIdx = clampFocusedStatsColumn(o.currentStatsCols(), o.statsFocusedColIdx)
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

func (o *AllAssigneesHistoryOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	o.err = nil
	o.patentsErr = nil

	if o.searchActive {
		switch msg.Type {
		case tea.KeyTab:
			o.searchScope = (o.searchScope + 1) % len(assigneeSearchScopes)
			o.applyFilter()
			return o, o.reloadPatentsCmd(), true
		case tea.KeyEsc:
			o.searchActive = false
			o.searchQuery = ""
			o.applyFilter()
			return o, o.reloadPatentsCmd(), true
		case tea.KeyEnter:
			o.searchActive = false
			return o, nil, true
		case tea.KeyBackspace, tea.KeyDelete:
			if len(o.searchQuery) > 0 {
				runes := []rune(o.searchQuery)
				o.searchQuery = string(runes[:len(runes)-1])
				o.applyFilter()
				return o, o.reloadPatentsCmd(), true
			}
			return o, nil, true
		case tea.KeyCtrlW:
			o.searchQuery = ""
			o.applyFilter()
			return o, o.reloadPatentsCmd(), true
		case tea.KeyRunes, tea.KeySpace:
			o.searchQuery += msg.String()
			o.applyFilter()
			return o, o.reloadPatentsCmd(), true
		}
		return o, nil, true
	}

	if o.patentsSearchActive {
		switch msg.Type {
		case tea.KeyEsc:
			o.patentsSearchActive = false
			o.patentsSearchQuery = ""
			return o, o.reloadPatentsCmd(), true
		case tea.KeyEnter:
			o.patentsSearchActive = false
			return o, nil, true
		case tea.KeyBackspace, tea.KeyDelete:
			if len(o.patentsSearchQuery) > 0 {
				runes := []rune(o.patentsSearchQuery)
				o.patentsSearchQuery = string(runes[:len(runes)-1])
				return o, o.reloadPatentsCmd(), true
			}
			return o, nil, true
		case tea.KeyCtrlW:
			o.patentsSearchQuery = ""
			return o, o.reloadPatentsCmd(), true
		case tea.KeyRunes, tea.KeySpace:
			o.patentsSearchQuery += msg.String()
			return o, o.reloadPatentsCmd(), true
		}
		return o, nil, true
	}

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
		case "l", "enter":
			o.focus = focusPatents
			o.focusedColIdx = 0
			return o, nil, true
		case "/":
			o.searchActive = true
			o.searchQuery = ""
			o.applyFilter()
			return o, o.reloadPatentsCmd(), true
		case "left":
			statsCols := o.currentStatsCols()
			o.statsFocusedColIdx = moveStatsColumn(statsCols, o.statsFocusedColIdx, -1)
			return o, nil, true
		case "right":
			statsCols := o.currentStatsCols()
			o.statsFocusedColIdx = moveStatsColumn(statsCols, o.statsFocusedColIdx, 1)
			return o, nil, true
		case ".":
			if len(o.stats) > 0 {
				statsCols := o.currentStatsCols()
				colIdx := o.statsFocusedColIdx
				if colIdx < 0 || colIdx >= len(statsCols) {
					colIdx = 1
					for idx, c := range statsCols {
						if c.SortKey == o.statsSortCol {
							colIdx = idx
							break
						}
					}
				}
				col := statsCols[colIdx]
				if col.SortKey != "" {
					if o.statsSortCol == col.SortKey {
						o.statsSortAsc = !o.statsSortAsc
					} else {
						o.statsSortCol = col.SortKey
						o.statsSortAsc = true
					}
					o.sortStats()
					o.patentsPage.Top()
					o.loadSeq++
					o.loadID = o.loadSeq
					o.patentsLoading = true
					return o, o.loadPatentsCmd(o.stats[o.selected].Assignee, o.loadID), true
				}
			}
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

	if msg.String() == "/" {
		o.patentsSearchActive = true
		o.patentsSearchQuery = ""
		return o, o.reloadPatentsCmd(), true
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

func (o *AllAssigneesHistoryOverlay) OverlaySize(termW, termH int) (w, h int) {
	return PctSize(termW, termH, 90, 90, 90, 22)
}

func (o *AllAssigneesHistoryOverlay) currentCols() []render.TableColumn {
	w := o.lastWidth - statsTableMargin
	if w < 40 {
		w = 40
	}
	switch {
	case w >= 80:
		fixedWidth := 75
		titleWidth := max(10, w-fixedWidth)
		return []render.TableColumn{
			{Key: "number", Label: "Number", SortKey: string(domain.SortByNumber), Width: 16},
			{Key: "kind", Label: "Kind", SortKey: string(domain.SortByKind), Width: 4},
			{Key: "title", Label: "Title", SortKey: string(domain.SortByTitle), Width: titleWidth},
			{Key: "inventor", Label: "Inventor", SortKey: string(domain.SortByInventor), Width: 16},
			{Key: "year", Label: "Year", SortKey: string(domain.SortByExpires), Width: 4},
			{Key: "tags", Label: "Tags", SortKey: string(domain.SortByTags), Width: 15},
			{Key: "state", Label: "State", SortKey: string(domain.SortByReviewState), Width: 12},
		}
	case w >= 65:
		fixedWidth := 59
		titleWidth := max(10, w-fixedWidth)
		return []render.TableColumn{
			{Key: "number", Label: "Number", SortKey: string(domain.SortByNumber), Width: 16},
			{Key: "kind", Label: "Kind", SortKey: string(domain.SortByKind), Width: 4},
			{Key: "title", Label: "Title", SortKey: string(domain.SortByTitle), Width: titleWidth},
			{Key: "inventor", Label: "Inventor", SortKey: string(domain.SortByInventor), Width: 16},
			{Key: "year", Label: "Year", SortKey: string(domain.SortByExpires), Width: 4},
			{Key: "state", Label: "State", SortKey: string(domain.SortByReviewState), Width: 12},
		}
	case w >= 55:
		fixedWidth := 42
		titleWidth := max(10, w-fixedWidth)
		return []render.TableColumn{
			{Key: "number", Label: "Number", SortKey: string(domain.SortByNumber), Width: 16},
			{Key: "kind", Label: "Kind", SortKey: string(domain.SortByKind), Width: 4},
			{Key: "title", Label: "Title", SortKey: string(domain.SortByTitle), Width: titleWidth},
			{Key: "year", Label: "Year", SortKey: string(domain.SortByExpires), Width: 4},
			{Key: "state", Label: "State", SortKey: string(domain.SortByReviewState), Width: 12},
		}
	default:
		fixedWidth := 37
		titleWidth := max(10, w-fixedWidth)
		return []render.TableColumn{
			{Key: "number", Label: "Number", SortKey: string(domain.SortByNumber), Width: 16},
			{Key: "kind", Label: "Kind", SortKey: string(domain.SortByKind), Width: 4},
			{Key: "title", Label: "Title", SortKey: string(domain.SortByTitle), Width: titleWidth},
			{Key: "state", Label: "State", SortKey: string(domain.SortByReviewState), Width: 12},
		}
	}
}

func (o *AllAssigneesHistoryOverlay) currentStatsCols() []render.TableColumn {
	maxNameLen := 8
	for _, s := range o.stats {
		if len(s.Assignee) > maxNameLen {
			maxNameLen = len(s.Assignee)
		}
	}
	nameColWidth := maxNameLen + 2
	targetW := o.lastWidth - statsTableMargin
	if nameColWidth > targetW-35 {
		nameColWidth = max(15, targetW-35)
	}
	fixedColWidths := nameColWidth + 9 + 4 + 4 + 4 + 4
	tagsColWidth := targetW - 10 - fixedColWidths
	if tagsColWidth < 10 {
		tagsColWidth = 10
	}
	return []render.TableColumn{
		{Key: "name", Label: "Assignee", SortKey: "name", Width: nameColWidth},
		{Key: "total", Label: "Total", SortKey: "patents", Width: 9},
		{Key: "unknown", Label: o.theme.Glyphs.ReviewStateUnknown, SortKey: "unknown", Width: 4},
		{Key: "under_review", Label: o.theme.Glyphs.ReviewStateUnderReview, SortKey: "under_review", Width: 4},
		{Key: "active", Label: o.theme.Glyphs.ReviewStateActive, SortKey: "active", Width: 4},
		{Key: "ignored", Label: o.theme.Glyphs.ReviewStateIgnored, SortKey: "ignored", Width: 4},
		{Key: "tags", Label: "Tags", SortKey: "tags", Width: tagsColWidth},
	}
}

func (o *AllAssigneesHistoryOverlay) View(maxW, maxH int) string {
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

	statsCols := o.currentStatsCols()
	startStats := max(0, o.selected-statsH/2)
	endStats := min(len(o.stats), startStats+statsH)
	if endStats-startStats < statsH && startStats > 0 {
		startStats = max(0, endStats-statsH)
	}

	statsTableStr := render.RenderTable(render.TableParams{
		Theme:         o.theme,
		Columns:       statsCols,
		RowCount:      endStats - startStats,
		FocusedColIdx: o.statsFocusedColIdx,
		ActiveSort:    o.statsSortCol,
		SortAscending: o.statsSortAsc,
		FocusActive:   o.focus == focusAssignees,
		IsRowCursor: func(rowIdx int) bool {
			return startStats+rowIdx == o.selected
		},
	}, targetW, func(rowIdx, colIdx int) string {
		absIdx := startStats + rowIdx
		if absIdx < 0 || absIdx >= len(o.stats) {
			return ""
		}
		s := o.stats[absIdx]
		switch statsCols[colIdx].Key {
		case "name":
			return s.Assignee
		case "total":
			return strconv.Itoa(s.Total)
		case "unknown":
			return strconv.Itoa(s.States["unknown"])
		case "under_review":
			return strconv.Itoa(s.States["under_review"])
		case "active":
			return strconv.Itoa(s.States["active"])
		case "ignored":
			return strconv.Itoa(s.States["ignored"])
		case "tags":
			if val := render.FormatTagsForSort(s.Tags); val != "" {
				return val
			}
			return "-"
		default:
			return ""
		}
	})
	b.WriteString(statsTableStr)

	b.WriteString("\n")
	dividerText := fmt.Sprintf("─── Patents by Selected Assignee (%s) ───", o.stats[o.selected].Assignee)
	dashCount := targetW - len(dividerText)
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
		cols := o.currentCols()
		offset := o.patentsPage.Offset()
		tableStr := renderSubtable(subtableParams{
			Theme:         o.theme,
			Columns:       cols,
			Page:          &o.patentsPage,
			Total:         o.patentsPage.Total(),
			PageSize:      patentsH,
			FocusedColIdx: o.focusedColIdx,
			ActiveSort:    string(o.activeSort),
			SortAscending: o.sortAscending,
			FocusActive:   o.focus == focusPatents,
			VisualMode:    o.patentsPage.VisualMode(),
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
			case "kind":
				num := p.Number
				if !p.DisplayNumber.IsZero() {
					num = p.DisplayNumber
				}
				return o.theme.StageGlyph(string(num.Stage()))
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
	if o.searchActive {
		searchLine := "/ " + o.searchQuery + "▋  [Scope: " + assigneeSearchScopes[o.searchScope].Label + " (Tab to cycle)]"
		b.WriteString(o.theme.Selected.Render(render.Pad(searchLine, targetW)))
	} else if o.patentsSearchActive {
		searchLine := "/ " + o.patentsSearchQuery + "▋ (patents)"
		b.WriteString(o.theme.Selected.Render(render.Pad(searchLine, targetW)))
	} else if o.focus == focusAssignees {
		status := fmt.Sprintf("[%d/%d]", o.selected+1, len(o.stats))
		b.WriteString(o.theme.Dim.Render(render.Truncate(fmt.Sprintf("%s  [Tab/l/Enter] Focus Patents  [/] Search  [j/k/↑/↓] Scroll  [←/→] Focus Col  [.] Sort  [q/Q/Esc] Close", status), targetW)))
	} else {
		status := subtableStatus(o.patentsPage)
		footnote := fmt.Sprintf("%s  [Tab/h/←] Focus Assignees  [j/k/↑/↓] Scroll  [l/Enter] View  [v] Visual  [ga] All  [←/→] Focus Col  [.] Sort  [/] Search  [ctrl+u/d] Page  [s/r/i/x] Review  [t] Tag  [I] IDS  [q/Q/Esc] Close", status)
		if o.patentsPage.VisualMode() {
			footnote = fmt.Sprintf("%s VISUAL MODE  [j/k/↑/↓] Select  [ga] All  [s/r/i/x] Review  [t] Tag  [I] IDS  [v/q/Q/Esc] Clear", status)
		}
		b.WriteString(o.theme.Dim.Render(render.Truncate(footnote, targetW)))
	}
	return b.String()
}

func (o *AllAssigneesHistoryOverlay) loadStatsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var res proto.AllAssigneesHistoryResult
		if err := o.client.Call(ctx, proto.MethodAllAssigneesHistory, proto.AllAssigneesHistoryParams{Project: o.project}, &res); err != nil {
			return loadedAllAssigneesHistoryMsg{err: err}
		}

		if o.filterToPatent && !o.patent.Number.IsZero() {
			allowedAssignees := make(map[string]bool)
			if name := strings.TrimSpace(o.patent.Assignee); name != "" {
				allowedAssignees[strings.ToLower(name)] = true
			}

			var hist proto.AssigneeHistoryResult
			if err := o.client.Call(ctx, proto.MethodAssigneeHistory, proto.AssigneeHistoryParams{Number: o.patent.Number}, &hist); err == nil {
				for _, entry := range hist.Entries {
					if name := strings.TrimSpace(entry.AssigneeName); name != "" {
						allowedAssignees[strings.ToLower(name)] = true
					}
				}
			}

			var filtered []domain.AllAssigneesHistory
			for _, s := range res.Stats {
				if allowedAssignees[strings.ToLower(s.Assignee)] {
					filtered = append(filtered, s)
				}
			}

			if len(filtered) == 0 {
				for name := range allowedAssignees {
					origName := name
					if strings.ToLower(o.patent.Assignee) == name {
						origName = o.patent.Assignee
					}
					found := false
					for _, f := range filtered {
						if strings.ToLower(f.Assignee) == name {
							found = true
							break
						}
					}
					if !found && origName != "" {
						filtered = append(filtered, domain.AllAssigneesHistory{
							Assignee: origName,
							Total:    1,
							States:   map[string]int{"unknown": 1},
						})
					}
				}
			}
			return loadedAllAssigneesHistoryMsg{stats: filtered}
		}

		return loadedAllAssigneesHistoryMsg{stats: res.Stats}
	}
}

func (o *AllAssigneesHistoryOverlay) reloadPatentsCmd() tea.Cmd {
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

func (o *AllAssigneesHistoryOverlay) loadPatentsCmd(assignee string, requestID uint64) tea.Cmd {
	offset := o.patentsPage.Offset()
	limit := o.patentsPage.PageSize()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var res proto.PatentListResult
		err := o.client.Call(ctx, proto.MethodPatentList, proto.PatentListParams{Project: o.project, Assignee: assignee, Limit: limit, Offset: offset, SortColumn: o.activeSort, SortAscending: o.sortAscending, Search: o.patentsSearchQuery}, &res)
		return loadedPatentListMsg{requestID: requestID, patents: res.Patents, total: res.Total, err: err}
	}
}

func (o *AllAssigneesHistoryOverlay) calcHeights(innerHeight int) (statsH, patentsH int) {
	availH := innerHeight - 10
	if availH < 2 {
		availH = 2
	}
	statsH = max(1, availH/3)
	patentsH = max(1, availH-statsH)
	return statsH, patentsH
}

func (o *AllAssigneesHistoryOverlay) clearVisual() {
	o.patentsPage.SaveVisual()
	o.patentsPage.ClearVisual()
}

func (o *AllAssigneesHistoryOverlay) selections() []domain.PatentNumber {
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

func (o *AllAssigneesHistoryOverlay) sortStats() {
	if len(o.stats) == 0 {
		return
	}
	var selectedAssignee string
	if o.selected >= 0 && o.selected < len(o.stats) {
		selectedAssignee = o.stats[o.selected].Assignee
	}

	sort.SliceStable(o.stats, func(i, j int) bool {
		var cmp bool
		var equal bool

		switch o.statsSortCol {
		case "name":
			cmp = o.stats[i].Assignee < o.stats[j].Assignee
			equal = o.stats[i].Assignee == o.stats[j].Assignee
		case "total", "patents":
			cmp = o.stats[i].Total < o.stats[j].Total
			equal = o.stats[i].Total == o.stats[j].Total
		case "unknown":
			valI := o.stats[i].States["unknown"]
			valJ := o.stats[j].States["unknown"]
			cmp = valI < valJ
			equal = valI == valJ
		case "under_review":
			valI := o.stats[i].States["under_review"]
			valJ := o.stats[j].States["under_review"]
			cmp = valI < valJ
			equal = valI == valJ
		case "active":
			valI := o.stats[i].States["active"]
			valJ := o.stats[j].States["active"]
			cmp = valI < valJ
			equal = valI == valJ
		case "ignored":
			valI := o.stats[i].States["ignored"]
			valJ := o.stats[j].States["ignored"]
			cmp = valI < valJ
			equal = valI == valJ
		case "tags":
			strI := render.FormatTagsForSort(o.stats[i].Tags)
			strJ := render.FormatTagsForSort(o.stats[j].Tags)
			cmp = strI < strJ
			equal = strI == strJ
		default:
			cmp = o.stats[i].Total < o.stats[j].Total
			equal = o.stats[i].Total == o.stats[j].Total
		}

		if equal {
			return o.stats[i].Assignee < o.stats[j].Assignee
		}

		if o.statsSortAsc {
			return cmp
		}
		return !cmp
	})

	if selectedAssignee != "" {
		for idx, stat := range o.stats {
			if stat.Assignee == selectedAssignee {
				o.selected = idx
				break
			}
		}
	}
}

var assigneeSearchScopes = []struct {
	Key   string
	Label string
}{
	{"all", "All Columns"},
	{"name", "Name"},
	{"tags", "Tags"},
	{"states", "States"},
}

func (o *AllAssigneesHistoryOverlay) applyFilter() {
	if o.searchQuery == "" {
		o.stats = o.allStats
	} else {
		o.stats = nil
		q := strings.ToLower(o.searchQuery)
		scope := assigneeSearchScopes[o.searchScope].Key
		for _, s := range o.allStats {
			match := false
			switch scope {
			case "all":
				match = strings.Contains(strings.ToLower(s.Assignee), q) ||
					strings.Contains(strings.ToLower(render.FormatTagsForSort(s.Tags)), q) ||
					strings.Contains(strings.ToLower(fmt.Sprintf("%d %d %d %d", s.States["unknown"], s.States["under_review"], s.States["active"], s.States["ignored"])), q)
			case "name":
				match = strings.Contains(strings.ToLower(s.Assignee), q)
			case "tags":
				match = strings.Contains(strings.ToLower(render.FormatTagsForSort(s.Tags)), q)
			case "states":
				match = strings.Contains(strings.ToLower(fmt.Sprintf("unknown:%d under_review:%d active:%d ignored:%d", s.States["unknown"], s.States["under_review"], s.States["active"], s.States["ignored"])), q)
			}
			if match {
				o.stats = append(o.stats, s)
			}
		}
	}
	o.sortStats()
	if o.selected >= len(o.stats) {
		o.selected = max(0, len(o.stats)-1)
	}
}
