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

// OpenOfficeActionEditorMsg asks the app to open the examiner-text / notes split
// editor for one office action (the pane cannot import the overlay package).
type OpenOfficeActionEditorMsg struct {
	OA domain.OfficeAction
}

// OpenOfficeActionEditFormMsg asks the app to open the office action metadata edit form.
type OpenOfficeActionEditFormMsg struct {
	OA domain.OfficeAction
}

// OpenOfficeActionPatentsMsg asks the app to open the patent-assignment overlay
// for one office action (the two-pane "assigned / all patents" view).
type OpenOfficeActionPatentsMsg struct {
	OA domain.OfficeAction
}

// OpenOfficeActionDocumentsMsg asks the app to open the document manager scoped
// to this office action.
type OpenOfficeActionDocumentsMsg struct {
	OA domain.OfficeAction
}

type oaDetailLoadedMsg struct {
	requestID uint64
	oa        domain.OfficeAction
	assigned  []proto.OfficeActionAssignedPatent
	documents []domain.MatterDocument
	comms     int
	time      domain.TimeSummary
	ai        domain.AIUsageSummary
	err       error
}

// OfficeActionDetail is the drill-down for one office action: its metadata and
// response deadline, plus the matter's document/communication counts and the
// time + AI-usage tally. Action keys reach the matter's documents (f),
// communications (c), a drafted response (R), and the notes editor (enter).
type OfficeActionDetail struct {
	client   *rpc.Client
	theme    render.Theme
	oa       domain.OfficeAction
	handlers map[command.ID]cmdHandler
	logger   *slog.Logger

	loading   bool
	loadErr   string
	loadID    uint64
	row       int
	documents []domain.MatterDocument
	comms     int
	time      domain.TimeSummary
	ai        domain.AIUsageSummary
	assigned  []proto.OfficeActionAssignedPatent
}

// NewOfficeActionDetail builds the drill-down pane for one office action.
func NewOfficeActionDetail(client *rpc.Client, theme render.Theme, oa domain.OfficeAction) *OfficeActionDetail {
	o := &OfficeActionDetail{client: client, theme: theme, oa: oa, loading: true}
	o.handlers = map[command.ID]cmdHandler{
		command.NavDown: func(inv Invocation) tea.Cmd { o.moveRow(inv.Repeat); return nil },
		command.NavUp:   func(inv Invocation) tea.Cmd { o.moveRow(-inv.Repeat); return nil },
		command.Refresh: func(Invocation) tea.Cmd { o.loading = true; return o.load() },
	}
	return o
}

// WithLogger attaches a logger.
func (o *OfficeActionDetail) WithLogger(l *slog.Logger) *OfficeActionDetail { o.logger = l; return o }

func (o *OfficeActionDetail) Scope() command.Scope { return command.ScopeMatterOADetail }

func (o *OfficeActionDetail) Title() string {
	if name := strings.TrimSpace(o.oa.Name); name != "" {
		return "Office Action — " + name
	}
	t := "Office Action"
	if !o.oa.MailDate.IsZero() {
		t += " · " + o.oa.MailDate.Format(domain.DateLayout)
	}
	return t
}

func (o *OfficeActionDetail) OfficeAction() domain.OfficeAction { return o.oa }

func (o *OfficeActionDetail) Init() tea.Cmd { return o.load() }

