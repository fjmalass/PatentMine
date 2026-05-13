package tui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	ansi "github.com/charmbracelet/x/ansi"

	"patentmine/internal/ai"
	"patentmine/internal/config"
	"patentmine/internal/domain"
	"patentmine/internal/importer"
	"patentmine/internal/storage"
	sqliterepo "patentmine/internal/storage/sqlite"
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
	viewStatusSelect         viewMode = "status-select"
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
	inventorSelected           int
	familySelected             int
	visualMode                 bool
	selectionStart             int
	current                    domain.Patent
	pendingBundle              domain.PatentBundle
	pendingCitation            domain.CitationEdge
	reviewStatus               string
	filter                     string
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
	classFilters               []string
	classFilterOp              string
	classFilter                string // display label derived from classFilters
	statusFilter               string // domain.CitationStatusStored (default), "ignored", "under_review", statusFilterNone
	citesStatusFilter          string // "" (all), "stored", "ignored", "under_review"
	listNumWidth               int
	unpaidCounts               map[string]int
	familyTreeCache            []familyNode
	familyTreeCacheFor         string
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
	statusSelected             int
	version                    string
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
	inventorSelected           int
	familySelected             int
	visualMode                 bool
	selectionStart             int
	current                    domain.Patent
	pendingBundle              domain.PatentBundle
	pendingCitation            domain.CitationEdge
	reviewStatus               string
	filter                     string
	message                    string
	err                        string
	countBuffer                string
	ProjectID                  string
	sortColumn                 string
	sortOrder                  string
	sortColumn2                string
	sortOrder2                 string
	classFilters               []string
	classFilterOp              string
	classFilter                string
	statusFilter               string
	citesStatusFilter          string
	listNumWidth               int
	classificationQuery        string
	classificationSearchActive bool
	listSearchQuery            string
	listSearchActive           bool
	popupSearchQuery           string
	popupSearchActive          bool
	statusSelected             int
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
		StatusFilter: domain.CitationStatusStored,
		SortColumn:   EmptySortColumn,
		SortOrder:    EmptySortOrder,
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
		patents:         patents,
		projectSelected: 0,
		logger:          logger,
		activityLog:     activityLog,
		text:            EnglishText(),
		statusFilter:    domain.CitationStatusStored,
		importCfg:       cfg,
		version:         version,
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

func (m *Model) reloadProjects() *Model {
	m.projects, _ = m.repo.ListProjects(m.ctx)
	m.unpaidCounts, _ = m.repo.CountUnpaidInvoicesByProject(m.ctx)
	return m
}

// importPatent fetches a patent bundle using the configured import source.
func (m *Model) importPatent(number string) (domain.PatentBundle, error) {
	if m.importCfg.ImportSource == config.ImportSourceUSPTO {
		if m.importCfg.USPTO.APIKey == "" {
			return domain.PatentBundle{}, fmt.Errorf("USPTO import source configured but no API key set (see --uspto-api-key or config.toml)")
		}
		bundle, err := importer.ImportUSPTO(number, m.importCfg.USPTO.APIKey, m.logger)
		if err == nil {
			bundle.Patent.ImportSource = ImportSourceUSPTO
		}
		return bundle, err
	}
	rawURL, err := importer.GooglePatentsURL(number)
	if err != nil {
		return domain.PatentBundle{}, err
	}
	bundle, err := importer.ImportGooglePatents(rawURL, m.logger)
	if err == nil {
		bundle.Patent.ImportSource = ImportSourceGoogle
	}
	return bundle, err
}

// importByNumber adds or refreshes a patent identified by number using the configured source.
// Used by :add command in USPTO mode.
func (m *Model) importByNumber(number, verb string) (tea.Model, tea.Cmd) {
	if verb != importActionRefreshed {
		m.backStack = append(m.backStack, m.snapshot())
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.loading = true
	m.loadingMsg = fmt.Sprintf("%s%s %s via USPTO ODP...", strings.ToUpper(verb[:1]), verb[1:], number)
	m.cancel = cancel

	repo := m.repo
	projectID := m.ProjectID
	logger := m.logger
	apiKey := m.importCfg.USPTO.APIKey
	currentStatus := m.current.Status

	return m, tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			bundle, err := importer.ImportUSPTO(number, apiKey, logger)
			if err != nil {
				return refreshResultMsg{err: fmt.Errorf("import failed: %w", err)}
			}
			bundle.Patent.ImportSource = ImportSourceUSPTO
			if verb == importActionRefreshed {
				bundle.Patent.Status = currentStatus
			} else {
				bundle.Patent.Status = domain.CitationStatusStored
			}
			if err := repo.UpsertPatentBundle(ctx, projectID, bundle); err != nil {
				return refreshResultMsg{err: fmt.Errorf("storage failed: %w", err)}
			}
			p, err := repo.GetPatent(ctx, projectID, bundle.Patent.Number)
			if err != nil {
				return refreshResultMsg{err: err}
			}
			return refreshResultMsg{
				patent:  p,
				mode:    viewDetail,
				message: fmt.Sprintf("%s %s via USPTO ODP", verb, bundle.Patent.Number),
				action:  ActivityPatentImport,
				source:  ImportSourceUSPTO,
			}
		},
	)
}

func (m *Model) saveLastProject() {
	_ = m.repo.SetSetting(m.ctx, SettingLastProjectID, m.ProjectID)
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
	withDetails     bool
	action          string
	source          string
}

type refreshDetailsResultMsg struct {
	err     error
	message string
}

func (m *Model) enrichClassificationDescriptionsCommand(number string) tea.Cmd {
	return func() tea.Msg {
		classifications, err := m.repo.ListClassifications(m.ctx, m.ProjectID, number)
		if err != nil || len(classifications) == 0 {
			return nil
		}

		missing := false
		for _, cls := range classifications {
			if cls.Description == "" {
				missing = true
				break
			}
		}

		if !missing {
			return nil
		}

		// Scrape to get descriptions
		rawURL, err := importer.GooglePatentsURL(number)
		if err != nil {
			return nil
		}

		bundle, err := importer.ImportGooglePatents(rawURL, m.logger)
		if err != nil {
			return nil
		}

		// Update missing descriptions in DB
		for _, scraped := range bundle.Classifications {
			if scraped.Description != "" {
				_ = m.repo.UpdateClassificationDescription(m.ctx, m.ProjectID, scraped.System, scraped.Code, scraped.Description)
			}
		}

		return classificationEnrichedMsg{Number: number}
	}
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
		m.mode = msg.mode
		m.citesSelected = msg.citesSelected
		m.citedBySelected = msg.citedBySelected
		m.message = msg.message
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
			m.mode = viewSplash
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

		if m.mode == viewStatusSelect {
			switch msg.String() {
			case keyVimDown, keyArrowDown:
				m.statusSelected = clamp(m.statusSelected+1, 0, len(m.selectableStatuses())-1)
			case keyVimUp, keyArrowUp:
				m.statusSelected = clamp(m.statusSelected-1, 0, len(m.selectableStatuses())-1)
			case keyEnter:
				return m.applyStatusSelection()
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
					m.mode = viewList
					return m.refreshList()
				}
			case keyEvents:
				if len(m.projects) > 0 {
					m.ProjectID = m.projects[m.projectSelected].ID
				}
				m.projectEventsSelected = 0
				m.mode = viewProjectEvents
				return m, nil
			case keyInvoices:
				if len(m.projects) > 0 {
					m.ProjectID = m.projects[m.projectSelected].ID
				}
				m.projectInvoicesSelected = 0
				m.mode = viewProjectInvoices
				return m, nil
			case keyIDS:
				if len(m.projects) > 0 {
					m.ProjectID = m.projects[m.projectSelected].ID
				}
				m.projectIDSSelected = 0
				m.mode = viewProjectIDS
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
		if m.mode == viewNoteEdit {
			switch msg.String() {
			case "ctrl+s":
				body := strings.TrimSpace(m.noteTA.Value())
				if body != "" {
					// Automatic date stamping
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
		if m.mode == viewHelp {
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
			if msg.String() == keyEnter && m.mode == viewClassifications && m.popupSearchQuery != "" {
				m.popupSearchActive = false
				return m, nil
			}
			if m.mode == viewPreview {
				return m.storePendingPatent()
			}
			if m.mode == viewHelpPopup {
				return m.goBack()
			}
			if m.mode == viewConfirmDelete {
				return m.deleteSelectedPatent()
			}
			if m.mode == viewClassificationDetail {
				classifications, _ := m.repo.ListClassifications(m.ctx, m.ProjectID, m.current.Number)
				if len(classifications) > 0 {
					sel := clamp(m.classificationSelected, 0, len(classifications)-1)
					code := classifications[sel].Code
					m.classFilters = []string{code}
					m.classFilterOp = domain.FilterOpAnd
					m.classFilter = code
					m.mode = viewList
					return m.refreshList()
				}
				return m.goBack()
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
				m.mode = viewConfirmDelete
				return m, nil
			}
		case keyVimDown, keyArrowDown:
			count := m.consumeCount(1)
			return m.moveSelection(count), nil
		case keyVimUp, keyArrowUp:
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
				m.mode = viewList
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
				return m.pullFamilyCommand()
			}
			m = m.navigateTo(viewRefs)
		case keyRefreshAll:
			if m.isCitationView() {
				return m.refreshVisibleCitationDetails()
			}
		case keyAI:
			m = m.navigateTo(viewAI)
		case keyWeb:
			return m.openBrowser(nil)
		case keyHelp:
			if m.mode == viewHelpPopup {
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
		case keyStatus:
			if m.isCitationView() {
				m.citesStatusFilter = nextCitesStatusFilter(m.citesStatusFilter)
				return m, nil
			}
			// Pre-select current status in the list
			currentStatus := ""
			if m.mode == viewDetail {
				currentStatus = m.current.Status
			} else if m.mode == viewList && len(m.patents) > 0 {
				currentStatus = m.patents[m.selected].Status
			}
			m.statusSelected = 0
			if currentStatus != "" {
				for i, s := range m.selectableStatuses() {
					if s == currentStatus {
						m.statusSelected = i
						break
					}
				}
			}
			return m.navigateTo(viewStatusSelect), nil
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
				return m.updatePendingCitation(domain.CitationStatusIgnored, TextMessageIgnoredPatent)
			}
			if m.isCitationView() {
				return m.updateSelectedCitationStatus(domain.CitationStatusIgnored, TextMessageIgnoredPatent)
			}
			if m.mode == viewReview {
				return m.updateSelectedReviewCitationStatus(domain.CitationStatusIgnored, TextMessageIgnoredPatent)
			}
			return m.navigateTo(viewProjectInfo), nil
		case keyUnreview:
			if m.mode == viewPreview {
				return m.updatePendingCitation(domain.CitationStatusUnderReview, TextMessageUnderReviewPatent)
			}
			if m.isCitationView() {
				return m.updateSelectedCitationStatus(domain.CitationStatusUnderReview, TextMessageUnderReviewPatent)
			}
			if m.mode == viewReview {
				return m.updateSelectedReviewCitationStatus(domain.CitationStatusUnderReview, TextMessageUnderReviewPatent)
			}
		}
	}
	return m, nil
}

func (m *Model) navigateTo(mode viewMode) *Model {
	if m.mode == mode {
		return m
	}
	m.backStack = append(m.backStack, m.snapshot())
	m.mode = mode
	m.err = EmptyError
	m.message = EmptyMessage

	return m
}

func (m *Model) snapshot() navSnapshot {
	patents := m.patents
	projects := make([]domain.Project, len(m.projects))
	copy(projects, m.projects)
	return navSnapshot{
		mode:                       m.mode,
		patents:                    patents,
		projects:                   projects,
		selected:                   m.selected,
		projectSelected:            m.projectSelected,
		projectEventsSelected:      m.projectEventsSelected,
		projectInvoicesSelected:    m.projectInvoicesSelected,
		projectIDSSelected:         m.projectIDSSelected,
		detailSelected:             m.detailSelected,
		citesSelected:              m.citesSelected,
		citedBySelected:            m.citedBySelected,
		reviewSelected:             m.reviewSelected,
		classificationSelected:     m.classificationSelected,
		inventorSelected:           m.inventorSelected,
		familySelected:             m.familySelected,
		visualMode:                 m.visualMode,
		selectionStart:             m.selectionStart,
		current:                    m.current,
		pendingBundle:              m.pendingBundle,
		pendingCitation:            m.pendingCitation,
		reviewStatus:               m.reviewStatus,
		filter:                     m.filter,
		message:                    m.message,
		err:                        m.err,
		countBuffer:                m.countBuffer,
		ProjectID:                  m.ProjectID,
		sortColumn:                 m.sortColumn,
		sortOrder:                  m.sortOrder,
		sortColumn2:                m.sortColumn2,
		sortOrder2:                 m.sortOrder2,
		classFilters:               append([]string(nil), m.classFilters...),
		classFilterOp:              m.classFilterOp,
		classFilter:                m.classFilter,
		statusFilter:               m.statusFilter,
		citesStatusFilter:          m.citesStatusFilter,
		listNumWidth:               m.listNumWidth,
		classificationQuery:        m.classificationQuery,
		classificationSearchActive: m.classificationSearchActive,
		listSearchQuery:            m.listSearchQuery,
		listSearchActive:           m.listSearchActive,
		popupSearchQuery:           m.popupSearchQuery,
		popupSearchActive:          m.popupSearchActive,
		statusSelected:             m.statusSelected,
		width:                      m.effectiveWidth(),
	}
}

func (m *Model) effectiveWidth() int {
	if mustModeSpec(m.mode).isOverlay {
		return m.overlayWidth()
	}
	return m.width
}

func (m *Model) restore(snapshot navSnapshot) *Model {
	m.mode = snapshot.mode
	m.patents = snapshot.patents
	m.projects = snapshot.projects
	m.selected = snapshot.selected
	m.projectSelected = snapshot.projectSelected
	m.projectEventsSelected = snapshot.projectEventsSelected
	m.projectInvoicesSelected = snapshot.projectInvoicesSelected
	m.projectIDSSelected = snapshot.projectIDSSelected
	m.detailSelected = snapshot.detailSelected
	m.citesSelected = snapshot.citesSelected
	m.citedBySelected = snapshot.citedBySelected
	m.reviewSelected = snapshot.reviewSelected
	m.classificationSelected = snapshot.classificationSelected
	m.inventorSelected = snapshot.inventorSelected
	m.familySelected = snapshot.familySelected
	m.visualMode = snapshot.visualMode
	m.selectionStart = snapshot.selectionStart
	m.current = snapshot.current
	m.pendingBundle = snapshot.pendingBundle
	m.pendingCitation = snapshot.pendingCitation
	m.reviewStatus = snapshot.reviewStatus
	m.filter = snapshot.filter
	m.message = snapshot.message
	m.err = snapshot.err
	m.countBuffer = snapshot.countBuffer
	m.ProjectID = snapshot.ProjectID
	m.sortColumn = snapshot.sortColumn
	m.sortOrder = snapshot.sortOrder
	m.sortColumn2 = snapshot.sortColumn2
	m.sortOrder2 = snapshot.sortOrder2
	m.classFilters = snapshot.classFilters
	m.classFilterOp = snapshot.classFilterOp
	m.classFilter = snapshot.classFilter
	m.statusFilter = snapshot.statusFilter
	m.citesStatusFilter = snapshot.citesStatusFilter
	m.listNumWidth = snapshot.listNumWidth
	m.classificationQuery = snapshot.classificationQuery
	m.classificationSearchActive = snapshot.classificationSearchActive
	m.listSearchQuery = snapshot.listSearchQuery
	m.listSearchActive = snapshot.listSearchActive
	m.popupSearchQuery = snapshot.popupSearchQuery
	m.popupSearchActive = snapshot.popupSearchActive
	m.statusSelected = snapshot.statusSelected
	return m
}
func (m *Model) goBack() (tea.Model, tea.Cmd) {
	if m.mode == viewHelp && m.helpSearchActive {
		m.helpSearchActive = false
		m.helpQuery = ""
		m.helpScroll = 0
		return m, nil
	}
	if m.isPopupSearchMode() && m.popupSearchActive {
		m.popupSearchActive = false
		m.popupSearchQuery = ""
		return m, nil
	}
	if m.mode == viewList && m.listSearchActive {
		m.listSearchActive = false
		m.listSearchQuery = ""
		return m, nil
	}
	if len(m.backStack) > 0 {
		last := m.backStack[len(m.backStack)-1]
		m.backStack = m.backStack[:len(m.backStack)-1]
		m = m.restore(last)
		if m.mode == viewList {
			return m.refreshList()
		}
		return m, nil
	}
	if m.mode == viewList && m.filter != EmptyFilter {
		m.filter = EmptyFilter
		return m.refreshList()
	}
	if m.mode != viewList {
		m.mode = viewList
		return m.refreshList()
	}
	return m, nil
}

func (m *Model) runCommand(command Command) (tea.Model, tea.Cmd) {
	m.err = EmptyError
	m.message = EmptyMessage

	m.logger.Info("tui command", "name", command.Name, "args", command.Args)
	switch command.Name {
	case commandSearch:
		if m.isPopupSearchMode() {
			if len(command.Args) > 0 {
				m.popupSearchQuery = command.Args[0]
				m.popupSearchActive = true
				return m.popupSearchFirst(), nil
			}
			return m, nil
		}
		if m.mode == viewList {
			if len(command.Args) > 0 && command.Args[0] != "clear" {
				m.listSearchQuery = command.Args[0]
				m.listSearchActive = true
				return m.listSearchFirst(), nil
			}
			m.listSearchActive = false
			m.listSearchQuery = ""
			return m, nil
		}
		if len(command.Args) > 0 && command.Args[0] != "clear" {
			m.backStack = append(m.backStack, m.snapshot())
			m.filter = command.Args[0]
		} else {
			m.filter = EmptyFilter
		}
		m.mode = viewList
		return m.refreshList()
	case commandOpen:
		if len(command.Args) != 1 {
			m.err = "usage: :open US11611785B2"
			return m, nil
		}
		return m.openPatent(command.Args[0])
	case commandAdd:
		if len(command.Args) != 1 {
			m.err = "usage: :add US11611785B2"
			return m, nil
		}
		if m.importCfg.ImportSource == config.ImportSourceUSPTO {
			if m.importCfg.USPTO.APIKey == "" {
				m.err = "USPTO ODP source configured but no API key set (use --uspto-api-key or config.toml)"
				return m, nil
			}
			return m.importByNumber(command.Args[0], importActionAdded)
		}
		rawURL, err := importer.GooglePatentsURL(command.Args[0])
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		return m.importGooglePatent(rawURL, importActionAdded)
	case commandImport:
		if len(command.Args) != 1 {
			m.err = "usage: :import https://patents.google.com/patent/US11611785B2/en?oq=US11611785B2+"
			return m, nil
		}
		return m.importGooglePatent(command.Args[0], importActionImported)
	case commandRefresh:
		return m.refreshCommand(command.Args)
	case commandRefreshRefsDetails:
		return m.refreshVisibleCitationDetails()
	case commandFilter:
		return m.filterCommand(command.Args)
	case commandFamily:
		return m.familyCommand(command.Args)
	case commandSort:
		return m.sortCommand(command.Args)
	case domain.RelationCites:
		m = m.navigateTo(viewCites)
	case commandCitedBy:
		m = m.navigateTo(viewCitedBy)
	case commandClassification:
		m = m.navigateTo(viewClassifications)
	case commandText:
		m = m.navigateTo(viewText)
	case commandRefs:
		m = m.navigateTo(viewRefs)
	case commandNotes:
		m = m.navigateTo(viewNotes)
	case commandDate:
		return m.patentDateCommand(command.Args)
	case commandNum:
		return m.patentNumberCommand(command.Args)
	case commandSummarize:
		return m.summarize()
	case commandCompare:
		if len(command.Args) != 1 {
			m.err = "usage: :compare US11611785B2"
			return m, nil
		}
		return m.compare(command.Args[0])
	case commandRef:
		return m.refCommand(command.Args)
	case commandIgnored:
		return m.openReviewQueue(domain.CitationStatusIgnored)
	case commandUnderReview:
		return m.openReviewQueue(domain.CitationStatusUnderReview)
	case commandReview:
		return m.reviewCommand(command.Args)
	case commandBrowser, commandWeb:
		return m.openBrowser(command.Args)
	case commandHelp, commandHelpShort, keyHelp:
		m = m.navigateTo(viewHelp)
	case commandVersion:
		m.message = "PatentMine " + m.displayVersion()
	case commandProject:
		return m.projectCommand(command.Args)
	case commandPurge:
		return m.purgeCommand(command.Args)
	case commandCompact:
		if err := m.repo.Compact(m.ctx); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "database compacted"
	case commandNote:
		if len(command.Args) == 0 {
			m.err = fmt.Sprintf("usage: :%s <text>", commandNote)
			return m, nil
		}
		text := strings.Join(command.Args, " ")
		m.logActivity(activityNoteAdd, m.current.Number, text)
		m.message = "noted"
	case commandIDS:
		return m.idsEditCommand(command.Args)
	case commandExit:
		return m, tea.Quit
	default:
		m.err = "unknown command: " + command.Name
	}
	return m, nil
}

func (m *Model) importGooglePatent(rawURL, verb string) (tea.Model, tea.Cmd) {
	if verb != importActionRefreshed {
		m.backStack = append(m.backStack, m.snapshot())
	}

	ctx, cancel := context.WithCancel(m.ctx)
	m.loading = true
	m.loadingMsg = fmt.Sprintf("%s%s from %s...", strings.ToUpper(verb[:1]), verb[1:], rawURL)
	m.cancel = cancel

	repo := m.repo
	projectID := m.ProjectID
	logger := m.logger
	currentStatus := m.current.Status

	return m, tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			bundle, err := importer.ImportGooglePatents(rawURL, logger)
			if err != nil {
				return refreshResultMsg{err: fmt.Errorf("import failed: %w", err)}
			}
			bundle.Patent.ImportSource = ImportSourceGoogle
			if verb == importActionRefreshed {
				bundle.Patent.Status = currentStatus
			} else {
				bundle.Patent.Status = domain.CitationStatusStored
			}
			if err := repo.UpsertPatentBundle(ctx, projectID, bundle); err != nil {
				return refreshResultMsg{err: fmt.Errorf("storage failed: %w", err)}
			}
			p, err := repo.GetPatent(ctx, projectID, bundle.Patent.Number)
			if err != nil {
				return refreshResultMsg{err: err}
			}

			msg := fmt.Sprintf("%s %s from %s", verb, bundle.Patent.Number, rawURL)
			if n := len(bundle.FamilyEdges); n > 0 {
				msg += fmt.Sprintf(" · %d family edge(s) — :refresh family to import", n)
			}
			logger.Info("google patent imported", "url", rawURL, "patent", bundle.Patent.Number)
			return refreshResultMsg{
				patent:  p,
				mode:    viewDetail,
				message: msg,
				action:  activityPatentImport,
				source:  ImportSourceGoogle,
			}
		},
	)
}

