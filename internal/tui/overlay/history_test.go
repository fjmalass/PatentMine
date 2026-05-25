package overlay

import (
	"strings"
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

func TestHistoryIDSSaveDetailsShowStatusTransition(t *testing.T) {
	_, details := historyIconAndDetails(render.NewTheme(), observability.Record{
		Action:   observability.ActionIDSEntrySave,
		Entity:   "ids_entry",
		EntityID: "p-1779646755967531735/US20080011946A1",
		Status:   "committed",
		Metadata: map[string]any{"prior_status": "ignored", "status": "pending"},
	})
	if !strings.Contains(details, "IDS ignored") || !strings.Contains(details, "pending") || !strings.Contains(details, "US20080011946A1") {
		t.Fatalf("IDS details = %q", details)
	}
}
