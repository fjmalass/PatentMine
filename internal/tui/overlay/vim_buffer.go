package overlay

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// editorPageRows is the page step for PgUp/PgDn and ctrl-u/ctrl-d.
const editorPageRows = 10

// editorIntent reports a host-level action a key implies that the buffer itself
// does not perform — the host owns what "save" and "close" mean.
type editorIntent int

const (
	intentNone  editorIntent = iota
	intentSave               // ctrl+s
	intentClose              // esc outside vim normal/insert
)

type vimUndo struct {
	value  string
	line   int
	column int
}

// yankRegister is the linewise copy buffer. When two buffers share one register
// (the split editor assigns the same pointer to both panes), text yanked in the
// read-only source pane can be pasted into the editable response pane — the
// copy-from-document-into-response path. text always ends with a newline.
type yankRegister struct{ text string }

// editorRow is one rendered display row of the buffer.
type editorRow struct {
	text     string
	selected bool
}

// vimBuffer is the one modal-vim text buffer behind every overlay editor (the
// multiline text input, patent notes, and office-action notes). It owns the
// text, cursor, vim state machine, undo stack, and wrapped rendering, so the
// editing behaviour lives in exactly one place rather than being copied per
// editor. Hosts embed it by name and map its intents (save/close) to their own
// commands.
type vimBuffer struct {
	value  string
	line   int
	column int
	offset int

	vimMode    bool
	vimInsert  bool
	vimPending rune
	vimCount   int
	undos      []vimUndo

	// readOnly makes the buffer a navigable reference: vim motions and search
	// work, but every mutation is a no-op and insert mode is never entered. Used
	// for the examiner-text pane of the office-action split. A read-only buffer
	// still yanks (yank does not mutate), so a source document can be copied from.
	readOnly bool

	// yank is the linewise copy register. Hosts that want copy-paste between two
	// panes assign the same *yankRegister to both; otherwise it is lazily created
	// per buffer (so yank/paste also works within a single editor).
	yank *yankRegister

	dirty bool
}

// newVimBuffer returns a buffer holding initial with the cursor at the start.
func newVimBuffer(initial string) *vimBuffer {
	return &vimBuffer{value: initial}
}

// Value returns the current text.
func (b *vimBuffer) Value() string { return b.value }

// SetValue replaces the text, clamping the cursor into the new bounds without
// moving it to the end (so a reload keeps the caret where it was).
func (b *vimBuffer) SetValue(s string) {
	b.value = s
	lines := b.lines()
	if b.line >= len(lines) {
		b.line = len(lines) - 1
	}
	if b.line < 0 {
		b.line = 0
	}
	b.column = min(b.column, len([]rune(lines[b.line])))
}

// cursorToEnd places the caret at the end of the last line — the natural start
// point for an editor opened on existing text the user means to append to.
func (b *vimBuffer) cursorToEnd() {
	lines := b.lines()
	b.line = max(len(lines)-1, 0)
	b.column = len([]rune(lines[b.line]))
}

// clear empties the buffer and resets the cursor and dirty flag.
func (b *vimBuffer) clear() {
	b.value = ""
	b.line, b.column, b.offset = 0, 0, 0
	b.dirty = false
}

func (b *vimBuffer) markDirty() { b.dirty = true }

// handleKey applies one key press, returning whether it was consumed and any
// host-level intent (save/close) the key implies. The buffer consumes every key
// while focused, so consumed is always true.
func (b *vimBuffer) handleKey(msg tea.KeyMsg) (bool, editorIntent) {
	if isVimToggleKey(msg) {
		b.vimMode = !b.vimMode
		if b.vimMode {
			b.vimInsert = false
			b.vimPending = 0
			b.saveUndo()
		}
		return true, intentNone
	}
	if b.vimMode {
		return b.handleVimKey(msg)
	}
	switch msg.Type {
	case tea.KeyEsc:
		return true, intentClose
	case tea.KeyCtrlS:
		return true, intentSave
	case tea.KeyEnter:
		b.insertText("\n")
	case tea.KeyBackspace:
		b.backspace()
	case tea.KeyDelete:
		b.deleteForward()
	case tea.KeyLeft:
		b.moveLeft()
	case tea.KeyRight:
		b.moveRight()
	case tea.KeyUp:
		b.moveVertical(-1)
	case tea.KeyDown:
		b.moveVertical(1)
	case tea.KeyPgUp:
		b.pageVertical(-editorPageRows)
	case tea.KeyPgDown:
		b.pageVertical(editorPageRows)
	case tea.KeyRunes, tea.KeySpace:
		// Plain (non-vim) mode is an ordinary text field: every printable key
		// inserts. Navigation here is on the arrow keys; j/k are literal text.
		// (Vim-style j/k motion lives in vim NORMAL mode.)
		b.insertText(msg.String())
	}
	return true, intentNone
}

