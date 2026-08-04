package pane

import (
	"fmt"
	"log/slog"
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

// officeActionsLoadedMsg delivers a finished office_action.list result.
type officeActionsLoadedMsg struct {
	requestID uint64
	items     []domain.OfficeAction
	billable  map[string]int
	err       error
}

// OpenOfficeActionDetailMsg asks the app to push the drill-down detail pane for
// one office action.
type OpenOfficeActionDetailMsg struct {
	OA domain.OfficeAction
}

// RequestDeleteOfficeActionMsg asks the app to prompt the user to confirm deleting
// the specified office action.
type RequestDeleteOfficeActionMsg struct {
	OA domain.OfficeAction
}

// OpenOfficeActionTagOverlayMsg asks the app to open the shared tag-manager
// overlay for one office action — the same overlay the catalog uses for patents,
// so office-action tagging behaves identically. (The pane cannot import the
// overlay package, so it signals the app.)
type OpenOfficeActionTagOverlayMsg struct {
	OA domain.OfficeAction
}

// OfficeActions is the matter's office-action table — the home of the
// prosecution workspace. Each row is one examiner action with its response-due
// countdown and status; `enter` drills into the detail pane (documents, timing,
// communications, response), `a` imports a new one, `R` drafts a response.
type OfficeActions struct {
	client   *rpc.Client
	theme    render.Theme
	project  domain.ProjectID
	handlers map[command.ID]cmdHandler
	logger   *slog.Logger

	allItems []domain.OfficeAction
	items    []domain.OfficeAction
	billable map[string]int // office action id → billable (validated) seconds
	page     render.Paginator
	loading  bool
	loadErr  string
	loadID   uint64

	searchActive bool
	searchQuery  string

	table render.TableState
}

// NewOfficeActions builds the office-action table pane for a project.
func NewOfficeActions(client *rpc.Client, theme render.Theme, project domain.ProjectID) *OfficeActions {
	o := &OfficeActions{
		client:  client,
		theme:   theme,
		project: project,
		page:    render.NewPaginator(20),
		loading: true,
		table:   render.NewTableState(),
	}
	o.handlers = map[command.ID]cmdHandler{
		command.NavDown:     func(inv Invocation) tea.Cmd { o.page.MoveDown(inv.Repeat); return nil },
		command.NavUp:       func(inv Invocation) tea.Cmd { o.page.MoveUp(inv.Repeat); return nil },
		command.NavPageDown: func(Invocation) tea.Cmd { o.page.PageDown(); return nil },
		command.NavPageUp:   func(Invocation) tea.Cmd { o.page.PageUp(); return nil },
		command.NavTop:      func(inv Invocation) tea.Cmd { o.page.NavTop(inv.Repeat); return nil },
		command.NavBottom:   func(inv Invocation) tea.Cmd { o.page.NavBottom(inv.Repeat); return nil },
		command.Refresh:     func(Invocation) tea.Cmd { o.loading = true; return o.load() },
		command.ColNext:     func(Invocation) tea.Cmd { return o.focusNext() },
		command.ColPrev:     func(Invocation) tea.Cmd { return o.focusPrev() },
		command.SortApply:   func(Invocation) tea.Cmd { return o.applySort() },
		command.FindOpen: func(Invocation) tea.Cmd {
			o.searchActive = true
			o.searchQuery = ""
			o.applyFilter()
			return nil
		},
	}
	return o
}

// WithLogger attaches a logger.
func (o *OfficeActions) WithLogger(l *slog.Logger) *OfficeActions { o.logger = l; return o }

func (o *OfficeActions) Scope() command.Scope { return command.ScopeMatterOA }

func (o *OfficeActions) Title() string {
	if len(o.allItems) == 0 {
		return "Office Actions"
	}
	return fmt.Sprintf("Office Actions (%d)", len(o.allItems))
}

func (o *OfficeActions) Init() tea.Cmd { return o.load() }

func (o *OfficeActions) load() tea.Cmd {
	client, project := o.client, o.project
	if client == nil {
		return nil
	}
	requestID := nextAsyncID()
	o.loadID = requestID
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.OfficeActionListResult
		err := client.Call(ctx, proto.MethodOfficeActionList, proto.OfficeActionListParams{Project: project}, &res)
		return officeActionsLoadedMsg{requestID: requestID, items: res.OfficeActions, billable: res.BillableSecs, err: err}
	}
}

