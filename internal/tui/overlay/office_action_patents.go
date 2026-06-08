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
	"patentmine/internal/tui/pane"
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
	all      []domain.PatentRow                 // bottom pane (filtered + sorted view of rawAll)
	rawAll   []domain.PatentRow                 // every matter patent, before the search filter
	status   map[string]domain.OAReviewStatus   // normalized number → review status, for the bottom marker

	searchActive bool   // typing into the bottom-table search box
	searchQuery  string // current filter applied to the bottom table

	loading bool
	loadErr string
	msg     string
}

type oaPatentsLoadedMsg struct {
	assigned []proto.OfficeActionAssignedPatent
	all      []domain.PatentRow
	err      error
}

type oaPatentsMutatedMsg struct {
	patents []domain.PatentNumber
	err     error
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
	switch m := msg.(type) {
	case oaPatentsLoadedMsg:
		o.loading = false
		if m.err != nil {
			o.loadErr = m.err.Error()
			return o, nil
		}
		o.loadErr = ""
		o.assigned = m.assigned
		o.rawAll = m.all
		o.status = make(map[string]domain.OAReviewStatus, len(m.assigned))
		for _, a := range m.assigned {
			o.status[a.Number.Normalized()] = a.Status
		}
		o.applyFilter() // builds o.all (filtered) and sorts it
		o.sortAssigned()
		o.table.clamp(o.rowCounts(), o.colCounts())
	case oaPatentsMutatedMsg:
		if m.err != nil {
			o.loadErr = m.err.Error()
			return o, nil
		}
		return o, tea.Batch(
			o.loadCmd(),
			func() tea.Msg { return pane.OfficeActionAssignmentsChangedMsg{Project: o.project, Patents: m.patents} },
		)
	}
	return o, nil
}

// applyFilter rebuilds the bottom (all-patents) view from rawAll, keeping only
// rows matching the search query (number / title / inventor / assignee), then
// re-sorts it. The assigned (top) table is not filtered.
func (o *OfficeActionPatents) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(o.searchQuery))
	if q == "" {
		o.all = make([]domain.PatentRow, len(o.rawAll))
		copy(o.all, o.rawAll)
	} else {
		o.all = o.all[:0]
		for _, r := range o.rawAll {
			hay := strings.ToLower(strings.Join([]string{
				r.Number.DisplayString(), r.DisplayNumber.DisplayString(),
				r.Title, domain.ShortInventors(r.Inventors), r.Assignee,
			}, " "))
			if strings.Contains(hay, q) {
				o.all = append(o.all, r)
			}
		}
	}
	o.sortAll()
}

func (o *OfficeActionPatents) rowCounts() [2]int { return [2]int{len(o.assigned), len(o.all)} }
func (o *OfficeActionPatents) colCounts() [2]int {
	return [2]int{len(topOAPatentCols), len(allOAPatentCols)}
}

// Column keys for the two panes. Defined once and referenced by the column
// slices, cell renderers, and sort comparators below so the identifier is never
// retyped as a bare string.
const (
	oaColNumber   = "number"
	oaColTitle    = "title"
	oaColInventor = "inventor"
	oaColAssignee = "assignee"
	oaColStatus   = "status"
	oaColAssigned = "assigned"
	oaColMarker   = "oa" // "assigned?" marker in the all-patents pane
)

// topOAPatentCols / allOAPatentCols are the column keys per pane (labels/widths
// are computed per render against the available width).
var topOAPatentCols = []string{oaColNumber, oaColTitle, oaColInventor, oaColAssignee, oaColStatus, oaColAssigned}
var allOAPatentCols = []string{oaColNumber, oaColTitle, oaColInventor, oaColAssignee, oaColMarker}

