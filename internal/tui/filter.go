package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"patentmine/internal/domain"
)

var validStatusFilters = map[string]string{
	domain.CitationStatusStored:      domain.CitationStatusStored,
	domain.CitationStatusIgnored:     domain.CitationStatusIgnored,
	domain.CitationStatusUnderReview: domain.CitationStatusUnderReview,
	"under-review":                   domain.CitationStatusUnderReview, // user-friendly alias
	statusFilterNone:                 statusFilterNone,                 // no restriction = show all statuses
}

// statusFilterCommand handles :statusfilter <stored|ignored|under-review|none>.
func (m Model) statusFilterCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.err = "usage: :statusfilter <stored|ignored|under-review|none>"
		return m, nil
	}
	raw := strings.ToLower(args[0])
	canonical, ok := validStatusFilters[raw]
	if !ok {
		m.err = fmt.Sprintf("unknown status %q — valid values: stored, ignored, under-review, none", raw)
		return m, nil
	}
	m.statusFilter = canonical
	m.mode = viewList
	if canonical == statusFilterNone {
		m.message = "showing all patents (no status filter)"
	} else {
		m.message = "showing patents with status: " + canonical
	}
	return m.refreshList()
}

// classCommand handles :classfilter <cpc> [&& <cpc2> | || <cpc2>] and :classfilter clear.
func (m Model) classCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 || args[0] == "clear" {
		m.classFilters = nil
		m.classFilterOp = EmptyFilter
		m.classFilter = EmptyFilter
		m.filter = EmptyFilter
		m.message = "all filters cleared"
	} else {
		raw := strings.ToUpper(strings.Join(args, " "))
		op := "and"
		var parts []string
		if strings.Contains(raw, "||") {
			op = "or"
			parts = splitTrim(raw, "||")
		} else {
			parts = splitTrim(raw, "&&")
		}
		m.classFilters = parts
		m.classFilterOp = op
		if op == "or" {
			m.classFilter = strings.Join(parts, " || ")
		} else {
			m.classFilter = strings.Join(parts, " && ")
		}
		m.message = "filtering by classification: " + m.classFilter
	}
	m.mode = viewList
	return m.refreshList()
}

// inventorFilterCommand handles :inventorfilter <name> and :inventorfilter clear.
func (m Model) inventorFilterCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 || args[0] == "clear" {
		m.filter = EmptyFilter
		m.message = "inventor filter cleared"
	} else {
		m.filter = strings.Join(args, " ")
		m.message = "filtering by inventor: " + m.filter
	}
	m.mode = viewList
	return m.refreshList()
}

// sortCommand handles :sort <col>[,<col2>] [asc|desc].
func (m Model) sortCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.err = "usage: :sort <col>[,<col2>] [asc|desc]"
		return m, nil
	}

	cols := strings.SplitN(strings.ToLower(args[0]), ",", 2)
	order := domain.SortOrderAsc
	if len(args) > 1 {
		order = strings.ToLower(args[1])
	}
	if order != domain.SortOrderAsc && order != domain.SortOrderDesc {
		m.err = "invalid order: " + order
		return m, nil
	}

	col1 := normalizeSortCol(cols[0])
	if col1 == "" {
		m.err = "invalid sort column: " + cols[0]
		return m, nil
	}

	m.sortColumn = col1
	m.sortOrder = order
	m.sortColumn2 = ""
	m.sortOrder2 = ""

	if len(cols) == 2 {
		col2 := normalizeSortCol(cols[1])
		if col2 == "" {
			m.err = "invalid secondary sort column: " + cols[1]
			return m, nil
		}
		m.sortColumn2 = col2
		m.sortOrder2 = order
	}

	label := m.sortColumn
	if m.sortColumn2 != "" {
		label += "," + m.sortColumn2
	}
	m.message = fmt.Sprintf("sorting by %s %s", label, order)
	return m.refreshList()
}

// filterBySelectedDetail filters the patent list by the currently focused detail field.
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
	case detailActionFamily:
		return m.navigateTo(viewFamily), nil
	case detailActionInventors:
		if len(m.current.Inventors) <= 1 {
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

// filterBySelectedInventor filters the patent list by the inventor selected in the inventor popup.
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

// normalizeSortCol maps user-supplied column name aliases to canonical domain constants.
func normalizeSortCol(col string) string {
	switch strings.ToLower(col) {
	case "cpc":
		return domain.SortColumnClass
	case domain.SortColumnNumber, domain.SortColumnTitle, domain.SortColumnDate,
		domain.SortColumnStatus, domain.SortColumnAssignee, domain.SortColumnInventor,
		domain.SortColumnClass, domain.SortColumnExpiration:
		return strings.ToLower(col)
	}
	return ""
}

// splitTrim splits s by sep and trims whitespace from each part, discarding empty parts.
func splitTrim(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// nextCitesStatusFilter cycles through citation view status filters.
// "" (all) → stored → ignored → under_review → "" (all)
func nextCitesStatusFilter(current string) string {
	switch current {
	case "":
		return domain.CitationStatusStored
	case domain.CitationStatusStored:
		return domain.CitationStatusIgnored
	case domain.CitationStatusIgnored:
		return domain.CitationStatusUnderReview
	default:
		return ""
	}
}

// purgeCommand handles :purge ignored — deletes ignored records for the current project and vacuums.
func (m Model) purgeCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 || args[0] != "ignored" {
		m.err = "usage: :purge ignored"
		return m, nil
	}
	n, err := m.repo.PurgeIgnored(m.ctx, m.ProjectID)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.message = fmt.Sprintf("purged %d ignored records, database compacted", n)
	return m.refreshList()
}
