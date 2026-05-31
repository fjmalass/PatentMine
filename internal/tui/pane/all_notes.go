package pane

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

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

	notes      []domain.PatentNote
	page       render.Paginator
	sortByDate bool
	loading    bool
	loadErr    string
	loadID     uint64
	logger     *slog.Logger
	metrics    *observability.Metrics
	exportDir  string // directory for exported .md files; empty = user home dir
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
		sortByDate:    true,
		loading:       true,
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
	sortByDate := a.sortByDate
	requestID := nextAsyncID()
	a.loadID = requestID
	return func() tea.Msg {
		ctx, cancel := callContext()
		defer cancel()
		sortBy := proto.NoteSortByDate
		if !sortByDate {
			sortBy = proto.NoteSortByPatent
		}
		var res proto.PatentNoteListResult
		t0 := time.Now()
		err := client.Call(ctx, proto.MethodPatentNoteList,
			proto.PatentNoteListParams{Project: project, SortBy: sortBy}, &res)
		return allNotesLoadedMsg{requestID: requestID, notes: res.Notes, err: err, loadDuration: time.Since(t0)}
	}
}

func (a *AllNotes) toggleSort() tea.Cmd {
	a.sortByDate = !a.sortByDate
	a.loading = true
	label := "patent number"
	if a.sortByDate {
		label = "date"
	}
	return tea.Batch(
		a.load(),
		func() tea.Msg { return StatusMsg{Key: text.StatusNotesSorted, Args: []any{label}} },
	)
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
	sortByDate := a.sortByDate
	exportDir := a.exportDir
	client := a.client
	var projectID domain.ProjectID
	var projectName string
	if a.activeProject != nil {
		projectID = a.activeProject.ID
		projectName = a.activeProject.Name
	}
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
		if !sortByDate {
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
		a.notes = m.notes
		a.page.SetTotal(len(a.notes))
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

const notesSnippetLen = 60

// View implements Pane.
func (a *AllNotes) View(w, h int) string {
	if a.loading && len(a.notes) == 0 {
		return a.theme.Dim.Render("loading notes…")
	}
	if a.loadErr != "" {
		return a.theme.Error.Render("error: " + a.loadErr)
	}
	if len(a.notes) == 0 {
		if a.activeProject == nil {
			return a.theme.Dim.Render("no active project")
		}
		return a.theme.Dim.Render("no notes for this project — open a patent and press N")
	}

	a.page.SetPageSize(max(h-headerRows, 1))

	sortLabel := "date"
	if !a.sortByDate {
		sortLabel = "patent"
	}

	var b strings.Builder
	b.WriteString(renderTableStatusLine(a.theme, w, a.page.Cursor(), a.page.Total(), "sort:"+sortLabel))
	b.WriteByte('\n')
	b.WriteString(a.theme.Header.Render(notesHeaderRow(w)))

	start, end := a.page.Window()
	for i := start; i < end; i++ {
		note := a.notes[i]
		line := notesRow(formatViewIndex(i), note, w)
		b.WriteByte('\n')
		if i == a.page.Cursor() {
			b.WriteString(a.theme.Selected.Render(render.Pad(line, w)))
		} else {
			b.WriteString(tableRowStyle(a.theme, i)(render.Pad(line, w)))
		}
	}
	b.WriteByte('\n')
	b.WriteString(a.theme.Dim.Render("  [s] sort  [e] export .md  [enter/N] open note  [esc] back"))
	return b.String()
}

func notesHeaderRow(w int) string {
	const numW = 6
	const dateW = 12
	patent := "PATENT"
	dateCol := "UPDATED"
	snippet := "NOTE"
	rest := w - numW - dateW - len(patent) - 4
	if rest < len(snippet) {
		rest = len(snippet)
	}
	return fmt.Sprintf("%-*s  %-20s  %-*s  %s", numW, "#", patent, rest, snippet, dateCol)
}

func notesRow(idx string, note domain.PatentNote, w int) string {
	const numW = 6
	const dateW = 12
	patentStr := note.Patent.String()
	dateStr := note.UpdatedAt.Format(domain.DateLayout)
	snippet := firstLine(strings.TrimSpace(note.Markdown))
	snippetMax := w - numW - len(patentStr) - dateW - 6
	if snippetMax < 0 {
		snippetMax = 0
	}
	if utf8.RuneCountInString(snippet) > snippetMax {
		runes := []rune(snippet)
		if snippetMax > 3 {
			snippet = string(runes[:snippetMax-1]) + "…"
		} else {
			snippet = string(runes[:snippetMax])
		}
	}
	return fmt.Sprintf("%-*s  %-20s  %-*s  %s", numW, idx, patentStr, snippetMax, snippet, dateStr)
}

func firstLine(s string) string {
	s = strings.TrimPrefix(s, "#")
	s = strings.TrimSpace(s)
	if nl := strings.IndexByte(s, '\n'); nl >= 0 {
		s = s[:nl]
	}
	return strings.TrimSpace(s)
}
