package pane

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/tui/render"
)

// detailDateLayout formats dates in the detail view.
const detailDateLayout = "2006-01-02"

// detailLoadedMsg delivers a finished patent.get result.
type detailLoadedMsg struct {
	requestID uint64
	patent    domain.Patent
	err       error
}

// Detail shows one patent's full record. The record can be longer than the
// body area, so the pane scrolls — every navigation binding in the detail
// keymap layer must resolve to a handler here.
type Detail struct {
	client   *rpc.Client
	theme    render.Theme
	number   domain.PatentNumber
	handlers map[command.ID]cmdHandler

	patent  domain.Patent
	page    render.Paginator
	loading bool
	loadErr string
	loadID  uint64
}

// NewDetail builds a detail pane for one patent number.
func NewDetail(client *rpc.Client, theme render.Theme, number domain.PatentNumber) *Detail {
	d := &Detail{
		client:  client,
		theme:   theme,
		number:  number,
		page:    render.NewPaginator(10),
		loading: true,
	}
	d.handlers = map[command.ID]cmdHandler{
		command.NavDown:     func(r int) tea.Cmd { d.page.MoveDown(r); return nil },
		command.NavUp:       func(r int) tea.Cmd { d.page.MoveUp(r); return nil },
		command.NavPageDown: func(int) tea.Cmd { d.page.PageDown(); return nil },
		command.NavPageUp:   func(int) tea.Cmd { d.page.PageUp(); return nil },
		command.NavTop:      func(int) tea.Cmd { d.page.Top(); return nil },
		command.NavBottom:   func(int) tea.Cmd { d.page.Bottom(); return nil },
		command.Refresh:     func(int) tea.Cmd { d.loading = true; return d.load() },
		command.IngestFamily: func(int) tea.Cmd {
			return ingestFamilyCmd(d.client, d.number)
		},
	}
	return d
}

// Context implements Pane.
func (d *Detail) Context() command.Context { return command.ContextDetail }

// Title implements Pane.
func (d *Detail) Title() string { return "Detail · " + d.number.String() }

// Init implements Pane.
func (d *Detail) Init() tea.Cmd { return d.load() }

// load fetches the patent record from the daemon.
func (d *Detail) load() tea.Cmd {
	client, number := d.client, d.number
	requestID := nextAsyncID()
	d.loadID = requestID
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.PatentResult
		err := client.Call(ctx, proto.MethodPatentGet,
			proto.PatentGetParams{Number: number}, &res)
		return detailLoadedMsg{requestID: requestID, patent: res.Patent, err: err}
	}
}

// Command implements Pane.
func (d *Detail) Command(id command.ID, repeat int) (Pane, tea.Cmd) {
	if handler, ok := d.handlers[id]; ok {
		return d, handler(repeat)
	}
	return d, nil
}

// Handles implements Pane.
func (d *Detail) Handles() []command.ID { return handlerIDs(d.handlers) }

// Update implements Pane.
func (d *Detail) Update(msg tea.Msg) (Pane, tea.Cmd) {
	if m, ok := msg.(detailLoadedMsg); ok {
		if m.requestID != d.loadID {
			return d, nil
		}
		d.loading = false
		if m.err != nil {
			d.loadErr = m.err.Error()
			return d, nil
		}
		d.loadErr = ""
		d.patent = m.patent
		d.page.Top()
	}
	return d, nil
}

// Selection implements Pane: a detail pane's selection is its own patent.
func (d *Detail) Selection() (domain.PatentNumber, bool) {
	return d.number, true
}

// View implements Pane. Long records scroll: the body is built in full, then
// windowed to the visible height by the paginator.
func (d *Detail) View(w, h int) string {
	switch {
	case d.loading:
		return d.theme.Dim.Render("loading " + d.number.String() + "…")
	case d.loadErr != "":
		return d.theme.Error.Render("error: " + d.loadErr)
	}
	lines := strings.Split(d.body(w), "\n")
	d.page.SetTotal(len(lines))
	d.page.SetPageSize(max(h, 1))
	start, end := d.page.Window()
	return strings.Join(lines[start:end], "\n")
}

// body renders the full, unwindowed detail record.
func (d *Detail) body(w int) string {
	p := d.patent
	var b strings.Builder
	d.field(&b, w, "Shown as", numberToShow(p).String())
	d.field(&b, w, "Record key", p.Number.String())
	d.field(&b, w, "Title", p.Title)
	d.field(&b, w, "Assignee", p.Assignee)
	d.field(&b, w, "Inventors", strings.Join(p.Inventors, ", "))
	d.field(&b, w, "Fetch state", string(p.FetchState))
	d.field(&b, w, "Source", string(p.Source))
	d.field(&b, w, "Abstract", p.Abstract)

	// Every life-stage document — the application stays visible here even once
	// the patent has published.
	b.WriteByte('\n')
	b.WriteString(d.theme.Header.Render("Documents"))
	b.WriteByte('\n')
	if len(p.Documents) == 0 {
		b.WriteString(d.theme.Dim.Render("  (none)"))
	}
	for _, doc := range p.Documents {
		line := "  " + render.Pad(string(doc.Stage), 13) + " " +
			render.Pad(doc.Number.String(), 20) + " " + dateText(doc.Dated)
		b.WriteString(d.theme.Row.Render(render.Truncate(line, w)))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// numberToShow returns the record's display number, falling back to the
// record key when no documents set one.
func numberToShow(p domain.Patent) domain.PatentNumber {
	if !p.DisplayNumber.IsZero() {
		return p.DisplayNumber
	}
	return p.Number
}

// field writes one "Label: value" line.
func (d *Detail) field(b *strings.Builder, w int, label, value string) {
	const labelW = 14
	b.WriteString(d.theme.Header.Render(render.Pad(label, labelW)))
	b.WriteString(" ")
	b.WriteString(d.theme.Row.Render(render.Truncate(value, max(w-labelW-1, 0))))
	b.WriteByte('\n')
}

// dateText renders a date, or a dash when it is unset.
func dateText(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Format(detailDateLayout)
}
