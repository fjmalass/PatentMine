package pane

import (
	"log/slog"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

type idsField string

const (
	idsFieldStatus    idsField = "status"
	idsFieldKind      idsField = "kind"
	idsFieldInFull    idsField = "in_full"
	idsFieldAdded     idsField = "added_at"
	idsFieldSubmitted idsField = "submitted_at"
	idsFieldPassages  idsField = "passages"
	idsFieldNotes     idsField = "notes"
)

type idsLoadedMsg struct {
	requestID uint64
	patent    domain.Patent
	entry     *domain.IDSEntry
	err       error
}

// EditIDSFieldMsg asks the app to open a text input for the selected IDS field.
type EditIDSFieldMsg struct{ Field string }

// IDSDetail edits one patent's curated IDS entry inside the active project.
type IDSDetail struct {
	client   *rpc.Client
	theme    render.Theme
	number   domain.PatentNumber
	project  domain.ProjectID
	handlers map[command.ID]cmdHandler

	patent  domain.Patent
	entry   domain.IDSEntry
	page    render.Paginator
	loading bool
	loadErr string
	loadID  uint64
	logger  *slog.Logger
	metrics *observability.Metrics
}

// WithLogger attaches a logger so the pane can persist RPC errors.
func (p *IDSDetail) WithLogger(l *slog.Logger) *IDSDetail { p.logger = l; return p }

// WithMetrics attaches the runtime metrics sink. Optional: nil is safe.
func (p *IDSDetail) WithMetrics(m *observability.Metrics) *IDSDetail { p.metrics = m; return p }

func (p *IDSDetail) incCounter(name string, delta int64) {
	if p.metrics != nil {
		p.metrics.IncCounter(name, delta)
	}
}

func (p *IDSDetail) log() *slog.Logger {
	if p.logger != nil {
		return p.logger
	}
	return slog.Default()
}

func NewIDSDetail(client *rpc.Client, theme render.Theme, number domain.PatentNumber, project domain.ProjectID) *IDSDetail {
	p := &IDSDetail{
		client:  client,
		theme:   theme,
		number:  number,
		project: project,
		page:    render.NewPaginator(10),
		loading: true,
	}
	p.handlers = map[command.ID]cmdHandler{
		command.NavDown:       func(inv Invocation) tea.Cmd { p.page.MoveDown(inv.Repeat); return nil },
		command.NavUp:         func(inv Invocation) tea.Cmd { p.page.MoveUp(inv.Repeat); return nil },
		command.NavPageDown:   func(Invocation) tea.Cmd { p.page.PageDown(); return nil },
		command.NavPageUp:     func(Invocation) tea.Cmd { p.page.PageUp(); return nil },
		command.NavTop:        func(inv Invocation) tea.Cmd { p.page.NavTop(inv.Repeat); return nil },
		command.NavBottom:     func(inv Invocation) tea.Cmd { p.page.NavBottom(inv.Repeat); return nil },
		command.Refresh:       func(Invocation) tea.Cmd { p.loading = true; return p.load() },
		command.IDSEditField:  func(Invocation) tea.Cmd { return p.editFieldCmd() },
		command.IDSToggleFull: func(Invocation) tea.Cmd { return p.toggleFullCmd() },
		command.IDSCycleStatus: func(Invocation) tea.Cmd {
			p.entry.Status = p.entry.Status.Next()
			return p.saveCmd()
		},
		command.IDSDelete: func(Invocation) tea.Cmd { return p.deleteCmd() },
	}
	return p
}

func (p *IDSDetail) Scope() command.Scope { return command.ScopeIDS }

func (p *IDSDetail) Title() string { return "IDS · " + p.number.String() }

func (p *IDSDetail) Init() tea.Cmd { return p.load() }

func (p *IDSDetail) Command(id command.ID, inv Invocation) (Pane, tea.Cmd) {
	if handler, ok := p.handlers[id]; ok {
		return p, handler(inv)
	}
	return p, nil
}

func (p *IDSDetail) Handles() []command.ID { return handlerIDs(p.handlers) }

func (p *IDSDetail) Selection() (domain.PatentNumber, bool) { return p.number, true }

func (p *IDSDetail) CurrentTextValue(field string) string {
	switch idsField(field) {
	case idsFieldKind:
		return p.entry.KindCode
	case idsFieldPassages:
		return p.entry.RelevantPassages
	case idsFieldNotes:
		return p.entry.Notes
	case idsFieldAdded:
		return formatDateInput(p.entry.AddedAt)
	case idsFieldSubmitted:
		return formatDateInput(p.entry.SubmittedAt)
	default:
		return ""
	}
}

func (p *IDSDetail) load() tea.Cmd {
	client, number, project := p.client, p.number, p.project
	requestID := nextAsyncID()
	p.loadID = requestID
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.PatentResult
		err := client.Call(ctx, proto.MethodPatentGet,
			proto.PatentGetParams{Number: number, Project: project}, &res)
		var entry *domain.IDSEntry
		if res.IDSEntry != nil {
			copied := *res.IDSEntry
			entry = &copied
		}
		return idsLoadedMsg{requestID: requestID, patent: res.Patent, entry: entry, err: err}
	}
}

