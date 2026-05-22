package overlay

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/pane"
	"patentmine/internal/tui/render"
)

const patentNoteTimeLayout = "2006-01-02 15:04:05"
const patentNoteTimeBracket = "[" + patentNoteTimeLayout + "]"

type patentNoteLoadedMsg struct {
	patent domain.Patent
	note   *domain.PatentNote
	err    error
}

type patentNoteSavedMsg struct {
	note domain.PatentNote
	err  error
}

type patentNoteDeletedMsg struct{ err error }

// PatentNoteEditor edits one project-scoped markdown note for a patent.
type PatentNoteEditor struct {
	client  *rpc.Client
	theme   render.Theme
	project domain.ProjectID
	number  domain.PatentNumber

	patent       domain.Patent
	note         *domain.PatentNote
	value        string
	line         int
	column       int
	offset       int
	loading      bool
	loadErr      string
	msg          string
	dirty        bool
	confirmClear bool
}

func NewPatentNoteEditor(client *rpc.Client, theme render.Theme, project domain.ProjectID, number domain.PatentNumber) *PatentNoteEditor {
	return &PatentNoteEditor{client: client, theme: theme, project: project, number: number, loading: true}
}

func (o *PatentNoteEditor) Title() string { return "Patent Notes · " + o.number.String() }

func (o *PatentNoteEditor) Handles() []command.ID { return []command.ID{command.CloseOverlay} }

func (o *PatentNoteEditor) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if id == command.CloseOverlay {
		return o, func() tea.Msg { return CloseOverlayMsg{} }
	}
	return o, nil
}

func (o *PatentNoteEditor) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case patentNoteLoadedMsg:
		o.loading = false
		if m.err != nil {
			o.loadErr = m.err.Error()
			return o, nil
		}
		o.loadErr = ""
		o.patent = m.patent
		o.note = m.note
		if m.note != nil {
			o.value = m.note.Markdown
		}
		return o, nil
	case patentNoteSavedMsg:
		if m.err != nil {
			o.msg = "save failed: " + m.err.Error()
			return o, nil
		}
		note := m.note
		o.note = &note
		o.value = note.Markdown
		o.dirty = false
		o.confirmClear = false
		o.msg = "note saved"
		return o, func() tea.Msg {
			return pane.StatusMsg{Key: text.StatusFilter, Args: []any{"saved patent note for " + o.number.String()}}
		}
	case patentNoteDeletedMsg:
		if m.err != nil {
			o.msg = "delete failed: " + m.err.Error()
			return o, nil
		}
		o.note = nil
		o.value = ""
		o.line, o.column, o.offset = 0, 0, 0
		o.dirty = false
		o.confirmClear = false
		o.msg = "note cleared"
		return o, func() tea.Msg {
			return pane.StatusMsg{Key: text.StatusFilter, Args: []any{"cleared patent note for " + o.number.String()}}
		}
	}
	return o, nil
}

func (o *PatentNoteEditor) Init() tea.Cmd {
	return o.loadCmd()
}

func (o *PatentNoteEditor) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if o.loading {
		return o, nil, true
	}
	if o.confirmClear {
		switch strings.ToLower(msg.String()) {
		case "y":
			o.confirmClear = false
			return o, o.deleteCmd(), true
		case "n", "esc", "q":
			o.confirmClear = false
			return o, nil, true
		default:
			return o, nil, true
		}
	}
	switch msg.Type {
	case tea.KeyEsc:
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case tea.KeyCtrlS:
		return o, o.saveCmd(), true
	case tea.KeyEnter:
		o.insertText("\n")
		return o, nil, true
	case tea.KeyBackspace:
		o.backspace()
		return o, nil, true
	case tea.KeyDelete:
		o.deleteForward()
		return o, nil, true
	case tea.KeyLeft:
		o.moveLeft()
		return o, nil, true
	case tea.KeyRight:
		o.moveRight()
		return o, nil, true
	case tea.KeyUp:
		o.moveVertical(-1)
		return o, nil, true
	case tea.KeyDown:
		o.moveVertical(1)
		return o, nil, true
	case tea.KeyRunes, tea.KeySpace:
		if msg.String() == "D" {
			if strings.TrimSpace(o.value) != "" || o.note != nil {
				o.confirmClear = true
			}
			return o, nil, true
		}
		o.insertText(msg.String())
		return o, nil, true
	}
	return o, nil, true
}

