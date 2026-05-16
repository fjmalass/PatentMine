package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"patentmine/internal/domain"
	"patentmine/internal/storage"
)

const (
	citationColSource = "source"
	overlayIndexWidth  = 4  // index column width shared across all overlay list views
	listJumpWidth      = 2  // width reserved for jump-mode hint prefix (e.g. "a ")
	listRowPrefixWidth = 2  // width of rowNoCursor / rowCursor
	rowCursor          = "> "
	rowNoCursor        = "  "

	citColNumWidth    = 16
	citColInvWidth    = 20
	citColExpWidth    = 12
	citColStateWidth  = 10
	citColSourceWidth = 16
	citColTagsWidth   = 10

	citColMinNum    = 12
	citColMinTitle  = 20
	citColMinInv    = 10
	citColMinExp    = 10
	citColMinState  = 8
	citColMinSource = 12
	citColMinTags   = 8
)

func (m *Model) citationColumns(availableWidth int) []listColumn {
	fixed := citColNumWidth + citColInvWidth + citColExpWidth + citColStateWidth + citColTagsWidth
	pads := 5 * listColPad // 5 non-last cols each get listColPad gap
	titleWidth := max(citColMinTitle, availableWidth-fixed-pads)
	return fitColumns(
		[]listColumn{
			{label: colHeaderNumber, width: citColNumWidth, id: domain.SortColumnNumber, jumpLabel: jumpLabelPublication},
			{label: colHeaderTitle, width: titleWidth, id: domain.SortColumnTitle},
			{label: colHeaderInventor, width: citColInvWidth, id: domain.SortColumnInventor, jumpLabel: jumpLabelInventors},
			{label: colHeaderExpires, width: citColExpWidth, id: domain.SortColumnExpiration, jumpLabel: jumpLabelExpiration},
			{label: colHeaderReview, width: citColStateWidth, id: domain.SortColumnReviewState, jumpLabel: keyReviewState},
			{label: colHeaderTags, width: citColTagsWidth, id: domain.SortColumnTags, jumpLabel: ksTags.jump},
		},
		availableWidth,
		map[string]int{
			domain.SortColumnNumber:      citColMinNum,
			domain.SortColumnTitle:       citColMinTitle,
			domain.SortColumnInventor:    citColMinInv,
			domain.SortColumnExpiration:  citColMinExp,
			domain.SortColumnReviewState: citColMinState,
			domain.SortColumnTags:        citColMinTags,
		},
		[]string{
			domain.SortColumnTitle,
			domain.SortColumnInventor,
			domain.SortColumnNumber,
			domain.SortColumnReviewState,
			domain.SortColumnTags,
			domain.SortColumnExpiration,
		},
	)
}

func (m *Model) reviewOverlayColumns(availableWidth int) []listColumn {
	fixed := citColNumWidth + citColInvWidth + citColExpWidth + citColSourceWidth
	pads := 4 * listColPad
	titleWidth := max(citColMinTitle, availableWidth-fixed-pads)
	return fitColumns(
		[]listColumn{
			{label: colHeaderNumber, width: citColNumWidth, id: domain.SortColumnNumber, jumpLabel: jumpLabelPublication},
			{label: colHeaderTitle, width: titleWidth, id: domain.SortColumnTitle},
			{label: colHeaderInventor, width: citColInvWidth, id: domain.SortColumnInventor, jumpLabel: jumpLabelInventors},
			{label: colHeaderExpires, width: citColExpWidth, id: domain.SortColumnExpiration, jumpLabel: jumpLabelExpiration},
			{label: colHeaderSource, width: citColSourceWidth, id: citationColSource},
		},
		availableWidth,
		map[string]int{
			domain.SortColumnNumber:    citColMinNum,
			domain.SortColumnTitle:     citColMinTitle,
			domain.SortColumnInventor:  citColMinInv,
			domain.SortColumnExpiration: citColMinExp,
			citationColSource:          citColMinSource,
		},
		[]string{
			domain.SortColumnTitle,
			domain.SortColumnInventor,
			citationColSource,
			domain.SortColumnNumber,
			domain.SortColumnExpiration,
		},
	)
}