func (o *OfficeActions) Command(id command.ID, inv Invocation) (Pane, tea.Cmd) {
	if handler, ok := o.handlers[id]; ok {
		return o, handler(inv)
	}
	return o, nil
}

func (o *OfficeActions) Handles() []command.ID { return handlerIDs(o.handlers) }

func (o *OfficeActions) Update(msg tea.Msg) (Pane, tea.Cmd) {
	if m, ok := msg.(officeActionsLoadedMsg); ok {
		if m.requestID != o.loadID {
			return o, nil
		}
		o.loading = false
		if m.err != nil {
			o.loadErr = m.err.Error()
			return o, nil
		}
		o.loadErr = ""
		o.allItems = m.items
		o.billable = m.billable
		o.applyFilter()
	}
	return o, nil
}

// Selection reports no patent — this pane lists office actions, not patents.
func (o *OfficeActions) Selection() (domain.PatentNumber, bool) {
	return domain.PatentNumber{}, false
}

// FullTextNumber returns the selected office action's application as a patent
// number, so :open.fulltext (T) can open the application's full text.
func (o *OfficeActions) FullTextNumber() (domain.PatentNumber, bool) {
	oa, ok := o.selected()
	if !ok {
		return domain.PatentNumber{}, false
	}
	return officeActionFullTextNumber(oa)
}

// officeActionFullTextNumber parses an office action's application number into a
// PatentNumber for the full-text viewer. ok is false when the OA carries no
// usable application number.
func officeActionFullTextNumber(oa domain.OfficeAction) (domain.PatentNumber, bool) {
	if strings.TrimSpace(oa.ApplicationNumber) == "" {
		return domain.PatentNumber{}, false
	}
	n, err := domain.ParsePatentNumber(oa.ApplicationNumber)
	if err != nil {
		return domain.PatentNumber{}, false
	}
	return n, true
}

func (o *OfficeActions) selected() (domain.OfficeAction, bool) {
	cur := o.page.Cursor()
	if cur < 0 || cur >= len(o.items) {
		return domain.OfficeAction{}, false
	}
	return o.items[cur], true
}

func (o *OfficeActions) applyFilter() {
	if o.searchQuery == "" {
		o.items = make([]domain.OfficeAction, len(o.allItems))
		copy(o.items, o.allItems)
	} else {
		o.items = nil
		q := strings.ToLower(o.searchQuery)
		for _, oa := range o.allItems {
			hay := strings.ToLower(oa.Name + " " + string(oa.Type) + " " + oa.Examiner + " " + oa.ApplicationNumber)
			if strings.Contains(hay, q) {
				o.items = append(o.items, oa)
			}
		}
	}
	o.sortItems()
	o.page.SetTotal(len(o.items))
	if o.page.Cursor() >= len(o.items) {
		o.page.NavTop(0)
	}
}

func (o *OfficeActions) sortItems() {
	if o.table.ActiveSort == "" {
		return
	}
	slices.SortFunc(o.items, func(i, j domain.OfficeAction) int {
		var cmp int
		switch o.table.ActiveSort {
		case "name":
			cmp = strings.Compare(strings.ToLower(i.Name), strings.ToLower(j.Name))
		case "type":
			cmp = strings.Compare(strings.ToLower(string(i.Type)), strings.ToLower(string(j.Type)))
		case "examiner":
			cmp = strings.Compare(strings.ToLower(i.Examiner), strings.ToLower(j.Examiner))
		case "due":
			if i.ResponseDue.IsZero() && j.ResponseDue.IsZero() {
				cmp = 0
			} else if i.ResponseDue.IsZero() {
				cmp = 1
			} else if j.ResponseDue.IsZero() {
				cmp = -1
			} else if i.ResponseDue.Before(j.ResponseDue) {
				cmp = -1
			} else if i.ResponseDue.After(j.ResponseDue) {
				cmp = 1
			} else {
				cmp = 0
			}
		case "status":
			cmp = strings.Compare(strings.ToLower(string(i.Status)), strings.ToLower(string(j.Status)))
		case "tags":
			cmp = strings.Compare(strings.ToLower(tagsText(i.Tags)), strings.ToLower(tagsText(j.Tags)))
		case "worklog":
			secsI := o.billable[i.ID]
			secsJ := o.billable[j.ID]
			if secsI < secsJ {
				cmp = -1
			} else if secsI > secsJ {
				cmp = 1
			} else {
				cmp = 0
			}
		}
		if !o.table.SortAscending {
			cmp = -cmp
		}
		if cmp == 0 {
			if i.MailDate.IsZero() && j.MailDate.IsZero() {
				cmp = strings.Compare(i.ID, j.ID)
			} else if i.MailDate.IsZero() {
				cmp = 1
			} else if j.MailDate.IsZero() {
				cmp = -1
			} else if i.MailDate.Before(j.MailDate) {
				cmp = 1
			} else if i.MailDate.After(j.MailDate) {
				cmp = -1
			} else {
				cmp = strings.Compare(i.ID, j.ID)
			}
		}
		return cmp
	})
}

