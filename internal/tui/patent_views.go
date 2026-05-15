package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"patentmine/internal/storage"
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

	// Get classification stats via SQL
	stats, _ := m.repo.GetClassificationStats(m.ctx, m.ProjectID, cls.Code)

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

	body.WriteString(boldStyle.Render("Patents in Project:") + "\n")

	renderCount := func(row classificationDetailRow, val int, style lipgloss.Style) {
		prefix := "  "
		if row == m.classificationDetailSelected {
			prefix = "> "
		}
		line := fmt.Sprintf("%-13s %d", classDetailRowLabel[row]+":", val)
		if row == m.classificationDetailSelected {
			body.WriteString(style.Bold(true).Render(prefix+line) + "\n")
		} else {
			body.WriteString(style.Render(prefix+line) + "\n")
		}
	}

	renderCount(classDetailRowTotal, stats.Total, base)
	renderCount(classDetailRowStored, stats.Stored, base.Foreground(lipgloss.Color(ColorSuccess)))
	renderCount(classDetailRowUnderReview, stats.UnderReview, base.Foreground(lipgloss.Color(ColorWarning)))
	renderCount(classDetailRowIgnored, stats.Ignored, base.Foreground(lipgloss.Color(ColorDim)))
	renderCount(classDetailRowCached, stats.Cached, base)

	return m.renderPopup(fmt.Sprintf("Classification · %s", cls.Code), body.String())
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
		body.WriteString(lipgloss.NewStyle().Bold(true).Render(fmt.Sprintf("[%s %d]", cases.Title(language.Und, cases.NoLower).String(section.SectionType), section.Ordinal)) + "\n")
		body.WriteString(wrapText(section.Text, width) + "\n\n")
	}
	if body.Len() == 0 {
		return m.renderPopup("Full Text", "No text sections available. Re-import the patent to fetch full text.\n")
	}
	return m.renderPopup("Full Text · "+m.current.Number, body.String())
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
