package pane

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

// allNotesLoadedMsg delivers a finished patent.note.list result.
type allNotesLoadedMsg struct {
	requestID    uint64
	notes        []domain.PatentNote
	err          error
	loadDuration time.Duration
}

// ExportNotesPreparedMsg asks the app to show the export confirm overlay. The
// WriteCmd performs the actual file write when the user confirms.
type ExportNotesPreparedMsg struct {
	Path          string
	ExistingFiles []string
	WriteCmd      tea.Cmd
}

// AllNotes lists every patent note for the active project, with sort and
// export-to-file support.
type AllNotes struct {
	client        *rpc.Client
	theme         render.Theme
	activeProject *domain.Project
	handlers      map[command.ID]cmdHandler

	allNotes      []domain.PatentNote
	notes         []domain.PatentNote
	page          render.Paginator
	loading       bool
	loadErr       string
	loadID        uint64
	logger        *slog.Logger
	metrics       *observability.Metrics
	exportDir     string // directory for exported .md files; empty = user home dir

	searchActive  bool
	searchQuery   string
	searchScope   int
	focusedColIdx int
	sortAscending bool
	activeSort    string
}

func (a *AllNotes) log() *slog.Logger {
	if a.logger != nil {
		return a.logger
	}
	return slog.Default()
}

// NewAllNotes builds the all-notes pane for a project.
func NewAllNotes(client *rpc.Client, theme render.Theme, project *domain.Project) *AllNotes {
	a := &AllNotes{
		client:        client,
		theme:         theme,
		activeProject: project,
		page:          render.NewPaginator(20),
		loading:       true,
		focusedColIdx: -1,
		sortAscending: true,
		activeSort:    "date",
		searchScope:   0,
	}
	a.handlers = map[command.ID]cmdHandler{
		command.NavDown:         func(inv Invocation) tea.Cmd { a.page.MoveDown(inv.Repeat); return nil },
		command.NavUp:           func(inv Invocation) tea.Cmd { a.page.MoveUp(inv.Repeat); return nil },
		command.NavPageDown:     func(Invocation) tea.Cmd { a.page.PageDown(); return nil },
		command.NavPageUp:       func(Invocation) tea.Cmd { a.page.PageUp(); return nil },
		command.NavTop:          func(inv Invocation) tea.Cmd { a.page.NavTop(inv.Repeat); return nil },
		command.NavBottom:       func(inv Invocation) tea.Cmd { a.page.NavBottom(inv.Repeat); return nil },
		command.Refresh:         func(Invocation) tea.Cmd { a.loading = true; return a.load() },
		command.NotesSortToggle: func(Invocation) tea.Cmd { return a.toggleSort() },
		command.NotesExportMD:   func(Invocation) tea.Cmd { return a.exportMD() },
		command.OpenPatentNote:  func(Invocation) tea.Cmd { return a.openSelectedNote() },
		command.ColNext:         func(Invocation) tea.Cmd { return a.focusNext() },
		command.ColPrev:         func(Invocation) tea.Cmd { return a.focusPrev() },
		command.SortApply:       func(Invocation) tea.Cmd { return a.applySort() },
		command.FindOpen: func(Invocation) tea.Cmd {
			a.searchActive = true
			a.searchQuery = ""
			a.applyFilter()
			return nil
		},
	}
	return a
}

// WithExportDir sets the directory for exported .md files. Empty falls back to
// the user's home directory.
func (a *AllNotes) WithExportDir(dir string) *AllNotes { a.exportDir = dir; return a }

// WithMetrics attaches the metrics sink.
func (a *AllNotes) WithMetrics(m *observability.Metrics) *AllNotes { a.metrics = m; return a }

func (a *AllNotes) Scope() command.Scope { return command.ScopeNotes }

func (a *AllNotes) Title() string {
	if a.activeProject != nil {
		return "Notes · " + a.activeProject.Name
	}
	return "Notes"
}

func (a *AllNotes) Init() tea.Cmd { return a.load() }