func (m *Model) refreshCommand(args []string) (tea.Model, tea.Cmd) {
	target := refreshTargetAll
	withDetails := false

	refreshUsage := fmt.Sprintf("usage: :%s [%s|%s] [%s]", commandRefresh, refreshTargetCitations, refreshTargetCitedBy, refreshArgDetails)
	switch len(args) {
	case 0:
		// :refresh — all
	case 1:
		target = strings.ToLower(args[0])
	case 2:
		target = strings.ToLower(args[0])
		if strings.ToLower(args[1]) != refreshArgDetails {
			m.err = refreshUsage
			return m, nil
		}
		withDetails = true
	default:
		m.err = refreshUsage
		return m, nil
	}

	if target == "family" || target == commandFamily {
		return m.pullFamilyCommand()
	}
	if target != refreshTargetAll && target != refreshTargetCitations && target != domain.RelationCites && target != refreshTargetCitedBy && target != domain.RelationCitedBy {
		m.err = refreshUsage
		return m, nil
	}
	if m.current.Number == EmptyFilter {
		m.err = "open a patent before refreshing citations"
		return m, nil
	}

	importSource := m.importCfg.ImportSource
	apiKey := m.importCfg.USPTO.APIKey

	if importSource == config.ImportSourceUSPTO && apiKey == "" {
		m.err = "USPTO ODP source configured but no API key set (use --uspto-api-key or config.toml)"
		return m, nil
	}

	var rawURL string
	if importSource != config.ImportSourceUSPTO {
		rawURL = m.current.SourceGoogleURL
		if rawURL == "" {
			var err error
			rawURL, err = importer.GooglePatentsURL(m.current.Number)
			if err != nil {
				m.err = err.Error()
				return m, nil
			}
		}
	}

	sourceLabel := string(importSource)
	ctx, cancel := context.WithCancel(m.ctx)
	m.loading = true
	m.loadingMsg = fmt.Sprintf("Refreshing %s...", m.current.Number)
	m.cancel = cancel

	// Capture state for the command
	repo := m.repo
	projectID := m.ProjectID
	currentNumber := m.current.Number
	text := m.text
	currentMode := m.mode
	citedBySelected := m.citedBySelected
	citesSelected := m.citesSelected
	logger := m.logger
	currentStatus := m.current.Status

	return m, tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			beforeCites, _ := repo.ListCitations(ctx, projectID, currentNumber, domain.RelationCites, storage.ListCitationsOptions{})
			beforeCitedBy, _ := repo.ListCitations(ctx, projectID, currentNumber, domain.RelationCitedBy, storage.ListCitationsOptions{})

			var bundle domain.PatentBundle
			var err error
			if importSource == config.ImportSourceUSPTO && apiKey != "" {
				bundle, err = importer.ImportUSPTO(currentNumber, apiKey, logger)
			} else {
				bundle, err = importer.ImportGooglePatents(rawURL, logger)
			}
			if err != nil {
				return refreshResultMsg{err: fmt.Errorf("import failed: %w", err)}
			}

			bundle.Patent.ImportSource = sourceLabel
			bundle.Patent.Status = currentStatus
			if err := repo.UpsertPatentBundle(ctx, projectID, bundle); err != nil {
				return refreshResultMsg{err: fmt.Errorf("storage failed: %w", err)}
			}

			p, err := repo.GetPatent(ctx, projectID, currentNumber)
			if err != nil {
				return refreshResultMsg{err: err}
			}

			afterCites, _ := repo.ListCitations(ctx, projectID, currentNumber, domain.RelationCites, storage.ListCitationsOptions{})
			afterCitedBy, _ := repo.ListCitations(ctx, projectID, currentNumber, domain.RelationCitedBy, storage.ListCitationsOptions{})

			familySuffix := ""
			if n := len(bundle.FamilyEdges); n > 0 {
				familySuffix = fmt.Sprintf(" · %d family edge(s) — :refresh family to import", n)
			}

			msg := refreshResultMsg{
				patent:      p,
				mode:        currentMode,
				withDetails: withDetails,
				action:      ActivityPatentRefresh,
				source:      sourceLabel,
			}

			switch target {
			case refreshTargetCitedBy, domain.RelationCitedBy:
				msg.mode = viewCitedBy
				msg.citedBySelected = 0
				msg.message = fmt.Sprintf(text.T(TextMessageRefreshCitedBy), len(afterCitedBy), len(beforeCitedBy)) + familySuffix
			case refreshTargetCitations, domain.RelationCites:
				msg.mode = viewCites
				msg.citesSelected = 0
				msg.message = fmt.Sprintf(text.T(TextMessageRefreshCitations), len(afterCites), len(beforeCites)) + familySuffix
			default:
				msg.citedBySelected = clamp(citedBySelected, 0, max(0, len(afterCitedBy)-1))
				msg.citesSelected = clamp(citesSelected, 0, max(0, len(afterCites)-1))
				msg.message = fmt.Sprintf(text.T(TextMessageRefreshAll), len(afterCites), len(beforeCites), len(afterCitedBy), len(beforeCitedBy)) + familySuffix
			}

			logger.Info("citations refreshed", "url", rawURL, "patent", currentNumber, domain.RelationCites, len(afterCites), domain.RelationCitedBy, len(afterCitedBy))
			return msg
		},
	)
}

func (m *Model) refreshVisibleCitationDetails() (tea.Model, tea.Cmd) {
	edges, err := m.visibleCitationEdges()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if len(edges) == 0 {
		m.err = "open citations, cited-by, or a review queue before refreshing details"
		return m, nil
	}

	ctx, cancel := context.WithCancel(m.ctx)
	m.loading = true
	m.loadingMsg = fmt.Sprintf("Refreshing details for %d citations...", len(edges))
	m.cancel = cancel

	repo := m.repo
	projectID := m.ProjectID
	logger := m.logger
	importSource := m.importCfg.ImportSource
	apiKey := m.importCfg.USPTO.APIKey

	if importSource == config.ImportSourceUSPTO && apiKey == "" {
		m.err = "USPTO ODP source configured but no API key set (use --uspto-api-key or config.toml)"
		return m, nil
	}

	return m, tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			updatedCount := 0
			skippedCount := 0
			for _, edge := range edges {
				select {
				case <-ctx.Done():
					return refreshDetailsResultMsg{err: ctx.Err()}
				default:
				}

				if edge.TargetTitle != "" && len(edge.TargetInventors) > 0 && edge.TargetExpirationDate != "" {
					skippedCount++
					continue
				}

				// Check if patent already exists
				_, err := repo.GetPatent(ctx, projectID, edge.TargetPatent)
				exists := err == nil

				var bundle domain.PatentBundle
				if importSource == config.ImportSourceUSPTO {
					bundle, err = importer.ImportUSPTO(edge.TargetPatent, apiKey, logger)
				} else {
					var rawURL string
					rawURL, err = importer.GooglePatentsURL(edge.TargetPatent)
					if err == nil {
						bundle, err = importer.ImportGooglePatents(rawURL, logger)
					}
				}
				if err != nil {
					logger.Error("citation details import failed", "patent", edge.TargetPatent, "error", err)
					skippedCount++
					continue
				}

				if importSource == config.ImportSourceUSPTO && apiKey != "" {
					bundle.Patent.ImportSource = ImportSourceUSPTO
				} else {
					bundle.Patent.ImportSource = ImportSourceGoogle
				}
				bundle.Patent.Status = domain.CitationStatusCached
				if err := repo.UpsertPatentBundle(ctx, projectID, bundle); err != nil {
					logger.Error("citation details storage failed", "patent", edge.TargetPatent, "error", err)
					return refreshDetailsResultMsg{err: err}
				}

				status := nextCitationDetailStatus(edge.Status, exists)
				if err := repo.UpdateCitationStatus(ctx, projectID, edge, status); err != nil {
					return refreshDetailsResultMsg{err: err}
				}
				updatedCount++
			}
			return refreshDetailsResultMsg{
				message: fmt.Sprintf("citation details refreshed: %d updated, %d skipped", updatedCount, skippedCount),
			}
		},
	)
}

// logActivity writes one structured line to the monthly activity log.
// action uses dot notation: "patent.add", "citation.store", "note", etc.
// subject is the primary identifier (patent number, invoice id, etc.) or "".
// note is optional free-text annotation.
func (m *Model) logActivity(action, subject, note string) {
	if m.activityLog == nil {
		return
	}
	args := []any{"project", m.ProjectID, "action", action}
	if subject != "" {
		args = append(args, "subject", subject)
	}
	if note != "" {
		args = append(args, "note", note)
	}
	m.activityLog.Info("activity", args...)
}

func (m *Model) idsEntryForPatent(number string) *domain.IDSEntry {
	// Try cache first
	if m.detailCache.Number != "" {
		for i, e := range m.detailCache.IDSEntries {
			if e.PatentNumber == number {
				return &m.detailCache.IDSEntries[i]
			}
		}
	}

	// Fallback for safety (though detailFields should have populated cache)
	entries, err := m.repo.ListIDSEntries(m.ctx, m.ProjectID)
	if err != nil {
		return nil
	}
	for i, e := range entries {
		if e.PatentNumber == number {
			return &entries[i]
		}
	}
	return nil
}

// markdownHeadingSummary extracts # and ## heading lines from body as a compact
// summary. Falls back to the first non-empty line when no headings are present.
func markdownHeadingSummary(body string) string {
	var headings []string
	for _, line := range strings.SplitAfter(body, "\n") {
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "## ") {
			headings = append(headings, strings.TrimPrefix(line, "## "))
		} else if strings.HasPrefix(line, "# ") {
			headings = append(headings, strings.TrimPrefix(line, "# "))
		}
	}
	if len(headings) > 0 {
		return strings.Join(headings, " · ")
	}
	for _, line := range strings.SplitAfter(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func (m *Model) recentNotesSnippet(number string) string {
	notes, err := m.repo.ListNotes(m.ctx, m.ProjectID, number)
	if err != nil || len(notes) == 0 {
		return ""
	}
	subtleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle))
	var b strings.Builder
	limit := noteDetailSnippetCount
	if len(notes) < limit {
		limit = len(notes)
	}
	for i := 0; i < limit; i++ {
		note := notes[i]
		b.WriteString(subtleStyle.Render(note.CreatedAt.Format("2006-01-02 15:04")))
		b.WriteString("  ")
		body := note.Body
		if idx := strings.Index(body, "\n"); idx >= 0 {
			body = body[:idx]
		}
		b.WriteString(body)
		b.WriteString("\n")
	}
	return b.String()
}

func nextIDSStatus(current domain.IDSStatus) domain.IDSStatus {
	switch current {
	case "":
		return domain.IDSStatusPending
	case domain.IDSStatusPending:
		return domain.IDSStatusSubmitted
	case domain.IDSStatusSubmitted:
		return domain.IDSStatusAccepted
	case domain.IDSStatusAccepted:
		return ""
	default:
		return domain.IDSStatusPending
	}
}

func idsStatusColor(status domain.IDSStatus) string {
	switch status {
	case domain.IDSStatusSubmitted:
		return ColorTheme
	case domain.IDSStatusAccepted:
		return ColorSuccess
	default:
		return ColorSubtle
	}
}

func (m *Model) cycleCurrentPatentIDSStatus() (tea.Model, tea.Cmd) {
	if m.current.Number == "" {
		return m, nil
	}

	entry := m.idsEntryForPatent(m.current.Number)
	if entry == nil {
		created, err := m.repo.AddIDSEntry(m.ctx, domain.IDSEntry{
			ProjectID:    m.ProjectID,
			PatentNumber: m.current.Number,
			Status:       domain.IDSStatusPending,
		})
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.populateDetailCache()
		m.logActivity(ActivityIDSAdd, m.current.Number, string(created.Status))
		m.message = "IDS status: " + string(created.Status)
		return m, nil
	}

	next := nextIDSStatus(entry.Status)
	if next == "" {
		if err := m.repo.DeleteIDSEntry(m.ctx, entry.ID); err != nil {
			m.err = err.Error()
			return m, nil
		}
		filtered := m.detailCache.IDSEntries[:0]
		for _, idsEntry := range m.detailCache.IDSEntries {
			if idsEntry.ID != entry.ID {
				filtered = append(filtered, idsEntry)
			}
		}
		m.detailCache.IDSEntries = filtered
		m.logActivity(ActivityIDSRemove, m.current.Number, "")
		m.message = "IDS entry removed"
		return m, nil
	}
	if err := m.repo.UpdateIDSEntryStatus(m.ctx, entry.ID, next); err != nil {
		m.err = err.Error()
		return m, nil
	}
	for i := range m.detailCache.IDSEntries {
		if m.detailCache.IDSEntries[i].ID == entry.ID {
			m.detailCache.IDSEntries[i].Status = next
			break
		}
	}
	m.logActivity(ActivityIDSStatus, m.current.Number, string(next))
	m.message = "IDS status: " + string(next)
	return m, nil
}

func (m *Model) idsEditCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) < 1 {
		m.err = "usage: :ids <note|kind|country|passages> <value>"
		return m, nil
	}
	entry := m.idsEntryForPatent(m.current.Number)
	if entry == nil {
		m.err = "no IDS entry for this patent; add one first"
		return m, nil
	}
	value := strings.Join(args[1:], " ")
	switch args[0] {
	case idsEditSubNote:
		entry.Notes = value
	case idsEditSubKind:
		entry.KindCode = value
	case idsEditSubCountry:
		entry.CountryCode = value
	case idsEditSubPassages:
		if err := domain.ValidateIDSPassages(value); err != nil {
			m.err = err.Error()
			return m, nil
		}
		entry.RelevantPassages = value
		entry.InFull = false
	case idsEditSubFull:
		entry.InFull = !entry.InFull
	default:
		m.err = "unknown ids field: " + args[0] + ". Use: note, kind, country, passages, full"
		return m, nil
	}
	if err := m.repo.UpdateIDSEntry(m.ctx, *entry); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.populateDetailCache()
	m.message = "IDS " + args[0] + " updated"
	return m, nil
}

// refreshSelectedCitationDetail re-fetches Google Patents for the single
// selected citation row and updates its title, inventors, and expiration.
func (m *Model) refreshSelectedCitationDetail() (tea.Model, tea.Cmd) {
	edges, err := m.currentCitationEdges()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if len(edges) == 0 {
		m.err = "no citations visible"
		return m, nil
	}
	sel := clamp(m.citationSelection(), 0, len(edges)-1)
	edge := edges[sel]

	ctx, cancel := context.WithCancel(m.ctx)
	m.loading = true
	m.loadingMsg = fmt.Sprintf("Refreshing %s...", edge.TargetPatent)
	m.cancel = cancel

	repo := m.repo
	projectID := m.ProjectID
	logger := m.logger
	importSource := m.importCfg.ImportSource
	apiKey := m.importCfg.USPTO.APIKey
	targetPatent := edge.TargetPatent

	if importSource == config.ImportSourceUSPTO && apiKey == "" {
		m.err = "USPTO ODP source configured but no API key set (use --uspto-api-key or config.toml)"
		return m, nil
	}

	return m, tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			var bundle domain.PatentBundle
			var err error
			if importSource == config.ImportSourceUSPTO {
				bundle, err = importer.ImportUSPTO(targetPatent, apiKey, logger)
			} else {
				var rawURL string
				rawURL, err = importer.GooglePatentsURL(targetPatent)
				if err == nil {
					bundle, err = importer.ImportGooglePatents(rawURL, logger)
				}
			}
			if err != nil {
				return refreshDetailsResultMsg{err: err}
			}
			bundle.Patent.ImportSource = string(importSource)
			bundle.Patent.Status = domain.CitationStatusCached
			_, existsErr := repo.GetPatent(ctx, projectID, edge.TargetPatent)
			exists := existsErr == nil
			if err := repo.UpsertPatentBundle(ctx, projectID, bundle); err != nil {
				logger.Error("citation refresh failed", "patent", edge.TargetPatent, "error", err)
				return refreshDetailsResultMsg{err: err}
			}
			status := nextCitationDetailStatus(edge.Status, exists)
			if err := repo.UpdateCitationStatus(ctx, projectID, edge, status); err != nil {
				return refreshDetailsResultMsg{err: err}
			}
			return refreshDetailsResultMsg{message: fmt.Sprintf("refreshed %s", edge.TargetPatent)}
		},
	)
}

func (m *Model) importCitationDetailsCommand(edge domain.CitationEdge) tea.Cmd {
	repo := m.repo
	projectID := m.ProjectID
	target := edge.TargetPatent
	importSource := m.importCfg.ImportSource

	return func() tea.Msg {
		bundle, err := m.importPatent(target)
		if err != nil {
			return refreshDetailsResultMsg{err: fmt.Errorf("bulk import failed for %s: %w", target, err)}
		}

		bundle.Patent.Status = domain.CitationStatusStored
		bundle.Patent.ImportSource = string(importSource)
		if err := repo.UpsertPatentBundle(context.Background(), projectID, bundle); err != nil {
			return refreshDetailsResultMsg{err: fmt.Errorf("bulk save failed for %s: %w", target, err)}
		}

		if err := repo.UpdateCitationStatus(context.Background(), projectID, edge, domain.CitationStatusStored); err != nil {
			return refreshDetailsResultMsg{err: fmt.Errorf("bulk status update failed for %s: %w", target, err)}
		}

		return refreshDetailsResultMsg{message: fmt.Sprintf("imported %s", target)}
	}
}

func nextCitationDetailStatus(current string, exists bool) string {
	if exists {
		return current
	}
	return domain.CitationStatusCached
}

func (m *Model) visibleCitationEdges() ([]domain.CitationEdge, error) {
	var edges []domain.CitationEdge
	var selected int
	var err error
	switch {
	case m.isCitationView():
		edges, err = m.currentCitationEdges()
		selected = m.citationSelection()
	case m.mode == viewReview:
		edges, err = m.currentReviewCitationEdges()
		selected = m.reviewSelected
	case m.mode == viewDetail || m.mode == viewList:
		if m.current.Number == "" {
			return nil, nil
		}
		cites, e1 := m.repo.ListCitations(m.ctx, m.ProjectID, m.current.Number, domain.RelationCites, storage.ListCitationsOptions{})
		citedBy, e2 := m.repo.ListCitations(m.ctx, m.ProjectID, m.current.Number, domain.RelationCitedBy, storage.ListCitationsOptions{})
		if e1 != nil {
			return nil, e1
		}
		if e2 != nil {
			return nil, e2
		}
		edges = append(cites, citedBy...)
		if len(edges) == 0 {
			return nil, nil
		}
		return edges, nil
	default:
		return nil, nil
	}
	if err != nil || len(edges) == 0 {
		return nil, err
	}
	selected = clamp(selected, 0, len(edges)-1)
	window := pageWindow(selected, len(edges), m.pageSize())
	out := make([]domain.CitationEdge, window.End-window.Start)
	copy(out, edges[window.Start:window.End])
	return out, nil
}

func (m *Model) refreshList() (tea.Model, tea.Cmd) {
	if m.repo == nil {
		return m, nil
	}
	opts := storage.ListPatentsOptions{
		Filter:        m.filter,
		StatusFilter:  m.statusFilter,
		ClassFilters:  m.classFilters,
		ClassFilterOp: m.classFilterOp,
		SortColumn:    m.sortColumn,
		SortOrder:     m.sortOrder,
		SortColumn2:   m.sortColumn2,
		SortOrder2:    m.sortOrder2,
	}
	patents, err := m.repo.ListPatents(m.ctx, m.ProjectID, opts)
	if err != nil {
		m.err = err.Error()
		if m.logger != nil {
			m.logger.Error("list patents failed", "filter", m.filter, "error", err)
		}
		return m, nil
	}
	m.patents = patents
	// Pre-calculate number width for list view
	m.listNumWidth = 6
	for _, p := range m.patents {
		w := lipgloss.Width(p.Number)
		if w > m.listNumWidth {
			m.listNumWidth = w
		}
	}
	if m.selected >= len(m.patents) {
		m.selected = max(0, len(m.patents)-1)
	}
	if len(patents) > 0 && m.current.Number == "" {
		m.current = patents[0]
	}
	return m, nil
}

func (m *Model) reviewCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) != 1 {
		m.err = m.text.T(TextMessageReviewUsage)
		return m, nil
	}
	switch strings.ToLower(args[0]) {
	case domain.CitationStatusIgnored:
		return m.openReviewQueue(domain.CitationStatusIgnored)
	case domain.CitationStatusUnderReview:
		return m.openReviewQueue(domain.CitationStatusUnderReview)
	default:
		m.err = m.text.T(TextMessageReviewUsage)
		return m, nil
	}
}

func (m *Model) openReviewQueue(status string) (tea.Model, tea.Cmd) {
	m.backStack = append(m.backStack, m.snapshot())
	m.mode = viewReview
	m.reviewStatus = status
	m.reviewSelected = 0
	m.err = EmptyError
	m.message = EmptyMessage

	return m, nil
}

func (m *Model) openBrowser(args []string) (tea.Model, tea.Cmd) {
	rawURL, err := m.browserURL(args)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.message = EmptyMessage
	m.err = EmptyError

	m.logger.Info("open browser", "url", rawURL)
	return m, openBrowserCommand(rawURL)
}

func (m *Model) browserURL(args []string) (string, error) {
	if len(args) > 1 {
		return "", errors.New(m.text.T(TextMessageBrowserUsage))
	}
	if len(args) == 1 {
		return m.patentBrowserURL(args[0])
	}
	switch {
	case m.isCitationView():
		edge, ok, err := m.selectedCitationEdge()
		if err != nil {
			return "", err
		}
		if !ok {
			return "", errors.New(m.text.T(TextMessageBrowserNoPatent))
		}
		return m.patentBrowserURL(edge.TargetPatent)
	case m.mode == viewReview:
		edge, ok, err := m.selectedReviewCitationEdge()
		if err != nil {
			return "", err
		}
		if !ok {
			return "", errors.New(m.text.T(TextMessageBrowserNoPatent))
		}
		return m.patentBrowserURL(edge.TargetPatent)
	case m.mode == viewPreview:
		return m.patentURL(m.pendingBundle.Patent)
	case m.mode == viewList && len(m.patents) > 0:
		return m.patentURL(m.patents[clamp(m.selected, 0, len(m.patents)-1)])
	default:
		return m.patentURL(m.current)
	}
}

func (m *Model) patentURL(p domain.Patent) (string, error) {
	if strings.TrimSpace(p.SourceGoogleURL) != "" {
		return p.SourceGoogleURL, nil
	}
	return m.patentBrowserURL(p.Number)
}

func (m *Model) patentBrowserURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New(m.text.T(TextMessageBrowserEmpty))
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value, nil
	}
	return importer.GooglePatentsURL(value)
}

