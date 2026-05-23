package overlay

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/engine"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/pane"
	"patentmine/internal/tui/render"
)

// loadedPatentClassificationsMsg carries the result of MethodPatentClassificationList.
type loadedPatentClassificationsMsg struct {
	classifications []domain.Classification
	err             error
}

// patentClassificationLookupOneMsg signals one bulk-lookup step finished.
type patentClassificationLookupOneMsg struct {
	code           string
	classification domain.Classification
	err            error
}

// Overlay column keys. Stable strings so the cellValue callback can dispatch
// without tying to a numeric enum. The status of each row is conveyed by the
// description column itself (e.g. "(unsupported system)" inside parentheses)
// so a dedicated Status column would duplicate information and waste width.
const (
	classificationColumnSystem      = "classification.system"
	classificationColumnCode        = "classification.code"
	classificationColumnGroup       = "classification.group"
	classificationColumnDescription = "classification.description"
)

// classificationRowStatus categorises one row's lookup state. The Status
// column shows the textual label; sorting by this key collates rows by
// usefulness.
type classificationRowStatus int

const (
	classificationRowStatusCached classificationRowStatus = iota
	classificationRowStatusEPOMiss
	classificationRowStatusUnsupported
	classificationRowStatusUncached
)

func (s classificationRowStatus) String() string {
	switch s {
	case classificationRowStatusCached:
		return "cached"
	case classificationRowStatusEPOMiss:
		return "EPO miss"
	case classificationRowStatusUnsupported:
		return "unsupported"
	default:
		return "uncached"
	}
}

func classificationStatusOf(c domain.Classification) classificationRowStatus {
	if c.System != "CPC" {
		return classificationRowStatusUnsupported
	}
	if c.Description == engine.ClassificationDescriptionUnavailable {
		return classificationRowStatusEPOMiss
	}
	if c.Description == "" {
		return classificationRowStatusUncached
	}
	return classificationRowStatusCached
}

// PatentClassificationOverlay shows the classifications attached to one patent
// in a sortable, visual-selectable table. Marked codes are applied as a
// classification filter to the parent pane on close.
type PatentClassificationOverlay struct {
	client          *rpc.Client
	theme           render.Theme
	catalog         *text.Catalog
	metrics         *observability.Metrics
	project         domain.ProjectID
	patent          domain.PatentNumber
	classifications []domain.Classification
	cursor          int
	loading         bool
	fetching        bool
	queue           []string
	progressDone    int
	progressTotal   int
	progressCurrent string
	err             error
	msg             string

	activeSort    domain.SortColumn
	sortAscending bool
	focusedColIdx int

	visual VisualState
	marked map[string]bool

	pendingG bool
}

// VisualState aliases pane.VisualState so callers don't need to import pane.
type VisualState = pane.VisualState

// NewPatentClassificationOverlay returns the overlay and the initial load Cmd.
func NewPatentClassificationOverlay(client *rpc.Client, theme render.Theme, catalog *text.Catalog, metrics *observability.Metrics, project domain.ProjectID, patent domain.PatentNumber) (*PatentClassificationOverlay, tea.Cmd) {
	o := &PatentClassificationOverlay{
		client:        client,
		theme:         theme,
		catalog:       catalog,
		metrics:       metrics,
		project:       project,
		patent:        patent,
		loading:       true,
		activeSort:    domain.SortByClassificationSystem,
		sortAscending: true,
		focusedColIdx: 0,
		marked:        make(map[string]bool),
	}
	if metrics != nil {
		metrics.IncCounter("patent_classifications_overlay_open_total", 1)
	}
	return o, o.loadCmd()
}

func (o *PatentClassificationOverlay) Title() string {
	return fmt.Sprintf("Classifications — %s", o.patent.String())
}

// OverlaySize implements DynamicSize: take 90% of the terminal.
func (o *PatentClassificationOverlay) OverlaySize(termW, termH int) (w, h int) {
	return PctSize(termW, termH, 90, 90, 80, 20)
}

func (o *PatentClassificationOverlay) Handles() []command.ID {
	return []command.ID{command.CloseOverlay}
}

func (o *PatentClassificationOverlay) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if id == command.CloseOverlay {
		return o, o.closeCmd()
	}
	return o, nil
}