func (o *OfficeActionPatents) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if o.loading {
		return o, nil, true
	}
	if o.searchActive {
		switch msg.Type {
		case tea.KeyEsc:
			o.searchActive = false
			o.searchQuery = ""
			o.applyFilter()
			o.table.clamp(o.rowCounts(), o.colCounts())
		case tea.KeyEnter:
			o.searchActive = false
		case tea.KeyBackspace, tea.KeyDelete:
			if len(o.searchQuery) > 0 {
				r := []rune(o.searchQuery)
				o.searchQuery = string(r[:len(r)-1])
				o.applyFilter()
				o.table.clamp(o.rowCounts(), o.colCounts())
			}
		case tea.KeyRunes, tea.KeySpace:
			o.searchQuery += msg.String()
			o.applyFilter()
			o.table.clamp(o.rowCounts(), o.colCounts())
		}
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
	case "/":
		// Search filters the all-patents (browse) table; focus it so results show.
		o.searchActive = true
		o.table.SetActive(1)
		return o, nil, true
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
		patents := []domain.PatentNumber{row.Number}
		return o, o.mutate(proto.MethodOfficeActionAssignPatents, proto.OfficeActionAssignPatentsParams{ID: o.oa.ID, Patents: patents}, patents), true
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
		patents := []domain.PatentNumber{a.Number}
		return o, o.mutate(proto.MethodOfficeActionReleasePatents, proto.OfficeActionReleasePatentsParams{ID: o.oa.ID, Patents: patents}, patents), true
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
		return o, o.mutate(proto.MethodOfficeActionReviewStatus, proto.OfficeActionReviewStatusParams{ID: o.oa.ID, Patent: a.Number, Status: next}, []domain.PatentNumber{a.Number}), true
	case "enter":
		if n, ok := o.selectedNumber(); ok {
			return o, func() tea.Msg { return OpenPatentDetailMsg{Number: n} }, true
		}
	}
	return o, nil, true
}

// mutate runs an assign/release/status RPC and reloads both panes on success.
func (o *OfficeActionPatents) mutate(method proto.Method, params any, patents []domain.PatentNumber) tea.Cmd {
	client := o.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.OfficeActionPatentsResult
		if err := client.Call(ctx, method, params, &res); err != nil {
			return oaPatentsMutatedMsg{err: err}
		}
		return oaPatentsMutatedMsg{patents: patents}
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
	if o.table.Active() == 0 {
		o.sortAssigned()
	} else {
		o.sortAll()
	}
}

func (o *OfficeActionPatents) sortAssigned() {
	col, desc := o.table.SortColIn(0), o.table.SortDescIn(0)
	flip := flipFunc(desc)
	slices.SortFunc(o.assigned, func(a, b proto.OfficeActionAssignedPatent) int {
		var c int
		switch topOAPatentCols[col] {
		case oaColTitle:
			c = strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
		case oaColInventor:
			c = strings.Compare(strings.ToLower(a.Inventor), strings.ToLower(b.Inventor))
		case oaColAssignee:
			c = strings.Compare(strings.ToLower(a.Assignee), strings.ToLower(b.Assignee))
		case oaColStatus:
			c = strings.Compare(string(a.Status), string(b.Status))
		case oaColAssigned:
			c = a.AssignedAt.Compare(b.AssignedAt)
		default:
			c = strings.Compare(a.Number.Normalized(), b.Number.Normalized())
		}
		return flip(c)
	})
}

func (o *OfficeActionPatents) sortAll() {
	col, desc := o.table.SortColIn(1), o.table.SortDescIn(1)
	flip := flipFunc(desc)
	slices.SortFunc(o.all, func(a, b domain.PatentRow) int {
		var c int
		switch allOAPatentCols[col] {
		case oaColTitle:
			c = strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
		case oaColInventor:
			c = strings.Compare(strings.ToLower(domain.ShortInventors(a.Inventors)), strings.ToLower(domain.ShortInventors(b.Inventors)))
		case oaColAssignee:
			c = strings.Compare(strings.ToLower(a.Assignee), strings.ToLower(b.Assignee))
		case oaColMarker:
			c = boolCompare(o.isAssigned(a.Number), o.isAssigned(b.Number))
		default:
			c = strings.Compare(a.Number.Normalized(), b.Number.Normalized())
		}
		return flip(c)
	})
}