func (m *Model) openPatent(number string) (tea.Model, tea.Cmd) {
	m.backStack = append(m.backStack, m.snapshot())
	p, err := m.repo.GetPatent(m.ctx, m.ProjectID, number)
	if err != nil {
		m.backStack = m.backStack[:len(m.backStack)-1]
		m.err = err.Error()
		m.logger.Error("open patent failed", "patent", number, "error", err)
		return m, nil
	}
	m.current = p
	m.populateDetailCache()
	m.mode = viewDetail
	m.message = "opened " + p.Number
	return m, m.enrichClassificationDescriptionsCommand(number)
}

func (m *Model) openSelectedCitation() (tea.Model, tea.Cmd) {
	edge, ok, err := m.selectedCitationEdge()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if !ok {
		return m, nil
	}
	target := edge.TargetPatent
	var bundle domain.PatentBundle
	if p, err := m.repo.GetPatent(m.ctx, m.ProjectID, target); err == nil {
		bundle.Patent = p
	} else {
		bundle, err = m.importPatent(target)
		if err != nil {
			m.err = fmt.Sprintf("%s: %v", m.text.T(TextCitationsOpenFailed), err)
			return m, nil
		}
	}
	m.backStack = append(m.backStack, m.snapshot())
	m.pendingBundle = bundle
	m.pendingCitation = edge
	m.mode = viewPreview
	m.message = fmt.Sprintf(m.text.T(TextMessagePreviewLoaded), bundle.Patent.Number)
	return m, nil
}

func (m *Model) selectedCitationEdge() (domain.CitationEdge, bool, error) {
	edges, err := m.currentCitationEdges()
	if err != nil {
		return domain.CitationEdge{}, false, err
	}
	if len(edges) == 0 {
		return domain.CitationEdge{}, false, nil
	}
	selected := clamp(m.citationSelection(), 0, len(edges)-1)
	return edges[selected], true, nil
}

func (m *Model) storeSelectedCitation() (tea.Model, tea.Cmd) {
	indices := m.selectedIndices()
	edges, err := m.currentCitationEdges()
	if err != nil || len(edges) == 0 {
		return m, nil
	}

	if len(indices) > 1 {
		m.bulkAction = bulkActionStore
		m.bulkActionIndices = indices
		return m.navigateTo(viewBulkConfirm), nil
	}

	idx := indices[0]
	edge := edges[idx]
	if _, err := m.repo.GetPatent(m.ctx, m.ProjectID, edge.TargetPatent); err != nil {
		return m.openSelectedCitation()
	}
	if err := m.repo.UpdateCitationStatus(m.ctx, m.ProjectID, edge, domain.CitationStatusStored); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.logActivity(ActivityCitationStore, edge.TargetPatent, "")
	m.message = fmt.Sprintf(m.text.T(TextMessageStoredPatent), edge.TargetPatent)
	m.visualMode = false
	return m, nil
}

func (m *Model) storePendingPatent() (tea.Model, tea.Cmd) {
	if m.pendingBundle.Patent.Number == "" {
		return m, nil
	}
	m.pendingBundle.Patent.Status = domain.CitationStatusStored
	if err := m.repo.UpsertPatentBundle(m.ctx, m.ProjectID, m.pendingBundle); err != nil {
		m.err = err.Error()
		return m, nil
	}
	number := m.pendingBundle.Patent.Number
	m.logActivity(ActivityPatentImport, number, string(m.importCfg.ImportSource))
	if m.pendingCitation.TargetPatent != "" {
		if err := m.repo.UpdateCitationStatus(m.ctx, m.ProjectID, m.pendingCitation, domain.CitationStatusStored); err != nil {
			m.err = err.Error()
			return m, nil
		}
	}
	m.pendingBundle = domain.PatentBundle{}
	m.pendingCitation = domain.CitationEdge{}
	p, err := m.repo.GetPatent(m.ctx, m.ProjectID, number)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.current = p
	m.populateDetailCache()
	m.mode = viewDetail
	m.message = fmt.Sprintf(m.text.T(TextMessageStoredPatent), number)
	return m.refreshList()
}

func (m *Model) skipPendingPatent() (tea.Model, tea.Cmd) {
	number := m.pendingBundle.Patent.Number
	m.pendingBundle = domain.PatentBundle{}
	m.pendingCitation = domain.CitationEdge{}
	model, cmd := m.goBack()
	updated := model.(*Model)
	if number != "" {
		updated.message = fmt.Sprintf(updated.text.T(TextMessageSkippedPatent), number)
	}
	return updated, cmd
}

func (m *Model) updatePendingCitation(status string, messageKey TextKey) (tea.Model, tea.Cmd) {
	number := m.pendingBundle.Patent.Number
	if m.pendingCitation.TargetPatent != "" {
		if err := m.repo.UpdateCitationStatus(m.ctx, m.ProjectID, m.pendingCitation, status); err != nil {
			m.err = err.Error()
			return m, nil
		}
	}
	m.pendingBundle = domain.PatentBundle{}
	m.pendingCitation = domain.CitationEdge{}
	model, cmd := m.goBack()
	updated := model.(*Model)
	if number != "" {
		updated.message = fmt.Sprintf(updated.text.T(messageKey), number)
	}
	return updated, cmd
}

func (m *Model) updateSelectedCitationStatus(status string, messageKey TextKey) (tea.Model, tea.Cmd) {
	indices := m.selectedIndices()
	edges, err := m.currentCitationEdges()
	if err != nil || len(edges) == 0 {
		return m, nil
	}

	if len(indices) > 1 {
		m.bulkActionIndices = indices
		if status == domain.CitationStatusIgnored {
			m.bulkAction = bulkActionIgnore
		} else {
			m.bulkAction = bulkActionUnderReview
		}
		return m.navigateTo(viewBulkConfirm), nil
	}

	edge := edges[indices[0]]
	if err := m.repo.UpdateCitationStatus(m.ctx, m.ProjectID, edge, status); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.logActivity(ActivityCitationStatus, edge.TargetPatent, status)
	m.message = fmt.Sprintf(m.text.T(messageKey), edge.TargetPatent)

	m.visualMode = false
	return m, nil
}

func (m *Model) currentReviewCitationEdges() ([]domain.CitationEdge, error) {
	if strings.TrimSpace(m.reviewStatus) == "" {
		return nil, nil
	}
	opts := storage.ListCitationsOptions{
		SortColumn: m.sortColumn,
		SortOrder:  m.sortOrder,
	}
	return m.repo.ListCitationsByStatus(m.ctx, m.ProjectID, m.reviewStatus, opts)
}

func (m *Model) selectedReviewCitationEdge() (domain.CitationEdge, bool, error) {
	edges, err := m.currentReviewCitationEdges()
	if err != nil {
		return domain.CitationEdge{}, false, err
	}
	if len(edges) == 0 {
		return domain.CitationEdge{}, false, nil
	}
	selected := clamp(m.reviewSelected, 0, len(edges)-1)
	return edges[selected], true, nil
}

func (m *Model) openSelectedReviewCitation() (tea.Model, tea.Cmd) {
	edge, ok, err := m.selectedReviewCitationEdge()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if !ok {
		return m, nil
	}
	target := edge.TargetPatent
	var bundle domain.PatentBundle
	if p, err := m.repo.GetPatent(m.ctx, m.ProjectID, target); err == nil {
		bundle.Patent = p
	} else {
		bundle, err = m.importPatent(target)
		if err != nil {
			m.err = fmt.Sprintf("%s: %v", m.text.T(TextCitationsOpenFailed), err)
			return m, nil
		}
	}
	m.backStack = append(m.backStack, m.snapshot())
	m.pendingBundle = bundle
	m.pendingCitation = edge
	m.mode = viewPreview
	m.message = fmt.Sprintf(m.text.T(TextMessagePreviewLoaded), bundle.Patent.Number)
	return m, nil
}

func (m *Model) storeSelectedReviewCitation() (tea.Model, tea.Cmd) {
	indices := m.selectedIndices()
	edges, err := m.currentReviewCitationEdges()
	if err != nil || len(edges) == 0 {
		return m, nil
	}

	if len(indices) > 1 {
		m.bulkAction = bulkActionStore
		m.bulkActionIndices = indices
		return m.navigateTo(viewBulkConfirm), nil
	}

	idx := indices[0]
	edge := edges[idx]
	if _, err := m.repo.GetPatent(m.ctx, m.ProjectID, edge.TargetPatent); err != nil {
		return m.openSelectedReviewCitation()
	}
	if err := m.repo.UpdateCitationStatus(m.ctx, m.ProjectID, edge, domain.CitationStatusStored); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.message = fmt.Sprintf(m.text.T(TextMessageStoredPatent), edge.TargetPatent)
	return m, nil
}

func (m *Model) updateSelectedReviewCitationStatus(status string, messageKey TextKey) (tea.Model, tea.Cmd) {
	indices := m.selectedIndices()
	edges, err := m.currentReviewCitationEdges()
	if err != nil || len(edges) == 0 {
		return m, nil
	}

	if len(indices) > 1 {
		m.bulkActionIndices = indices
		if status == domain.CitationStatusIgnored {
			m.bulkAction = bulkActionIgnore
		} else {
			m.bulkAction = bulkActionUnderReview
		}
		return m.navigateTo(viewBulkConfirm), nil
	}

	edge := edges[indices[0]]
	if err := m.repo.UpdateCitationStatus(m.ctx, m.ProjectID, edge, status); err != nil {
		m.err = err.Error()
		return m, nil
	}
	if status != m.reviewStatus {
		edges, _ := m.currentReviewCitationEdges()
		m.reviewSelected = clamp(m.reviewSelected, 0, max(0, len(edges)-1))
	}
	m.message = fmt.Sprintf(m.text.T(messageKey), edge.TargetPatent)
	m.visualMode = false
	return m, nil
}

func (m *Model) executeBulkAction() (tea.Model, tea.Cmd) {
	indices := m.bulkActionIndices
	if len(indices) == 0 {
		return m.goBack()
	}

	var edges []domain.CitationEdge
	var err error
	if m.mode == viewReview {
		edges, err = m.currentReviewCitationEdges()
	} else {
		edges, err = m.currentCitationEdges()
	}

	if err != nil || len(edges) == 0 {
		m.err = "bulk action failed: " + err.Error()
		return m.goBack()
	}

	var cmds []tea.Cmd
	action := m.bulkAction
	status := domain.CitationStatusStored
	if action == bulkActionIgnore {
		status = domain.CitationStatusIgnored
	} else if action == bulkActionUnderReview {
		status = domain.CitationStatusUnderReview
	}

	executedCount := 0
	importCount := 0
	for _, idx := range indices {
		if idx < 0 || idx >= len(edges) {
			continue
		}
		edge := edges[idx]
		if err := m.repo.UpdateCitationStatus(m.ctx, m.ProjectID, edge, status); err == nil {
			m.logActivity(ActivityBulkPrefix+string(action), edge.TargetPatent, "")
			executedCount++

			// If storing, trigger download if not already in DB
			if action == bulkActionStore {
				if _, err := m.repo.GetPatent(m.ctx, m.ProjectID, edge.TargetPatent); err != nil {
					// Trigger background import
					cmds = append(cmds, m.importCitationDetailsCommand(edge))
					importCount++
				}
			}
		}
	}

	m.message = fmt.Sprintf("performed bulk %s on %d items", action, executedCount)
	m.visualMode = false
	m.bulkActionIndices = nil

	// pop viewBulkConfirm from backstack
	model, _ := m.goBack()
	if importCount > 0 {
		m.loading = true
		m.loadingMsg = fmt.Sprintf("downloading %d patents...", importCount)
		cmds = append(cmds, m.spinner.Tick)
	}
	return model, tea.Batch(cmds...)
}

func (m *Model) isCitationView() bool {
	return m.mode == viewCites || m.mode == viewCitedBy
}

func (m *Model) citationSelection() int {
	if m.mode == viewCitedBy {
		return m.citedBySelected
	}
	return m.citesSelected
}

func (m *Model) setCitationSelection(val int) {
	if m.mode == viewCitedBy {
		m.citedBySelected = val
	} else {
		m.citesSelected = val
	}
}

func (m *Model) currentCitationEdges() ([]domain.CitationEdge, error) {
	if m.current.Number == EmptyFilter {
		return nil, nil
	}
	relation := domain.RelationCites
	if m.mode == viewCitedBy {
		relation = domain.RelationCitedBy
	}
	opts := storage.ListCitationsOptions{
		SortColumn:   m.sortColumn,
		SortOrder:    m.sortOrder,
		StatusFilter: m.citesStatusFilter,
	}
	return m.repo.ListCitations(m.ctx, m.ProjectID, m.current.Number, relation, opts)
}

func (m *Model) projectCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.mode = viewSplash
		m = m.reloadProjects()
		// Set selection to current project
		for i, p := range m.projects {
			if p.ID == m.ProjectID {
				m.projectSelected = i
				break
			}
		}
		return m, nil
	}

	sub := args[0]
	switch sub {
	case projectSubList:
		m.mode = viewSplash
		m = m.reloadProjects()
		for i, p := range m.projects {
			if p.ID == m.ProjectID {
				m.projectSelected = i
				break
			}
		}
		return m, nil
	case projectSubCreate:
		if len(args) < 2 {
			m.err = "usage: :project create <id> [name]"
			return m, nil
		}
		id := args[1]
		name := id
		if len(args) > 2 {
			name = strings.Join(args[2:], " ")
		}
		p := domain.Project{ID: id, Name: name}
		if err := m.repo.CreateProject(m.ctx, p); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "project created: " + id
		m = m.reloadProjects()
		m.ProjectID = id
		m.saveLastProject()
		for i, proj := range m.projects {
			if proj.ID == id {
				m.projectSelected = i
				break
			}
		}
		m.mode = viewList
		return m.refreshList()
	case projectSubAdd:
		if len(args) != 2 {
			m.err = "usage: :project add <project_id>"
			return m, nil
		}
		if m.current.Number == EmptyFilter {
			m.err = "open a patent first"
			return m, nil
		}
		targetID := args[1]
		if err := m.repo.AddPatentToProject(m.ctx, targetID, m.current.Number); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = fmt.Sprintf("added %s to project %s", m.current.Number, targetID)
		return m, nil
	case projectSubSwitch:
		if len(args) != 2 {
			m.err = "usage: :project switch <id>"
			return m, nil
		}
		id := args[1]
		p, err := m.repo.GetProject(m.ctx, id)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.ProjectID = p.ID
		m.saveLastProject()
		m.mode = viewList
		m.message = "switched to project: " + p.Name
		return m.refreshList()
	case projectSubStatus:
		if len(args) < 2 {
			m.err = "usage: :project status <status>"
			return m, nil
		}
		status := strings.Join(args[1:], " ")
		p, err := m.repo.GetProject(m.ctx, m.ProjectID)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		p.Status = status
		if err := m.repo.UpdateProject(m.ctx, p); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "project status updated"
		m = m.reloadProjects()
		return m, nil
	case projectSubSummaryStatus:
		if len(args) < 2 {
			m.err = "usage: :project summary-status <work-in-progress|provisional-filed|application-filed|published|granted>"
			return m, nil
		}
		summaryStatus := strings.Join(args[1:], " ")
		validSummaryStatuses := map[string]bool{
			domain.ProjectSummaryStatusWorkInProgress:   true,
			domain.ProjectSummaryStatusProvisionalFiled: true,
			domain.ProjectSummaryStatusApplicationFiled: true,
			domain.ProjectSummaryStatusPublished:        true,
			domain.ProjectSummaryStatusGranted:          true,
		}
		if !validSummaryStatuses[summaryStatus] {
			m.err = "invalid summary status: " + summaryStatus
			return m, nil
		}
		p, err := m.repo.GetProject(m.ctx, m.ProjectID)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		p.SummaryStatus = summaryStatus
		if err := m.repo.UpdateProject(m.ctx, p); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "project summary status updated: " + summaryStatus
		m = m.reloadProjects()
		return m, nil
	case projectSubEvent:
		if len(args) < 2 {
			m.err = "usage: :project event <type> [date YYYY-MM-DD] [due YYYY-MM-DD] [ref <ref>] [note <text>]"
			return m, nil
		}
		eventType := args[1]
		e := domain.ProjectEvent{ProjectID: m.ProjectID, EventType: eventType}
		rest := args[2:]
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case argDate:
				if i+1 < len(rest) {
					i++
					e.EventDate = rest[i]
				}
			case argDue:
				if i+1 < len(rest) {
					i++
					e.DueDate = rest[i]
				}
			case argRef:
				if i+1 < len(rest) {
					i++
					e.Reference = rest[i]
				}
			case argNote:
				if i+1 < len(rest) {
					e.Notes = strings.Join(rest[i+1:], " ")
					i = len(rest)
				}
			}
		}
		if _, err := m.repo.AddProjectEvent(m.ctx, e); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "event added: " + eventType
		return m, nil
	case projectSubEvents:
		if len(m.projects) > 0 {
			m.ProjectID = m.projects[m.projectSelected].ID
		}
		m.projectEventsSelected = 0
		m.mode = viewProjectEvents
		return m, nil
	case projectSubInvoices:
		if len(m.projects) > 0 {
			m.ProjectID = m.projects[m.projectSelected].ID
		}
		m.projectInvoicesSelected = 0
		m.mode = viewProjectInvoices
		return m, nil
	case projectSubInvoice:
		if len(args) < 2 {
			m.err = "usage: :project invoice <amount> [currency USD] [direction to-firm|from-firm] [date YYYY-MM-DD] [due YYYY-MM-DD] [firm <name>] [ref <number>] [note <text>]"
			return m, nil
		}
		inv := domain.ProjectInvoice{
			ProjectID: m.ProjectID,
			Amount:    args[1],
			Currency:  "USD",
			Direction: domain.InvoiceDirectionToFirm,
			Status:    domain.InvoiceStatusOutstanding,
		}
		rest := args[2:]
		for i := 0; i < len(rest); i++ {
			switch rest[i] {
			case argCurrency:
				if i+1 < len(rest) {
					i++
					inv.Currency = rest[i]
				}
			case argDirection:
				if i+1 < len(rest) {
					i++
					inv.Direction = rest[i]
				}
			case argDate:
				if i+1 < len(rest) {
					i++
					inv.InvoiceDate = rest[i]
				}
			case argDue:
				if i+1 < len(rest) {
					i++
					inv.DueDate = rest[i]
				}
			case argFirm:
				if i+1 < len(rest) {
					i++
					inv.FirmName = rest[i]
				}
			case argRef:
				if i+1 < len(rest) {
					i++
					inv.InvoiceNumber = rest[i]
				}
			case argStatus:
				if i+1 < len(rest) {
					i++
					inv.Status = rest[i]
				}
			case argNote:
				if i+1 < len(rest) {
					inv.Notes = strings.Join(rest[i+1:], " ")
					i = len(rest)
				}
			}
		}
		if _, err := m.repo.AddProjectInvoice(m.ctx, inv); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "invoice added: " + inv.Amount + " " + inv.Currency
		m = m.reloadProjects()
		return m, nil
	case projectSubID:
		m.message = fmt.Sprintf("project: %s (%s)", m.projectNameForExport(), m.ProjectID)
		return m, nil
	case projectSubIDS:
		return m.idsCommand(args[1:])
	case projectSubExport:
		return m.exportCommand(args[1:])
	case projectSubSummary:
		if len(args) < 2 {
			m.err = "usage: :project summary <text>"
			return m, nil
		}
		summary := strings.Join(args[1:], " ")
		p, err := m.repo.GetProject(m.ctx, m.ProjectID)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		p.Summary = summary
		if err := m.repo.UpdateProject(m.ctx, p); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "project summary updated"
		m = m.reloadProjects()
		return m, nil
	case projectSubComment:
		if len(args) < 2 {
			m.err = "usage: :project comment <text>"
			return m, nil
		}
		comment := strings.Join(args[1:], " ")
		p, err := m.repo.GetProject(m.ctx, m.ProjectID)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		p.Comments = comment
		if err := m.repo.UpdateProject(m.ctx, p); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "project comment updated"
		m = m.reloadProjects()
		return m, nil
	default:
		m.err = "unknown project subcommand: " + sub
	}
	return m, nil
}

func (m *Model) summarize() (tea.Model, tea.Cmd) {
	if m.current.Number == EmptyFilter {
		m.err = "open a patent before summarizing"
		return m, nil
	}
	classifications, _ := m.repo.ListClassifications(m.ctx, m.ProjectID, m.current.Number)
	cites, _ := m.repo.ListCitations(m.ctx, m.ProjectID, m.current.Number, domain.RelationCites, storage.ListCitationsOptions{})

	// Convert Classification to ClassificationCode for AI summarizer compatibility if needed,
	// but better to update AI summarizer too.
	// For now, let's see what Summarize expects.
	artifact := ai.Summarize(m.current, classifications, cites)
	if _, err := m.repo.AddAIAnalysis(m.ctx, m.ProjectID, artifact); err != nil {
		m.err = err.Error()
		m.logger.Error("summary artifact insert failed", "patent", m.current.Number, "error", err)
		return m, nil
	}
	m.mode = viewAI
	m.message = "created local summary"
	return m, nil
}

func (m *Model) compare(otherNumber string) (tea.Model, tea.Cmd) {
	if m.current.Number == EmptyFilter {
		m.err = "open a patent before comparing"
		return m, nil
	}
	other, err := m.repo.GetPatent(m.ctx, m.ProjectID, otherNumber)
	if err != nil {
		m.err = err.Error()
		m.logger.Error("comparison target open failed", "patent", otherNumber, "error", err)
		return m, nil
	}
	if _, err := m.repo.AddAIAnalysis(m.ctx, m.ProjectID, ai.Compare(m.current, other)); err != nil {
		m.err = err.Error()
		m.logger.Error("comparison artifact insert failed", "patent", m.current.Number, "other", otherNumber, "error", err)
		return m, nil
	}
	m.mode = viewAI
	m.message = "created local comparison"
	return m, nil
}

func (m *Model) refCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) != 1 {
		m.err = "usage: :ref add or :ref export"
		return m, nil
	}
	switch args[0] {
	case refActionAdd:
		if m.current.Number == EmptyFilter {
			m.err = "open a patent before adding a reference"
			return m, nil
		}
		if _, err := m.repo.AddReference(m.ctx, m.ProjectID, m.current.Number, sqliterepo.CitationLabel(m.current)); err != nil {
			m.err = err.Error()
			m.logger.Error("reference insert failed", "patent", m.current.Number, "error", err)
			return m, nil
		}
		m.logActivity(ActivityRefAdd, m.current.Number, "")
		m.message = "reference added"
	case refActionExport:
		m.mode = viewRefs
		m.message = "Markdown reference export is shown below"
	default:
		m.err = "usage: :ref add or :ref export"
	}
	return m, nil
}

func (m *Model) styleRow(index int, selected int, content string) string {
	return m.styleRowW(index, selected, content, m.width)
}

