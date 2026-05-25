package overlay

import (
	"testing"

	"patentmine/internal/observability"
	"patentmine/internal/tui/render"
)

func TestHistoryFilterActionDetails(t *testing.T) {
	_, details := historyIconAndDetails(render.NewTheme(), observability.Record{
		Action:   observability.ActionFilterApply,
		Entity:   "filter",
		EntityID: "ids_status:pending",
		Status:   "requested",
		Metadata: map[string]any{"filter": "ids_status:pending"},
	})
	if details != `Filter: "ids_status:pending"` {
		t.Fatalf("filter details = %q", details)
	}

	_, details = historyIconAndDetails(render.NewTheme(), observability.Record{
		Action:   observability.ActionFilterApply,
		Entity:   "filter",
		EntityID: "needle",
		Status:   "requested",
		Metadata: map[string]any{"search": "needle"},
	})
	if details != `Search: "needle"` {
		t.Fatalf("search details = %q", details)
	}
}
