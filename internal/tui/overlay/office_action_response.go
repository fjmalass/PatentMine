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

const (
	respWidthPct  = 88
	respHeightPct = 82
	respMinWidth  = 84
	respMinHeight = 22
)

type responseSavedMsg struct {
	draft domain.Draft
	err   error
}

type responseExportedMsg struct {
	path string
	err  error
}

// ResponseEditor drafts an office-action response by copying from the matter's
// documents into it. The left pane shows one source document (read-only,
// vim-navigable; ctrl-n/ctrl-p cycle which document); the right pane is the
// response's REMARKS section (editable). A shared yank register makes vim
// yy/p copy a passage from the left into the right. ctrl-w h/l switches panes,
// ctrl+s saves the draft, ctrl-e exports it to .docx, esc closes. The response
// is a first-class DraftOAResponse, so the existing renderer produces the .docx.
type ResponseEditor struct {
	client *rpc.Client
	theme  render.Theme
	draft  domain.Draft

	remarksIdx int // index of the REMARKS section in draft.Sections
	docs       []domain.MatterDocument
	docIdx     int

	source   *vimBuffer // read-only source document
	response *vimBuffer // editable REMARKS body

	focusResponse bool
	pendingWindow bool
	focus         focusTimer
	msg           string
}

// NewResponseEditor builds the editor over a response draft and the matter's
// documents. Both panes share one yank register so a passage yanked on the left
// pastes on the right.
func NewResponseEditor(client *rpc.Client, theme render.Theme, draft domain.Draft, docs []domain.MatterDocument) *ResponseEditor {
	remarksIdx := remarksSectionIndex(&draft)
	reg := &yankRegister{}

	source := newVimBuffer(documentText(docs, 0))
	source.vimMode = true
	source.readOnly = true
	source.yank = reg

	response := newVimBuffer(draft.Sections[remarksIdx].Body)
	response.vimMode = true
	response.yank = reg

	return &ResponseEditor{
		client:        client,
		theme:         theme,
		draft:         draft,
		remarksIdx:    remarksIdx,
		docs:          docs,
		source:        source,
		response:      response,
		focusResponse: true,
		focus:         newFocusTimer(true), // opens focused on the editable response
	}
}

// closeCmd flushes the session's reading/writing time as auto entries and closes.
func (o *ResponseEditor) closeCmd() tea.Cmd {
	return tea.Batch(
		o.focus.flushCmd(o.client, o.draft.Project, o.draft.OfficeActionID),
		func() tea.Msg { return CloseOverlayMsg{} },
	)
}

// remarksSectionIndex returns the index of the REMARKS section, creating one in
// memory if the draft somehow has none, so the editor always has a target.
func remarksSectionIndex(d *domain.Draft) int {
	for i, s := range d.Sections {
		if s.Kind == domain.SectionRemarks {
			return i
		}
	}
	d.Sections = append(d.Sections, domain.DraftSection{
		Ordinal: len(d.Sections), Kind: domain.SectionRemarks, Heading: domain.SectionRemarks.Heading(),
	})
	return len(d.Sections) - 1
}

func documentText(docs []domain.MatterDocument, i int) string {
	if i < 0 || i >= len(docs) {
		return "(no documents — add sources with :add.document, then ctrl-n to cycle them)"
	}
	if strings.TrimSpace(docs[i].ExtractedText) == "" {
		return "(no embedded text; OCR appears needed but is not implemented)"
	}
	return docs[i].ExtractedText
}

func (o *ResponseEditor) Title() string {
	if o.draft.Title != "" {
		return "Response · " + o.draft.Title
	}
	return "Office Action Response"
}

func (o *ResponseEditor) Handles() []command.ID { return nil }

func (o *ResponseEditor) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *ResponseEditor) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, respWidthPct, respHeightPct, respMinWidth, respMinHeight)
}

func (o *ResponseEditor) focused() *vimBuffer {
	if o.focusResponse {
		return o.response
	}
	return o.source
}

func (o *ResponseEditor) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case responseSavedMsg:
		if m.err != nil {
			o.msg = "save failed: " + m.err.Error()
			return o, nil
		}
		o.draft = m.draft
		o.response.dirty = false
		o.msg = "saved"
		return o, func() tea.Msg {
			return pane.StatusMsg{Key: text.StatusFilter, Args: []any{"saved office-action response"}}
		}
	case responseExportedMsg:
		if m.err != nil {
			o.msg = "export failed: " + m.err.Error()
			return o, nil
		}
		o.msg = "exported .docx → " + m.path
		return o, nil
	}
	return o, nil
}

