package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
)

type tableQueryTelemetry struct {
	TableType      domain.TableType
	Search         string
	FilterFields   []string
	SortFields     []string
	SearchTerms    int
	FilterCount    int
	SortCount      int
	ColumnCount    int
	Complexity     int
	HasSavedView   bool
	SavedViewID    string
	SavedViewName  string
	SavedViewScope string
}

func tableQueryTelemetryFromURL(table domain.TableType, q url.Values) tableQueryTelemetry {
	search := strings.TrimSpace(q.Get("search"))
	fields := make([]string, 0, 8)
	add := func(field string, active bool) {
		if active {
			fields = append(fields, field)
		}
	}
	add("filter", q.Get("filter") != "")
	add("review_state", firstNonEmpty(q.Get("review_state"), q.Get("state")) != "")
	add("ids_status", q.Get("ids_status") != "")
	add("classification", firstNonEmpty(q.Get("classification"), q.Get("class")) != "")
	add("classification_code", q.Get("classification_code") != "")
	add("inventor", q.Get("inventor") != "")
	add("assignee", firstNonEmpty(q.Get("assignee"), q.Get("owner")) != "")
	add("component", q.Get("component") != "")
	add("since", q.Get("since") != "")

	sorts := make([]string, 0, 1)
	if sortColumn := strings.TrimSpace(q.Get("sort_column")); sortColumn != "" {
		sorts = append(sorts, sortColumn)
	}
	return buildTableQueryTelemetry(table, search, fields, sorts, 0)
}

func tableQueryTelemetryFromView(view domain.SavedTableView) tableQueryTelemetry {
	var body map[string]any
	_ = json.Unmarshal(view.ViewJSON, &body)
	search, _ := body["search"].(string)
	filterFields, filterCount := filterComplexity(body["filters"])
	sortFields, sortCount := sortComplexity(body["sort"])
	columnCount := columnComplexity(body["columns"])
	t := buildTableQueryTelemetry(view.TableType, search, filterFields, sortFields, columnCount)
	t.FilterCount = filterCount
	t.SortCount = sortCount
	t.Complexity = t.SearchTerms + filterCount + sortCount + columnCount
	t.HasSavedView = true
	t.SavedViewID = view.ID
	t.SavedViewName = view.Name
	t.SavedViewScope = string(view.Scope)
	return t
}

func buildTableQueryTelemetry(table domain.TableType, search string, filters, sorts []string, columns int) tableQueryTelemetry {
	searchTerms := len(strings.Fields(search))
	filterCount := len(filters)
	sortCount := len(sorts)
	return tableQueryTelemetry{
		TableType:    table,
		Search:       search,
		FilterFields: filters,
		SortFields:   sorts,
		SearchTerms:  searchTerms,
		FilterCount:  filterCount,
		SortCount:    sortCount,
		ColumnCount:  columns,
		Complexity:   searchTerms + filterCount + sortCount + columns,
	}
}

func filterComplexity(raw any) ([]string, int) {
	filters, ok := raw.(map[string]any)
	if !ok || len(filters) == 0 {
		return nil, 0
	}
	fields := make([]string, 0, len(filters))
	count := 0
	for field, value := range filters {
		fields = append(fields, field)
		switch v := value.(type) {
		case []any:
			if len(v) == 0 {
				count++
			} else {
				count += len(v)
			}
		case map[string]any:
			if len(v) == 0 {
				count++
			} else {
				count += len(v)
			}
		default:
			count++
		}
	}
	return fields, count
}

func sortComplexity(raw any) ([]string, int) {
	switch sorts := raw.(type) {
	case []any:
		fields := make([]string, 0, len(sorts))
		for _, item := range sorts {
			if object, ok := item.(map[string]any); ok {
				if field, ok := object["field"].(string); ok && strings.TrimSpace(field) != "" {
					fields = append(fields, field)
					continue
				}
			}
			fields = append(fields, "unknown")
		}
		return fields, len(sorts)
	case map[string]any:
		if field, ok := sorts["field"].(string); ok && strings.TrimSpace(field) != "" {
			return []string{field}, 1
		}
		return []string{"unknown"}, 1
	default:
		return nil, 0
	}
}

func columnComplexity(raw any) int {
	columns, ok := raw.(map[string]any)
	if !ok {
		return 0
	}
	count := 0
	for _, key := range []string{"visible", "order"} {
		if values, ok := columns[key].([]any); ok {
			count += len(values)
		}
	}
	return count
}

