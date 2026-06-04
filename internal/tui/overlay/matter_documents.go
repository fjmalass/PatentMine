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
	"patentmine/internal/tui/render"
)

const (
	mdListWidthPct  = 72
	mdListHeightPct = 62
	mdListMinWidth  = 64
	mdListMinHeight = 12
)

type matterDocsLoadedMsg struct {
	items []domain.MatterDocument
	err   error
}

type matterDocExtractedMsg struct {
	name string
	text string
	err  error
}

// MatterDocuments lists every document filed under a matter (the active project):
// the office action, its response, cited references, and so on. Enter opens a
// read-only viewer over a document's text (to read it, and — once response
// drafting lands — to copy from it); r renames; d deletes. New documents are
// added with :add.document.
type MatterDocuments struct {
	client  *rpc.Client
	theme   render.Theme
	project domain.ProjectID

	items   []domain.MatterDocument
	cursor  int
	loading bool
	loadErr string

	renaming bool   // editing the selected document's display name
	rename   string // the in-progress new name

	confirmDelete bool // d pressed, awaiting y/n
	extracting    bool // an AI text extraction is in flight
	msg           string
}

func NewMatterDocuments(client *rpc.Client, theme render.Theme, project domain.ProjectID) *MatterDocuments {
	return &MatterDocuments{client: client, theme: theme, project: project, loading: true}
}

func (o *MatterDocuments) Title() string { return "Matter Documents" }

func (o *MatterDocuments) Handles() []command.ID { return nil }

func (o *MatterDocuments) Command(command.ID, int) (Overlay, tea.Cmd) { return o, nil }

func (o *MatterDocuments) OverlaySize(termW, termH int) (int, int) {
	return PctSize(termW, termH, mdListWidthPct, mdListHeightPct, mdListMinWidth, mdListMinHeight)
}

func (o *MatterDocuments) Init() tea.Cmd { return o.loadCmd() }

// loadCmd lists the matter's documents over RPC.
func (o *MatterDocuments) loadCmd() tea.Cmd {
	client, project := o.client, o.project
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.MatterDocumentListResult
		err := client.Call(ctx, proto.MethodMatterDocumentList, proto.MatterDocumentListParams{Project: project}, &res)
		return matterDocsLoadedMsg{items: res.Documents, err: err}
	}
}

func (o *MatterDocuments) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch m := msg.(type) {
	case matterDocsLoadedMsg:
		o.loading = false
		o.extracting = false
		if m.err != nil {
			o.loadErr = m.err.Error()
			return o, nil
		}
		o.items = m.items
		o.loadErr = ""
		if o.cursor >= len(o.items) {
			o.cursor = max(len(o.items)-1, 0)
		}
	case matterDocExtractedMsg:
		o.extracting = false
		if m.err != nil {
			o.msg = "extract failed: " + m.err.Error()
			return o, nil
		}
		o.msg = fmt.Sprintf("extracted %d chars from %s", len(m.text), m.name)
		return o, o.loadCmd()
	}
	return o, nil
}

func (o *MatterDocuments) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if o.loading {
		return o, nil, true
	}
	if o.renaming {
		return o.handleRenameKey(msg)
	}
	if o.confirmDelete {
		switch msg.String() {
		case "y", "Y":
			o.confirmDelete = false
			return o, o.deleteSelected(), true
		default:
			o.confirmDelete = false
			o.msg = "delete cancelled"
			return o, nil, true
		}
	}
	switch msg.Type {
	case tea.KeyEsc:
		return o, func() tea.Msg { return CloseOverlayMsg{} }, true
	case tea.KeyUp:
		o.moveCursor(-1)
		return o, nil, true
	case tea.KeyDown:
		o.moveCursor(1)
		return o, nil, true
	case tea.KeyEnter:
		return o, o.viewSelected(), true
	case tea.KeyRunes:
		switch msg.String() {
		case "k":
			o.moveCursor(-1)
		case "j":
			o.moveCursor(1)
		case "r":
			o.beginRename()
		case "d":
			if len(o.items) > 0 {
				o.confirmDelete = true
				o.msg = ""
			}
		case "e":
			if cmd := o.extractSelected(); cmd != nil {
				return o, cmd, true
			}
		case "q":
			return o, func() tea.Msg { return CloseOverlayMsg{} }, true
		}
		return o, nil, true
	}
	return o, nil, true
}

func (o *MatterDocuments) handleRenameKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyEsc:
		o.renaming = false
		o.msg = "rename cancelled"
		return o, nil, true
	case tea.KeyEnter:
		o.renaming = false
		return o, o.commitRename(), true
	case tea.KeyBackspace:
		if len(o.rename) > 0 {
			r := []rune(o.rename)
			o.rename = string(r[:len(r)-1])
		}
		return o, nil, true
	case tea.KeyCtrlU:
		o.rename = ""
		return o, nil, true
	case tea.KeyRunes, tea.KeySpace:
		o.rename += msg.String()
		return o, nil, true
	}
	return o, nil, true
}

