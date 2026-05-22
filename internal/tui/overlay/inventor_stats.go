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

// loadedInventorStatsMsg carries the asynchronously fetched stats.
type loadedInventorStatsMsg struct {
	stats []domain.InventorStats
	err   error
}

// loadedPatentListMsg carries the asynchronously fetched patents for the selected inventor.
type loadedPatentListMsg struct {
	requestID uint64
	patents   []domain.PatentRow
	total     int
	err       error
}

type statsFocus int

const (
	focusInventors statsFocus = iota
	focusPatents
)

// InventorStatsOverlay presents interactive statistics for the patent's inventors.
type InventorStatsOverlay struct {
	client         *rpc.Client
	theme          render.Theme
	catalog        *text.Catalog
	patent         domain.Patent
	project        domain.ProjectID
	stats          []domain.InventorStats
	selected       int
	loading        bool
	err            error

	// Dual-pane focus, pagination, and state
	focus          statsFocus
	patents        []domain.PatentRow
	patentsPage    render.Paginator
	patentsLoading bool
	patentsErr     error
	loadSeq        uint64
	loadID         uint64

	// Sorting and column navigation
	activeSort     domain.SortColumn
	sortAscending  bool
	focusedColIdx  int
	lastWidth      int
}

// NewInventorStatsOverlay initializes and triggers an async query for inventor stats.
func NewInventorStatsOverlay(client *rpc.Client, theme render.Theme, catalog *text.Catalog, patent domain.Patent, project domain.ProjectID) (*InventorStatsOverlay, tea.Cmd) {
	o := &InventorStatsOverlay{
		client:         client,
		theme:          theme,
		catalog:        catalog,
		patent:         patent,
		project:        project,
		loading:        true,
		patentsPage:    render.NewPaginator(5), // Initial visible size, dynamically updated on View / resize
		activeSort:     domain.SortByNumber,
		sortAscending:  true,
		focusedColIdx:  -1,
		lastWidth:      90,
	}
	return o, o.loadStatsCmd()
}

// Title implements Overlay.
func (o *InventorStatsOverlay) Title() string {
	return "Inventor Analytics"
}

// Handles implements Overlay.
func (o *InventorStatsOverlay) Handles() []command.ID {
	return []command.ID{
		command.CloseOverlay,
	}
}

// Command implements Overlay.
func (o *InventorStatsOverlay) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if id == command.CloseOverlay {
		return o, func() tea.Msg { return CloseOverlayMsg{} }
	}
	return o, nil
}

// Update processes background load responses.
func (o *InventorStatsOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case loadedInventorStatsMsg:
		o.loading = false
		if m.err != nil {
			o.err = m.err
		} else {
			o.stats = m.stats
			if o.selected >= len(o.stats) {
				o.selected = max(0, len(o.stats)-1)
			}
			if len(o.stats) > 0 {
				o.loadSeq++
				o.loadID = o.loadSeq
				o.patentsLoading = true
				o.patentsErr = nil
				return o, o.loadPatentsCmd(o.stats[o.selected].Inventor, o.loadID)
			}
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
				cmds = append(cmds, o.loadPatentsCmd(o.stats[o.selected].Inventor, o.loadID))
			}
			return o, tea.Batch(cmds...)
		}
		return o, nil

	case tea.WindowSizeMsg:
		w, h := o.OverlaySize(m.Width, m.Height)
		o.lastWidth = w - 4 // innerWidth
		innerHeight := h - 4
		availH := innerHeight - 6
		if availH < 2 {
			availH = 2
		}
		statsH := max(1, availH/3)
		patentsH := max(1, availH-statsH)

		o.focusedColIdx = clampFocusedStatsColumn(o.currentCols(), o.focusedColIdx)

		if patentsH != o.patentsPage.PageSize() {
			o.patentsPage.SetPageSize(patentsH)
			if len(o.stats) > 0 && o.selected < len(o.stats) {
				o.loadSeq++
				o.loadID = o.loadSeq
				o.patentsLoading = true
				o.patentsErr = nil
				return o, o.loadPatentsCmd(o.stats[o.selected].Inventor, o.loadID)
			}
		}
		return o, nil
	}
	return o, nil
}

