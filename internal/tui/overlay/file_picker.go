package overlay

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/tui/render"
)

// PurposeAddOfficeAction routes a picked file to the :add.officeaction import.
const PurposeAddOfficeAction Purpose = "add-officeaction"

// PurposeAddMatterDocument routes a picked file to the :add.document import (a
// supporting document — reference, response, etc. — filed under the matter).
const PurposeAddMatterDocument Purpose = "add-matter-document"

// FilePickedMsg reports a file chosen in a FilePicker overlay. Purpose tells the
// App which action to route the path to.
type FilePickedMsg struct {
	Purpose Purpose
	Path    string
}

// fileEntry is one row of the browser: a directory or a selectable file.
type fileEntry struct {
	name  string
	isDir bool
}

// FilePicker is a modal directory browser over the local filesystem. It is a
// KeyHandler overlay and emits a FilePickedMsg when a file is chosen (or
// CloseOverlayMsg on Esc). It is deliberately hand-rolled rather than using
// bubbles/filepicker, which panics when an entry's os.FileInfo cannot be
// stat-ed (a broken symlink or a file that vanished mid-read); this lister keys
// off DirEntry.IsDir() and never stats, so it cannot panic on a bad entry.
//
// The path is local to this machine, which is exactly what the daemon needs: it
// shares the filesystem (unix socket) and copies the file itself, so only the
// path travels over RPC.
type FilePicker struct {
	theme   render.Theme
	title   string
	purpose Purpose
	allowed []string // selectable file extensions (lowercase, with dot); empty = any

	dir     string
	entries []fileEntry
	cursor  int
	offset  int
	err     string
}

// NewFilePicker builds a browser rooted at startDir. allowedTypes restricts
// which file extensions are selectable (directories are always shown); empty
// allows any file.
func NewFilePicker(theme render.Theme, title string, purpose Purpose, startDir string, allowedTypes []string) *FilePicker {
	dir := startDir
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	lower := make([]string, 0, len(allowedTypes))
	for _, t := range allowedTypes {
		lower = append(lower, strings.ToLower(t))
	}
	o := &FilePicker{theme: theme, title: title, purpose: purpose, allowed: lower, dir: dir}
	o.load()
	return o
}

func (o *FilePicker) Title() string { return o.title }

// Handles returns nil: the picker consumes its keys via HandleKey and binds no
// catalog commands, so it needs no entry in the wiring validator.
func (o *FilePicker) Handles() []command.ID { return nil }

func (o *FilePicker) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

// Init satisfies the app's overlay-open path; the directory is read eagerly in
// NewFilePicker, so there is nothing async to start.
func (o *FilePicker) Init() tea.Cmd { return nil }

// load reads the current directory. Reading keys off DirEntry only (no stat), so
// a broken symlink or a vanished file cannot crash the browser.
func (o *FilePicker) load() {
	o.cursor, o.offset, o.err = 0, 0, ""
	entries, err := os.ReadDir(o.dir)
	if err != nil {
		o.err = err.Error()
		o.entries = []fileEntry{{name: "..", isDir: true}}
		return
	}
	var dirs, files []fileEntry
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue // hide dotfiles
		}
		if e.IsDir() {
			dirs = append(dirs, fileEntry{name: name, isDir: true})
		} else if o.allows(name) {
			files = append(files, fileEntry{name: name})
		}
	}
	byName := func(s []fileEntry) {
		sort.Slice(s, func(i, j int) bool {
			return strings.ToLower(s[i].name) < strings.ToLower(s[j].name)
		})
	}
	byName(dirs)
	byName(files)
	o.entries = append([]fileEntry{{name: "..", isDir: true}}, append(dirs, files...)...)
}

func (o *FilePicker) allows(name string) bool {
	if len(o.allowed) == 0 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	for _, a := range o.allowed {
		if ext == a {
			return true
		}
	}
	return false
}

func (o *FilePicker) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyEsc:
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case tea.KeyUp:
		o.move(-1)
	case tea.KeyDown:
		o.move(1)
	case tea.KeyEnter, tea.KeyRight:
		return o, o.activate(), true
	case tea.KeyBackspace, tea.KeyLeft:
		o.ascend()
	case tea.KeyRunes:
		switch msg.String() {
		case "k":
			o.move(-1)
		case "j":
			o.move(1)
		case "h":
			o.ascend()
		case "l":
			return o, o.activate(), true
		case "q":
			return o, func() tea.Msg { return CloseOverlayMsg{} }, true
		}
	}
	return o, nil, true
}

func (o *FilePicker) move(delta int) {
	if len(o.entries) == 0 {
		return
	}
	o.cursor = max(0, min(o.cursor+delta, len(o.entries)-1))
}

// ascend moves to the parent directory.
func (o *FilePicker) ascend() {
	o.dir = filepath.Dir(o.dir)
	o.load()
}

// activate enters the selected directory or returns the command emitting the
// selected file.
func (o *FilePicker) activate() tea.Cmd {
	if o.cursor < 0 || o.cursor >= len(o.entries) {
		return nil
	}
	e := o.entries[o.cursor]
	if e.isDir {
		if e.name == ".." {
			o.ascend()
		} else {
			o.dir = filepath.Join(o.dir, e.name)
			o.load()
		}
		return nil
	}
	path := filepath.Join(o.dir, e.name)
	purpose := o.purpose
	return func() tea.Msg { return FilePickedMsg{Purpose: purpose, Path: path} }
}

func (o *FilePicker) View(maxW, maxH int) string {
	var b strings.Builder
	b.WriteString(o.theme.Dim.Render(render.Truncate("dir: "+o.dir, maxW)))
	b.WriteByte('\n')
	if o.err != "" {
		b.WriteString(o.theme.Warn.Render(render.Truncate(o.err, maxW)))
		b.WriteByte('\n')
	}

	bodyRows := max(maxH-3, 1)
	o.scrollInto(bodyRows)
	for i := o.offset; i < len(o.entries) && i < o.offset+bodyRows; i++ {
		e := o.entries[i]
		label := e.name
		if e.isDir && e.name != ".." {
			label += "/"
		}
		cell := render.Pad(render.Truncate(label, maxW), maxW)
		if i == o.cursor {
			b.WriteString(o.theme.Selected.Render(cell))
		} else if e.isDir {
			b.WriteString(o.theme.Dim.Render(cell))
		} else {
			b.WriteString(o.theme.Row.Render(cell))
		}
		b.WriteByte('\n')
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(
		"↑/↓ or j/k move · enter open/select · ←/backspace up · esc cancel", maxW)))
	return b.String()
}

func (o *FilePicker) scrollInto(bodyRows int) {
	if o.cursor < o.offset {
		o.offset = o.cursor
	}
	if o.cursor >= o.offset+bodyRows {
		o.offset = o.cursor - bodyRows + 1
	}
	if o.offset < 0 {
		o.offset = 0
	}
}