func (o *OfficeActions) currentCols(w int) []render.TableColumn {
	const colGap = 1
	nameW, typeW, dueW, statusW, tagsW, worklogW := 20, 12, 14, 16, 14, 8
	examinerW := max(10, w-o.theme.TablePrefixWidth()-nameW-typeW-dueW-statusW-tagsW-worklogW-6*colGap)
	return []render.TableColumn{
		{Key: "name", Label: "NAME", SortKey: "name", Width: nameW},
		{Key: "type", Label: "TYPE", SortKey: "type", Width: typeW},
		{Key: "examiner", Label: "EXAMINER", SortKey: "examiner", Width: examinerW},
		{Key: "due", Label: "RESPONSE DUE", SortKey: "due", Width: dueW},
		{Key: "status", Label: "STATUS", SortKey: "status", Width: statusW},
		{Key: "tags", Label: "TAGS", SortKey: "tags", Width: tagsW},
		{Key: "worklog", Label: "WORKLOG", SortKey: "worklog", Width: worklogW},
	}
}

func (o *OfficeActions) focusNext() tea.Cmd {
	o.table.MoveFocus(o.currentCols(80), 1)
	return nil
}

func (o *OfficeActions) focusPrev() tea.Cmd {
	o.table.MoveFocus(o.currentCols(80), -1)
	return nil
}

func (o *OfficeActions) applySort() tea.Cmd {
	if o.table.ApplySort(o.currentCols(80)) {
		o.applyFilter()
	}
	return nil
}

// HandleKey intercepts text entry while searching and the drill-in keys; every
// other key falls through to the keymap (nav, find, refresh, add, respond).
func (o *OfficeActions) HandleKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool) {
	if o.searchActive {
		switch msg.Type {
		case tea.KeyEsc:
			o.searchActive = false
			o.searchQuery = ""
			o.applyFilter()
		case tea.KeyEnter:
			o.searchActive = false
		case tea.KeyBackspace, tea.KeyDelete:
			if len(o.searchQuery) > 0 {
				r := []rune(o.searchQuery)
				o.searchQuery = string(r[:len(r)-1])
				o.applyFilter()
			}
		case tea.KeyRunes, tea.KeySpace:
			o.searchQuery += msg.String()
			o.applyFilter()
		}
		return o, nil, true
	}
	switch msg.String() {
	case "enter":
		oa, ok := o.selected()
		if !ok {
			return o, nil, true
		}
		return o, func() tea.Msg { return OpenOfficeActionDetailMsg{OA: oa} }, true
	case "ctrl+n":
		// Jump straight from the list into the selected action's notes editor,
		// skipping the detail pane (which also offers ctrl+n).
		oa, ok := o.selected()
		if !ok {
			return o, nil, true
		}
		return o, func() tea.Msg { return OpenOfficeActionEditorMsg{OA: oa} }, true
	case "t":
		// Open the shared tag-manager overlay for the selected action — identical
		// to patent tagging in the catalog.
		oa, ok := o.selected()
		if !ok {
			return o, nil, true
		}
		return o, func() tea.Msg { return OpenOfficeActionTagOverlayMsg{OA: oa} }, true
	case "D":
		oa, ok := o.selected()
		if !ok {
			return o, nil, true
		}
		return o, func() tea.Msg { return RequestDeleteOfficeActionMsg{OA: oa} }, true
	}
	return o, nil, false
}

