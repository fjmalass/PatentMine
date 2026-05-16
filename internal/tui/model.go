package tui

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/changes"
	"patentmine/internal/config"
	"patentmine/internal/domain"
	"patentmine/internal/storage"
)

type viewMode string

const (
	viewList                 viewMode = "list"
	viewDetail               viewMode = "detail"
	viewCites                viewMode = "citations"
	viewCitedBy              viewMode = "cited-by"
	viewClassifications      viewMode = "classifications"
	viewFullText             viewMode = "full-text"
	viewNotes                viewMode = "notes"
	viewRefs                 viewMode = "references"
	viewAI                   viewMode = "ai"
	viewHelp                 viewMode = "help"
	viewHelpPopup            viewMode = "help-popup"
	viewKeymap               viewMode = "keymap"
	viewKeymapPopup          viewMode = "keymap-popup"
	viewPopupPatentDetail    viewMode = "popup-patent-detail"
	viewReview               viewMode = "review"
	viewConfirmDelete        viewMode = "confirm-delete"
	viewClassificationDetail viewMode = "classification-detail"
	viewInventors            viewMode = "inventors"
	viewFamily               viewMode = "family"
	viewSplash               viewMode = "splash"
	viewProjectEvents        viewMode = "project-events"
	viewProjectInvoices      viewMode = "project-invoices"
	viewProjectIDS           viewMode = "project-ids"
	viewProjectInfo          viewMode = "project-info"
	viewNoteEdit             viewMode = "note-edit"
	viewIDSEdit              viewMode = "ids-edit"
	viewDateEdit             viewMode = "date-edit"
	viewAbstract             viewMode = "abstract"
	viewClaim                viewMode = "view-claim"
	viewUSPTOKeyWarning      viewMode = "uspto-key-warning"
	viewBulkConfirm          viewMode = "bulk-confirm"
	viewReviewStateSelect    viewMode = "status-select"
	viewProjectTags          viewMode = "project-tags"
	viewTagSelect            viewMode = "tag-select"
	viewCountrySelect        viewMode = "country-select"
)

type bulkActionType string

const (
	bulkActionStore       bulkActionType = "store"
	bulkActionIgnore      bulkActionType = "ignore"
	bulkActionUnderReview bulkActionType = "under_review"
	bulkActionDelete      bulkActionType = "delete"
)

type Model struct {
	ctx                          context.Context
	repo                         storage.Repository
	input                        textinput.Model
	dateInput                    textinput.Model
	editDateType                 string // "app", "pub", "grant"
	noteTA                       textarea.Model
	spinner                      spinner.Model
	loading                      bool
	loadingMsg                   string
	lastVisualStart              int
	lastVisualEnd                int
	lastVisualValid              bool
	lastVisualNumbers            []string // patent numbers at save time; nil for non-list views
	cancel                       context.CancelFunc
	ProjectID                    string
	mode                         viewMode
	patents                      []domain.Patent
	totalPatents                 int // unfiltered count for the current project
	projects                     []domain.Project
	patentSelected               int
	projectSelected              int
	projectEventsSelected        int
	projectInvoicesSelected      int
	projectIDSSelected           int
	detailSelected               int
	citationLocalIdx             int
	citationKey               string
	citesTextFilter              string
	reviewSelected               int
	classificationSelected       int
	classificationDetailSelected classificationDetailRow
	inventorSelected             int
	familySelected               int
	visualMode                   bool
	selectionStart               int
	current                      domain.Patent
	pendingBundle                domain.PatentBundle
	pendingCitation              domain.CitationEdge
	reviewState                  string
	listFilter                   PatentFilter
	message                      string
	err                          string
	logger                       *slog.Logger
	text                         TextCatalog
	width                        int
	height                       int
	backStack                    []navSnapshot
	jumpMode                     bool
	countBuffer                  string
	sortColumn                   string
	sortOrder                    string
	sortColumn2                  string
	sortOrder2                   string
	citesReviewStateFilter       string // "" (all), "stored", "ignored", "under_review"
	numberColWidth                 int
	unpaidCounts                 map[string]int
	familyTreeCache              []familyNode
	familyTreeCacheFor           string
	familyPatentCache            map[string]domain.Patent
	familyPatentCacheMisses      map[string]bool
	helpQuery                    string
	helpSearchActive             bool
	helpScroll                   int
	fullTextScroll               int
	activityLog                  *slog.Logger
	importCfg                    config.Config
	detailCache                  detailCache
	jumpLabelsCache              []jumpLabel
	bulkAction                   bulkActionType
	bulkActionEdges              []domain.CitationEdge // citation bulk: stable edge IDs captured at selection
	bulkActionNumbers            []string              // patent bulk: stable patent numbers captured at selection
	sortColumnIndex              int
	classificationQuery          string
	classificationSearchActive   bool
	listSearchQuery              string
	listSearchActive             bool
	popupSearchQuery             string
	popupSearchActive            bool
	reviewStateSelected          int
	countrySelectSelected        int
	projectTagsSelected          int
	tagSelectSelected            int
	projectTags                  []domain.TagWithCount
	availableTags                []domain.Tag
	selectedPatentTags           map[int64]bool
	activeSelection              selectionContext
	history                      *changes.History
	dirty                        dirtyState
	familyRefreshElapsed         string
	overlayBackdropCache         string
	overlayBackdropCacheKey      string
	version                      string
	activeKeys                   KeyMap
}

