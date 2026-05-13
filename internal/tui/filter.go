package tui

import (
	"fmt"
	"strings"

	"patentmine/internal/domain"

	tea "github.com/charmbracelet/bubbletea"
)

var validReviewStateFilters = map[string]string{
	domain.ReviewStateStored:      domain.ReviewStateStored,
	domain.ReviewStateIgnored:     domain.ReviewStateIgnored,
	domain.ReviewStateUnderReview: domain.ReviewStateUnderReview,
	domain.ReviewStateCached:      domain.ReviewStateCached,
	"under-review":                domain.ReviewStateUnderReview,
	reviewStateFilterNone:         reviewStateFilterNone,
}

type FilterType string

const (
	FilterReviewState    FilterType = "review_state"
	FilterClassification FilterType = "classification"
	FilterInventor       FilterType = "inventor"
	FilterCountry        FilterType = "country"
	FilterClear          FilterType = "clear"
)

var filterAliases = map[string]FilterType{
	"review_state":  FilterReviewState,
	"status":        FilterReviewState,
	"classification": FilterClassification,
	"class":          FilterClassification,
	"cpc":            FilterClassification,
	"inventor":       FilterInventor,
	"country":        FilterCountry,
	"clear":          FilterClear,
	"none":           FilterClear,
	"reset":          FilterClear,
}

var SupportedFilters = map[FilterType]bool{
	FilterReviewState:    true,
	FilterClassification: true,
	FilterInventor:       true,
	FilterCountry:        true,
	FilterClear:          true,
}

func SupportedFilterTypes() []string {
	var list []string
	for ft := range SupportedFilters {
		list = append(list, string(ft))
	}
	return list
}

func SupportedFilterTypesString() string {
	return strings.Join(SupportedFilterTypes(), ", ")
}

func (f FilterType) String() string {
	return string(f)
}

func ParseFilterType(s string) (FilterType, bool) {
	key := strings.ToLower(strings.TrimSpace(s))
	if ft, ok := filterAliases[key]; ok {
		return ft, true
	}
	return "", false
}

// filterCommand is the unified :filter gateway.
// :filter review_state <stored|ignored|under-review|none>
// :filter class <cpc> [&& <cpc2> | || <cpc2>]
// :filter inventor <name>
// :filter clear  — resets all filters to defaults
func (m *Model) filterCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.err = fmt.Sprintf("usage: :filter <%s> ...", SupportedFilterTypesString())
		return m, nil
	}
	ft, ok := ParseFilterType(args[0])
	if !ok {
		m.err = fmt.Sprintf("unknown filter type '%q' - valid: [%s]", args[0], SupportedFilterTypesString())
	}
	switch ft {
	case FilterReviewState:
		return m.reviewStateFilterCommand(args[1:])
	case FilterClassification:
		return m.classCommand(args[1:])
	case FilterInventor:
		return m.inventorFilterCommand(args[1:])
	case FilterCountry:
		return m.countryFilterCommand(args[1:])
	case FilterClear:
		m.reviewStateFilter = domain.ReviewStateStored
		m.classFilters = nil
		m.classFilterOp = EmptyFilter
		m.classFilter = EmptyFilter
		m.countryFilter = EmptyFilter
		m.tagFilter = EmptyFilter
		m.filter = EmptyFilter
		m.message = "all filters cleared"
		m.setMode(viewList)
		return m.refreshList()
	default:
		m.err = fmt.Sprintf("internal error: unhandled filter type: '%s', only [%s] supported",
			args[0], SupportedFilterTypesString())
		return m, nil
	}
}

// reviewStateFilterCommand handles :filter review_state <stored|ignored|under-review|none>.
func (m *Model) reviewStateFilterCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.err = "usage: :filter review_state <stored|ignored|under-review|none>"
		return m, nil
	}
	raw := strings.ToLower(args[0])
	canonical, ok := validReviewStateFilters[raw]
	if !ok {
		m.err = fmt.Sprintf("unknown review state %q — valid values: stored, ignored, under-review, none", raw)
		return m, nil
	}
	m.reviewStateFilter = canonical
	m.setMode(viewList)
	if canonical == reviewStateFilterNone {
		m.message = "showing all patents (no review state filter)"
	} else {
		m.message = "showing patents with review state: " + canonical
	}
	return m.refreshList()
}

// classCommand handles :classfilter <cpc> [&& <cpc2> | || <cpc2>] and :classfilter clear.
func (m *Model) classCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 || args[0] == "clear" {
		m.classFilters = nil
		m.classFilterOp = EmptyFilter
		m.classFilter = EmptyFilter
		m.filter = EmptyFilter
		m.message = "all filters cleared"
	} else {
		raw := strings.ToUpper(strings.Join(args, " "))
		op := domain.FilterOpAnd
		var parts []string
		if strings.Contains(raw, "||") {
			op = domain.FilterOpOr
			parts = splitTrim(raw, "||")
		} else {
			parts = splitTrim(raw, "&&")
		}
		m.classFilters = parts
		m.classFilterOp = op
		if op == domain.FilterOpOr {
			m.classFilter = strings.Join(parts, " || ")
		} else {
			m.classFilter = strings.Join(parts, " && ")
		}
		m.message = "filtering by classification: " + m.classFilter
	}
	m.setMode(viewList)
	return m.refreshList()
}