func (s *Server) observeTableQuery(r *http.Request, telemetry tableQueryTelemetry) {
	if telemetry.TableType == "" {
		return
	}
	metadata := telemetryMetadata(r, telemetry)
	if s.activityMetrics != nil {
		s.recordTableQueryMetrics(telemetry)
	}
	if s.activity != nil && telemetry.Complexity > 0 {
		_ = s.activity.Record(r.Context(), observability.Record{
			Action:   observability.ActionTableFilterApply,
			Entity:   "table_filter",
			EntityID: string(telemetry.TableType),
			Status:   "requested",
			Metadata: metadata,
		})
	}
}

func (s *Server) observeTableView(r *http.Request, action string, view domain.SavedTableView) {
	telemetry := tableQueryTelemetryFromView(view)
	metadata := telemetryMetadata(r, telemetry)
	metadata["view_id"] = view.ID
	metadata["view_name"] = view.Name
	metadata["scope"] = string(view.Scope)
	if s.activityMetrics != nil {
		s.recordTableViewMetrics(action, telemetry)
	}
	if s.activity != nil {
		status := "observed"
		if action == observability.ActionTableViewSave || action == observability.ActionTableViewDelete {
			status = "committed"
		}
		_ = s.activity.Record(r.Context(), observability.Record{
			Action:   action,
			Entity:   "table_view",
			EntityID: view.ID,
			Status:   status,
			Metadata: metadata,
		})
	}
}

func (s *Server) recordTableQueryMetrics(t tableQueryTelemetry) {
	base := "api.table_filter." + metricPart(string(t.TableType))
	s.activityMetrics.IncCounter(base+".query_total", 1)
	s.activityMetrics.IncCounter(base+".complexity_total", int64(t.Complexity))
	s.activityMetrics.IncCounter(base+".filter_total", int64(t.FilterCount))
	s.activityMetrics.IncCounter(base+".sort_total", int64(t.SortCount))
	s.activityMetrics.IncCounter(base+".search_terms_total", int64(t.SearchTerms))
	s.activityMetrics.SetGauge(base+".last_complexity", int64(t.Complexity))
	if strings.TrimSpace(t.Search) != "" {
		s.activityMetrics.IncCounter(base+".search_used_total", 1)
	}
	for _, field := range t.FilterFields {
		s.activityMetrics.IncCounter(base+".filter_field."+metricPart(field)+".total", 1)
	}
	for _, field := range t.SortFields {
		s.activityMetrics.IncCounter(base+".sort_field."+metricPart(field)+".total", 1)
	}
}

func (s *Server) recordTableViewMetrics(action string, t tableQueryTelemetry) {
	table := metricPart(string(t.TableType))
	actionPart := metricPart(strings.TrimPrefix(action, "table_view."))
	base := "api.table_view." + table + "." + actionPart
	s.activityMetrics.IncCounter(base+"_total", 1)
	s.activityMetrics.IncCounter(base+".complexity_total", int64(t.Complexity))
	s.activityMetrics.SetGauge(base+".last_complexity", int64(t.Complexity))
	if t.SavedViewID != "" {
		s.activityMetrics.IncCounter("api.table_view."+table+".view."+metricPart(t.SavedViewID)+"."+actionPart+"_total", 1)
	}
}

func telemetryMetadata(r *http.Request, t tableQueryTelemetry) map[string]any {
	metadata := observability.TableFilter{
		Source:       "http",
		TableType:    string(t.TableType),
		Search:       t.Search,
		SearchTerms:  t.SearchTerms,
		FilterCount:  t.FilterCount,
		SortCount:    t.SortCount,
		ColumnCount:  t.ColumnCount,
		Complexity:   t.Complexity,
		FilterFields: t.FilterFields,
		SortFields:   t.SortFields,
	}.Metadata()
	metadata["path"] = r.URL.Path
	metadata["query"] = r.URL.RawQuery
	if t.HasSavedView {
		metadata["saved_view_id"] = t.SavedViewID
		metadata["saved_view_name"] = t.SavedViewName
		metadata["saved_view_scope"] = t.SavedViewScope
	}
	return metadata
}

func metricPart(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range s {
		valid := unicode.IsLetter(r) || unicode.IsDigit(r)
		if valid {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	part := strings.Trim(b.String(), "_")
	if part == "" {
		return "unknown"
	}
	return part
}