type detailCache struct {
	Number              string
	CitationCount       int
	CitationRefreshedAt time.Time
	CitedByCount        int
	CitedByRefreshedAt  time.Time
	ExpectedCitations   int
	ExpectedCitedBy     int
	Parents             []domain.FamilyEdge
	Children            []domain.FamilyEdge
	Notes               []domain.ResearchNote
	IDSEntries          []domain.IDSEntry
	Tags                []domain.Tag
}

type navSnapshot struct {
	mode                         viewMode
	patents                      []domain.Patent
	projects                     []domain.Project
	patentSelected               int
	projectSelected              int
	projectEventsSelected        int
	projectInvoicesSelected      int
	projectIDSSelected           int
	detailSelected               int
	citationLocalIdx             int
	citationKey               string
	citesTextFilter              string
	reviewSelected               int
	classificationSelected       int
	classificationDetailSelected classificationDetailRow
	inventorSelected             int
	familySelected               int
	visualMode                   bool
	selectionStart               int
	current                      domain.Patent
	pendingBundle                domain.PatentBundle
	pendingCitation              domain.CitationEdge
	reviewState                  string
	listFilter                   PatentFilter
	message                      string
	err                          string
	countBuffer                  string
	ProjectID                    string
	sortColumn                   string
	sortOrder                    string
	sortColumn2                  string
	sortOrder2                   string
	citesReviewStateFilter       string
	numberColWidth                 int
	classificationQuery          string
	classificationSearchActive   bool
	listSearchQuery              string
	listSearchActive             bool
	popupSearchQuery             string
	popupSearchActive            bool
	reviewStateSelected          int
	width                        int
}

