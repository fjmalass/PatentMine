package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"patentmine/internal/domain"
	"patentmine/internal/storage"
)

func (m *Model) viewDetail() string {
	p := m.current
	style := lipgloss.NewStyle().Width(m.width)
	var b strings.Builder
	b.WriteString(style.Bold(true).Render(p.Number) + "\n")
	b.WriteString(style.Render(p.Title) + "\n\n")
	b.WriteString(m.renderDetailFields(m.detailFields(), m.width, true))
	return b.String()
}

// renderDetailFields renders a detail field list at the given width.
// withJump includes jump-mode prefixes (used in full-screen viewDetail; omitted in popup).
func (m *Model) renderDetailFields(fields []detailField, width int, withJump bool) string {
	groupWidths := m.detailGroupWidths(fields)
	selected := clamp(m.detailSelected, 0, max(0, len(fields)-1))
	style := lipgloss.NewStyle().Width(width)
	sep := lipgloss.NewStyle().Width(width).Foreground(lipgloss.Color(ColorDim)).Render(strings.Repeat(sepRuleChar, width))
	groupIndex := 0
	var b strings.Builder
	for i, field := range fields {
		if field.separator {
			b.WriteString(sep + "\n")
			groupIndex++
			continue
		}
		prefix := rowNoCursor
		if i == selected {
			prefix = rowCursor
		}
		value := field.value
		if field.displayValue != "" {
			value = field.displayValue
		}
		lead := prefix
		if withJump {
			lead += m.jumpPrefix(i)
		}
		b.WriteString(style.Render(lead+m.detailRow(field.label, value, groupWidths[groupIndex], lipgloss.Width(lead))) + "\n")
	}
	b.WriteString(sep + "\n")
	return b.String()
}

func (m *Model) detailGroupWidths(fields []detailField) []int {
	var widths []int
	currentMax := 0
	for _, f := range fields {
		if f.separator {
			widths = append(widths, currentMax)
			currentMax = 0
		} else {
			w := lipgloss.Width(m.text.T(f.label) + ":")
			if w > currentMax {
				currentMax = w
			}
		}
	}
	return append(widths, currentMax)
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
	detailActionCountryFilter
	detailActionEditDate
	detailActionEditNumber
	detailActionStatic
	detailActionReviewState
	detailActionTags
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

	if code := patentCountryLabel(p); code != "-" {
		countryValue := code
		storedCount := m.storedPatentCountForCountry(code)
		countryDisplay := fmt.Sprintf("%s · %d stored", code, storedCount)
		fields = append(fields, detailField{
			label:        TextDetailCountry,
			value:        countryValue,
			displayValue: countryDisplay,
			action:       detailActionCountryFilter,
			jumpLabel:    jumpLabelCountry,
		})
	}

	fields = append(fields,
		detailField{label: TextDetailCitationCount, value: m.formatCitationSummary(cache.CitationCount, p.ExpectedCitations, cache.CitationRefreshedAt), jumpLabel: ksCitations.jump, action: detailActionCitations},
		detailField{label: TextDetailCitedByCount, value: m.formatCitationSummary(cache.CitedByCount, p.ExpectedCitedBy, cache.CitedByRefreshedAt), jumpLabel: ksCitedBy.jump, action: detailActionCitedBy},
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

		fields = append(fields,
			detailField{separator: true},
			detailField{label: TextDetailTags, value: formatTags(m.detailCache.Tags), jumpLabel: ksTags.jump, action: detailActionTags},
		)

		// Add Status and IDS
		if color, ok := ReviewStateColors[p.ReviewState]; ok {
			fields = append(fields, detailField{
				label:        TextDetailReviewState,
				displayValue: lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(p.ReviewState),
				jumpLabel:    keyReviewState,
				action:       detailActionReviewState,
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
			jumpLabel:    ksNotes.jump,
			action:       detailActionNotes,
		},
	)

	importSourceValue := p.ImportSource
	if importSourceValue == "" {
		importSourceValue = m.text.T(TextValueUnknown)
	}
	fields = append(fields,
		detailField{separator: true},
		detailField{label: TextDetailImportSource, value: importSourceValue, jumpLabel: jumpLabelImportSource},
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

func (m *Model) jumpLabelForInventor(idx int) string {
	if idx == 0 {
		return jumpLabelInventors
	}
	numberIndex := idx
	if numberIndex >= 0 && numberIndex < len(inventorJumpNumberLabels) {
		return string(inventorJumpNumberLabels[numberIndex])
	}
	return m.fallbackJumpLabels(idx+1, []string{
		jumpLabelAssignee,
		jumpLabelInventors,
		jumpLabelPublication,
		jumpLabelGrant,
		jumpLabelExpiration,
		jumpLabelStoredLocal,
		jumpLabelUpdated,
		jumpLabelSource,
	})[idx].key
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
	tags, _ := m.repo.GetPatentTags(m.ctx, m.ProjectID, p.Number)

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
		Tags:                tags,
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

const recentTimeThreshold = 24 * time.Hour

func formatDatetime(value time.Time) string {
	if time.Since(value) < recentTimeThreshold {
		return value.Local().Format(dateFmtDateTime)
	}
	return value.Local().Format(dateFmtDate)
}

func formatCitationTime(value time.Time, fallback string) string {
	if value.IsZero() {
		return fallback
	}
	return formatDatetime(value)
}

func formatStoredTime(value time.Time, fallback string) string {
	if value.IsZero() {
		return fallback
	}
	return formatDatetime(value)
}

// skipDetailSeparators advances idx in the direction of delta until it lands on
// a non-separator field, so j/k navigation in viewDetail skips visual dividers.
func (m *Model) skipDetailSeparators(idx, delta int) int {
	fields := m.detailFields()
	if len(fields) == 0 {
		return 0
	}
	dir := 1
	if delta < 0 {
		dir = -1
	}
	for idx >= 0 && idx < len(fields) && fields[idx].separator {
		idx += dir
	}
	return clamp(idx, 0, len(fields)-1)
}

func formatElapsedHint(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("updated in %dms", d.Milliseconds())
	}
	return fmt.Sprintf("updated in %.1fs", d.Seconds())
}