func (o *PatentNoteEditor) View(maxW, maxH int) string {
	if o.loading {
		return o.theme.Dim.Render("loading patent note…")
	}
	if o.loadErr != "" {
		return o.theme.Error.Render("error: " + o.loadErr)
	}
	lines := o.lines()
	bodyRows := max(maxH-8, 3)
	o.keepCursorVisible(bodyRows)
	start := o.offset
	end := min(start+bodyRows, len(lines))
	var b strings.Builder
	b.WriteString(o.theme.Header.Render(render.Truncate(o.number.String()+" · "+o.patent.Title, maxW)))
	b.WriteString("\n")
	b.WriteString(o.theme.Dim.Render(render.Truncate(o.metaLine(), maxW)))
	b.WriteString("\n")
	if o.msg != "" {
		b.WriteString(o.theme.OK.Render(render.Truncate(o.msg, maxW)))
		b.WriteString("\n")
	}
	if o.confirmClear {
		b.WriteString(o.theme.Warn.Render(render.Truncate("clear this patent note? [y] yes  [n] no", maxW)))
		b.WriteString("\n")
	}
	for i := start; i < end; i++ {
		line := lines[i]
		prefix := fmt.Sprintf("%3d ", i+1)
		if i == o.line {
			line = o.renderCursorLine(line)
			b.WriteString(o.theme.Selected.Render(render.Pad(render.Truncate(prefix+line, maxW), maxW)))
		} else {
			b.WriteString(o.theme.Row.Render(render.Truncate(prefix+line, maxW)))
		}
		if i < end-1 {
			b.WriteByte('\n')
		}
	}
	if end == start {
		b.WriteString(o.theme.Selected.Render(render.Pad("  1 "+o.theme.Title.Render("█"), maxW)))
	}
	b.WriteString("\n")
	b.WriteString(o.theme.Dim.Render(render.Truncate("markdown editor · ctrl+s save · D clear · esc close", maxW)))
	return b.String()
}

func (o *PatentNoteEditor) loadCmd() tea.Cmd {
	client, project, number := o.client, o.project, o.number
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.PatentResult
		err := client.Call(ctx, proto.MethodPatentGet, proto.PatentGetParams{Number: number, Project: project}, &res)
		if err != nil {
			return patentNoteLoadedMsg{err: err}
		}
		var note *domain.PatentNote
		if res.PatentNote != nil {
			copied := *res.PatentNote
			note = &copied
		}
		return patentNoteLoadedMsg{patent: res.Patent, note: note}
	}
}

func (o *PatentNoteEditor) saveCmd() tea.Cmd {
	markdown := strings.TrimSpace(o.value)
	if markdown == "" {
		o.msg = "enter note text first or press D to clear"
		return nil
	}
	if o.note == nil {
		now := time.Now().Format(patentNoteTimeBracket)
		markdown = now + " " + markdown
	}
	note := domain.PatentNote{Project: o.project, Patent: o.number, Markdown: markdown}
	if o.note != nil {
		note.AddedAt = o.note.AddedAt
	}
	client := o.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.PatentNoteResult
		err := client.Call(ctx, proto.MethodPatentNoteSave, proto.PatentNoteSaveParams{Note: note}, &res)
		return patentNoteSavedMsg{note: res.Note, err: err}
	}
}

func (o *PatentNoteEditor) deleteCmd() tea.Cmd {
	client, project, number := o.client, o.project, o.number
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.Empty
		err := client.Call(ctx, proto.MethodPatentNoteDelete, proto.PatentNoteParams{Project: project, Patent: number}, &res)
		return patentNoteDeletedMsg{err: err}
	}
}