func New(ctx context.Context, repo storage.Repository, logger *slog.Logger, activityLog *slog.Logger, cfg config.Config, version string) *Model {
	input := textinput.New()
	input.Placeholder = ":add US11611785B2, :open US11611785B2, /machine learning"
	input.Prompt = EmptyPrompt
	input.CharLimit = commandInputCharLimit

	ta := textarea.New()
	ta.Placeholder = "Write your research note..."
	ta.SetWidth(noteTextareaMinWidth)
	ta.SetHeight(noteTextareaHeight)
	ta.CharLimit = noteTextareaCharLimit
	ta.ShowLineNumbers = false

	di := textinput.New()
	di.Placeholder = "YYYY-MM-DD"
	di.CharLimit = dateInputCharLimit

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAccent))

	projectID := DefaultProjectID
	if last, err := repo.GetSetting(ctx, SettingLastProjectID); err == nil && last != "" {
		projectID = last
	}

	patents, _ := repo.ListPatents(ctx, projectID, storage.ListPatentsOptions{
		ReviewStateFilter: domain.ReviewStateStored,
		SortColumn:        EmptySortColumn,
		SortOrder:         EmptySortOrder,
	})
	allPatents, _ := repo.ListPatents(ctx, projectID, storage.ListPatentsOptions{
		ReviewStateFilter: storage.ReviewStateFilterNone,
	})
	if logger == nil {
		logger = slog.Default()
	}
	if activityLog == nil {
		activityLog = slog.Default()
	}
	model := &Model{
		ctx:                ctx,
		repo:               repo,
		input:              input,
		dateInput:          di,
		noteTA:             ta,
		spinner:            s,
		ProjectID:          projectID,
		mode:               viewSplash,
		patents:            patents,
		totalPatents:       len(allPatents),
		projectSelected:    0,
		logger:             logger,
		activityLog:        activityLog,
		text:               EnglishText(),
		listFilter:         defaultPatentFilter(),
		importCfg:          cfg,
		version:            version,
		projectTags:        []domain.TagWithCount{},
		availableTags:      []domain.Tag{},
		selectedPatentTags: make(map[int64]bool),
		history:            changes.NewHistory(repo),
	}

	if model.importCfg.ImportSource == config.ImportSourceUSPTO && model.importCfg.USPTO.APIKey == "" {
		model.importCfg.ImportSource = config.ImportSourceGoogle
		model.mode = viewUSPTOKeyWarning
	}

	model = model.reloadProjects()
	for i, p := range model.projects {
		if p.ID == projectID {
			model.projectSelected = i
			break
		}
	}
	if len(patents) > 0 {
		model.current = patents[0]
	}
	return model
}

type classificationEnrichedMsg struct {
	Number string
}

type refreshResultMsg struct {
	err             error
	message         string
	patent          domain.Patent
	mode            viewMode
	citationLocalIdx int
	familySelected   int
	elapsed         time.Duration
	withDetails     bool
	action          string
	source          string
}

type refreshDetailsResultMsg struct {
	err     error
	message string
}

func (m *Model) Init() tea.Cmd {
	return nil
}

// Update handles a message and then flushes any cache invalidations a change
// applied during the turn — so screen and DB never drift, no matter which
// branch the handler returned from.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := m.dispatch(msg)
	if mm, ok := model.(*Model); ok {
		return mm.flushDirty(), cmd
	}
	return model, cmd
}