func (o *MatterDocuments) moveCursor(delta int) {
	if len(o.items) == 0 {
		return
	}
	o.cursor = max(0, min(o.cursor+delta, len(o.items)-1))
}

func (o *MatterDocuments) selected() (domain.MatterDocument, bool) {
	if o.cursor < 0 || o.cursor >= len(o.items) {
		return domain.MatterDocument{}, false
	}
	return o.items[o.cursor], true
}

func (o *MatterDocuments) viewSelected() tea.Cmd {
	doc, ok := o.selected()
	if !ok {
		return nil
	}
	title, text := doc.DisplayName, doc.ExtractedText
	if strings.TrimSpace(text) == "" {
		text = "(no extractable text — this may be a scanned PDF; open the file directly to read it)"
	}
	return func() tea.Msg { return OpenDocumentTextMsg{Title: title, Text: text} }
}

func (o *MatterDocuments) beginRename() {
	doc, ok := o.selected()
	if !ok {
		return
	}
	o.renaming = true
	o.rename = doc.DisplayName
	o.msg = ""
}

func (o *MatterDocuments) commitRename() tea.Cmd {
	doc, ok := o.selected()
	if !ok {
		return nil
	}
	name := strings.TrimSpace(o.rename)
	if name == "" || name == doc.DisplayName {
		return nil
	}
	client, id := o.client, doc.ID
	reload := o.loadCmd()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.MatterDocumentResult
		if err := client.Call(ctx, proto.MethodMatterDocumentRename,
			proto.MatterDocumentRenameParams{ID: id, Name: name}, &res); err != nil {
			return matterDocsLoadedMsg{err: err}
		}
		return reload()
	}
}

// extractSelected runs AI text extraction (OCR) on the selected document — for
// a scanned, image-only PDF the importer could not read. It uses a generous
// timeout since multimodal OCR of a multi-page scan is slow.
func (o *MatterDocuments) extractSelected() tea.Cmd {
	doc, ok := o.selected()
	if !ok {
		return nil
	}
	o.extracting = true
	o.msg = ""
	client, id, name := o.client, doc.ID, doc.DisplayName
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		var res proto.MatterDocumentResult
		if err := client.Call(ctx, proto.MethodMatterDocumentExtract,
			proto.MatterDocumentIDParams{ID: id}, &res); err != nil {
			return matterDocExtractedMsg{name: name, err: err}
		}
		return matterDocExtractedMsg{name: res.Document.DisplayName, text: res.Document.ExtractedText}
	}
}

func (o *MatterDocuments) deleteSelected() tea.Cmd {
	doc, ok := o.selected()
	if !ok {
		return nil
	}
	client, id := o.client, doc.ID
	reload := o.loadCmd()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.MatterDocumentResult
		if err := client.Call(ctx, proto.MethodMatterDocumentDelete,
			proto.MatterDocumentIDParams{ID: id}, &res); err != nil {
			return matterDocsLoadedMsg{err: err}
		}
		return reload()
	}
}

func (o *MatterDocuments) View(maxW, maxH int) string {
	if o.loading {
		return o.theme.Dim.Render("loading documents…")
	}
	if o.loadErr != "" {
		return o.theme.Error.Render("error: " + o.loadErr)
	}
	if len(o.items) == 0 {
		return o.theme.Dim.Render(render.Truncate(
			"No documents yet. Add one with :add.officeaction or :add.document", maxW))
	}

	var b strings.Builder
	bodyRows := max(maxH-2, 1)
	for i := 0; i < len(o.items) && i < bodyRows; i++ {
		d := o.items[i]
		name := d.DisplayName
		if o.renaming && i == o.cursor {
			name = o.rename + "▏"
		}
		row := fmt.Sprintf("%-13s %s", d.Kind.Label(), name)
		cell := render.Pad(render.Truncate(row, maxW), maxW)
		if i == o.cursor {
			b.WriteString(o.theme.Selected.Render(cell))
		} else {
			b.WriteString(o.theme.Row.Render(cell))
		}
		b.WriteByte('\n')
	}

	footer := "↑/↓ move · enter view · e extract text · r rename · d delete · esc close"
	switch {
	case o.extracting:
		footer = "extracting text with AI… (this can take a minute for a scanned PDF)"
	case o.renaming:
		footer = "rename: type · enter save · esc cancel"
	case o.confirmDelete:
		footer = "delete this document? y to confirm, any key to cancel"
	case o.msg != "":
		footer = o.msg + " · " + footer
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(footer, maxW)))
	return b.String()
}