func (a *AllNotes) load() tea.Cmd {
	if a.client == nil || a.activeProject == nil {
		a.loading = false
		return nil
	}
	client := a.client
	project := a.activeProject.ID
	requestID := nextAsyncID()
	a.loadID = requestID
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.PatentNoteListResult
		t0 := time.Now()
		err := client.Call(ctx, proto.MethodPatentNoteList,
			proto.PatentNoteListParams{Project: project, SortBy: proto.NoteSortByDate}, &res)
		return allNotesLoadedMsg{requestID: requestID, notes: res.Notes, err: err, loadDuration: time.Since(t0)}
	}
}

func (a *AllNotes) focusNext() tea.Cmd {
	a.focusedColIdx = (a.focusedColIdx + 1) % 3
	return nil
}

func (a *AllNotes) focusPrev() tea.Cmd {
	a.focusedColIdx = (a.focusedColIdx - 1 + 3) % 3
	return nil
}

func (a *AllNotes) applySort() tea.Cmd {
	if a.focusedColIdx < 0 {
		return nil
	}
	cols := a.currentCols(80)
	col := cols[a.focusedColIdx]
	if col.SortKey == "" {
		return nil
	}
	if a.activeSort == col.SortKey {
		a.sortAscending = !a.sortAscending
	} else {
		a.activeSort = col.SortKey
		a.sortAscending = true
	}
	a.applyFilter()
	return nil
}

func (a *AllNotes) toggleSort() tea.Cmd {
	if a.activeSort == "date" {
		a.activeSort = "patent"
	} else {
		a.activeSort = "date"
	}
	a.applyFilter()
	label := "patent number"
	if a.activeSort == "date" {
		label = "date"
	}
	return func() tea.Msg { return StatusMsg{Key: text.StatusNotesSorted, Args: []any{label}} }
}

func (a *AllNotes) openSelectedNote() tea.Cmd {
	cur := a.page.Cursor()
	if cur < 0 || cur >= len(a.notes) {
		return nil
	}
	return func() tea.Msg {
		return PatentNoteOpenMsg{Number: a.notes[cur].Patent}
	}
}

func (a *AllNotes) exportMD() tea.Cmd {
	if len(a.notes) == 0 {
		return status(text.StatusNotesExportFailed, true, "no notes to export")
	}
	exportDir := a.exportDir
	client := a.client
	var projectID domain.ProjectID
	var projectName string
	if a.activeProject != nil {
		projectID = a.activeProject.ID
		projectName = a.activeProject.Name
	}
	activeSort := a.activeSort
	return func() tea.Msg {
		dir := exportDir
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				home = "."
			}
			dir = home
		}
		dateStr := time.Now().Format(domain.DateLayout)
		safeName := strings.NewReplacer(" ", "-", "/", "-", "\\", "-").Replace(projectName)
		filename := fmt.Sprintf("patentmine-notes-%s-%s.md", safeName, dateStr)
		path := filepath.Join(dir, filename)
		existing := scanExistingExports(dir)
		sortBy := proto.NoteSortByDate
		if activeSort == "patent" {
			sortBy = proto.NoteSortByPatent
		}
		writeCmd := buildExportRPCCmd(client, projectID, sortBy, path)
		return ExportNotesPreparedMsg{Path: path, ExistingFiles: existing, WriteCmd: writeCmd}
	}
}

// scanExistingExports returns all patentmine-notes-*.md filenames in dir.
func scanExistingExports(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "patentmine-notes-") && strings.HasSuffix(name, ".md") {
			out = append(out, name)
		}
	}
	return out
}

// buildExportRPCCmd returns the tea.Cmd that delegates export to the daemon.
func buildExportRPCCmd(client *rpc.Client, project domain.ProjectID, sortBy proto.NoteSortBy, path string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		var res proto.PatentNoteExportResult
		err := client.Call(ctx, proto.MethodPatentNoteExport,
			proto.PatentNoteExportParams{Project: project, SortBy: sortBy, OutputPath: path}, &res)
		if err != nil {
			return StatusMsg{Key: text.StatusNotesExportFailed, Args: []any{err.Error()}, Error: true}
		}
		return StatusMsg{Key: text.StatusNotesExportDone, Args: []any{res.Count, res.Path}}
	}
}

