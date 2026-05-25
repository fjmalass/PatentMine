// Package pane holds the TUI's screens. Each screen is a Pane that owns only
// its own state; the App owns the pane stack. Adding a screen means adding a
// Pane type — never extending a shared "model" struct. This decomposition is
// the structural defence against the god-object that sank the prior attempt.
package pane

import (
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/text"
	"patentmine/internal/tui/render"
)

// ResizeMsg reports the body area available to a pane after the app reserves
// its header and status lines.
type ResizeMsg struct {
	Width  int
	Height int
}

// ProjectChangedMsg reports that the app's active project changed.
type ProjectChangedMsg struct {
	Project *domain.Project
}

// Pane is one screen of the TUI.
type Pane interface {
	// Scope reports the keymap scope the pane uses, so the App can pick
	// the right key bindings while this pane is focused.
	Scope() command.Scope
	// Title is shown in the header bar.
	Title() string
	// Init returns a command to run when the pane is first shown.
	Init() tea.Cmd
	// Command applies a resolved command intent forwarded by the App.
	Command(id command.ID, inv Invocation) (Pane, tea.Cmd)
	// Handles reports every command ID the pane services. The App's wiring
	// check cross-references it against the keymap so a key can never resolve
	// to a command the focused pane silently drops.
	Handles() []command.ID
	// Update applies a non-command message: an rpc result, a daemon event, or
	// a resize.
	Update(msg tea.Msg) (Pane, tea.Cmd)
	// View renders the pane body into w columns by h rows.
	View(w, h int) string
	// Selection reports the highlighted patent, when the pane has one. The App
	// uses it to open detail/citation views for the current row.
	Selection() (domain.PatentNumber, bool)
}

// MultiSelector is implemented by panes that support visual range selection.
// App checks for this interface before falling back to single Selection().
type MultiSelector interface {
	Selections() []domain.PatentNumber
}

// ActivityFocus is the replayable thing the user is currently looking at.
type ActivityFocus struct {
	Entity   string
	EntityID string
	Label    string
	Metadata map[string]any
}

// ActivityFocusProvider exposes richer focus metadata than Selection.
type ActivityFocusProvider interface {
	ActivityFocus() (ActivityFocus, bool)
}

// VisualSelectionSaver is implemented by panes that support saving their
// visual selection for gv restore. The App calls this when a visual selection
// is consumed by an action (e.g. review state change).
type VisualSelectionSaver interface {
	SaveVisualSelection()
}

// KeyHandler is implemented by panes that need to intercept raw key events
// before keymap resolution — for example when an inline input bar is active.
// The App checks this interface before feeding the key to the chord reader.
type KeyHandler interface {
	HandleKey(msg tea.KeyMsg) (Pane, tea.Cmd, bool)
}

// JumpProvider is implemented by panes that support jump mode — a quick scroll
// straight to a labelled field in a long, scrolling pane. The App opens the
// jump overlay from JumpAnchors and calls JumpTo with the chosen line.
type JumpProvider interface {
	// JumpAnchors returns the pane's current jump targets, in display order.
	JumpAnchors() []render.JumpAnchor
	// JumpTo scrolls the pane so the given body line leads the view.
	JumpTo(line int)
}

// ClassificationFilterTarget is implemented by panes that can be filtered by
// a set of classification codes. The classification overlay sends its marked
// codes here on close so the underlying pane reloads with the corresponding
// filter applied.
type ClassificationFilterTarget interface {
	ApplyClassificationFilter(codes []string) tea.Cmd
}

// Invocation carries a resolved command's repeat count and any typed arguments.
type Invocation struct {
	Repeat int
	Args   []string
}

// cmdHandler carries out one command for a pane. The pane mutates itself
// through its pointer receiver and returns only a tea.Cmd.
type cmdHandler func(Invocation) tea.Cmd