func (m *Model) viewCitations(relation string) string {
	if m.current.Number == EmptyFilter {
		return m.renderPopup("Citations", "Open a patent first.\n")
	}
	citationPopupTitle := func() string {
		if relation == domain.RelationCitedBy {
			return "Cited By · " + m.current.Number
		}
		return "Citations · " + m.current.Number
	}
	opts := storage.ListCitationsOptions{
		SortColumn:        m.sortColumn,
		SortOrder:         m.sortOrder,
		ReviewStateFilter: m.citesReviewStateFilter,
	}
	edges, err := m.repo.ListCitations(m.ctx, m.ProjectID, m.current.Number, relation, opts)
	if err != nil {
		return m.renderPopup(citationPopupTitle(), err.Error()+"\n")
	}
	if len(edges) == 0 {
		return m.renderPopup(citationPopupTitle(), m.text.T(TextCitationsEmpty)+"\n")
	}
	selected := clamp(m.citationSelection(), 0, len(edges)-1)
	m.setCitationSelection(selected)
	window := pageWindow(selected, len(edges), m.overlayPageSize())

	tagsByPatent, _ := m.repo.ListPatentTagsForProject(m.ctx, m.ProjectID)

	var body strings.Builder
	body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Render(pageStatus(m.text.T(TextValuePageStatus), window)))
	body.WriteString("\n\n")

	jumpPrefixWidth := 0
	if m.hasJumpTargets() {
		jumpPrefixWidth = listJumpWidth
	}
	cols := m.citationColumns(m.overlayContentWidth() - (listRowPrefixWidth + jumpPrefixWidth + overlayIndexWidth))

	header := m.pad(rowNoCursor, listRowPrefixWidth) +
		m.pad("", jumpPrefixWidth) +
		m.pad("#", overlayIndexWidth)

	if m.sortColumnIndex >= len(cols) {
		m.sortColumnIndex = len(cols) - 1
	}
	for i, c := range cols {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Underline(true)

		sortIndicator := ""
		if m.sortColumn == c.id {
			if m.sortOrder == domain.SortOrderDesc {
				sortIndicator = " ▾"
			} else {
				sortIndicator = " ▴"
			}
		}
		avail := c.width - lipgloss.Width(sortIndicator)
		if avail < 1 {
			avail = 1
		}
		displayLabel := c.label
		if lipgloss.Width(c.label) > avail {
			displayLabel = m.truncate(c.label, avail)
		}
		label := displayLabel + sortIndicator

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

		colWidth := c.width
		if jumpColLabel != "" {
			colWidth = max(colWidth, lipgloss.Width(jumpColLabel+" "+label))
		}

		padding := listColPad
		if i == len(cols)-1 {
			padding = 0
		}
		header += m.pad(jumpColPrefix+style.Render(label), colWidth+padding)
	}

	body.WriteString(header + "\n")

	for i := window.Start; i < window.End; i++ {
		prefix := rowNoCursor
		if i == selected {
			prefix = rowCursor
		}

		jumpPrefix := m.jumpPrefix(i - window.Start)
		if jumpPrefix == "" && jumpPrefixWidth > 0 {
			jumpPrefix = strings.Repeat(" ", jumpPrefixWidth)
		}

		edge := edges[i]
		expDate := edge.TargetExpirationDate
		if expDate == "" {
			expDate = "-"
		}
		tagsLabel := "-"
		if tags, ok := tagsByPatent[edge.TargetPatent]; ok {
			tagsLabel = formatTags(tags)
		}

		rowValues := map[string]string{
			domain.SortColumnNumber:      edge.TargetPatent,
			domain.SortColumnTitle:       edge.TargetTitle,
			domain.SortColumnInventor:    formatInventorsShort(edge.TargetInventors),
			domain.SortColumnExpiration:  expDate,
			domain.SortColumnReviewState: edge.ReviewState,
			domain.SortColumnTags:        tagsLabel,
		}

		rowStyle := m.overlayRowStyle(i, selected)
		if color, ok := ReviewStateColors[edge.ReviewState]; ok {
			if !m.isInSelection(i) && !(i == selected && m.isPopupSearchMode() && m.popupSearchQuery != "") {
				rowStyle = rowStyle.Foreground(lipgloss.Color(color))
			}
		}

		row := m.pad(prefix, listRowPrefixWidth) +
			m.pad(jumpPrefix, jumpPrefixWidth) +
			m.pad(rowIndexLabel(i), overlayIndexWidth)

		for j, c := range cols {
			padding := listColPad
			if j == len(cols)-1 {
				padding = 0
			}
			row += m.renderCell(rowValues[c.id], c.width, padding, m.popupSearchQuery)
		}

		w := m.overlayContentWidth()
		if rw := lipgloss.Width(row); rw < w {
			row += strings.Repeat(" ", w-rw)
		}
		body.WriteString(rowStyle.Render(row) + "\n")
	}

	return m.renderPopup(citationPopupTitle(), body.String())
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
	body.WriteString(m.citationReviewStateLabel(m.reviewState) + "\n")
	body.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Render(pageStatus(m.text.T(TextValuePageStatus), window)))
	body.WriteString("\n\n")

	jumpPrefixWidth := 0
	if m.hasJumpTargets() {
		jumpPrefixWidth = listJumpWidth
	}
	cols := m.reviewOverlayColumns(m.overlayContentWidth() - (listRowPrefixWidth + jumpPrefixWidth + overlayIndexWidth))

	header := m.pad(rowNoCursor, listRowPrefixWidth) +
		m.pad("", jumpPrefixWidth) +
		m.pad("#", overlayIndexWidth)

	if m.sortColumnIndex >= len(cols) {
		m.sortColumnIndex = len(cols) - 1
	}
	for i, c := range cols {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtle)).Underline(true)

		sortIndicator := ""
		if m.sortColumn == c.id {
			if m.sortOrder == domain.SortOrderDesc {
				sortIndicator = " ▾"
			} else {
				sortIndicator = " ▴"
			}
		}
		avail := c.width - lipgloss.Width(sortIndicator)
		if avail < 1 {
			avail = 1
		}
		displayLabel := c.label
		if lipgloss.Width(c.label) > avail {
			displayLabel = m.truncate(c.label, avail)
		}
		label := displayLabel + sortIndicator

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

		colWidth := c.width
		if jumpColLabel != "" {
			colWidth = max(colWidth, lipgloss.Width(jumpColLabel+" "+label))
		}

		padding := listColPad
		if i == len(cols)-1 {
			padding = 0
		}
		header += m.pad(jumpColPrefix+style.Render(label), colWidth+padding)
	}

	body.WriteString(header + "\n")

	for i := window.Start; i < window.End; i++ {
		prefix := rowNoCursor
		if i == selected {
			prefix = rowCursor
		}

		jumpPrefix := m.jumpPrefix(i - window.Start)
		if jumpPrefix == "" && jumpPrefixWidth > 0 {
			jumpPrefix = strings.Repeat(" ", jumpPrefixWidth)
		}

		edge := edges[i]
		expDate := edge.TargetExpirationDate
		if expDate == "" {
			expDate = "-"
		}

		rowValues := map[string]string{
			domain.SortColumnNumber:    edge.TargetPatent,
			domain.SortColumnTitle:     edge.TargetTitle,
			domain.SortColumnInventor:  formatInventorsShort(edge.TargetInventors),
			domain.SortColumnExpiration: expDate,
			citationColSource:          edge.SourcePatent,
		}

		row := m.pad(prefix, listRowPrefixWidth) +
			m.pad(jumpPrefix, jumpPrefixWidth) +
			m.pad(rowIndexLabel(i), overlayIndexWidth)

		for j, c := range cols {
			padding := listColPad
			if j == len(cols)-1 {
				padding = 0
			}
			row += m.pad(m.truncate(rowValues[c.id], c.width), c.width+padding)
		}

		body.WriteString(m.styleRowOverlay(i, selected, row, m.overlayContentWidth()) + "\n")
	}
	return m.renderPopup("Review Queue", body.String())
}

func (m *Model) citationReviewStateLabel(status string) string {
	label := ""
	switch status {
	case domain.ReviewStateStored:
		label = m.text.T(TextReviewStateStored)
	case domain.ReviewStateIgnored:
		label = m.text.T(TextReviewStateIgnored)
	case domain.ReviewStateCached:
		label = m.text.T(TextReviewStateCached)
	default:
		label = m.text.T(TextReviewStateUnderReview)
	}

	if color, ok := ReviewStateColors[status]; ok {
		return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(label)
	}
	return label
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
func (m *Model) viewConfirmDelete() string {
	if m.patentSelected < 0 || m.patentSelected >= len(m.patents) {
		return ""
	}
	p := m.patents[m.patentSelected]
	return m.renderPopupHeader(fmt.Sprintf(m.text.T(TextDeleteConfirmPrompt), p.Number))
}