func (o *OfficeActionDetail) load() tea.Cmd {
	client, project := o.client, o.oa.Project
	if client == nil {
		return nil
	}
	requestID := nextAsyncID()
	o.loadID = requestID
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		out := oaDetailLoadedMsg{requestID: requestID}
		var docs proto.MatterDocumentListResult
		if err := client.Call(ctx, proto.MethodMatterDocumentList, proto.MatterDocumentListParams{Project: project}, &docs); err != nil {
			out.err = err
			return out
		}
		out.documents = docs.Documents
		var comms proto.MatterEventListResult
		if err := client.Call(ctx, proto.MethodMatterEventList, proto.MatterEventListParams{Project: project}, &comms); err != nil {
			out.err = err
			return out
		}
		out.comms = len(comms.Events)
		var summary proto.TimeSummaryResult
		if err := client.Call(ctx, proto.MethodTimeSummary, proto.TimeListParams{Project: project}, &summary); err != nil {
			out.err = err
			return out
		}
		out.time = summary.Time
		out.ai = summary.AI

		var oaList proto.OfficeActionListResult
		if err := client.Call(ctx, proto.MethodOfficeActionList, proto.OfficeActionListParams{Project: project}, &oaList); err != nil {
			out.err = err
			return out
		}
		var found bool
		for _, item := range oaList.OfficeActions {
			if item.ID == o.oa.ID {
				out.oa = item
				found = true
				break
			}
		}
		if !found {
			out.oa = o.oa
		}
		var patents proto.OfficeActionPatentsResult
		if err := client.Call(ctx, proto.MethodOfficeActionPatents, proto.OfficeActionPatentsParams{ID: out.oa.ID}, &patents); err != nil {
			out.err = err
			return out
		}
		out.assigned = patents.Patents

		return out
	}
}

func (o *OfficeActionDetail) moveRow(delta int) {
	total := o.rowCount()
	if total == 0 {
		return
	}
	o.row = max(0, min(o.row+delta, total-1))
}

func (o *OfficeActionDetail) Command(id command.ID, inv Invocation) (Pane, tea.Cmd) {
	if handler, ok := o.handlers[id]; ok {
		return o, handler(inv)
	}
	return o, nil
}

func (o *OfficeActionDetail) Handles() []command.ID { return handlerIDs(o.handlers) }

func (o *OfficeActionDetail) Update(msg tea.Msg) (Pane, tea.Cmd) {
	if m, ok := msg.(oaDetailLoadedMsg); ok {
		if m.requestID != o.loadID {
			return o, nil
		}
		o.loading = false
		if m.err != nil {
			o.loadErr = m.err.Error()
			return o, nil
		}
		o.loadErr = ""
		o.comms, o.time, o.ai = m.comms, m.time, m.ai
		o.oa = m.oa
		o.documents = m.documents
		o.assigned = m.assigned
		o.moveRow(0)
	}
	return o, nil
}

func (o *OfficeActionDetail) Selection() (domain.PatentNumber, bool) {
	return domain.PatentNumber{}, false
}

// FullTextNumber returns this office action's application as a patent number, so
// :open.fulltext (T) opens the application's full text.
func (o *OfficeActionDetail) FullTextNumber() (domain.PatentNumber, bool) {
	return officeActionFullTextNumber(o.oa)
}

func (o *OfficeActionDetail) HandleKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool) {
	switch msg.String() {
	case "ctrl+n":
		oa := o.oa
		return o, func() tea.Msg { return OpenOfficeActionEditorMsg{OA: oa} }, true
	case "enter":
		return o, o.activateRow(), true
	case "e":
		oa := o.oa
		return o, func() tea.Msg { return OpenOfficeActionEditFormMsg{OA: oa} }, true
	case "p":
		oa := o.oa
		return o, func() tea.Msg { return OpenOfficeActionPatentsMsg{OA: oa} }, true
	}
	return o, nil, false
}

type oaDetailRowKind int

const (
	oaDetailMeta oaDetailRowKind = iota
	oaDetailPatent
	oaDetailDocument
	oaDetailComms
	oaDetailWorklog
	oaDetailUnreviewed
	oaDetailAI
)

type oaDetailRow struct {
	kind  oaDetailRowKind
	index int
}

func (o *OfficeActionDetail) rowCount() int {
	return 9 + len(o.assigned) + len(o.documents) + o.matterRowCount()
}