func (m *Model) styleRowW(index int, selected int, content string, targetWidth int) string {
	style := lipgloss.NewStyle()
	if m.isInSelection(index) {
		style = style.Background(lipgloss.Color(ColorSelection))
	} else if index == selected {
		style = style.Background(lipgloss.Color(ColorHighlight))
	} else if index%2 != 0 {
		style = style.Background(lipgloss.Color(ColorAltRow))
	}
	if targetWidth > 0 {
		cw := lipgloss.Width(content)
		if cw < targetWidth {
			content += strings.Repeat(" ", targetWidth-cw)
		}
	}
	return style.Render(content)
}

// styleRowOverlay is like styleRowW but uses ColorSurface as the even-row base
// so that all rows inside overlay popups have a consistent background.
func (m *Model) styleRowOverlay(index int, selected int, content string, targetWidth int) string {
	style := overlayBase()
	if m.isInSelection(index) {
		style = style.Background(lipgloss.Color(ColorSelection))
	} else if index == selected {
		if m.isPopupSearchMode() && m.popupSearchQuery != "" {
			style = style.
				Background(lipgloss.Color(ColorWarning)).
				Foreground(lipgloss.Color(ColorBlack)).
				Bold(true)
		} else {
			style = style.Background(lipgloss.Color(ColorHighlight))
		}
	} else if index%2 != 0 {
		style = style.Background(lipgloss.Color(ColorAltRow))
	}
	if targetWidth > 0 {
		cw := lipgloss.Width(content)
		if cw < targetWidth {
			content += strings.Repeat(" ", targetWidth-cw)
		}
	}
	return style.Render(content)
}

type listColumn struct {
	label     string
	width     int
	id        string
	jumpLabel string
}

const listColumnIDS = domain.SortColumnIDS

func fitColumns(cols []listColumn, available int, minWidths map[string]int, shrinkOrder []string) []listColumn {
	if len(cols) == 0 || available <= 0 {
		return cols
	}

	fitted := append([]listColumn(nil), cols...)
	totalWidth := func(columns []listColumn) int {
		total := 0
		for i, col := range columns {
			total += col.width
			if i < len(columns)-1 {
				total += 2
			}
		}
		return total
	}

	for totalWidth(fitted) > available {
		shrunk := false
		for _, id := range shrinkOrder {
			for i := range fitted {
				minWidth := minWidths[fitted[i].id]
				if fitted[i].id == id && fitted[i].width > minWidth {
					fitted[i].width--
					shrunk = true
					break
				}
			}
			if shrunk {
				break
			}
		}
		if !shrunk {
			break
		}
	}

	return fitted
}

func (m *Model) listColumns() []listColumn {
	numWidth := max(6, m.listNumWidth)
	titleWidth := 40
	invWidth := 20
	cpcWidth := 15
	expWidth := 12
	statusWidth := 10
	idsWidth := 11
	updatedWidth := 16
	notesWidth := 6

	return []listColumn{
		{"Number", numWidth, domain.SortColumnNumber, jumpLabelPublication},
		{"Title", titleWidth, domain.SortColumnTitle, ""},
		{"Inventor", invWidth, domain.SortColumnInventor, jumpLabelInventors},
		{"Classification", cpcWidth, domain.SortColumnCPC, jumpLabelClassification},
		{"Expires", expWidth, domain.SortColumnExpiration, jumpLabelExpiration},
		{"Status", statusWidth, domain.SortColumnStatus, keyStatus},
		{"Updated", updatedWidth, domain.SortColumnUpdated, jumpLabelUpdated},
		{"Notes", notesWidth, domain.SortColumnNotes, jumpLabelNotes},
		{"IDS", idsWidth, listColumnIDS, keyIDS},
	}
}

func (m *Model) fitListColumns(cols []listColumn, available int) []listColumn {
	if len(cols) == 0 || available <= 0 {
		return cols
	}
	minWidths := map[string]int{
		domain.SortColumnNumber:     12,
		domain.SortColumnTitle:      18,
		domain.SortColumnInventor:   10,
		domain.SortColumnCPC:        8,
		domain.SortColumnExpiration: 10,
		domain.SortColumnStatus:     8,
		domain.SortColumnUpdated:    10,
		domain.SortColumnNotes:      5,
		listColumnIDS:               7,
	}
	shrinkOrder := []string{
		domain.SortColumnTitle,
		domain.SortColumnInventor,
		domain.SortColumnCPC,
		domain.SortColumnUpdated,
		domain.SortColumnNumber,
		listColumnIDS,
		domain.SortColumnStatus,
		domain.SortColumnExpiration,
		domain.SortColumnNotes,
	}
	return fitColumns(cols, available, minWidths, shrinkOrder)
}

func (m *Model) View() string {
	start := time.Now()
	// Pre-calculate jump labels for this render frame
	m.jumpLabelsCache = m.jumpLabels()

	spec := mustModeSpec(m.mode)
	var res string

	if spec.isOverlay {
		content := m.renderModeBody(m.mode)
		res = m.previewOverlay(content)
	} else {
		res = m.renderView()
	}

	if os.Getenv("PATENT_DEBUG") == "1" {
		elapsed := time.Since(start)
		m.logger.Debug("tui.render_frame", "mode", m.mode, "duration_ms", elapsed.Milliseconds())
	}

	// Enforce global background only for non-splash contexts
	if !m.isSplashContext() {
		res = lipgloss.NewStyle().
			Width(m.width).
			Render(res)

		return lipgloss.Place(m.width, m.height,
			lipgloss.Left, lipgloss.Top,
			res)
	}

	return res
}

func (m *Model) viewClaim() string {
	base := overlayBase()
	var b strings.Builder
	b.WriteString(m.renderPopupHeader("Claim 1"))
	text := m.current.FirstClaim
	if text == "" {
		text = m.text.T(TextValueEmpty)
	}
	// Wrap text to fit overlay
	width := m.overlayWidth() - 4
	b.WriteString(base.Render(wrapText(text, width)))
	return b.String()
}

func (m *Model) viewAbstract() string {
	base := overlayBase()
	var b strings.Builder
	b.WriteString(m.renderPopupHeader("Abstract"))
	text := m.current.Abstract
	if text == "" {
		text = m.text.T(TextValueEmpty)
	}
	// Wrap text to fit overlay
	width := m.overlayWidth() - 4
	b.WriteString(base.Render(wrapText(text, width)))
	return b.String()
}

func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	var b strings.Builder
	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}
	line := words[0]
	for _, word := range words[1:] {
		if lipgloss.Width(line+" "+word) <= width {
			line += " " + word
		} else {
			b.WriteString(line + "\n")
			line = word
		}
	}
	b.WriteString(line)
	return b.String()
}

func wrapIndentedText(text string, width int, indent string) string {
	wrapped := wrapText(text, width)
	if !strings.Contains(wrapped, "\n") {
		return wrapped
	}
	lines := strings.Split(wrapped, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = indent + lines[i]
	}
	return strings.Join(lines, "\n")
}

func (m *Model) styleLine(content string) string {
	return lipgloss.NewStyle().Width(m.width).Render(content)
}

func (m *Model) renderView() string {
	mode := m.mode
	if mode == viewSplash {
		return m.viewSplash()
	}

	lineStyle := lipgloss.NewStyle().Width(m.width)
	ruleStyle := lipgloss.NewStyle().Width(m.width).Foreground(lipgloss.Color(ColorSubtle))

	var b strings.Builder
	b.WriteString(m.renderScreenHeader())
	b.WriteString("\n")

	if m.input.Focused() {
		isPopupSearch := m.isPopupMode() && strings.HasPrefix(m.input.Value(), keySearch)
		if !isPopupSearch {
			b.WriteString(lineStyle.Render(m.input.View()) + "\n")
		} else {
			b.WriteString(lineStyle.Render(m.navDefault()) + "\n")
		}
	} else if m.jumpMode {
		prefix := ""
		if m.visualMode {
			prefix = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color(ColorSelection)).Foreground(lipgloss.Color(ColorWhite)).Render(" VISUAL ") + " "
		}
		b.WriteString(lineStyle.Render(prefix+m.text.T(TextNavJump)) + "\n")
	} else {
		prefix := ""
		if m.visualMode {
			prefix = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color(ColorSelection)).Foreground(lipgloss.Color(ColorWhite)).Render(" VISUAL ") + " "
		}
		b.WriteString(lineStyle.Render(prefix+m.navDefault()) + "\n")
	}
	b.WriteString(ruleStyle.Render(strings.Repeat("─", m.width)) + "\n")
	b.WriteString(m.renderModeBody(mode))

	if m.err != "" || m.message != "" {
		b.WriteString("\n" + ruleStyle.Render(strings.Repeat("─", m.width)) + "\n")
		if m.err != "" {
			errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorError))
			b.WriteString(lineStyle.Render(errStyle.Render(m.err)) + "\n")
		} else {
			messageStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSuccess))
			if strings.HasPrefix(m.message, "found match") {
				messageStyle = messageStyle.Bold(true)
			}
			b.WriteString(lineStyle.Render(messageStyle.Render(m.message)) + "\n")
		}
	}
	return b.String()
}

func (m *Model) renderModeBody(mode viewMode) string {
	switch mode {
	case viewList:
		return m.viewList()
	case viewDetail:
		return m.viewDetail()
	case viewCites:
		return m.viewCitations(domain.RelationCites)
	case viewCitedBy:
		return m.viewCitations(domain.RelationCitedBy)
	case viewClassifications:
		return m.viewClassifications()
	case viewText:
		return m.viewText()
	case viewNotes:
		return m.viewNotes()
	case viewRefs:
		return m.viewRefs()
	case viewAI:
		return m.viewAI()
	case viewHelp:
		return m.viewHelp()
	case viewHelpPopup:
		return m.viewHelpPopup()
	case viewPreview:
		return m.viewPreview()
	case viewReview:
		return m.viewReviewQueue()
	case viewConfirmDelete:
		return m.viewConfirmDelete()
	case viewClassificationDetail:
		return m.viewClassificationDetail()
	case viewInventors:
		return m.viewInventors()
	case viewFamily:
		return m.viewFamilyOverlay()
	case viewSplash:
		return m.viewSplash()
	case viewProjectEvents:
		return m.viewProjectEvents()
	case viewProjectInvoices:
		return m.viewProjectInvoices()
	case viewProjectIDS:
		return m.viewProjectIDS()
	case viewProjectInfo:
		return m.viewProjectInfo()
	case viewNoteEdit:
		return m.viewNoteEdit()
	case viewIDSEdit:
		return m.viewIDSEdit()
	case viewDateEdit:
		return m.viewDateEdit()
	case viewAbstract:
		return m.viewAbstract()
	case viewClaim:
		return m.viewClaim()
	case viewUSPTOKeyWarning:
		return m.viewUSPTOKeyWarning()
	case viewBulkConfirm:
		return m.viewBulkConfirm()
	case viewStatusSelect:
		return m.viewStatusSelect()
	default:
		return ""
	}
}

func (m *Model) viewDateEdit() string {
	title := "Edit Date"
	switch m.editDateType {
	case domain.LifecycleTypeApp:
		title = "Edit Application Date"
	case domain.LifecycleTypePub:
		title = "Edit Publication Date"
	case domain.LifecycleTypeGrant:
		title = "Edit Grant Date"
	case domain.LifecycleTypeExp:
		title = "Edit Expiration Date"
	}
	return m.renderPopup(title, m.dateInput.View())
}

func (m *Model) selectableStatuses() []string {
	return []string{
		domain.CitationStatusStored,
		domain.CitationStatusUnderReview,
		domain.CitationStatusIgnored,
	}
}

func (m *Model) viewStatusSelect() string {
	var body strings.Builder
	statuses := m.selectableStatuses()
	for i, s := range statuses {
		cursor := "  "
		if i == m.statusSelected {
			cursor = "> "
		}
		style := overlayBase()
		if color, ok := StatusColors[s]; ok {
			style = style.Foreground(lipgloss.Color(color))
		}
		if i == m.statusSelected {
			style = style.Bold(true).Underline(true)
		}
		body.WriteString(cursor + style.Render(s) + "\n")
	}
	return m.renderPopup("Change Status", body.String())
}

func (m *Model) viewBulkConfirm() string {
	count := len(m.bulkActionIndices)
	var actionVerb string
	switch m.bulkAction {
	case bulkActionStore:
		actionVerb = "save"
	case bulkActionIgnore:
		actionVerb = "ignore"
	case bulkActionUnderReview:
		actionVerb = "mark as under review"
	}
	content := fmt.Sprintf("Do you want to %s %d citation(s) in the database?", actionVerb, count)
	return m.renderPopup("Bulk Action Confirmation", content)
}

func (m *Model) viewUSPTOKeyWarning() string {
	var body strings.Builder
	body.WriteString("The application is configured to use the USPTO Open Data Portal,\n")
	body.WriteString("but no API key was found in your configuration.\n\n")
	body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorTheme)).Render("Fallback: Switching to Google Patents for this session."))
	body.WriteString("\n\n")
	body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Render("To fix this, add your key to configs/config.toml or ~/.ssh/uspto_odp_key"))
	return m.renderPopup("USPTO API Key Missing", body.String())
}

func (m *Model) isSplashContext() bool {
	return m.mode == viewSplash
}

func (m *Model) displayVersion() string {
	if strings.TrimSpace(m.version) == "" {
		return "dev"
	}
	return m.version
}

func (m *Model) navDefault() string {
	if m.mode == viewList {
		return fmt.Sprintf(m.text.T(TextNavList), keyVimDown, keyVimUp, keyColLeft, keyColRight, keyEnter, keyJump, keySort, keyCommand, keySearch, keyHelp, keyBack, keyQuit)
	}
	return fmt.Sprintf(m.text.T(TextNavDefault), keyVimDown, keyVimUp, keyEnter, keyJump, keySort, keyCommand, keySearch, keyHelp, keyBack, keyQuit)
}

func (m *Model) viewList() string {
	if len(m.patents) == 0 {
		return m.text.T(TextListEmpty) + "\n"
	}
	var b strings.Builder
	var activeFilters []string
	if m.filter != EmptyFilter {
		activeFilters = append(activeFilters, m.filter)
	}
	if m.classFilter != EmptyFilter {
		activeFilters = append(activeFilters, "class:"+m.classFilter)
	}
	if len(activeFilters) > 0 {
		b.WriteString(m.styleLine(m.text.T(TextListFilter)+": "+strings.Join(activeFilters, " · ")) + "\n")
		b.WriteString(m.styleLine("") + "\n")
	}

	window := pageWindow(m.selected, len(m.patents), m.pageSize())
	status := pageStatus(m.text.T(TextValuePageStatus), window)
	b.WriteString(m.styleLine(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Render(status)) + "\n")
	b.WriteString(m.styleLine("") + "\n")
	idsByPatent := map[string]string{}
	if idsEntries, err := m.repo.ListIDSEntries(m.ctx, m.ProjectID); err == nil {
		for _, entry := range idsEntries {
			idsByPatent[entry.PatentNumber] = string(entry.Status)
		}
	}

	idxWidth := len(fmt.Sprintf("%d", len(m.patents)))
	if idxWidth < 2 {
		idxWidth = 2
	}

	// Account for jump prefix width in header if jump targets exist
	jumpPrefixWidth := 0
	if m.hasJumpTargets() {
		jumpPrefixWidth = 2
	}

	cols := m.listColumns()
	cols = m.fitListColumns(cols, m.width-(2+jumpPrefixWidth+idxWidth+2))

	// Clamp sortColumnIndex
	if m.sortColumnIndex >= len(cols) {
		m.sortColumnIndex = len(cols) - 1
	}

	header := m.pad("  ", 2) +
		m.pad("", jumpPrefixWidth) +
		m.pad("#", idxWidth+2)

	for i, c := range cols {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Underline(true)
		label := c.label
		if m.sortColumn == c.id {
			indicator := " ▴"
			if m.sortOrder == domain.SortOrderDesc {
				indicator = " ▾"
			}
			label += indicator
		}
		if i == m.sortColumnIndex {
			style = style.Foreground(lipgloss.Color(ColorYellow)).Underline(true).Bold(true)
		}

		jumpColLabel := ""
		if m.jumpMode {
			jumpColLabel = c.jumpLabel
		}
		jumpColPrefix := ""
		if jumpColLabel != "" {
			jumpColPrefix = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorYellow)).Render(jumpColLabel) + " "
		}

		// Calculate total width for this column header (visible chars)
		colWidth := c.width
		if jumpColLabel != "" {
			colWidth = max(colWidth, lipgloss.Width(jumpColLabel+" "+label))
		}

		padding := 2
		if i == len(cols)-1 {
			padding = 0
		}
		header += m.pad(jumpColPrefix+style.Render(label), colWidth+padding)
	}

	b.WriteString(m.styleLine(header) + "\n")

	for i := window.Start; i < window.End; i++ {
		p := m.patents[i]
		prefix := "  "
		if i == m.selected {
			prefix = "> "
		}

		jumpRowPrefix := ""
		if m.jumpMode {
			jumpIdx := len(cols) + (i - window.Start)
			if jumpIdx < len(m.jumpLabelsCache) {
				label := m.jumpLabelsCache[jumpIdx]
				color := ColorYellow
				if !label.preferred {
					color = ColorThemeDetail
				}
				jumpRowPrefix = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(label.key) + " "
			}
		}
		if jumpRowPrefix == "" && jumpPrefixWidth > 0 {
			jumpRowPrefix = strings.Repeat(" ", jumpPrefixWidth)
		}

		rowValues := map[string]string{
			domain.SortColumnNumber:     p.Number,
			domain.SortColumnTitle:      p.Title,
			domain.SortColumnInventor:   formatInventorsShort(p.Inventors),
			domain.SortColumnCPC:        p.ClassificationLabel,
			domain.SortColumnExpiration: p.ExpirationDate,
			domain.SortColumnStatus:     p.Status,
			domain.SortColumnUpdated:    formatStoredTime(p.UpdatedAt, "-"),
			domain.SortColumnNotes:      "-",
			listColumnIDS:               "-",
		}
		if rowValues[domain.SortColumnCPC] == "" {
			rowValues[domain.SortColumnCPC] = "-"
		}
		if rowValues[domain.SortColumnExpiration] == "" {
			rowValues[domain.SortColumnExpiration] = "-"
		}
		if p.NotesCount > 0 {
			rowValues[domain.SortColumnNotes] = fmt.Sprintf("%d", p.NotesCount)
		}
		if idsStatus, ok := idsByPatent[p.Number]; ok && idsStatus != "" {
			rowValues[listColumnIDS] = idsStatus
		}

		idxLabel := fmt.Sprintf("%*d", idxWidth, i+1)
		row := m.pad(prefix, 2) +
			m.pad(jumpRowPrefix, jumpPrefixWidth) +
			m.pad(idxLabel, idxWidth+2)

		for j, c := range cols {
			val := rowValues[c.id]

			// Use the same width calculation as the header for alignment
			colWidth := c.width
			jumpColLabel := ""
			if m.jumpMode && j < len(m.jumpLabelsCache) {
				jumpColLabel = m.jumpLabelsCache[j].key
			}
			if jumpColLabel != "" {
				colWidth = max(colWidth, lipgloss.Width(jumpColLabel+" "+c.label))
			}
			val = m.truncate(val, c.width)

			padding := 2
			if j == len(cols)-1 {
				padding = 0
			}
			row += m.pad(val, colWidth+padding)
		}

		style := lipgloss.NewStyle()
		if color, ok := StatusColors[p.Status]; ok {
			style = style.Foreground(lipgloss.Color(color))
		}

		if i == m.selected {
			style = style.Bold(true)
		}

		b.WriteString(m.styleRow(i, m.selected, style.Render(row)) + "\n")
	}
	return b.String()
}

func (m *Model) pad(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func formatInventorsShort(inventors []string) string {
	if len(inventors) == 0 {
		return "-"
	}
	if len(inventors) == 1 {
		return inventors[0]
	}
	return inventors[0] + " et al."
}

func (m *Model) truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if width <= 3 {
		if lipgloss.Width(s) <= width {
			return s
		}
		return strings.Repeat(".", width)
	}
	if lipgloss.Width(s) <= width {
		return s
	}

	var b strings.Builder
	currentWidth := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if currentWidth+rw > width-3 {
			b.WriteString("...")
			break
		}
		b.WriteRune(r)
		currentWidth += rw
	}
	return b.String()
}

func (m *Model) viewDetail() string {
	p := m.current
	style := lipgloss.NewStyle().
		Width(m.width)
	var b strings.Builder
	b.WriteString(style.Bold(true).Render(p.Number) + "\n")
	b.WriteString(style.Render(p.Title) + "\n\n")
	fields := m.detailFields()

	// Calculate max label width per group for alignment
	groupWidths := []int{}
	currentMax := 0
	for _, f := range fields {
		if f.separator {
			groupWidths = append(groupWidths, currentMax)
			currentMax = 0
		} else {
			w := lipgloss.Width(m.text.T(f.label) + ":")
			if w > currentMax {
				currentMax = w
			}
		}
	}
	groupWidths = append(groupWidths, currentMax)

	selected := clamp(m.detailSelected, 0, max(0, len(fields)-1))
	groupIndex := 0
	ruleStyle := lipgloss.NewStyle().Width(m.width).Foreground(lipgloss.Color(ColorDim))
	separator := ruleStyle.Render(strings.Repeat("─", m.width))

	for i, field := range fields {
		if field.separator {
			b.WriteString(separator + "\n")
			groupIndex++
			continue
		}
		prefix := "  "
		if i == selected {
			prefix = "> "
		}
		value := field.value
		if field.displayValue != "" {
			value = field.displayValue
		}
		lead := prefix + m.jumpPrefix(i)
		b.WriteString(style.Render(lead+m.detailRow(field.label, value, groupWidths[groupIndex], lipgloss.Width(lead))) + "\n")
	}
	b.WriteString(separator + "\n")

	return b.String()
}

type detailField struct {
	label        TextKey
	value        string
	displayValue string
	jumpLabel    string
	action       detailAction
	data         string // extra data for actions
	separator    bool
}

type detailAction int

const (
	detailActionNone detailAction = iota
	detailActionCitations
	detailActionCitedBy
	detailActionClassification
	detailActionInventors
	detailActionFamily
	detailActionNotes
	detailActionFirstClaim
	detailActionAbstract
	detailActionIDS
	detailActionEditDate
	detailActionEditNumber
	detailActionStatic
)

