package overlay

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
	"patentmine/internal/tui/pane"
	"patentmine/internal/tui/render"
)

const (
	mdListWidthPct  = 85
	mdListHeightPct = 75
	mdListMinWidth  = 80
	mdListMinHeight = 16
)

type col0Type int

const (
	col0Name col0Type = iota
	col0Kind
	col0Origin
	col0Stage
	col0LoadDate
	col0LoadTime
	col0LastOpened
	col0Tags
	col0NotesCount
	col0OtherOAs
)

// col0Count is the number of columns in the associated-documents table; used for
// wrapping the column cursor.
const col0Count = 10

var sortKeys0 = []string{"name", "kind", "origin", "stage", "load_date", "load_time", "last_opened", "tags", "notes", "other_oas"}

type col1Type int

const (
	col1Name col1Type = iota
	col1Origin
	col1Stage
	col1Project
	col1OAs
	col1LoadDateTime
	col1LastOpened
	col1Tags
)

// col1Count is the number of columns in the all-documents table.
const col1Count = 8

var sortKeys1 = []string{"name", "origin", "stage", "project", "office_actions", "loaded", "last_opened", "tags"}

type matterDocsLoadedMsg struct {
	projectItems []domain.MatterDocument
	allItems     []domain.MatterDocument
	err          error
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
	oa      *domain.OfficeAction // current office action context if opened from one

	// Single-table state (if oa == nil)
	items   []domain.MatterDocument
	cursor  int
	offset  int

	// Dual-table state (if oa != nil)
	items0      []domain.MatterDocument
	rawItems0   []domain.MatterDocument
	cursor0     int
	offset0     int
	activeCol0  int
	sortCol0    col0Type
	sortDesc0   bool

	items1      []domain.MatterDocument
	rawItems1   []domain.MatterDocument
	cursor1     int
	offset1     int
	activeCol1  int
	sortCol1    col1Type
	sortDesc1   bool

	activeTable int // 0 = associated, 1 = all

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
	jump          JumpNavigator
}

func NewMatterDocuments(client *rpc.Client, theme render.Theme, project domain.ProjectID, oa *domain.OfficeAction) *MatterDocuments {
	return &MatterDocuments{
		client:      client,
		theme:       theme,
		project:     project,
		oa:          oa,
		loading:     true,
		activeTable: 0,
		sortCol0:    col0LoadDate,
		sortDesc0:   true,
		sortCol1:    col1LoadDateTime,
		sortDesc1:   true,
	}
}

func (o *MatterDocuments) Title() string {
	if o.oa != nil {
		return fmt.Sprintf("Document Loaded for Office Action - %s %s", o.project, o.oa.Name)
	}
	return "Documents"
}

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
		var resProject proto.MatterDocumentListResult
		err := client.Call(ctx, proto.MethodMatterDocumentList, proto.MatterDocumentListParams{Project: project}, &resProject)
		if err != nil {
			return matterDocsLoadedMsg{err: err}
		}
		var allDocs []domain.MatterDocument
		if o.oa != nil {
			var resAll proto.MatterDocumentListResult
			err = client.Call(ctx, proto.MethodMatterDocumentList, proto.MatterDocumentListParams{Project: ""}, &resAll)
			if err != nil {
				return matterDocsLoadedMsg{err: err}
			}
			allDocs = resAll.Documents
		}
		return matterDocsLoadedMsg{projectItems: resProject.Documents, allItems: allDocs, err: nil}
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
		if o.oa != nil {
			o.rawItems0 = nil
			for _, d := range m.projectItems {
				if isAssociatedWith(d, o.oa.ID) {
					o.rawItems0 = append(o.rawItems0, d)
				}
			}
			o.items0 = make([]domain.MatterDocument, len(o.rawItems0))
			copy(o.items0, o.rawItems0)
			sortItems0(o.items0, o.sortCol0, o.sortDesc0, o.oa.ID)

			o.rawItems1 = make([]domain.MatterDocument, len(m.allItems))
			copy(o.rawItems1, m.allItems)
			o.items1 = make([]domain.MatterDocument, len(o.rawItems1))
			copy(o.items1, o.rawItems1)
			sortItems1(o.items1, o.sortCol1, o.sortDesc1)

			if o.cursor0 >= len(o.items0) {
				o.cursor0 = max(len(o.items0)-1, 0)
			}
			if o.cursor1 >= len(o.items1) {
				o.cursor1 = max(len(o.items1)-1, 0)
			}
		} else {
			o.items = m.projectItems
			if o.cursor >= len(o.items) {
				o.cursor = max(len(o.items)-1, 0)
			}
		}
		o.loadErr = ""
	case matterDocExtractedMsg:
		o.extracting = false
		if m.err != nil {
			o.msg = "extract failed: " + m.err.Error()
			return o, nil
		}
		o.msg = fmt.Sprintf("converted %d line(s), %d chars from %s", convertedLineCount(m.text), len(m.text), m.name)
		return o, o.loadCmd()
	case pane.MatterDocumentImportedMsg:
		o.msg = "loaded document: " + m.Name
		return o, o.loadCmd()
	}
	return o, nil
}

