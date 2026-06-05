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

	renaming  bool   // editing the selected document's display name
	rename    string // the in-progress new name
	tagging   bool   // editing a tag to add/remove on the selected document
	untag     bool   // when true, tagging removes instead of adds
	tagInput  string
	assigning bool // editing an office-action assignment
	unassign  bool // when true, assigning removes instead of adds
	oaInput   string

	confirmDelete bool // d pressed, awaiting y/n
	extracting    bool // a text extraction is in flight
	msg           string
}

func NewMatterDocuments(client *rpc.Client, theme render.Theme, project domain.ProjectID) *MatterDocuments {
	return &MatterDocuments{client: client, theme: theme, project: project, loading: true}
}

func (o *MatterDocuments) Title() string { return "Documents" }

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
		o.msg = fmt.Sprintf("converted %d line(s), %d chars from %s", convertedLineCount(m.text), len(m.text), m.name)
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
	if o.tagging {
		return o.handleTagKey(msg)
	}
	if o.assigning {
		return o.handleAssignKey(msg)
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
		case "t":
			o.beginTag(false)
		case "u":
			o.beginTag(true)
		case "a":
			o.beginAssign(false)
		case "x":
			o.beginAssign(true)
		case "d":
			if len(o.items) > 0 {
				o.confirmDelete = true
				o.msg = ""
			}
		case "o":
			if cmd := o.openSelected(); cmd != nil {
				return o, cmd, true
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

func (o *MatterDocuments) openSelected() tea.Cmd {
	doc, ok := o.selected()
	if !ok || doc.BlobPath == "" {
		return nil
	}
	return func() tea.Msg { return OpenDocumentFileMsg{Path: doc.BlobPath} }
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

func (o *MatterDocuments) handleTagKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyEsc:
		o.tagging = false
		o.untag = false
		o.msg = "tag cancelled"
		return o, nil, true
	case tea.KeyEnter:
		o.tagging = false
		return o, o.commitTag(), true
	case tea.KeyBackspace:
		if len(o.tagInput) > 0 {
			r := []rune(o.tagInput)
			o.tagInput = string(r[:len(r)-1])
		}
		return o, nil, true
	case tea.KeyCtrlU:
		o.tagInput = ""
		return o, nil, true
	case tea.KeyRunes, tea.KeySpace:
		o.tagInput += msg.String()
		return o, nil, true
	}
	return o, nil, true
}

func (o *MatterDocuments) handleAssignKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyEsc:
		o.assigning = false
		o.unassign = false
		o.msg = "assignment cancelled"
		return o, nil, true
	case tea.KeyEnter:
		o.assigning = false
		return o, o.commitAssign(), true
	case tea.KeyBackspace:
		if len(o.oaInput) > 0 {
			r := []rune(o.oaInput)
			o.oaInput = string(r[:len(r)-1])
		}
		return o, nil, true
	case tea.KeyCtrlU:
		o.oaInput = ""
		return o, nil, true
	case tea.KeyRunes, tea.KeySpace:
		o.oaInput += msg.String()
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
		text = "(no embedded text; OCR appears needed but is not implemented; open the file directly to read it)"
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

func (o *MatterDocuments) beginTag(remove bool) {
	doc, ok := o.selected()
	if !ok {
		return
	}
	o.tagging = true
	o.untag = remove
	o.tagInput = ""
	if remove && len(doc.Tags) > 0 {
		o.tagInput = doc.Tags[0].Name
	}
	o.msg = ""
}

func (o *MatterDocuments) beginAssign(remove bool) {
	doc, ok := o.selected()
	if !ok {
		return
	}
	o.assigning = true
	o.unassign = remove
	o.oaInput = ""
	if remove && len(doc.OfficeActionIDs) > 0 {
		o.oaInput = doc.OfficeActionIDs[0]
	}
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

func (o *MatterDocuments) commitTag() tea.Cmd {
	doc, ok := o.selected()
	if !ok {
		return nil
	}
	tag := strings.TrimSpace(o.tagInput)
	if tag == "" {
		return nil
	}
	client, id, remove := o.client, doc.ID, o.untag
	reload := o.loadCmd()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.MatterDocumentResult
		method := proto.MethodMatterDocumentTag
		if remove {
			method = proto.MethodMatterDocumentUntag
		}
		if err := client.Call(ctx, method, proto.MatterDocumentTagParams{ID: id, Tag: tag}, &res); err != nil {
			return matterDocsLoadedMsg{err: err}
		}
		return reload()
	}
}

func (o *MatterDocuments) commitAssign() tea.Cmd {
	doc, ok := o.selected()
	if !ok {
		return nil
	}
	oaID := strings.TrimSpace(o.oaInput)
	if oaID == "" {
		return nil
	}
	client, id, remove := o.client, doc.ID, o.unassign
	reload := o.loadCmd()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.MatterDocumentResult
		method := proto.MethodMatterDocumentAssign
		if remove {
			method = proto.MethodMatterDocumentUnassign
		}
		if err := client.Call(ctx, method, proto.MatterDocumentOfficeActionParams{ID: id, OfficeActionID: oaID}, &res); err != nil {
			return matterDocsLoadedMsg{err: err}
		}
		return reload()
	}
}

// extractSelected re-runs the built-in document text parser on the selected
// document. Scanned/image-only files still require OCR, which is not implemented,
// and return a no-text error.
func (o *MatterDocuments) extractSelected() tea.Cmd {
	doc, ok := o.selected()
	if !ok {
		return nil
	}
	o.extracting = true
	o.msg = ""
	client, id, name := o.client, doc.ID, doc.DisplayName
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
		if tags := tagNames(d.Tags); tags != "" {
			name += " [" + tags + "]"
		}
		if len(d.OfficeActionIDs) > 1 {
			name += fmt.Sprintf(" (oa:%d)", len(d.OfficeActionIDs))
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

	footer := "↑/↓ move · enter view · o open · e extract · t tag · u untag · a assign OA · x unassign OA · r rename · d delete · esc close"
	switch {
	case o.extracting:
		footer = "converting embedded text… OCR is not implemented"
	case o.renaming:
		footer = "rename: type · enter save · esc cancel"
	case o.tagging:
		action := "tag"
		if o.untag {
			action = "untag"
		}
		footer = action + ": " + o.tagInput + "▏ · enter save · esc cancel · use lowercase snake_case"
	case o.assigning:
		action := "assign OA id"
		if o.unassign {
			action = "unassign OA id"
		}
		footer = action + ": " + o.oaInput + "▏ · enter save · esc cancel"
	case o.confirmDelete:
		footer = "delete this document? y to confirm, any key to cancel"
	case o.msg != "":
		footer = o.msg + " · " + footer
	}
	b.WriteString(o.theme.Dim.Render(render.Truncate(footer, maxW)))
	return b.String()
}

func tagNames(tags []domain.Tag) string {
	if len(tags) == 0 {
		return ""
	}
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		if tag.Name != "" {
			names = append(names, tag.Name)
		}
	}
	return strings.Join(names, ",")
}

func convertedLineCount(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}
