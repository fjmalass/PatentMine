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
	allStats []domain.ClassificationStats
	selected int
	loading  bool
	err      error

	searchActive bool
	searchQuery  string
	searchScope  int

	patentsSearchActive bool
	patentsSearchQuery  string

	focus          statsFocus
	patents        []domain.PatentRow
	patentsPage    render.Paginator
	patentsLoading bool
	patentsErr     error
	loadSeq        uint64
	loadID         uint64

	activeSort         domain.SortColumn
	sortAscending      bool
	focusedColIdx      int
	lastWidth          int
	preselect          map[string]bool
	statsSortCol       string
	statsSortAsc       bool
	statsFocusedColIdx int
	jump               JumpNavigator
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
		client:             client,
		theme:              theme,
		catalog:            catalog,
		patent:             patent,
		project:            project,
		loading:            true,
		focus:              focusStats,
		patentsPage:        render.NewPaginator(5),
		activeSort:         domain.SortByNumber,
		sortAscending:      true,
		focusedColIdx:      -1,
		lastWidth:          90,
		preselect:          preselect,
		statsSortCol:       "patents",
		statsSortAsc:       false,
		statsFocusedColIdx: 1,
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
		o.allStats = m.stats
		o.applyFilter()
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
		o.statsFocusedColIdx = clampFocusedStatsColumn(o.currentStatsCols(), o.statsFocusedColIdx)
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

	if !o.searchActive && !o.patentsSearchActive {
		var currentFocus, numFields int
		if o.focus == focusStats {
			currentFocus = o.selected
			numFields = len(o.stats)
		} else {
			currentFocus = o.patentsPage.Cursor()
			numFields = o.patentsPage.Total()
		}
		if newCursor, handled := o.jump.HandleKey(msg, currentFocus, numFields); handled {
			if o.focus == focusStats {
				o.selected = newCursor
				o.patentsLoading = true
				o.patentsErr = nil
				return o, o.loadPatentsCmd(o.stats[o.selected].Classification.Code, o.loadSeq), true // loadSeq is fine or loadID
			} else {
				o.patentsPage.ScrollTo(newCursor)
				return o, nil, true
			}
		}
	}

	if o.searchActive {
		switch msg.Type {
		case tea.KeyTab:
			o.searchScope = (o.searchScope + 1) % len(classificationSearchScopes)
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

	if o.focus == focusPatents && o.patentsPage.HandleKey(msg) {
		return o, nil, true
	}
	switch msg.String() {
	case ";":
		if !o.searchActive && !o.patentsSearchActive {
			o.jump.Active = true
			o.jump.PendingCount = 0
			o.jump.PendingG = false
			return o, nil, true
		}
	case "q", "Q", "esc":
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case "tab":
		if o.focus == focusStats {
			o.focus = focusPatents
			o.focusedColIdx = 0
		} else {
			o.focus = focusStats
			o.focusedColIdx = -1
		}
		return o, nil, true
	}
	if o.focus == focusStats {
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
		case "enter":
			o.focus = focusPatents
			o.focusedColIdx = 0
			return o, nil, true
		case "/":
			o.searchActive = true
			o.searchQuery = ""
			o.applyFilter()
			return o, o.reloadPatentsCmd(), true
		case "left", "l":
			statsCols := o.currentStatsCols()
			o.statsFocusedColIdx = moveStatsColumn(statsCols, o.statsFocusedColIdx, -1)
			return o, nil, true
		case "right", "h":
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
					return o, o.loadPatentsCmd(o.stats[o.selected].Classification.Code, o.loadID), true
				}
			}
		}
		return o, nil, true
	}
	switch msg.String() {
	case "left", "h":
		o.focusedColIdx = moveStatsColumn(o.currentCols(), o.focusedColIdx, -1)
		return o, nil, true
	case "right", "l":
		o.focusedColIdx = moveStatsColumn(o.currentCols(), o.focusedColIdx, 1)
		return o, nil, true
	// case "h":
	// 	o.focus = focusStats
	// 	o.focusedColIdx = -1
	// 	return o, nil, true
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
	case "enter":
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

func (o *ClassificationStatsOverlay) OverlaySize(termW, termH int) (w, h int) {
	return PctSize(termW, termH, 90, 90, 96, 22)
}

func (o *ClassificationStatsOverlay) currentCols() []render.TableColumn {
	return (&AllAssigneesHistoryOverlay{lastWidth: o.lastWidth}).currentCols()
}

func (o *ClassificationStatsOverlay) currentStatsCols() []render.TableColumn {
	maxNameLen := 8
	for _, s := range o.stats {
		label := classificationStatsLabel(s.Classification)
		if len(label) > maxNameLen {
			maxNameLen = len(label)
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
		{Key: "name", Label: "Classification", SortKey: "name", Width: nameColWidth},
		{Key: "total", Label: "Total", SortKey: "patents", Width: 9},
		{Key: "unknown", Label: o.theme.Glyphs.ReviewStateUnknown, SortKey: "unknown", Width: 4},
		{Key: "under_review", Label: o.theme.Glyphs.ReviewStateUnderReview, SortKey: "under_review", Width: 4},
		{Key: "active", Label: o.theme.Glyphs.ReviewStateActive, SortKey: "active", Width: 4},
		{Key: "ignored", Label: o.theme.Glyphs.ReviewStateIgnored, SortKey: "ignored", Width: 4},
		{Key: "tags", Label: "Tags", SortKey: "tags", Width: tagsColWidth},
	}
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
		FocusActive:   o.focus == focusStats,
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
			lineNum := startStats + rowIdx + 1
			gutter := ""
			if o.focus == focusStats {
				gutter = o.jump.GutterPrefix(lineNum)
			} else {
				gutter = fmt.Sprintf(" %d ", lineNum)
			}
			return gutter + classificationStatsLabel(s.Classification)
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
		patCursor := o.patentsPage.Cursor()
		focusPatents := o.focus == focusPatents
		params := render.TableParams{Theme: o.theme, Columns: cols, RowCount: endPat - startPat, FocusedColIdx: o.focusedColIdx, ActiveSort: string(o.activeSort), SortAscending: o.sortAscending, FocusActive: focusPatents, VisualMode: o.patentsPage.VisualMode(), IsRowCursor: func(rowIdx int) bool { return focusPatents && startPat+rowIdx == patCursor }, IsRowSelected: func(rowIdx int) bool { return o.patentsPage.IsRowSelected(startPat + rowIdx) }, IsRowMarked: func(rowIdx int) bool {
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
				lineNum := startPat + rowIdx + 1
				gutter := ""
				if focusPatents {
					gutter = o.jump.GutterPrefix(lineNum)
				} else {
					gutter = fmt.Sprintf(" %d ", lineNum)
				}
				numStr := p.Number.String()
				if !p.DisplayNumber.IsZero() {
					numStr = p.DisplayNumber.String()
				}
				return gutter + numStr
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
		searchLine := "/ " + o.searchQuery + "▋  [Scope: " + classificationSearchScopes[o.searchScope].Label + " (Tab to cycle)]"
		b.WriteString(o.theme.Selected.Render(render.Pad(searchLine, targetW)))
	} else if o.patentsSearchActive {
		searchLine := "/ " + o.patentsSearchQuery + "▋ (patents)"
		b.WriteString(o.theme.Selected.Render(render.Pad(searchLine, targetW)))
	} else if o.jump.Active {
		curr := o.selected
		if o.focus == focusPatents {
			curr = o.patentsPage.Cursor()
		}
		b.WriteString(o.theme.Dim.Render(render.Truncate(o.jump.HintSuffix(curr, -1, false), targetW)))
	} else {
		var footnote string
		if o.focus == focusStats {
			status := fmt.Sprintf("[%d/%d]", o.selected+1, len(o.stats))
			footnote = fmt.Sprintf("%s  [Tab/l/Enter] Focus Patents  [/] Search  [j/k/↑/↓] Scroll  [←/→] Focus Col  [.] Sort  [q/Q/Esc] Close · [;] jump mode", status)
		} else {
			status := "[0/0]"
			if o.patentsPage.Total() > 0 {
				status = fmt.Sprintf("[%d/%d]", o.patentsPage.Cursor()+1, o.patentsPage.Total())
			}
			footnote = fmt.Sprintf("%s  [Tab/h/←] Focus Classifications  [j/k/↑/↓] Scroll  [l/Enter] View  [v] Visual  [←/→] Focus Col  [.] Sort  [/] Search  [ctrl+u/d] Page  [s/r/i/x] Review  [t] Tag  [I] IDS  [q/Q/Esc] Close · [;] jump mode", status)
			if o.patentsPage.VisualMode() {
				footnote = fmt.Sprintf("%s VISUAL MODE  [j/k/↑/↓] Select  [s/r/i/x] Review  [t] Tag  [I] IDS  [v/q/Q/Esc] Clear", status)
			}
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

func (o *ClassificationStatsOverlay) reloadPatentsCmd() tea.Cmd {
	o.loadSeq++
	o.loadID = o.loadSeq
	o.patentsLoading = true
	o.patentsErr = nil

	code := ""
	if len(o.stats) > 0 && o.selected >= 0 && o.selected < len(o.stats) {
		code = o.stats[o.selected].Classification.Code
	}
	return o.loadPatentsCmd(code, o.loadID)
}

func (o *ClassificationStatsOverlay) loadPatentsCmd(code string, requestID uint64) tea.Cmd {
	offset := o.patentsPage.Offset()
	limit := o.patentsPage.PageSize()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var res proto.PatentListResult
		err := o.client.Call(ctx, proto.MethodPatentList, proto.PatentListParams{Project: o.project, ClassificationCode: code, Limit: limit, Offset: offset, SortColumn: o.activeSort, SortAscending: o.sortAscending, Search: o.patentsSearchQuery}, &res)
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

func (o *ClassificationStatsOverlay) sortStats() {
	if len(o.stats) == 0 {
		return
	}
	var selectedCode string
	if o.selected >= 0 && o.selected < len(o.stats) {
		selectedCode = o.stats[o.selected].Classification.Code
	}

	sort.SliceStable(o.stats, func(i, j int) bool {
		var cmp bool
		var equal bool

		switch o.statsSortCol {
		case "name":
			labelI := classificationStatsLabel(o.stats[i].Classification)
			labelJ := classificationStatsLabel(o.stats[j].Classification)
			cmp = labelI < labelJ
			equal = labelI == labelJ
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
			return o.stats[i].Classification.Code < o.stats[j].Classification.Code
		}

		if o.statsSortAsc {
			return cmp
		}
		return !cmp
	})

	if selectedCode != "" {
		for idx, stat := range o.stats {
			if stat.Classification.Code == selectedCode {
				o.selected = idx
				break
			}
		}
	}
}

var classificationSearchScopes = []struct {
	Key   string
	Label string
}{
	{"all", "All Columns"},
	{"code", "Code"},
	{"desc", "Description"},
	{"tags", "Tags"},
	{"states", "States"},
}

func (o *ClassificationStatsOverlay) applyFilter() {
	if o.searchQuery == "" {
		o.stats = o.allStats
	} else {
		o.stats = nil
		q := strings.ToLower(o.searchQuery)
		scope := classificationSearchScopes[o.searchScope].Key
		for _, s := range o.allStats {
			match := false
			switch scope {
			case "all":
				match = strings.Contains(strings.ToLower(s.Classification.Code), q) ||
					strings.Contains(strings.ToLower(s.Classification.Description), q) ||
					strings.Contains(strings.ToLower(render.FormatTagsForSort(s.Tags)), q) ||
					strings.Contains(strings.ToLower(fmt.Sprintf("%d %d %d %d", s.States["unknown"], s.States["under_review"], s.States["active"], s.States["ignored"])), q)
			case "code":
				match = strings.Contains(strings.ToLower(s.Classification.Code), q)
			case "desc":
				match = strings.Contains(strings.ToLower(s.Classification.Description), q)
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
