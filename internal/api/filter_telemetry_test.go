package api

import (
	"net/url"
	"testing"

	"patentmine/internal/domain"
)

func TestTableQueryTelemetryFromURL(t *testing.T) {
	values := url.Values{}
	values.Set("search", "office action")
	values.Set("review_state", "under_review")
	values.Set("ids_status", "pending")
	values.Set("sort_column", "expires")

	got := tableQueryTelemetryFromURL(domain.TablePatents, values)
	if got.SearchTerms != 2 || got.FilterCount != 2 || got.SortCount != 1 || got.Complexity != 5 {
		t.Fatalf("telemetry = %+v, want search=2 filters=2 sort=1 complexity=5", got)
	}
	if len(got.FilterFields) != 2 || len(got.SortFields) != 1 {
		t.Fatalf("fields = filters %v sorts %v", got.FilterFields, got.SortFields)
	}
}

func TestTableQueryTelemetryFromView(t *testing.T) {
	view := domain.SavedTableView{
		ID:        "view-1",
		Name:      "Needs Attention",
		TableType: domain.TableIDSActivityHistory,
		Scope:     domain.TableViewScopeUser,
		ViewJSON: []byte(`{
			"search":"failed ids",
			"filters":{"status":["failed","needs_attention"],"activityType":["ids_submitted"]},
			"sort":[{"field":"activityDate","direction":"desc"}],
			"columns":{"visible":["activityDate","status"]}
		}`),
	}

	got := tableQueryTelemetryFromView(view)
	if got.SearchTerms != 2 || got.FilterCount != 3 || got.SortCount != 1 || got.ColumnCount != 2 || got.Complexity != 8 {
		t.Fatalf("telemetry = %+v, want search=2 filters=3 sort=1 columns=2 complexity=8", got)
	}
	if !got.HasSavedView || got.SavedViewID != "view-1" || got.SavedViewName != "Needs Attention" {
		t.Fatalf("saved view metadata missing: %+v", got)
	}
}
