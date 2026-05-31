package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/command"
	"patentmine/internal/domain"
	"patentmine/internal/proto"
	"patentmine/internal/rpc"
	"patentmine/internal/text"
	"patentmine/internal/tui/overlay"
	"patentmine/internal/tui/pane"
	"patentmine/internal/tui/render"
)

// notesTimeLayout formats note capture timestamps for display and export.
const notesTimeLayout = domain.DateTimeLayout
const notesTimeLayoutBracket = "[" + notesTimeLayout + "]"

const (
	notesOverlayWidthPct  = 60
	notesOverlayHeightPct = 60
	notesOverlayMinWidth  = 76
	notesOverlayMinHeight = 18
)

// noteEntry is one captured passage: a locator, the time it was captured, and
// the selected text.
type noteEntry struct {
	Locator    string
	CapturedAt time.Time
	Text       string
}

// notesAccumulator holds per-patent captured passages accumulated during a
// session. It is owned by the App and accessed by the full text pane.
type notesAccumulator struct {
	entries map[domain.PatentNumber][]noteEntry
}

func newNotesAccumulator() *notesAccumulator {
	return &notesAccumulator{entries: make(map[domain.PatentNumber][]noteEntry)}
}

// Add inserts a captured passage for patent number. Returns true when the
// locator is new; a repeated locator is ignored.
func (n *notesAccumulator) Add(number domain.PatentNumber, locator string, capturedAt time.Time, body string) bool {
	locator = strings.TrimSpace(locator)
	if locator == "" {
		return false
	}
	lower := strings.ToLower(locator)
	for _, e := range n.entries[number] {
		if strings.ToLower(e.Locator) == lower {
			return false // already present
		}
	}
	n.entries[number] = append(n.entries[number], noteEntry{
		Locator:    locator,
		CapturedAt: capturedAt,
		Text:       strings.TrimSpace(body),
	})
	sort.Slice(n.entries[number], func(i, j int) bool {
		return n.entries[number][i].Locator < n.entries[number][j].Locator
	})
	return true
}

// Entries returns the accumulated passages for number, sorted by locator.
func (n *notesAccumulator) Entries(number domain.PatentNumber) []noteEntry {
	return n.entries[number]
}

// Locators returns just the locator strings for number, sorted.
func (n *notesAccumulator) Locators(number domain.PatentNumber) []string {
	entries := n.entries[number]
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Locator)
	}
	return out
}

// Clear resets the buffer for number.
func (n *notesAccumulator) Clear(number domain.PatentNumber) {
	delete(n.entries, number)
}

// FlushToIDS saves the accumulated locators to the patent's IDS
// relevant_passages field.
func (n *notesAccumulator) FlushToIDS(client *rpc.Client, number domain.PatentNumber, project domain.ProjectID) tea.Cmd {
	locators := n.Locators(number)
	if len(locators) == 0 {
		return nil
	}
	passages := strings.Join(locators, "; ")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var existing proto.PatentResult
		err := client.Call(ctx, proto.MethodPatentGet,
			proto.PatentGetParams{Number: number, Project: project}, &existing)
		if err != nil {
			return pane.StatusMsg{Key: text.StatusClipboardFailed, Args: []any{"flush to IDS: " + err.Error()}, Error: true}
		}
		entry := domain.IDSEntry{
			Project:          project,
			Patent:           number,
			RelevantPassages: passages,
			Status:           domain.IDSEntryPending,
		}
		if existing.IDSEntry != nil {
			entry = *existing.IDSEntry
			if entry.RelevantPassages != "" {
				entry.RelevantPassages += "; " + passages
			} else {
				entry.RelevantPassages = passages
			}
		}
		var result proto.IDSEntryResult
		err = client.Call(ctx, proto.MethodIDSEntrySave,
			proto.IDSEntrySaveParams{Entry: entry}, &result)
		if err != nil {
			return pane.StatusMsg{Key: text.StatusClipboardFailed, Args: []any{"flush to IDS: " + err.Error()}, Error: true}
		}
		n.Clear(number)
		return pane.StatusMsg{Key: text.StatusNotesFlushed, Args: []any{number.String(), passages}}
	}
}