// columns builds the table column descriptors at a given body width. The
// description column flexes to consume whatever width is left after the
// fixed-width columns. Status is implied by the description text itself
// (parenthesised hint for non-cached rows) so no separate Status column.
func (o *PatentClassificationOverlay) columns(maxW int) []pane.OverlayColumn {
	cols := []pane.OverlayColumn{
		{Key: classificationColumnSystem, Label: "Sys", SortKey: domain.SortByClassificationSystem, Width: 5},
		{Key: classificationColumnCode, Label: "Code", SortKey: domain.SortByClassificationCode, Width: 16},
		{Key: classificationColumnGroup, Label: "Group", Width: 10},
		{Key: classificationColumnDescription, Label: "Description", SortKey: domain.SortByClassificationDescription, Width: 30},
	}
	fixed := 0
	for _, c := range cols {
		if c.Key != classificationColumnDescription {
			fixed += c.Width
		}
	}
	fixed += len(cols) - 1 // one-space gap between columns
	flexW := max(maxW-fixed, 12)
	for i := range cols {
		if cols[i].Key == classificationColumnDescription {
			cols[i].Width = flexW
		}
	}
	return cols
}

func (o *PatentClassificationOverlay) loadCmd() tea.Cmd {
	project := o.project
	patent := o.patent
	client := o.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var res proto.ClassificationListResult
		if err := client.Call(ctx, proto.MethodPatentClassificationList, proto.PatentClassificationListParams{Project: project, Patent: patent}, &res); err != nil {
			return loadedPatentClassificationsMsg{err: err}
		}
		return loadedPatentClassificationsMsg{classifications: res.Classifications}
	}
}

// lookupOneCmd queries the engine for a single code with its own timeout.
func (o *PatentClassificationOverlay) lookupOneCmd(code string) tea.Cmd {
	client := o.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var res domain.Classification
		err := client.Call(ctx, proto.MethodClassificationLookup, proto.ClassificationLookupParams{Code: code}, &res)
		return patentClassificationLookupOneMsg{code: code, classification: res, err: err}
	}
}

// closeCmd returns the close message, possibly preceded by the apply-filter
// message when the user marked one or more codes.
func (o *PatentClassificationOverlay) closeCmd() tea.Cmd {
	if len(o.marked) == 0 {
		return func() tea.Msg { return CloseOverlayMsg{} }
	}
	codes := o.markedCodesInSortOrder()
	return tea.Batch(
		func() tea.Msg { return ApplyClassificationFilterMsg{Codes: codes} },
		func() tea.Msg { return CloseOverlayMsg{} },
	)
}

func (o *PatentClassificationOverlay) markedCodesInSortOrder() []string {
	out := make([]string, 0, len(o.marked))
	for _, c := range o.classifications {
		if o.marked[c.Code] {
			out = append(out, c.Code)
		}
	}
	return out
}

// missingDescriptionLabel returns a system-aware hint for empty descriptions.
// The hint is plain text (no ANSI styling) because cellValue results travel
// through pad/truncate/box rendering — embedded escape codes confuse the box
// wrap and cause a single row to fold onto a second physical line.
func (o *PatentClassificationOverlay) missingDescriptionLabel(c domain.Classification) string {
	switch classificationStatusOf(c) {
	case classificationRowStatusUnsupported:
		return "(unsupported system)"
	case classificationRowStatusEPOMiss:
		return "(EPO has no description)"
	default:
		return "(not cached — press L)"
	}
}

func hasUsefulDescription(c domain.Classification) bool {
	return classificationStatusOf(c) == classificationRowStatusCached
}

func (o *PatentClassificationOverlay) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case loadedPatentClassificationsMsg:
		o.loading = false
		o.progressDone = 0
		o.progressTotal = 0
		o.progressCurrent = ""
		if m.err != nil {
			o.err = m.err
		} else {
			o.classifications = m.classifications
			o.applySort()
			if o.cursor >= len(o.classifications) {
				o.cursor = max(0, len(o.classifications)-1)
			}
		}
		return o, nil
	case patentClassificationLookupOneMsg:
		o.progressDone++
		if m.err != nil && o.err == nil {
			o.err = m.err
		}
		if len(o.queue) == 0 {
			o.fetching = false
			o.progressCurrent = ""
			// Clear so View shows the loading branch while the reload is in
			// flight — otherwise stale rows look like nothing changed.
			o.classifications = nil
			o.loading = true
			return o, o.loadCmd()
		}
		next := o.queue[0]
		o.queue = o.queue[1:]
		o.progressCurrent = next
		return o, o.lookupOneCmd(next)
	}
	return o, nil
}

