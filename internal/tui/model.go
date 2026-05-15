package tui

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	viewText                 viewMode = "full text"
	viewNotes                viewMode = "notes"
	viewRefs                 viewMode = "references"
	viewAI                   viewMode = "ai"
	viewHelp                 viewMode = "help"
	viewHelpPopup            viewMode = "help-popup"
	viewKeymap               viewMode = "keymap"
	viewKeymapPopup          viewMode = "keymap-popup"
	viewPreview              viewMode = "preview"
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
	viewReviewStateSelect         viewMode = "status-select"
	viewProjectTags          viewMode = "project-tags"
	viewTagSelect            viewMode = "tag-select"
	viewCountrySelect        viewMode = "country-select"
)

type bulkActionType string

const (
	bulkActionStore       bulkActionType = "store"
	bulkActionIgnore      bulkActionType = "ignore"
	bulkActionUnderReview bulkActionType = "under_review"
)

type Model struct {
	ctx                        context.Context
	repo                       storage.Repository
	input                      textinput.Model
	dateInput                  textinput.Model
	editDateType               string // "app", "pub", "grant"
	noteTA                     textarea.Model
	spinner                    spinner.Model
	loading                    bool
	loadingMsg                 string
	cancel                     context.CancelFunc
	ProjectID                  string
	mode                       viewMode
	patents                    []domain.Patent
	totalPatents               int // unfiltered count for the current project
	projects                   []domain.Project
	selected                   int
	projectSelected            int
	projectEventsSelected      int
	projectInvoicesSelected    int
	projectIDSSelected         int
	detailSelected             int
	citesSelected              int
	citedBySelected            int
	reviewSelected             int
	classificationSelected     int
	classificationDetailSelected classificationDetailRow
	inventorSelected           int
	familySelected             int
	visualMode                 bool
	selectionStart             int
	current                    domain.Patent
	pendingBundle              domain.PatentBundle
	pendingCitation            domain.CitationEdge
	reviewState               string
	listFilter                 PatentFilter
	message                    string
	err                        string
	logger                     *slog.Logger
	text                       TextCatalog
	width                      int
	height                     int
	backStack                  []navSnapshot
	jumpMode                   bool
	countBuffer                string
	sortColumn                 string
	sortOrder                  string
	sortColumn2                string
	sortOrder2                 string
	citesReviewStateFilter     string // "" (all), "stored", "ignored", "under_review"
	listNumWidth               int
	unpaidCounts               map[string]int
	familyTreeCache            []familyNode
	familyTreeCacheFor         string
	familyPatentCache          map[string]domain.Patent
	familyPatentCacheMisses    map[string]bool
	helpQuery                  string
	helpSearchActive           bool
	helpScroll                 int
	activityLog                *slog.Logger
	importCfg                  config.Config
	detailCache                detailCache
	jumpLabelsCache            []jumpLabel
	bulkAction                 bulkActionType
	bulkActionIndices          []int
	sortColumnIndex            int
	classificationQuery        string
	classificationSearchActive bool
	listSearchQuery            string
	listSearchActive           bool
	popupSearchQuery           string
	popupSearchActive          bool
	reviewStateSelected             int
	countrySelectSelected           int
	projectTagsSelected        int
	tagSelectSelected          int
	projectTags                []domain.TagWithCount
	availableTags              []domain.Tag
	selectedPatentTags         map[int64]bool
	familyRefreshElapsed       string
	overlayBackdropCache       string
	overlayBackdropCacheKey    string
	version                    string
	activeKeys                 KeyMap
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
	mode                       viewMode
	patents                    []domain.Patent
	projects                   []domain.Project
	selected                   int
	projectSelected            int
	projectEventsSelected      int
	projectInvoicesSelected    int
	projectIDSSelected         int
	detailSelected             int
	citesSelected              int
	citedBySelected            int
	reviewSelected             int
	classificationSelected     int
	classificationDetailSelected classificationDetailRow
	inventorSelected           int
	familySelected             int
	visualMode                 bool
	selectionStart             int
	current                    domain.Patent
	pendingBundle              domain.PatentBundle
	pendingCitation            domain.CitationEdge
	reviewState                string
	listFilter                 PatentFilter
	message                    string
	err                        string
	countBuffer                string
	ProjectID                  string
	sortColumn                 string
	sortOrder                  string
	sortColumn2                string
	sortOrder2                 string
	citesReviewStateFilter     string
	listNumWidth               int
	classificationQuery        string
	classificationSearchActive bool
	listSearchQuery            string
	listSearchActive           bool
	popupSearchQuery           string
	popupSearchActive          bool
	reviewStateSelected             int
	width                      int
}