// Command implements Pane.
func (a *AllNotes) Command(id command.ID, inv Invocation) (Pane, tea.Cmd) {
	if handler, ok := a.handlers[id]; ok {
		return a, handler(inv)
	}
	return a, nil
}

// Handles implements Pane.
func (a *AllNotes) Handles() []command.ID { return handlerIDs(a.handlers) }

// Update implements Pane.
func (a *AllNotes) Update(msg tea.Msg) (Pane, tea.Cmd) {
	switch m := msg.(type) {
	case allNotesLoadedMsg:
		if m.requestID != a.loadID {
			return a, nil
		}
		a.loading = false
		if m.err != nil {
			a.loadErr = m.err.Error()
			a.metrics.IncCounter("tui.all_notes.load.error", 1)
			a.log().Error("all notes load failed",
				slog.String("error", m.err.Error()),
				slog.Int64("duration_ms", m.loadDuration.Milliseconds()))
			return a, nil
		}
		a.loadErr = ""
		a.allNotes = m.notes
		a.applyFilter()
		a.metrics.ObserveDuration("tui.all_notes.load", m.loadDuration, false)
		a.log().Info("all notes loaded",
			slog.Int("count", len(m.notes)),
			slog.Int64("duration_ms", m.loadDuration.Milliseconds()))
		return a, nil
	case ProjectChangedMsg:
		a.activeProject = cloneProject(m.Project)
		a.loading = true
		return a, a.load()
	}
	return a, nil
}

// Selection implements Pane: no patent selection from this pane.
func (a *AllNotes) Selection() (domain.PatentNumber, bool) {
	cur := a.page.Cursor()
	if cur >= 0 && cur < len(a.notes) {
		return a.notes[cur].Patent, true
	}
	return domain.PatentNumber{}, false
}

var notesSearchScopes = []struct {
	Key   string
	Label string
}{
	{"all", "All Columns"},
	{"patent", "Patent Number"},
	{"note", "Note Content"},
}

func (a *AllNotes) applyFilter() {
	if a.searchQuery == "" {
		a.notes = a.allNotes
	} else {
		a.notes = nil
		q := strings.ToLower(a.searchQuery)
		scope := notesSearchScopes[a.searchScope].Key
		for _, note := range a.allNotes {
			match := false
			switch scope {
			case "all":
				match = strings.Contains(strings.ToLower(note.Patent.String()), q) ||
					strings.Contains(strings.ToLower(note.Markdown), q)
			case "patent":
				match = strings.Contains(strings.ToLower(note.Patent.String()), q)
			case "note":
				match = strings.Contains(strings.ToLower(note.Markdown), q)
			}
			if match {
				a.notes = append(a.notes, note)
			}
		}
	}
	a.sortNotes()
	a.page.SetTotal(len(a.notes))
	if a.page.Cursor() >= len(a.notes) {
		a.page.NavTop(0)
	}
}

func (a *AllNotes) sortNotes() {
	slices.SortFunc(a.notes, func(i, j domain.PatentNote) int {
		var cmp int
		switch a.activeSort {
		case "patent":
			cmp = strings.Compare(i.Patent.String(), j.Patent.String())
		case "note":
			cmp = strings.Compare(i.Markdown, j.Markdown)
		case "date":
			if i.UpdatedAt.Before(j.UpdatedAt) {
				cmp = -1
			} else if i.UpdatedAt.After(j.UpdatedAt) {
				cmp = 1
			} else {
				cmp = 0
			}
		default:
			if i.UpdatedAt.Before(j.UpdatedAt) {
				cmp = -1
			} else if i.UpdatedAt.After(j.UpdatedAt) {
				cmp = 1
			} else {
				cmp = 0
			}
		}
		if !a.sortAscending {
			cmp = -cmp
		}
		return cmp
	})
}