func (b *vimBuffer) handleVimKey(msg tea.KeyMsg) (bool, editorIntent) {
	if b.vimPending != 0 {
		return b.handleVimPending(msg)
	}
	if b.vimInsert {
		switch msg.Type {
		case tea.KeyEsc:
			b.vimInsert = false
		case tea.KeyCtrlS:
			return true, intentSave
		case tea.KeyEnter:
			b.insertText("\n")
		case tea.KeyBackspace:
			b.backspace()
		case tea.KeyDelete:
			b.deleteForward()
		case tea.KeyLeft:
			b.moveLeft()
		case tea.KeyRight:
			b.moveRight()
		case tea.KeyUp:
			b.moveVertical(-1)
		case tea.KeyDown:
			b.moveVertical(1)
		case tea.KeyRunes, tea.KeySpace:
			b.insertText(msg.String())
		}
		return true, intentNone
	}
	switch msg.Type {
	case tea.KeyEsc:
		return true, intentNone
	case tea.KeyCtrlS:
		return true, intentSave
	case tea.KeyLeft:
		b.moveLeft()
		return true, intentNone
	case tea.KeyRight:
		b.moveRight()
		return true, intentNone
	case tea.KeyUp:
		b.moveVertical(-1)
		return true, intentNone
	case tea.KeyDown:
		b.moveVertical(1)
		return true, intentNone
	case tea.KeyPgUp:
		b.pageVertical(-editorPageRows)
		return true, intentNone
	case tea.KeyPgDown:
		b.pageVertical(editorPageRows)
		return true, intentNone
	case tea.KeyCtrlU:
		b.pageVertical(-editorPageRows / 2)
		return true, intentNone
	case tea.KeyCtrlD:
		b.pageVertical(editorPageRows / 2)
		return true, intentNone
	case tea.KeyRunes, tea.KeySpace:
		s := msg.String()
		if len(s) == 1 && s[0] >= '0' && s[0] <= '9' && (s != "0" || b.vimCount > 0) {
			b.vimCount = b.vimCount*10 + int(s[0]-'0')
			return true, intentNone
		}
		count := b.vimCount
		b.vimCount = 0
		// A read-only buffer accepts motions and search only — no edits, no
		// insert mode.
		if b.readOnly {
			switch s {
			case "h":
				b.moveLeft()
			case "j":
				b.moveVertical(1)
			case "k":
				b.moveVertical(-1)
			case "l":
				b.moveRight()
			case "w":
				b.moveWordForward()
			case "b":
				b.moveWordBackward()
			case "0":
				b.column = 0
			case "$":
				b.column = len([]rune(b.lines()[b.line]))
			case "g":
				b.vimPending = 'g'
			case "G":
				b.gotoLine(count, len(b.lines())-1)
			case "y":
				// Yank is non-mutating, so a read-only source pane supports it.
				b.vimPending = 'y'
				b.vimCount = count
			}
			return true, intentNone
		}
		switch s {
		case "h":
			b.moveLeft()
		case "j":
			b.moveVertical(1)
		case "k":
			b.moveVertical(-1)
		case "l":
			b.moveRight()
		case "w":
			b.moveWordForward()
		case "b":
			b.moveWordBackward()
		case "0":
			b.column = 0
		case "$":
			b.column = len([]rune(b.lines()[b.line]))
		case "i":
			b.vimInsert = true
			b.saveUndo()
		case "a":
			if b.column < len([]rune(b.lines()[b.line])) {
				b.column++
			}
			b.vimInsert = true
			b.saveUndo()
		case "I":
			b.column = 0
			b.vimInsert = true
			b.saveUndo()
		case "A":
			b.column = len([]rune(b.lines()[b.line]))
			b.vimInsert = true
			b.saveUndo()
		case "o":
			b.saveUndo()
			b.openLineBelow()
			b.vimInsert = true
		case "O":
			b.saveUndo()
			b.openLineAbove()
			b.vimInsert = true
		case "x":
			b.saveUndo()
			b.deleteForward()
		case "X":
			b.saveUndo()
			b.backspace()
		case "D":
			b.saveUndo()
			b.deleteToEndOfLine()
		case "d":
			b.vimPending = 'd'
			b.vimCount = count
		case "g":
			b.vimPending = 'g'
			b.vimCount = count
		case "G":
			b.gotoLine(count, len(b.lines())-1)
		case "u":
			b.undo()
		case "y":
			b.vimPending = 'y'
			b.vimCount = count
		case "p":
			b.saveUndo()
			b.pasteLinewise(false)
		case "P":
			b.saveUndo()
			b.pasteLinewise(true)
		}
		return true, intentNone
	}
	return true, intentNone
}