func (o *OfficeActionDetail) matterRowCount() int {
	n := 2 // Communications + Worklog
	if o.time.UnvalidatedCount > 0 {
		n++
	}
	if o.ai.Calls > 0 {
		n++
	}
	return n
}

func (o *OfficeActionDetail) rowAt(cursor int) oaDetailRow {
	if cursor < 9 {
		return oaDetailRow{kind: oaDetailMeta, index: cursor}
	}
	cursor -= 9
	if cursor < len(o.assigned) {
		return oaDetailRow{kind: oaDetailPatent, index: cursor}
	}
	cursor -= len(o.assigned)
	if cursor < len(o.documents) {
		return oaDetailRow{kind: oaDetailDocument, index: cursor}
	}
	cursor -= len(o.documents)
	if cursor == 0 {
		return oaDetailRow{kind: oaDetailComms}
	}
	cursor--
	if cursor == 0 {
		return oaDetailRow{kind: oaDetailWorklog}
	}
	cursor--
	if o.time.UnvalidatedCount > 0 {
		if cursor == 0 {
			return oaDetailRow{kind: oaDetailUnreviewed}
		}
		cursor--
	}
	return oaDetailRow{kind: oaDetailAI}
}

func (o *OfficeActionDetail) activateRow() tea.Cmd {
	switch o.rowAt(o.row).kind {
	case oaDetailMeta:
		oa := o.oa
		return func() tea.Msg { return OpenOfficeActionEditFormMsg{OA: oa} }
	case oaDetailPatent:
		oa := o.oa
		return func() tea.Msg { return OpenOfficeActionPatentsMsg{OA: oa} }
	case oaDetailDocument:
		oa := o.oa
		return func() tea.Msg { return OpenOfficeActionDocumentsMsg{OA: oa} }
	default:
		oa := o.oa
		return func() tea.Msg { return OpenOfficeActionEditorMsg{OA: oa} }
	}
}

func (o *OfficeActionDetail) View(w, _ int) string {
	var b strings.Builder
	row := 0
	selectable := func(line string, rowIdx int) {
		cell := render.Pad(render.Truncate(line, max(w, 8)), max(w, 8))
		if rowIdx == o.row {
			b.WriteString(o.theme.Selected.Render(cell))
		} else {
			b.WriteString(o.theme.Row.Render(cell))
		}
		b.WriteByte('\n')
	}
	field := func(label, value string) {
		line := render.Pad(label, 16) + render.Truncate(orDash(value), max(w-16, 8))
		selectable(line, row)
		row++
	}
	field("Name", o.oa.Name)
	field("Examiner", o.oa.Examiner)
	field("Application", o.oa.ApplicationNumber)
	field("Type", oaTypeLabel(o.oa.Type))
	field("Art unit", o.oa.ArtUnit)
	field("Mailed", dateOrDash(o.oa.MailDate))
	field("Response due", responseDueLabel(o.oa))
	field("Status", statusAgeLabel(o.oa))
	field("Tags", tagsText(o.oa.Tags))
	b.WriteByte('\n')
	b.WriteString(o.theme.Header.Render(render.Pad(fmt.Sprintf("ASSIGNED PATENTS (%d)", len(o.assigned)), max(w, 8))))
	b.WriteByte('\n')
	if len(o.assigned) == 0 {
		b.WriteString(o.theme.Dim.Render("none assigned"))
		b.WriteByte('\n')
	} else {
		for i := 0; i < len(o.assigned); i++ {
			selectable(assignedPatentSummary(o.assigned[i]), row)
			row++
		}
	}
	b.WriteByte('\n')
	b.WriteString(o.theme.Header.Render(render.Pad(fmt.Sprintf("DOCUMENTS (%d)", len(o.documents)), max(w, 8))))
	b.WriteByte('\n')
	if len(o.documents) == 0 {
		b.WriteString(o.theme.Dim.Render("none loaded"))
		b.WriteByte('\n')
	} else {
		for i := 0; i < len(o.documents); i++ {
			selectable(documentSummary(o.documents[i], o.oa.ID), row)
			row++
		}
	}
	b.WriteByte('\n')

	if o.loading {
		b.WriteString(o.theme.Dim.Render("loading matter details…"))
		b.WriteByte('\n')
	} else if o.loadErr != "" {
		b.WriteString(o.theme.Error.Render("error: " + o.loadErr))
		b.WriteByte('\n')
	} else {
		b.WriteString(o.theme.Header.Render(render.Pad("MATTER", max(w, 8))))
		b.WriteByte('\n')
		field("Communications", fmt.Sprintf("%d", o.comms))
		field("Worklog", formatTimeSummary(o.time))
		if o.time.UnvalidatedCount > 0 {
			field("Unreviewed", fmt.Sprintf("%d entries (%s) — press w to review before billing", o.time.UnvalidatedCount, fmtDuration(o.time.UnvalidatedSecs)))
		}
		if o.ai.Calls > 0 {
			field("AI usage", fmt.Sprintf("%d calls, %d tokens", o.ai.Calls, o.ai.TotalTokens))
		}
	}

	b.WriteByte('\n')
	b.WriteString(o.theme.Dim.Render(render.Truncate(
		"[j/k] move row  [enter] open/edit row  [ctrl+n] notes  [e] edit OA  [p] patents  [w] review worklog  [f] docs  [c] comms  [R] response  [esc] back", w)))
	return b.String()
}