// HandleKey implements KeyHandler for scroll and dismiss.
func (o *InventorStatsOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	o.err = nil
	o.patentsErr = nil

	switch msg.String() {
	case "q", "esc":
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case "tab":
		if o.focus == focusInventors {
			o.focus = focusPatents
			o.focusedColIdx = 0 // Initial column focus on Number
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
				return o, o.loadPatentsCmd(o.stats[o.selected].Inventor, o.loadID), true
			}
		case "k", "up":
			if len(o.stats) > 0 {
				o.selected = (o.selected - 1 + len(o.stats)) % len(o.stats)
				o.patentsPage.Top()
				o.loadSeq++
				o.loadID = o.loadSeq
				o.patentsLoading = true
				return o, o.loadPatentsCmd(o.stats[o.selected].Inventor, o.loadID), true
			}
		}
	} else { // focus == focusPatents
		// 1. Keys that DO NOT require patents to be populated
		switch msg.String() {
		case "left", "h":
			o.focusedColIdx = moveStatsColumn(o.currentCols(), o.focusedColIdx, -1)
			return o, nil, true
		case "right", "l":
			o.focusedColIdx = moveStatsColumn(o.currentCols(), o.focusedColIdx, 1)
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
					var inventor string
					if len(o.stats) > 0 && o.selected >= 0 && o.selected < len(o.stats) {
						inventor = o.stats[o.selected].Inventor
					}
					return o, o.loadPatentsCmd(inventor, o.loadID), true
				}
			}
			return o, nil, true
		}

		// 2. Keys that DO require patents to be populated
		if len(o.patents) == 0 {
			return o, nil, true
		}

		cursor := o.patentsPage.Cursor()
		if cursor < 0 || cursor >= len(o.patents) {
			return o, nil, true
		}
		selectedPatent := o.patents[cursor]

		switch msg.String() {
		case "j", "down":
			before := o.patentsPage.Offset()
			o.patentsPage.MoveDown(1)
			if o.patentsPage.Offset() != before {
				o.loadSeq++
				o.loadID = o.loadSeq
				o.patentsLoading = true
				var inventor string
				if len(o.stats) > 0 && o.selected >= 0 && o.selected < len(o.stats) {
					inventor = o.stats[o.selected].Inventor
				}
				return o, o.loadPatentsCmd(inventor, o.loadID), true
			}
			return o, nil, true
		case "k", "up":
			before := o.patentsPage.Offset()
			o.patentsPage.MoveUp(1)
			if o.patentsPage.Offset() != before {
				o.loadSeq++
				o.loadID = o.loadSeq
				o.patentsLoading = true
				var inventor string
				if len(o.stats) > 0 && o.selected >= 0 && o.selected < len(o.stats) {
					inventor = o.stats[o.selected].Inventor
				}
				return o, o.loadPatentsCmd(inventor, o.loadID), true
			}
			return o, nil, true
		case "ctrl+d":
			before := o.patentsPage.Offset()
			o.patentsPage.PageDown()
			if o.patentsPage.Offset() != before {
				o.loadSeq++
				o.loadID = o.loadSeq
				o.patentsLoading = true
				var inventor string
				if len(o.stats) > 0 && o.selected >= 0 && o.selected < len(o.stats) {
					inventor = o.stats[o.selected].Inventor
				}
				return o, o.loadPatentsCmd(inventor, o.loadID), true
			}
			return o, nil, true
		case "ctrl+u":
			before := o.patentsPage.Offset()
			o.patentsPage.PageUp()
			if o.patentsPage.Offset() != before {
				o.loadSeq++
				o.loadID = o.loadSeq
				o.patentsLoading = true
				var inventor string
				if len(o.stats) > 0 && o.selected >= 0 && o.selected < len(o.stats) {
					inventor = o.stats[o.selected].Inventor
				}
				return o, o.loadPatentsCmd(inventor, o.loadID), true
			}
			return o, nil, true
		case "s":
			cmd := pane.SetReviewStateCmd(o.client, o.project, []domain.PatentNumber{selectedPatent.Number}, domain.ReviewStateStored)
			return o, cmd, true
		case "r":
			cmd := pane.SetReviewStateCmd(o.client, o.project, []domain.PatentNumber{selectedPatent.Number}, domain.ReviewStateUnderReview)
			return o, cmd, true
		case "i":
			cmd := pane.SetReviewStateCmd(o.client, o.project, []domain.PatentNumber{selectedPatent.Number}, domain.ReviewStateIgnored)
			return o, cmd, true
		case "x":
			cmd := pane.SetReviewStateCmd(o.client, o.project, []domain.PatentNumber{selectedPatent.Number}, domain.ReviewStateDeleted)
			return o, cmd, true
		case "t":
			return o, func() tea.Msg {
				return OpenTagPatentOverlayMsg{Patents: []domain.PatentNumber{selectedPatent.Number}}
			}, true
		}
	}

	return o, nil, true
}

