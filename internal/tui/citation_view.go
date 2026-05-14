package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"patentmine/internal/domain"
	"patentmine/internal/storage"
)

func (m *Model) viewCitations(relation string) string {
	if m.current.Number == EmptyFilter {
		return m.renderPopup("Citations", "Open a patent first.\n")
	}
	opts := storage.ListCitationsOptions{
		SortColumn:        m.sortColumn,
		SortOrder:         m.sortOrder,
		ReviewStateFilter: m.citesReviewStateFilter,
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
	reviewStateWidth := 10

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
		{label: "ReviewState", width: reviewStateWidth, id: "review_state"},
	}
	cols = fitColumns(cols, m.overlayWidth()-4-(2+jumpPrefixWidth+indexWidth), map[string]int{
		"number":       12,
		"title":        18,
		"inventor":     10,
		"expires":      10,
		"review_state": 8,
	}, []string{"title", "inventor", "number", "review_state", "expires"})

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
		row += m.pad(m.citationReviewStateLabel(edge.ReviewState), cols[4].width)

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
	body.WriteString(m.citationReviewStateLabel(m.reviewState) + "\n")
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
	if m.selected < 0 || m.selected >= len(m.patents) {
		return ""
	}
	p := m.patents[m.selected]
	return m.renderPopupHeader(fmt.Sprintf(m.text.T(TextDeleteConfirmPrompt), p.Number))
}
