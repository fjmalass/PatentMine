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

type oaDetailLoadedMsg struct {
	requestID uint64
	oa        domain.OfficeAction
	docs      int
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

	loading bool
	loadErr string
	loadID  uint64
	docs    int
	comms   int
	time    domain.TimeSummary
	ai      domain.AIUsageSummary
}

// NewOfficeActionDetail builds the drill-down pane for one office action.
func NewOfficeActionDetail(client *rpc.Client, theme render.Theme, oa domain.OfficeAction) *OfficeActionDetail {
	o := &OfficeActionDetail{client: client, theme: theme, oa: oa, loading: true}
	o.handlers = map[command.ID]cmdHandler{
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
		out.docs = len(docs.Documents)
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

		return out
	}
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
		o.docs, o.comms, o.time, o.ai = m.docs, m.comms, m.time, m.ai
		o.oa = m.oa
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
	case "enter", "n":
		oa := o.oa
		return o, func() tea.Msg { return OpenOfficeActionEditorMsg{OA: oa} }, true
	case "e":
		oa := o.oa
		return o, func() tea.Msg { return OpenOfficeActionEditFormMsg{OA: oa} }, true
	}
	return o, nil, false
}

func (o *OfficeActionDetail) View(w, h int) string {
	var b strings.Builder
	field := func(label, value string) {
		b.WriteString(o.theme.Dim.Render(render.Pad(label, 16)))
		b.WriteString(o.theme.Row.Render(render.Truncate(orDash(value), max(w-16, 8))))
		b.WriteByte('\n')
	}
	field("Name", o.oa.Name)
	field("Examiner", o.oa.Examiner)
	field("Application", o.oa.ApplicationNumber)
	field("Type", oaTypeLabel(o.oa.Type))
	field("Art unit", o.oa.ArtUnit)
	field("Mailed", dateOrDash(o.oa.MailDate))
	field("Response due", responseDueLabel(o.oa))
	field("Status", statusAgeLabel(o.oa))
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
		field("Documents", fmt.Sprintf("%d", o.docs))
		field("Communications", fmt.Sprintf("%d", o.comms))
		field("Worklog", formatTimeSummary(o.time))
		if o.time.UnvalidatedCount > 0 {
			field("Unreviewed", fmt.Sprintf("%d entries (%s) — review before billing", o.time.UnvalidatedCount, fmtDuration(o.time.UnvalidatedSecs)))
		}
		if o.ai.Calls > 0 {
			field("AI usage", fmt.Sprintf("%d calls, %d tokens", o.ai.Calls, o.ai.TotalTokens))
		}
	}

	b.WriteByte('\n')
	b.WriteString(o.theme.Dim.Render(render.Truncate(
		"[enter] notes editor  [e] edit  [f] documents  [c] communications  [R] draft response  [esc] back", w)))
	return b.String()
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