func (o *MatterDocuments) HandleKey(msg tea.KeyMsg) (Overlay, tea.Cmd, bool) {
	if o.loading {
		return o, nil, true
	}
	if !o.renaming && !o.tagging && !o.assigning && !o.confirmDelete {
		var currentFocus, numFields int
		if o.oa != nil {
			if o.activeTable == 0 {
				currentFocus = o.cursor0
				numFields = len(o.items0)
			} else {
				currentFocus = o.cursor1
				numFields = len(o.items1)
			}
		} else {
			currentFocus = o.cursor
			numFields = len(o.items)
		}
		if newCursor, handled := o.jump.HandleKey(msg, currentFocus, numFields); handled {
			if o.oa != nil {
				if o.activeTable == 0 {
					o.cursor0 = newCursor
				} else {
					o.cursor1 = newCursor
				}
			} else {
				o.cursor = newCursor
			}
			return o, nil, true
		}
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
	case tea.KeyTab:
		if o.oa != nil {
			o.activeTable = 1 - o.activeTable
		}
		return o, nil, true
	case tea.KeyUp:
		o.moveCursor(-1)
		return o, nil, true
	case tea.KeyDown:
		o.moveCursor(1)
		return o, nil, true
	case tea.KeyLeft:
		if o.oa != nil {
			if o.activeTable == 0 {
				o.activeCol0 = (o.activeCol0 - 1 + col0Count) % col0Count
			} else {
				o.activeCol1 = (o.activeCol1 - 1 + col1Count) % col1Count
			}
		}
		return o, nil, true
	case tea.KeyRight:
		if o.oa != nil {
			if o.activeTable == 0 {
				o.activeCol0 = (o.activeCol0 + 1) % col0Count
			} else {
				o.activeCol1 = (o.activeCol1 + 1) % col1Count
			}
		}
		return o, nil, true
	case tea.KeyEnter:
		return o, o.viewSelected(), true
	case tea.KeyRunes:
		switch msg.String() {
		case ";":
			if !o.renaming && !o.tagging && !o.assigning && !o.confirmDelete {
				o.jump.Active = true
				o.jump.PendingCount = 0
				o.jump.PendingG = false
				return o, nil, true
			}
		case "k":
			o.moveCursor(-1)
		case "j":
			o.moveCursor(1)
		case ".":
			if o.oa != nil {
				if o.activeTable == 0 {
					col := col0Type(o.activeCol0)
					if o.sortCol0 == col {
						o.sortDesc0 = !o.sortDesc0
					} else {
						o.sortCol0 = col
						o.sortDesc0 = false
					}
					sortItems0(o.items0, o.sortCol0, o.sortDesc0, o.oa.ID)
				} else {
					col := col1Type(o.activeCol1)
					if o.sortCol1 == col {
						o.sortDesc1 = !o.sortDesc1
					} else {
						o.sortCol1 = col
						o.sortDesc1 = false
					}
					sortItems1(o.items1, o.sortCol1, o.sortDesc1)
				}
			}
		case "r":
			o.beginRename()
		case "s":
			if cmd := o.cycleStage(1); cmd != nil {
				return o, cmd, true
			}
		case "O":
			if cmd := o.cycleOrigin(1); cmd != nil {
				return o, cmd, true
			}
		case "t":
			o.beginTag(false)
		case "u":
			o.beginTag(true)
		case "a":
			if o.oa != nil && o.activeTable == 1 {
				// Instantly associate to the current OA
				if doc, ok := o.selected(); ok {
					return o, o.commitAssignFor(doc, o.oa.ID, false), true
				}
			} else {
				// Prompt to assign selected doc to another OA
				o.beginAssign(false)
			}
		case "x":
			if o.oa != nil && o.activeTable == 0 {
				// Instantly remove from the current OA
				if doc, ok := o.selected(); ok {
					return o, o.commitAssignFor(doc, o.oa.ID, true), true
				}
			} else {
				// Prompt to unassign from another OA
				o.beginAssign(true)
			}
		case "d":
			hasSelection := false
			if o.oa != nil {
				if o.activeTable == 0 {
					hasSelection = len(o.items0) > 0
				} else {
					hasSelection = len(o.items1) > 0
				}
			} else {
				hasSelection = len(o.items) > 0
			}
			if hasSelection {
				o.confirmDelete = true
				o.msg = ""
			}
		case "l":
			return o, func() tea.Msg { return StartDocumentImportMsg{} }, true
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
	return func() tea.Msg { return OpenDocumentFileMsg{ID: doc.ID, Path: doc.BlobPath} }
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
	if o.oa != nil {
		if o.activeTable == 0 {
			if len(o.items0) == 0 {
				return
			}
			o.cursor0 = max(0, min(o.cursor0+delta, len(o.items0)-1))
		} else {
			if len(o.items1) == 0 {
				return
			}
			o.cursor1 = max(0, min(o.cursor1+delta, len(o.items1)-1))
		}
	} else {
		if len(o.items) == 0 {
			return
		}
		o.cursor = max(0, min(o.cursor+delta, len(o.items)-1))
	}
}

func (o *MatterDocuments) selected() (domain.MatterDocument, bool) {
	if o.oa != nil {
		if o.activeTable == 0 {
			if o.cursor0 < 0 || o.cursor0 >= len(o.items0) {
				return domain.MatterDocument{}, false
			}
			return o.items0[o.cursor0], true
		} else {
			if o.cursor1 < 0 || o.cursor1 >= len(o.items1) {
				return domain.MatterDocument{}, false
			}
			return o.items1[o.cursor1], true
		}
	} else {
		if o.cursor < 0 || o.cursor >= len(o.items) {
			return domain.MatterDocument{}, false
		}
		return o.items[o.cursor], true
	}
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
	return func() tea.Msg { return OpenDocumentTextMsg{ID: doc.ID, Title: title, Text: text} }
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

// cycleStage advances the selected document's lifecycle stage by delta and
// persists it. cycleOrigin does the same for its origin. Both reuse setMeta.
func (o *MatterDocuments) cycleStage(delta int) tea.Cmd {
	doc, ok := o.selected()
	if !ok {
		return nil
	}
	return o.setMeta(doc.ID, "", doc.Stage.Cycle(delta))
}

func (o *MatterDocuments) cycleOrigin(delta int) tea.Cmd {
	doc, ok := o.selected()
	if !ok {
		return nil
	}
	return o.setMeta(doc.ID, doc.Origin.Cycle(delta), "")
}

// setMeta persists an origin and/or stage change for one document, then reloads.
// An empty origin or stage leaves that axis unchanged (see engine.SetMatterDocumentMeta).
func (o *MatterDocuments) setMeta(id string, origin domain.DocOrigin, stage domain.DocStage) tea.Cmd {
	client := o.client
	reload := o.loadCmd()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var res proto.MatterDocumentResult
		if err := client.Call(ctx, proto.MethodMatterDocumentSetMeta,
			proto.MatterDocumentMetaParams{ID: id, Origin: origin, Stage: stage}, &res); err != nil {
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

func (o *MatterDocuments) commitAssignFor(doc domain.MatterDocument, oaID string, remove bool) tea.Cmd {
	client, id := o.client, doc.ID
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

	if o.oa != nil {
		var b strings.Builder

		// Determine split heights
		totalBodyHeight := maxH - 6 // margins/titles/borders/footers
		if totalBodyHeight < 6 {
			totalBodyHeight = 6
		}
		h0 := totalBodyHeight / 2
		h1 := totalBodyHeight - h0

		colsW := maxW - o.theme.TablePrefixWidth()
		colWidths0 := getColWidths(colsW, []int{0, 14, 13, 12, 10, 8, 12, 12, 5, 10})
		colWidths1 := getColWidths(colsW, []int{0, 13, 12, 10, 12, 16, 12, 12})

		// 1. Table 0 (Associated Documents)
		b.WriteString(o.theme.Title.Render("Associated with Current Office Action:") + "\n")
		bodyH0 := max(1, h0-1)
		if o.cursor0 < o.offset0 {
			o.offset0 = o.cursor0
		} else if o.cursor0 >= o.offset0+bodyH0 {
			o.offset0 = o.cursor0 - bodyH0 + 1
		}
		renderRows0 := min(bodyH0, len(o.items0)-o.offset0)
		if renderRows0 < 0 {
			renderRows0 = 0
		}

		columns0 := []render.TableColumn{
			{Key: "name", Label: "Name", Width: colWidths0[0], SortKey: "name"},
			{Key: "kind", Label: "Kind", Width: colWidths0[1], SortKey: "kind"},
			{Key: "origin", Label: "Origin", Width: colWidths0[2], SortKey: "origin"},
			{Key: "stage", Label: "Stage", Width: colWidths0[3], SortKey: "stage"},
			{Key: "load_date", Label: "Load Date", Width: colWidths0[4], SortKey: "load_date"},
			{Key: "load_time", Label: "Load Time", Width: colWidths0[5], SortKey: "load_time"},
			{Key: "last_opened", Label: "Last Opened", Width: colWidths0[6], SortKey: "last_opened"},
			{Key: "tags", Label: "Tags", Width: colWidths0[7], SortKey: "tags"},
			{Key: "notes", Label: "Notes", Width: colWidths0[8], SortKey: "notes"},
			{Key: "other_oas", Label: "Other OAs", Width: colWidths0[9], SortKey: "other_oas"},
		}

		params0 := render.TableParams{
			Theme:         o.theme,
			Columns:       columns0,
			RowCount:      renderRows0,
			FocusedColIdx: o.activeCol0,
			ActiveSort:    sortKeys0[o.sortCol0],
			SortAscending: !o.sortDesc0,
			FocusActive:   o.activeTable == 0,
			IsRowCursor: func(rIdx int) bool {
				return o.offset0+rIdx == o.cursor0
			},
		}

		t0Str := render.RenderTable(params0, maxW, func(rowIdx, colIdx int) string {
			if o.offset0+rowIdx >= len(o.items0) {
				return ""
			}
			doc := o.items0[o.offset0+rowIdx]
			switch col0Type(colIdx) {
			case col0Name:
				lineNum := o.offset0 + rowIdx + 1
				gutter := ""
				if o.activeTable == 0 {
					gutter = o.jump.GutterPrefix(lineNum)
				} else {
					gutter = fmt.Sprintf(" %d ", lineNum)
				}
				return gutter + doc.DisplayName
			case col0Kind:
				return doc.Kind.Label()
			case col0Origin:
				return doc.Origin.Label()
			case col0Stage:
				return doc.Stage.Label()
			case col0LoadDate:
				return doc.AddedAt.Format("2006-01-02")
			case col0LoadTime:
				return doc.AddedAt.Format("15:04:05")
			case col0LastOpened:
				if doc.LastOpenedAt.IsZero() {
					return "-"
				}
				return doc.LastOpenedAt.Local().Format("01-02 15:04")
			case col0Tags:
				return tagNames(doc.Tags)
			case col0NotesCount:
				return "0"
			case col0OtherOAs:
				return otherOAs(doc, o.oa.ID)
			default:
				return ""
			}
		})
		b.WriteString(t0Str)

		b.WriteString("\n")

		// 2. Table 1 (All Documents)
		b.WriteString(o.theme.Title.Render("All Loaded Documents:") + "\n")
		bodyH1 := max(1, h1-1)
		if o.cursor1 < o.offset1 {
			o.offset1 = o.cursor1
		} else if o.cursor1 >= o.offset1+bodyH1 {
			o.offset1 = o.cursor1 - bodyH1 + 1
		}
		renderRows1 := min(bodyH1, len(o.items1)-o.offset1)
		if renderRows1 < 0 {
			renderRows1 = 0
		}

		columns1 := []render.TableColumn{
			{Key: "name", Label: "Name", Width: colWidths1[0], SortKey: "name"},
			{Key: "origin", Label: "Origin", Width: colWidths1[1], SortKey: "origin"},
			{Key: "stage", Label: "Stage", Width: colWidths1[2], SortKey: "stage"},
			{Key: "project", Label: "Project", Width: colWidths1[3], SortKey: "project"},
			{Key: "office_actions", Label: "Office Actions", Width: colWidths1[4], SortKey: "office_actions"},
			{Key: "loaded", Label: "Loaded", Width: colWidths1[5], SortKey: "loaded"},
			{Key: "last_opened", Label: "Last Opened", Width: colWidths1[6], SortKey: "last_opened"},
			{Key: "tags", Label: "Tags", Width: colWidths1[7], SortKey: "tags"},
		}

		params1 := render.TableParams{
			Theme:         o.theme,
			Columns:       columns1,
			RowCount:      renderRows1,
			FocusedColIdx: o.activeCol1,
			ActiveSort:    sortKeys1[o.sortCol1],
			SortAscending: !o.sortDesc1,
			FocusActive:   o.activeTable == 1,
			IsRowCursor: func(rIdx int) bool {
				return o.offset1+rIdx == o.cursor1
			},
		}

		t1Str := render.RenderTable(params1, maxW, func(rowIdx, colIdx int) string {
			if o.offset1+rowIdx >= len(o.items1) {
				return ""
			}
			doc := o.items1[o.offset1+rowIdx]
			switch col1Type(colIdx) {
			case col1Name:
				lineNum := o.offset1 + rowIdx + 1
				gutter := ""
				if o.activeTable == 1 {
					gutter = o.jump.GutterPrefix(lineNum)
				} else {
					gutter = fmt.Sprintf(" %d ", lineNum)
				}
				return gutter + doc.DisplayName
			case col1Origin:
				return doc.Origin.Label()
			case col1Stage:
				return doc.Stage.Label()
			case col1Project:
				return string(doc.Project)
			case col1OAs:
				return strings.Join(doc.OfficeActionIDs, ",")
			case col1LoadDateTime:
				return doc.AddedAt.Format("2006-01-02 15:04")
			case col1LastOpened:
				if doc.LastOpenedAt.IsZero() {
					return "-"
				}
				return doc.LastOpenedAt.Local().Format("01-02 15:04")
			case col1Tags:
				return tagNames(doc.Tags)
			default:
				return ""
			}
		})
		b.WriteString(t1Str)

		b.WriteString("\n")

		footer := "↑/↓ move · tab switch table · ←/→ move col cursor · . sort col · enter view · i load · o open · e extract · s stage · O origin · r rename · d delete · esc close"
		if o.activeTable == 0 {
			footer += " · a assign OA · x remove from current OA"
		} else {
			footer += " · a add to current OA · x unassign OA"
		}

		switch {
		case o.extracting:
			footer = "converting embedded text…"
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
		if o.jump.Active {
			curr := o.cursor0
			if o.activeTable == 1 {
				curr = o.cursor1
			}
			footer = o.jump.HintSuffix(curr, -1, false)
		}
		b.WriteString(o.theme.Dim.Render(render.Truncate(footer, maxW)))
		return b.String()
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
		lineNum := i + 1
		gutter := o.jump.GutterPrefix(lineNum)
		row := fmt.Sprintf("%s%-14s %-12s %-13s %s", gutter, d.Kind.Label(), d.Stage.Label(), d.Origin.Label(), name)
		cell := render.Pad(render.Truncate(row, maxW), maxW)
		if i == o.cursor {
			b.WriteString(o.theme.Selected.Render(cell))
		} else {
			b.WriteString(o.theme.Row.Render(cell))
		}
		b.WriteByte('\n')
	}

	footer := "↑/↓ move · enter view · i load · o open · e extract · s stage · O origin · t tag · u untag · a assign OA · x unassign OA · r rename · d delete · esc close · [;] jump mode"
	switch {
	case o.extracting:
		footer = "converting embedded text…"
	case o.renaming:
		footer = "rename: type · enter save · esc cancel"
	case o.tagging:
		action := "tag"
		if o.untag {
			action = "untag"
			footer = action + ": " + o.tagInput + "▏ · enter save · esc cancel · use lowercase snake_case"
		}
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
	if o.jump.Active {
		footer = o.jump.HintSuffix(o.cursor, -1, false)
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

func isAssociatedWith(doc domain.MatterDocument, oaID string) bool {
	if doc.OfficeActionID == oaID {
		return true
	}
	for _, id := range doc.OfficeActionIDs {
		if id == oaID {
			return true
		}
	}
	return false
}

func otherOAs(doc domain.MatterDocument, currentOAID string) string {
	var list []string
	for _, id := range doc.OfficeActionIDs {
		if id != currentOAID {
			list = append(list, id)
		}
	}
	return strings.Join(list, ", ")
}

func getColWidths(totalWidth int, widths []int) []int {
	out := make([]int, len(widths))
	copy(out, widths)
	fixedSum := 0
	flexibleIdx := -1
	for i, w := range widths {
		if w == 0 {
			flexibleIdx = i
		} else {
			fixedSum += w
		}
	}
	// add 1 space between columns
	fixedSum += len(widths) - 1
	flexWidth := totalWidth - fixedSum
	if flexWidth < 8 {
		flexWidth = 8 // minimum safety
	}
	if flexibleIdx != -1 {
		out[flexibleIdx] = flexWidth
	}
	return out
}

func sortItems0(items []domain.MatterDocument, col col0Type, desc bool, currentOAID string) {
	sort.SliceStable(items, func(i, j int) bool {
		var less bool
		d1, d2 := items[i], items[j]
		switch col {
		case col0Name:
			less = strings.ToLower(d1.DisplayName) < strings.ToLower(d2.DisplayName)
		case col0Kind:
			less = strings.ToLower(d1.Kind.Label()) < strings.ToLower(d2.Kind.Label())
		case col0Origin:
			less = strings.ToLower(d1.Origin.Label()) < strings.ToLower(d2.Origin.Label())
		case col0Stage:
			less = strings.ToLower(d1.Stage.Label()) < strings.ToLower(d2.Stage.Label())
		case col0LoadDate, col0LoadTime:
			less = d1.AddedAt.Before(d2.AddedAt)
		case col0LastOpened:
			less = d1.LastOpenedAt.Before(d2.LastOpenedAt)
		case col0Tags:
			less = strings.ToLower(tagNames(d1.Tags)) < strings.ToLower(tagNames(d2.Tags))
		case col0NotesCount:
			less = false // always 0 placeholder
		case col0OtherOAs:
			less = strings.ToLower(otherOAs(d1, currentOAID)) < strings.ToLower(otherOAs(d2, currentOAID))
		}
		if desc {
			return !less
		}
		return less
	})
}

func sortItems1(items []domain.MatterDocument, col col1Type, desc bool) {
	sort.SliceStable(items, func(i, j int) bool {
		var less bool
		d1, d2 := items[i], items[j]
		switch col {
		case col1Name:
			less = strings.ToLower(d1.DisplayName) < strings.ToLower(d2.DisplayName)
		case col1Origin:
			less = strings.ToLower(d1.Origin.Label()) < strings.ToLower(d2.Origin.Label())
		case col1Stage:
			less = strings.ToLower(d1.Stage.Label()) < strings.ToLower(d2.Stage.Label())
		case col1Project:
			less = strings.ToLower(string(d1.Project)) < strings.ToLower(string(d2.Project))
		case col1OAs:
			less = strings.ToLower(strings.Join(d1.OfficeActionIDs, ",")) < strings.ToLower(strings.Join(d2.OfficeActionIDs, ","))
		case col1LoadDateTime:
			less = d1.AddedAt.Before(d2.AddedAt)
		case col1LastOpened:
			less = d1.LastOpenedAt.Before(d2.LastOpenedAt)
		case col1Tags:
			less = strings.ToLower(tagNames(d1.Tags)) < strings.ToLower(tagNames(d2.Tags))
		}
		if desc {
			return !less
		}
		return less
	})
}
