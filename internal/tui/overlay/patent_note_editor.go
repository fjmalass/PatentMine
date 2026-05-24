package overlay

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

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
const patentNotePageRows = 10

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

type vimUndo struct {
	value  string
	line   int
	column int
}

// PatentNoteEditor edits one project-scoped markdown note for a patent.
type PatentNoteEditor struct {
	client  *rpc.Client
	theme   render.Theme
	project domain.ProjectID
	number  domain.PatentNumber

	patent       domain.Patent
	note         *domain.PatentNote
	openedAt     time.Time
	value        string
	line         int
	column       int
	offset       int
	loading      bool
	loadErr      string
	msg          string
	dirty        bool
	confirmClear bool

	vimMode    bool      // ctrl+v toggles vim mode
	vimInsert  bool      // true=insert, false=normal
	vimPending rune      // non-zero when waiting for second key (g→gg, d→dd/d$/dw)
	undos      []vimUndo
}

func NewPatentNoteEditor(client *rpc.Client, theme render.Theme, project domain.ProjectID, number domain.PatentNumber) *PatentNoteEditor {
	return &PatentNoteEditor{client: client, theme: theme, project: project, number: number, loading: true, openedAt: time.Now()}
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
	if isVimToggleKey(msg) {
		o.vimMode = !o.vimMode
		if o.vimMode {
			o.vimInsert = false
			o.vimPending = 0
			o.saveUndo()
			o.msg = "vim on"
		} else {
			o.msg = ""
		}
		return o, nil, true
	}
	if o.vimMode {
		return o.handleVimKey(msg)
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
	case tea.KeyPgUp:
		o.pageVertical(-patentNotePageRows)
		return o, nil, true
	case tea.KeyPgDown:
		o.pageVertical(patentNotePageRows)
		return o, nil, true
	case tea.KeyRunes, tea.KeySpace:
		switch msg.String() {
		case "j":
			o.moveVertical(1)
			return o, nil, true
		case "k":
			o.moveVertical(-1)
			return o, nil, true
		}
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

func (o *PatentNoteEditor) handleVimKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if o.vimPending != 0 {
		return o.handleVimPending(msg)
	}
	if o.vimInsert {
		switch msg.Type {
		case tea.KeyEsc:
			o.vimInsert = false
			o.msg = ""
			return o, nil, true
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
			o.insertText(msg.String())
			return o, nil, true
		}
		return o, nil, true
	}
	switch msg.Type {
	case tea.KeyEsc:
		return o, nil, true
	case tea.KeyCtrlS:
		return o, o.saveCmd(), true
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
	case tea.KeyPgUp:
		o.pageVertical(-patentNotePageRows)
		return o, nil, true
	case tea.KeyPgDown:
		o.pageVertical(patentNotePageRows)
		return o, nil, true
	case tea.KeyCtrlU:
		o.pageVertical(-patentNotePageRows / 2)
		return o, nil, true
	case tea.KeyCtrlD:
		o.pageVertical(patentNotePageRows / 2)
		return o, nil, true
	case tea.KeyRunes, tea.KeySpace:
		switch msg.String() {
		case "h":
			o.moveLeft()
		case "j":
			o.moveVertical(1)
		case "k":
			o.moveVertical(-1)
		case "l":
			o.moveRight()
		case "w":
			o.moveWordForward()
		case "b":
			o.moveWordBackward()
		case "0":
			o.column = 0
		case "$":
			o.column = len([]rune(o.lines()[o.line]))
		case "i":
			o.vimInsert = true
			o.saveUndo()
		case "a":
			o.moveRight()
			o.vimInsert = true
			o.saveUndo()
		case "I":
			o.column = 0
			o.vimInsert = true
			o.saveUndo()
		case "A":
			o.column = len([]rune(o.lines()[o.line]))
			o.vimInsert = true
			o.saveUndo()
		case "o":
			o.openLineBelow()
			o.vimInsert = true
			o.saveUndo()
		case "O":
			o.openLineAbove()
			o.vimInsert = true
			o.saveUndo()
		case "x":
			o.saveUndo()
			o.deleteForward()
		case "X":
			o.saveUndo()
			o.backspace()
		case "d":
			o.vimPending = 'd'
			return o, nil, true
		case "D":
			o.saveUndo()
			o.deleteToEndOfLine()
		case "u":
			o.undo()
		case "g":
			o.vimPending = 'g'
			return o, nil, true
		case "G":
			o.line = len(o.lines()) - 1
			o.column = min(o.column, len([]rune(o.lines()[o.line])))
		}
		return o, nil, true
	}
	return o, nil, true
}

func (o *PatentNoteEditor) handleVimPending(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch o.vimPending {
	case 'g':
		if msg.String() == "g" {
			o.line = 0
			o.column = 0
		}
		o.vimPending = 0
		return o, nil, true
	case 'd':
		switch msg.String() {
		case "d":
			o.saveUndo()
			o.deleteCurrentLine()
		case "$":
			o.saveUndo()
			o.deleteToEndOfLine()
		case "w":
			o.saveUndo()
			o.deleteWordForward()
		case "G":
			o.saveUndo()
			o.deleteToEndOfFile()
		}
		o.vimPending = 0
		return o, nil, true
	}
	o.vimPending = 0
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
	lines := o.lines()
	bodyRows := max(maxH-8, 3)
	o.keepCursorVisible(maxW, bodyRows)
	rows := o.visibleRows(lines, maxW, bodyRows)
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
	if o.vimMode {
		mode := "NORMAL"
		if o.vimInsert {
			mode = "INSERT"
		}
		fmt.Fprintf(&b, "%s",
			o.theme.Dim.Render(render.Truncate(fmt.Sprintf("-- %s -- · ctrl+] off · ctrl+s save · esc close", mode), maxW)))
	} else {
		b.WriteString(o.theme.Dim.Render(render.Truncate("markdown editor · ctrl+s save · D clear · esc close · ctrl+] vim", maxW)))
	}
	return b.String()
}

type patentNoteViewRow struct {
	text     string
	selected bool
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

func (o *PatentNoteEditor) lines() []string {
	lines := strings.Split(o.value, "\n")
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
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


func (o *PatentNoteEditor) keepCursorVisible(maxW, bodyRows int) {
	if o.line < o.offset {
		o.offset = o.line
	}
	for o.offset < o.line && o.visualRowsBetween(o.offset, o.line, maxW)+o.editorLineRows(o.line, maxW) > bodyRows {
		o.offset++
	}
	if o.offset < 0 {
		o.offset = 0
	}
}

func (o *PatentNoteEditor) visibleRows(lines []string, maxW, bodyRows int) []patentNoteViewRow {
	rows := make([]patentNoteViewRow, 0, bodyRows)
	for i := o.offset; i < len(lines) && len(rows) < bodyRows; i++ {
		prefix := fmt.Sprintf("%3d ", i+1)
		line := lines[i]
		selected := i == o.line
		contentW := max(maxW-ansi.StringWidth(prefix), 1)
		wrapped := wrapPatentNoteLine(line, contentW)
		if selected {
			wrapped = wrapPatentNoteCursor(wrapped, contentW, o.column, o.theme.Title.Render("█"))
		}
		for j, part := range wrapped {
			linePrefix := prefix
			if j > 0 {
				linePrefix = strings.Repeat(" ", ansi.StringWidth(prefix))
			}
			rows = append(rows, patentNoteViewRow{text: linePrefix + part, selected: selected})
			if len(rows) >= bodyRows {
				break
			}
		}
	}
	return rows
}

func (o *PatentNoteEditor) visualRowsBetween(start, end, maxW int) int {
	lines := o.lines()
	rows := 0
	for i := start; i < end && i < len(lines); i++ {
		rows += o.editorLineRows(i, maxW)
	}
	return rows
}

func (o *PatentNoteEditor) editorLineRows(index, maxW int) int {
	lines := o.lines()
	if index < 0 || index >= len(lines) {
		return 0
	}
	prefix := fmt.Sprintf("%3d ", index+1)
	contentW := max(maxW-ansi.StringWidth(prefix), 1)
	return len(wrapPatentNoteLine(lines[index], contentW))
}

func wrapPatentNoteLine(line string, width int) []string {
	if width < 1 {
		width = 1
	}
	runes := []rune(line)
	if len(runes) == 0 {
		return []string{""}
	}
	parts := make([]string, 0, (len(runes)+width-1)/width)
	for len(runes) > width {
		parts = append(parts, string(runes[:width]))
		runes = runes[width:]
	}
	parts = append(parts, string(runes))
	return parts
}

func wrapPatentNoteCursor(parts []string, width, col int, cursor string) []string {
	if len(parts) == 0 {
		return []string{cursor}
	}
	if width < 1 {
		width = 1
	}
	wrapped := make([]string, 0, len(parts))
	pos := max(col, 0)
	inserted := false
	for i, part := range parts {
		runes := []rune(part)
		segmentLen := len(runes)
		atEnd := i == len(parts)-1
		if !inserted && (pos < segmentLen || (atEnd && pos == segmentLen)) {
			offset := min(pos, segmentLen)
			wrapped = append(wrapped, string(runes[:offset])+cursor+string(runes[offset:]))
			inserted = true
			continue
		}
		wrapped = append(wrapped, part)
		pos -= segmentLen
	}
	if !inserted {
		wrapped[len(wrapped)-1] += cursor
	}
	return wrapped
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

func (o *PatentNoteEditor) pageVertical(delta int) {
	lines := o.lines()
	o.line = max(0, min(o.line+delta, len(lines)-1))
	o.column = min(o.column, len([]rune(lines[o.line])))
}

func (o *PatentNoteEditor) saveUndo() {
	e := vimUndo{value: o.value, line: o.line, column: o.column}
	o.undos = append(o.undos, e)
	if len(o.undos) > 100 {
		o.undos = o.undos[1:]
	}
}

func (o *PatentNoteEditor) undo() {
	if len(o.undos) == 0 {
		return
	}
	e := o.undos[len(o.undos)-1]
	o.undos = o.undos[:len(o.undos)-1]
	o.value = e.value
	o.line = e.line
	o.column = e.column
	o.dirty = true
}

func (o *PatentNoteEditor) moveWordForward() {
	lines := o.lines()
	runes := []rune(lines[o.line])
	col := o.column
	if col >= len(runes) {
		if o.line+1 < len(lines) {
			o.line++
			o.column = 0
		}
		return
	}
	wordType := isWordChar(runes[col])
	for col < len(runes) && isWordChar(runes[col]) == wordType {
		col++
	}
	for col < len(runes) && (runes[col] == ' ' || runes[col] == '\t') {
		col++
	}
	if col < len(runes) {
		o.column = col
	} else if o.line+1 < len(lines) {
		o.line++
		o.column = 0
	}
}

func (o *PatentNoteEditor) moveWordBackward() {
	if o.column == 0 && o.line == 0 {
		return
	}
	lines := o.lines()
	col := o.column
	line := o.line
	if col == 0 {
		line--
		col = len([]rune(lines[line]))
	}
	runes := []rune(lines[line])
	if len(runes) == 0 {
		o.line = line
		o.column = 0
		return
	}
	col = min(col, len(runes))
	if col > 0 {
		col--
	}
	for col > 0 && (runes[col] == ' ' || runes[col] == '\t') {
		col--
	}
	if runes[col] == ' ' || runes[col] == '\t' {
		o.line = line
		o.column = col
		return
	}
	wordType := isWordChar(runes[col])
	for col > 0 && isWordChar(runes[col]) == wordType {
		col--
	}
	if col == 0 && isWordChar(runes[0]) == wordType {
		o.line = line
		o.column = 0
		return
	}
	if isWordChar(runes[col]) != wordType {
		col++
	}
	o.line = line
	o.column = col
}

func (o *PatentNoteEditor) deleteCurrentLine() {
	lines := o.lines()
	if len(lines) <= 1 {
		o.value = ""
		o.line = 0
		o.column = 0
		o.dirty = true
		return
	}
	lines = append(lines[:o.line], lines[o.line+1:]...)
	if o.line >= len(lines) {
		o.line = len(lines) - 1
	}
	o.column = 0
	o.value = strings.Join(lines, "\n")
	o.dirty = true
}

func (o *PatentNoteEditor) deleteToEndOfLine() {
	lines := o.lines()
	runes := []rune(lines[o.line])
	col := min(o.column, len(runes))
	lines[o.line] = string(runes[:col])
	o.value = strings.Join(lines, "\n")
	o.dirty = true
}

func (o *PatentNoteEditor) deleteWordForward() {
	lines := o.lines()
	runes := []rune(lines[o.line])
	col := min(o.column, len(runes))
	if col >= len(runes) {
		return
	}
	wordType := isWordChar(runes[col])
	end := col
	for end < len(runes) && isWordChar(runes[end]) == wordType {
		end++
	}
	for end < len(runes) && (runes[end] == ' ' || runes[end] == '\t') {
		end++
	}
	lines[o.line] = string(runes[:col]) + string(runes[end:])
	o.value = strings.Join(lines, "\n")
	o.dirty = true
}

func (o *PatentNoteEditor) deleteToEndOfFile() {
	lines := o.lines()
	runes := []rune(lines[o.line])
	col := min(o.column, len(runes))
	lines[o.line] = string(runes[:col])
	o.value = strings.Join(lines[:o.line+1], "\n")
	o.dirty = true
}

func (o *PatentNoteEditor) openLineAbove() {
	lines := o.lines()
	lines = append(lines[:o.line], append([]string{""}, lines[o.line:]...)...)
	o.value = strings.Join(lines, "\n")
	o.column = 0
	o.dirty = true
}

func (o *PatentNoteEditor) openLineBelow() {
	lines := o.lines()
	lines = append(lines[:o.line+1], append([]string{""}, lines[o.line+1:]...)...)
	o.value = strings.Join(lines, "\n")
	o.line++
	o.column = 0
	o.dirty = true
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}