func (m *Model) dispatch(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.repo == nil {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		taWidth := m.overlayWidth() - 4
		if taWidth < noteTextareaMinWidth {
			taWidth = noteTextareaMinWidth
		}
		m.noteTA.SetWidth(taWidth)
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case refreshResultMsg:
		m.loading = false
		m.cancel = nil
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		if msg.action != "" && msg.patent.Number != "" {
			m.logActivity(msg.action, msg.patent.Number, msg.source)
		}
		m.current = msg.patent
		m.populateDetailCache()
		m.setMode(msg.mode)
		m.citationLocalIdx = msg.citationLocalIdx
		m.familySelected = msg.familySelected
		m.message = msg.message
		if msg.action == ActivityFamilyRefresh && msg.elapsed > 0 {
			m.familyRefreshElapsed = formatElapsedHint(msg.elapsed)
			if m.logger != nil {
				m.logger.Info("family refresh completed", "patent", msg.patent.Number, "duration_ms", msg.elapsed.Milliseconds())
			}
		}
		updated, listCmd := m.refreshList()
		if msg.withDetails {
			detailsModel, detailsCmd := updated.(*Model).refreshVisibleCitationDetails()
			return detailsModel, tea.Batch(listCmd, detailsCmd)
		}
		return updated, listCmd
	case refreshDetailsResultMsg:
		m.loading = false
		m.cancel = nil
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.message = msg.message
		return m, nil
	case browserOpenedMsg:
		m.message = fmt.Sprintf(m.text.T(TextMessageBrowserOpened), msg.URL)
		m.err = ""
	case browserOpenFailedMsg:
		m.err = msg.Err.Error()
		m.message = ""
	case classificationEnrichedMsg:
		if m.current.Number == msg.Number {
			if p, err := m.repo.GetPatent(m.ctx, m.ProjectID, msg.Number); err == nil {
				m.current = p
				m.populateDetailCache()
			}
		}
		return m, nil
	case tea.KeyMsg:
		if m.mode == viewUSPTOKeyWarning {
			if msg.String() == keyCtrlC {
				return m, tea.Quit
			}
			m.setMode(viewSplash)
			return m, nil
		}
		if m.input.Focused() {
			switch msg.String() {
			case keyEnter:
				command := m.input.Value()
				m.input.Blur()
				m.input.SetValue("")
				return m.runCommand(ParseCommand(command))
			case keyEsc:
				m.input.Blur()
				m.input.SetValue("")
				return m, nil
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}

		if m.mode == viewDateEdit {
			return m.handleViewDateEditKey(msg)
		}

		if m.mode == viewReviewStateSelect {
			return m.handleViewReviewStateSelectKey(msg)
		}

		if m.mode == viewCountrySelect {
			return m.handleViewCountrySelectKey(msg)
		}

		// Mode-specific key handlers
		if m.mode == viewSplash {
			return m.handleViewSplashKey(msg)
		}
		if m.mode == viewProjectInfo {
			return m.handleViewProjectInfoKey(msg)
		}
		if m.mode == viewIDSEdit {
			return m.handleViewIDSEditKey(msg)
		}
		if m.mode == viewProjectEvents {
			return m.handleViewProjectEventsKey(msg)
		}
		if m.mode == viewProjectInvoices {
			return m.handleViewProjectInvoicesKey(msg)
		}
		if m.mode == viewProjectIDS {
			return m.handleViewProjectIDSKey(msg)
		}
		if m.mode == viewProjectTags {
			return m.handleViewProjectTagsKey(msg)
		}
		if m.mode == viewTagSelect {
			return m.handleViewTagSelectKey(msg)
		}
		if m.mode == viewNoteEdit {
			return m.handleViewNoteEditKey(msg)
		}
		if m.mode == viewFullText {
			return m.handleViewFullTextKey(msg)
		}
		if m.mode == viewHelp || m.mode == viewKeymap {
			return m.handleViewHelpKey(msg)
		}

		if m.mode == viewList && m.listSearchActive {
			switch msg.String() {
			case keyEsc, keyBack:
				m.listSearchActive = false
				m.listSearchQuery = ""
				return m, nil
			case keyBackspace, keyCtrlH:
				if len(m.listSearchQuery) > 0 {
					m.listSearchQuery = m.listSearchQuery[:len(m.listSearchQuery)-1]
					if m.listSearchQuery != "" {
						return m.listSearchFirst(), nil
					}
				}
				return m, nil
			case keyEnter:
				query := m.listSearchQuery
				m.listSearchActive = false
				m.listSearchQuery = ""
				if query != "" {
					m.listFilter.FreeFormSearch = query
					return m.refreshList()
				}
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.listSearchQuery += msg.String()
					return m.listSearchFirst(), nil
				}
				return m, nil
			}
		}

		if m.isPopupSearchMode() && m.popupSearchActive {
			switch msg.String() {
			case keyEsc, keyBack:
				m.popupSearchActive = false
				m.popupSearchQuery = ""
				return m, nil
			case keyBackspace, keyCtrlH:
				if len(m.popupSearchQuery) > 0 {
					m.popupSearchQuery = m.popupSearchQuery[:len(m.popupSearchQuery)-1]
					if m.popupSearchQuery != "" {
						return m.popupSearchFirst(), nil
					}
				}
				return m, nil
			case keyEnter:
				if m.isCitationView() {
					m.citesTextFilter = m.popupSearchQuery
					m.popupSearchQuery = ""
				}
				m.popupSearchActive = false
				return m, nil
			default:
				if len(msg.String()) == 1 {
					m.popupSearchQuery += msg.String()
					return m.popupSearchFirst(), nil
				}
				return m, nil
			}
		}

		if m.jumpMode {
			if msg.String() == keyEsc || msg.String() == keyJump {
				m.jumpMode = false
				return m, nil
			}
			return m.applyJump(msg.String())
		}

		switch msg.String() {
		case keyFirstClaim:
			if m.mode == viewDetail && m.current.FirstClaim != "" {
				m.detailSelected = m.indexJumpLabel(jumpLabelFirstClaim)
				return m.navigateTo(viewClaim), nil
			}
			m.countBuffer += msg.String()
			return m, nil
		case keyEditSummary:
			if m.mode == viewDetail && m.current.Number != "" {
				m.detailSelected = m.indexJumpLabel(jumpLabelAbstract)
				return m.navigateTo(viewAbstract), nil
			}
		default:
			if isCountKey(msg.String()) {
				// Don't start a count with '0'
				if msg.String() == "0" && m.countBuffer == "" {
					return m, nil
				}
				m.countBuffer += msg.String()
				return m, nil
			}
		}

		switch msg.String() {
		case keyCtrlC:
			return m, tea.Quit
		case keyEsc, keyBack:
			if m.visualMode {
				m.clearVisualMode()
				return m, nil
			}
			m.countBuffer = EmptyCount
			return m.goBack()
		case keyQuit:
			return m, tea.Quit
		case keyIDS:
			if m.mode == viewDetail && m.current.Number != "" {
				m.detailSelected = m.indexJumpLabel(keyIDS)
				return m.openCurrentPatentIDSEdit(), nil
			}
			m.projectIDSSelected = 0
			return m.navigateTo(viewProjectIDS), nil
		case keyProject:
			m = m.navigateTo(viewSplash)
			m = m.reloadProjects()
			for i, p := range m.projects {
				if p.ID == m.ProjectID {
					m.projectSelected = i
					break
				}
			}
			return m, nil
		case keyCommand, keySearch:
			m.countBuffer = EmptyCount
			if msg.String() == keySearch && m.mode == viewList {
				m.listSearchActive = true
				m.listSearchQuery = ""
				return m, nil
			}
			if msg.String() == keySearch && m.isPopupSearchMode() {
				m.popupSearchActive = true
				m.popupSearchQuery = ""
				return m, nil
			}
			m.input.Focus()
			m.input.SetValue(msg.String())
			return m, nil
		case keyEnter, keyOpen:
			m.countBuffer = EmptyCount
			if m.mode == viewPopupPatentDetail {
				return m.storePendingPatent()
			}
			if m.mode == viewHelpPopup || m.mode == viewKeymapPopup {
				return m.goBack()
			}
			if m.mode == viewConfirmDelete {
				return m.deleteSelectedPatent()
			}
			if m.mode == viewClassificationDetail {
				return m.filterBySelectedClassification()
			}
			if m.mode == viewInventors {
				return m.filterBySelectedInventor()
			}
			if m.mode == viewFamily {
				return m.openSelectedFamilyMember()
			}
			if m.mode == viewClassifications {
				cls, _ := m.repo.ListClassifications(m.ctx, m.ProjectID, m.current.Number)
				if len(cls) > 0 {
					m.classificationDetailSelected = 0
					m = m.navigateTo(viewClassificationDetail)
				}
				return m, nil
			}
			if m.isCitationView() {
				return m.openSelectedCitation()
			}
			if m.mode == viewReview {
				return m.openSelectedReviewCitation()
			}
			if m.mode == viewDetail || m.mode == viewPopupPatentDetail {
				return m.filterBySelectedDetail()
			}
			if m.mode == viewList && len(m.patents) > 0 {
				return m.openPatent(m.patents[m.patentSelected].Number)
			}
		case keyDelete:
			if m.mode == viewFamily {
				return m.removeSelectedFamilyEdge()
			}
			if m.mode == viewList && len(m.patents) > 0 {
				if m.visualMode {
					indices := m.selectedIndices()
					if len(indices) > 1 {
						m.bulkAction = bulkActionDelete
						nums := make([]string, 0, len(indices))
						for _, idx := range indices {
							if idx >= 0 && idx < len(m.patents) {
								nums = append(nums, m.patents[idx].Number)
							}
						}
						m.bulkActionNumbers = nums
						m.clearVisualMode()
						return m.navigateTo(viewBulkConfirm), nil
					}
				}
				m.setMode(viewConfirmDelete)
				return m, nil
			}
		case keyVimDown, keyArrowDown:
			if m.mode == viewClassificationDetail {
				m.classificationDetailSelected = classificationDetailRow(clamp(int(m.classificationDetailSelected)+1, 0, int(classDetailRowCount-1)))
				return m, nil
			}
			count := m.consumeCount(1)
			return m.moveSelection(count), nil
		case keyVimUp, keyArrowUp:
			if m.mode == viewClassificationDetail {
				m.classificationDetailSelected = classificationDetailRow(clamp(int(m.classificationDetailSelected)-1, 0, int(classDetailRowCount-1)))
				return m, nil
			}
			count := m.consumeCount(1)
			return m.moveSelection(-count), nil
		case keyCtrlF, keyPgDown:
			m.countBuffer = EmptyCount
			return m.moveSelection(m.pageSize()), nil
		case keyCtrlD, keyPgUp:
			m.countBuffer = EmptyCount
			return m.moveSelection(-m.pageSize()), nil
		case keyGoto:
			if m.countBuffer == "g" {
				m.countBuffer = ""
				return m.goToRow(1), nil
			}
			if m.countBuffer != "" {
				count := m.consumeCount(1)
				return m.goToRow(count), nil
			}
			// wait for second 'g' for 'gg'
			m.countBuffer = "g"
			return m, nil
		case keyBottom:
			m.countBuffer = ""
			return m.goToRow(m.activeItemCount()), nil
		case keyJump:
			m.countBuffer = EmptyCount
			m.jumpMode = !m.jumpMode
			return m, nil
		case keyVisual, keyVisualLine:
			if m.countBuffer == "g" && msg.String() == keyVisual {
				m.countBuffer = ""
				return m.restoreVisualSelection(), nil
			}
			if m.mode == viewList || m.isCitationView() || m.mode == viewReview {
				if m.visualMode {
					m.clearVisualMode()
				} else {
					m.visualMode = true
					cur := m.activeSelectionIndex()
					m.selectionStart = cur
					m.trackVisualEnd(cur)
				}
			}
			return m, nil
		case keyVisualAll:
			if m.mode == viewList || m.isCitationView() || m.mode == viewReview {
				m.visualMode = true
				m.selectionStart = 0
				m.setActiveSelectionIndex(m.activeItemCount() - 1)
			}
			return m, nil
		case keyColLeft, "left":
			count := m.consumeCount(1)
			if cols, ok := m.tabularColumns(); ok {
				m.sortColumnIndex = clamp(m.sortColumnIndex-count, 0, len(cols)-1)
				return m, nil
			}
		case keyColRight, "right":
			count := m.consumeCount(1)
			if cols, ok := m.tabularColumns(); ok {
				m.sortColumnIndex = clamp(m.sortColumnIndex+count, 0, len(cols)-1)
				return m, nil
			}
		case keyClassification: // "L"
			if m.mode == viewList && len(m.patents) > 0 {
				m.current = m.patents[m.patentSelected]
				m.populateDetailCache()
			}
			return m.navigateTo(viewClassifications), nil
		case keyCites:
			m = m.navigateTo(viewCites)
		case keyCitedBy:
			m = m.navigateTo(viewCitedBy)
		case keyFamily:
			if m.mode == viewList && len(m.patents) > 0 {
				m.current = m.patents[m.patentSelected]
			}
			m = m.navigateTo(viewFamily)
			m.familySelected = familyCurrentIdx(m.buildFamilyTree())
		case keyTag:
			if m.mode == viewList && len(m.patents) > 0 {
				m.current = m.patents[m.patentSelected]
			}
			m.activeSelection = m.captureSelection()
			if m.activeSelection.livePatent != "" {
				m = m.navigateTo(viewTagSelect).reloadAvailableTags()
			}
		case keyFullText:
			if m.mode == viewList && len(m.patents) > 0 {
				m.current = m.patents[m.patentSelected]
			}
			m = m.navigateTo(viewFullText)
		case keyNarrow:
			if m.mode == viewList && m.listSearchQuery != "" {
				query := m.listSearchQuery
				m.listSearchQuery = ""
				m.listFilter.FreeFormSearch = query
				m.message = "filter: " + query
				return m.refreshList()
			}
		case keyNotes: // which is "N"
			if m.isPopupSearchMode() && m.popupSearchQuery != "" {
				return m.popupSearchNext(), nil
			}
			if m.mode == viewList && m.listSearchQuery != "" {
				return m.listSearchNext(), nil
			}
			if m.mode == viewBulkConfirm {
				m.bulkActionEdges = nil
				m.bulkActionNumbers = nil
				return m.goBack()
			}
			if m.mode == viewConfirmDelete {
				m.setMode(viewList)
				return m, nil
			}
			if m.mode == viewPopupPatentDetail {
				return m.skipPendingPatent()
			}
			m = m.navigateTo(viewNotes)
		case keyRefresh:
			if m.isCitationView() {
				return m.refreshSelectedCitationDetail()
			}
			if m.mode == viewFamily {
				return m.refreshSelectedFamilyMember()
			}
		case keyRefreshAll:
			if m.isCitationView() {
				return m.refreshVisibleCitationDetails()
			}
			if m.mode == viewFamily {
				return m.pullFamilyCommand()
			}
		case keyAI:
			m = m.navigateTo(viewAI)
		case keyWeb:
			return m.openBrowser(nil)
		case keyUndo:
			return m.undoLastChange()
		case keyRedo:
			return m.redoChange()
		case keyHelp:
			if m.mode == viewHelpPopup || m.mode == viewKeymapPopup {
				return m.goBack()
			}
			m = m.navigateTo(viewHelpPopup)
		case keyNoteEdit:
			if m.mode == viewList && len(m.patents) > 0 {
				m.current = m.patents[clamp(m.patentSelected, 0, len(m.patents)-1)]
			}
			if m.current.Number != "" {
				m.noteTA.Reset()
				m.noteTA.Focus()
				m = m.navigateTo(viewNoteEdit)
				var cmd tea.Cmd
				m.noteTA, cmd = m.noteTA.Update(nil)
				return m, cmd
			}
		case keyAddToIDS:
			var targetNumber string
			if m.mode == viewList && len(m.patents) > 0 {
				targetNumber = m.patents[clamp(m.patentSelected, 0, len(m.patents)-1)].Number
			} else if m.mode == viewDetail && m.current.Number != "" {
				targetNumber = m.current.Number
			}
			if targetNumber != "" {
				entry := domain.IDSEntry{
					ProjectID:    m.ProjectID,
					PatentNumber: targetNumber,
					Status:       domain.IDSStatusPending,
				}
				if _, err := m.repo.AddIDSEntry(m.ctx, entry); err != nil {
					m.err = err.Error()
				} else {
					m.logActivity(ActivityIDSAdd, targetNumber, "")
					m.message = "Added to IDS: " + targetNumber
				}
				return m, nil
			}
		case keySort:
			if cols, ok := m.tabularColumns(); ok {
				newCol := cols[clamp(m.sortColumnIndex, 0, len(cols)-1)].id
				if m.sortColumn == newCol {
					if m.sortOrder == domain.SortOrderAsc {
						m.sortOrder = domain.SortOrderDesc
					} else {
						m.sortOrder = domain.SortOrderAsc
					}
				} else {
					m.sortColumn = newCol
					m.sortOrder = domain.SortOrderAsc
				}
				if m.mode == viewList {
					m.clearVisualMode()
					return m.refreshList()
				}
				return m, nil
			}
		case keyReviewState:
			m.activeSelection = m.captureSelection()
			if m.isCitationView() {
				// Pre-select the review state of the first edge in the selection so
				// the picker opens with the cursor already on the current value.
				// In visual mode "first" is the lower bound of the selection range.
				m.reviewStateSelected = 0
				if edges, err := m.currentCitationEdges(); err == nil && len(edges) > 0 {
					firstIdx := m.citationLocalIdx
					if m.visualMode && m.selectionStart < firstIdx {
						firstIdx = m.selectionStart
					}
					firstIdx = clamp(firstIdx, 0, len(edges)-1)
					for i, s := range m.selectableReviewStates() {
						if s == edges[firstIdx].ReviewState {
							m.reviewStateSelected = i
							break
						}
					}
				}
				return m.navigateTo(viewReviewStateSelect), nil
			}
			if m.mode == viewFamily {
				nodes := m.buildFamilyTree()
				if m.familySelected < 0 || m.familySelected >= len(nodes) {
					return m, nil
				}
				node := nodes[m.familySelected]
				currentStatus := ""
				if p, ok := m.familyPatentCache[node.number]; ok {
					currentStatus = p.ReviewState
				} else if p, err := m.repo.GetPatent(m.ctx, m.ProjectID, node.number); err == nil {
					currentStatus = p.ReviewState
				}

				m.reviewStateSelected = 0
				for i, s := range m.selectableReviewStates() {
					if s == currentStatus {
						m.reviewStateSelected = i
						break
					}
				}
				return m.navigateTo(viewReviewStateSelect), nil
			}
			if m.mode == viewReview {
				edge, ok, err := m.selectedReviewCitationEdge()
				if err == nil && ok {
					currentStatus := edge.ReviewState
					m.reviewStateSelected = 0
					for i, s := range m.selectableReviewStates() {
						if s == currentStatus {
							m.reviewStateSelected = i
							break
						}
					}
				}
				return m.navigateTo(viewReviewStateSelect), nil
			}
			// Pre-select current status in the list
			currentStatus := ""
			if m.mode == viewDetail || m.mode == viewPopupPatentDetail {
				currentStatus = m.current.ReviewState
				m.detailSelected = m.indexJumpLabel(keyReviewState)
			} else if m.mode == viewList && len(m.patents) > 0 {
				currentStatus = m.patents[m.patentSelected].ReviewState
			}
			m.reviewStateSelected = 0
			if currentStatus != "" {
				for i, s := range m.selectableReviewStates() {
					if s == currentStatus {
						m.reviewStateSelected = i
						break
					}
				}
			}
			return m.navigateTo(viewReviewStateSelect), nil
		case keyCountry:
			m.countrySelectSelected = 0
			if m.listFilter.Country != EmptyFilter {
				for i, c := range m.selectableCountries() {
					if c == m.listFilter.Country {
						m.countrySelectSelected = i
						break
					}
				}
			}
			return m.navigateTo(viewCountrySelect), nil
		case keyPlus:
			if m.mode == viewFamily {
				m.input.Focus()
				m.input.SetValue(":family child ")
				return m, nil
			}
		case keyYes:
			if m.mode == viewBulkConfirm {
				return m.executeBulkAction()
			}
			if m.mode == viewConfirmDelete {
				return m.deleteSelectedPatent()
			}
			if m.mode == viewPopupPatentDetail {
				return m.storePendingPatent()
			}
			if m.isCitationView() {
				return m.storeSelectedCitation()
			}
			if m.mode == viewReview {
				return m.storeSelectedReviewCitation()
			}
		case keyIgnore:
			if m.mode == viewPopupPatentDetail {
				return m.updatePendingCitation(domain.ReviewStateIgnored, TextMessageIgnoredPatent)
			}
			if m.isCitationView() {
				return m.updateSelectedCitationReviewState(domain.ReviewStateIgnored, TextMessageIgnoredPatent)
			}
			if m.mode == viewReview {
				return m.updateSelectedReviewCitationReviewState(domain.ReviewStateIgnored, TextMessageIgnoredPatent)
			}
			return m.navigateTo(viewProjectInfo), nil
		case keyUnreview:
			if m.mode == viewPopupPatentDetail {
				return m.updatePendingCitation(domain.ReviewStateUnderReview, TextMessageUnderReviewPatent)
			}
			if m.isCitationView() {
				return m.updateSelectedCitationReviewState(domain.ReviewStateUnderReview, TextMessageUnderReviewPatent)
			}
			if m.mode == viewReview {
				return m.updateSelectedReviewCitationReviewState(domain.ReviewStateUnderReview, TextMessageUnderReviewPatent)
			}
		}
	}
	return m, nil
}