func (o *PatentClassificationOverlay) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	o.err = nil
	o.msg = ""

	key := msg.String()

	// Handle the 'g a' chord for select-all.
	if o.pendingG {
		o.pendingG = false
		if key == "a" {
			if len(o.classifications) > 0 {
				o.visual.SelectAll(len(o.classifications))
				o.cursor = len(o.classifications) - 1
			}
			return o, nil, true
		}
		// Any other key cancels the chord; fall through.
	}

	switch key {
	case "esc":
		if o.visual.Mode {
			o.visual.Clear()
			return o, nil, true
		}
		return o, o.closeCmd(), true
	case "q":
		return o, o.closeCmd(), true
	case "j", "down":
		o.moveCursor(1)
		return o, nil, true
	case "k", "up":
		o.moveCursor(-1)
		return o, nil, true
	case "g":
		o.pendingG = true
		return o, nil, true
	case "v":
		o.visual.Toggle(o.cursor)
		return o, nil, true
	case "h", "left":
		o.focusedColIdx = pane.MoveSortableOverlayColumn(o.columns(80), o.focusedColIdx, -1)
		return o, nil, true
	case "l", "right":
		o.focusedColIdx = pane.MoveSortableOverlayColumn(o.columns(80), o.focusedColIdx, 1)
		return o, nil, true
	case ".":
		o.applySortFromFocus()
		return o, nil, true
	case "f":
		o.toggleMark()
		return o, nil, true
	case "F":
		o.marked = make(map[string]bool)
		o.msg = "marks cleared"
		return o, nil, true
	case "L":
		return o, o.startBulkFetch(), true
	}
	return o, nil, true
}

func (o *PatentClassificationOverlay) moveCursor(delta int) {
	if len(o.classifications) == 0 {
		return
	}
	o.cursor = (o.cursor + delta + len(o.classifications)) % len(o.classifications)
}

func (o *PatentClassificationOverlay) applySortFromFocus() {
	cols := o.columns(80)
	if o.focusedColIdx < 0 || o.focusedColIdx >= len(cols) {
		return
	}
	col := cols[o.focusedColIdx]
	if col.SortKey == "" {
		return
	}
	if o.activeSort == col.SortKey {
		o.sortAscending = !o.sortAscending
	} else {
		o.activeSort = col.SortKey
		o.sortAscending = true
	}
	o.applySort()
}

// applySort sorts o.classifications in place. The primary key is the
// configured SortColumn; ties break by Code ascending so the order is
// deterministic.
func (o *PatentClassificationOverlay) applySort() {
	if len(o.classifications) == 0 {
		return
	}
	cs := o.classifications
	asc := o.sortAscending
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		var less bool
		switch o.activeSort {
		case domain.SortByClassificationCode:
			less = a.Code < b.Code
		case domain.SortByClassificationDescription:
			less = strings.ToLower(a.Description) < strings.ToLower(b.Description)
		default: // SortByClassificationSystem and fallback
			less = a.System < b.System
		}
		if !less && cs[i] != cs[j] {
			// equal on primary key — break ties by Code asc.
			if a.System == b.System && a.Code == b.Code {
				// fall through to stable order
				return false
			}
			if cs[i] == cs[j] {
				return false
			}
		}
		if a.System == b.System && o.activeSort != domain.SortByClassificationCode {
			if a.Code != b.Code {
				less = a.Code < b.Code
				if !asc {
					return !less
				}
				return less
			}
		}
		if !asc {
			return !less
		}
		return less
	})
}

func (o *PatentClassificationOverlay) toggleMark() {
	if len(o.classifications) == 0 {
		return
	}
	if o.visual.Mode {
		for i := range o.classifications {
			if o.visual.InRange(o.cursor, i) {
				code := o.classifications[i].Code
				if o.marked[code] {
					delete(o.marked, code)
				} else {
					o.marked[code] = true
				}
			}
		}
		o.visual.Clear()
		return
	}
	code := o.classifications[o.cursor].Code
	if o.marked[code] {
		delete(o.marked, code)
	} else {
		o.marked[code] = true
	}
}

func (o *PatentClassificationOverlay) startBulkFetch() tea.Cmd {
	if o.fetching {
		return nil
	}
	var candidates []string
	for _, c := range o.classifications {
		if classificationStatusOf(c) == classificationRowStatusUncached {
			candidates = append(candidates, c.Code)
		}
	}
	if len(candidates) == 0 {
		o.msg = "no missing CPC descriptions to fetch"
		return nil
	}
	o.fetching = true
	o.progressTotal = len(candidates)
	o.progressDone = 0
	o.queue = candidates[1:]
	o.progressCurrent = candidates[0]
	return o.lookupOneCmd(candidates[0])
}