func (m *Model) detailFields() []detailField {
	p := m.current
	// Use cached values if available and matching current patent
	cache := m.detailCache
	if cache.Number != p.Number {
		m.populateDetailCache()
		cache = m.detailCache
	}

	formatLifecycle := func(num, date string) string {
		num = strings.TrimSpace(num)
		date = strings.TrimSpace(date)
		if num == "" && date == "" {
			return m.text.T(TextValuePending)
		}
		if num != "" && date != "" {
			return fmt.Sprintf("%s (%s)", num, date)
		}
		if num != "" {
			return num
		}
		return date
	}

	fields := []detailField{
		{label: TextDetailAssignee, value: p.Assignee, jumpLabel: jumpLabelAssignee},
		{label: TextDetailLatestAssignment, value: p.LatestAssignment, jumpLabel: jumpLabelLatestAssignment},
	}

	// Add grouped inventors as a single field
	if len(p.Inventors) > 0 {
		fields = append(fields, detailField{
			label:        TextDetailInventors,
			value:        strings.Join(p.Inventors, ", "),
			displayValue: fmt.Sprintf("(%d) %s", len(p.Inventors), strings.Join(p.Inventors, ", ")),
			jumpLabel:    jumpLabelInventors,
			action:       detailActionInventors,
		})
	} else {
		fields = append(fields, detailField{label: TextDetailInventors, jumpLabel: jumpLabelInventors})
	}

	fields = append(fields,
		detailField{label: TextDetailApplication, value: formatLifecycle(p.ApplicationNumber, p.ApplicationDate), jumpLabel: jumpLabelApplication, action: detailActionEditDate, data: domain.LifecycleTypeApp},
		detailField{label: TextDetailPublicationLong, value: formatLifecycle(p.PublicationNumber, p.PublicationDate), jumpLabel: jumpLabelPublication, action: detailActionEditDate, data: domain.LifecycleTypePub},
		detailField{label: TextDetailGrantLong, value: formatLifecycle(p.GrantNumber, p.GrantDate), jumpLabel: jumpLabelGrant, action: detailActionEditDate, data: domain.LifecycleTypeGrant},
		detailField{label: TextDetailExpiration, value: m.formatExpiration(p), jumpLabel: jumpLabelExpiration, action: detailActionEditDate, data: domain.LifecycleTypeExp},
	)

	// Add grouped Classification codes as a single field
	cpcs := strings.Split(p.ClassificationLabel, ", ")
	var validClassifications []string
	for _, c := range cpcs {
		if strings.TrimSpace(c) != "" {
			validClassifications = append(validClassifications, strings.TrimSpace(c))
		}
	}
	classificationValue := m.text.T(TextValueEmpty)
	classificationDisplayValue := classificationValue
	if len(validClassifications) > 0 {
		classificationValue = p.ClassificationLabel
		classificationDisplayValue = fmt.Sprintf("(%d) %s", len(validClassifications), p.ClassificationLabel)
	}
	fields = append(fields, detailField{
		label:        TextDetailClassification,
		value:        classificationValue,
		displayValue: classificationDisplayValue,
		jumpLabel:    jumpLabelClassification,
		action:       detailActionClassification,
	})

	fields = append(fields,
		detailField{label: TextDetailCitationCount, value: m.formatCitationSummary(cache.CitationCount, p.ExpectedCitations, cache.CitationRefreshedAt), jumpLabel: jumpLabelCitationCount, action: detailActionCitations},
		detailField{label: TextDetailCitedByCount, value: m.formatCitationSummary(cache.CitedByCount, p.ExpectedCitedBy, cache.CitedByRefreshedAt), jumpLabel: jumpLabelCitedByCount, action: detailActionCitedBy},
	)

	notesValue := m.text.T(TextValueEmpty)
	notesDisplay := notesValue
	abstractValue := m.text.T(TextValueEmpty)
	if p.Abstract != "" {
		abstractValue = m.truncate(p.Abstract, 60)
	}

	if p.Number != "" {
		parents, children := cache.Parents, cache.Children

		parentValue, parentDisplay := "-", "-"
		if len(parents) > 0 {
			nums := make([]string, len(parents))
			for i, e := range parents {
				nums[i] = e.ParentNumber
			}
			parentValue = strings.Join(nums, ", ")
			parentDisplay = fmt.Sprintf("(%d) %s", len(parents), parentValue)
		}
		fields = append(fields, detailField{
			label:        TextDetailFamilyParents,
			value:        parentValue,
			displayValue: parentDisplay,
			jumpLabel:    jumpLabelFamilyParents,
			action:       detailActionFamily,
		})

		childValue, childDisplay := "-", "-"
		if len(children) > 0 {
			nums := make([]string, len(children))
			for i, e := range children {
				nums[i] = e.ChildNumber
			}
			childValue = strings.Join(nums, ", ")
			childDisplay = fmt.Sprintf("(%d) %s", len(children), childValue)
		}
		fields = append(fields, detailField{
			label:        TextDetailFamilyChildren,
			value:        childValue,
			displayValue: childDisplay,
			jumpLabel:    jumpLabelFamilyChildren,
			action:       detailActionFamily,
		})

		// Add Status and IDS
		if color, ok := StatusColors[p.Status]; ok {
			fields = append(fields, detailField{
				label:        TextDetailStatus,
				displayValue: lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(p.Status),
				jumpLabel:    keyStatus,
				action:       detailActionNone, // cycle status is handled by 's' globally
			})
		}
		idsEntry := m.idsEntryForPatent(p.Number)
		if idsEntry != nil {
			statusColor := idsStatusColor(idsEntry.Status)
			value := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(string(idsEntry.Status))
			if idsEntry.Notes != "" {
				value += lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDim)).Italic(true).Render("  " + idsEntry.Notes)
			}
			fields = append(fields, detailField{
				label:        TextDetailIDS,
				displayValue: value,
				jumpLabel:    keyIDS,
				action:       detailActionIDS,
			})
		} else {
			fields = append(fields, detailField{
				label:        TextDetailIDS,
				displayValue: lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDim)).Italic(true).Render("Not in IDS"),
				jumpLabel:    keyIDS,
				action:       detailActionIDS,
			})
		}

		if len(cache.Notes) > 0 {
			notesValue = fmt.Sprintf("(%d)", len(cache.Notes))
			notesDisplay = fmt.Sprintf("(%d) %s", len(cache.Notes), markdownHeadingSummary(cache.Notes[0].Body))
		}
	}

	// Add First Claim field
	claimValue := m.text.T(TextValueEmpty)
	claimDisplay := claimValue
	if p.FirstClaim != "" {
		claimValue = p.FirstClaim
		claimDisplay = m.truncate(p.FirstClaim, 120) // Roughly 2 lines
	}

	fields = append(fields,
		detailField{separator: true},
		detailField{
			label:        TextDetailFirstClaim,
			value:        claimValue,
			displayValue: claimDisplay,
			jumpLabel:    jumpLabelFirstClaim,
			action:       detailActionFirstClaim,
		},
		detailField{
			label:        TextDetailAbstract,
			value:        p.Abstract,
			displayValue: abstractValue,
			jumpLabel:    jumpLabelAbstract,
			action:       detailActionAbstract,
		},
		detailField{
			label:        TextDetailNotes,
			value:        notesValue,
			displayValue: notesDisplay,
			jumpLabel:    jumpLabelNotes,
			action:       detailActionNotes,
		},
	)

	importSourceValue := p.ImportSource
	if importSourceValue == "" {
		importSourceValue = m.text.T(TextValueUnknown)
	}
	fields = append(fields,
		detailField{separator: true},
		detailField{label: TextDetailImportSource, value: importSourceValue},
		detailField{label: TextDetailSource, value: p.SourceGoogleURL, jumpLabel: jumpLabelSource},
		detailField{label: TextDetailStoredLocal, value: formatStoredTime(p.StoredAt, m.text.T(TextValueUnknown)), jumpLabel: jumpLabelStoredLocal},
		detailField{label: TextDetailUpdated, value: formatStoredTime(p.UpdatedAt, m.text.T(TextValueUnknown)), jumpLabel: jumpLabelUpdated},
	)

	return fields
}

func (m *Model) citationStats(number string) (int, time.Time, int, time.Time) {
	if strings.TrimSpace(number) == "" || m.repo == nil {
		return 0, time.Time{}, 0, time.Time{}
	}
	citations, _ := m.repo.ListCitations(m.ctx, m.ProjectID, number, domain.RelationCites, storage.ListCitationsOptions{})
	citedBy, _ := m.repo.ListCitations(m.ctx, m.ProjectID, number, domain.RelationCitedBy, storage.ListCitationsOptions{})
	return len(citations), latestCitationRefresh(citations), len(citedBy), latestCitationRefresh(citedBy)
}

func (m *Model) formatCitationSummary(count int, expected int, refreshedAt time.Time) string {
	refreshed := formatCitationTime(refreshedAt, m.text.T(TextCitationNeverRefreshed))
	expectedStr := "—"
	if expected >= 0 {
		expectedStr = fmt.Sprintf("%d", expected)
	}
	actualStr := fmt.Sprintf("%d", count)
	if count == 0 && expected > 0 {
		actualStr = "—"
	}
	if count == 0 && expected <= 0 {
		actualStr = "—"
		expectedStr = "—"
	}
	return fmt.Sprintf("%s/%-4s  %s: %s", actualStr, expectedStr, m.text.T(TextCitationRefreshed), refreshed)
}

func latestCitationRefresh(edges []domain.CitationEdge) time.Time {
	var latest time.Time
	for _, edge := range edges {
		if edge.RefreshedAt.After(latest) {
			latest = edge.RefreshedAt
		}
	}
	return latest
}

func (m *Model) jumpLabelForInventor(index int) string {
	if index == 0 {
		return jumpLabelInventors
	}
	numberIndex := index
	if numberIndex >= 0 && numberIndex < len(inventorJumpNumberLabels) {
		return string(inventorJumpNumberLabels[numberIndex])
	}
	return m.fallbackJumpLabels(index+1, []string{
		jumpLabelAssignee,
		jumpLabelInventors,
		jumpLabelPublication,
		jumpLabelGrant,
		jumpLabelExpiration,
		jumpLabelStoredLocal,
		jumpLabelUpdated,
		jumpLabelSource,
	})[index].key
}

func (m *Model) detailRow(label TextKey, value string, width int, leadWidth ...int) string {
	if strings.TrimSpace(value) == "" {
		value = m.text.T(TextValueUnknown)
	}
	prefixWidth := 0
	if len(leadWidth) > 0 {
		prefixWidth = leadWidth[0]
	}
	l := m.text.T(label) + ":"
	padding := ""
	if w := lipgloss.Width(l); w < width {
		padding = strings.Repeat(" ", width-w)
	}
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle))
	indent := strings.Repeat(" ", prefixWidth+width+1)
	if m.width > 0 && !strings.Contains(value, "\x1b[") {
		value = wrapIndentedText(value, max(12, m.width-prefixWidth-width-1), indent)
	}
	return fmt.Sprintf("%s%s %s", labelStyle.Render(l), padding, value)
}

func (m *Model) populateDetailCache() {
	p := m.current
	if p.Number == "" || m.repo == nil {
		return
	}
	c1, r1, c2, r2 := m.citationStats(p.Number)
	parents, children, _ := m.repo.ListFamilyEdges(m.ctx, m.ProjectID, p.Number)
	notes, _ := m.repo.ListNotes(m.ctx, m.ProjectID, p.Number)
	ids, _ := m.repo.ListIDSEntries(m.ctx, m.ProjectID)

	m.detailCache = detailCache{
		Number:              p.Number,
		CitationCount:       c1,
		CitationRefreshedAt: r1,
		CitedByCount:        c2,
		CitedByRefreshedAt:  r2,
		ExpectedCitations:   p.ExpectedCitations,
		ExpectedCitedBy:     p.ExpectedCitedBy,
		Parents:             parents,
		Children:            children,
		Notes:               notes,
		IDSEntries:          ids,
	}
}
func (m *Model) formatExpiration(p domain.Patent) string {
	if p.ExpirationDate == "" {
		return m.text.T(TextValueUnknown)
	}
	label := p.ExpirationDate
	switch p.ExpirationSource {
	case domain.ExpirationSourceManual:
		label += m.text.T(TextValueExpirationManual)
	case domain.ExpirationSourceImported:
		label += m.text.T(TextValueExpirationImported)
	case domain.ExpirationSourceEstimated:
		label += m.text.T(TextValueExpirationEstimated)
	}
	// Highlight if expired

	if p.IsExpired(time.Now()) {
		return lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color(ColorDisabled)).Render(label)
	}
	return label
}

func (m *Model) viewCitations(relation string) string {
	if m.current.Number == EmptyFilter {
		return m.renderPopup("Citations", "Open a patent first.\n")
	}
	opts := storage.ListCitationsOptions{
		SortColumn:   m.sortColumn,
		SortOrder:    m.sortOrder,
		StatusFilter: m.citesStatusFilter,
	}
	edges, err := m.repo.ListCitations(m.ctx, m.ProjectID, m.current.Number, relation, opts)
	if err != nil {
		return m.renderPopup("Citations", err.Error()+"\n")
	}
	if len(edges) == 0 {
		return m.renderPopup("Citations", m.text.T(TextCitationsEmpty)+"\n")
	}
	selected := clamp(m.citationSelection(), 0, len(edges)-1)
	m.setCitationSelection(selected)
	window := pageWindow(selected, len(edges), m.overlayPageSize())
	var body strings.Builder
	body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Render(pageStatus(m.text.T(TextValuePageStatus), window)))
	body.WriteString("\n\n")

	indexWidth := 4
	numWidth := 16
	titleWidth := max(20, m.overlayWidth()-64)
	invWidth := 20
	expWidth := 12
	statusWidth := 10

	// Account for jump prefix width in header if jump targets exist
	jumpPrefixWidth := 0
	if m.hasJumpTargets() {
		jumpPrefixWidth = 2
	}

	cols := []listColumn{
		{label: "Number", width: numWidth, id: "number"},
		{label: "Title", width: titleWidth, id: "title"},
		{label: "Inventor", width: invWidth, id: "inventor"},
		{label: "Expires", width: expWidth, id: "expires"},
		{label: "Status", width: statusWidth, id: "status"},
	}
	cols = fitColumns(cols, m.overlayWidth()-4-(2+jumpPrefixWidth+indexWidth), map[string]int{
		"number":   12,
		"title":    18,
		"inventor": 10,
		"expires":  10,
		"status":   8,
	}, []string{"title", "inventor", "number", "status", "expires"})

	header := m.pad("  ", 2) +
		m.pad("", jumpPrefixWidth) +
		m.pad("#", indexWidth)

	for i, c := range cols {
		padding := 2
		if i == len(cols)-1 {
			padding = 0
		}
		header += m.pad(c.label, c.width+padding)
	}

	body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Underline(true).Render(header))
	body.WriteString("\n")

	for i := window.Start; i < window.End; i++ {
		prefix := "  "
		if i == selected {
			prefix = "> "
		}

		jumpPrefix := m.jumpPrefix(i - window.Start)
		if jumpPrefix == "" && jumpPrefixWidth > 0 {
			jumpPrefix = strings.Repeat(" ", jumpPrefixWidth)
		}

		edge := edges[i]
		title := m.truncate(edge.TargetTitle, titleWidth)
		inventors := m.truncate(formatInventorsShort(edge.TargetInventors), invWidth)
		expDate := edge.TargetExpirationDate
		if expDate == "" {
			expDate = "-"
		}
		numCell := edge.TargetPatent

		row := m.pad(prefix, 2) +
			m.pad(jumpPrefix, jumpPrefixWidth) +
			m.pad(rowIndexLabel(i), indexWidth)

		row += m.pad(m.truncate(numCell, cols[0].width), cols[0].width+2)
		row += m.pad(m.truncate(title, cols[1].width), cols[1].width+2)
		row += m.pad(m.truncate(inventors, cols[2].width), cols[2].width+2)
		row += m.pad(m.truncate(expDate, cols[3].width), cols[3].width+2)
		row += m.pad(m.citationStatusLabel(edge.Status), cols[4].width)

		body.WriteString(m.styleRowOverlay(i, selected, row, m.overlayWidth()-4) + "\n")
	}

	title := "Citations"
	if relation == domain.RelationCitedBy {
		title = "Cited By"
	}
	return m.renderPopup(title+" · "+m.current.Number, body.String())
}

func (m *Model) viewReviewQueue() string {
	edges, err := m.currentReviewCitationEdges()
	if err != nil {
		return m.renderPopup("Review Queue", err.Error()+"\n")
	}
	if len(edges) == 0 {
		return m.renderPopup("Review Queue", m.text.T(TextReviewQueueEmpty)+"\n")
	}
	selected := clamp(m.reviewSelected, 0, len(edges)-1)
	m.reviewSelected = selected
	window := pageWindow(selected, len(edges), m.overlayPageSize())
	var body strings.Builder
	body.WriteString(m.citationStatusLabel(m.reviewStatus) + "\n")
	body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Render(pageStatus(m.text.T(TextValuePageStatus), window)))
	body.WriteString("\n\n")

	indexWidth := 4
	numWidth := 16
	titleWidth := max(20, m.overlayWidth()-64)
	invWidth := 20
	expWidth := 12
	sourceWidth := 16

	// Account for jump prefix width in header if jump targets exist
	jumpPrefixWidth := 0
	if m.hasJumpTargets() {
		jumpPrefixWidth = 2
	}

	cols := []listColumn{
		{label: "Number", width: numWidth, id: "number"},
		{label: "Title", width: titleWidth, id: "title"},
		{label: "Inventor", width: invWidth, id: "inventor"},
		{label: "Expires", width: expWidth, id: "expires"},
		{label: "Source", width: sourceWidth, id: "source"},
	}
	cols = fitColumns(cols, m.overlayWidth()-4-(2+jumpPrefixWidth+indexWidth), map[string]int{
		"number":   12,
		"title":    18,
		"inventor": 10,
		"expires":  10,
		"source":   12,
	}, []string{"title", "inventor", "source", "number", "expires"})

	header := m.pad("  ", 2) +
		m.pad("", jumpPrefixWidth) +
		m.pad("#", indexWidth)

	for i, c := range cols {
		padding := 2
		if i == len(cols)-1 {
			padding = 0
		}
		header += m.pad(c.label, c.width+padding)
	}

	body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Underline(true).Render(header))
	body.WriteString("\n")

	for i := window.Start; i < window.End; i++ {
		prefix := "  "
		if i == selected {
			prefix = "> "
		}

		jumpPrefix := m.jumpPrefix(i - window.Start)
		if jumpPrefix == "" && jumpPrefixWidth > 0 {
			jumpPrefix = strings.Repeat(" ", jumpPrefixWidth)
		}

		edge := edges[i]
		title := m.truncate(edge.TargetTitle, titleWidth)
		inventors := m.truncate(formatInventorsShort(edge.TargetInventors), invWidth)
		expDate := edge.TargetExpirationDate
		if expDate == "" {
			expDate = "-"
		}

		row := m.pad(prefix, 2) +
			m.pad(jumpPrefix, jumpPrefixWidth) +
			m.pad(rowIndexLabel(i), indexWidth)

		row += m.pad(m.truncate(edge.TargetPatent, cols[0].width), cols[0].width+2)
		row += m.pad(m.truncate(title, cols[1].width), cols[1].width+2)
		row += m.pad(m.truncate(inventors, cols[2].width), cols[2].width+2)
		row += m.pad(m.truncate(expDate, cols[3].width), cols[3].width+2)
		row += m.pad(m.truncate(edge.SourcePatent, cols[4].width), cols[4].width)

		body.WriteString(m.styleRowOverlay(i, selected, row, m.overlayWidth()-4) + "\n")
	}
	return m.renderPopup("Review Queue", body.String())
}

func (m *Model) citationStatusLabel(status string) string {
	label := ""
	switch status {
	case domain.CitationStatusStored:
		label = m.text.T(TextCitationStored)
	case domain.CitationStatusIgnored:
		label = m.text.T(TextCitationIgnored)
	case domain.CitationStatusCached:
		label = m.text.T(TextCitationCached)
	default:
		label = m.text.T(TextCitationUnderReview)
	}

	if color, ok := StatusColors[status]; ok {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(label)
	}
	return label
}

func (m *Model) moveSelection(delta int) *Model {
	count := m.activeItemCount()
	if count == 0 {
		return m
	}
	current := m.activeSelectionIndex()
	next := clamp(current+delta, 0, count-1)
	m.setActiveSelectionIndex(next)
	return m
}

func (m *Model) isPopupMode() bool {
	return m.mode == viewClassifications ||
		m.mode == viewInventors ||
		m.mode == viewProjectEvents ||
		m.mode == viewProjectInvoices ||
		m.mode == viewProjectIDS ||
		m.mode == viewClassificationDetail ||
		m.mode == viewHelpPopup ||
		m.mode == viewBulkConfirm ||
		m.mode == viewConfirmDelete ||
		m.mode == viewPreview
}

func (m *Model) isPopupSearchMode() bool {
	return m.mode == viewClassifications ||
		m.mode == viewInventors ||
		m.mode == viewProjectEvents ||
		m.mode == viewProjectInvoices ||
		m.mode == viewProjectIDS ||
		m.mode == viewFamily
}

func (m *Model) citationOpenHint() string {
	return fmt.Sprintf(m.text.T(TextValueOpenHint), keyEnter, keyYes, keyIgnore, keyUnreview, keyRefreshAll, keyCtrlF, keyCtrlD)
}

func (m *Model) reviewOpenHint() string {
	return fmt.Sprintf(m.text.T(TextValueReviewOpenHint), keyEnter, keyYes, keyIgnore, keyUnreview, keyRefreshAll, keyWeb, keyCtrlF, keyCtrlD)
}

func (m *Model) classificationOpenHint() string {
	return fmt.Sprintf(m.text.T(TextValueClassificationHint), keyEnter, keySearch, keyNotes, keyCtrlF, keyCtrlD)
}

func (m *Model) previewStorePrompt() string {
	return fmt.Sprintf(m.text.T(TextPreviewStorePrompt), keyYes, keyIgnore, keyUnreview, keyNo, keyEsc)
}

func formatCitationTime(value time.Time, fallback string) string {
	if value.IsZero() {
		return fallback
	}
	return value.Local().Format("2006-01-02 15:04")
}

const storedTimeThreshold = 48 * time.Hour

func formatStoredTime(value time.Time, fallback string) string {
	if value.IsZero() {
		return fallback
	}
	local := value.Local()
	if time.Since(value) < storedTimeThreshold {
		return local.Format("2006-01-02 15:04")
	}
	return local.Format("2006-01-02")
}

func (m *Model) viewPreview() string {
	base := overlayBase()

	p := m.pendingBundle.Patent
	if p.Number == "" {
		return base.Render(m.text.T(TextValueUnknown)) + "\n"
	}
	var b strings.Builder
	b.WriteString(m.renderPopupHeader(m.text.T(TextPreviewTitle)))

	b.WriteString(base.Bold(true).Render(p.Number) + "\n")
	b.WriteString(base.Render(p.Title) + "\n\n")

	// Calculate max label width for alignment
	previewLabels := []TextKey{TextDetailAssignee, TextDetailPublication, TextDetailGrant, TextDetailExpiration}
	if len(p.Inventors) == 0 {
		previewLabels = append(previewLabels, TextDetailInventors)
	} else {
		previewLabels = append(previewLabels, TextDetailInventor)
	}
	maxW := 0
	for _, l := range previewLabels {
		w := lipgloss.Width(m.text.T(l) + ":")
		if w > maxW {
			maxW = w
		}
	}

	b.WriteString(m.detailRow(TextDetailAssignee, p.Assignee, maxW) + "\n")
	if len(p.Inventors) == 0 {
		b.WriteString(m.detailRow(TextDetailInventors, "", maxW) + "\n")
	} else {
		for i, inventor := range p.Inventors {
			b.WriteString(m.detailRow(TextDetailInventor, fmt.Sprintf("%d. %s", i+1, inventor), maxW) + "\n")
		}
	}
	b.WriteString(m.detailRow(TextDetailPublication, p.PublicationDate, maxW) + "\n")
	b.WriteString(m.detailRow(TextDetailGrant, p.GrantDate, maxW) + "\n")
	b.WriteString(m.detailRow(TextDetailExpiration, m.formatExpiration(p), maxW) + "\n")
	b.WriteString("\n")
	if strings.TrimSpace(p.Abstract) == "" {
		b.WriteString(base.Render(m.text.T(TextPreviewNoAbstract)) + "\n")
	} else {
		b.WriteString(base.Render(p.Abstract) + "\n")
	}
	return b.String()
}

