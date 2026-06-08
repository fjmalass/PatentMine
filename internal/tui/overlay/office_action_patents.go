package overlay

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/tui/render"
)

// OfficeActionPatents is the two-pane assignment overlay for one office action:
// the top table lists the reference patents assigned to it for review, the bottom
// table lists every patent in the matter. It mirrors the document overlay's
// interaction (built on the shared dualTable): tab switches panes, a assigns the
// selected patent (bottom) to this OA, x releases it (top), v toggles its review
// status, enter opens the patent. Assignments show in the catalog OA column.
type OfficeActionPatents struct {
	client  *rpc.Client
	theme   render.Theme
	oa      domain.OfficeAction
	project domain.ProjectID

	table    dualTable
	assigned []proto.OfficeActionAssignedPatent // top pane
	all      []domain.PatentRow                 // bottom pane (all matter patents)
	status   map[string]domain.OAReviewStatus   // normalized number → review status, for the bottom marker

	loading bool
	loadErr string
	msg     string
}

type oaPatentsLoadedMsg struct {
	assigned []proto.OfficeActionAssignedPatent
	all      []domain.PatentRow
	err      error
}

// NewOfficeActionPatents builds the assignment overlay for one office action.
func NewOfficeActionPatents(client *rpc.Client, theme render.Theme, project domain.ProjectID, oa domain.OfficeAction) *OfficeActionPatents {
	return &OfficeActionPatents{
		client:  client,
		theme:   theme,
		oa:      oa,
		project: project,
		table:   newDualTable(),
		loading: true,
	}
}

func (o *OfficeActionPatents) Title() string {
	return "Patents for Office Action — " + o.oa.DisplayLabel()
}

func (o *OfficeActionPatents) Handles() []command.ID { return nil }

func (o *OfficeActionPatents) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *OfficeActionPatents) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, 88, 85, 70, 18)
}

func (o *OfficeActionPatents) Init() tea.Cmd { return o.loadCmd() }

func (o *OfficeActionPatents) loadCmd() tea.Cmd {
	client, project, oaID := o.client, o.project, o.oa.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var assignedRes proto.OfficeActionPatentsResult
		if err := client.Call(ctx, proto.MethodOfficeActionPatents, proto.OfficeActionPatentsParams{ID: oaID}, &assignedRes); err != nil {
			return oaPatentsLoadedMsg{err: err}
		}
		var allRes proto.PatentListResult
		if err := client.Call(ctx, proto.MethodPatentList, proto.PatentListParams{Project: project, Limit: 5000}, &allRes); err != nil {
			return oaPatentsLoadedMsg{err: err}
		}
		return oaPatentsLoadedMsg{assigned: assignedRes.Patents, all: allRes.Patents}
	}
}

func (o *OfficeActionPatents) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if m, ok := msg.(oaPatentsLoadedMsg); ok {
		o.loading = false
		if m.err != nil {
			o.loadErr = m.err.Error()
			return o, nil
		}
		o.loadErr = ""
		o.assigned = m.assigned
		o.all = m.all
		o.status = make(map[string]domain.OAReviewStatus, len(m.assigned))
		for _, a := range m.assigned {
			o.status[a.Number.Normalized()] = a.Status
		}
		o.sortActive()
		o.table.clamp(o.rowCounts(), o.colCounts())
	}
	return o, nil
}

func (o *OfficeActionPatents) rowCounts() [2]int { return [2]int{len(o.assigned), len(o.all)} }
func (o *OfficeActionPatents) colCounts() [2]int { return [2]int{len(topOAPatentCols), len(allOAPatentCols)} }

// topOAPatentCols / allOAPatentCols are the column keys per pane (labels/widths
// are computed per render against the available width).
var topOAPatentCols = []string{"number", "title", "status", "assigned"}
var allOAPatentCols = []string{"number", "title", "oa"}

