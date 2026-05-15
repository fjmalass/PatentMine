package tui

import (
	"os"
	"strings"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/logging"

	"github.com/charmbracelet/lipgloss"
)

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

	if os.Getenv(logging.EnvDebug) == "1" {
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
	} else if m.listSearchActive {
		searchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorWarning)).Italic(true)
		b.WriteString(lineStyle.Render(searchStyle.Render("search: /"+m.listSearchQuery+"█")) + "\n")
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
	case viewFullText:
		return m.viewFullText()
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
	case viewKeymap:
		return m.viewKeymap()
	case viewKeymapPopup:
		return m.viewKeymapPopup()
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
	case viewReviewStateSelect:
		return m.viewReviewStateSelect()
	case viewCountrySelect:
		return m.viewCountrySelect()
	case viewProjectTags:
		return m.viewProjectTags()
	case viewTagSelect:
		return m.viewTagSelect()
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
	if m.current.Number != "" {
		title += " · " + m.current.Number
	}
	return m.renderPopup(title, m.dateInput.View())
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
	return BuildHelperLine(m.activeKeys, m.text)
}

func (m *Model) isPopupMode() bool {
	return m.mode == viewClassifications ||
		m.mode == viewInventors ||
		m.mode == viewProjectEvents ||
		m.mode == viewProjectInvoices ||
		m.mode == viewProjectIDS ||
		m.mode == viewClassificationDetail ||
		m.mode == viewHelpPopup ||
		m.mode == viewKeymapPopup ||
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
