package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"patentmine/internal/ai"
	"patentmine/internal/config"
	"patentmine/internal/domain"
	"patentmine/internal/importer"
	"patentmine/internal/storage"
	sqliterepo "patentmine/internal/storage/sqlite"
)

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
			m.listFilter.Text = command.Args[0]
		} else {
			m.listFilter.Text = EmptyFilter
		}
		m.setMode(viewList)
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
	case commandCountry:
		return m.countryCommand(command.Args)
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
	case commandFullText:
		m = m.navigateTo(viewFullText)
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
		return m.openReviewQueue(domain.ReviewStateIgnored)
	case commandUnderReview:
		return m.openReviewQueue(domain.ReviewStateUnderReview)
	case commandReview:
		return m.reviewCommand(command.Args)
	case commandBrowser, commandWeb:
		return m.openBrowser(command.Args)
	case commandHelp, commandHelpShort, keyHelp:
		m = m.navigateTo(viewHelp)
	case commandVersion:
		m.message = "PatentMine " + m.displayVersion()
	case commandKeymap:
		if len(command.Args) > 0 && command.Args[0] == "export" {
			if m.logger == nil {
				m.err = "no logger available"
				return m, nil
			}
			modePath := DefaultLogDir + "/" + keymapModeFile
			keyPath := DefaultLogDir + "/" + keymapKeyFile
			if err := exportKeymapCSV(modePath, keyPath); err != nil {
				m.err = fmt.Sprintf("failed to export keymap: %s", err)
			} else {
				m.logger.Info("keymap exported", "mode_file", modePath, "key_file", keyPath)
				m.message = fmt.Sprintf("keymap exported to %s and %s", modePath, keyPath)
			}
			return m, nil
		}
		m = m.navigateTo(viewKeymap)
	case commandProject:
		return m.projectCommand(command.Args)
	case commandTag:
		return m.tagCommand(command.Args)
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
	m.setMode(viewAI)
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
	m.setMode(viewAI)
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
		m.setMode(viewRefs)
		m.message = "Markdown reference export is shown below"
	default:
		m.err = "usage: :ref add or :ref export"
	}
	return m, nil
}

func (m *Model) patentDateCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) < 2 {
		m.err = "usage: :date app|pub|grant|exp <YYYY-MM-DD>"
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

	if m.mode == viewDateEdit {
		m.dateInput.Blur()
		if len(m.backStack) > 0 {
			m.backStack = m.backStack[:len(m.backStack)-1]
		}
		m.setMode(viewDetail)
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