func assignedPatentSummary(a proto.OfficeActionAssignedPatent) string {
	n := a.Number.DisplayString()
	if !a.Display.IsZero() {
		n = a.Display.DisplayString()
	}
	title := strings.TrimSpace(a.Title)
	if title == "" {
		title = "—"
	}
	return fmt.Sprintf("%s  %s  %s", render.Pad(render.Truncate(n, 16), 16), render.Pad(render.Truncate(a.Status.Label(), 12), 12), title)
}

func documentSummary(d domain.MatterDocument, oaID string) string {
	name := strings.TrimSpace(d.DisplayName)
	if name == "" {
		name = "(unnamed)"
	}
	linked := "matter"
	if documentLinkedToOA(d, oaID) {
		linked = "linked"
	}
	date := "—"
	if !d.AddedAt.IsZero() {
		date = d.AddedAt.Format(domain.DateLayout)
	}
	return fmt.Sprintf("%s  %s  %s  %s  %s",
		render.Pad(render.Truncate(name, 30), 30),
		render.Pad(render.Truncate(d.Kind.Label(), 14), 14),
		render.Pad(render.Truncate(d.Stage.Label(), 12), 12),
		render.Pad(linked, 6),
		date)
}

func documentLinkedToOA(d domain.MatterDocument, oaID string) bool {
	if oaID == "" {
		return false
	}
	if d.OfficeActionID == oaID {
		return true
	}
	for _, id := range d.OfficeActionIDs {
		if id == oaID {
			return true
		}
	}
	return false
}

// formatTimeSummary renders recorded time by activity, e.g. "reading 0:42 ·
// writing 1:15 · ai 0:08".
func formatTimeSummary(s domain.TimeSummary) string {
	if len(s.Seconds) == 0 {
		return "none yet"
	}
	order := []domain.TimeActivity{domain.TimeReading, domain.TimeWriting, domain.TimeAI, domain.TimeCall, domain.TimeAdmin}
	var parts []string
	for _, a := range order {
		if secs := s.Seconds[a]; secs > 0 {
			parts = append(parts, a.Label()+" "+fmtDuration(secs))
		}
	}
	if len(parts) == 0 {
		return "none yet"
	}
	return strings.Join(parts, " · ")
}

// fmtDuration renders seconds as h:mm (or m:ss under an hour).
func fmtDuration(secs int) string {
	d := time.Duration(secs) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d", h, m)
	}
	return fmt.Sprintf("%d:%02d", m, secs%60)
}