func (o *ResponseEditor) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	// ctrl-w window prefix: next h/l/w switches pane focus (gated to non-insert).
	if o.pendingWindow {
		o.pendingWindow = false
		switch msg.String() {
		case "h":
			o.focusResponse = false
		case "l":
			o.focusResponse = true
		case "w":
			o.focusResponse = !o.focusResponse
		}
		o.focus.switchTo(o.focusResponse) // left=source(reading), right=response(writing)
		return o, nil, true
	}
	if msg.Type == tea.KeyCtrlW && !(o.focusResponse && o.response.vimInsert) {
		o.pendingWindow = true
		return o, nil, true
	}

	switch msg.Type {
	case tea.KeyCtrlN:
		o.cycleSource(1)
		return o, nil, true
	case tea.KeyCtrlP:
		o.cycleSource(-1)
		return o, nil, true
	case tea.KeyCtrlE:
		return o, o.exportCmd(), true
	case tea.KeyEsc:
		if o.focusResponse && o.response.vimInsert {
			o.response.handleKey(msg)
			return o, nil, true
		}
		return o, o.closeCmd(), true
	}

	switch _, intent := o.focused().handleKey(msg); intent {
	case intentSave:
		return o, o.saveCmd(), true
	case intentClose:
		return o, o.closeCmd(), true
	}
	return o, nil, true
}

// cycleSource switches which matter document the left pane shows, keeping the
// shared yank register so a yank survives the switch.
func (o *ResponseEditor) cycleSource(delta int) {
	if len(o.docs) == 0 {
		return
	}
	o.docIdx = (o.docIdx + delta + len(o.docs)) % len(o.docs)
	o.source.SetValue(documentText(o.docs, o.docIdx))
	o.source.line, o.source.column, o.source.offset = 0, 0, 0
}

func (o *ResponseEditor) saveCmd() tea.Cmd {
	o.draft.Sections[o.remarksIdx].Body = o.response.Value()
	o.draft.Sections[o.remarksIdx].HumanEdited = true
	client, d := o.client, o.draft
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.DraftResult
		err := client.Call(ctx, proto.MethodDraftSave, proto.DraftSaveParams{Draft: d}, &res)
		return responseSavedMsg{draft: res.Draft, err: err}
	}
}

// exportCmd saves the current body, then renders the response to .docx.
func (o *ResponseEditor) exportCmd() tea.Cmd {
	o.draft.Sections[o.remarksIdx].Body = o.response.Value()
	o.draft.Sections[o.remarksIdx].HumanEdited = true
	client, d := o.client, o.draft
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var saved proto.DraftResult
		if err := client.Call(ctx, proto.MethodDraftSave, proto.DraftSaveParams{Draft: d}, &saved); err != nil {
			return responseExportedMsg{err: err}
		}
		var res proto.DraftExportResult
		err := client.Call(ctx, proto.MethodDraftExportDocx, proto.DraftIDParams{ID: d.ID}, &res)
		return responseExportedMsg{path: res.Path, err: err}
	}
}

func (o *ResponseEditor) View(maxW, maxH int) string {
	const sep = " │ "
	leftW := max((maxW-len(sep))/2, 8)
	rightW := max(maxW-len(sep)-leftW, 8)
	bodyRows := max(maxH-4, 3)

	leftCursor, rightCursor := "", ""
	if o.focusResponse {
		rightCursor = o.theme.Title.Render("█")
	} else {
		leftCursor = o.theme.Title.Render("█")
	}
	leftRows := o.source.view(leftW, bodyRows, leftCursor)
	rightRows := o.response.view(rightW, bodyRows, rightCursor)

	srcLabel := "SOURCE"
	if len(o.docs) > 0 {
		srcLabel = fmt.Sprintf("SOURCE %d/%d · %s", o.docIdx+1, len(o.docs), o.docs[o.docIdx].DisplayName)
	}

	var b strings.Builder
	b.WriteString(o.colHeader(render.Truncate(srcLabel, leftW), leftW, !o.focusResponse))
	b.WriteString(sep)
	b.WriteString(o.colHeader("RESPONSE — REMARKS  (ctrl-w l)", rightW, o.focusResponse))
	b.WriteByte('\n')

	for i := 0; i < bodyRows; i++ {
		b.WriteString(o.colRow(leftRows, i, leftW, !o.focusResponse))
		b.WriteString(sep)
		b.WriteString(o.colRow(rightRows, i, rightW, o.focusResponse))
		b.WriteByte('\n')
	}

	footer := "ctrl-w h/l pane · ctrl-n/p source · yy/p copy→paste · ctrl+s save · ctrl-e export .docx · esc close"
	if mode := o.response.modeLabel(); mode != "" && o.focusResponse {
		footer = "-- " + mode + " -- · " + footer
	}
	if o.msg != "" {
		footer = o.msg + " · " + footer
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(footer, maxW)))
	return b.String()
}

func (o *ResponseEditor) colHeader(label string, w int, focused bool) string {
	cell := render.Pad(render.Truncate(label, w), w)
	if focused {
		return o.theme.Header.Render(cell)
	}
	return o.theme.Dim.Render(cell)
}

func (o *ResponseEditor) colRow(rows []editorRow, i, w int, focused bool) string {
	if i >= len(rows) {
		return render.Pad("", w)
	}
	cell := render.Pad(render.Truncate(rows[i].text, w), w)
	if rows[i].selected && focused {
		return o.theme.Selected.Render(cell)
	}
	return o.theme.Row.Render(cell)
}