// SaveToPatentNote saves the accumulated entries to the persistent patent note
// (project-scoped markdown). Appends to existing note content if present.
func (n *notesAccumulator) SaveToPatentNote(client *rpc.Client, number domain.PatentNumber, project domain.ProjectID) tea.Cmd {
	entries := n.Entries(number)
	if len(entries) == 0 {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var existing proto.PatentResult
		err := client.Call(ctx, proto.MethodPatentGet,
			proto.PatentGetParams{Number: number, Project: project}, &existing)
		if err != nil {
			return pane.StatusMsg{Key: text.StatusClipboardFailed, Args: []any{"save to patent note: " + err.Error()}, Error: true}
		}
		var b strings.Builder
		b.WriteString("## Notes Buffer (" + time.Now().Format(notesTimeLayout) + ")\n\n")
		for _, e := range entries {
			stamp := ""
			if !e.CapturedAt.IsZero() {
				stamp = " " + e.CapturedAt.Format(notesTimeLayoutBracket)
			}
			b.WriteString("### • " + e.Locator + stamp + "\n")
			if e.Text != "" {
				b.WriteString(e.Text + "\n\n")
			}
		}
		markdown := b.String()
		if existing.PatentNote != nil && existing.PatentNote.Markdown != "" {
			markdown = existing.PatentNote.Markdown + "\n" + markdown
		}
		note := domain.PatentNote{
			Project:  project,
			Patent:   number,
			Markdown: markdown,
		}
		if existing.PatentNote != nil {
			note.AddedAt = existing.PatentNote.AddedAt
		}
		var result proto.PatentNoteResult
		err = client.Call(ctx, proto.MethodPatentNoteSave,
			proto.PatentNoteSaveParams{Note: note}, &result)
		if err != nil {
			return pane.StatusMsg{Key: text.StatusClipboardFailed, Args: []any{"save to patent note: " + err.Error()}, Error: true}
		}
		n.Clear(number)
		return pane.StatusMsg{Key: text.StatusNotesSavedToPatentNote, Args: []any{number.String()}}
	}
}

// ExportNotes builds the clipboard text for the notes buffer: every captured
// passage with its locator and timestamp, without patent attrs.
func (n *notesAccumulator) ExportNotes(number domain.PatentNumber) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Notes · %s\n", number.String()))
	writeNotes(&b, n.entries[number])
	return b.String()
}

// ExportText builds the clipboard text for the notes buffer with the patent
// attrs header prepended ("patent number info + notes").
func (n *notesAccumulator) ExportText(number domain.PatentNumber, patent *domain.Patent) string {
	var b strings.Builder
	writeMeta(&b, number, patent)
	writeNotes(&b, n.entries[number])
	return b.String()
}

// writeNotes writes the accumulated passages to b.
func writeNotes(b *strings.Builder, entries []noteEntry) {
	sep := strings.Repeat("─", 48)
	if len(entries) == 0 {
		b.WriteString("(no notes accumulated)\n")
		return
	}
	b.WriteString(sep + "\n")
	for _, e := range entries {
		stamp := ""
		if !e.CapturedAt.IsZero() {
			stamp = " " + e.CapturedAt.Format(notesTimeLayoutBracket)
		}
		b.WriteString("• " + e.Locator + stamp + "\n")
		if e.Text != "" {
			b.WriteString("  " + e.Text + "\n")
		}
	}
	b.WriteString(sep + "\n")
}

// NotesBufferOverlay shows the accumulated notes for one patent and supports
// copying them to the clipboard or flushing the locators to the patent's IDS.
type NotesBufferOverlay struct {
	theme  render.Theme
	number domain.PatentNumber
	patent domain.Patent
	app    *App
}

func newNotesBufferOverlay(theme render.Theme, number domain.PatentNumber, patent domain.Patent, app *App) *NotesBufferOverlay {
	return &NotesBufferOverlay{
		theme:  theme,
		number: number,
		patent: patent,
		app:    app,
	}
}

func (o *NotesBufferOverlay) Title() string { return "Notes Buffer · " + o.number.String() }

// OverlaySize implements DynamicSize so the notes buffer gets a readable popup
// footprint for longer captured passages.
func (o *NotesBufferOverlay) OverlaySize(termW, termH int) (int, int) {
	return overlay.PctSize(termW, termH, notesOverlayWidthPct, notesOverlayHeightPct, notesOverlayMinWidth, notesOverlayMinHeight)
}

// Handles returns no command IDs: the overlay consumes its keys directly as a
// KeyHandler, so there are no overlay-scoped keymap bindings to validate.
func (o *NotesBufferOverlay) Handles() []command.ID { return nil }

func (o *NotesBufferOverlay) Command(command.ID, int) (overlay.Overlay, tea.Cmd) { return o, nil }