func (o *PatentNoteEditor) lines() []string {
	lines := strings.Split(o.value, "\n")
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func (o *PatentNoteEditor) metaLine() string {
	parts := []string{"project " + string(o.project)}
	if o.note != nil {
		if !o.note.AddedAt.IsZero() {
			parts = append(parts, "added "+o.note.AddedAt.Format(patentNoteTimeLayout))
		}
		if !o.note.UpdatedAt.IsZero() {
			parts = append(parts, "updated "+o.note.UpdatedAt.Format(patentNoteTimeLayout))
		}
	}
	if o.dirty {
		parts = append(parts, "modified")
	}
	return strings.Join(parts, "  ·  ")
}

func (o *PatentNoteEditor) renderCursorLine(line string) string {
	runes := []rune(line)
	col := min(o.column, len(runes))
	return string(runes[:col]) + o.theme.Title.Render("█") + string(runes[col:])
}

func (o *PatentNoteEditor) keepCursorVisible(bodyRows int) {
	if o.line < o.offset {
		o.offset = o.line
	}
	if o.line >= o.offset+bodyRows {
		o.offset = o.line - bodyRows + 1
	}
	if o.offset < 0 {
		o.offset = 0
	}
}

func (o *PatentNoteEditor) insertText(s string) {
	lines := o.lines()
	runes := []rune(lines[o.line])
	col := min(o.column, len(runes))
	parts := strings.Split(s, "\n")
	if len(parts) == 1 {
		lines[o.line] = string(runes[:col]) + s + string(runes[col:])
		o.value = strings.Join(lines, "\n")
		o.column += len([]rune(s))
		o.dirty = true
		return
	}
	inserted := make([]string, 0, len(parts))
	inserted = append(inserted, string(runes[:col])+parts[0])
	if len(parts) > 2 {
		inserted = append(inserted, parts[1:len(parts)-1]...)
	}
	inserted = append(inserted, parts[len(parts)-1]+string(runes[col:]))
	lines = append(lines[:o.line], append(inserted, lines[o.line+1:]...)...)
	o.value = strings.Join(lines, "\n")
	o.line += len(inserted) - 1
	o.column = len([]rune(parts[len(parts)-1]))
	o.dirty = true
}

func (o *PatentNoteEditor) backspace() {
	lines := o.lines()
	if o.line == 0 && o.column == 0 {
		return
	}
	if o.column > 0 {
		runes := []rune(lines[o.line])
		col := min(o.column, len(runes))
		lines[o.line] = string(append(runes[:col-1], runes[col:]...))
		o.column--
		o.value = strings.Join(lines, "\n")
		o.dirty = true
		return
	}
	prev := []rune(lines[o.line-1])
	lines[o.line-1] += lines[o.line]
	lines = append(lines[:o.line], lines[o.line+1:]...)
	o.line--
	o.column = len(prev)
	o.value = strings.Join(lines, "\n")
	o.dirty = true
}

func (o *PatentNoteEditor) deleteForward() {
	lines := o.lines()
	runes := []rune(lines[o.line])
	col := min(o.column, len(runes))
	if col < len(runes) {
		lines[o.line] = string(append(runes[:col], runes[col+1:]...))
		o.value = strings.Join(lines, "\n")
		o.dirty = true
		return
	}
	if o.line+1 >= len(lines) {
		return
	}
	lines[o.line] += lines[o.line+1]
	lines = append(lines[:o.line+1], lines[o.line+2:]...)
	o.value = strings.Join(lines, "\n")
	o.dirty = true
}

func (o *PatentNoteEditor) moveLeft() {
	if o.column > 0 {
		o.column--
		return
	}
	if o.line == 0 {
		return
	}
	o.line--
	o.column = len([]rune(o.lines()[o.line]))
}

func (o *PatentNoteEditor) moveRight() {
	lines := o.lines()
	if o.column < len([]rune(lines[o.line])) {
		o.column++
		return
	}
	if o.line+1 >= len(lines) {
		return
	}
	o.line++
	o.column = 0
}

func (o *PatentNoteEditor) moveVertical(delta int) {
	lines := o.lines()
	o.line = max(0, min(o.line+delta, len(lines)-1))
	o.column = min(o.column, len([]rune(lines[o.line])))
}