func (b *vimBuffer) handleVimPending(msg tea.KeyMsg) (bool, editorIntent) {
	pending := b.vimPending
	b.vimPending = 0
	count := b.vimCount
	b.vimCount = 0
	switch pending {
	case 'g':
		if msg.String() == "g" {
			b.gotoLine(count, 0)
		}
	case 'd':
		switch msg.String() {
		case "d":
			b.saveUndo()
			b.deleteCurrentLine()
		case "$", "D":
			b.saveUndo()
			b.deleteToEndOfLine()
		case "w":
			b.saveUndo()
			b.deleteWordForward()
		case "G":
			b.saveUndo()
			b.deleteToEndOfFile()
		}
	case 'y':
		if msg.String() == "y" {
			b.yankLines(count)
		}
	}
	return true, intentNone
}

// reg returns the buffer's yank register, lazily allocating a private one so
// yank/paste works within a single editor; the split editor overrides this by
// assigning one shared register to both panes before use.
func (b *vimBuffer) reg() *yankRegister {
	if b.yank == nil {
		b.yank = &yankRegister{}
	}
	return b.yank
}

// yankLines copies n lines (default 1) starting at the cursor into the register,
// linewise (trailing newline), the way vim's yy / Nyy do.
func (b *vimBuffer) yankLines(n int) {
	if n <= 0 {
		n = 1
	}
	lines := b.lines()
	end := min(b.line+n, len(lines))
	b.reg().text = strings.Join(lines[b.line:end], "\n") + "\n"
}

// pasteLinewise inserts the register's linewise text below the cursor line (or
// above it when before is true), placing the cursor on the first pasted line —
// vim's p / P. A no-op in a read-only buffer or with an empty register.
func (b *vimBuffer) pasteLinewise(before bool) {
	if b.readOnly {
		return
	}
	text := b.reg().text
	if text == "" {
		return
	}
	paste := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	lines := b.lines()
	at := b.line + 1
	if before {
		at = b.line
	}
	at = min(at, len(lines))
	out := make([]string, 0, len(lines)+len(paste))
	out = append(out, lines[:at]...)
	out = append(out, paste...)
	out = append(out, lines[at:]...)
	b.value = strings.Join(out, "\n")
	b.line = at
	b.column = 0
	b.markDirty()
}

// gotoLine jumps to the 1-based line count, or fallback when count==0. The
// cursor column is clamped to the target line's length.
func (b *vimBuffer) gotoLine(count, fallback int) {
	lines := b.lines()
	if len(lines) == 0 {
		return
	}
	target := fallback
	if count > 0 {
		target = count - 1
	}
	b.line = min(max(target, 0), len(lines)-1)
	b.column = min(b.column, len([]rune(lines[b.line])))
}

