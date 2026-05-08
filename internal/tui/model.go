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
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"patentmine/internal/ai"
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
)

type Model struct {
	ctx                    context.Context
	repo                   storage.Repository
	input                  textinput.Model
	spinner                spinner.Model
	loading                bool
	loadingMsg             string
	cancel                 context.CancelFunc
	mode                   viewMode
	patents                []domain.Patent
	selected               int
	detailSelected         int
	citesSelected          int
	citedBySelected        int
	reviewSelected         int
	classificationSelected int
	inventorSelected       int
	current                domain.Patent
	pendingBundle          domain.PatentBundle
	pendingCitation        domain.CitationEdge
	reviewStatus           string
	filter                 string
	message                string
	err                    string
	logger                 *slog.Logger
	text                   TextCatalog
	width                  int
	height                 int
	backStack              []navSnapshot
	jumpMode               bool
	countBuffer            string
}

type navSnapshot struct {
	mode                   viewMode
	patents                []domain.Patent
	selected               int
	detailSelected         int
	citesSelected          int
	citedBySelected        int
	reviewSelected         int
	classificationSelected int
	current                domain.Patent
	pendingBundle          domain.PatentBundle
	pendingCitation        domain.CitationEdge
	reviewStatus           string
	filter                 string
	message                string
	err                    string
	countBuffer            string
}