func (a *AllNotes) currentCols(w int) []render.TableColumn {
	noteWidth := max(10, w-20-12-6)
	return []render.TableColumn{
		{Key: "patent", Label: "PATENT", SortKey: "patent", Width: 20},
		{Key: "note", Label: "NOTE", SortKey: "note", Width: noteWidth},
		{Key: "updated", Label: "UPDATED", SortKey: "date", Width: 12},
	}
}

func (a *AllNotes) HandleKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool) {
	if a.searchActive {
		switch msg.Type {
		case tea.KeyTab:
			a.searchScope = (a.searchScope + 1) % len(notesSearchScopes)
			a.applyFilter()
			return a, nil, true
		case tea.KeyEsc:
			a.searchActive = false
			a.searchQuery = ""
			a.applyFilter()
			return a, nil, true
		case tea.KeyEnter:
			a.searchActive = false
			return a, nil, true
		case tea.KeyBackspace, tea.KeyDelete:
			if len(a.searchQuery) > 0 {
				runes := []rune(a.searchQuery)
				a.searchQuery = string(runes[:len(runes)-1])
				a.applyFilter()
			}
			return a, nil, true
		case tea.KeyCtrlW:
			a.searchQuery = ""
			a.applyFilter()
			return a, nil, true
		case tea.KeyRunes, tea.KeySpace:
			a.searchQuery += msg.String()
			a.applyFilter()
			return a, nil, true
		}
		return a, nil, true
	}
	return a, nil, false
}

// View implements Pane.
func (a *AllNotes) View(w, h int) string {
	if a.loading && len(a.allNotes) == 0 {
		return a.theme.Dim.Render("loading notes…")
	}
	if a.loadErr != "" {
		return a.theme.Error.Render("error: " + a.loadErr)
	}
	if len(a.allNotes) == 0 {
		if a.activeProject == nil {
			return a.theme.Dim.Render("no active project")
		}
		return a.theme.Dim.Render("no notes for this project — open a patent and press N")
	}

	a.page.SetPageSize(max(h-headerRows-2, 1))

	var b strings.Builder
	// Render status line
	statusText := "sort:" + a.activeSort
	if a.sortAscending {
		statusText += " (asc)"
	} else {
		statusText += " (desc)"
	}
	b.WriteString(renderTableStatusLine(a.theme, w, a.page.Cursor(), a.page.Total(), statusText))
	b.WriteByte('\n')

	cols := a.currentCols(w)
	start, end := a.page.Window()

	tableStr := render.RenderTable(render.TableParams{
		Theme:         a.theme,
		Columns:       cols,
		RowCount:      end - start,
		FocusedColIdx: a.focusedColIdx,
		ActiveSort:    a.activeSort,
		SortAscending: a.sortAscending,
		FocusActive:   true,
		IsRowCursor: func(rowIdx int) bool {
			return start+rowIdx == a.page.Cursor()
		},
	}, w, func(rowIdx, colIdx int) string {
		absIdx := start + rowIdx
		if absIdx < 0 || absIdx >= len(a.notes) {
			return ""
		}
		note := a.notes[absIdx]
		switch cols[colIdx].Key {
		case "patent":
			return note.Patent.String()
		case "note":
			return firstLine(strings.TrimSpace(note.Markdown))
		case "updated":
			return note.UpdatedAt.Format(domain.DateLayout)
		default:
			return ""
		}
	})
	b.WriteString(tableStr)

	b.WriteByte('\n')
	if a.searchActive {
		searchLine := "/ " + a.searchQuery + "▋  [Scope: " + notesSearchScopes[a.searchScope].Label + " (Tab to cycle)]"
		b.WriteString(a.theme.Selected.Render(render.Pad(searchLine, w)))
	} else {
		b.WriteString(a.theme.Dim.Render(render.Pad("  [←/→] Focus Col  [.] Sort  [/] Search  [e] export .md  [enter/N] open note  [esc] back", w)))
	}
	return b.String()
}

func firstLine(s string) string {
	s = strings.TrimPrefix(s, "#")
	s = strings.TrimSpace(s)
	if nl := strings.IndexByte(s, '\n'); nl >= 0 {
		s = s[:nl]
	}
	return strings.TrimSpace(s)
}