func flipFunc(desc bool) func(int) int {
	return func(c int) int {
		if desc {
			return -c
		}
		return c
	}
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

	// number(16)+inventor(18)+assignee(18) + status/assigned or marker; the title
	// column flexes into the rest.
	top := dualTablePane{
		heading:  fmt.Sprintf("Assigned to this office action (%d):", len(o.assigned)),
		columns:  oaPatentColumns(topOAPatentCols, max(16, maxW-92)),
		rowCount: len(o.assigned),
		cell:     o.assignedCell,
	}
	bottom := dualTablePane{
		heading:  fmt.Sprintf("All patents in this matter (%d):", len(o.all)),
		columns:  oaPatentColumns(allOAPatentCols, max(16, maxW-74)),
		rowCount: len(o.all),
		cell:     o.allCell,
	}

	searching := o.searchActive || o.searchQuery != ""
	bodyH := maxH - 2
	if searching {
		bodyH = maxH - 3
	}
	body := o.table.Render(o.theme, maxW, bodyH, top, bottom)

	hint := "[a] assign  [x] remove  [v] reviewed  [/] search  [enter] detail  [tab] switch  [pgup/dn] page  [.] sort  [esc] close"
	if o.msg != "" {
		hint = o.msg + "  ·  " + hint
	}
	out := body + "\n"
	if searching {
		prompt := "/" + o.searchQuery
		if o.searchActive {
			prompt += "▋"
		}
		prompt += fmt.Sprintf("   (%d of %d patents)", len(o.all), len(o.rawAll))
		out += o.theme.Selected.Render(render.Pad(render.Truncate(prompt, maxW), maxW)) + "\n"
	}
	return out + o.theme.Dim.Render(render.Truncate(hint, maxW))
}

// orDashText returns s, or an em dash when blank.
func orDashText(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func (o *OfficeActionPatents) assignedCell(row, col int) string {
	if row < 0 || row >= len(o.assigned) {
		return ""
	}
	a := o.assigned[row]
	switch topOAPatentCols[col] {
	case oaColNumber:
		return oaPatentDisplay(a.Display, a.Number)
	case oaColTitle:
		return a.Title
	case oaColInventor:
		return orDashText(a.Inventor)
	case oaColAssignee:
		return orDashText(a.Assignee)
	case oaColStatus:
		return a.Status.Glyph() + " " + a.Status.Label()
	case oaColAssigned:
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
	case oaColNumber:
		return oaPatentDisplay(r.DisplayNumber, r.Number)
	case oaColTitle:
		return r.Title
	case oaColInventor:
		return domain.ShortInventors(r.Inventors)
	case oaColAssignee:
		return orDashText(r.Assignee)
	case oaColMarker:
		if st, ok := o.status[r.Number.Normalized()]; ok {
			return st.Glyph()
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
		case oaColNumber:
			cols = append(cols, render.TableColumn{Key: k, Label: "NUMBER", Width: 16})
		case oaColTitle:
			cols = append(cols, render.TableColumn{Key: k, Label: "TITLE", Width: titleW})
		case oaColInventor:
			cols = append(cols, render.TableColumn{Key: k, Label: "INVENTOR", Width: 18})
		case oaColAssignee:
			cols = append(cols, render.TableColumn{Key: k, Label: "ASSIGNEE", Width: 18})
		case oaColStatus:
			cols = append(cols, render.TableColumn{Key: k, Label: "STATUS", Width: 12})
		case oaColAssigned:
			cols = append(cols, render.TableColumn{Key: k, Label: "ASSIGNED", Width: 12})
		case oaColMarker:
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