// OverlaySize implements DynamicSize, giving us extra width for stats formatting.
func (o *InventorStatsOverlay) OverlaySize(termW, termH int) (w, h int) {
	return PctSize(termW, termH, 90, 90, 90, 22)
}

func (o *InventorStatsOverlay) currentCols() []render.TableColumn {
	w := o.lastWidth
	switch {
	case w >= 80:
		fixedWidth := 70
		titleWidth := max(10, w-fixedWidth)
		return []render.TableColumn{
			{Key: "number", Label: "Number", SortKey: string(domain.SortByNumber), Width: 16},
			{Key: "title", Label: "Title", SortKey: string(domain.SortByTitle), Width: titleWidth},
			{Key: "inventor", Label: "Inventor", SortKey: string(domain.SortByInventor), Width: 16},
			{Key: "year", Label: "Year", SortKey: string(domain.SortByExpires), Width: 4},
			{Key: "tags", Label: "Tags", SortKey: string(domain.SortByTags), Width: 15},
			{Key: "state", Label: "State", SortKey: string(domain.SortByReviewState), Width: 12},
		}
	case w >= 65:
		fixedWidth := 54
		titleWidth := max(10, w-fixedWidth)
		return []render.TableColumn{
			{Key: "number", Label: "Number", SortKey: string(domain.SortByNumber), Width: 16},
			{Key: "title", Label: "Title", SortKey: string(domain.SortByTitle), Width: titleWidth},
			{Key: "inventor", Label: "Inventor", SortKey: string(domain.SortByInventor), Width: 16},
			{Key: "year", Label: "Year", SortKey: string(domain.SortByExpires), Width: 4},
			{Key: "state", Label: "State", SortKey: string(domain.SortByReviewState), Width: 12},
		}
	case w >= 55:
		fixedWidth := 37
		titleWidth := max(10, w-fixedWidth)
		return []render.TableColumn{
			{Key: "number", Label: "Number", SortKey: string(domain.SortByNumber), Width: 16},
			{Key: "title", Label: "Title", SortKey: string(domain.SortByTitle), Width: titleWidth},
			{Key: "year", Label: "Year", SortKey: string(domain.SortByExpires), Width: 4},
			{Key: "state", Label: "State", SortKey: string(domain.SortByReviewState), Width: 12},
		}
	default:
		fixedWidth := 32
		titleWidth := max(10, w-fixedWidth)
		return []render.TableColumn{
			{Key: "number", Label: "Number", SortKey: string(domain.SortByNumber), Width: 16},
			{Key: "title", Label: "Title", SortKey: string(domain.SortByTitle), Width: titleWidth},
			{Key: "state", Label: "State", SortKey: string(domain.SortByReviewState), Width: 12},
		}
	}
}

