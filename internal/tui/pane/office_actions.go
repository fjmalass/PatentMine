package pane

import (
	"fmt"
	"log/slog"
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
	page     render.Paginator
	loading  bool
	loadErr  string
	loadID   uint64

	searchActive bool
	searchQuery  string
}

// NewOfficeActions builds the office-action table pane for a project.
func NewOfficeActions(client *rpc.Client, theme render.Theme, project domain.ProjectID) *OfficeActions {
	o := &OfficeActions{
		client:  client,
		theme:   theme,
		project: project,
		page:    render.NewPaginator(20),
		loading: true,
	}
	o.handlers = map[command.ID]cmdHandler{
		command.NavDown:     func(inv Invocation) tea.Cmd { o.page.MoveDown(inv.Repeat); return nil },
		command.NavUp:       func(inv Invocation) tea.Cmd { o.page.MoveUp(inv.Repeat); return nil },
		command.NavPageDown: func(Invocation) tea.Cmd { o.page.PageDown(); return nil },
		command.NavPageUp:   func(Invocation) tea.Cmd { o.page.PageUp(); return nil },
		command.NavTop:      func(inv Invocation) tea.Cmd { o.page.NavTop(inv.Repeat); return nil },
		command.NavBottom:   func(inv Invocation) tea.Cmd { o.page.NavBottom(inv.Repeat); return nil },
		command.Refresh:     func(Invocation) tea.Cmd { o.loading = true; return o.load() },
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
		return officeActionsLoadedMsg{requestID: requestID, items: res.OfficeActions, err: err}
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
		o.applyFilter()
	}
	return o, nil
}

// Selection reports no patent — this pane lists office actions, not patents.
func (o *OfficeActions) Selection() (domain.PatentNumber, bool) {
	return domain.PatentNumber{}, false
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
		o.items = o.allItems
	} else {
		o.items = nil
		q := strings.ToLower(o.searchQuery)
		for _, oa := range o.allItems {
			hay := strings.ToLower(string(oa.Type) + " " + oa.Examiner + " " + oa.ApplicationNumber)
			if strings.Contains(hay, q) {
				o.items = append(o.items, oa)
			}
		}
	}
	o.page.SetTotal(len(o.items))
	if o.page.Cursor() >= len(o.items) {
		o.page.NavTop(0)
	}
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
	case "enter", "l", "right":
		oa, ok := o.selected()
		if !ok {
			return o, nil, true
		}
		return o, func() tea.Msg { return OpenOfficeActionDetailMsg{OA: oa} }, true
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
		return o.theme.Dim.Render("No office actions yet. Press a (or :add.officeaction) to import one.")
	}
	o.page.SetPageSize(max(h-headerRows-2, 1))

	cols := []render.TableColumn{
		{Key: "date", Label: "MAILED", Width: 12},
		{Key: "type", Label: "TYPE", Width: 14},
		{Key: "examiner", Label: "EXAMINER", Width: max(10, (w-12-14-14-10)/2)},
		{Key: "due", Label: "RESPONSE DUE", Width: 16},
		{Key: "status", Label: "STATUS", Width: 10},
	}
	start, end := o.page.Window()
	var b strings.Builder
	b.WriteString(renderTableStatusLine(o.theme, w, o.page.Cursor(), o.page.Total(), ""))
	b.WriteByte('\n')
	b.WriteString(render.RenderTable(render.TableParams{
		Theme:       o.theme,
		Columns:     cols,
		RowCount:    end - start,
		FocusActive: true,
		IsRowCursor: func(rowIdx int) bool { return start+rowIdx == o.page.Cursor() },
	}, w, func(rowIdx, colIdx int) string {
		absIdx := start + rowIdx
		if absIdx < 0 || absIdx >= len(o.items) {
			return ""
		}
		oa := o.items[absIdx]
		switch cols[colIdx].Key {
		case "date":
			return dateOrDash(oa.MailDate)
		case "type":
			return oaTypeLabel(oa.Type)
		case "examiner":
			return orDash(oa.Examiner)
		case "due":
			return responseDueLabel(oa)
		case "status":
			return statusLabel(oa.Status)
		default:
			return ""
		}
	}))
	b.WriteByte('\n')
	if o.searchActive {
		b.WriteString(o.theme.Selected.Render(render.Pad("/ "+o.searchQuery+"▋", w)))
	} else {
		b.WriteString(o.theme.Dim.Render(render.Pad(
			"  [enter] open  [a] add  [D] delete  [R] respond  [/] filter  [esc] back", w)))
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
