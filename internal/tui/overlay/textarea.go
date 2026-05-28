package overlay

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"patentmine/internal/command"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

// TextArea is a multiline text-entry overlay for longer free-text fields.
type TextArea struct {
	theme   render.Theme
	catalog *text.Catalog
	purpose Purpose
	title   text.Key
	caption text.Key
	value   string
	line    int
	column  int
	offset  int

	vimMode    bool
	vimInsert  bool
	vimPending rune
	vimCount   int
	undos      []vimUndo
}

func NewTextArea(theme render.Theme, catalog *text.Catalog, purpose Purpose, title, caption text.Key, initial string) *TextArea {
	lines := strings.Split(initial, "\n")
	line := max(len(lines)-1, 0)
	column := 0
	if len(lines) > 0 {
		column = len([]rune(lines[line]))
	}
	return &TextArea{
		theme:   theme,
		catalog: catalog,
		purpose: purpose,
		title:   title,
		caption: caption,
		value:   initial,
		line:    line,
		column:  column,
	}
}

func (t *TextArea) Title() string { return t.catalog.T(t.title) }

func (t *TextArea) Handles() []command.ID { return []command.ID{command.CloseOverlay} }

func (t *TextArea) Command(id command.ID, repeat int) (Overlay, tea.Cmd) {
	if id == command.CloseOverlay {
		return t, func() tea.Msg { return CloseOverlayMsg{} }
	}
	return t, nil
}

func (t *TextArea) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, patentNoteOverlayWidthPct, patentNoteOverlayHeightPct, patentNoteMinWidth, patentNoteMinHeight)
}

func (t *TextArea) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if isVimToggleKey(msg) {
		t.vimMode = !t.vimMode
		if t.vimMode {
			t.vimInsert = false
			t.vimPending = 0
			t.saveUndo()
		}
		return t, nil, true
	}
	if t.vimMode {
		return t.handleVimKey(msg)
	}
	switch msg.Type {
	case tea.KeyEsc:
		return t, func() tea.Msg { return CloseOverlayMsg{} }, true
	case tea.KeyCtrlS:
		return t, t.submit(), true
	case tea.KeyEnter:
		t.insertText("\n")
		return t, nil, true
	case tea.KeyBackspace:
		t.backspace()
		return t, nil, true
	case tea.KeyDelete:
		t.deleteForward()
		return t, nil, true
	case tea.KeyLeft:
		t.moveLeft()
		return t, nil, true
	case tea.KeyRight:
		t.moveRight()
		return t, nil, true
	case tea.KeyUp:
		t.moveVertical(-1)
		return t, nil, true
	case tea.KeyDown:
		t.moveVertical(1)
		return t, nil, true
	case tea.KeyPgUp:
		t.pageVertical(-patentNotePageRows)
		return t, nil, true
	case tea.KeyPgDown:
		t.pageVertical(patentNotePageRows)
		return t, nil, true
	case tea.KeyRunes, tea.KeySpace:
		switch msg.String() {
		case "j":
			t.moveVertical(1)
			return t, nil, true
		case "k":
			t.moveVertical(-1)
			return t, nil, true
		}
		t.insertText(msg.String())
		return t, nil, true
	}
	return t, nil, true
}

