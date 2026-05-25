package observability

import (
	"reflect"
	"testing"
)

func TestTableFilterMetadataMinimal(t *testing.T) {
	got := TableFilter{
		Source:    "tui.history_overlay",
		TableType: "ids_activity_history",
	}.Metadata()

	want := map[string]any{
		"source":       "tui.history_overlay",
		"table_type":   "ids_activity_history",
		"search_terms": 0,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("minimal metadata = %v, want %v", got, want)
	}
}

func TestTableFilterMetadataOmitsZeroCounts(t *testing.T) {
	got := TableFilter{
		Source:      "tui.history_overlay",
		TableType:   "ids_activity_history",
		SearchTerms: 2,
		Search:      "office action",
	}.Metadata()

	if _, ok := got["filter_count"]; ok {
		t.Errorf("filter_count must be omitted when zero, got %v", got["filter_count"])
	}
	if _, ok := got["sort_count"]; ok {
		t.Errorf("sort_count must be omitted when zero, got %v", got["sort_count"])
	}
	if _, ok := got["column_count"]; ok {
		t.Errorf("column_count must be omitted when zero, got %v", got["column_count"])
	}
	if _, ok := got["complexity"]; ok {
		t.Errorf("complexity must be omitted when zero, got %v", got["complexity"])
	}
	if _, ok := got["filter_fields"]; ok {
		t.Errorf("filter_fields must be omitted when empty, got %v", got["filter_fields"])
	}
	if _, ok := got["sort_fields"]; ok {
		t.Errorf("sort_fields must be omitted when empty, got %v", got["sort_fields"])
	}
	if got["search"] != "office action" {
		t.Errorf("search = %v, want \"office action\"", got["search"])
	}
	if got["search_terms"] != 2 {
		t.Errorf("search_terms = %v, want 2", got["search_terms"])
	}
}

func TestTableFilterMetadataOmitsEmptySearch(t *testing.T) {
	got := TableFilter{
		Source:    "http",
		TableType: "patents",
	}.Metadata()

	if _, ok := got["search"]; ok {
		t.Errorf("search must be omitted when empty, got %v", got["search"])
	}
}

func TestTableFilterMetadataKeepsRealValues(t *testing.T) {
	got := TableFilter{
		Source:       "http",
		TableType:    "patents",
		Search:       "office action",
		SearchTerms:  2,
		FilterCount:  2,
		SortCount:    1,
		ColumnCount:  3,
		Complexity:   8,
		FilterFields: []string{"review_state", "ids_status"},
		SortFields:   []string{"expires"},
	}.Metadata()

	want := map[string]any{
		"source":        "http",
		"table_type":    "patents",
		"search":        "office action",
		"search_terms":  2,
		"filter_count":  2,
		"sort_count":    1,
		"column_count":  3,
		"complexity":    8,
		"filter_fields": []string{"review_state", "ids_status"},
		"sort_fields":   []string{"expires"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("full metadata = %v, want %v", got, want)
	}
}
