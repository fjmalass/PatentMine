package tui

import (
	"fmt"
	"strings"

	"patentmine/internal/storage"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

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

	rowWidth := max(40, m.overlayContentWidth())
	countWidth := 6 // "(NNN)"

	var b strings.Builder
	b.WriteString(m.renderPopupHeader("Inventors · " + m.current.Number))
	for i, inventor := range inventors {
		prefix := rowNoCursor
		if i == selected {
			prefix = rowCursor
		}
		cnt := countFor(inventor)
		countCol := fmt.Sprintf("(%d)", cnt)
		nameWidth := max(10, rowWidth-len(prefix)-overlayIndexWidth-countWidth-1)
		row := prefix + m.pad(rowIndexLabel(i), overlayIndexWidth) + m.pad(m.truncate(inventor, nameWidth), nameWidth) + " " + m.pad(countCol, countWidth)
		b.WriteString(m.styleRowOverlay(i, selected, row, rowWidth) + "\n")
	}
	return b.String()
}

func (m *Model) viewFullText() string {
	sections, err := m.repo.ListTextSections(m.ctx, m.ProjectID, m.current.Number)
	if err != nil {
		return m.renderPopup("Full Text", err.Error()+"\n")
	}
	if len(sections) == 0 {
		return m.renderPopup("Full Text", "No text sections available. Re-import the patent to fetch full text.\n")
	}

	width := m.overlayWidth() - 4
	var allLines []string
	for _, section := range sections {
		header := lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("[%s %d]", cases.Title(language.Und, cases.NoLower).String(section.SectionType), section.Ordinal))
		allLines = append(allLines, header)
		for _, l := range strings.Split(wrapText(section.Text, width), "\n") {
			allLines = append(allLines, l)
		}
		allLines = append(allLines, "")
	}

	pageH := m.pageSize() - 4
	if pageH < 1 {
		pageH = 1
	}
	totalPages := (len(allLines) + pageH - 1) / pageH
	if totalPages < 1 {
		totalPages = 1
	}
	maxScroll := (totalPages - 1) * pageH
	if m.fullTextScroll > maxScroll {
		m.fullTextScroll = maxScroll
	}
	if m.fullTextScroll < 0 {
		m.fullTextScroll = 0
	}

	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle))
	currentPage := (m.fullTextScroll / pageH) + 1
	title := fmt.Sprintf("Full Text · %s %s", m.current.Number, subtle.Render(fmt.Sprintf("(Page %d/%d)", currentPage, totalPages)))

	end := m.fullTextScroll + pageH
	if end > len(allLines) {
		end = len(allLines)
	}
	var body strings.Builder
	for _, l := range allLines[m.fullTextScroll:end] {
		body.WriteString(l + "\n")
	}
	return m.renderPopup(title, body.String())
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
	return m.renderHelp(FilterHelpSections(m.helpQuery, m.text))
}

func (m *Model) viewKeymap() string {
	return m.renderHelp(FilterHelpSections(m.helpQuery, m.text))
}

func (m *Model) renderHelp(sections []HelpSection) string {
	subtle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle))
	accent := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(m.screenColor()))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorTheme)).Bold(true)
	cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorAccent))
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorWarning))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorDim))

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

	// Search bar & Pagination
	var searchBar strings.Builder
	if m.helpSearchActive {
		searchBar.WriteString(accent.Render("/") + " " + m.helpQuery + "█")
	} else if m.helpQuery != "" {
		searchBar.WriteString(accent.Render("/") + " " + m.helpQuery + subtle.Render(" (esc to clear)"))
	} else {
		searchBar.WriteString(subtle.Render(m.text.T(TextHelpSearchHint)))
	}

	// Add pagination info
	totalLines := len(lines)
	if totalLines > 0 {
		currentPage := (m.helpScroll / pageH) + 1
		totalPages := (totalLines + pageH - 1) / pageH
		if totalPages < 1 {
			totalPages = 1
		}
		pageInfo := fmt.Sprintf(" (Page %d/%d)", currentPage, totalPages)
		searchBar.WriteString(subtle.Render(pageInfo))
	}
	b.WriteString(searchBar.String() + "\n\n")

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

func (m *Model) viewKeymapPopup() string {
	return RenderContextHelp(m.text, m.activeMode())
}

func (m *Model) handleViewDateEditKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

func (m *Model) handleViewFullTextKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyVimDown, keyArrowDown:
		m.fullTextScroll++
		return m, nil
	case keyVimUp, keyArrowUp:
		if m.fullTextScroll > 0 {
			m.fullTextScroll--
		}
		return m, nil
	case keyPgDown, keyCtrlF, keyCtrlD:
		m.fullTextScroll += m.pageSize() - 4
		return m, nil
	case keyPgUp, keyCtrlB, keyCtrlU:
		m.fullTextScroll -= m.pageSize() - 4
		if m.fullTextScroll < 0 {
			m.fullTextScroll = 0
		}
		return m, nil
	case keyEsc, keyBack:
		m.fullTextScroll = 0
		return m.goBack()
	}
	return m, nil
}

func (m *Model) handleViewHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.helpSearchActive {
		switch msg.String() {
		case keyEsc, keyBack:
			m.helpSearchActive = false
			m.helpQuery = ""
			m.helpScroll = 0
			return m, nil
		case keyBackspace, keyCtrlH:
			if len(m.helpQuery) > 0 {
				m.helpQuery = m.helpQuery[:len(m.helpQuery)-1]
				m.helpScroll = 0
			}
			return m, nil
		case keyEnter:
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
	case keySearch:
		m.helpSearchActive = true
		m.helpQuery = ""
		return m, nil
	case keyVimDown, keyArrowDown:
		m.helpScroll++
		return m, nil
	case keyVimUp, keyArrowUp:
		if m.helpScroll > 0 {
			m.helpScroll--
		}
		return m, nil
	case keyPgDown, keyCtrlF, keyCtrlD:
		m.helpScroll += m.pageSize() - 4
		return m, nil
	case keyPgUp, keyCtrlB, keyCtrlU:
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
	return m, nil
}