// inventorFilterCommand handles :inventorfilter <name> and :inventorfilter clear.
func (m *Model) inventorFilterCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 || args[0] == "clear" {
		m.filter = EmptyFilter
		m.message = "inventor filter cleared"
	} else {
		m.filter = strings.Join(args, " ")
		m.message = "filtering by inventor: " + m.filter
	}
	m.setMode(viewList)
	return m.refreshList()
}

func (m *Model) countryFilterCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 || args[0] == "clear" {
		m.countryFilter = EmptyFilter
		m.message = "country filter cleared"
		m.setMode(viewList)
		return m.refreshList()
	}
	code := strings.ToUpper(strings.TrimSpace(args[0]))
	valid := false
	for _, supported := range domain.PatentCountryCodes {
		if code == supported {
			valid = true
			break
		}
	}
	if !valid {
		m.err = fmt.Sprintf("unknown country %q — valid values: %s", code, strings.Join(domain.PatentCountryCodes, ", "))
		return m, nil
	}
	m.countryFilter = code
	m.message = "filtering by country: " + code
	m.setMode(viewList)
	return m.refreshList()
}

// sortCommand handles :sort <col>[,<col2>] [asc|desc].
func (m *Model) sortCommand(args []string) (tea.Model, tea.Cmd) {
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
func (m *Model) filterBySelectedDetail() (tea.Model, tea.Cmd) {
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
	case detailActionNotes:
		return m.navigateTo(viewNotes), nil
	case detailActionAbstract:
		if m.current.Number != "" {
			return m.navigateTo(viewAbstract), nil
		}
		return m, nil
	case detailActionIDS:
		return m.openCurrentPatentIDSEdit(), nil
	case detailActionCountryFilter:
		code := strings.ToUpper(strings.TrimSpace(field.value))
		if code == "" || code == "-" {
			return m, nil
		}
		m.countryFilter = code
		m.reviewStateFilter = domain.ReviewStateStored
		m.message = fmt.Sprintf("showing stored patents from %s", code)
		m.setMode(viewList)
		return m.refreshList()
	case detailActionFirstClaim:
		if m.current.FirstClaim == "" {
			return m, nil
		}
		return m.navigateTo(viewClaim), nil
	case detailActionInventors:
		if len(m.current.Inventors) <= 1 {
			field.value = m.current.Inventors[0]
		} else {
			return m.navigateTo(viewInventors), nil
		}
	case detailActionEditDate:
		m.editDateType = field.data
		// Pre-fill current value if possible
		currentVal := ""
		switch field.data {
		case domain.LifecycleTypeApp:
			currentVal = m.current.ApplicationDate
		case domain.LifecycleTypePub:
			currentVal = m.current.PublicationDate
		case domain.LifecycleTypeGrant:
			currentVal = m.current.GrantDate
		case domain.LifecycleTypeExp:
			currentVal = m.current.ExpirationDate
		}
		m.dateInput.SetValue(currentVal)
		m.dateInput.Focus()
		m.dateInput.CursorEnd()
		return m.navigateTo(viewDateEdit), nil
	case detailActionStatic:
		return m, nil
	}
	if strings.TrimSpace(field.value) == "" || field.value == m.text.T(TextValueUnknown) {
		return m, nil
	}
	m.backStack = append(m.backStack, m.snapshot())
	m.filter = field.value
	m.setMode(viewList)
	model, cmd := m.refreshList()
	updated := model.(*Model)
	updated.message = fmt.Sprintf(updated.text.T(TextMessageFilteredBy), updated.text.T(field.label), field.value)
	return updated, cmd
}

// filterBySelectedInventor filters the patent list by the inventor selected in the inventor popup.
func (m *Model) filterBySelectedInventor() (tea.Model, tea.Cmd) {
	inventors := m.current.Inventors
	if len(inventors) == 0 {
		m.setMode(viewDetail)
		return m, nil
	}
	selected := clamp(m.inventorSelected, 0, len(inventors)-1)
	inventor := inventors[selected]

	m.backStack = append(m.backStack, m.snapshot())
	m.filter = inventor
	m.setMode(viewList)
	model, cmd := m.refreshList()
	updated := model.(*Model)
	updated.message = fmt.Sprintf(updated.text.T(TextMessageFilteredBy), updated.text.T(TextDetailInventor), inventor)
	return updated, cmd
}

// normalizeSortCol maps user-supplied column name aliases to canonical domain constants.
func normalizeSortCol(col string) string {
	switch strings.ToLower(col) {
	case "cpc":
		return domain.SortColumnClass
	case domain.SortColumnNumber, domain.SortColumnTitle, domain.SortColumnDate,
		domain.SortColumnReviewState, domain.SortColumnAssignee, domain.SortColumnInventor,
		domain.SortColumnClass, domain.SortColumnExpiration, domain.SortColumnUpdated,
		domain.SortColumnNotes, domain.SortColumnTags, domain.SortColumnIDS:
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

// nextCitesReviewStateFilter cycles through citation view review state filters.
// "" (all) → stored → ignored → under_review → "" (all)
func nextCitesReviewStateFilter(current string) string {
	switch current {
	case "":
		return domain.ReviewStateStored
	case domain.ReviewStateStored:
		return domain.ReviewStateIgnored
	case domain.ReviewStateIgnored:
		return domain.ReviewStateUnderReview
	default:
		return ""
	}
}

// purgeCommand handles :purge ignored — deletes ignored records for the current project and vacuums.
func (m *Model) purgeCommand(args []string) (tea.Model, tea.Cmd) {
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
