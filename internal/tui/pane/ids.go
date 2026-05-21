package pane

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

type idsField string

const (
	idsFieldStatus   idsField = "status"
	idsFieldKind     idsField = "kind"
	idsFieldCountry  idsField = "country"
	idsFieldInFull   idsField = "in_full"
	idsFieldPassages idsField = "passages"
	idsFieldNotes    idsField = "notes"
)

type idsLoadedMsg struct {
	requestID uint64
	patent    domain.Patent
	entry     *domain.IDSEntry
	err       error
}

type idsSavedMsg struct {
	entry domain.IDSEntry
	err   error
}

type idsDeletedMsg struct{ err error }

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
		command.NavTop:        func(Invocation) tea.Cmd { p.page.Top(); return nil },
		command.NavBottom:     func(Invocation) tea.Cmd { p.page.Bottom(); return nil },
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
	case idsSavedMsg:
		if m.err != nil {
			return p, status(text.StatusExportFailed, true, m.err.Error())
		}
		p.entry = m.entry
		return p, status(text.StatusFilter, false, "IDS updated")
	case idsDeletedMsg:
		if m.err != nil {
			return p, status(text.StatusExportFailed, true, m.err.Error())
		}
		p.entry = domain.IDSEntry{Project: p.project, Patent: p.patent.Number, Status: domain.IDSEntryPending}
		return p, status(text.StatusFilter, false, "IDS entry removed")
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
	case p.loading:
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
		{"Status", string(p.entry.Status)},
		{"Kind code", orDash(p.entry.KindCode)},
		{"Country code", orDash(p.entry.CountryCode)},
		{"In full", yesNo(p.entry.InFull)},
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
	case idsFieldKind, idsFieldCountry, idsFieldPassages, idsFieldNotes:
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
		return idsDeletedMsg{err: err}
	}
}

func (p *IDSDetail) saveCmd() tea.Cmd {
	entry := p.entry
	client := p.client
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.IDSEntryResult
		err := client.Call(ctx, proto.MethodIDSEntrySave,
			proto.IDSEntrySaveParams{Entry: entry}, &res)
		return idsSavedMsg{entry: res.Entry, err: err}
	}
}

func (p *IDSDetail) ApplyTextValue(field, value string) tea.Cmd {
	switch idsField(field) {
	case idsFieldKind:
		p.entry.KindCode = strings.TrimSpace(value)
	case idsFieldCountry:
		p.entry.CountryCode = strings.ToUpper(strings.TrimSpace(value))
	case idsFieldPassages:
		p.entry.RelevantPassages = strings.TrimSpace(value)
		if p.entry.RelevantPassages != "" {
			p.entry.InFull = false
		}
	case idsFieldNotes:
		p.entry.Notes = strings.TrimSpace(value)
	default:
		return nil
	}
	return p.saveCmd()
}

func (p *IDSDetail) currentField() idsField {
	fields := []idsField{idsFieldStatus, idsFieldKind, idsFieldCountry, idsFieldInFull, idsFieldPassages, idsFieldNotes}
	idx := max(p.page.Cursor()-2, 0)
	if idx >= len(fields) {
		idx = len(fields) - 1
	}
	return fields[idx]
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