func (b *vimBuffer) lines() []string {
	lines := strings.Split(b.value, "\n")
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

// view renders the visible portion of the buffer into display rows, scrolling
// so the cursor stays in frame. cursorGlyph is the styled caret the host wants
// drawn at the cursor position.
func (b *vimBuffer) view(maxW, bodyRows int, cursorGlyph string) []editorRow {
	b.keepCursorVisible(maxW, bodyRows)
	lines := b.lines()
	rows := make([]editorRow, 0, bodyRows)
	for i := b.offset; i < len(lines) && len(rows) < bodyRows; i++ {
		prefix := fmt.Sprintf("%3d ", i+1)
		contentW := max(maxW-ansi.StringWidth(prefix), 1)
		wrapped := wrapEditorLine(lines[i], contentW)
		selected := i == b.line
		if selected {
			wrapped = wrapEditorCursor(wrapped, contentW, b.column, cursorGlyph)
		}
		for j, part := range wrapped {
			linePrefix := prefix
			if j > 0 {
				linePrefix = strings.Repeat(" ", ansi.StringWidth(prefix))
			}
			rows = append(rows, editorRow{text: linePrefix + part, selected: selected})
			if len(rows) >= bodyRows {
				break
			}
		}
	}
	return rows
}

func (b *vimBuffer) keepCursorVisible(maxW, bodyRows int) {
	if b.line < b.offset {
		b.offset = b.line
	}
	for b.offset < b.line && b.visualRowsBetween(b.offset, b.line, maxW)+b.editorLineRows(b.line, maxW) > bodyRows {
		b.offset++
	}
	if b.offset < 0 {
		b.offset = 0
	}
}

func (b *vimBuffer) visualRowsBetween(start, end, maxW int) int {
	lines := b.lines()
	rows := 0
	for i := start; i < end && i < len(lines); i++ {
		rows += b.editorLineRows(i, maxW)
	}
	return rows
}

func (b *vimBuffer) editorLineRows(index, maxW int) int {
	lines := b.lines()
	if index < 0 || index >= len(lines) {
		return 0
	}
	prefix := fmt.Sprintf("%3d ", index+1)
	contentW := max(maxW-ansi.StringWidth(prefix), 1)
	return len(wrapEditorLine(lines[index], contentW))
}

func (b *vimBuffer) insertText(s string) {
	if b.readOnly {
		return
	}
	lines := b.lines()
	runes := []rune(lines[b.line])
	col := min(b.column, len(runes))
	parts := strings.Split(s, "\n")
	if len(parts) == 1 {
		lines[b.line] = string(runes[:col]) + s + string(runes[col:])
		b.value = strings.Join(lines, "\n")
		b.column += len([]rune(s))
		b.markDirty()
		return
	}
	inserted := make([]string, 0, len(parts))
	inserted = append(inserted, string(runes[:col])+parts[0])
	if len(parts) > 2 {
		inserted = append(inserted, parts[1:len(parts)-1]...)
	}
	inserted = append(inserted, parts[len(parts)-1]+string(runes[col:]))
	lines = append(lines[:b.line], append(inserted, lines[b.line+1:]...)...)
	b.value = strings.Join(lines, "\n")
	b.line += len(inserted) - 1
	b.column = len([]rune(parts[len(parts)-1]))
	b.markDirty()
}

func (b *vimBuffer) backspace() {
	if b.readOnly {
		return
	}
	lines := b.lines()
	if b.line == 0 && b.column == 0 {
		return
	}
	if b.column > 0 {
		runes := []rune(lines[b.line])
		col := min(b.column, len(runes))
		lines[b.line] = string(append(runes[:col-1], runes[col:]...))
		b.column--
		b.value = strings.Join(lines, "\n")
		b.markDirty()
		return
	}
	prev := []rune(lines[b.line-1])
	lines[b.line-1] += lines[b.line]
	lines = append(lines[:b.line], lines[b.line+1:]...)
	b.line--
	b.column = len(prev)
	b.value = strings.Join(lines, "\n")
	b.markDirty()
}

func (b *vimBuffer) deleteForward() {
	if b.readOnly {
		return
	}
	lines := b.lines()
	runes := []rune(lines[b.line])
	col := min(b.column, len(runes))
	if col < len(runes) {
		lines[b.line] = string(append(runes[:col], runes[col+1:]...))
		b.value = strings.Join(lines, "\n")
		b.markDirty()
		return
	}
	if b.line+1 >= len(lines) {
		return
	}
	lines[b.line] += lines[b.line+1]
	lines = append(lines[:b.line+1], lines[b.line+2:]...)
	b.value = strings.Join(lines, "\n")
	b.markDirty()
}

func (b *vimBuffer) moveLeft() {
	if b.column > 0 {
		b.column--
		return
	}
	if b.line == 0 {
		return
	}
	b.line--
	b.column = len([]rune(b.lines()[b.line]))
}

func (b *vimBuffer) moveRight() {
	lines := b.lines()
	if b.column < len([]rune(lines[b.line])) {
		b.column++
		return
	}
	if b.line+1 >= len(lines) {
		return
	}
	b.line++
	b.column = 0
}

func (b *vimBuffer) moveVertical(delta int) {
	lines := b.lines()
	b.line = max(0, min(b.line+delta, len(lines)-1))
	b.column = min(b.column, len([]rune(lines[b.line])))
}

func (b *vimBuffer) pageVertical(delta int) {
	lines := b.lines()
	b.line = max(0, min(b.line+delta, len(lines)-1))
	b.column = min(b.column, len([]rune(lines[b.line])))
}

func (b *vimBuffer) saveUndo() {
	b.undos = append(b.undos, vimUndo{value: b.value, line: b.line, column: b.column})
	if len(b.undos) > 100 {
		b.undos = b.undos[1:]
	}
}

func (b *vimBuffer) undo() {
	if len(b.undos) == 0 {
		return
	}
	e := b.undos[len(b.undos)-1]
	b.undos = b.undos[:len(b.undos)-1]
	b.value = e.value
	b.line = e.line
	b.column = e.column
	b.markDirty()
}

func (b *vimBuffer) moveWordForward() {
	lines := b.lines()
	runes := []rune(lines[b.line])
	col := b.column
	if col >= len(runes) {
		if b.line+1 < len(lines) {
			b.line++
			b.column = 0
		}
		return
	}
	wordType := isEditorWordChar(runes[col])
	for col < len(runes) && isEditorWordChar(runes[col]) == wordType {
		col++
	}
	for col < len(runes) && (runes[col] == ' ' || runes[col] == '\t') {
		col++
	}
	if col < len(runes) {
		b.column = col
	} else if b.line+1 < len(lines) {
		b.line++
		b.column = 0
	}
}

func (b *vimBuffer) moveWordBackward() {
	if b.column == 0 && b.line == 0 {
		return
	}
	lines := b.lines()
	col := b.column
	line := b.line
	if col == 0 {
		line--
		col = len([]rune(lines[line]))
	}
	runes := []rune(lines[line])
	if len(runes) == 0 {
		b.line = line
		b.column = 0
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
		b.line = line
		b.column = col
		return
	}
	wordType := isEditorWordChar(runes[col])
	for col > 0 && isEditorWordChar(runes[col]) == wordType {
		col--
	}
	if col == 0 && isEditorWordChar(runes[0]) == wordType {
		b.line = line
		b.column = 0
		return
	}
	if isEditorWordChar(runes[col]) != wordType {
		col++
	}
	b.line = line
	b.column = col
}

func (b *vimBuffer) deleteCurrentLine() {
	lines := b.lines()
	if len(lines) <= 1 {
		b.value = ""
		b.line = 0
		b.column = 0
		b.markDirty()
		return
	}
	lines = append(lines[:b.line], lines[b.line+1:]...)
	if b.line >= len(lines) {
		b.line = len(lines) - 1
	}
	b.column = 0
	b.value = strings.Join(lines, "\n")
	b.markDirty()
}

func (b *vimBuffer) deleteToEndOfLine() {
	lines := b.lines()
	runes := []rune(lines[b.line])
	col := min(b.column, len(runes))
	lines[b.line] = string(runes[:col])
	b.value = strings.Join(lines, "\n")
	b.markDirty()
}

func (b *vimBuffer) deleteWordForward() {
	lines := b.lines()
	runes := []rune(lines[b.line])
	col := min(b.column, len(runes))
	if col >= len(runes) {
		return
	}
	wordType := isEditorWordChar(runes[col])
	end := col
	for end < len(runes) && isEditorWordChar(runes[end]) == wordType {
		end++
	}
	for end < len(runes) && (runes[end] == ' ' || runes[end] == '\t') {
		end++
	}
	lines[b.line] = string(runes[:col]) + string(runes[end:])
	b.value = strings.Join(lines, "\n")
	b.markDirty()
}

func (b *vimBuffer) deleteToEndOfFile() {
	lines := b.lines()
	runes := []rune(lines[b.line])
	col := min(b.column, len(runes))
	lines[b.line] = string(runes[:col])
	b.value = strings.Join(lines[:b.line+1], "\n")
	b.markDirty()
}

func (b *vimBuffer) openLineAbove() {
	lines := b.lines()
	lines = append(lines[:b.line], append([]string{""}, lines[b.line:]...)...)
	b.value = strings.Join(lines, "\n")
	b.column = 0
	b.markDirty()
}

func (b *vimBuffer) openLineBelow() {
	lines := b.lines()
	lines = append(lines[:b.line+1], append([]string{""}, lines[b.line+1:]...)...)
	b.value = strings.Join(lines, "\n")
	b.line++
	b.column = 0
	b.markDirty()
}

// modeLabel returns "NORMAL"/"INSERT" for the status line, or "" when vim mode
// is off.
func (b *vimBuffer) modeLabel() string {
	if !b.vimMode {
		return ""
	}
	if b.vimInsert {
		return "INSERT"
	}
	return "NORMAL"
}

func wrapEditorLine(line string, width int) []string {
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

func wrapEditorCursor(parts []string, width, col int, cursor string) []string {
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

func isEditorWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func isVimToggleKey(msg tea.KeyMsg) bool {
	return msg.String() == "ctrl+]"
}