func (p *IDSDetail) Update(msg tea.Msg) (Pane, tea.Cmd) {
	switch m := msg.(type) {
	case idsLoadedMsg:
		if m.requestID != p.loadID {
			return p, nil
		}
		p.loading = false
		if m.err != nil {
			p.loadErr = m.err.Error()
			p.log().Error("IDS load failed", slog.String("number", p.number.String()), slog.String("error", m.err.Error()))
			return p, nil
		}
		p.loadErr = ""
		p.patent = m.patent
		if m.entry != nil {
			p.entry = *m.entry
		} else {
			p.entry = domain.IDSEntry{Project: p.project, Patent: m.patent.Number, Status: domain.IDSEntryPending}
		}
		p.page.Top()
	case IDSEntryChangedMsg:
		if p.project == m.Project && p.patent.Number == m.Patent {
			prior := p.entry.Status
			if m.Entry != nil {
				p.entry = *m.Entry
			} else {
				p.entry = domain.IDSEntry{Project: p.project, Patent: p.patent.Number, Status: domain.IDSEntryPending}
			}
			p.loading = false
			p.incCounter("tui.ids_detail.mutation.applied", 1)
			if prior != p.entry.Status {
				p.incCounter("tui.ids_detail.status_transition", 1)
			}
			p.log().Info("tui.ids_detail.mutation",
				slog.String("project_id", string(p.project)),
				slog.String("patent", p.patent.Number.String()),
				slog.String("prior_status", string(prior)),
				slog.String("status", string(p.entry.Status)),
				slog.Bool("deleted", m.Entry == nil),
				slog.String("source", "single"))
		}
	case IDSEntriesChangedMsg:
		for i := range m.Entries {
			entry := m.Entries[i]
			if entry.Project == p.project && entry.Patent == p.patent.Number {
				prior := p.entry.Status
				p.entry = entry
				p.loading = false
				p.incCounter("tui.ids_detail.mutation.applied", 1)
				if prior != p.entry.Status {
					p.incCounter("tui.ids_detail.status_transition", 1)
				}
				p.log().Info("tui.ids_detail.mutation",
					slog.String("project_id", string(p.project)),
					slog.String("patent", p.patent.Number.String()),
					slog.String("prior_status", string(prior)),
					slog.String("status", string(p.entry.Status)),
					slog.Int("batch_size", len(m.Entries)),
					slog.String("source", "bulk"))
				break
			}
		}
	case ProjectChangedMsg:
		var project domain.ProjectID
		if m.Project != nil {
			project = m.Project.ID
		}
		if project != p.project {
			p.project = project
			p.loading = true
			return p, p.load()
		}
	}
	return p, nil
}

func (p *IDSDetail) View(w, h int) string {
	switch {
	case p.loading && p.patent.Number.Serial == "":
		return p.theme.Dim.Render("loading IDS…")
	case p.loadErr != "":
		return p.theme.Error.Render("error: " + p.loadErr)
	case p.project == "":
		return p.theme.Error.Render("error: select an active project first")
	}
	lines := strings.Split(p.body(w), "\n")
	p.page.SetTotal(len(lines))
	p.page.SetPageSize(max(h, 1))
	start, end := p.page.Window()
	return strings.Join(lines[start:end], "\n")
}

