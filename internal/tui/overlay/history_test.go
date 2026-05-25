package overlay

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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

func TestHistoryOverlayFreeFormFilter(t *testing.T) {
	records := []observability.Record{
		{Timestamp: time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC), Action: observability.ActionIDSEntrySave, Entity: "ids_entry", EntityID: "p-1/US10000001A1", Status: "committed", Metadata: map[string]any{"status": "submitted"}},
		{Timestamp: time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC), Action: observability.ActionProjectSwitch, Entity: "project", EntityID: "p-2", Status: "observed"},
	}
	h := NewHistoryOverlay(render.NewTheme(), records, map[string]string{"p-1": "IDS Project"})

	_, _, handled := h.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	if !handled || !h.filterMode {
		t.Fatalf("/ should enter filter mode")
	}
	for _, r := range []rune("submitted") {
		h.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd, handled := h.HandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled || h.filterMode || h.filterQuery != "submitted" {
		t.Fatalf("filter not applied: mode=%v query=%q", h.filterMode, h.filterQuery)
	}
	if len(h.records) != 1 || h.records[0].Action != observability.ActionIDSEntrySave {
		t.Fatalf("filtered records = %+v", h.records)
	}
	if cmd == nil {
		t.Fatal("filter application should emit telemetry message")
	}
	msg := cmd()
	filterMsg, ok := msg.(HistoryFilterAppliedMsg)
	if !ok || filterMsg.Query != "submitted" || filterMsg.ResultCount != 1 || filterMsg.TotalCount != 2 {
		t.Fatalf("telemetry msg = %#v", msg)
	}
}

func TestHistoryOverlayToggleSort(t *testing.T) {
	records := []observability.Record{
		{Timestamp: time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC), Action: observability.ActionProjectSwitch, Entity: "project", EntityID: "new", Status: "observed"},
		{Timestamp: time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC), Action: observability.ActionProjectSwitch, Entity: "project", EntityID: "old", Status: "observed"},
	}
	h := NewHistoryOverlay(render.NewTheme(), records, nil)
	if h.records[0].EntityID != "new" {
		t.Fatalf("default sort first = %q, want newest", h.records[0].EntityID)
	}

	_, cmd, handled := h.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(".")})
	if !handled || !h.sortAscending || h.records[0].EntityID != "old" {
		t.Fatalf("sort toggle failed: ascending=%v records=%+v", h.sortAscending, h.records)
	}
	msg := cmd()
	filterMsg, ok := msg.(HistoryFilterAppliedMsg)
	if !ok || !filterMsg.SortAscending {
		t.Fatalf("sort telemetry msg = %#v", msg)
	}
}