func (o *OfficeActionPatents) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if o.loading {
		return o, nil, true
	}
	if handled, sortReq := o.table.HandleNav(msg, o.rowCounts(), o.colCounts()); handled {
		if sortReq {
			o.sortActive()
		}
		return o, nil, true
	}
	switch msg.String() {
	case "esc", "q":
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case "a":
		// Assign the selected patent (bottom pane) to this office action.
		if o.table.Active() != 1 {
			o.msg = "switch to the lower table (tab) to assign"
			return o, nil, true
		}
		row, ok := o.selectedAll()
		if !ok {
			return o, nil, true
		}
		return o, o.mutate(proto.MethodOfficeActionAssignPatents, proto.OfficeActionAssignPatentsParams{ID: o.oa.ID, Patents: []domain.PatentNumber{row.Number}}), true
	case "x":
		// Release the selected patent (top pane) from this office action.
		if o.table.Active() != 0 {
			o.msg = "switch to the upper table (tab) to remove"
			return o, nil, true
		}
		a, ok := o.selectedAssigned()
		if !ok {
			return o, nil, true
		}
		return o, o.mutate(proto.MethodOfficeActionReleasePatents, proto.OfficeActionReleasePatentsParams{ID: o.oa.ID, Patents: []domain.PatentNumber{a.Number}}), true
	case "v":
		// Toggle the selected assigned patent's review status.
		if o.table.Active() != 0 {
			o.msg = "review status toggles on the upper (assigned) table"
			return o, nil, true
		}
		a, ok := o.selectedAssigned()
		if !ok {
			return o, nil, true
		}
		next := domain.OAReviewReviewed
		if a.Status == domain.OAReviewReviewed {
			next = domain.OAReviewToReview
		}
		return o, o.mutate(proto.MethodOfficeActionReviewStatus, proto.OfficeActionReviewStatusParams{ID: o.oa.ID, Patent: a.Number, Status: next}), true
	case "enter":
		if n, ok := o.selectedNumber(); ok {
			return o, func() tea.Msg { return OpenPatentDetailMsg{Number: n} }, true
		}
	}
	return o, nil, true
}

// mutate runs an assign/release/status RPC and reloads both panes on success.
func (o *OfficeActionPatents) mutate(method proto.Method, params any) tea.Cmd {
	client := o.client
	reload := o.loadCmd()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.OfficeActionPatentsResult
		if err := client.Call(ctx, method, params, &res); err != nil {
			return oaPatentsLoadedMsg{err: err}
		}
		return reload()
	}
}

func (o *OfficeActionPatents) selectedAssigned() (proto.OfficeActionAssignedPatent, bool) {
	i := o.table.CursorIn(0)
	if i < 0 || i >= len(o.assigned) {
		return proto.OfficeActionAssignedPatent{}, false
	}
	return o.assigned[i], true
}

func (o *OfficeActionPatents) selectedAll() (domain.PatentRow, bool) {
	i := o.table.CursorIn(1)
	if i < 0 || i >= len(o.all) {
		return domain.PatentRow{}, false
	}
	return o.all[i], true
}

// selectedNumber returns the canonical number under the cursor in whichever pane
// is active.
func (o *OfficeActionPatents) selectedNumber() (domain.PatentNumber, bool) {
	if o.table.Active() == 0 {
		if a, ok := o.selectedAssigned(); ok {
			return a.Number, true
		}
		return domain.PatentNumber{}, false
	}
	if r, ok := o.selectedAll(); ok {
		return r.Number, true
	}
	return domain.PatentNumber{}, false
}

// sortActive re-orders the active pane by its current sort column/direction.
func (o *OfficeActionPatents) sortActive() {
	col := o.table.SortColumn()
	desc := o.table.SortDescending()
	flip := func(c int) int {
		if desc {
			return -c
		}
		return c
	}
	if o.table.Active() == 0 {
		slices.SortFunc(o.assigned, func(a, b proto.OfficeActionAssignedPatent) int {
			var c int
			switch topOAPatentCols[col] {
			case "title":
				c = strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
			case "status":
				c = strings.Compare(string(a.Status), string(b.Status))
			case "assigned":
				c = a.AssignedAt.Compare(b.AssignedAt)
			default:
				c = strings.Compare(a.Number.Normalized(), b.Number.Normalized())
			}
			return flip(c)
		})
		return
	}
	slices.SortFunc(o.all, func(a, b domain.PatentRow) int {
		var c int
		switch allOAPatentCols[col] {
		case "title":
			c = strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
		case "oa":
			c = boolCompare(o.isAssigned(a.Number), o.isAssigned(b.Number))
		default:
			c = strings.Compare(a.Number.Normalized(), b.Number.Normalized())
		}
		return flip(c)
	})
}