// overlayBase returns a base lipgloss style with ColorSurface background
// used by all overlay popup view functions for consistent text rendering.
func overlayBase() lipgloss.Style {
	return lipgloss.NewStyle().Background(lipgloss.Color(ColorSurface))
}

func (m *Model) overlayBackdrop() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	idx := len(m.backStack) - 1
	for idx >= 0 {
		if !mustModeSpec(m.backStack[idx].mode).isOverlay {
			break
		}
		idx--
	}
	if idx < 0 {
		return lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, "")
	}

	backdrop := *m
	backdrop.restore(m.backStack[idx])
	backdrop.width = m.width
	backdrop.height = m.height
	backdrop.backStack = append([]navSnapshot(nil), m.backStack[:idx]...)
	backdrop.jumpLabelsCache = nil

	plain := ansi.Strip(backdrop.renderView())
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDim))
	lines := strings.Split(lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, plain), "\n")
	for i := range lines {
		lines[i] = dim.Render(lines[i])
	}
	return strings.Join(lines, "\n")
}

func overlayLine(base, overlay string, x, totalWidth int) string {
	left := ansi.Cut(base, 0, x)
	rightStart := x + lipgloss.Width(overlay)
	right := ""
	if rightStart < totalWidth {
		right = ansi.Cut(base, rightStart, totalWidth)
	}
	return left + overlay + right
}

func (m *Model) previewOverlay(content string) string {
	popup := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(ColorSubtle)).
		Padding(1, 2).
		Width(m.overlayWidth()).
		Render(content)

	if m.width <= 0 || m.height <= 0 {
		return popup
	}

	backdrop := m.overlayBackdrop()
	popupWidth := lipgloss.Width(popup)
	popupHeight := lipgloss.Height(popup)
	x := max(0, (m.width-popupWidth)/2)
	y := max(0, (m.height-popupHeight)/2)

	baseLines := strings.Split(lipgloss.Place(m.width, m.height, lipgloss.Left, lipgloss.Top, backdrop), "\n")
	popupLines := strings.Split(popup, "\n")
	scrimLine := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDim)).Render(strings.Repeat(" ", m.width))
	for i := 0; i < popupHeight && y+i < len(baseLines) && i < len(popupLines); i++ {
		baseLines[y+i] = overlayLine(scrimLine, popupLines[i], x, m.width)
	}
	return strings.Join(baseLines, "\n")
}

func (m *Model) overlayWidth() int {
	if m.width <= 0 {
		return OverlayFallbackWidth
	}

	// Determine the base width to scale from.
	// If the previous view was an overlay, we scale relative to its width.
	// Otherwise, we scale relative to the full terminal width.
	baseWidth := m.width
	if len(m.backStack) > 0 {
		last := m.backStack[len(m.backStack)-1]
		if last.width > 0 {
			baseWidth = last.width
		}
	}

	width := int(float64(baseWidth) * OverlayExpandedRatio)
	width = max(width, OverlayMinWidth)

	// Ensure we don't exceed physical terminal width - margin
	if width > m.width-4 {
		width = max(OverlayAbsoluteMinWidth, m.width-4)
	}
	return width
}

func (m *Model) viewConfirmDelete() string {
	if m.selected < 0 || m.selected >= len(m.patents) {
		return ""
	}
	p := m.patents[m.selected]
	return m.renderPopupHeader(fmt.Sprintf(m.text.T(TextDeleteConfirmPrompt), p.Number))
}

func (m *Model) patentDateCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) < 2 {
		m.err = "usage: :date app|pub|grant <YYYY-MM-DD>"
		return m, nil
	}
	dateType := strings.ToLower(args[0])
	value := strings.Join(args[1:], " ")
	// Normalize 2019/05/19 to 2019-05-19
	value = strings.ReplaceAll(value, "/", "-")

	if err := m.repo.UpdatePatentDate(m.ctx, m.current.Number, dateType, value); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.logActivity(ActivityPatentDate, m.current.Number, fmt.Sprintf("%s: %s", dateType, value))
	m.message = fmt.Sprintf("updated %s date: %s", dateType, value)

	// Refresh current patent from DB
	if p, err := m.repo.GetPatent(m.ctx, m.ProjectID, m.current.Number); err == nil {
		m.current = p
		m.populateDetailCache()
	}

	return m, nil
}

func (m *Model) patentNumberCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) < 2 {
		m.err = "usage: :num app|pub|grant <number>"
		return m, nil
	}
	numType := strings.ToLower(args[0])
	value := strings.Join(args[1:], " ")

	if err := m.repo.UpdatePatentNumber(m.ctx, m.current.Number, numType, value); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.logActivity(ActivityPatentNumber, m.current.Number, fmt.Sprintf("%s: %s", numType, value))
	m.message = fmt.Sprintf("updated %s number: %s", numType, value)

	// Refresh current patent from DB
	if p, err := m.repo.GetPatent(m.ctx, m.ProjectID, m.current.Number); err == nil {
		m.current = p
		m.populateDetailCache()
	}

	return m, nil
}

func (m *Model) deleteSelectedPatent() (tea.Model, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.patents) {
		m.mode = viewList
		return m, nil
	}
	p := m.patents[m.selected]
	if err := m.repo.DeletePatent(m.ctx, m.ProjectID, p.Number); err != nil {
		m.err = err.Error()
		m.mode = viewList
		return m, nil
	}

	// Delete PDF if it exists
	pdfPath := filepath.Join(defaultPDFDir, p.Number+".pdf")
	if _, err := os.Stat(pdfPath); err == nil {
		if err := os.Remove(pdfPath); err != nil {
			m.logger.Error("failed to delete pdf", "path", pdfPath, "error", err)
		} else {
			m.logger.Info("deleted pdf", "path", pdfPath)
		}
	}

	m.logActivity(ActivityPatentDelete, p.Number, "")
	m.message = fmt.Sprintf(m.text.T(TextMessageDeletedPatent), p.Number)
	m.mode = viewList
	return m.refreshList()
}

func (m *Model) applyStatusSelection() (tea.Model, tea.Cmd) {
	statuses := m.selectableStatuses()
	if m.statusSelected < 0 || m.statusSelected >= len(statuses) {
		return m.goBack()
	}
	next := statuses[m.statusSelected]

	// Determine which patents to update
	indices := m.selectedIndices()
	if len(indices) == 0 && m.mode == viewDetail {
		// If in detail view, update current patent
		if err := m.repo.UpdatePatentStatus(m.ctx, m.ProjectID, m.current.Number, next); err != nil {
			m.err = err.Error()
			return m.goBack()
		}
		m.current.Status = next
		for i, p := range m.patents {
			if p.Number == m.current.Number {
				m.patents[i].Status = next
				break
			}
		}
		m.message = fmt.Sprintf("%s → %s", m.current.Number, next)
		m.logActivity(ActivityPatentStatus, m.current.Number, next)
		return m.goBack()
	}

	if len(indices) == 0 && m.mode == viewList {
		indices = []int{m.selected}
	}

	updatedCount := 0
	for _, idx := range indices {
		if idx < 0 || idx >= len(m.patents) {
			continue
		}
		p := m.patents[idx]
		if err := m.repo.UpdatePatentStatus(m.ctx, m.ProjectID, p.Number, next); err != nil {
			m.logger.Error("status selection update failed", "patent", p.Number, "error", err)
			continue
		}
		m.patents[idx].Status = next
		m.logActivity(ActivityPatentStatus, p.Number, next)
		updatedCount++
	}

	if updatedCount > 1 {
		m.message = fmt.Sprintf("updated status to %s for %d patents", next, updatedCount)
	} else if updatedCount == 1 {
		p := m.patents[indices[0]]
		m.message = fmt.Sprintf("%s → %s", p.Number, p.Status)
	}

	m.visualMode = false
	return m.goBack()
}

func (m *Model) hasJumpTargets() bool {
	return len(m.jumpLabelsCache) > 0
}

func (m *Model) jumpTargetCount() int {
	return len(m.jumpLabelsCache)
}

type jumpLabel struct {
	key       string
	preferred bool
}

func (m *Model) jumpLabels() []jumpLabel {
	switch {
	case m.mode == viewList:
		window := pageWindow(m.selected, len(m.patents), m.pageSize())
		cols := m.listColumns()
		// column headers + rows in window
		labels := make([]string, 0, len(cols)+(window.End-window.Start))
		for _, c := range cols {
			labels = append(labels, c.jumpLabel)
		}
		return m.fallbackJumpLabels(len(cols)+window.End-window.Start, labels)
	case m.mode == viewDetail:
		fields := m.detailFields()
		labels := make([]jumpLabel, 0, len(fields))
		for _, field := range fields {
			labels = append(labels, jumpLabel{key: field.jumpLabel, preferred: true})
		}
		return labels
	case m.isCitationView():
		edges, err := m.currentCitationEdges()
		if err != nil || len(edges) == 0 {
			return nil
		}
		start := pageStart(clamp(m.citationSelection(), 0, len(edges)-1), m.pageSize())
		end := min(start+m.pageSize(), len(edges))
		preferred := jumpLabelCitation
		if m.mode == viewCitedBy {
			preferred = jumpLabelCitedBy
		}
		return m.fallbackJumpLabels(end-start, []string{preferred})
	case m.mode == viewReview:
		edges, err := m.currentReviewCitationEdges()
		if err != nil || len(edges) == 0 {
			return nil
		}
		start := pageStart(clamp(m.reviewSelected, 0, len(edges)-1), m.pageSize())
		end := min(start+m.pageSize(), len(edges))
		return m.fallbackJumpLabels(end-start, nil)
	case m.mode == viewFamily:
		nodes := m.buildFamilyTree()
		return m.fallbackJumpLabels(len(nodes), nil)
	default:
		return nil
	}
}