func (p *IDSDetail) body(w int) string {
	fields := [][2]string{
		{"Patent", numberToShow(p.patent).String()},
		{"Title", p.patent.Title},
		{"Country", orDash(p.patent.Number.Country)},
		{"Status", idsStatusDisplayText(p.theme, p.entry.Status)},
		{"Kind code", orDash(p.entry.KindCode)},
		{"In full", p.inFullGlyph(p.entry.InFull)},
		{"Added", formatDate(p.entry.AddedAt)},
		{"Submitted", formatDate(p.entry.SubmittedAt)},
		{"Relevant passages", orDash(p.entry.RelevantPassages)},
		{"Notes", orDash(p.entry.Notes)},
	}
	var b strings.Builder
	for i, field := range fields {
		line := render.Pad(field[0]+":", 18) + " " + field[1]
		styled := p.theme.Row.Render(render.Truncate(line, w))
		if i == p.page.Cursor() {
			styled = p.theme.Selected.Render(render.Pad(render.Truncate(line, w), w))
		}
		b.WriteString(styled)
		if i < len(fields)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (p *IDSDetail) editFieldCmd() tea.Cmd {
	field := p.currentField()
	switch field {
	case idsFieldKind, idsFieldPassages, idsFieldNotes, idsFieldAdded, idsFieldSubmitted:
		return func() tea.Msg { return EditIDSFieldMsg{Field: string(field)} }
	default:
		return nil
	}
}

func (p *IDSDetail) toggleFullCmd() tea.Cmd {
	p.entry.InFull = !p.entry.InFull
	if p.entry.InFull {
		p.entry.RelevantPassages = ""
	}
	return p.saveCmd()
}

func (p *IDSDetail) deleteCmd() tea.Cmd {
	client, project, patent := p.client, p.project, p.entry.Patent
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.Empty
		err := client.Call(ctx, proto.MethodIDSEntryDelete,
			proto.IDSEntryParams{Project: project, Patent: patent}, &res)
		return IDSEntryDeletedMsg{Project: project, Patent: patent, Err: err}
	}
}

func (p *IDSDetail) saveCmd() tea.Cmd {
	entry := p.entry
	client := p.client
	project := p.project
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.IDSEntryResult
		err := client.Call(ctx, proto.MethodIDSEntrySave,
			proto.IDSEntrySaveParams{Entry: entry}, &res)
		return IDSEntrySavedMsg{Project: project, Entry: res.Entry, Err: err}
	}
}

func (p *IDSDetail) ApplyTextValue(field, value string) tea.Cmd {
	switch idsField(field) {
	case idsFieldKind:
		p.entry.KindCode = strings.TrimSpace(value)
	case idsFieldPassages:
		p.entry.RelevantPassages = strings.TrimSpace(value)
		if p.entry.RelevantPassages != "" {
			p.entry.InFull = false
		}
	case idsFieldNotes:
		p.entry.Notes = strings.TrimSpace(value)
	case idsFieldAdded:
		t, err := parseDateInput(value)
		if err != nil {
			return status(text.StatusExportFailed, true, "added date: "+err.Error())
		}
		p.entry.AddedAt = t
	case idsFieldSubmitted:
		t, err := parseDateInput(value)
		if err != nil {
			return status(text.StatusExportFailed, true, "submitted date: "+err.Error())
		}
		p.entry.SubmittedAt = t
	default:
		return nil
	}
	return p.saveCmd()
}

func (p *IDSDetail) currentField() idsField {
	// Cursor index → field map. Read-only rows (Patent/Title/Country) map to
	// the empty field so edit/toggle commands are no-ops there.
	fields := []idsField{
		"", "", "",
		idsFieldStatus, idsFieldKind, idsFieldInFull,
		idsFieldAdded, idsFieldSubmitted,
		idsFieldPassages, idsFieldNotes,
	}
	idx := p.page.Cursor()
	if idx < 0 || idx >= len(fields) {
		return ""
	}
	return fields[idx]
}

// parseDateInput accepts YYYY-MM-DD; empty returns zero time. Anything else
// errors so the user sees feedback in the status line.
func parseDateInput(raw string) (time.Time, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return time.Time{}, nil
	}
	return time.Parse("2006-01-02", v)
}

// formatDateInput renders a time as YYYY-MM-DD for the edit overlay; empty for
// the zero value so the input opens blank rather than showing "0001-01-01".
func formatDateInput(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

// formatDate renders a date as YYYY-MM-DD or "-" when zero.
func formatDate(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02")
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// inFullGlyph renders the IDS in-full flag as a full-moon glyph when set and
// a half-moon (partial) glyph when not. Glyphs live on the theme so a
// terminal-specific build can override them without touching pane code.
func (p *IDSDetail) inFullGlyph(v bool) string {
	if v {
		return p.theme.Glyphs.IDSInFullFull
	}
	return p.theme.Glyphs.IDSInFullPartial
}

func (p *IDSDetail) PatentNumber() domain.PatentNumber { return p.number }