func (t *TextArea) handleVimKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if t.vimPending != 0 {
		return t.handleVimPending(msg)
	}
	if t.vimInsert {
		switch msg.Type {
		case tea.KeyEsc:
			t.vimInsert = false
			return t, nil, true
		case tea.KeyCtrlS:
			return t, t.submit(), true
		case tea.KeyEnter:
			t.insertText("\n")
			return t, nil, true
		case tea.KeyBackspace:
			t.backspace()
			return t, nil, true
		case tea.KeyDelete:
			t.deleteForward()
			return t, nil, true
		case tea.KeyLeft:
			t.moveLeft()
			return t, nil, true
		case tea.KeyRight:
			t.moveRight()
			return t, nil, true
		case tea.KeyUp:
			t.moveVertical(-1)
			return t, nil, true
		case tea.KeyDown:
			t.moveVertical(1)
			return t, nil, true
		case tea.KeyRunes, tea.KeySpace:
			t.insertText(msg.String())
			return t, nil, true
		}
		return t, nil, true
	}
	switch msg.Type {
	case tea.KeyEsc:
		return t, nil, true
	case tea.KeyCtrlS:
		return t, t.submit(), true
	case tea.KeyLeft:
		t.moveLeft()
		return t, nil, true
	case tea.KeyRight:
		t.moveRight()
		return t, nil, true
	case tea.KeyUp:
		t.moveVertical(-1)
		return t, nil, true
	case tea.KeyDown:
		t.moveVertical(1)
		return t, nil, true
	case tea.KeyRunes, tea.KeySpace:
		s := msg.String()
		if len(s) == 1 && s[0] >= '0' && s[0] <= '9' && (s != "0" || t.vimCount > 0) {
			t.vimCount = t.vimCount*10 + int(s[0]-'0')
			return t, nil, true
		}
		count := t.vimCount
		t.vimCount = 0
		switch s {
		case "h":
			t.moveLeft()
		case "l":
			t.moveRight()
		case "j":
			t.moveVertical(1)
		case "k":
			t.moveVertical(-1)
		case "i":
			t.vimInsert = true
			t.saveUndo()
		case "a":
			if t.column < len([]rune(t.lines()[t.line])) {
				t.column++
			}
			t.vimInsert = true
			t.saveUndo()
		case "I":
			t.column = 0
			t.vimInsert = true
			t.saveUndo()
		case "A":
			t.column = len([]rune(t.lines()[t.line]))
			t.vimInsert = true
			t.saveUndo()
		case "x":
			t.saveUndo()
			t.deleteForward()
		case "X":
			t.saveUndo()
			t.backspace()
		case "o":
			t.saveUndo()
			t.openLineBelow()
			t.vimInsert = true
		case "O":
			t.saveUndo()
			t.openLineAbove()
			t.vimInsert = true
		case "0":
			t.column = 0
		case "$":
			t.column = len([]rune(t.lines()[t.line]))
		case "g", "d":
			t.vimPending = []rune(s)[0]
			t.vimCount = count // preserve count across the prefix
		case "G":
			t.gotoLine(count, len(t.lines())-1)
		case "w":
			t.moveWordForward()
		case "b":
			t.moveWordBackward()
		case "u":
			t.undo()
		}
		return t, nil, true
	}
	return t, nil, true
}

// gotoLine jumps to the 1-based line count, or fallback when count==0.
// The cursor column is clamped to the target line's length.
func (t *TextArea) gotoLine(count, fallback int) {
	lines := t.lines()
	if len(lines) == 0 {
		return
	}
	target := fallback
	if count > 0 {
		target = count - 1
	}
	t.line = min(max(target, 0), len(lines)-1)
	t.column = min(t.column, len([]rune(lines[t.line])))
}

func (t *TextArea) handleVimPending(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	pending := t.vimPending
	t.vimPending = 0
	count := t.vimCount
	t.vimCount = 0
	if msg.Type != tea.KeyRunes || len(msg.Runes) != 1 {
		return t, nil, true
	}
	switch pending {
	case 'g':
		if msg.String() == "g" {
			t.gotoLine(count, 0)
		}
	case 'd':
		t.saveUndo()
		switch msg.String() {
		case "d":
			t.deleteCurrentLine()
		case "$", "D":
			t.deleteToEndOfLine()
		case "w":
			t.deleteWordForward()
		case "g":
			t.deleteToEndOfFile()
		}
	}
	return t, nil, true
}

func (t *TextArea) submit() tea.Cmd {
	return func() tea.Msg { return TextSubmitMsg{Purpose: t.purpose, Value: t.value} }
}

func (t *TextArea) View(maxW, maxH int) string {
	lines := t.lines()
	bodyRows := max(maxH-6, 3)
	t.keepCursorVisible(maxW, bodyRows)
	rows := t.visibleRows(lines, maxW, bodyRows)
	var b strings.Builder
	b.WriteString(t.theme.Row.Render(render.Truncate(t.catalog.T(t.caption), maxW)))
	b.WriteString("\n\n")
	for i, row := range rows {
		if row.selected {
			b.WriteString(t.theme.Selected.Render(render.Pad(render.Truncate(row.text, maxW), maxW)))
		} else {
			b.WriteString(t.theme.Row.Render(render.Truncate(row.text, maxW)))
		}
		if i < len(rows)-1 {
			b.WriteByte('\n')
		}
	}
	if len(rows) == 0 {
		b.WriteString(t.theme.Selected.Render(render.Pad("  1 "+t.theme.Title.Render("█"), maxW)))
	}
	b.WriteString("\n\n")
	if t.vimMode {
		mode := "NORMAL"
		if t.vimInsert {
			mode = "INSERT"
		}
		b.WriteString(t.theme.Dim.Render(render.Truncate(fmt.Sprintf("-- %s -- · ctrl+s save · esc close · ctrl+] off", mode), maxW)))
	} else {
		b.WriteString(t.theme.Dim.Render(render.Truncate("multiline editor · enter newline · ctrl+s save · esc close · ctrl+] vim", maxW)))
	}
	return b.String()
}