func (m *Model) jumpPrefix(index int) string {
	labels := m.jumpLabelsCache
	if !m.jumpMode || index < 0 || index >= len(labels) {
		return ""
	}
	label := labels[index]
	if label.key == "" {
		return ""
	}
	color := ColorYellow
	if !label.preferred {
		color = ColorThemeDetail // Cyan/Light Blue for fallbacks
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(label.key) + " "
}

func (m *Model) applyJump(key string) (tea.Model, tea.Cmd) {
	if len(key) != 1 {
		return m, nil
	}
	index := m.indexJumpLabel(key)
	if index < 0 {
		return m, nil
	}
	m.jumpMode = false

	switch {
	case m.mode == viewList:
		cols := m.listColumns()
		colCount := len(cols)
		if index < colCount {
			m.sortColumnIndex = index
			return m, nil
		}
		// Adjust index for row jump (skip header labels)
		index -= colCount
		window := pageWindow(m.selected, len(m.patents), m.pageSize())

		target := window.Start + index
		if target < len(m.patents) {
			m.selected = target
			return m.openPatent(m.patents[target].Number)
		}
	case m.mode == viewDetail:
		m.detailSelected = index
	case m.isCitationView():
		edges, err := m.currentCitationEdges()
		if err != nil || len(edges) == 0 {
			return m, nil
		}
		start := pageStart(clamp(m.citationSelection(), 0, len(edges)-1), m.pageSize())
		m.setCitationSelection(clamp(start+index, 0, len(edges)-1))
	case m.mode == viewReview:
		edges, err := m.currentReviewCitationEdges()
		if err != nil || len(edges) == 0 {
			return m, nil
		}
		start := pageStart(clamp(m.reviewSelected, 0, len(edges)-1), m.pageSize())
		m.reviewSelected = clamp(start+index, 0, len(edges)-1)
	case m.mode == viewFamily:
		nodes := m.buildFamilyTree()
		m.familySelected = clamp(index, 0, len(nodes)-1)
	}
	return m, nil
}

func (m *Model) indexJumpLabel(target string) int {
	for i, label := range m.jumpLabelsCache {
		if label.key == target {
			return i
		}
	}
	return -1
}

func (m *Model) fallbackJumpLabels(count int, preferred []string) []jumpLabel {
	if count <= 0 {
		return nil
	}
	labels := make([]jumpLabel, 0, count)
	used := map[string]bool{}

	// First pass: add preferred labels
	for _, label := range preferred {
		if label == "" {
			labels = append(labels, jumpLabel{}) // placeholder
			continue
		}
		if used[label] {
			labels = append(labels, jumpLabel{}) // conflict, will fallback
			continue
		}
		labels = append(labels, jumpLabel{key: label, preferred: true})
		used[label] = true
	}

	// Second pass: fill in gaps with fallbacks
	poolIdx := 0
	for i := 0; i < count; i++ {
		if i < len(labels) && labels[i].key != "" {
			continue
		}

		// Find next available character from fallback pool
		for poolIdx < len(jumpFallbackLabels) {
			char := string(jumpFallbackLabels[poolIdx])
			poolIdx++
			if !used[char] {
				if i < len(labels) {
					labels[i] = jumpLabel{key: char, preferred: false}
				} else {
					labels = append(labels, jumpLabel{key: char, preferred: false})
				}
				used[char] = true
				break
			}
		}
	}

	// Limit to requested count (should already be correctly sized)
	if len(labels) > count {
		return labels[:count]
	}
	return labels
}

func indexString(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}

func (m *Model) isInSelection(idx int) bool {
	if !m.visualMode {
		return false
	}
	current := m.activeSelectionIndex()
	start, end := m.selectionStart, current
	if start > end {
		start, end = end, start
	}
	return idx >= start && idx <= end
}

func (m *Model) activeSelectionIndex() int {
	switch {
	case m.isCitationView():
		return m.citationSelection()
	case m.mode == viewReview:
		return m.reviewSelected
	case m.mode == viewList:
		return m.selected
	case m.mode == viewClassifications:
		return m.classificationSelected
	case m.mode == viewInventors:
		return m.inventorSelected
	case m.mode == viewDetail:
		return m.detailSelected
	default:
		return 0
	}
}

func (m *Model) selectedIndices() []int {
	current := m.activeSelectionIndex()
	if !m.visualMode {
		return []int{current}
	}
	start, end := m.selectionStart, current
	if start > end {
		start, end = end, start
	}
	var res []int
	for i := start; i <= end; i++ {
		res = append(res, i)
	}
	return res
}

func (m *Model) setActiveSelectionIndex(val int) {
	count := m.activeItemCount()
	if count == 0 {
		return
	}
	val = clamp(val, 0, count-1)
	switch {
	case m.isCitationView():
		m.setCitationSelection(val)
	case m.mode == viewReview:
		m.reviewSelected = val
	case m.mode == viewList:
		m.selected = val
	case m.mode == viewClassifications:
		m.classificationSelected = val
	case m.mode == viewInventors:
		m.inventorSelected = val
	case m.mode == viewFamily:
		m.familySelected = val
	case m.mode == viewDetail:
		m.detailSelected = val
	}
}

func (m *Model) activeItemCount() int {
	switch {
	case m.isCitationView():
		edges, _ := m.currentCitationEdges()
		return len(edges)
	case m.mode == viewReview:
		edges, _ := m.currentReviewCitationEdges()
		return len(edges)
	case m.mode == viewList:
		return len(m.patents)
	case m.mode == viewClassifications:
		cls, _ := m.repo.ListClassifications(m.ctx, m.ProjectID, m.current.Number)
		return len(cls)
	case m.mode == viewInventors:
		return len(m.current.Inventors)
	case m.mode == viewFamily:
		return len(m.buildFamilyTree())
	case m.mode == viewDetail:
		return len(m.detailFields())
	default:
		return 0
	}
}

func (m *Model) pageSize() int {
	if m.height <= 0 {
		return 20
	}
	// Full screen views use about 10-12 lines for layout (header, nav, status, rule, message)
	return max(5, m.height-14)
}

func (m *Model) overlayPageSize() int {
	if m.height <= 0 {
		return 15
	}
	// Overlays use more internal layout lines and we want a margin to see background
	return max(3, m.height-18)
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func rowIndexLabel(zeroBasedIndex int) string {
	return fmt.Sprintf("%3d", zeroBasedIndex+1)
}

func isCountKey(key string) bool {
	return len(key) == 1 && key[0] >= '0' && key[0] <= '9'
}

func (m *Model) consumeCount(defaultValue int) int {
	if m.countBuffer == "" {
		return defaultValue
	}
	count, err := strconv.Atoi(m.countBuffer)
	m.countBuffer = EmptyCount
	if err != nil || count <= 0 {
		return defaultValue
	}
	return count
}

func (m *Model) goToRow(index int) *Model {
	if index <= 0 {
		index = 1
	}
	target := index - 1
	m.setActiveSelectionIndex(target)
	return m
}

func (m *Model) viewClassifications() string {
	if m.current.Number == "" {
		return overlayBase().Render("No patent open. Open a patent first.") + "\n"
	}
	classifications, err := m.repo.ListClassifications(m.ctx, m.ProjectID, m.current.Number)
	if err != nil {
		return overlayBase().Render(err.Error()) + "\n"
	}
	if len(classifications) == 0 {
		return overlayBase().Render("No CPC/USPC classification codes stored for "+m.current.Number+".\n"+
			"Re-import the patent or run :"+commandRefreshRefsDetails+" to fetch row details.") + "\n"
	}

	selected := clamp(m.classificationSelected, 0, len(classifications)-1)
	m.classificationSelected = selected
	window := pageWindow(selected, len(classifications), m.overlayPageSize())

	var body strings.Builder
	body.WriteString(overlayBase().Foreground(lipgloss.Color(ColorSubtle)).Render(pageStatus(m.text.T(TextValuePageStatus), window)))
	body.WriteString("\n\n")

	rowWidth := max(44, m.overlayWidth()-6)
	indexWidth := 4
	codeWidth := 18
	descriptionWidth := max(20, rowWidth-indexWidth-codeWidth-2)

	header := m.pad("  ", 2) +
		m.pad("#", indexWidth) +
		m.pad("Code", codeWidth) +
		m.pad("Description", descriptionWidth)

	body.WriteString(overlayBase().Foreground(lipgloss.Color(ColorSubtle)).Underline(true).Render(header))
	body.WriteString("\n")

	for i := window.Start; i < window.End; i++ {
		cls := classifications[i]
		prefix := "  "
		if i == selected {
			prefix = "> "
		}
		row := m.pad(prefix, 2) +
			m.pad(rowIndexLabel(i), indexWidth) +
			m.pad(cls.Code, codeWidth) +
			m.pad(m.truncate(cls.Description, descriptionWidth), descriptionWidth)
		body.WriteString(m.styleRowOverlay(i, selected, row, rowWidth) + "\n")
	}
	return m.renderPopup("Classifications · "+m.current.Number, body.String())
}

func (m *Model) viewClassificationDetail() string {
	base := overlayBase()
	boldStyle := base.Bold(true)

	classifications, err := m.repo.ListClassifications(m.ctx, m.ProjectID, m.current.Number)
	if err != nil {
		return base.Render("Error loading classifications: "+err.Error()) + "\n"
	}
	if len(classifications) == 0 {
		return base.Render("No classification data stored for "+m.current.Number+".\nRun :refresh-refs-details or re-import the patent.") + "\n"
	}
	selected := clamp(m.classificationSelected, 0, len(classifications)-1)
	cls := classifications[selected]

	// count patents in project with this classification code (prefix match)
	projectPatents, _ := m.repo.ListPatents(m.ctx, m.ProjectID, storage.ListPatentsOptions{
		ClassFilters:  []string{cls.Code},
		ClassFilterOp: domain.FilterOpAnd,
	})
	count := len(projectPatents)

	var body strings.Builder
	body.WriteString(base.Render(fmt.Sprintf("System: %s", cls.System)) + "\n")
	body.WriteString(base.Render(fmt.Sprintf("Code:   %s", cls.Code)) + "\n\n")

	if cls.System == "CPC" {
		body.WriteString(boldStyle.Render("Hierarchy:") + "\n")
		body.WriteString(base.Render(fmt.Sprintf("  Section:  %s", cls.Section)) + "\n")
		body.WriteString(base.Render(fmt.Sprintf("  Class:    %s", cls.Class)) + "\n")
		body.WriteString(base.Render(fmt.Sprintf("  Subclass: %s", cls.Subclass)) + "\n")
		body.WriteString(base.Render(fmt.Sprintf("  Group:    %s", cls.MainGroup)) + "\n")
		body.WriteString(base.Render(fmt.Sprintf("  Subgroup: %s", cls.Subgroup)) + "\n\n")
	} else if cls.System == "USPC" {
		body.WriteString(boldStyle.Render("Hierarchy:") + "\n")
		body.WriteString(base.Render(fmt.Sprintf("  Class:    %s", cls.Class)) + "\n")
		body.WriteString(base.Render(fmt.Sprintf("  Subclass: %s", cls.Subclass)) + "\n\n")
	}

	body.WriteString(boldStyle.Render("Description:") + "\n")
	body.WriteString(base.Render(cls.Description) + "\n\n")
	body.WriteString(base.Render(fmt.Sprintf("Patents: %d patent(s)", count)) + "\n")
	return m.renderPopup(fmt.Sprintf("Classification · %s", cls.Code), body.String())
}

func (m *Model) listSearchNext() *Model {
	return m.listSearchFrom(true)
}

func (m *Model) listSearchFirst() *Model {
	return m.listSearchFrom(false)
}

func (m *Model) listSearchFrom(next bool) *Model {
	if m.listSearchQuery == "" {
		return m
	}
	if len(m.patents) == 0 {
		return m
	}

	query := m.listSearchQuery
	ignoreCase := strings.ToLower(query) == query

	start := 0
	if next {
		start = m.selected + 1
	}
	for i := 0; i < len(m.patents); i++ {
		idx := (start + i) % len(m.patents)
		p := m.patents[idx]

		match := containsMatch(p.Number, query, ignoreCase) ||
			containsMatch(p.Title, query, ignoreCase) ||
			containsMatch(p.Abstract, query, ignoreCase) ||
			containsMatch(p.Assignee, query, ignoreCase) ||
			containsMatch(p.ClassificationLabel, query, ignoreCase)

		if !match {
			for _, inv := range p.Inventors {
				if containsMatch(inv, query, ignoreCase) {
					match = true
					break
				}
			}
		}

		if match {
			m.selected = idx
			m.message = fmt.Sprintf("found match: %s", p.Number)
			return m
		}
	}
	m.message = "no matches found for: " + m.listSearchQuery
	return m
}

type popupSearchable interface {
	ItemCount() int
	Match(idx int, query string, ignoreCase bool) bool
	SetSelected(idx int)
	GetSelected() int
	MatchLabel(idx int) string
}

type classificationSearchable struct{ m *Model }

func (s classificationSearchable) ItemCount() int {
	cls, _ := s.m.repo.ListClassifications(s.m.ctx, s.m.ProjectID, s.m.current.Number)
	return len(cls)
}
func (s classificationSearchable) Match(idx int, query string, ignoreCase bool) bool {
	cls, _ := s.m.repo.ListClassifications(s.m.ctx, s.m.ProjectID, s.m.current.Number)
	if idx < 0 || idx >= len(cls) {
		return false
	}
	c := cls[idx]
	return containsMatch(c.Code, query, ignoreCase) || containsMatch(c.Description, query, ignoreCase)
}
func (s classificationSearchable) SetSelected(idx int) { s.m.classificationSelected = idx }
func (s classificationSearchable) GetSelected() int    { return s.m.classificationSelected }
func (s classificationSearchable) MatchLabel(idx int) string {
	cls, _ := s.m.repo.ListClassifications(s.m.ctx, s.m.ProjectID, s.m.current.Number)
	return cls[idx].Code
}

type inventorSearchable struct{ m *Model }

func (s inventorSearchable) ItemCount() int { return len(s.m.current.Inventors) }
func (s inventorSearchable) Match(idx int, query string, ignoreCase bool) bool {
	return containsMatch(s.m.current.Inventors[idx], query, ignoreCase)
}
func (s inventorSearchable) SetSelected(idx int)       { s.m.inventorSelected = idx }
func (s inventorSearchable) GetSelected() int          { return s.m.inventorSelected }
func (s inventorSearchable) MatchLabel(idx int) string { return s.m.current.Inventors[idx] }

type eventSearchable struct{ m *Model }

func (s eventSearchable) ItemCount() int {
	events, _ := s.m.repo.ListProjectEvents(s.m.ctx, s.m.ProjectID)
	return len(events)
}
func (s eventSearchable) Match(idx int, query string, ignoreCase bool) bool {
	events, _ := s.m.repo.ListProjectEvents(s.m.ctx, s.m.ProjectID)
	e := events[idx]
	return containsMatch(e.EventType, query, ignoreCase) || containsMatch(e.Notes, query, ignoreCase) || containsMatch(e.Reference, query, ignoreCase)
}
func (s eventSearchable) SetSelected(idx int)       { s.m.projectEventsSelected = idx }
func (s eventSearchable) GetSelected() int          { return s.m.projectEventsSelected }
func (s eventSearchable) MatchLabel(idx int) string { return "" }

type invoiceSearchable struct{ m *Model }

func (s invoiceSearchable) ItemCount() int {
	invoices, _ := s.m.repo.ListProjectInvoices(s.m.ctx, s.m.ProjectID)
	return len(invoices)
}
func (s invoiceSearchable) Match(idx int, query string, ignoreCase bool) bool {
	invoices, _ := s.m.repo.ListProjectInvoices(s.m.ctx, s.m.ProjectID)
	inv := invoices[idx]
	return containsMatch(inv.FirmName, query, ignoreCase) || containsMatch(inv.Description, query, ignoreCase) || containsMatch(inv.InvoiceNumber, query, ignoreCase)
}
func (s invoiceSearchable) SetSelected(idx int)       { s.m.projectInvoicesSelected = idx }
func (s invoiceSearchable) GetSelected() int          { return s.m.projectInvoicesSelected }
func (s invoiceSearchable) MatchLabel(idx int) string { return "" }

type idsSearchable struct{ m *Model }

func (s idsSearchable) ItemCount() int {
	entries, _ := s.m.repo.ListIDSEntries(s.m.ctx, s.m.ProjectID)
	return len(entries)
}
func (s idsSearchable) Match(idx int, query string, ignoreCase bool) bool {
	entries, _ := s.m.repo.ListIDSEntries(s.m.ctx, s.m.ProjectID)
	e := entries[idx]
	return containsMatch(e.PatentNumber, query, ignoreCase) || containsMatch(e.Notes, query, ignoreCase)
}
func (s idsSearchable) SetSelected(idx int)       { s.m.projectIDSSelected = idx }
func (s idsSearchable) GetSelected() int          { return s.m.projectIDSSelected }
func (s idsSearchable) MatchLabel(idx int) string { return "" }

type familySearchable struct{ m *Model }

func (s familySearchable) ItemCount() int {
	nodes := s.m.buildFamilyTree()
	return len(nodes)
}
func (s familySearchable) Match(idx int, query string, ignoreCase bool) bool {
	nodes := s.m.buildFamilyTree()
	if idx < 0 || idx >= len(nodes) {
		return false
	}
	node := nodes[idx]
	return containsMatch(node.number, query, ignoreCase) || containsMatch(node.relType, query, ignoreCase) || containsMatch(node.grantYear, query, ignoreCase)
}
func (s familySearchable) SetSelected(idx int) { s.m.familySelected = idx }
func (s familySearchable) GetSelected() int    { return s.m.familySelected }
func (s familySearchable) MatchLabel(idx int) string {
	nodes := s.m.buildFamilyTree()
	if idx < 0 || idx >= len(nodes) {
		return ""
	}
	return nodes[idx].number
}

func (m *Model) popupSearchNext() *Model {
	return m.popupSearchFrom(true)
}

func (m *Model) popupSearchFirst() *Model {
	return m.popupSearchFrom(false)
}

func (m *Model) popupSearchFrom(next bool) *Model {
	if m.popupSearchQuery == "" {
		return m
	}

	var s popupSearchable
	switch m.mode {
	case viewClassifications:
		s = classificationSearchable{m}
	case viewInventors:
		s = inventorSearchable{m}
	case viewProjectEvents:
		s = eventSearchable{m}
	case viewProjectInvoices:
		s = invoiceSearchable{m}
	case viewProjectIDS:
		s = idsSearchable{m}
	case viewFamily:
		s = familySearchable{m}
	default:
		return m
	}

	count := s.ItemCount()
	if count == 0 {
		return m
	}

	query := m.popupSearchQuery
	ignoreCase := strings.ToLower(query) == query
	start := 0
	if next {
		start = s.GetSelected() + 1
	}

	for i := 0; i < count; i++ {
		idx := (start + i) % count
		if s.Match(idx, query, ignoreCase) {
			s.SetSelected(idx)
			label := s.MatchLabel(idx)
			if label != "" {
				m.message = fmt.Sprintf("found match: %s", label)
			} else {
				m.message = "found match"
			}
			return m
		}
	}

	m.message = "no matches found for: " + query
	return m
}

func containsMatch(s, query string, ignoreCase bool) bool {
	if ignoreCase {
		return strings.Contains(strings.ToLower(s), strings.ToLower(query))
	}
	return strings.Contains(s, query)
}

func (m *Model) goToClassification(index int) *Model {
	classifications, _ := m.repo.ListClassifications(m.ctx, m.ProjectID, m.current.Number)
	if len(classifications) == 0 {
		return m
	}
	m.classificationSelected = clamp(index-1, 0, len(classifications)-1)
	return m
}

func (m *Model) viewInventors() string {
	inventors := m.current.Inventors
	if len(inventors) == 0 {
		return overlayBase().Render("No inventors listed.") + "\n"
	}

	selected := clamp(m.inventorSelected, 0, len(inventors)-1)
	m.inventorSelected = selected

	// count patents per inventor across all project patents (no filter)
	allPatents, _ := m.repo.ListPatents(m.ctx, m.ProjectID, storage.ListPatentsOptions{})
	countFor := func(name string) int {
		n := 0
		for _, p := range allPatents {
			for _, inv := range p.Inventors {
				if strings.EqualFold(inv, name) {
					n++
					break
				}
			}
		}
		return n
	}

	rowWidth := max(40, m.overlayWidth()-6)
	indexWidth := 4
	countWidth := 6 // "(NNN)"

	var b strings.Builder
	b.WriteString(m.renderPopupHeader("Inventors · " + m.current.Number))
	for i, inventor := range inventors {
		prefix := "  "
		if i == selected {
			prefix = "> "
		}
		cnt := countFor(inventor)
		countCol := fmt.Sprintf("(%d)", cnt)
		nameWidth := max(10, rowWidth-len(prefix)-indexWidth-countWidth-1)
		row := prefix + m.pad(rowIndexLabel(i), indexWidth) + m.pad(m.truncate(inventor, nameWidth), nameWidth) + " " + m.pad(countCol, countWidth)
		b.WriteString(m.styleRowOverlay(i, selected, row, rowWidth) + "\n")
	}
	return b.String()
}

func (m *Model) viewText() string {
	sections, err := m.repo.ListTextSections(m.ctx, m.ProjectID, m.current.Number)
	if err != nil {
		return m.renderPopup("Full Text", err.Error()+"\n")
	}
	var body strings.Builder
	width := m.overlayWidth() - 4
	for _, section := range sections {
		body.WriteString(lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("[%s %d]", strings.Title(section.SectionType), section.Ordinal)) + "\n")
		body.WriteString(wrapText(section.Text, width) + "\n\n")
	}
	if body.Len() == 0 {
		return m.renderPopup("Full Text", "No text sections available. Re-import the patent to fetch full text.\n")
	}
	return m.renderPopup("Full Text · "+m.current.Number, body.String())
}

func (m *Model) viewNoteEdit() string {
	var b strings.Builder
	p := m.current
	year := ""
	switch {
	case len(p.GrantDate) >= 4:
		year = p.GrantDate[:4]
	case len(p.PublicationDate) >= 4:
		year = p.PublicationDate[:4]
	}

	title := "Note · " + p.Number
	if inv := formatInventorsShort(p.Inventors); inv != "-" {
		title += " · " + inv
	}
	if year != "" {
		title += " · " + year
	}
	b.WriteString(m.renderPopupHeader(title))
	b.WriteString(m.noteTA.View())
	return b.String()
}

func (m *Model) viewNotes() string {
	notes, err := m.repo.ListNotes(m.ctx, m.ProjectID, m.current.Number)
	if err != nil {
		return m.renderPopup("Notes", err.Error()+"\n")
	}
	base := overlayBase()
	dimStyle := base.Foreground(lipgloss.Color(ColorDim)).Italic(true)
	subtleStyle := base.Foreground(lipgloss.Color(ColorSubtle))

	var body strings.Builder
	if len(notes) == 0 {
		body.WriteString(dimStyle.Render("No notes. Press N to add one."))
		body.WriteString("\n")
	} else {
		for _, note := range notes {
			year := note.CreatedAt.Format("2006")
			name := markdownHeadingSummary(note.Body)
			if len(name) > 48 {
				name = name[:48] + "…"
			}
			header := year
			if name != "" {
				header += " · " + name
			}
			body.WriteString(subtleStyle.Render(header))
			body.WriteString("\n")
			body.WriteString(note.Body)
			body.WriteString("\n\n")
		}
	}
	return m.renderPopup("Notes · "+m.current.Number, body.String())
}

func (m *Model) viewRefs() string {
	refs, err := m.repo.ListReferences(m.ctx, m.ProjectID)
	if err != nil {
		return err.Error() + "\n"
	}
	if len(refs) == 0 {
		return m.renderPopup("References", "No reference entries.\n")
	}
	var body strings.Builder
	for _, ref := range refs {
		body.WriteString(fmt.Sprintf("- %s\n", ref.CitationLabel))
	}
	return m.renderPopup("References", body.String())
}

func (m *Model) viewSplash() string {
	logo := `
  ██████╗  █████╗ ████████╗███████╗███╗   ██╗████████╗    ███╗   ███╗██╗███╗   ██╗███████╗
  ██╔══██╗██╔══██╗╚══██╔══╝██╔════╝████╗  ██║╚══██╔══╝    ████╗ ████║██║████╗  ██║██╔════╝
  ██████╔╝███████║   ██║   █████╗  ██╔██╗ ██║   ██║       ██╔████╔██║██║██╔██╗ ██║█████╗  
  ██╔═══╝ ██╔══██║   ██║   ██╔══╝  ██║╚██╗██║   ██║       ██║╚██╔╝██║██║██║╚██╗██║██╔══╝  
  ██║     ██║  ██║   ██║   ███████╗██║ ╚████║   ██║       ██║ ╚═╝ ██║██║██║ ╚████║███████╗
  ╚═╝     ╚═╝  ╚═╝   ╚═╝   ╚══════╝╚═╝  ╚═══╝   ╚═╝       ╚═╝     ╚═╝╚═╝╚═╝  ╚═══╝╚══════╝
	`
	logoStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.screenColor())).Bold(true)
	subStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Italic(true)

	var b strings.Builder
	b.WriteString("\n\n")
	b.WriteString(m.center(logoStyle.Render(logo)))
	b.WriteString("\n")
	b.WriteString(m.center(subStyle.Render("Local Patent Research & Intelligence")))
	b.WriteString("\n")
	b.WriteString(m.center(subStyle.Render("Version " + m.displayVersion())))
	separator := lipgloss.NewStyle().Width(m.width).Foreground(lipgloss.Color(ColorSubtle)).Render(strings.Repeat("─", m.width))

	b.WriteString("\n\n" + separator + "\n\n")

	if m.input.Focused() {
		promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(m.screenColor())).Bold(true)
		b.WriteString(m.center(promptStyle.Render("COMMAND: ") + m.input.View()))
		b.WriteString("\n\n")
	}

	b.WriteString(m.center(lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color(m.screenColor())).Render("SELECT PROJECT")))
	b.WriteString("\n\n")

	if len(m.projects) == 0 {
		b.WriteString(m.center("No projects found. Create one with 'n'"))
	} else {
		pHeader := fmt.Sprintf("   %-20s %-12s %-10s %-13s %-10s %s", "Name", "ID", "Status", "Summary", "Unpaid", "Updated")
		b.WriteString(m.center(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Underline(true).Render(pHeader)))
		b.WriteString("\n")

		for i, p := range m.projects {
			prefix := "  "
			if i == m.projectSelected {
				prefix = "→ "
			}

			updated := p.UpdatedAt.Format("2006-01-02")

			summaryLabel := p.SummaryStatus
			if label, ok := SummaryStatusLabels[p.SummaryStatus]; ok {
				summaryLabel = label
			}
			summaryColor := ColorSubtle
			if c, ok := SummaryStatusColors[p.SummaryStatus]; ok {
				summaryColor = c
			}

			unpaidCount := m.unpaidCounts[p.ID]
			unpaidLabel := "—"
			unpaidColor := ColorSubtle
			if unpaidCount > 0 {
				unpaidLabel = fmt.Sprintf("%d unpaid", unpaidCount)
				unpaidColor = ColorWarning
			}

			rowBase := fmt.Sprintf("%-20s %-12s %-10s ", p.Name, "["+p.ID+"]", p.Status)
			summaryPart := fmt.Sprintf("%-13s", summaryLabel)
			unpaidPart := fmt.Sprintf("%-10s", unpaidLabel)
			datePart := updated

			rowStyle := lipgloss.NewStyle()
			if i == m.projectSelected {
				rowStyle = rowStyle.Foreground(lipgloss.Color(ColorTheme)).Bold(true)
			} else {
				rowStyle = rowStyle.Foreground(lipgloss.Color(ColorSubtle))
			}
			summaryStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(summaryColor))
			unpaidStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(unpaidColor))
			if i == m.projectSelected {
				summaryStyle = summaryStyle.Bold(true)
				unpaidStyle = unpaidStyle.Bold(true)
			}

			b.WriteString(m.center(prefix + rowStyle.Render(rowBase) + summaryStyle.Render(summaryPart) + unpaidStyle.Render(unpaidPart) + rowStyle.Render(datePart)))
			b.WriteString("\n")
			if p.Summary != "" {
				summaryTextStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDim)).Italic(true)
				b.WriteString(m.center("    " + summaryTextStyle.Render(p.Summary)))
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n\n" + separator + "\n")
	b.WriteString(m.center(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Render("[j/k/↓↑]: move · [enter]: select · [e]: events · [i]: invoices · [d]: IDS · [n]: new · [Q]: quit")))

	// Center vertically
	content := b.String()
	lines := strings.Count(content, "\n")
	padding := (m.height - lines) / 2
	if padding > 0 {
		content = strings.Repeat("\n", padding) + content
	}

	return content
}

