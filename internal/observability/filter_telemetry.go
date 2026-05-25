package observability

// TableFilter carries the shared shape of a "table filter applied" activity
// record. Both the HTTP API and the TUI build records around the same core
// signals; this builder keeps them in sync and prevents callers from filling
// the metadata map with hardcoded zeros to mimic each other.
//
// Required fields: Source, TableType. SearchTerms is always emitted (zero is a
// real value — no search). Everything else is omit-when-zero / omit-when-empty
// so a caller that has no column filters does not pollute the log with
// "filter_count": 0.
type TableFilter struct {
	Source       string
	TableType    string
	Search       string
	SearchTerms  int
	FilterCount  int
	SortCount    int
	ColumnCount  int
	Complexity   int
	FilterFields []string
	SortFields   []string
}

// Metadata renders the builder as an activity record metadata map. Callers may
// merge additional keys (e.g. HTTP path, TUI result_count) on top of the
// returned map.
func (t TableFilter) Metadata() map[string]any {
	m := map[string]any{
		"source":       t.Source,
		"table_type":   t.TableType,
		"search_terms": t.SearchTerms,
	}
	if t.Search != "" {
		m["search"] = t.Search
	}
	if t.FilterCount > 0 {
		m["filter_count"] = t.FilterCount
	}
	if t.SortCount > 0 {
		m["sort_count"] = t.SortCount
	}
	if t.ColumnCount > 0 {
		m["column_count"] = t.ColumnCount
	}
	if t.Complexity > 0 {
		m["complexity"] = t.Complexity
	}
	if len(t.FilterFields) > 0 {
		m["filter_fields"] = t.FilterFields
	}
	if len(t.SortFields) > 0 {
		m["sort_fields"] = t.SortFields
	}
	return m
}