// View renders the list of inventors and their respective metrics.
func (o *InventorStatsOverlay) View(maxW, maxH int) string {
	o.lastWidth = maxW
	var b strings.Builder

	if o.err != nil {
		b.WriteString(o.theme.Error.Render(render.Truncate("Error: "+o.err.Error(), maxW)))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("[q/Esc] Close"))
		return b.String()
	}

	if o.loading {
		b.WriteString(o.theme.MutedItalic.Render("Loading inventor statistics..."))
		b.WriteString("\n\n")
		b.WriteString(o.theme.Dim.Render("[q/Esc] Close"))
		return b.String()
	}

	titleText := fmt.Sprintf("  Inventor Stats for Patent %s (%d inventors)", o.patent.DisplayNumber.String(), len(o.stats))
	b.WriteString(o.theme.Dim.Render(titleText))
	b.WriteString("\n\n")

	// 2. Calculate heights and update Paginator dynamically
	availH := maxH - 10
	if availH < 2 {
		availH = 2
	}
	statsH := max(1, availH/3)
	patentsH := max(1, availH-statsH)

	o.patentsPage.SetPageSize(patentsH)

	// 3. Render Inventor Stats List
	maxNameLen := 0
	for _, s := range o.stats {
		if len(s.Inventor) > maxNameLen {
			maxNameLen = len(s.Inventor)
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
		isSelectedRow := i == o.selected
		if isSelectedRow && o.focus == focusInventors {
			prefix = "→ "
		}

		paddedName := render.Pad(s.Inventor, maxNameLen+2)
		statsStr := render.FormatEntityStats(s.Total, s.States, s.Tags)
		line := fmt.Sprintf("%s%s%s", prefix, paddedName, statsStr)

		if isSelectedRow && o.focus == focusInventors {
			b.WriteString(o.theme.Selected.Render(render.Truncate(line, maxW)))
		} else {
			if i%2 == 1 {
				b.WriteString(o.theme.RowAlt.Render(render.Truncate(line, maxW)))
			} else {
				b.WriteString(o.theme.Row.Render(render.Truncate(line, maxW)))
			}
		}
		b.WriteString("\n")
	}

	// 4. Divider / Patents Header
	b.WriteString("\n")
	dividerText := fmt.Sprintf("─── Patents by Selected Inventor (%s) ───", o.stats[o.selected].Inventor)
	dashCount := maxW - len(dividerText)
	if dashCount > 0 {
		dividerText += strings.Repeat("─", dashCount)
	}
	b.WriteString(o.theme.Header.Render(render.Truncate(dividerText, maxW)))
	b.WriteString("\n")

	// 5. Render Patents Table
	if o.patentsErr != nil {
		b.WriteString(o.theme.Error.Render(render.Truncate("Error loading patents: "+o.patentsErr.Error(), maxW)))
		b.WriteString("\n")
	} else if o.patentsLoading && len(o.patents) == 0 {
		b.WriteString(o.theme.MutedItalic.Render("Loading patents..."))
		b.WriteString("\n")
	} else if len(o.patents) == 0 {
		b.WriteString(o.theme.MutedItalic.Render("No patents found for this inventor."))
		b.WriteString("\n")
	} else {
		cols := o.currentCols()
		startPat, endPat := o.patentsPage.Window()

		params := render.TableParams{
			Theme:         o.theme,
			Columns:       cols,
			RowCount:      endPat - startPat,
			Cursor:        o.patentsPage.Cursor() - startPat,
			FocusedColIdx: o.focusedColIdx,
			ActiveSort:    string(o.activeSort),
			SortAscending: o.sortAscending,
			FocusActive:   o.focus == focusPatents,
			PrefixCursor:  "→ ",
			PrefixNormal:  "  ",
		}

		tableStr := render.RenderTable(params, maxW, func(rowIdx, colIdx int) string {
			pIdx := startPat + rowIdx
			if pIdx >= len(o.patents) {
				return ""
			}
			p := o.patents[pIdx]
			col := cols[colIdx]

			switch col.Key {
			case "number":
				valNum := p.Number.String()
				if !p.DisplayNumber.IsZero() {
					valNum = p.DisplayNumber.String()
				}
				return valNum
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
				valState := string(p.FetchState)
				if o.project != "" && p.ReviewState.Valid() {
					valState = string(p.ReviewState)
				}
				return valState
			default:
				return ""
			}
		})
		b.WriteString(tableStr)
	}

	// 6. Help Footnote / Instructions
	b.WriteString("\n")
	var footnote string
	if o.focus == focusInventors {
		footnote = "[Tab] Switch Focus  [j/k/↑/↓] Select Inventor  [q/Esc] Close"
	} else {
		footnote = "[Tab] Switch Focus  [j/k/↑/↓] Scroll Patents  [h/l/←/→] Focus Col  [.] Sort  [ctrl+u/d] Page  [s/r/i/x] Review  [t] Tag  [q/Esc] Close"
	}
	b.WriteString(o.theme.Dim.Render(footnote))

	return b.String()
}