func New(ctx context.Context, repo storage.Repository, logger *slog.Logger, activityLog *slog.Logger, cfg config.Config, version string) *Model {
	input := textinput.New()
	input.Placeholder = ":add US11611785B2, :open US11611785B2, /machine learning"
	input.Prompt = EmptyPrompt
	input.CharLimit = 512

	ta := textarea.New()
	ta.Placeholder = "Write your research note..."
	ta.SetWidth(noteTextareaMinWidth)
	ta.SetHeight(noteTextareaHeight)
	ta.CharLimit = noteTextareaCharLimit
	ta.ShowLineNumbers = false

	di := textinput.New()
	di.Placeholder = "YYYY-MM-DD"
	di.CharLimit = 10

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
		ctx:             ctx,
		repo:            repo,
		input:           input,
		dateInput:       di,
		noteTA:          ta,
		spinner:         s,
		ProjectID:       projectID,
		mode:            viewSplash,
		patents:           patents,
		totalPatents:      len(allPatents),
		projectSelected:   0,
		logger:            logger,
		activityLog:       activityLog,
		text:              EnglishText(),
		listFilter:        defaultPatentFilter(),
		importCfg:       cfg,
		version:                version,
		projectTags:            []domain.TagWithCount{},
		availableTags:          []domain.Tag{},
		selectedPatentTags:     make(map[int64]bool),
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
	citesSelected   int
	citedBySelected int
	familySelected  int
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

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.citesSelected = msg.citesSelected
		m.citedBySelected = msg.citedBySelected
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
			switch msg.String() {
			case keyEnter:
				val := m.dateInput.Value()
				m.dateInput.Blur()
				return m.patentDateCommand([]string{m.editDateType, val})
			case keyEsc, keyBack:
				m.dateInput.Blur()
				return m.goBack()
			}
			var cmd tea.Cmd
			m.dateInput, cmd = m.dateInput.Update(msg)
			return m, cmd
		}

		if m.mode == viewReviewStateSelect {
			switch msg.String() {
			case keyVimDown, keyArrowDown:
				m.reviewStateSelected = clamp(m.reviewStateSelected+1, 0, len(m.selectableReviewStates())-1)
			case keyVimUp, keyArrowUp:
				m.reviewStateSelected = clamp(m.reviewStateSelected-1, 0, len(m.selectableReviewStates())-1)
			case keyEnter:
				return m.applyReviewStateSelection()
			case keyEsc, keyBack:
				return m.goBack()
			}
			return m, nil
		}

		if m.mode == viewCountrySelect {
			switch msg.String() {
			case keyVimDown, keyArrowDown:
				m.countrySelectSelected = clamp(m.countrySelectSelected+1, 0, len(m.selectableCountries())-1)
			case keyVimUp, keyArrowUp:
				m.countrySelectSelected = clamp(m.countrySelectSelected-1, 0, len(m.selectableCountries())-1)
			case keyEnter:
				return m.applyCountrySelection()
			case keyEsc, keyBack:
				return m.goBack()
			}
			return m, nil
		}

		// Mode-specific key handlers
		if m.mode == viewSplash {
			switch msg.String() {
			case keyVimDown, keyArrowDown:
				m.projectSelected = clamp(m.projectSelected+1, 0, len(m.projects)-1)
			case keyVimUp, keyArrowUp:
				m.projectSelected = clamp(m.projectSelected-1, 0, len(m.projects)-1)
			case keyEnter:
				if len(m.projects) > 0 {
					m.ProjectID = m.projects[m.projectSelected].ID
					m.saveLastProject()
					m.setMode(viewList)
					return m.refreshList()
				}
			case keyEvents:
				if len(m.projects) > 0 {
					m.ProjectID = m.projects[m.projectSelected].ID
				}
				m.projectEventsSelected = 0
				m.setMode(viewProjectEvents)
				return m, nil
			case keyInvoices:
				if len(m.projects) > 0 {
					m.ProjectID = m.projects[m.projectSelected].ID
				}
				m.projectInvoicesSelected = 0
				m.setMode(viewProjectInvoices)
				return m, nil
			case keyIDS:
				if len(m.projects) > 0 {
					m.ProjectID = m.projects[m.projectSelected].ID
				}
				m.projectIDSSelected = 0
				m.setMode(viewProjectIDS)
				return m, nil
			case keyNew:
				m.input.Focus()
				m.input.SetValue(":project create ")
				return m, nil
			case keyQuit:
				return m, tea.Quit
			case keyEsc:
				return m.goBack()
			}
			return m, nil
		}
		if m.mode == viewProjectInfo {
			switch msg.String() {
			case keyEsc, keyBack, keyProjectInfo:
				return m.goBack()
			case keyEditAppStatus:
				m.input.Focus()
				m.input.SetValue(":project summary-status ")
				return m, nil
			case keyEditSummary:
				m.input.Focus()
				m.input.SetValue(":project summary ")
				return m, nil
			case keyEditComment:
				m.input.Focus()
				m.input.SetValue(":project comment ")
				return m, nil
			case keyEditProjectStatus:
				m.input.Focus()
				m.input.SetValue(":project status ")
				return m, nil
			}
			return m, nil
		}
		if m.mode == viewIDSEdit {
			switch msg.String() {
			case keyEsc, keyBack:
				return m.goBack()
			case "s":
				return m.cycleCurrentPatentIDSStatus()
			case "n":
				entry := m.idsEntryForPatent(m.current.Number)
				if entry != nil {
					m.input.Focus()
					m.input.SetValue(":ids note " + entry.Notes)
					m.input.CursorEnd()
				}
			case "k":
				entry := m.idsEntryForPatent(m.current.Number)
				if entry != nil {
					m.input.Focus()
					m.input.SetValue(":ids kind " + entry.KindCode)
					m.input.CursorEnd()
				}
			case "c":
				entry := m.idsEntryForPatent(m.current.Number)
				if entry != nil {
					m.input.Focus()
					m.input.SetValue(":ids country " + entry.CountryCode)
					m.input.CursorEnd()
				}
			case "p":
				entry := m.idsEntryForPatent(m.current.Number)
				if entry != nil {
					m.input.Focus()
					m.input.SetValue(":ids passages " + entry.RelevantPassages)
					m.input.CursorEnd()
				}
			case "f":
				return m.idsEditCommand([]string{idsEditSubFull})
			case keyDelete:
				entry := m.idsEntryForPatent(m.current.Number)
				if entry != nil {
					if err := m.repo.DeleteIDSEntry(m.ctx, entry.ID); err != nil {
						m.err = err.Error()
					} else {
						m.populateDetailCache()
						m.message = "IDS entry removed"
						return m.goBack()
					}
				}
			}
			return m, nil
		}
		if m.mode == viewProjectEvents {
			switch msg.String() {
			case keyVimDown, keyArrowDown:
				events, _ := m.repo.ListProjectEvents(m.ctx, m.ProjectID)
				m.projectEventsSelected = clamp(m.projectEventsSelected+1, 0, len(events)-1)
			case keyVimUp, keyArrowUp:
				m.projectEventsSelected = clamp(m.projectEventsSelected-1, 0, 0)
			case keyDelete:
				events, _ := m.repo.ListProjectEvents(m.ctx, m.ProjectID)
				if m.projectEventsSelected >= 0 && m.projectEventsSelected < len(events) {
					_ = m.repo.DeleteProjectEvent(m.ctx, events[m.projectEventsSelected].ID)
					m.projectEventsSelected = 0
					m.message = "event deleted"
				}
			case keyEsc, keyBack:
				return m.goBack()
			}
			return m, nil
		}
		if m.mode == viewProjectInvoices {
			switch msg.String() {
			case keyVimDown, keyArrowDown:
				invoices, _ := m.repo.ListProjectInvoices(m.ctx, m.ProjectID)
				m.projectInvoicesSelected = clamp(m.projectInvoicesSelected+1, 0, len(invoices)-1)
			case keyVimUp, keyArrowUp:
				m.projectInvoicesSelected = clamp(m.projectInvoicesSelected-1, 0, 0)
			case keyDelete:
				invoices, _ := m.repo.ListProjectInvoices(m.ctx, m.ProjectID)
				if m.projectInvoicesSelected >= 0 && m.projectInvoicesSelected < len(invoices) {
					_ = m.repo.DeleteProjectInvoice(m.ctx, invoices[m.projectInvoicesSelected].ID)
					m.projectInvoicesSelected = 0
					m.message = "invoice deleted"
					m = m.reloadProjects()
				}
			case keyMarkPaid:
				invoices, _ := m.repo.ListProjectInvoices(m.ctx, m.ProjectID)
				if m.projectInvoicesSelected >= 0 && m.projectInvoicesSelected < len(invoices) {
					inv := invoices[m.projectInvoicesSelected]
					inv.Status = domain.InvoiceStatusPaid
					_ = m.repo.UpdateProjectInvoice(m.ctx, inv)
					m.message = "marked as paid"
					m = m.reloadProjects()
				}
			case keyEsc, keyBack:
				return m.goBack()
			}
			return m, nil
		}
		if m.mode == viewProjectIDS {
			switch msg.String() {
			case keyVimDown, keyArrowDown:
				ids, _ := m.repo.ListIDSEntries(m.ctx, m.ProjectID)
				m.projectIDSSelected = clamp(m.projectIDSSelected+1, 0, len(ids)-1)
			case keyVimUp, keyArrowUp:
				ids, _ := m.repo.ListIDSEntries(m.ctx, m.ProjectID)
				m.projectIDSSelected = clamp(m.projectIDSSelected-1, 0, max(0, len(ids)-1))
			case "s":
				ids, _ := m.repo.ListIDSEntries(m.ctx, m.ProjectID)
				if m.projectIDSSelected >= 0 && m.projectIDSSelected < len(ids) {
					entry := ids[m.projectIDSSelected]
					next := nextIDSStatus(entry.Status)
					if next == "" {
						if err := m.repo.DeleteIDSEntry(m.ctx, entry.ID); err != nil {
							m.err = err.Error()
						} else {
							m.projectIDSSelected = clamp(m.projectIDSSelected, 0, max(0, len(ids)-2))
							m.logActivity(ActivityIDSRemove, entry.PatentNumber, "")
							m.message = "IDS entry removed"
						}
					} else if err := m.repo.UpdateIDSEntryStatus(m.ctx, entry.ID, next); err != nil {
						m.err = err.Error()
					} else {
						m.logActivity(ActivityIDSStatus, entry.PatentNumber, string(next))
						m.message = "IDS status: " + string(next)
					}
				}
			case keyDelete:
				ids, _ := m.repo.ListIDSEntries(m.ctx, m.ProjectID)
				if m.projectIDSSelected >= 0 && m.projectIDSSelected < len(ids) {
					_ = m.repo.DeleteIDSEntry(m.ctx, ids[m.projectIDSSelected].ID)
					m.projectIDSSelected = 0
					m.message = "IDS entry removed"
				}
			case keyEsc, keyBack:
				return m.goBack()
			}
			return m, nil
		}
		if m.mode == viewProjectTags {
			switch msg.String() {
			case keyVimDown, keyArrowDown:
				m.projectTagsSelected = clamp(m.projectTagsSelected+1, 0, len(m.projectTags)-1)
			case keyVimUp, keyArrowUp:
				m.projectTagsSelected = clamp(m.projectTagsSelected-1, 0, max(0, len(m.projectTags)-1))
			case keyDelete:
				if m.projectTagsSelected >= 0 && m.projectTagsSelected < len(m.projectTags) {
					tag := m.projectTags[m.projectTagsSelected]
					if err := m.repo.DeleteTag(m.ctx, tag.ID); err != nil {
						m.err = err.Error()
					} else {
						m.message = fmt.Sprintf("tag '%s' deleted", tag.Name)
						m.reloadProjectTags()
					}
				}
			case "r":
				if m.projectTagsSelected >= 0 && m.projectTagsSelected < len(m.projectTags) {
					tag := m.projectTags[m.projectTagsSelected]
					m.input.Focus()
					m.input.SetValue(fmt.Sprintf(":tag rename %s ", tag.Name))
					return m, nil
				}
			case keyEsc, keyBack:
				return m.goBack()
			}
			return m, nil
		}
		if m.mode == viewTagSelect {
			switch msg.String() {
			case keyVimDown, keyArrowDown:
				m.tagSelectSelected = clamp(m.tagSelectSelected+1, 0, len(m.availableTags)-1)
			case keyVimUp, keyArrowUp:
				m.tagSelectSelected = clamp(m.tagSelectSelected-1, 0, max(0, len(m.availableTags)-1))
			case " ":
				if m.tagSelectSelected >= 0 && m.tagSelectSelected < len(m.availableTags) {
					tag := m.availableTags[m.tagSelectSelected]
					if m.selectedPatentTags[tag.ID] {
						if err := m.repo.RemoveTagFromPatent(m.ctx, m.current.Number, tag.ID); err != nil {
							m.err = err.Error()
						} else {
							m.selectedPatentTags[tag.ID] = false
						}
					} else {
						if err := m.repo.ApplyTagToPatent(m.ctx, m.current.Number, tag.ID); err != nil {
							m.err = err.Error()
						} else {
							m.selectedPatentTags[tag.ID] = true
						}
					}
				}
			case keyEnter, "ctrl+s":
				m, cmd := m.goBack()
				m.(*Model).populateDetailCache()
				return m, cmd
			case keyEsc, keyBack:
				return m.goBack()
			}
			return m, nil
		}
		if m.mode == viewNoteEdit {
			switch msg.String() {
			case keyCtrlS:
				body := strings.TrimSpace(m.noteTA.Value())
				if body != "" {
					stamp := time.Now().Format("2006-01-02 15:04")
					body = fmt.Sprintf("[%s]\n%s", stamp, body)

					if _, err := m.repo.AddNote(m.ctx, m.ProjectID, m.current.Number, body); err != nil {
						m.err = err.Error()
					} else {
						m.logActivity(ActivityNoteAdd, m.current.Number, "")
						m.message = "note saved"
					}
				}
				m.noteTA.Reset()
				m.noteTA.Blur()
				return m.goBack()
			case keyEsc:
				m.noteTA.Reset()
				m.noteTA.Blur()
				return m.goBack()
			default:
				var cmd tea.Cmd
				m.noteTA, cmd = m.noteTA.Update(msg)
				return m, cmd
			}
		}
		if m.mode == viewHelp || m.mode == viewKeymap {
			if m.helpSearchActive {
				switch msg.String() {
				case keyEsc, keyBack:
					m.helpSearchActive = false
					m.helpQuery = ""
					m.helpScroll = 0
					return m, nil
				case "backspace", "ctrl+h":
					if len(m.helpQuery) > 0 {
						m.helpQuery = m.helpQuery[:len(m.helpQuery)-1]
						m.helpScroll = 0
					}
					return m, nil
				case "enter":
					m.helpSearchActive = false
					return m, nil
				default:
					if len(msg.String()) == 1 {
						m.helpQuery += msg.String()
						m.helpScroll = 0
					}
					return m, nil
				}
			}
			switch msg.String() {
			case "/":
				m.helpSearchActive = true
				m.helpQuery = ""
				return m, nil
			case "j", "down":
				m.helpScroll++
				return m, nil
			case "k", "up":
				if m.helpScroll > 0 {
					m.helpScroll--
				}
				return m, nil
			case "pgdown", "ctrl+f", "ctrl+d":
				m.helpScroll += m.pageSize() - 4
				return m, nil
			case "pgup", "ctrl+b", "ctrl+u":
				m.helpScroll -= m.pageSize() - 4
				if m.helpScroll < 0 {
					m.helpScroll = 0
				}
				return m, nil
			case keyEsc, keyBack:
				m.helpQuery = ""
				m.helpSearchActive = false
				m.helpScroll = 0
				return m.goBack()
			}
		}

		if m.mode == viewList && m.listSearchActive {
			switch msg.String() {
			case keyEsc, keyBack:
				m.listSearchActive = false
				m.listSearchQuery = ""
				return m, nil
			case "backspace", "ctrl+h":
				if len(m.listSearchQuery) > 0 {
					m.listSearchQuery = m.listSearchQuery[:len(m.listSearchQuery)-1]
					if m.listSearchQuery != "" {
						return m.listSearchFirst(), nil
					}
				}
				return m, nil
			case keyEnter:
				m.listSearchActive = false
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
			case "backspace", "ctrl+h":
				if len(m.popupSearchQuery) > 0 {
					m.popupSearchQuery = m.popupSearchQuery[:len(m.popupSearchQuery)-1]
					if m.popupSearchQuery != "" {
						return m.popupSearchFirst(), nil
					}
				}
				return m, nil
			case keyEnter:
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
		case keyText:
			if m.mode == viewDetail {
				m.detailSelected = m.indexJumpLabel(jumpLabelTags)
				return m, nil
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
				m.visualMode = false
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
			if m.mode == viewPreview {
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
			if m.mode == viewDetail {
				return m.filterBySelectedDetail()
			}
			if m.mode == viewList && len(m.patents) > 0 {
				return m.openPatent(m.patents[m.selected].Number)
			}
		case keyDelete:
			if m.mode == viewFamily {
				return m.removeSelectedFamilyEdge()
			}
			if m.mode == viewList && len(m.patents) > 0 {
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
		case keyCtrlF, "pgdown":
			m.countBuffer = EmptyCount
			return m.moveSelection(m.pageSize()), nil
		case keyCtrlD, "pgup":
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
		case "v", "V":
			if m.mode == viewList || m.isCitationView() || m.mode == viewReview {
				m.visualMode = !m.visualMode
				if m.visualMode {
					m.selectionStart = m.activeSelectionIndex()
				}
			}
			return m, nil
		case "%":
			if m.mode == viewList || m.isCitationView() || m.mode == viewReview {
				m.visualMode = true
				m.selectionStart = 0
				m.setActiveSelectionIndex(m.activeItemCount() - 1)
			}
			return m, nil
		case keyColLeft, "left":
			count := m.consumeCount(1)
			if m.mode == viewList {
				m.sortColumnIndex = clamp(m.sortColumnIndex-count, 0, len(m.listColumns())-1)
				return m, nil
			}
		case keyColRight, "right":
			count := m.consumeCount(1)
			if m.mode == viewList {
				m.sortColumnIndex = clamp(m.sortColumnIndex+count, 0, len(m.listColumns())-1)
				return m, nil
			}
		case keyClassification: // "L"
			if m.mode == viewList && len(m.patents) > 0 {
				m.current = m.patents[m.selected]
				m.populateDetailCache()
			}
			return m.navigateTo(viewClassifications), nil
		case keyCites:
			m = m.navigateTo(viewCites)
		case keyCitedBy:
			m = m.navigateTo(viewCitedBy)
		case keyFamily:
			if m.mode == viewList && len(m.patents) > 0 {
				m.current = m.patents[m.selected]
			}
			m = m.navigateTo(viewFamily)
			m.familySelected = familyCurrentIdx(m.buildFamilyTree())
		case keyTag:
			if m.mode == viewList && len(m.patents) > 0 {
				m.current = m.patents[m.selected]
			}
			if m.current.Number != "" {
				m = m.navigateTo(viewTagSelect).reloadAvailableTags()
			}
		case keyText:
			m = m.navigateTo(viewText)
		case keyNotes: // which is "n"
			if m.isPopupSearchMode() && m.popupSearchQuery != "" {
				return m.popupSearchNext(), nil
			}
			if m.mode == viewList && m.listSearchQuery != "" {
				return m.listSearchNext(), nil
			}
			if m.mode == viewBulkConfirm {
				m.bulkActionIndices = nil
				return m.goBack()
			}
			if m.mode == viewConfirmDelete {
				m.setMode(viewList)
				return m, nil
			}
			if m.mode == viewPreview {
				return m.skipPendingPatent()
			}
			m = m.navigateTo(viewNotes)
		case keyRefs:
			if m.isCitationView() {
				return m.refreshSelectedCitationDetail()
			}
			if m.mode == viewFamily {
				return m.refreshSelectedFamilyMember()
			}
			m = m.navigateTo(viewRefs)
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
		case keyHelp:
			if m.mode == viewHelpPopup || m.mode == viewKeymapPopup {
				return m.goBack()
			}
			m = m.navigateTo(viewHelpPopup)
		case keyNoteEdit:
			if m.mode == viewList && len(m.patents) > 0 {
				m.current = m.patents[clamp(m.selected, 0, len(m.patents)-1)]
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
				targetNumber = m.patents[clamp(m.selected, 0, len(m.patents)-1)].Number
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
			if m.mode == viewList {
				// Column mapping for sorting
				cols := m.listColumns()
				sortCols := make([]string, len(cols))
				for i, c := range cols {
					sortCols[i] = c.id
				}
				newCol := sortCols[clamp(m.sortColumnIndex, 0, len(sortCols)-1)]
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
				m.visualMode = false
				return m.refreshList()
			}
		case keyReviewState:
			if m.isCitationView() {
				if m.visualMode {
					m.citesReviewStateFilter = nextCitesReviewStateFilter(m.citesReviewStateFilter)
					return m, nil
				}
				// Change status of selected citation
				edge, ok, err := m.selectedCitationEdge()
				if err != nil || !ok {
					return m, nil
				}
				currentStatus := edge.ReviewState
				m.reviewStateSelected = 0
				for i, s := range m.selectableReviewStates() {
					if s == currentStatus {
						m.reviewStateSelected = i
						break
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
			// Pre-select current status in the list
			currentStatus := ""
			if m.mode == viewDetail {
				currentStatus = m.current.ReviewState
				m.detailSelected = m.indexJumpLabel(keyReviewState)
			} else if m.mode == viewList && len(m.patents) > 0 {
				currentStatus = m.patents[m.selected].ReviewState
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
		case "+":
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
			if m.mode == viewPreview {
				return m.storePendingPatent()
			}
			if m.isCitationView() {
				return m.storeSelectedCitation()
			}
			if m.mode == viewReview {
				return m.storeSelectedReviewCitation()
			}
		case keyIgnore:
			if m.mode == viewPreview {
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
			if m.mode == viewPreview {
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

