func New(ctx context.Context, repo storage.Repository, logger *slog.Logger) Model {
	input := textinput.New()
	input.Placeholder = ":add US11611785B2, :open US11611785B2, /machine learning"
	input.Prompt = ""
	input.CharLimit = 512

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	patents, _ := repo.ListPatents(ctx, "")
	if logger == nil {
		logger = slog.Default()
	}
	model := Model{
		ctx:     ctx,
		repo:    repo,
		input:   input,
		spinner: s,
		mode:    viewList,
		patents: patents,
		logger:  logger,
		text:    EnglishText(),
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
	err            error
	message        string
	patent         domain.Patent
	mode           viewMode
	citesSelected  int
	citedBySelected int
}

type refreshDetailsResultMsg struct {
	err     error
	message string
}

func (m Model) enrichClassificationDescriptionsCommand(number string) tea.Cmd {
	return func() tea.Msg {
		classifications, err := m.repo.ListClassifications(m.ctx, number)
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

		bundle, err := importer.ImportGooglePatents(rawURL)
		if err != nil {
			return nil
		}

		// Update missing descriptions in DB
		for _, scraped := range bundle.Classifications {
			if scraped.Description != "" {
				_ = m.repo.UpdateClassificationDescription(m.ctx, scraped.System, scraped.Code, scraped.Description)
			}
		}

		return classificationEnrichedMsg{Number: number}
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
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
		m.current = msg.patent
		m.mode = msg.mode
		m.citesSelected = msg.citesSelected
		m.citedBySelected = msg.citedBySelected
		m.message = msg.message
		return m, nil
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
			// Refresh current patent to pick up new descriptions
			p, _ := m.repo.GetPatent(m.ctx, msg.Number)
			m.current = p
		}
		return m, nil
	case tea.KeyMsg:
		if m.loading {
			if msg.String() == keyEsc || msg.String() == keyQuit {
				if m.cancel != nil {
					m.cancel()
					m.loading = false
					m.cancel = nil
					m.message = "operation cancelled"
					return m, nil
				}
			}
			if msg.String() == keyCtrlC {
				return m, tea.Quit
			}
			return m, nil
		}
		if m.jumpMode {
			if msg.String() == keyEsc {
				m.jumpMode = false
				return m, nil
			}
			return m.applyJump(msg.String()), nil
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
		if isCountKey(msg.String()) {
			m.countBuffer += msg.String()
			return m, nil
		}
		switch msg.String() {
		case keyCtrlC:
			return m, tea.Quit
		case keyEsc:
			m.countBuffer = ""
			return m.goBack()
		case keyQuit:
			if m.mode == viewList {
				return m, tea.Quit
			}
			return m.goBack()
		case keyCommand, keySearch:
			m.countBuffer = ""
			m.input.Focus()
			m.input.SetValue(msg.String())
			return m, nil
		case keyEnter, keyOpen:
			m.countBuffer = ""
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
				m.mode = viewClassifications
				return m, nil
			}
			if m.mode == viewInventors {
				return m.filterBySelectedInventor()
			}
			if m.mode == viewClassifications {
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
			if m.mode == viewList && len(m.patents) > 0 {
				m.mode = viewConfirmDelete
				return m, nil
			}
		case keyDown, keyArrowDown:
			count := m.consumeCount(1)
			if m.isCitationView() {
				return m.moveCitationSelection(count), nil
			}
			if m.mode == viewClassifications {
				return m.moveClassificationSelection(count), nil
			}
			if m.mode == viewInventors {
				return m.moveInventorSelection(count), nil
			}
			if m.mode == viewReview {
				return m.moveReviewSelection(count), nil
			}
			if m.mode == viewDetail {
				return m.moveDetailSelection(count), nil
			}
			if m.mode == viewList && len(m.patents) > 0 {
				m.selected = clamp(m.selected+count, 0, len(m.patents)-1)
			}
		case keyUp, keyArrowUp:
			count := m.consumeCount(1)
			if m.isCitationView() {
				return m.moveCitationSelection(-count), nil
			}
			if m.mode == viewClassifications {
				return m.moveClassificationSelection(-count), nil
			}
			if m.mode == viewInventors {
				return m.moveInventorSelection(-count), nil
			}
			if m.mode == viewReview {
				return m.moveReviewSelection(-count), nil
			}
			if m.mode == viewDetail {
				return m.moveDetailSelection(-count), nil
			}
			if m.mode == viewList && len(m.patents) > 0 {
				m.selected = clamp(m.selected-count, 0, len(m.patents)-1)
			}
		case keyCtrlF:
			m.countBuffer = ""
			if m.isCitationView() {
				return m.moveCitationSelection(m.pageSize()), nil
			}
			if m.mode == viewClassifications {
				return m.moveClassificationSelection(m.pageSize()), nil
			}
			if m.mode == viewReview {
				return m.moveReviewSelection(m.pageSize()), nil
			}
		case keyCtrlD:
			m.countBuffer = ""
			if m.isCitationView() {
				return m.moveCitationSelection(-m.pageSize()), nil
			}
			if m.mode == viewClassifications {
				return m.moveClassificationSelection(-m.pageSize()), nil
			}
			if m.mode == viewReview {
				return m.moveReviewSelection(-m.pageSize()), nil
			}
		case keyGoto:
			if m.countBuffer != "" {
				count := m.consumeCount(1)
				return m.goToRow(count), nil
			}
		case keyJump:
			m.countBuffer = ""
			if m.hasJumpTargets() {
				m.jumpMode = true
			}
		case keyCites:
			m = m.navigateTo(viewCites)
		case keyCitedBy:
			m = m.navigateTo(viewCitedBy)
		case keyClassification:
			m = m.navigateTo(viewClassifications)
		case keyText:
			m = m.navigateTo(viewText)
		case keyNotes:
			if m.mode == viewConfirmDelete {
				m.mode = viewList
				return m, nil
			}
			if m.mode == viewPreview {
				return m.skipPendingPatent()
			}
			m = m.navigateTo(viewNotes)
		case keyRefs:
			m = m.navigateTo(viewRefs)
		case keyAI:
			m = m.navigateTo(viewAI)
		case keyWeb:
			return m.openBrowser(nil)
		case keyHelp:
			if m.mode == viewHelpPopup {
				return m.goBack()
			}
			m = m.navigateTo(viewHelpPopup)
		case keyYes:
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

func (m Model) navigateTo(mode viewMode) Model {
	if m.mode == mode {
		return m
	}
	m.backStack = append(m.backStack, m.snapshot())
	m.mode = mode
	m.err = ""
	m.message = ""
	return m
}

func (m Model) snapshot() navSnapshot {
	patents := make([]domain.Patent, len(m.patents))
	copy(patents, m.patents)
	return navSnapshot{
		mode:                   m.mode,
		patents:                patents,
		selected:               m.selected,
		detailSelected:         m.detailSelected,
		citesSelected:          m.citesSelected,
		citedBySelected:        m.citedBySelected,
		reviewSelected:         m.reviewSelected,
		classificationSelected: m.classificationSelected,
		current:                m.current,
		pendingBundle:          m.pendingBundle,
		pendingCitation:        m.pendingCitation,
		reviewStatus:           m.reviewStatus,
		filter:                 m.filter,
		message:                m.message,
		err:                    m.err,
		countBuffer:            m.countBuffer,
	}
}

func (m Model) restore(snapshot navSnapshot) Model {
	m.mode = snapshot.mode
	m.patents = snapshot.patents
	m.selected = snapshot.selected
	m.detailSelected = snapshot.detailSelected
	m.citesSelected = snapshot.citesSelected
	m.citedBySelected = snapshot.citedBySelected
	m.reviewSelected = snapshot.reviewSelected
	m.classificationSelected = snapshot.classificationSelected
	m.current = snapshot.current
	m.pendingBundle = snapshot.pendingBundle
	m.pendingCitation = snapshot.pendingCitation
	m.reviewStatus = snapshot.reviewStatus
	m.filter = snapshot.filter
	m.message = snapshot.message
	m.err = snapshot.err
	m.countBuffer = snapshot.countBuffer
	return m
}

func (m Model) goBack() (tea.Model, tea.Cmd) {
	if len(m.backStack) > 0 {
		last := m.backStack[len(m.backStack)-1]
		m.backStack = m.backStack[:len(m.backStack)-1]
		return m.restore(last), nil
	}
	if m.mode == viewList && m.filter != "" {
		m.filter = ""
		return m.refreshList()
	}
	if m.mode != viewList {
		m.mode = viewList
		return m.refreshList()
	}
	return m, nil
}

func (m Model) runCommand(command Command) (tea.Model, tea.Cmd) {
	m.err = ""
	m.message = ""
	m.logger.Info("tui command", "name", command.Name, "args", command.Args)
	switch command.Name {
	case commandSearch:
		if len(command.Args) > 0 {
			m.backStack = append(m.backStack, m.snapshot())
			m.filter = command.Args[0]
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
	case commandRefreshDetails:
		return m.refreshVisibleCitationDetails()
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
	default:
		m.err = "unknown command: " + command.Name
	}
	return m, nil
}

func (m Model) importGooglePatent(rawURL, verb string) (tea.Model, tea.Cmd) {
	if verb != importActionRefreshed {
		m.backStack = append(m.backStack, m.snapshot())
	}

	ctx, cancel := context.WithCancel(m.ctx)
	m.loading = true
	m.loadingMsg = fmt.Sprintf("%s%s from %s...", strings.ToUpper(verb[:1]), verb[1:], rawURL)
	m.cancel = cancel

	repo := m.repo
	logger := m.logger
	currentStatus := m.current.Status

	return m, tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			bundle, err := importer.ImportGooglePatents(rawURL)
			if err != nil {
				return refreshResultMsg{err: fmt.Errorf("import failed: %w", err)}
			}
			if verb == importActionRefreshed {
				bundle.Patent.Status = currentStatus
			} else {
				bundle.Patent.Status = domain.CitationStatusStored
			}
			if err := repo.UpsertPatentBundle(ctx, bundle); err != nil {
				return refreshResultMsg{err: fmt.Errorf("storage failed: %w", err)}
			}
			p, err := repo.GetPatent(ctx, bundle.Patent.Number)
			if err != nil {
				return refreshResultMsg{err: err}
			}

			logger.Info("google patent imported", "url", rawURL, "patent", bundle.Patent.Number)
			return refreshResultMsg{
				patent:  p,
				mode:    viewDetail,
				message: fmt.Sprintf("%s %s from %s", verb, bundle.Patent.Number, rawURL),
			}
		},
	)
}

func (m Model) refreshCommand(args []string) (tea.Model, tea.Cmd) {
	target := refreshTargetAll
	if len(args) > 1 {
		m.err = "usage: :refresh, :refresh citations, or :refresh citedby"
		return m, nil
	}
	if len(args) == 1 {
		target = strings.ToLower(args[0])
	}
	if target != refreshTargetAll && target != refreshTargetCitations && target != domain.RelationCites && target != refreshTargetCitedBy && target != domain.RelationCitedBy {
		m.err = "usage: :refresh, :refresh citations, or :refresh citedby"
		return m, nil
	}
	if m.current.Number == "" {
		m.err = "open a patent before refreshing citations"
		return m, nil
	}

	rawURL := m.current.SourceURL
	if rawURL == "" {
		var err error
		rawURL, err = importer.GooglePatentsURL(m.current.Number)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
	}

	ctx, cancel := context.WithCancel(m.ctx)
	m.loading = true
	m.loadingMsg = fmt.Sprintf("Refreshing %s...", m.current.Number)
	m.cancel = cancel

	// Capture state for the command
	repo := m.repo
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
			beforeCites, _ := repo.ListCitations(ctx, currentNumber, domain.RelationCites)
			beforeCitedBy, _ := repo.ListCitations(ctx, currentNumber, domain.RelationCitedBy)

			bundle, err := importer.ImportGooglePatents(rawURL)
			if err != nil {
				return refreshResultMsg{err: fmt.Errorf("import failed: %w", err)}
			}

			bundle.Patent.Status = currentStatus
			if err := repo.UpsertPatentBundle(ctx, bundle); err != nil {
				return refreshResultMsg{err: fmt.Errorf("storage failed: %w", err)}
			}

			p, err := repo.GetPatent(ctx, currentNumber)
			if err != nil {
				return refreshResultMsg{err: err}
			}

			afterCites, _ := repo.ListCitations(ctx, currentNumber, domain.RelationCites)
			afterCitedBy, _ := repo.ListCitations(ctx, currentNumber, domain.RelationCitedBy)

			msg := refreshResultMsg{
				patent: p,
				mode:   currentMode,
			}

			switch target {
			case refreshTargetCitedBy, domain.RelationCitedBy:
				msg.mode = viewCitedBy
				msg.citedBySelected = 0
				msg.message = fmt.Sprintf(text.T(TextMessageRefreshCitedBy), len(afterCitedBy), len(beforeCitedBy))
			case refreshTargetCitations, domain.RelationCites:
				msg.mode = viewCites
				msg.citesSelected = 0
				msg.message = fmt.Sprintf(text.T(TextMessageRefreshCitations), len(afterCites), len(beforeCites))
			default:
				msg.citedBySelected = clamp(citedBySelected, 0, max(0, len(afterCitedBy)-1))
				msg.citesSelected = clamp(citesSelected, 0, max(0, len(afterCites)-1))
				msg.message = fmt.Sprintf(text.T(TextMessageRefreshAll), len(afterCites), len(beforeCites), len(afterCitedBy), len(beforeCitedBy))
			}

			logger.Info("citations refreshed", "url", rawURL, "patent", currentNumber, domain.RelationCites, len(afterCites), domain.RelationCitedBy, len(afterCitedBy))
			return msg
		},
	)
}

func (m Model) refreshVisibleCitationDetails() (tea.Model, tea.Cmd) {
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
	logger := m.logger

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
				_, err := repo.GetPatent(ctx, edge.TargetPatent)
				exists := err == nil

				rawURL, err := importer.GooglePatentsURL(edge.TargetPatent)
				if err != nil {
					logger.Error("citation details url failed", "patent", edge.TargetPatent, "error", err)
					skippedCount++
					continue
				}
				bundle, err := importer.ImportGooglePatents(rawURL)
				if err != nil {
					logger.Error("citation details import failed", "patent", edge.TargetPatent, "error", err)
					skippedCount++
					continue
				}

				bundle.Patent.Status = domain.CitationStatusCached
				if err := repo.UpsertPatentBundle(ctx, bundle); err != nil {
					logger.Error("citation details storage failed", "patent", edge.TargetPatent, "error", err)
					return refreshDetailsResultMsg{err: err}
				}

				status := edge.Status
				if !exists {
					status = domain.CitationStatusIgnored
				}
				if err := repo.UpdateCitationStatus(ctx, edge, status); err != nil {
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

func (m Model) visibleCitationEdges() ([]domain.CitationEdge, error) {
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

func (m Model) refreshList() (tea.Model, tea.Cmd) {
	patents, err := m.repo.ListPatents(m.ctx, m.filter)
	if err != nil {
		m.err = err.Error()
		m.logger.Error("list patents failed", "filter", m.filter, "error", err)
		return m, nil
	}
	m.patents = patents
	if m.selected >= len(m.patents) {
		m.selected = max(0, len(m.patents)-1)
	}
	if len(patents) > 0 && m.current.Number == "" {
		m.current = patents[0]
	}
	return m, nil
}

func (m Model) reviewCommand(args []string) (tea.Model, tea.Cmd) {
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

func (m Model) openReviewQueue(status string) (tea.Model, tea.Cmd) {
	m.backStack = append(m.backStack, m.snapshot())
	m.mode = viewReview
	m.reviewStatus = status
	m.reviewSelected = 0
	m.err = ""
	m.message = ""
	return m, nil
}

func (m Model) openBrowser(args []string) (tea.Model, tea.Cmd) {
	rawURL, err := m.browserURL(args)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.message = ""
	m.err = ""
	m.logger.Info("open browser", "url", rawURL)
	return m, openBrowserCommand(rawURL)
}

func (m Model) browserURL(args []string) (string, error) {
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

func (m Model) patentURL(p domain.Patent) (string, error) {
	if strings.TrimSpace(p.SourceURL) != "" {
		return p.SourceURL, nil
	}
	return m.patentBrowserURL(p.Number)
}

func (m Model) patentBrowserURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New(m.text.T(TextMessageBrowserEmpty))
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value, nil
	}
	return importer.GooglePatentsURL(value)
}

func (m Model) filterBySelectedDetail() (tea.Model, tea.Cmd) {
	fields := m.detailFields()
	if len(fields) == 0 {
		return m, nil
	}
	selected := clamp(m.detailSelected, 0, len(fields)-1)
	field := fields[selected]
	switch field.action {
	case detailActionCitations:
		return m.navigateTo(viewCites), nil
	case detailActionCitedBy:
		return m.navigateTo(viewCitedBy), nil
	case detailActionClassification:
		return m.navigateTo(viewClassifications), nil
	case detailActionInventors:
		if len(m.current.Inventors) <= 1 {
			// If only one inventor, filter directly
			field.value = m.current.Inventors[0]
		} else {
			return m.navigateTo(viewInventors), nil
		}
	}
	if strings.TrimSpace(field.value) == "" || field.value == m.text.T(TextValueUnknown) {
		return m, nil
	}
	m.backStack = append(m.backStack, m.snapshot())
	m.filter = field.value
	m.mode = viewList
	model, cmd := m.refreshList()
	updated := model.(Model)
	updated.message = fmt.Sprintf(updated.text.T(TextMessageFilteredBy), updated.text.T(field.label), field.value)
	return updated, cmd
}

func (m Model) moveDetailSelection(delta int) Model {
	fields := m.detailFields()
	if len(fields) == 0 {
		return m
	}
	m.detailSelected = clamp(m.detailSelected+delta, 0, len(fields)-1)
	return m
}

func (m Model) openPatent(number string) (tea.Model, tea.Cmd) {
	m.backStack = append(m.backStack, m.snapshot())
	p, err := m.repo.GetPatent(m.ctx, number)
	if err != nil {
		m.backStack = m.backStack[:len(m.backStack)-1]
		m.err = err.Error()
		m.logger.Error("open patent failed", "patent", number, "error", err)
		return m, nil
	}
	m.current = p
	m.mode = viewDetail
	m.message = "opened " + p.Number
	return m, m.enrichClassificationDescriptionsCommand(number)
}

func (m Model) openSelectedCitation() (tea.Model, tea.Cmd) {
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
	if p, err := m.repo.GetPatent(m.ctx, target); err == nil {
		bundle.Patent = p
	} else {
		rawURL, err := importer.GooglePatentsURL(target)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		bundle, err = importer.ImportGooglePatents(rawURL)
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

func (m Model) selectedCitationEdge() (domain.CitationEdge, bool, error) {
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

func (m Model) storeSelectedCitation() (tea.Model, tea.Cmd) {
	edge, ok, err := m.selectedCitationEdge()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if !ok {
		return m, nil
	}
	if _, err := m.repo.GetPatent(m.ctx, edge.TargetPatent); err != nil {
		return m.openSelectedCitation()
	}
	if err := m.repo.UpdateCitationStatus(m.ctx, edge, domain.CitationStatusStored); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.message = fmt.Sprintf(m.text.T(TextMessageStoredPatent), edge.TargetPatent)
	return m, nil
}

func (m Model) storePendingPatent() (tea.Model, tea.Cmd) {
	if m.pendingBundle.Patent.Number == "" {
		return m, nil
	}
	m.pendingBundle.Patent.Status = domain.CitationStatusStored
	if err := m.repo.UpsertPatentBundle(m.ctx, m.pendingBundle); err != nil {
		m.err = err.Error()
		return m, nil
	}
	number := m.pendingBundle.Patent.Number
	if m.pendingCitation.TargetPatent != "" {
		if err := m.repo.UpdateCitationStatus(m.ctx, m.pendingCitation, domain.CitationStatusStored); err != nil {
			m.err = err.Error()
			return m, nil
		}
	}
	m.pendingBundle = domain.PatentBundle{}
	m.pendingCitation = domain.CitationEdge{}
	p, err := m.repo.GetPatent(m.ctx, number)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.current = p
	m.mode = viewDetail
	m.message = fmt.Sprintf(m.text.T(TextMessageStoredPatent), number)
	return m, nil
}

func (m Model) skipPendingPatent() (tea.Model, tea.Cmd) {
	number := m.pendingBundle.Patent.Number
	m.pendingBundle = domain.PatentBundle{}
	m.pendingCitation = domain.CitationEdge{}
	model, cmd := m.goBack()
	updated := model.(Model)
	if number != "" {
		updated.message = fmt.Sprintf(updated.text.T(TextMessageSkippedPatent), number)
	}
	return updated, cmd
}

func (m Model) updatePendingCitation(status string, messageKey TextKey) (tea.Model, tea.Cmd) {
	number := m.pendingBundle.Patent.Number
	if m.pendingCitation.TargetPatent != "" {
		if err := m.repo.UpdateCitationStatus(m.ctx, m.pendingCitation, status); err != nil {
			m.err = err.Error()
			return m, nil
		}
	}
	m.pendingBundle = domain.PatentBundle{}
	m.pendingCitation = domain.CitationEdge{}
	model, cmd := m.goBack()
	updated := model.(Model)
	if number != "" {
		updated.message = fmt.Sprintf(updated.text.T(messageKey), number)
	}
	return updated, cmd
}

func (m Model) updateSelectedCitationStatus(status string, messageKey TextKey) (tea.Model, tea.Cmd) {
	edge, ok, err := m.selectedCitationEdge()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if !ok {
		return m, nil
	}
	if err := m.repo.UpdateCitationStatus(m.ctx, edge, status); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.message = fmt.Sprintf(m.text.T(messageKey), edge.TargetPatent)
	return m, nil
}

func (m Model) currentReviewCitationEdges() ([]domain.CitationEdge, error) {
	if strings.TrimSpace(m.reviewStatus) == "" {
		return nil, nil
	}
	return m.repo.ListCitationsByStatus(m.ctx, m.reviewStatus)
}

func (m Model) selectedReviewCitationEdge() (domain.CitationEdge, bool, error) {
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

func (m Model) moveReviewSelection(delta int) Model {
	edges, err := m.currentReviewCitationEdges()
	if err != nil || len(edges) == 0 {
		return m
	}
	m.reviewSelected = clamp(m.reviewSelected+delta, 0, len(edges)-1)
	return m
}

func (m Model) openSelectedReviewCitation() (tea.Model, tea.Cmd) {
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
	if p, err := m.repo.GetPatent(m.ctx, target); err == nil {
		bundle.Patent = p
	} else {
		rawURL, err := importer.GooglePatentsURL(target)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		bundle, err = importer.ImportGooglePatents(rawURL)
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

func (m Model) storeSelectedReviewCitation() (tea.Model, tea.Cmd) {
	edge, ok, err := m.selectedReviewCitationEdge()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if !ok {
		return m, nil
	}
	if _, err := m.repo.GetPatent(m.ctx, edge.TargetPatent); err != nil {
		return m.openSelectedReviewCitation()
	}
	if err := m.repo.UpdateCitationStatus(m.ctx, edge, domain.CitationStatusStored); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.message = fmt.Sprintf(m.text.T(TextMessageStoredPatent), edge.TargetPatent)
	return m, nil
}

func (m Model) updateSelectedReviewCitationStatus(status string, messageKey TextKey) (tea.Model, tea.Cmd) {
	edge, ok, err := m.selectedReviewCitationEdge()
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	if !ok {
		return m, nil
	}
	if err := m.repo.UpdateCitationStatus(m.ctx, edge, status); err != nil {
		m.err = err.Error()
		return m, nil
	}
	if status != m.reviewStatus {
		edges, _ := m.currentReviewCitationEdges()
		m.reviewSelected = clamp(m.reviewSelected, 0, max(0, len(edges)-1))
	}
	m.message = fmt.Sprintf(m.text.T(messageKey), edge.TargetPatent)
	return m, nil
}

func (m Model) isCitationView() bool {
	return m.mode == viewCites || m.mode == viewCitedBy
}

func (m Model) moveCitationSelection(delta int) Model {
	edges, err := m.currentCitationEdges()
	if err != nil || len(edges) == 0 {
		return m
	}
	next := clamp(m.citationSelection()+delta, 0, len(edges)-1)
	m.setCitationSelection(next)
	return m
}

func (m Model) citationSelection() int {
	if m.mode == viewCitedBy {
		return m.citedBySelected
	}
	return m.citesSelected
}

func (m *Model) setCitationSelection(value int) {
	if m.mode == viewCitedBy {
		m.citedBySelected = value
		return
	}
	m.citesSelected = value
}

func (m Model) currentCitationEdges() ([]domain.CitationEdge, error) {
	if m.current.Number == "" {
		return nil, nil
	}
	relation := domain.RelationCites
	if m.mode == viewCitedBy {
		relation = domain.RelationCitedBy
	}
	return m.repo.ListCitations(m.ctx, m.current.Number, relation)
}

func (m Model) summarize() (tea.Model, tea.Cmd) {
	if m.current.Number == "" {
		m.err = "open a patent before summarizing"
		return m, nil
	}
	classifications, _ := m.repo.ListClassifications(m.ctx, m.current.Number)
	cites, _ := m.repo.ListCitations(m.ctx, m.current.Number, domain.RelationCites)

	// Convert Classification to ClassificationCode for AI summarizer compatibility if needed,
	// but better to update AI summarizer too.
	// For now, let's see what Summarize expects.
	artifact := ai.Summarize(m.current, classifications, cites)
	if _, err := m.repo.AddAIArtifact(m.ctx, artifact); err != nil {
		m.err = err.Error()
		m.logger.Error("summary artifact insert failed", "patent", m.current.Number, "error", err)
		return m, nil
	}
	m.mode = viewAI
	m.message = "created local summary"
	return m, nil
}

func (m Model) compare(otherNumber string) (tea.Model, tea.Cmd) {
	if m.current.Number == "" {
		m.err = "open a patent before comparing"
		return m, nil
	}
	other, err := m.repo.GetPatent(m.ctx, otherNumber)
	if err != nil {
		m.err = err.Error()
		m.logger.Error("comparison target open failed", "patent", otherNumber, "error", err)
		return m, nil
	}
	if _, err := m.repo.AddAIArtifact(m.ctx, ai.Compare(m.current, other)); err != nil {
		m.err = err.Error()
		m.logger.Error("comparison artifact insert failed", "patent", m.current.Number, "other", otherNumber, "error", err)
		return m, nil
	}
	m.mode = viewAI
	m.message = "created local comparison"
	return m, nil
}

func (m Model) refCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) != 1 {
		m.err = "usage: :ref add or :ref export"
		return m, nil
	}
	switch args[0] {
	case refActionAdd:
		if m.current.Number == "" {
			m.err = "open a patent before adding a reference"
			return m, nil
		}
		if _, err := m.repo.AddReference(m.ctx, m.current.Number, sqliterepo.CitationLabel(m.current)); err != nil {
			m.err = err.Error()
			m.logger.Error("reference insert failed", "patent", m.current.Number, "error", err)
			return m, nil
		}
		m.message = "reference added"
	case refActionExport:
		m.mode = viewRefs
		m.message = "Markdown reference export is shown below"
	default:
		m.err = "usage: :ref add or :ref export"
	}
	return m, nil
}

func (m Model) styleRow(index int, selected int, content string) string {
	style := lipgloss.NewStyle()
	if index == selected {
		style = style.Background(lipgloss.Color("236")) // Faint gray for selection
	} else if index%2 != 0 {
		style = style.Background(lipgloss.Color("233")) // Very dark gray for alternating
	}

	// Ensure the background spans the full width
	if m.width > 0 {
		contentWidth := lipgloss.Width(content)
		if contentWidth < m.width {
			content += strings.Repeat(" ", m.width-contentWidth)
		}
	}

	return style.Render(content)
}

func (m Model) View() string {
	bg := m.renderView()

	if m.mode == viewPreview || m.mode == viewConfirmDelete || m.mode == viewClassificationDetail || m.mode == viewClassifications || m.mode == viewInventors || m.mode == viewHelpPopup {
		var content string
		if m.mode == viewPreview {
			content = m.viewPreview()
		} else if m.mode == viewConfirmDelete {
			content = m.viewConfirmDelete()
		} else if m.mode == viewClassificationDetail {
			content = m.viewClassificationDetail()
		} else if m.mode == viewClassifications {
			content = m.viewClassifications()
		} else if m.mode == viewHelpPopup {
			content = m.viewHelpPopup()
		} else {
			content = m.viewInventors()
		}
		overlay := m.previewOverlay(content)

		// Dim the background to simulate transparency
		dimmedBg := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(bg)
		return m.composite(dimmedBg, overlay)
	}

	return bg
}

func (m Model) renderView() string {
	var b strings.Builder
	b.WriteString(m.renderScreenHeader())
	b.WriteString("\n")
	if m.input.Focused() {
		b.WriteString(m.input.View() + "\n")
	} else if m.jumpMode {
		b.WriteString(m.text.T(TextNavJump) + "\n")
	} else {
		b.WriteString(m.navDefault() + "\n")
	}
	b.WriteString(m.rule() + "\n")
	mode := m.activeMode()
	switch mode {
	case viewList, viewConfirmDelete: // Also render list in background for delete confirmation
		b.WriteString(m.viewList())
	case viewDetail, viewPreview: // Also render detail in background for preview
		b.WriteString(m.viewDetail())
	case viewCites:
		b.WriteString(m.viewCitations(domain.RelationCites))
	case viewCitedBy:
		b.WriteString(m.viewCitations(domain.RelationCitedBy))
	case viewClassifications, viewClassificationDetail:
		// If we are in Classification mode, we might want to show the underlying view in background.
		// Usually Classification mode is entered from Detail view.
		b.WriteString(m.viewDetail())
	case viewText:
		b.WriteString(m.viewText())
	case viewNotes:
		b.WriteString(m.viewNotes())
	case viewRefs:
		b.WriteString(m.viewRefs())
	case viewAI:
		b.WriteString(m.viewAI())
	case viewHelp:
		b.WriteString(m.viewHelp())
	case viewHelpPopup:
		b.WriteString(m.viewDetail())
	case viewReview:
		b.WriteString(m.viewReviewQueue())
	}
	if m.err != "" || m.message != "" {
		b.WriteString("\n" + m.rule() + "\n")
		if m.err != "" {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(m.singleLine(m.err)) + "\n")
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(m.singleLine(m.message)) + "\n")
		}
	}
	return b.String()
}

func (m Model) composite(bg, overlay string) string {
	bgLines := strings.Split(bg, "\n")
	overlayLines := strings.Split(overlay, "\n")

	// Ensure bg has enough height
	for len(bgLines) < m.height {
		bgLines = append(bgLines, "")
	}

	oWidth := lipgloss.Width(overlayLines[0])
	oHeight := len(overlayLines)

	startX := (m.width - oWidth) / 2
	startY := (m.height - oHeight) / 2

	if startX < 0 {
		startX = 0
	}
	if startY < 0 {
		startY = 0
	}

	for i, oLine := range overlayLines {
		y := startY + i
		if y >= len(bgLines) || y >= m.height {
			break
		}

		bgLine := bgLines[y]
		// Pad bgLine to m.width to ensure we can overwrite correctly
		bgLineWidth := lipgloss.Width(bgLine)
		if bgLineWidth < m.width {
			bgLine += strings.Repeat(" ", m.width-bgLineWidth)
		}

		// Truncate bg to startX
		left := lipgloss.NewStyle().MaxWidth(startX).Render(bgLine)
		// Since reverse truncation is hard with ANSI, we'll just fill the right side with spaces
		// for this prototype. Real terminal transparency is complex.
		right := strings.Repeat(" ", max(0, m.width-(startX+oWidth)))

		bgLines[y] = left + oLine + right
	}

	return strings.Join(bgLines, "\n")
}

func (m Model) rule() string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	return strings.Repeat("─", width)
}

func (m Model) singleLine(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if m.width <= 0 || len(value) <= m.width {
		return value
	}
	if m.width <= 3 {
		return value[:m.width]
	}
	return value[:m.width-3] + "..."
}

func (m Model) navDefault() string {
	return fmt.Sprintf(m.text.T(TextNavDefault), keyDown, keyUp, keyEnter, keyJump, keyCommand, keySearch, keyHelp, keyEsc, keyQuit)
}

func (m Model) viewList() string {
	if len(m.patents) == 0 {
		return m.text.T(TextListEmpty) + "\n"
	}
	var b strings.Builder
	if m.filter != "" {
		b.WriteString(m.text.T(TextListFilter) + ": " + m.filter + "\n\n")
	}

	numWidth := max(m.listNumberWidth(), 6)
	titleWidth := 40
	invWidth := 20
	cpcWidth := 15
	expWidth := 12
	statusWidth := 10

	// Account for jump prefix width in header if jump targets exist
	jumpPrefixWidth := 0
	if m.hasJumpTargets() {
		jumpPrefixWidth = 2
	}

	header := m.pad("  ", 2) +
		m.pad("", jumpPrefixWidth) +
		m.pad("Number", numWidth+2) +
		m.pad("Title", titleWidth+2) +
		m.pad("Inventor", invWidth+2) +
		m.pad("Classification", cpcWidth+2) +
		m.pad("Expires", expWidth+2) +
		m.pad("Status", statusWidth)

	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Underline(true).Render(header))
	b.WriteString("\n")

	for i, p := range m.patents {
		prefix := "  "
		if i == m.selected {
			prefix = "> "
		}

		jumpPrefix := m.jumpPrefix(i)
		if jumpPrefix == "" && jumpPrefixWidth > 0 {
			jumpPrefix = strings.Repeat(" ", jumpPrefixWidth)
		}

		title := m.truncate(p.Title, titleWidth)
		inventors := m.truncate(formatInventorsShort(p.Inventors), invWidth)
		cpc := m.truncate(p.ClassificationLabel, cpcWidth)
		if cpc == "" {
			cpc = "-"
		}
		expDate := p.ExpirationDate
		if expDate == "" {
			expDate = "-"
		}
		status := p.Status

		row := m.pad(prefix, 2) +
			m.pad(jumpPrefix, jumpPrefixWidth) +
			m.pad(p.Number, numWidth+2) +
			m.pad(title, titleWidth+2) +
			m.pad(inventors, invWidth+2) +
			m.pad(cpc, cpcWidth+2) +
			m.pad(expDate, expWidth+2) +
			m.pad(status, statusWidth)

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

func (m Model) pad(s string, width int) string {
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

func (m Model) truncate(s string, width int) string {
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

func (m Model) listNumberWidth() int {
	width := 0
	for _, patent := range m.patents {
		width = max(width, lipgloss.Width(patent.Number))
	}
	return width
}

func (m Model) viewDetail() string {
	p := m.current
	var b strings.Builder
	b.WriteString(p.Number + "\n")
	b.WriteString(p.Title + "\n\n")
	fields := m.detailFields()
	selected := clamp(m.detailSelected, 0, max(0, len(fields)-1))
	for i, field := range fields {
		prefix := "  "
		if i == selected {
			prefix = "> "
		}
		value := field.value
		if field.displayValue != "" {
			value = field.displayValue
		}
		b.WriteString(prefix + m.jumpPrefix(i) + m.detailRow(field.label, value))
	}
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(m.text.T(TextDetailOpenHint)))
	b.WriteString("\n")
	b.WriteString(p.Abstract + "\n")
	return b.String()
}

type detailField struct {
	label        TextKey
	value        string
	displayValue string
	jumpLabel    string
	action       detailAction
}

type detailAction int

const (
	detailActionNone detailAction = iota
	detailActionCitations
	detailActionCitedBy
	detailActionClassification
	detailActionInventors
)

func (m Model) detailFields() []detailField {
	p := m.current
	citationCount, citationRefreshedAt, citedByCount, citedByRefreshedAt := m.citationStats(p.Number)
	fields := []detailField{
		{label: TextDetailAssignee, value: p.Assignee, jumpLabel: jumpLabelAssignee},
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
		detailField{label: TextDetailPublication, value: p.PublicationDate, jumpLabel: jumpLabelPublication},
		detailField{label: TextDetailGrant, value: p.GrantDate, jumpLabel: jumpLabelGrant},
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
		detailField{label: TextDetailExpiration, value: m.formatExpiration(p), jumpLabel: jumpLabelExpiration},
		detailField{label: TextDetailStoredLocal, value: formatCitationTime(p.StoredAt, m.text.T(TextValueUnknown)), jumpLabel: jumpLabelStoredLocal},
		detailField{label: TextDetailCitationCount, value: m.formatCitationSummary(citationCount, citationRefreshedAt), jumpLabel: jumpLabelCitationCount, action: detailActionCitations},
		detailField{label: TextDetailCitedByCount, value: m.formatCitationSummary(citedByCount, citedByRefreshedAt), jumpLabel: jumpLabelCitedByCount, action: detailActionCitedBy},
		detailField{label: TextDetailSource, value: p.SourceURL, jumpLabel: jumpLabelSource},
	)
	return fields
}

func (m Model) citationStats(number string) (int, time.Time, int, time.Time) {
	if strings.TrimSpace(number) == "" || m.repo == nil {
		return 0, time.Time{}, 0, time.Time{}
	}
	citations, _ := m.repo.ListCitations(m.ctx, number, domain.RelationCites)
	citedBy, _ := m.repo.ListCitations(m.ctx, number, domain.RelationCitedBy)
	return len(citations), latestCitationRefresh(citations), len(citedBy), latestCitationRefresh(citedBy)
}

func (m Model) formatCitationSummary(count int, refreshedAt time.Time) string {
	refreshed := formatCitationTime(refreshedAt, m.text.T(TextCitationNeverRefreshed))
	return fmt.Sprintf("%d  %s: %s", count, m.text.T(TextCitationRefreshed), refreshed)
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

func jumpLabelForInventor(index int) string {
	if index == 0 {
		return jumpLabelInventors
	}
	numberIndex := index
	if numberIndex >= 0 && numberIndex < len(inventorJumpNumberLabels) {
		return string(inventorJumpNumberLabels[numberIndex])
	}
	return fallbackJumpLabels(index+1, []string{
		jumpLabelAssignee,
		jumpLabelInventors,
		jumpLabelPublication,
		jumpLabelGrant,
		jumpLabelExpiration,
		jumpLabelStoredLocal,
		jumpLabelSource,
	})[index]
}

func (m Model) detailRow(label TextKey, value string) string {
	if strings.TrimSpace(value) == "" {
		value = m.text.T(TextValueUnknown)
	}
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	return fmt.Sprintf("%-12s %s\n", labelStyle.Render(m.text.T(label)+":"), value)
}

func (m Model) formatExpiration(p domain.Patent) string {
	if p.ExpirationDate == "" {
		return m.text.T(TextValueUnknown)
	}
	label := p.ExpirationDate
	if p.ExpirationEstimated {
		label += " (est.)"
	}
	if p.IsExpired(time.Now()) {
		return lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("244")).Render(label)
	}
	return label
}

func (m Model) viewCitations(relation string) string {
	if m.current.Number == "" {
		return "Open a patent first.\n"
	}
	edges, err := m.repo.ListCitations(m.ctx, m.current.Number, relation)
	if err != nil {
		return err.Error() + "\n"
	}
	if len(edges) == 0 {
		return m.text.T(TextCitationsEmpty) + "\n"
	}
	selected := clamp(m.citationSelection(), 0, len(edges)-1)
	m.setCitationSelection(selected)
	window := pageWindow(selected, len(edges), m.pageSize())
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(pageStatus(m.text.T(TextValuePageStatus), window)))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(m.citationOpenHint()))
	b.WriteString("\n\n")

	indexWidth := 4
	numWidth := 14
	titleWidth := 40
	invWidth := 20
	expWidth := 12
	statusWidth := 10

	jumpPrefixWidth := 0
	if m.hasJumpTargets() {
		jumpPrefixWidth = 2
	}

	header := m.pad("  ", 2) +
		m.pad("", jumpPrefixWidth) +
		m.pad("#", indexWidth) +
		m.pad("Number", numWidth+2) +
		m.pad("Title", titleWidth+2) +
		m.pad("Inventor", invWidth+2) +
		m.pad("Expires", expWidth+2) +
		m.pad("Status", statusWidth)

	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Underline(true).Render(header))
	b.WriteString("\n")

	for i := window.Start; i < window.End; i++ {
		prefix := "  "
		if i == selected {
			prefix = "> "
		}

		jumpPrefix := m.jumpPrefix(i - window.Start)
		if jumpPrefix == "" && jumpPrefixWidth > 0 {
			jumpPrefix = strings.Repeat(" ", jumpPrefixWidth)
		}

		title := m.truncate(edges[i].TargetTitle, titleWidth)
		inventors := m.truncate(formatInventorsShort(edges[i].TargetInventors), invWidth)
		expDate := edges[i].TargetExpirationDate
		if expDate == "" {
			expDate = "-"
		}

		row := m.pad(prefix, 2) +
			m.pad(jumpPrefix, jumpPrefixWidth) +
			m.pad(rowIndexLabel(i), indexWidth) +
			m.pad(edges[i].TargetPatent, numWidth+2) +
			m.pad(title, titleWidth+2) +
			m.pad(inventors, invWidth+2) +
			m.pad(expDate, expWidth+2) +
			m.pad(m.citationStatusLabel(edges[i].Status), statusWidth)

		b.WriteString(m.styleRow(i, selected, row) + "\n")
	}
	return b.String()
}

func (m Model) viewReviewQueue() string {
	edges, err := m.currentReviewCitationEdges()
	if err != nil {
		return err.Error() + "\n"
	}
	if len(edges) == 0 {
		return m.text.T(TextReviewQueueEmpty) + "\n"
	}
	selected := clamp(m.reviewSelected, 0, len(edges)-1)
	m.reviewSelected = selected
	window := pageWindow(selected, len(edges), m.pageSize())
	var b strings.Builder
	b.WriteString(m.citationStatusLabel(m.reviewStatus) + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(pageStatus(m.text.T(TextValuePageStatus), window)))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(m.reviewOpenHint()))
	b.WriteString("\n\n")

	indexWidth := 4
	numWidth := 14
	titleWidth := 40
	invWidth := 20
	expWidth := 12
	sourceWidth := 14

	jumpPrefixWidth := 0
	if m.hasJumpTargets() {
		jumpPrefixWidth = 2
	}

	header := m.pad("  ", 2) +
		m.pad("", jumpPrefixWidth) +
		m.pad("#", indexWidth) +
		m.pad("Number", numWidth+2) +
		m.pad("Title", titleWidth+2) +
		m.pad("Inventor", invWidth+2) +
		m.pad("Expires", expWidth+2) +
		m.pad("Source", sourceWidth)

	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Underline(true).Render(header))
	b.WriteString("\n")

	for i := window.Start; i < window.End; i++ {
		prefix := "  "
		if i == selected {
			prefix = "> "
		}

		jumpPrefix := m.jumpPrefix(i - window.Start)
		if jumpPrefix == "" && jumpPrefixWidth > 0 {
			jumpPrefix = strings.Repeat(" ", jumpPrefixWidth)
		}

		title := m.truncate(edges[i].TargetTitle, titleWidth)
		inventors := m.truncate(formatInventorsShort(edges[i].TargetInventors), invWidth)
		expDate := edges[i].TargetExpirationDate
		if expDate == "" {
			expDate = "-"
		}

		row := m.pad(prefix, 2) +
			m.pad(jumpPrefix, jumpPrefixWidth) +
			m.pad(rowIndexLabel(i), indexWidth) +
			m.pad(edges[i].TargetPatent, numWidth+2) +
			m.pad(title, titleWidth+2) +
			m.pad(inventors, invWidth+2) +
			m.pad(expDate, expWidth+2) +
			m.pad(edges[i].SourcePatent, sourceWidth)

		b.WriteString(m.styleRow(i, selected, row) + "\n")
	}
	return b.String()
}

func (m Model) citationStatusLabel(status string) string {
	label := ""
	switch status {
	case domain.CitationStatusStored:
		label = m.text.T(TextCitationStored)
	case domain.CitationStatusIgnored:
		label = m.text.T(TextCitationIgnored)
	default:
		label = m.text.T(TextCitationUnderReview)
	}

	if color, ok := StatusColors[status]; ok {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(label)
	}
	return label
}

func (m Model) citationOpenHint() string {
	return fmt.Sprintf(m.text.T(TextValueOpenHint), keyEnter, keyYes, keyIgnore, keyUnreview, keyCtrlF, keyCtrlD)
}

func (m Model) reviewOpenHint() string {
	return fmt.Sprintf(m.text.T(TextValueReviewOpenHint), keyEnter, keyYes, keyIgnore, keyUnreview, keyWeb, keyCtrlF, keyCtrlD)
}

func (m Model) classificationOpenHint() string {
	return fmt.Sprintf(m.text.T(TextValueClassificationHint), keyEnter, keyCtrlF, keyCtrlD)
}

func (m Model) previewStorePrompt() string {
	return fmt.Sprintf(m.text.T(TextPreviewStorePrompt), keyYes, keyIgnore, keyUnreview, keyNo, keyEsc)
}

func formatCitationTime(value time.Time, fallback string) string {
	if value.IsZero() {
		return fallback
	}
	return value.Local().Format("2006-01-02 15:04")
}

func (m Model) viewPreview() string {
	p := m.pendingBundle.Patent
	if p.Number == "" {
		return m.text.T(TextValueUnknown) + "\n"
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(m.text.T(TextPreviewTitle)))
	b.WriteString("\n\n")
	b.WriteString(p.Number + "\n")
	b.WriteString(p.Title + "\n\n")
	b.WriteString(m.detailRow(TextDetailAssignee, p.Assignee))
	if len(p.Inventors) == 0 {
		b.WriteString(m.detailRow(TextDetailInventors, ""))
	} else {
		for i, inventor := range p.Inventors {
			b.WriteString(m.detailRow(TextDetailInventor, fmt.Sprintf("%d. %s", i+1, inventor)))
		}
	}
	b.WriteString(m.detailRow(TextDetailPublication, p.PublicationDate))
	b.WriteString(m.detailRow(TextDetailGrant, p.GrantDate))
	b.WriteString(m.detailRow(TextDetailExpiration, m.formatExpiration(p)))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Render(m.previewStorePrompt()))
	b.WriteString("\n\n")
	if strings.TrimSpace(p.Abstract) == "" {
		b.WriteString(m.text.T(TextPreviewNoAbstract) + "\n")
	} else {
		b.WriteString(p.Abstract + "\n")
	}
	return b.String()
}

func (m Model) previewOverlay(content string) string {
	width := m.overlayWidth()
	style := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("245")).
		Padding(1, 2).
		Width(width).
		Background(lipgloss.Color("235")) // Slightly lighter background for the box
	return style.Render(content)
}