// HandleKey consumes the notes-overlay action keys directly. Navigation keys
// are swallowed (the buffer is short and does not scroll); everything else
// falls through to the keymap so Esc/q still close the overlay.
func (o *NotesBufferOverlay) HandleKey(msg tea.KeyMsg) (overlay.Overlay, tea.Cmd, bool) {
	switch msg.String() {
	case "y":
		txt := o.app.notes.ExportNotes(o.number)
		return o, func() tea.Msg { return pane.CopyToClipboardMsg{Text: txt} }, true
	case "Y":
		txt := o.app.notes.ExportText(o.number, &o.patent)
		return o, func() tea.Msg { return pane.CopyToClipboardMsg{Text: txt} }, true
	case "F":
		project := domain.ProjectID("")
		if o.app.activeProject != nil {
			project = o.app.activeProject.ID
		}
		if project == "" {
			return o, func() tea.Msg {
				return pane.StatusMsg{Key: text.StatusNoActiveProject, Error: true}
			}, true
		}
		return o, o.app.notes.FlushToIDS(o.app.client, o.number, project), true
	case "s":
		project := domain.ProjectID("")
		if o.app.activeProject != nil {
			project = o.app.activeProject.ID
		}
		if project == "" {
			return o, func() tea.Msg {
				return pane.StatusMsg{Key: text.StatusNoActiveProject, Error: true}
			}, true
		}
		return o, o.app.notes.SaveToPatentNote(o.app.client, o.number, project), true
	case "j", "k", "up", "down", "ctrl+d", "ctrl+u", "pgup", "pgdown":
		return o, nil, true
	}
	return o, nil, false
}

func (o *NotesBufferOverlay) View(maxW, maxH int) string {
	var b strings.Builder
	entries := o.app.notes.Entries(o.number)
	if len(entries) == 0 {
		b.WriteString(o.theme.Dim.Render("  (no notes accumulated)"))
	} else {
		for _, e := range entries {
			stamp := ""
			if !e.CapturedAt.IsZero() {
				stamp = " " + e.CapturedAt.Format(notesTimeLayoutBracket)
			}
			b.WriteString(o.theme.Header.Render("  · " + e.Locator))
			b.WriteString(o.theme.Dim.Render(stamp))
			b.WriteByte('\n')
			if e.Text != "" {
				b.WriteString(o.theme.Row.Render("    " + e.Text))
				b.WriteByte('\n')
			}
		}
	}
	b.WriteString("\n\n")
	b.WriteString(o.theme.Dim.Render("  [y] copy notes  [Y] copy with patent info  [s] save to patent note  [F] flush to IDS  [esc] close"))
	return b.String()
}

// writeMeta writes the patent attrs header to b.
func writeMeta(b *strings.Builder, number domain.PatentNumber, p *domain.Patent) {
	sep := strings.Repeat("═", 48)
	b.WriteString(sep + "\n")
	b.WriteString(fmt.Sprintf("Patent #:     %s\n", number.String()))
	if p != nil {
		b.WriteString(fmt.Sprintf("Title:        %s\n", p.Title))
		if len(p.Inventors) > 0 {
			inventorStr := string(p.Inventors[0])
			if len(p.Inventors) > 1 {
				inventorStr += " et al. (" + fmt.Sprintf("%d", len(p.Inventors)) + ")"
			}
			b.WriteString(fmt.Sprintf("Inventors:    %s\n", inventorStr))
		}
		if p.Assignee != "" {
			b.WriteString(fmt.Sprintf("Assignee:     %s\n", p.Assignee))
		}
		for _, doc := range p.Documents {
			if doc.Stage == domain.StageApplication {
				dateStr := ""
				if !doc.Dated.IsZero() {
					dateStr = " (" + doc.Dated.Format(domain.DateLayout) + ")"
				}
				b.WriteString(fmt.Sprintf("Application #: %s%s\n", doc.Number.String(), dateStr))
				break
			}
		}
		if !p.PublicationDate.IsZero() {
			b.WriteString(fmt.Sprintf("Publication:  %s\n", p.PublicationDate.Format(domain.DateLayout)))
		}
		if !p.ExpirationDate.IsZero() {
			src := ""
			if p.ExpirationSource != "" {
				src = " (" + p.ExpirationSource + ")"
			}
			b.WriteString(fmt.Sprintf("Expiration:   %s%s\n", p.ExpirationDate.Format(domain.DateLayout), src))
		}
	}
	b.WriteString(sep + "\n")
}