func (t *TextArea) lines() []string {
	lines := strings.Split(t.value, "\n")
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func (t *TextArea) keepCursorVisible(maxW, bodyRows int) {
	if t.line < t.offset {
		t.offset = t.line
	}
	for t.offset < t.line && t.visualRowsBetween(t.offset, t.line, maxW)+t.editorLineRows(t.line, maxW) > bodyRows {
		t.offset++
	}
	if t.offset < 0 {
		t.offset = 0
	}
}

func (t *TextArea) visibleRows(lines []string, maxW, bodyRows int) []patentNoteViewRow {
	rows := make([]patentNoteViewRow, 0, bodyRows)
	for i := t.offset; i < len(lines) && len(rows) < bodyRows; i++ {
		prefix := fmt.Sprintf("%3d ", i+1)
		contentW := max(maxW-ansi.StringWidth(prefix), 1)
		wrapped := wrapPatentNoteLine(lines[i], contentW)
		selected := i == t.line
		if selected {
			wrapped = wrapPatentNoteCursor(wrapped, contentW, t.column, t.theme.Title.Render("█"))
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

func (t *TextArea) visualRowsBetween(start, end, maxW int) int {
	lines := t.lines()
	rows := 0
	for i := start; i < end && i < len(lines); i++ {
		rows += t.editorLineRows(i, maxW)
	}
	return rows
}

func (t *TextArea) editorLineRows(index, maxW int) int {
	lines := t.lines()
	if index < 0 || index >= len(lines) {
		return 0
	}
	prefix := fmt.Sprintf("%3d ", index+1)
	contentW := max(maxW-ansi.StringWidth(prefix), 1)
	return len(wrapPatentNoteLine(lines[index], contentW))
}

func (t *TextArea) insertText(s string) {
	lines := t.lines()
	runes := []rune(lines[t.line])
	col := min(t.column, len(runes))
	parts := strings.Split(s, "\n")
	if len(parts) == 1 {
		lines[t.line] = string(runes[:col]) + s + string(runes[col:])
		t.value = strings.Join(lines, "\n")
		t.column += len([]rune(s))
		return
	}
	inserted := make([]string, 0, len(parts))
	inserted = append(inserted, string(runes[:col])+parts[0])
	if len(parts) > 2 {
		inserted = append(inserted, parts[1:len(parts)-1]...)
	}
	inserted = append(inserted, parts[len(parts)-1]+string(runes[col:]))
	lines = append(lines[:t.line], append(inserted, lines[t.line+1:]...)...)
	t.value = strings.Join(lines, "\n")
	t.line += len(inserted) - 1
	t.column = len([]rune(parts[len(parts)-1]))
}

func (t *TextArea) backspace() {
	lines := t.lines()
	if t.line == 0 && t.column == 0 {
		return
	}
	if t.column > 0 {
		runes := []rune(lines[t.line])
		col := min(t.column, len(runes))
		lines[t.line] = string(append(runes[:col-1], runes[col:]...))
		t.column--
		t.value = strings.Join(lines, "\n")
		return
	}
	prev := []rune(lines[t.line-1])
	lines[t.line-1] += lines[t.line]
	lines = append(lines[:t.line], lines[t.line+1:]...)
	t.line--
	t.column = len(prev)
	t.value = strings.Join(lines, "\n")
}

func (t *TextArea) deleteForward() {
	lines := t.lines()
	runes := []rune(lines[t.line])
	col := min(t.column, len(runes))
	if col < len(runes) {
		lines[t.line] = string(append(runes[:col], runes[col+1:]...))
		t.value = strings.Join(lines, "\n")
		return
	}
	if t.line+1 >= len(lines) {
		return
	}
	lines[t.line] += lines[t.line+1]
	lines = append(lines[:t.line+1], lines[t.line+2:]...)
	t.value = strings.Join(lines, "\n")
}

func (t *TextArea) moveLeft() {
	if t.column > 0 {
		t.column--
		return
	}
	if t.line == 0 {
		return
	}
	t.line--
	t.column = len([]rune(t.lines()[t.line]))
}

func (t *TextArea) moveRight() {
	lines := t.lines()
	if t.column < len([]rune(lines[t.line])) {
		t.column++
		return
	}
	if t.line+1 >= len(lines) {
		return
	}
	t.line++
	t.column = 0
}

func (t *TextArea) moveVertical(delta int) {
	lines := t.lines()
	t.line = max(0, min(t.line+delta, len(lines)-1))
	t.column = min(t.column, len([]rune(lines[t.line])))
}

func (t *TextArea) pageVertical(delta int) {
	lines := t.lines()
	t.line = max(0, min(t.line+delta, len(lines)-1))
	t.column = min(t.column, len([]rune(lines[t.line])))
}

func (t *TextArea) saveUndo() {
	e := vimUndo{value: t.value, line: t.line, column: t.column}
	t.undos = append(t.undos, e)
	if len(t.undos) > 100 {
		t.undos = t.undos[1:]
	}
}

func (t *TextArea) undo() {
	if len(t.undos) == 0 {
		return
	}
	e := t.undos[len(t.undos)-1]
	t.undos = t.undos[:len(t.undos)-1]
	t.value = e.value
	t.line = e.line
	t.column = e.column
}

func (t *TextArea) moveWordForward() {
	lines := t.lines()
	runes := []rune(lines[t.line])
	col := t.column
	if col >= len(runes) {
		if t.line+1 < len(lines) {
			t.line++
			t.column = 0
		}
		return
	}
	wordType := isTextAreaWordChar(runes[col])
	for col < len(runes) && isTextAreaWordChar(runes[col]) == wordType {
		col++
	}
	for col < len(runes) && (runes[col] == ' ' || runes[col] == '\t') {
		col++
	}
	if col < len(runes) {
		t.column = col
	} else if t.line+1 < len(lines) {
		t.line++
		t.column = 0
	}
}

func (t *TextArea) moveWordBackward() {
	if t.column == 0 && t.line == 0 {
		return
	}
	lines := t.lines()
	col := t.column
	line := t.line
	if col == 0 {
		line--
		col = len([]rune(lines[line]))
	}
	runes := []rune(lines[line])
	if len(runes) == 0 {
		t.line = line
		t.column = 0
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
		t.line = line
		t.column = col
		return
	}
	wordType := isTextAreaWordChar(runes[col])
	for col > 0 && isTextAreaWordChar(runes[col]) == wordType {
		col--
	}
	if col == 0 && isTextAreaWordChar(runes[0]) == wordType {
		t.line = line
		t.column = 0
		return
	}
	if isTextAreaWordChar(runes[col]) != wordType {
		col++
	}
	t.line = line
	t.column = col
}

func (t *TextArea) deleteCurrentLine() {
	lines := t.lines()
	if len(lines) <= 1 {
		t.value = ""
		t.line = 0
		t.column = 0
		return
	}
	lines = append(lines[:t.line], lines[t.line+1:]...)
	if t.line >= len(lines) {
		t.line = len(lines) - 1
	}
	t.column = 0
	t.value = strings.Join(lines, "\n")
}

func (t *TextArea) deleteToEndOfLine() {
	lines := t.lines()
	runes := []rune(lines[t.line])
	col := min(t.column, len(runes))
	lines[t.line] = string(runes[:col])
	t.value = strings.Join(lines, "\n")
}

func (t *TextArea) deleteWordForward() {
	lines := t.lines()
	runes := []rune(lines[t.line])
	col := min(t.column, len(runes))
	if col >= len(runes) {
		return
	}
	wordType := isTextAreaWordChar(runes[col])
	end := col
	for end < len(runes) && isTextAreaWordChar(runes[end]) == wordType {
		end++
	}
	for end < len(runes) && (runes[end] == ' ' || runes[end] == '\t') {
		end++
	}
	lines[t.line] = string(runes[:col]) + string(runes[end:])
	t.value = strings.Join(lines, "\n")
}

func (t *TextArea) deleteToEndOfFile() {
	lines := t.lines()
	runes := []rune(lines[t.line])
	col := min(t.column, len(runes))
	lines[t.line] = string(runes[:col])
	t.value = strings.Join(lines[:t.line+1], "\n")
}

func (t *TextArea) openLineAbove() {
	lines := t.lines()
	lines = append(lines[:t.line], append([]string{""}, lines[t.line:]...)...)
	t.value = strings.Join(lines, "\n")
	t.column = 0
}

func (t *TextArea) openLineBelow() {
	lines := t.lines()
	lines = append(lines[:t.line+1], append([]string{""}, lines[t.line+1:]...)...)
	t.value = strings.Join(lines, "\n")
	t.line++
	t.column = 0
}

func isTextAreaWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func isVimToggleKey(msg tea.KeyMsg) bool {
	return msg.String() == "ctrl+]"
}