func (m Model) overlayWidth() int {
	if m.width <= 0 {
		return 76
	}
	width := m.width * 4 / 5
	width = min(width, 96)
	width = max(width, 44)
	if width > m.width-4 {
		width = max(20, m.width-4)
	}
	return width
}

func (m Model) viewConfirmDelete() string {
	if m.selected < 0 || m.selected >= len(m.patents) {
		return ""
	}
	p := m.patents[m.selected]
	return fmt.Sprintf(m.text.T(TextDeleteConfirmPrompt), p.Number)
}

func (m Model) deleteSelectedPatent() (tea.Model, tea.Cmd) {
	if m.selected < 0 || m.selected >= len(m.patents) {
		m.mode = viewList
		return m, nil
	}
	p := m.patents[m.selected]
	if err := m.repo.DeletePatent(m.ctx, p.Number); err != nil {
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

	m.message = fmt.Sprintf(m.text.T(TextMessageDeletedPatent), p.Number)
	m.mode = viewList
	return m.refreshList()
}

func (m Model) hasJumpTargets() bool {
	return m.jumpTargetCount() > 0
}

func (m Model) jumpTargetCount() int {
	return len(m.jumpLabels())
}

func (m Model) jumpLabels() []string {
	switch {
	case m.mode == viewList:
		return fallbackJumpLabels(len(m.patents), nil)
	case m.mode == viewDetail:
		fields := m.detailFields()
		labels := make([]string, 0, len(fields))
		for _, field := range fields {
			labels = append(labels, field.jumpLabel)
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
		return fallbackJumpLabels(end-start, []string{preferred})
	case m.mode == viewReview:
		edges, err := m.currentReviewCitationEdges()
		if err != nil || len(edges) == 0 {
			return nil
		}
		start := pageStart(clamp(m.reviewSelected, 0, len(edges)-1), m.pageSize())
		end := min(start+m.pageSize(), len(edges))
		return fallbackJumpLabels(end-start, nil)
	default:
		return nil
	}
}

func (m Model) jumpPrefix(index int) string {
	labels := m.jumpLabels()
	if !m.jumpMode || index < 0 || index >= len(labels) {
		return ""
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")).Render(labels[index]) + " "
}

func (m Model) applyJump(key string) Model {
	if len(key) != 1 {
		return m
	}
	index := indexString(m.jumpLabels(), key)
	if index < 0 {
		return m
	}
	m.jumpMode = false
	switch {
	case m.mode == viewList:
		m.selected = index
	case m.mode == viewDetail:
		m.detailSelected = index
	case m.isCitationView():
		edges, err := m.currentCitationEdges()
		if err != nil || len(edges) == 0 {
			return m
		}
		start := pageStart(clamp(m.citationSelection(), 0, len(edges)-1), m.pageSize())
		m.setCitationSelection(clamp(start+index, 0, len(edges)-1))
	case m.mode == viewReview:
		edges, err := m.currentReviewCitationEdges()
		if err != nil || len(edges) == 0 {
			return m
		}
		start := pageStart(clamp(m.reviewSelected, 0, len(edges)-1), m.pageSize())
		m.reviewSelected = clamp(start+index, 0, len(edges)-1)
	}
	return m
}

func fallbackJumpLabels(count int, preferred []string) []string {
	if count <= 0 {
		return nil
	}
	labels := make([]string, 0, min(count, len(jumpFallbackLabels)+len(preferred)))
	used := map[string]bool{}
	for _, label := range preferred {
		if label == "" || used[label] {
			continue
		}
		labels = append(labels, label)
		used[label] = true
		if len(labels) == count {
			return labels
		}
	}
	for _, r := range jumpFallbackLabels {
		label := string(r)
		if used[label] {
			continue
		}
		labels = append(labels, label)
		used[label] = true
		if len(labels) == count {
			return labels
		}
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

func (m Model) pageSize() int {
	if m.height <= 0 {
		return 20
	}
	return max(5, m.height-8)
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
	m.countBuffer = ""
	if err != nil || count <= 0 {
		return defaultValue
	}
	return count
}

func (m Model) goToRow(index int) Model {
	if index <= 0 {
		index = 1
	}
	target := index - 1
	switch {
	case m.mode == viewList && len(m.patents) > 0:
		m.selected = clamp(target, 0, len(m.patents)-1)
	case m.mode == viewDetail:
		fields := m.detailFields()
		if len(fields) > 0 {
			m.detailSelected = clamp(target, 0, len(fields)-1)
		}
	case m.isCitationView():
		edges, err := m.currentCitationEdges()
		if err == nil && len(edges) > 0 {
			m.setCitationSelection(clamp(target, 0, len(edges)-1))
		}
	case m.mode == viewReview:
		edges, err := m.currentReviewCitationEdges()
		if err == nil && len(edges) > 0 {
			m.reviewSelected = clamp(target, 0, len(edges)-1)
		}
	case m.mode == viewClassifications:
		return m.goToClassification(index)
	case m.mode == viewInventors && len(m.current.Inventors) > 0:
		m.inventorSelected = clamp(target, 0, len(m.current.Inventors)-1)
	}
	return m
}

func (m Model) viewClassifications() string {
	classifications, err := m.repo.ListClassifications(m.ctx, m.current.Number)
	if err != nil {
		return err.Error() + "\n"
	}
	if len(classifications) == 0 {
		return m.text.T(TextValueEmpty) + "\n"
	}

	selected := clamp(m.classificationSelected, 0, len(classifications)-1)
	m.classificationSelected = selected
	window := pageWindow(selected, len(classifications), m.pageSize())

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(pageStatus(m.text.T(TextValuePageStatus), window)))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(m.classificationOpenHint()))
	b.WriteString("\n\n")

	indexWidth := 4
	codeWidth := 18
	descriptionWidth := max(20, m.width-indexWidth-codeWidth-8)

	header := m.pad("  ", 2) +
		m.pad("#", indexWidth) +
		m.pad("Code", codeWidth) +
		m.pad("Description", descriptionWidth)

	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Underline(true).Render(header))
	b.WriteString("\n")

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
		b.WriteString(m.styleRow(i, selected, row) + "\n")
	}
	return b.String()
}

func (m Model) viewClassificationDetail() string {
	classifications, err := m.repo.ListClassifications(m.ctx, m.current.Number)
	if err != nil || len(classifications) == 0 {
		return ""
	}
	selected := clamp(m.classificationSelected, 0, len(classifications)-1)
	cls := classifications[selected]

	var b strings.Builder
	b.WriteString(fmt.Sprintf("System: %s\n", cls.System))
	b.WriteString(fmt.Sprintf("Code: %s\n\n", cls.Code))

	if cls.System == "CPC" {
		b.WriteString("Hierarchy:\n")
		b.WriteString(fmt.Sprintf("  Section: %s\n", cls.Section))
		b.WriteString(fmt.Sprintf("  Class: %s\n", cls.Class))
		b.WriteString(fmt.Sprintf("  Subclass: %s\n", cls.Subclass))
		b.WriteString(fmt.Sprintf("  Group: %s\n", cls.MainGroup))
		b.WriteString(fmt.Sprintf("  Subgroup: %s\n\n", cls.Subgroup))
	} else if cls.System == "USPC" {
		b.WriteString("Hierarchy:\n")
		b.WriteString(fmt.Sprintf("  Class: %s\n", cls.Class))
		b.WriteString(fmt.Sprintf("  Subclass: %s\n\n", cls.Subclass))
	}

	b.WriteString("Description:\n")
	b.WriteString(cls.Description)
	return b.String()
}

func (m Model) moveClassificationSelection(delta int) Model {
	classifications, _ := m.repo.ListClassifications(m.ctx, m.current.Number)
	if len(classifications) == 0 {
		m.countBuffer = ""
		return m
	}
	m.classificationSelected = clamp(m.classificationSelected+delta, 0, len(classifications)-1)
	return m
}

func (m Model) goToClassification(index int) Model {
	classifications, _ := m.repo.ListClassifications(m.ctx, m.current.Number)
	if len(classifications) == 0 {
		return m
	}
	m.classificationSelected = clamp(index-1, 0, len(classifications)-1)
	return m
}

func (m Model) viewInventors() string {
	inventors := m.current.Inventors
	if len(inventors) == 0 {
		return "No inventors listed.\n"
	}

	selected := clamp(m.inventorSelected, 0, len(inventors)-1)
	m.inventorSelected = selected

	var b strings.Builder
	b.WriteString("Select an inventor to filter:\n\n")
	for i, inventor := range inventors {
		prefix := "  "
		if i == selected {
			prefix = "> "
		}
		row := fmt.Sprintf("%s%s", prefix, inventor)
		b.WriteString(m.styleRow(i, selected, row) + "\n")
	}
	return b.String()
}

func (m Model) moveInventorSelection(delta int) Model {
	inventors := m.current.Inventors
	if len(inventors) == 0 {
		return m
	}
	m.inventorSelected = clamp(m.inventorSelected+delta, 0, len(inventors)-1)
	return m
}

func (m Model) filterBySelectedInventor() (tea.Model, tea.Cmd) {
	inventors := m.current.Inventors
	if len(inventors) == 0 {
		m.mode = viewDetail
		return m, nil
	}
	selected := clamp(m.inventorSelected, 0, len(inventors)-1)
	inventor := inventors[selected]

	m.backStack = append(m.backStack, m.snapshot())
	m.filter = inventor
	m.mode = viewList
	model, cmd := m.refreshList()
	updated := model.(Model)
	updated.message = fmt.Sprintf(updated.text.T(TextMessageFilteredBy), updated.text.T(TextDetailInventor), inventor)
	return updated, cmd
}

func (m Model) viewText() string {
	sections, err := m.repo.ListTextSections(m.ctx, m.current.Number)
	if err != nil {
		return err.Error() + "\n"
	}
	var b strings.Builder
	for _, section := range sections {
		b.WriteString(fmt.Sprintf("[%s %d]\n%s\n\n", section.SectionType, section.Ordinal, section.Text))
	}
	if b.Len() == 0 {
		return "No text sections.\n"
	}
	return b.String()
}

func (m Model) viewNotes() string {
	notes, err := m.repo.ListNotes(m.ctx, m.current.Number)
	if err != nil {
		return err.Error() + "\n"
	}
	var b strings.Builder
	b.WriteString("Use :notes to view notes. This prototype stores notes through repository tests; note entry can be wired to an editor later.\n\n")
	for _, note := range notes {
		b.WriteString(fmt.Sprintf("- %s %s\n", note.CreatedAt.Format("2006-01-02"), note.Body))
	}
	return b.String()
}

func (m Model) viewRefs() string {
	refs, err := m.repo.ListReferences(m.ctx)
	if err != nil {
		return err.Error() + "\n"
	}
	if len(refs) == 0 {
		return "No reference entries.\n"
	}
	var b strings.Builder
	b.WriteString("## References\n\n")
	for _, ref := range refs {
		b.WriteString(fmt.Sprintf("- %s\n", ref.CitationLabel))
	}
	return b.String()
}

func (m Model) viewAI() string {
	artifacts, err := m.repo.ListAIArtifacts(m.ctx, m.current.Number)
	if err != nil {
		return err.Error() + "\n"
	}
	if len(artifacts) == 0 {
		return "No AI artifacts. Run :summarize or :compare US11611785B2.\n"
	}
	var b strings.Builder
	for _, artifact := range artifacts {
		label := artifact.ArtifactType
		if artifact.ComparedPatentNumber != "" {
			label += " vs " + artifact.ComparedPatentNumber
		}
		b.WriteString(fmt.Sprintf("[%s, %s]\n%s\n\n", label, artifact.Provider, artifact.Body))
	}
	return b.String()
}

func (m Model) viewHelp() string {
	return RenderHelp(m.text)
}

func (m Model) viewHelpPopup() string {
	return RenderContextHelp(m.text, m.activeMode())
}