func (o *InventorStatsOverlay) loadStatsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var res proto.PatentInventorStatsResult
		params := proto.PatentGetParams{
			Number:  o.patent.Number,
			Project: o.project,
		}
		if err := o.client.Call(ctx, proto.MethodPatentInventorStats, params, &res); err != nil {
			return loadedInventorStatsMsg{err: err}
		}
		return loadedInventorStatsMsg{stats: res.Stats}
	}
}

func (o *InventorStatsOverlay) loadPatentsCmd(inventor string, requestID uint64) tea.Cmd {
	offset := o.patentsPage.Offset()
	limit := o.patentsPage.PageSize()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var res proto.PatentListResult
		err := o.client.Call(ctx, proto.MethodPatentList,
			proto.PatentListParams{
				Project:       o.project,
				Inventor:      inventor,
				Limit:         limit,
				Offset:        offset,
				SortColumn:    o.activeSort,
				SortAscending: o.sortAscending,
			}, &res)

		return loadedPatentListMsg{
			requestID: requestID,
			patents:   res.Patents,
			total:     res.Total,
			err:       err,
		}
	}
}

// formatInventorsShort returns the first inventor's name, appending "et al." when there are multiple.
func formatInventorsShort(inventors []domain.Inventor) string {
	if len(inventors) == 0 {
		return "-"
	}
	if len(inventors) == 1 {
		return string(inventors[0])
	}
	return string(inventors[0]) + " et al."
}

// moveStatsColumn navigates left/right among columns with a sortKey.
func moveStatsColumn(cols []render.TableColumn, current, step int) int {
	if len(cols) == 0 {
		return -1
	}
	if current < 0 {
		for i, c := range cols {
			if c.SortKey != "" {
				return i
			}
		}
		return -1
	}
	idx := current
	for range len(cols) {
		idx = (idx + step + len(cols)) % len(cols)
		if cols[idx].SortKey != "" {
			return idx
		}
	}
	return -1
}

// clampFocusedStatsColumn ensures the focused column index remains in a valid sortable range.
func clampFocusedStatsColumn(cols []render.TableColumn, current int) int {
	if len(cols) == 0 {
		return -1
	}
	if current < 0 {
		return -1
	}
	if current >= len(cols) {
		current = len(cols) - 1
	}
	if cols[current].SortKey != "" {
		return current
	}
	for i := current; i >= 0; i-- {
		if cols[i].SortKey != "" {
			return i
		}
	}
	for i := current; i < len(cols); i++ {
		if cols[i].SortKey != "" {
			return i
		}
	}
	return -1
}
