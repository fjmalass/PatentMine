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

const patentNoteTimeLayout = domain.DateTimeLayout
const patentNoteTimeBracket = "[" + patentNoteTimeLayout + "]"

const (
	patentNoteOverlayWidthPct  = 60
	patentNoteOverlayHeightPct = 60
	patentNoteMinWidth         = 76
	patentNoteMinHeight        = 22
)

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

// PatentNoteEditor edits one project-scoped markdown note for a patent. The text
// editing lives in the shared vimBuffer; this overlay adds the load/save/delete
// wiring, the clear-confirm flow, and the note metadata header.
type PatentNoteEditor struct {
	client  *rpc.Client
	theme   render.Theme
	project domain.ProjectID
	number  domain.PatentNumber

	patent       domain.Patent
	note         *domain.PatentNote
	openedAt     time.Time
	buf          *vimBuffer
	loading      bool
	loadErr      string
	msg          string
	modifiedAt   time.Time
	confirmClear bool
}

func NewPatentNoteEditor(client *rpc.Client, theme render.Theme, project domain.ProjectID, number domain.PatentNumber) *PatentNoteEditor {
	return &PatentNoteEditor{
		client:   client,
		theme:    theme,
		project:  project,
		number:   number,
		buf:      newVimBuffer(""),
		loading:  true,
		openedAt: time.Now(),
	}
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
			o.buf.SetValue(m.note.Markdown)
		}
		o.buf.dirty = false
		return o, nil
	case patentNoteSavedMsg:
		if m.err != nil {
			o.msg = "save failed: " + m.err.Error()
			return o, nil
		}
		note := m.note
		o.note = &note
		o.buf.SetValue(note.Markdown)
		o.buf.dirty = false
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
		o.buf.clear()
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
	// In plain (non-vim) mode, D clears the note (after confirm) rather than
	// inserting a "D"; in vim mode D is the buffer's delete-to-end-of-line.
	if !o.buf.vimMode && msg.Type == tea.KeyRunes && msg.String() == "D" {
		if strings.TrimSpace(o.buf.Value()) != "" || o.note != nil {
			o.confirmClear = true
		}
		return o, nil, true
	}

	wasDirty := o.buf.dirty
	_, intent := o.buf.handleKey(msg)
	if o.buf.dirty && !wasDirty {
		o.modifiedAt = time.Now()
	}
	switch intent {
	case intentSave:
		return o, o.saveCmd(), true
	case intentClose:
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	return o, nil, true
}

// OverlaySize implements tui.DynamicSize, requesting at least 60% of the
// terminal so there is room to read and edit longer notes.
func (o *PatentNoteEditor) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, patentNoteOverlayWidthPct, patentNoteOverlayHeightPct, patentNoteMinWidth, patentNoteMinHeight)
}

func (o *PatentNoteEditor) View(maxW, maxH int) string {
	if o.loading {
		return o.theme.Dim.Render("loading patent note…")
	}
	if o.loadErr != "" {
		return o.theme.Error.Render("error: " + o.loadErr)
	}
	bodyRows := max(maxH-8, 3)
	rows := o.buf.view(maxW, bodyRows, o.theme.Title.Render("█"))

	var b strings.Builder
	b.WriteString(o.theme.Header.Render(render.Truncate(o.number.String()+" · "+o.patent.Title, maxW)))
	b.WriteString("\n")
	b.WriteString(o.theme.Dim.Render(render.Truncate(o.infoLine(), maxW)))
	b.WriteString("\n")
	if o.msg != "" {
		b.WriteString(o.theme.OK.Render(render.Truncate(o.msg, maxW)))
		b.WriteString("\n")
	}
	if o.confirmClear {
		b.WriteString(o.theme.Warn.Render(render.Truncate("clear this patent note? [y] yes  [n] no", maxW)))
		b.WriteString("\n")
	}
	for i, row := range rows {
		if row.selected {
			b.WriteString(o.theme.Selected.Render(render.Pad(render.Truncate(row.text, maxW), maxW)))
		} else {
			b.WriteString(o.theme.Row.Render(render.Truncate(row.text, maxW)))
		}
		if i < len(rows)-1 {
			b.WriteByte('\n')
		}
	}
	if len(rows) == 0 {
		b.WriteString(o.theme.Selected.Render(render.Pad("  1 "+o.theme.Title.Render("█"), maxW)))
	}
	b.WriteString("\n")
	if mode := o.buf.modeLabel(); mode != "" {
		b.WriteString(o.theme.Dim.Render(render.Truncate(
			fmt.Sprintf("-- %s -- · ctrl+] off · ctrl+s save · esc close", mode), maxW)))
	} else {
		b.WriteString(o.theme.Dim.Render(render.Truncate("markdown editor · ctrl+s save · D clear · esc close · ctrl+] vim", maxW)))
	}
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
	markdown := strings.TrimSpace(o.buf.Value())
	if markdown == "" {
		o.msg = "enter note text first or press D to clear"
		return nil
	}
	if o.note == nil {
		markdown = o.openedAt.Format(patentNoteTimeBracket) + " " + markdown
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

func (o *PatentNoteEditor) infoLine() string {
	parts := []string{"project " + string(o.project)}
	if o.note != nil {
		if !o.note.AddedAt.IsZero() {
			parts = append(parts, "added "+o.note.AddedAt.Format(patentNoteTimeLayout))
		}
		if !o.note.UpdatedAt.IsZero() {
			parts = append(parts, "updated "+o.note.UpdatedAt.Format(patentNoteTimeLayout))
		}
	} else {
		parts = append(parts, "new note · "+o.openedAt.Format(patentNoteTimeLayout))
	}
	if o.buf.dirty {
		parts = append(parts, "modified "+o.modifiedAt.Format(patentNoteTimeLayout))
	}
	return strings.Join(parts, "  ·  ")
}