func (m *Model) viewProjectEvents() string {
	events, err := m.repo.ListProjectEvents(m.ctx, m.ProjectID)
	if err != nil {
		return "error loading events: " + err.Error()
	}

	base := overlayBase()
	titleStyle := base.Bold(true).Underline(true)
	subtleStyle := base.Foreground(lipgloss.Color(ColorSubtle))
	dimStyle := base.Foreground(lipgloss.Color(ColorDim)).Italic(true)

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Prosecution History · %s", m.ProjectID)))
	b.WriteString("\n\n")

	if len(events) == 0 {
		b.WriteString(dimStyle.Render("No events. Add with :project event <type> [date YYYY-MM-DD] [due YYYY-MM-DD] [note <text>]"))
		b.WriteString("\n\n")
		b.WriteString(subtleStyle.Render("Event types: provisional-filed · application-filed · publication · office-action-non-final\n"))
		b.WriteString(subtleStyle.Render("             office-action-final · response-filed · rce-filed · notice-of-allowance\n"))
		b.WriteString(subtleStyle.Render("             issue-fee-paid · patent-granted · maintenance-due-3y/7y/11y\n"))
		b.WriteString(subtleStyle.Render("             continuation-filed · divisional-filed · cip-filed · appeal-filed\n"))
		b.WriteString(subtleStyle.Render("             ptab-decision · ipr-filed · reexam-requested · extension-filed\n"))
		b.WriteString(subtleStyle.Render("             abandonment · revival-filed"))
	} else {
		headerStyle := base.Foreground(lipgloss.Color(ColorSubtle)).Underline(true)
		b.WriteString(headerStyle.Render(fmt.Sprintf("  %-26s %-12s %-12s %-14s %s", "Event", "Date", "Due", "Reference", "Notes")))
		b.WriteString("\n")
		for i, e := range events {
			label := e.EventType
			if l, ok := EventTypeLabels[e.EventType]; ok {
				label = l
			}
			color := ColorSubtle
			if c, ok := EventTypeColors[e.EventType]; ok {
				color = c
			}
			eventStyle := base.Foreground(lipgloss.Color(color))
			rowStyle := base.Foreground(lipgloss.Color(ColorSubtle))
			if i == m.projectEventsSelected {
				eventStyle = eventStyle.Bold(true).Reverse(true)
				rowStyle = rowStyle.Bold(true)
			}

			prefix := "  "
			if i == m.projectEventsSelected {
				prefix = "→ "
			}

			due := e.DueDate
			if due == "" {
				due = "—"
			}
			date := e.EventDate
			if date == "" {
				date = "—"
			}
			ref := e.Reference
			if ref == "" {
				ref = "—"
			}
			notes := e.Notes
			if len(notes) > 30 {
				notes = notes[:27] + "..."
			}
			if notes == "" {
				notes = "—"
			}

			b.WriteString(prefix + eventStyle.Render(fmt.Sprintf("%-26s", label)) + rowStyle.Render(fmt.Sprintf("%-12s %-12s %-14s %s", date, due, ref, notes)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(subtleStyle.Render("j/k: move · D: delete · esc: back"))
	return b.String()
}

func (m *Model) viewProjectInvoices() string {
	invoices, err := m.repo.ListProjectInvoices(m.ctx, m.ProjectID)
	if err != nil {
		return "error loading invoices: " + err.Error()
	}

	base := overlayBase()
	titleStyle := base.Bold(true).Underline(true)
	subtleStyle := base.Foreground(lipgloss.Color(ColorSubtle))
	dimStyle := base.Foreground(lipgloss.Color(ColorDim)).Italic(true)

	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Invoices · %s", m.ProjectID)))
	b.WriteString("\n\n")

	if len(invoices) == 0 {
		b.WriteString(dimStyle.Render("No invoices. Add with :project invoice <amount> [currency USD] [direction to-firm|from-firm] [date YYYY-MM-DD] [due YYYY-MM-DD] [firm <name>] [ref <number>] [note <text>]"))
	} else {
		headerStyle := base.Foreground(lipgloss.Color(ColorSubtle)).Underline(true)
		b.WriteString(headerStyle.Render(fmt.Sprintf("  %-12s %-10s %-10s %-14s %-16s %-12s %s", "Amount", "Currency", "Direction", "Status", "Firm", "Date", "Notes")))
		b.WriteString("\n")
		for i, inv := range invoices {
			statusColor := ColorSubtle
			if c, ok := InvoiceStatusColors[inv.Status]; ok {
				statusColor = c
			}
			statusLabel := inv.Status
			if l, ok := InvoiceStatusLabels[inv.Status]; ok {
				statusLabel = l
			}
			dirLabel := inv.Direction
			if l, ok := InvoiceDirectionLabels[inv.Direction]; ok {
				dirLabel = l
			}

			amtStyle := base.Foreground(lipgloss.Color(statusColor))
			rowStyle := base.Foreground(lipgloss.Color(ColorSubtle))
			if i == m.projectInvoicesSelected {
				amtStyle = amtStyle.Bold(true).Reverse(true)
				rowStyle = rowStyle.Bold(true)
			}

			prefix := "  "
			if i == m.projectInvoicesSelected {
				prefix = "→ "
			}

			notes := inv.Notes
			if len(notes) > 20 {
				notes = notes[:17] + "..."
			}
			if notes == "" {
				notes = "—"
			}
			date := inv.InvoiceDate
			if date == "" {
				date = "—"
			}
			firm := inv.FirmName
			if firm == "" {
				firm = "—"
			}
			if len(firm) > 14 {
				firm = firm[:12] + ".."
			}

			b.WriteString(prefix + amtStyle.Render(fmt.Sprintf("%-12s", inv.Amount+" "+inv.Currency)) + rowStyle.Render(fmt.Sprintf("%-10s %-14s %-16s %-12s %s", dirLabel, statusLabel, firm, date, notes)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(subtleStyle.Render("j/k: move · p: mark paid · D: delete · esc: back"))
	return b.String()
}

func (m *Model) viewProjectIDS() string {
	ids, err := m.repo.ListIDSEntries(m.ctx, m.ProjectID)
	if err != nil {
		return "error loading IDS: " + err.Error()
	}

	base := overlayBase()
	subtleStyle := base.Foreground(lipgloss.Color(ColorSubtle))
	dimStyle := base.Foreground(lipgloss.Color(ColorDim)).Italic(true)

	var b strings.Builder
	b.WriteString(m.renderPopupTitle(fmt.Sprintf("IDS · %s", m.ProjectID)))
	b.WriteString("\n\n")

	if len(ids) == 0 {
		b.WriteString(dimStyle.Render("No IDS entries. Add with :project ids add <patent-number> [note <text>]"))
		b.WriteString("\n\n")
		b.WriteString(subtleStyle.Render("IDS entries are prior art references to disclose to the patent office."))
	} else {
		sel := clamp(m.projectIDSSelected, 0, len(ids)-1)
		window := pageWindow(sel, len(ids), m.overlayPageSize())

		b.WriteString(subtleStyle.Render(pageStatus(m.text.T(TextValuePageStatus), window)))
		b.WriteString("\n\n")

		// fixed cols: prefix(2) + #(4) + patent(17) + status(12) + date(11) = 46
		// remaining split: notes ~20, passages ~rest
		rowWidth := max(60, m.overlayWidth()-4)
		noteW := 20
		passW := max(10, rowWidth-46-noteW-1)

		headerStyle := base.Foreground(lipgloss.Color(ColorSubtle)).Underline(true)
		b.WriteString(headerStyle.Render(fmt.Sprintf("  %-3s %-16s %-11s %-10s %-*s %s", "#", "Patent", "Status", "Added", noteW, "Notes", "Passages")))
		b.WriteString("\n")
		for i := window.Start; i < window.End; i++ {
			e := ids[i]
			rowStyle := base.Foreground(lipgloss.Color(ColorSubtle))
			numStyle := base.Foreground(lipgloss.Color(ColorTheme))
			statusColor := idsStatusColor(e.Status)
			if i == sel {
				rowStyle = rowStyle.Bold(true).Reverse(true)
				numStyle = numStyle.Bold(true)
			}
			prefix := "  "
			if i == sel {
				prefix = "→ "
			}
			notes := e.Notes
			if len(notes) > noteW {
				notes = notes[:noteW-3] + "..."
			}
			if notes == "" {
				notes = "—"
			}
			passages := domain.IDSPassagesText(e)
			if len(passages) > passW {
				passages = passages[:passW-3] + "..."
			}
			if passages == "" {
				passages = "—"
			}
			statusStr := string(e.Status)
			if statusStr == "" {
				statusStr = string(domain.IDSStatusPending)
			}
			statusRendered := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Render(fmt.Sprintf("%-11s", statusStr))
			b.WriteString(prefix + numStyle.Render(fmt.Sprintf("%-3d", i+1)) + " " + rowStyle.Render(fmt.Sprintf("%-16s", e.PatentNumber)) + " " + statusRendered + " " + rowStyle.Render(fmt.Sprintf("%-10s %-*s %s", e.AddedAt.Format("2006-01-02"), noteW, notes, passages)))
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m *Model) idsCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.projectIDSSelected = 0
		m.mode = viewProjectIDS
		return m, nil
	}
	switch args[0] {
	case idsSubAdd:
		if len(args) < 2 {
			m.err = "usage: :project ids add <patent-number> [note <text>]"
			return m, nil
		}
		entry := domain.IDSEntry{
			ProjectID:    m.ProjectID,
			PatentNumber: strings.ToUpper(strings.TrimSpace(args[1])),
		}
		rest := args[2:]
		for i := 0; i < len(rest); i++ {
			if rest[i] == argNote && i+1 < len(rest) {
				entry.Notes = strings.Join(rest[i+1:], " ")
				break
			}
		}
		if _, err := m.repo.AddIDSEntry(m.ctx, entry); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.message = "IDS entry added: " + entry.PatentNumber
	case idsSubMeta:
		// :project ids meta <field> <value>
		// fields: app, filed, inventor, art, examiner, docket
		if len(args) < 3 {
			m.err = "usage: :project ids meta <field> <value>"
			return m, nil
		}
		meta, err := m.repo.GetIDSMetadata(m.ctx, m.ProjectID)
		if err != nil {
			m.err = "error loading IDS metadata: " + err.Error()
			return m, nil
		}
		value := strings.Join(args[2:], " ")
		switch strings.ToLower(args[1]) {
		case "appnumber", "app":
			meta.AppNumber = value
		case "filingdate", "filed":
			meta.FilingDate = value
		case "inventor":
			meta.FirstInventor = value
		case "artunit", "art":
			meta.ArtUnit = value
		case "examiner":
			meta.ExaminerName = value
		case "docket":
			meta.AttorneyDocket = value
		default:
			m.err = "unknown IDS meta field: " + args[1] + ". Use: app, filed, inventor, art, examiner, docket"
			return m, nil
		}
		if err := m.repo.SaveIDSMetadata(m.ctx, meta); err != nil {
			m.err = "error saving IDS metadata: " + err.Error()
			return m, nil
		}
		m.message = "IDS metadata updated"
	default:
		m.err = "unknown ids subcommand: " + args[0]
	}
	return m, nil
}

func (m *Model) exportCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.err = "usage: :project export ids|status|state [<state>] [filename]"
		return m, nil
	}
	switch args[0] {
	case exportSubIDS:
		return m.idsExportCommand(args[1:])
	case exportSubStatus:
		return m.statusExportCommand(args[1:])
	case exportSubState:
		return m.stateExportCommand(args[1:])
	default:
		m.err = "unknown export type: " + args[0] + " — use ids, status, or state"
	}
	return m, nil
}

func (m *Model) projectNameForExport() string {
	for _, p := range m.projects {
		if p.ID == m.ProjectID {
			return p.Name
		}
	}
	return m.ProjectID
}

func (m *Model) statusExportCommand(args []string) (tea.Model, tea.Cmd) {
	projectName := m.projectNameForExport()
	filename := fmt.Sprintf("%s_status_%s.md", strings.ReplaceAll(projectName, " ", "_"), time.Now().Format("2006-01-02"))
	if len(args) > 0 {
		filename = args[0]
	}

	var proj domain.Project
	for _, p := range m.projects {
		if p.ID == m.ProjectID {
			proj = p
			break
		}
	}

	events, _ := m.repo.ListProjectEvents(m.ctx, m.ProjectID)
	invoices, _ := m.repo.ListProjectInvoices(m.ctx, m.ProjectID)
	ids, _ := m.repo.ListIDSEntries(m.ctx, m.ProjectID)
	allPatents, _ := m.repo.ListPatents(m.ctx, m.ProjectID, storage.ListPatentsOptions{StatusFilter: statusFilterNone})

	statusCounts := map[string]int{}
	for _, p := range allPatents {
		statusCounts[p.Status]++
	}

	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("# Project Status: %s\n\n", projectName))
	buf.WriteString(fmt.Sprintf("**ID:** %s  \n", proj.ID))
	buf.WriteString(fmt.Sprintf("**Status:** %s  \n", proj.Status))
	if proj.SummaryStatus != "" {
		label := proj.SummaryStatus
		if l, ok := SummaryStatusLabels[proj.SummaryStatus]; ok {
			label = l
		}
		buf.WriteString(fmt.Sprintf("**Stage:** %s  \n", label))
	}
	buf.WriteString(fmt.Sprintf("**Updated:** %s  \n", proj.UpdatedAt.Format("2006-01-02")))
	if proj.Summary != "" {
		buf.WriteString(fmt.Sprintf("\n> %s\n", proj.Summary))
	}
	if proj.Comments != "" {
		buf.WriteString(fmt.Sprintf("\n*%s*\n", proj.Comments))
	}

	buf.WriteString("\n## Patents\n\n")
	buf.WriteString(fmt.Sprintf("| Status | Count |\n|--------|-------|\n"))
	for _, s := range []string{domain.CitationStatusStored, domain.CitationStatusUnderReview, domain.CitationStatusIgnored, domain.CitationStatusCached} {
		if n := statusCounts[s]; n > 0 {
			buf.WriteString(fmt.Sprintf("| %s | %d |\n", s, n))
		}
	}
	buf.WriteString(fmt.Sprintf("\n**IDS entries:** %d\n", len(ids)))

	if len(events) > 0 {
		buf.WriteString("\n## Prosecution History\n\n")
		buf.WriteString("| Event | Date | Due | Reference | Notes |\n")
		buf.WriteString("|-------|------|-----|-----------|-------|\n")
		for _, e := range events {
			label := e.EventType
			if l, ok := EventTypeLabels[e.EventType]; ok {
				label = l
			}
			due, date, ref, notes := e.DueDate, e.EventDate, e.Reference, e.Notes
			if due == "" {
				due = "—"
			}
			if date == "" {
				date = "—"
			}
			if ref == "" {
				ref = "—"
			}
			if notes == "" {
				notes = "—"
			}
			buf.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n", label, date, due, ref, notes))
		}
	}

	if len(invoices) > 0 {
		buf.WriteString("\n## Invoices\n\n")
		buf.WriteString("| Amount | Currency | Direction | Status | Firm | Date | Notes |\n")
		buf.WriteString("|--------|----------|-----------|--------|------|------|-------|\n")
		for _, inv := range invoices {
			dir := inv.Direction
			if l, ok := InvoiceDirectionLabels[inv.Direction]; ok {
				dir = l
			}
			firm, date, notes := inv.FirmName, inv.InvoiceDate, inv.Notes
			if firm == "" {
				firm = "—"
			}
			if date == "" {
				date = "—"
			}
			if notes == "" {
				notes = "—"
			}
			buf.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s |\n",
				inv.Amount, inv.Currency, dir, inv.Status, firm, date, notes))
		}
	}

	if err := os.WriteFile(filename, []byte(buf.String()), 0644); err != nil {
		m.err = "export failed: " + err.Error()
		return m, nil
	}
	m.message = fmt.Sprintf("status exported to %s", filename)
	return m, nil
}

var exportStateAliases = map[string]string{
	exportStateStored:      domain.CitationStatusStored,
	exportStateIgnored:     domain.CitationStatusIgnored,
	exportStateUnderReview: domain.CitationStatusUnderReview,
	"under_review":         domain.CitationStatusUnderReview,
	exportStateAll:         statusFilterNone,
	exportStateNone:        statusFilterNone,
}

func (m *Model) stateExportCommand(args []string) (tea.Model, tea.Cmd) {
	statusFilter := m.statusFilter
	filenameArgs := args

	if len(args) > 0 {
		if canonical, ok := exportStateAliases[strings.ToLower(args[0])]; ok {
			statusFilter = canonical
			filenameArgs = args[1:]
		}
	}

	if statusFilter == "" {
		statusFilter = statusFilterNone
	}

	projectName := m.projectNameForExport()
	stateLabel := statusFilter
	filename := fmt.Sprintf("%s_state_%s_%s.md", strings.ReplaceAll(projectName, " ", "_"), stateLabel, time.Now().Format("2006-01-02"))
	if len(filenameArgs) > 0 {
		filename = filenameArgs[0]
	}

	patents, err := m.repo.ListPatents(m.ctx, m.ProjectID, storage.ListPatentsOptions{StatusFilter: statusFilter})
	if err != nil {
		m.err = "export failed: " + err.Error()
		return m, nil
	}

	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("# Patent List: %s\n\n", projectName))
	buf.WriteString(fmt.Sprintf("**State filter:** %s  \n", stateLabel))
	buf.WriteString(fmt.Sprintf("**Date:** %s  \n", time.Now().Format("2006-01-02")))
	buf.WriteString(fmt.Sprintf("**Count:** %d  \n\n", len(patents)))

	if len(patents) == 0 {
		buf.WriteString("*No patents found.*\n")
	} else {
		buf.WriteString("| # | Number | Title | Assignee | Publication | Expiration | Status |\n")
		buf.WriteString("|---|--------|-------|----------|-------------|------------|--------|\n")
		for i, p := range patents {
			title := p.Title
			if len(title) > 50 {
				title = title[:47] + "..."
			}
			assignee := p.Assignee
			if assignee == "" {
				assignee = "—"
			}
			pub := p.PublicationDate
			if pub == "" {
				pub = "—"
			}
			exp := p.ExpirationDate
			if exp == "" {
				exp = "—"
			}
			buf.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | %s | %s |\n",
				i+1, p.Number, title, assignee, pub, exp, p.Status))
		}
	}

	if err := os.WriteFile(filename, []byte(buf.String()), 0644); err != nil {
		m.err = "export failed: " + err.Error()
		return m, nil
	}
	m.message = fmt.Sprintf("state exported to %s (%d patents)", filename, len(patents))
	return m, nil
}

func (m *Model) viewProjectInfo() string {
	var proj domain.Project
	for _, p := range m.projects {
		if p.ID == m.ProjectID {
			proj = p
			break
		}
	}

	events, _ := m.repo.ListProjectEvents(m.ctx, m.ProjectID)
	invoices, _ := m.repo.ListProjectInvoices(m.ctx, m.ProjectID)

	base := overlayBase()
	labelStyle := base.Foreground(lipgloss.Color(ColorSubtle))
	valueStyle := base.Foreground(lipgloss.Color(ColorTheme))
	dimStyle := base.Foreground(lipgloss.Color(ColorDim)).Italic(true)

	var body strings.Builder
	body.WriteString(labelStyle.Render(fmt.Sprintf("%-16s", "Name:")) + valueStyle.Render(proj.Name) + "\n")
	body.WriteString(labelStyle.Render(fmt.Sprintf("%-16s", "ID:")) + valueStyle.Render(proj.ID) + "\n")
	body.WriteString(labelStyle.Render(fmt.Sprintf("%-16s", "Status:")) + valueStyle.Render(proj.Status) + "\n")

	if proj.SummaryStatus != "" {
		label := proj.SummaryStatus
		if l, ok := SummaryStatusLabels[proj.SummaryStatus]; ok {
			label = l
		}
		color := ColorSubtle
		if c, ok := SummaryStatusColors[proj.SummaryStatus]; ok {
			color = c
		}
		body.WriteString(labelStyle.Render(fmt.Sprintf("%-16s", "App Status:")) +
			base.Foreground(lipgloss.Color(color)).Bold(true).Render(label) + "\n")
	}

	body.WriteString(labelStyle.Render(fmt.Sprintf("%-16s", "Updated:")) + valueStyle.Render(proj.UpdatedAt.Format("2006-01-02")) + "\n")

	if proj.Summary != "" {
		body.WriteString("\n" + dimStyle.Render(proj.Summary) + "\n")
	}
	if proj.Comments != "" {
		body.WriteString(dimStyle.Render(proj.Comments) + "\n")
	}

	// Invoice summary
	body.WriteString("\n" + base.Bold(true).Render("Invoices") + "\n\n")
	if len(invoices) == 0 {
		body.WriteString(dimStyle.Render("No invoices.") + "\n")
	} else {
		counts := map[string]int{}
		for _, inv := range invoices {
			counts[inv.Status]++
		}
		for _, status := range []string{
			domain.InvoiceStatusOutstanding,
			domain.InvoiceStatusOverdue,
			domain.InvoiceStatusDisputed,
			domain.InvoiceStatusPaid,
		} {
			if n := counts[status]; n > 0 {
				label := status
				if l, ok := InvoiceStatusLabels[status]; ok {
					label = l
				}
				color := ColorSubtle
				if c, ok := InvoiceStatusColors[status]; ok {
					color = c
				}
				body.WriteString(labelStyle.Render(fmt.Sprintf("  %-14s", label+":")) +
					base.Foreground(lipgloss.Color(color)).Render(fmt.Sprintf("%d", n)) + "\n")
			}
		}
	}

	// Recent events
	body.WriteString("\n" + base.Bold(true).Render("Recent Events") + "\n\n")
	if len(events) == 0 {
		body.WriteString(dimStyle.Render("No events.") + "\n")
	} else {
		limit := 5
		if len(events) < limit {
			limit = len(events)
		}
		for _, e := range events[:limit] {
			label := e.EventType
			if l, ok := EventTypeLabels[e.EventType]; ok {
				label = l
			}
			color := ColorSubtle
			if c, ok := EventTypeColors[e.EventType]; ok {
				color = c
			}
			date := e.EventDate
			if date == "" {
				date = "—"
			}
			body.WriteString(base.Foreground(lipgloss.Color(color)).Render(fmt.Sprintf("  %-26s", label)) +
				labelStyle.Render(date) + "\n")
		}
		if len(events) > 5 {
			body.WriteString(dimStyle.Render(fmt.Sprintf("  … and %d more", len(events)-5)) + "\n")
		}
	}

	body.WriteString("\n")
	if m.input.Focused() {
		body.WriteString(base.Foreground(lipgloss.Color(ColorTheme)).Bold(true).Render("COMMAND: ") + m.input.View() + "\n")
	}
	return m.renderPopup("Project Info", body.String())
}

func (m *Model) viewIDSEdit() string {
	entry := m.idsEntryForPatent(m.current.Number)

	base := overlayBase()
	labelStyle := base.Foreground(lipgloss.Color(ColorSubtle))
	valueStyle := base.Foreground(lipgloss.Color(ColorTheme))
	dimStyle := base.Foreground(lipgloss.Color(ColorDim)).Italic(true)
	hintStyle := base.Foreground(lipgloss.Color(ColorSubtle))

	var body strings.Builder
	if entry == nil {
		body.WriteString(dimStyle.Render("No IDS entry.") + "\n")
		return m.renderPopup("IDS Entry · "+m.current.Number, body.String())
	}

	statusColor := idsStatusColor(entry.Status)
	statusStr := string(entry.Status)
	if statusStr == "" {
		statusStr = string(domain.IDSStatusPending)
	}
	body.WriteString(labelStyle.Render(fmt.Sprintf("%-18s", "Status:")) +
		base.Foreground(lipgloss.Color(statusColor)).Bold(true).Render(statusStr) + "\n")

	kindVal := entry.KindCode
	if kindVal == "" {
		kindVal = dimStyle.Render("—")
	} else {
		kindVal = valueStyle.Render(kindVal)
	}
	body.WriteString(labelStyle.Render(fmt.Sprintf("%-18s", "Kind Code:")) + kindVal + "\n")

	ccVal := entry.CountryCode
	if ccVal == "" {
		ccVal = dimStyle.Render("—")
	} else {
		ccVal = valueStyle.Render(ccVal)
	}
	body.WriteString(labelStyle.Render(fmt.Sprintf("%-18s", "Country Code:")) + ccVal + "\n")

	notesVal := entry.Notes
	if notesVal == "" {
		notesVal = dimStyle.Render("—")
	} else {
		notesVal = valueStyle.Render(notesVal)
	}
	body.WriteString(labelStyle.Render(fmt.Sprintf("%-18s", "Notes:")) + notesVal + "\n")

	inFullVal := dimStyle.Render("—")
	if entry.InFull {
		inFullVal = valueStyle.Render(domain.IDSIconInFull + " Yes")
	}
	body.WriteString(labelStyle.Render(fmt.Sprintf("%-18s", "In Full:")) + inFullVal + "\n")

	passagesVal := domain.IDSPassagesText(*entry)
	if passagesVal == "" {
		passagesVal = dimStyle.Render("—")
	} else {
		passagesVal = valueStyle.Render(passagesVal)
	}
	body.WriteString(labelStyle.Render(fmt.Sprintf("%-18s", "Relevant Passages:")) + passagesVal + "\n")

	if m.input.Focused() {
		body.WriteString("\n")
		body.WriteString(base.Foreground(lipgloss.Color(ColorTheme)).Bold(true).Render("COMMAND: ") + base.Render(m.input.View()) + "\n")
		if strings.HasPrefix(m.input.Value(), ":ids passages") {
			body.WriteString(hintStyle.Render("fmt: "+domain.IDSPassagesFormatGuide) + "\n")
		}
	}
	return m.renderPopup("IDS Entry · "+m.current.Number, body.String())
}

func (m *Model) center(s string) string {
	if m.width <= 0 {
		return s
	}
	if m.isSplashContext() {
		return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, s)
	}
	return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, s,
		lipgloss.WithWhitespaceBackground(lipgloss.Color(ColorSurface)))
}

func (m *Model) viewAI() string {
	artifacts, err := m.repo.ListAIAnalyses(m.ctx, m.ProjectID, m.current.Number)
	if err != nil {
		return err.Error() + "\n"
	}
	if len(artifacts) == 0 {
		return m.renderPopup("AI", "No AI artifacts. Run :summarize or :compare US11611785B2.\n")
	}
	var body strings.Builder
	for _, artifact := range artifacts {
		label := artifact.AnalysisType
		if artifact.ComparedPatentNumber != "" {
			label += " vs " + artifact.ComparedPatentNumber
		}
		body.WriteString(fmt.Sprintf("[%s, %s]\n%s\n\n", label, artifact.Provider, artifact.Body))
	}
	return m.renderPopup("AI", body.String())
}

func (m *Model) viewHelp() string {
	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle))
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.screenColor()))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorTheme)).Bold(true)
	cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAccent))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorWarning))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDim))

	// Build filtered sections
	sections := FilterHelpSections(m.helpQuery, m.text)

	// Build flat line list
	type helpLine struct {
		text     string
		isHeader bool
	}
	var lines []helpLine

	const keyWidth = 22
	const cmdWidth = 42

	for _, section := range sections {
		lines = append(lines, helpLine{
			text:     sectionStyle.Render("▸ " + section.Title),
			isHeader: true,
		})
		for _, e := range section.Entries {
			keyPart := fmt.Sprintf("%-*s", keyWidth, e.Key)
			cmdPart := ""
			if e.Command != "" {
				cmd := e.Command
				if len(cmd) > cmdWidth {
					cmd = cmd[:cmdWidth-1] + "…"
				}
				cmdPart = fmt.Sprintf("%-*s", cmdWidth, cmd)
			} else {
				cmdPart = fmt.Sprintf("%-*s", cmdWidth, "")
			}
			desc := m.text.T(e.Description)
			line := "  " + keyStyle.Render(keyPart) + " " + cmdStyle.Render(cmdPart) + " " + dimStyle.Render(desc)
			lines = append(lines, helpLine{text: line})
		}
		lines = append(lines, helpLine{text: ""})
	}

	// Clamp scroll
	pageH := m.pageSize() - 4 // leave room for header + search bar + hint
	if pageH < 1 {
		pageH = 1
	}
	maxScroll := len(lines) - pageH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.helpScroll > maxScroll {
		m.helpScroll = maxScroll
	}

	var b strings.Builder

	// Search bar
	searchBar := ""
	if m.helpSearchActive {
		searchBar = accent.Render("/") + " " + m.helpQuery + "█"
	} else if m.helpQuery != "" {
		searchBar = accent.Render("/") + " " + m.helpQuery + subtle.Render(" (esc to clear)")
	} else {
		searchBar = subtle.Render(m.text.T(TextHelpSearchHint))
	}
	b.WriteString(searchBar + "\n\n")

	// Content with scroll window
	end := m.helpScroll + pageH
	if end > len(lines) {
		end = len(lines)
	}
	for _, line := range lines[m.helpScroll:end] {
		b.WriteString(line.text + "\n")
	}

	return b.String()
}

func (m *Model) viewHelpPopup() string {
	return RenderContextHelp(m.text, m.activeMode())
}