func (o *PatentClassificationOverlay) cellValue(col pane.OverlayColumn, c domain.Classification) string {
	switch col.Key {
	case classificationColumnSystem:
		if o.marked[c.Code] {
			return "★" + c.System
		}
		return " " + c.System
	case classificationColumnCode:
		return c.Code
	case classificationColumnGroup:
		if c.MainGroup == "" {
			return ""
		}
		if c.Subgroup == "" {
			return c.MainGroup
		}
		return c.MainGroup + "/" + c.Subgroup
	case classificationColumnDescription:
		if hasUsefulDescription(c) {
			return c.Description
		}
		return o.missingDescriptionLabel(c)
	}
	return ""
}

func (o *PatentClassificationOverlay) View(maxW, maxH int) string {
	var b strings.Builder

	if o.err != nil {
		b.WriteString(o.theme.Error.Render(render.Truncate("Error: "+o.err.Error(), maxW)))
		b.WriteString("\n\n")
	} else if o.msg != "" {
		b.WriteString(o.theme.OK.Render(render.Truncate(o.msg, maxW)))
		b.WriteString("\n\n")
	}

	if o.fetching {
		line := fmt.Sprintf("Fetching %d/%d  %s", o.progressDone, o.progressTotal, o.progressCurrent)
		b.WriteString(o.theme.OK.Render(render.Truncate(line, maxW)))
		b.WriteString("\n")
	}

	cols := o.columns(maxW)

	// Header row.
	b.WriteString(pane.RenderOverlayTableHeader(o.theme, cols, o.activeSort, o.sortAscending, o.focusedColIdx))
	b.WriteString("\n")

	if o.loading {
		b.WriteString(o.theme.MutedItalic.Render(render.Pad("Loading…", maxW)))
		b.WriteString("\n")
	} else if len(o.classifications) == 0 {
		b.WriteString(o.theme.MutedItalic.Render(render.Pad("No classifications on this patent.", maxW)))
		b.WriteString("\n")
	} else {
		// Reserved rows: header line, status, progress (when fetching), focus card.
		reserved := 5
		if o.fetching {
			reserved++
		}
		if o.err != nil || o.msg != "" {
			reserved += 2
		}
		pageSize := max(maxH-reserved, 1)
		start := max(0, o.cursor-pageSize/2)
		end := min(len(o.classifications), start+pageSize)
		if end-start < pageSize && start > 0 {
			start = max(0, end-pageSize)
		}

		for i := start; i < end; i++ {
			c := o.classifications[i]
			selected := i == o.cursor
			row := pane.RenderOverlayTableRow(o.theme, cols, i, o.focusedColIdx, selected, func(col pane.OverlayColumn) string {
				return o.cellValue(col, c)
			})
			switch {
			case o.visual.InRange(o.cursor, i):
				b.WriteString(o.theme.Visual.Render(render.Pad(row, maxW)))
			case selected:
				b.WriteString(o.theme.Selected.Render(render.Pad(row, maxW)))
			case i%2 == 1:
				b.WriteString(o.theme.RowAlt.Render(render.Pad(row, maxW)))
			default:
				b.WriteString(o.theme.Row.Render(render.Pad(row, maxW)))
			}
			b.WriteString("\n")
		}
	}

	// Focus detail card under the table.
	if len(o.classifications) > 0 && o.cursor >= 0 && o.cursor < len(o.classifications) {
		c := o.classifications[o.cursor]
		b.WriteString("\n")
		b.WriteString(o.theme.Dim.Render(strings.Repeat("─", maxW)))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("%s %s  %s %s  %s %s\n",
			o.theme.HelpKey.Render("System:"), c.System,
			o.theme.HelpKey.Render("Code:"), c.Code,
			o.theme.HelpKey.Render("Status:"), classificationStatusOf(c).String()))
		if hasUsefulDescription(c) {
			for _, line := range wrapText(c.Description, maxW) {
				b.WriteString(line)
				b.WriteString("\n")
			}
		} else {
			b.WriteString(o.missingDescriptionLabel(c))
			b.WriteString("\n")
		}
	}

	// Status line.
	extras := []string{}
	if len(o.marked) > 0 {
		extras = append(extras, fmt.Sprintf("marked: %d", len(o.marked)))
	}
	if o.visual.Mode {
		extras = append(extras, "VISUAL")
	}
	b.WriteString(pane.RenderOverlayStatusLine(o.theme, maxW, o.cursor, len(o.classifications), extras...))
	b.WriteString("\n")

	// Footer hint.
	hint := "[j/k] move  [h/l] col  [.] sort  [v] visual  [g a] all  [f] mark  [F] clear  [L] lookup  [q/Esc] close"
	b.WriteString(o.theme.Dim.Render(render.Truncate(hint, maxW)))
	return b.String()
}