func (o *OfficeActions) View(w, h int) string {
	switch {
	case o.loading && len(o.allItems) == 0:
		return o.theme.Dim.Render("loading office actions…")
	case o.loadErr != "":
		return o.theme.Error.Render("error: " + o.loadErr)
	case len(o.allItems) == 0:
		return o.theme.Dim.Render("No office actions yet. Press a (or :officeaction.add) to import one.")
	}
	o.page.SetPageSize(max(h-headerRows-2, 1))

	var statusText string
	if o.table.ActiveSort != "" {
		statusText = "sort:" + o.table.ActiveSort
		if o.table.SortAscending {
			statusText += " (asc)"
		} else {
			statusText += " (desc)"
		}
	}

	cols := o.currentCols(w)
	start, end := o.page.Window()
	var b strings.Builder
	b.WriteString(renderTableStatusLine(o.theme, w, o.page.Cursor(), o.page.Total(), statusText))
	b.WriteByte('\n')
	b.WriteString(render.RenderTable(render.TableParams{
		Theme:         o.theme,
		Columns:       cols,
		RowCount:      end - start,
		FocusedColIdx: o.table.FocusedColIdx,
		ActiveSort:    o.table.ActiveSort,
		SortAscending: o.table.SortAscending,
		FocusActive:   true,
		IsRowCursor:   func(rowIdx int) bool { return start+rowIdx == o.page.Cursor() },
	}, w, func(rowIdx, colIdx int) string {
		absIdx := start + rowIdx
		if absIdx < 0 || absIdx >= len(o.items) {
			return ""
		}
		oa := o.items[absIdx]
		switch cols[colIdx].Key {
		case "name":
			return nameLabel(oa)
		case "type":
			return oaTypeLabel(oa.Type)
		case "examiner":
			return orDash(oa.Examiner)
		case "due":
			return responseDueLabel(oa)
		case "status":
			return statusAgeLabel(oa)
		case "tags":
			return tagsText(oa.Tags)
		case "worklog":
			return worklogLabel(o.billable[oa.ID])
		default:
			return ""
		}
	}))
	b.WriteByte('\n')
	switch {
	case o.searchActive:
		b.WriteString(o.theme.Selected.Render(render.Pad("/ "+o.searchQuery+"▋", w)))
	default:
		b.WriteString(o.theme.Dim.Render(render.Pad(
			"  [enter] open  [ctrl+n] notes  [t] tag  [a] add  [D] delete  [R] respond  [/] filter", w)))
	}
	return b.String()
}

func dateOrDash(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format(domain.DateLayout)
}

func oaTypeLabel(t domain.OAType) string {
	if t == "" {
		return "—"
	}
	return strings.ReplaceAll(string(t), "_", " ")
}

func statusLabel(s domain.OAStatus) string {
	if s == "" {
		return string(domain.OAStatusOpen)
	}
	return string(s)
}

// nameLabel is the office action's name, or a placeholder when it has none.
func nameLabel(oa domain.OfficeAction) string {
	if strings.TrimSpace(oa.Name) == "" {
		return "(unnamed)"
	}
	return oa.Name
}

// statusAgeLabel renders the current status together with how long it has held,
// e.g. "open · 5d" or "responded · 2d". The age is omitted when unknown.
func statusAgeLabel(oa domain.OfficeAction) string {
	label := statusLabel(oa.Status)
	if oa.StatusChangedAt.IsZero() {
		return label
	}
	return label + " · " + shortAge(oa.StatusChangedAt)
}

// worklogLabel renders an office action's billable time, or a dash when none has
// been validated yet.
func worklogLabel(secs int) string {
	if secs <= 0 {
		return "—"
	}
	return fmtDuration(secs)
}

// shortAge renders a compact, human elapsed time since t: "just now", "12m",
// "3h", or "5d". It is a coarse at-a-glance cue, not a precise duration.
func shortAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// responseDueLabel renders the deadline with an at-a-glance urgency cue: overdue,
// due-soon day count, or the date.
func responseDueLabel(oa domain.OfficeAction) string {
	if oa.ResponseDue.IsZero() {
		return "—"
	}
	if oa.Status != "" && oa.Status != domain.OAStatusOpen {
		return oa.ResponseDue.Format(domain.DateLayout)
	}
	days := int(time.Until(oa.ResponseDue).Hours() / 24)
	switch {
	case days < 0:
		return "OVERDUE"
	case days == 0:
		return "due today"
	case days <= 14:
		return fmt.Sprintf("%dd · %s", days, oa.ResponseDue.Format(domain.DateLayout))
	default:
		return oa.ResponseDue.Format(domain.DateLayout)
	}
}