func (o *OfficeActionPatents) isAssigned(n domain.PatentNumber) bool {
	_, ok := o.status[n.Normalized()]
	return ok
}

func (o *OfficeActionPatents) View(maxW, maxH int) string {
	if o.loading {
		return o.theme.Dim.Render("loading patents…")
	}
	if o.loadErr != "" {
		return o.theme.Error.Render("error: " + o.loadErr)
	}

	titleW := max(20, maxW-46)
	top := dualTablePane{
		heading:  fmt.Sprintf("Assigned to this office action (%d):", len(o.assigned)),
		columns:  oaPatentColumns(topOAPatentCols, titleW),
		rowCount: len(o.assigned),
		cell:     o.assignedCell,
	}
	allTitleW := max(20, maxW-32)
	bottom := dualTablePane{
		heading:  fmt.Sprintf("All patents in this matter (%d):", len(o.all)),
		columns:  oaPatentColumns(allOAPatentCols, allTitleW),
		rowCount: len(o.all),
		cell:     o.allCell,
	}

	bodyH := maxH - 2
	body := o.table.Render(o.theme, maxW, bodyH, top, bottom)

	hint := "[a] assign  [x] remove  [v] reviewed  [enter] detail  [tab] switch  [.] sort  [esc] close"
	if o.msg != "" {
		hint = o.msg + "  ·  " + hint
	}
	return body + "\n" + o.theme.Dim.Render(render.Truncate(hint, maxW))
}

func (o *OfficeActionPatents) assignedCell(row, col int) string {
	if row < 0 || row >= len(o.assigned) {
		return ""
	}
	a := o.assigned[row]
	switch topOAPatentCols[col] {
	case "number":
		return oaPatentDisplay(a.Display, a.Number)
	case "title":
		return a.Title
	case "status":
		return reviewStatusLabel(a.Status)
	case "assigned":
		if a.AssignedAt.IsZero() {
			return "—"
		}
		return a.AssignedAt.Format(domain.DateLayout)
	}
	return ""
}

func (o *OfficeActionPatents) allCell(row, col int) string {
	if row < 0 || row >= len(o.all) {
		return ""
	}
	r := o.all[row]
	switch allOAPatentCols[col] {
	case "number":
		return oaPatentDisplay(r.DisplayNumber, r.Number)
	case "title":
		return r.Title
	case "oa":
		if st, ok := o.status[r.Number.Normalized()]; ok {
			return reviewStatusGlyph(st)
		}
		return "—"
	}
	return ""
}

// oaPatentColumns builds render columns for the given keys, sizing the title
// column to the available width.
func oaPatentColumns(keys []string, titleW int) []render.TableColumn {
	cols := make([]render.TableColumn, 0, len(keys))
	for _, k := range keys {
		switch k {
		case "number":
			cols = append(cols, render.TableColumn{Key: k, Label: "NUMBER", Width: 16})
		case "title":
			cols = append(cols, render.TableColumn{Key: k, Label: "TITLE", Width: titleW})
		case "status":
			cols = append(cols, render.TableColumn{Key: k, Label: "STATUS", Width: 12})
		case "assigned":
			cols = append(cols, render.TableColumn{Key: k, Label: "ASSIGNED", Width: 12})
		case "oa":
			cols = append(cols, render.TableColumn{Key: k, Label: "ASSIGNED", Width: 10})
		}
	}
	return cols
}

func oaPatentDisplay(display, number domain.PatentNumber) string {
	if !display.IsZero() {
		return display.DisplayString()
	}
	return number.DisplayString()
}

func reviewStatusGlyph(s domain.OAReviewStatus) string {
	if s == domain.OAReviewReviewed {
		return "✓"
	}
	return "○"
}

func reviewStatusLabel(s domain.OAReviewStatus) string {
	if s == domain.OAReviewReviewed {
		return "✓ reviewed"
	}
	return "○ to-review"
}

func boolCompare(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	default:
		return -1
	}
}