// handlerIDs returns the command IDs of a handler table, sorted for stable
// output in the wiring check and help screen.
func handlerIDs(handlers map[command.ID]cmdHandler) []command.ID {
	ids := make([]command.ID, 0, len(handlers))
	for id := range handlers {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// StatusMsg asks the App to show a line of status text. Panes emit a text key
// plus arguments rather than a resolved string, so the App resolves it through
// the active locale catalog and status appears in one consistent place.
type StatusMsg struct {
	Key   text.Key
	Args  []any
	Error bool
}

// ReviewStateChangedMsg reports a committed review-state change so visible panes
// can update immediately instead of waiting for the debounced db.changed event.
type ReviewStateChangedMsg struct {
	Project domain.ProjectID
	Patents []domain.PatentNumber
	State   domain.ReviewState
}

// IDSEntryChangedMsg reports a committed change to a patent's curated IDS entry.
type IDSEntryChangedMsg struct {
	Project domain.ProjectID
	Patent  domain.PatentNumber
	Entry   *domain.IDSEntry // nil if the entry was deleted
}

// IDSEntriesChangedMsg reports a committed batch IDS status update.
type IDSEntriesChangedMsg struct {
	Entries []domain.IDSEntry
	Err     error
}

// MultiCrawlStartedMsg is emitted when multiple patents are selected and a
// crawl or lookup is started for each of them. It carries all job IDs so the
// app can show a single aggregate overlay instead of stacking one per job.
type MultiCrawlStartedMsg struct {
	Numbers []domain.PatentNumber
	JobIDs  []string
	Depth   int
}

// CopyToClipboardMsg asks the app to copy the given text to the system clipboard.
type CopyToClipboardMsg struct {
	Text string
}

// NoteAddMsg tells the app to add a selected passage to the notes buffer.
type NoteAddMsg struct {
	Number     domain.PatentNumber
	Locator    string
	Text       string
	CapturedAt time.Time
}

// NoteOpenMsg tells the app to show the notes buffer overlay for a patent.
type NoteOpenMsg struct {
	Number domain.PatentNumber
	Patent domain.Patent
}

// PatentNoteOpenMsg tells the app to show the persistent project note editor.
type PatentNoteOpenMsg struct {
	Number domain.PatentNumber
}

// FullTextLoadedMsg delivers the result of fetching full patent claims text.
type FullTextLoadedMsg struct {
	RequestID uint64
	Number    domain.PatentNumber
	FullText  *domain.FullText
	Patent    domain.Patent
	Duration  time.Duration
	Err       error
}

// status returns a tea.Cmd that emits a StatusMsg for key.
func status(key text.Key, isErr bool, args ...any) tea.Cmd {
	return func() tea.Msg { return StatusMsg{Key: key, Args: args, Error: isErr} }
}

// ServiceStatusChangedMsg broadcasts active capabilities to all active panes.
type ServiceStatusChangedMsg struct {
	ActiveAI     string
	ActiveSearch string
}

// PaneHandlers returns a map of all handled command IDs per scope.
// It is used by the TUI wiring validator to remain decoupled from individual
// pane constructors.
func PaneHandlers() map[command.Scope][]command.ID {
	theme := render.NewTheme()
	panes := []Pane{
		NewCatalog(nil, theme),
		NewDetail(nil, theme, domain.PatentNumber{}, "", nil),
		NewCitations(nil, theme, domain.PatentNumber{}, domain.RelationCites),
		NewFamilyGraph(nil, theme, domain.PatentNumber{}, 0, nil),
		NewIDSDetail(nil, theme, domain.PatentNumber{}, ""),
		NewProjects(nil, theme, "", ""),
		NewFullText(nil, theme, domain.PatentNumber{}, "", nil),
		NewAllNotes(nil, theme, nil),
	}
	out := make(map[command.Scope][]command.ID, len(panes))
	for _, p := range panes {
		out[p.Scope()] = p.Handles()
	}
	return out
}

// SearchAppliedMsg reports that a search query has been confirmed and applied by a pane.
type SearchAppliedMsg struct {
	Query string
}
