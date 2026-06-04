package overlay

import (
	"context"
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

const (
	oaEditorWidthPct  = 86
	oaEditorHeightPct = 80
	oaEditorMinWidth  = 80
	oaEditorMinHeight = 22
)

type officeActionSavedMsg struct {
	oa  domain.OfficeAction
	err error
}

// OfficeActionEditor is the split view over one office action: the examiner's
// extracted text on the left (a read-only, vim-navigable reference) and the
// attorney's notes on the right (an editable vim buffer opening in NORMAL).
// ctrl-w h/l/w moves focus between the two panes, exactly like nvim windows.
type OfficeActionEditor struct {
	client *rpc.Client
	theme  render.Theme
	oa     domain.OfficeAction

	examiner *vimBuffer // read-only reference
	notes    *vimBuffer // editable

	focusNotes    bool // which pane has focus
	pendingWindow bool // saw ctrl-w, awaiting h/l/w
	msg           string
}

func NewOfficeActionEditor(client *rpc.Client, theme render.Theme, oa domain.OfficeAction) *OfficeActionEditor {
	// One shared yank register so a passage yanked (yy) in the read-only examiner
	// pane can be pasted (p) into the editable notes pane.
	reg := &yankRegister{}

	examiner := newVimBuffer(oa.ExtractedText)
	examiner.vimMode = true
	examiner.readOnly = true
	examiner.yank = reg

	notes := newVimBuffer(oa.Notes)
	notes.vimMode = true // open in NORMAL — j/k navigate, i to edit
	notes.yank = reg

	return &OfficeActionEditor{
		client:     client,
		theme:      theme,
		oa:         oa,
		examiner:   examiner,
		notes:      notes,
		focusNotes: true,
	}
}

func (o *OfficeActionEditor) Title() string {
	t := "Office Action"
	if !o.oa.MailDate.IsZero() {
		t += " · " + o.oa.MailDate.Format(domain.DateLayout)
	}
	if o.oa.Examiner != "" {
		t += " · " + o.oa.Examiner
	}
	return t
}

func (o *OfficeActionEditor) Handles() []command.ID { return nil }

func (o *OfficeActionEditor) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *OfficeActionEditor) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, oaEditorWidthPct, oaEditorHeightPct, oaEditorMinWidth, oaEditorMinHeight)
}

func (o *OfficeActionEditor) focused() *vimBuffer {
	if o.focusNotes {
		return o.notes
	}
	return o.examiner
}

func (o *OfficeActionEditor) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if m, ok := msg.(officeActionSavedMsg); ok {
		if m.err != nil {
			o.msg = "save failed: " + m.err.Error()
			return o, nil
		}
		o.oa = m.oa
		o.notes.SetValue(m.oa.Notes)
		o.notes.dirty = false
		o.msg = "notes saved"
		return o, func() tea.Msg {
			return pane.StatusMsg{Key: text.StatusFilter, Args: []any{"saved office-action notes"}}
		}
	}
	return o, nil
}

func (o *OfficeActionEditor) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	// ctrl-w window prefix: next h/l/w moves focus. Gated to non-insert so it
	// never shadows in-buffer typing.
	if o.pendingWindow {
		o.pendingWindow = false
		switch msg.String() {
		case "h":
			o.focusNotes = false
		case "l":
			o.focusNotes = true
		case "w":
			o.focusNotes = !o.focusNotes
		}
		return o, nil, true
	}
	if msg.Type == tea.KeyCtrlW && !(o.focusNotes && o.notes.vimInsert) {
		o.pendingWindow = true
		return o, nil, true
	}

	// Esc exits insert mode if we're in it; otherwise it closes the editor.
	if msg.Type == tea.KeyEsc {
		if o.focusNotes && o.notes.vimInsert {
			o.notes.handleKey(msg)
			return o, nil, true
		}
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	}

	switch _, intent := o.focused().handleKey(msg); intent {
	case intentSave:
		return o, o.saveCmd(), true
	case intentClose:
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	}
	return o, nil, true
}

func (o *OfficeActionEditor) saveCmd() tea.Cmd {
	client, id, notes := o.client, o.oa.ID, o.notes.Value()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.OfficeActionResult
		err := client.Call(ctx, proto.MethodOfficeActionSaveNotes,
			proto.OfficeActionSaveNotesParams{ID: id, Notes: notes}, &res)
		return officeActionSavedMsg{oa: res.OfficeAction, err: err}
	}
}

func (o *OfficeActionEditor) View(maxW, maxH int) string {
	const sep = " │ "
	leftW := max((maxW-len(sep))/2, 8)
	rightW := max(maxW-len(sep)-leftW, 8)
	bodyRows := max(maxH-4, 3)

	leftCursor, rightCursor := "", ""
	if o.focusNotes {
		rightCursor = o.theme.Title.Render("█")
	} else {
		leftCursor = o.theme.Title.Render("█")
	}
	leftRows := o.examiner.view(leftW, bodyRows, leftCursor)
	rightRows := o.notes.view(rightW, bodyRows, rightCursor)

	var b strings.Builder
	// Headers — the focused pane's header is highlighted.
	b.WriteString(o.colHeader("EXAMINER TEXT  (ctrl-w h)", leftW, !o.focusNotes))
	b.WriteString(sep)
	b.WriteString(o.colHeader("NOTES  (ctrl-w l)", rightW, o.focusNotes))
	b.WriteByte('\n')

	for i := 0; i < bodyRows; i++ {
		b.WriteString(o.colRow(leftRows, i, leftW, !o.focusNotes))
		b.WriteString(sep)
		b.WriteString(o.colRow(rightRows, i, rightW, o.focusNotes))
		b.WriteByte('\n')
	}

	footer := "ctrl-w h/l switch · ctrl+s save · esc close"
	if mode := o.notes.modeLabel(); mode != "" && o.focusNotes {
		footer = "-- " + mode + " -- · " + footer
	}
	if o.msg != "" {
		footer = o.msg + " · " + footer
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(footer, maxW)))
	return b.String()
}

func (o *OfficeActionEditor) colHeader(label string, w int, focused bool) string {
	cell := render.Pad(render.Truncate(label, w), w)
	if focused {
		return o.theme.Header.Render(cell)
	}
	return o.theme.Dim.Render(cell)
}

func (o *OfficeActionEditor) colRow(rows []editorRow, i, w int, focused bool) string {
	if i >= len(rows) {
		return render.Pad("", w)
	}
	cell := render.Pad(render.Truncate(rows[i].text, w), w)
	if rows[i].selected && focused {
		return o.theme.Selected.Render(cell)
	}
	return o.theme.Row.Render(cell)
}
