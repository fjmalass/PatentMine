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

// PurposeAddPatentList routes a picked file to the :add.file import (a
// plain-text list of patent numbers bulk-added into the active project).
const PurposeAddPatentList Purpose = "add-patent-list"

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

	dir        string
	rawEntries []fileEntry
	entries    []fileEntry
	cursor     int
	offset     int
	err        string

	showSearch  bool
	searchQuery string
	jump        JumpNavigator
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

func (o *FilePicker) Dir() string { return o.dir }

func (o *FilePicker) Purpose() Purpose { return o.purpose }

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
		o.rawEntries = []fileEntry{{name: "..", isDir: true}}
		o.filter()
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
	o.rawEntries = append([]fileEntry{{name: "..", isDir: true}}, append(dirs, files...)...)
	o.filter()
}

func (o *FilePicker) filter() {
	if o.searchQuery == "" {
		o.entries = o.rawEntries
	} else {
		o.entries = nil
		q := strings.ToLower(o.searchQuery)
		for _, e := range o.rawEntries {
			if e.name == ".." {
				o.entries = append(o.entries, e)
				continue
			}
			if strings.Contains(strings.ToLower(e.name), q) {
				o.entries = append(o.entries, e)
			}
		}
	}
	if o.cursor >= len(o.entries) {
		o.cursor = max(len(o.entries)-1, 0)
	}
}

func (o *FilePicker) activateQuery() (tea.Cmd, bool) {
	cleanedQuery := strings.TrimSpace(o.searchQuery)
	if cleanedQuery == "" {
		return nil, false
	}
	path := cleanedQuery
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[1:])
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(o.dir, path)
	}

	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			o.dir = path
			o.searchQuery = ""
			o.showSearch = false
			o.load()
			return nil, true
		} else {
			if o.allows(path) {
				purpose := o.purpose
				return func() tea.Msg { return FilePickedMsg{Purpose: purpose, Path: path} }, true
			}
		}
	}
	return nil, false
}

func (o *FilePicker) complete() {
	if o.searchQuery == "" {
		return
	}

	// 0. If the query is already a valid directory, navigate into it directly!
	queryPath := strings.TrimSpace(o.searchQuery)
	if strings.HasPrefix(queryPath, "~") {
		home, _ := os.UserHomeDir()
		queryPath = filepath.Join(home, queryPath[1:])
	}
	if !filepath.IsAbs(queryPath) {
		queryPath = filepath.Join(o.dir, queryPath)
	}
	if info, err := os.Stat(queryPath); err == nil && info.IsDir() {
		if abs, err := filepath.Abs(queryPath); err == nil {
			o.dir = abs
		} else {
			o.dir = queryPath
		}
		o.searchQuery = ""
		o.load()
		return
	}

	hasSep := strings.ContainsRune(o.searchQuery, '/') || strings.ContainsRune(o.searchQuery, filepath.Separator)

	if !hasSep {
		q := strings.ToLower(o.searchQuery)
		var matches []fileEntry
		for _, e := range o.rawEntries {
			if e.name == ".." {
				continue
			}
			if strings.HasPrefix(strings.ToLower(e.name), q) {
				matches = append(matches, e)
			}
		}

		if len(matches) == 1 {
			if matches[0].isDir {
				o.dir = filepath.Join(o.dir, matches[0].name)
				o.searchQuery = ""
				o.load()
			} else {
				o.searchQuery = matches[0].name
				o.filter()
			}
		} else if len(matches) > 1 {
			var names []string
			for _, m := range matches {
				names = append(names, m.name)
			}
			lcp := longestCommonPrefix(names)
			if lcp != "" {
				o.searchQuery = lcp
				o.filter()
			}
		}
		return
	}

	dirPart := filepath.Dir(o.searchQuery)
	filePart := filepath.Base(o.searchQuery)
	if strings.HasSuffix(o.searchQuery, "/") || strings.HasSuffix(o.searchQuery, string(filepath.Separator)) {
		dirPart = o.searchQuery
		filePart = ""
	}

	resolvedDir := dirPart
	if strings.HasPrefix(resolvedDir, "~") {
		home, _ := os.UserHomeDir()
		resolvedDir = filepath.Join(home, resolvedDir[1:])
	}
	if !filepath.IsAbs(resolvedDir) {
		resolvedDir = filepath.Join(o.dir, resolvedDir)
	}

	entries, err := os.ReadDir(resolvedDir)
	if err != nil {
		return
	}

	var matches []os.DirEntry
	q := strings.ToLower(filePart)
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if strings.HasPrefix(strings.ToLower(name), q) {
			matches = append(matches, e)
		}
	}

	if len(matches) == 1 {
		name := matches[0].Name()
		if matches[0].IsDir() {
			path := filepath.Join(dirPart, name)
			if strings.HasPrefix(path, "~") {
				home, _ := os.UserHomeDir()
				path = filepath.Join(home, path[1:])
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(o.dir, path)
			}
			if abs, err := filepath.Abs(path); err == nil {
				o.dir = abs
			} else {
				o.dir = path
			}
			o.searchQuery = ""
			o.load()
		} else {
			if strings.HasSuffix(dirPart, "/") || strings.HasSuffix(dirPart, string(filepath.Separator)) {
				o.searchQuery = dirPart + name
			} else {
				o.searchQuery = dirPart + "/" + name
			}
			o.filter()
		}
	} else if len(matches) > 1 {
		var names []string
		for _, m := range matches {
			names = append(names, m.Name())
		}
		lcp := longestCommonPrefix(names)
		if lcp != "" {
			if strings.HasSuffix(dirPart, "/") || strings.HasSuffix(dirPart, string(filepath.Separator)) {
				o.searchQuery = dirPart + lcp
			} else {
				o.searchQuery = dirPart + "/" + lcp
			}
			o.filter()
		}
	}
}

func longestCommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	first := strs[0]
	for i := 0; i < len(first); i++ {
		c := first[i]
		for j := 1; j < len(strs); j++ {
			if i >= len(strs[j]) || strings.ToLower(string(strs[j][i])) != strings.ToLower(string(c)) {
				return first[:i]
			}
		}
	}
	return first
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
	if !o.showSearch {
		if newCursor, handled := o.jump.HandleKey(msg, o.cursor, len(o.entries)); handled {
			o.cursor = newCursor
			return o, nil, true
		}
	}
	if o.showSearch {
		switch msg.Type {
		case tea.KeyEsc:
			o.showSearch = false
			o.searchQuery = ""
			o.filter()
			return o, nil, true
		case tea.KeyTab:
			o.complete()
			return o, nil, true
		case tea.KeyEnter:
			if cmd, handled := o.activateQuery(); handled {
				return o, cmd, true
			}
			return o, o.activate(), true
		case tea.KeyBackspace:
			if len(o.searchQuery) > 0 {
				o.searchQuery = o.searchQuery[:len(o.searchQuery)-1]
				o.filter()
			} else {
				o.showSearch = false
			}
			return o, nil, true
		case tea.KeyUp:
			o.move(-1)
			return o, nil, true
		case tea.KeyDown:
			o.move(1)
			return o, nil, true
		case tea.KeyRunes:
			o.searchQuery += msg.String()
			o.filter()
			return o, nil, true
		default:
			return o, nil, true
		}
	}

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
		case ";":
			if !o.showSearch {
				o.jump.Active = true
				o.jump.PendingCount = 0
				o.jump.PendingG = false
				return o, nil, true
			}
		case "/":
			o.showSearch = true
			o.searchQuery = ""
			return o, nil, true
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
	if o.showSearch {
		b.WriteString(o.theme.Selected.Render(render.Truncate("search/path: "+o.searchQuery+"█", maxW)))
		b.WriteByte('\n')
	}
	if o.err != "" {
		b.WriteString(o.theme.Warn.Render(render.Truncate(o.err, maxW)))
		b.WriteByte('\n')
	}

	headerLines := 2
	if o.showSearch {
		headerLines++
	}
	if o.err != "" {
		headerLines++
	}
	bodyRows := max(maxH-headerLines-1, 1)

	o.scrollInto(bodyRows)
	for i := o.offset; i < len(o.entries) && i < o.offset+bodyRows; i++ {
		e := o.entries[i]
		label := e.name
		if e.isDir && e.name != ".." {
			label += "/"
		}
		lineNum := i + 1
		gutter := o.jump.GutterPrefix(lineNum)
		label = gutter + label
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

	footerText := "/ search/path · ↑/↓ or j/k move · enter open/select · ←/backspace up · esc cancel · [;] jump mode"
	if o.showSearch {
		footerText = "tab autocomplete · esc exit search · enter open/select · backspace edit"
	} else if o.jump.Active {
		footerText = o.jump.HintSuffix(o.cursor, -1, false)
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(footerText, maxW)))
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
