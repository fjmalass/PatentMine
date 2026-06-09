package observability

import (
	"context"
	"path/filepath"
	"testing"
)

func TestReadHistoryFeedPreservesMutationsAndCollapsesFocus(t *testing.T) {
	logsDir := filepath.Join(t.TempDir(), "logs")
	rt, err := Open(logsDir, "test", "test-version")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })

	records := []Record{
		{Action: ActionUIFocus, Entity: "patent", EntityID: "US10000000B2", Status: "observed", Attributes: map[string]any{"scope": "detail"}},
		{Action: ActionUIFocus, Entity: "patent", EntityID: "US10000000B2", Status: "observed", Attributes: map[string]any{"scope": "detail"}},
		{Action: ActionIDSEntrySave, Entity: "ids_entry", EntityID: "p-1/US10000000B2", Status: "committed", Attributes: map[string]any{"prior_status": "ignored", "status": "pending"}},
		{Action: ActionIDSEntrySave, Entity: "ids_entry", EntityID: "p-1/US10000000B2", Status: "committed", Attributes: map[string]any{"prior_status": "pending", "status": "submitted"}},
		{Action: ActionPatentSave, Entity: "patent", EntityID: "US10000000B2", Status: "committed"},
	}
	for _, record := range records {
		if err := rt.Activity.Record(context.Background(), record); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	feed, err := ReadHistoryFeed(logsDir, HistoryQuery{RawLimit: 10})
	if err != nil {
		t.Fatalf("ReadHistoryFeed: %v", err)
	}
	if feed.RawScanned != 5 {
		t.Fatalf("RawScanned = %d, want 5", feed.RawScanned)
	}
	if feed.Returned != 3 || len(feed.Records) != 3 {
		t.Fatalf("Returned = %d records = %d, want 3", feed.Returned, len(feed.Records))
	}
	if feed.Suppressed != 2 {
		t.Fatalf("Suppressed = %d, want 2", feed.Suppressed)
	}
}

func TestReadHistoryFeedIncludesIDSSaveRecord(t *testing.T) {
	logsDir := filepath.Join(t.TempDir(), "logs")
	rt, err := Open(logsDir, "daemon", "test-version")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })

	rec := Record{
		ID:         "2026-05-25-1779702888891050258-10",
		Action:     ActionIDSEntrySave,
		Entity:     "ids_entry",
		EntityID:   "p-1779646755967531735/US20080011946A1",
		Status:     "committed",
		Attributes: map[string]any{"prior_status": "ignored", "status": "pending"},
	}
	if err := rt.Activity.Record(context.Background(), rec); err != nil {
		t.Fatalf("Record: %v", err)
	}

	feed, err := ReadHistoryFeed(logsDir, HistoryQuery{RawLimit: 10})
	if err != nil {
		t.Fatalf("ReadHistoryFeed: %v", err)
	}
	if feed.Returned != 1 || len(feed.Records) != 1 {
		t.Fatalf("history feed returned %d records (%d slice), want 1", feed.Returned, len(feed.Records))
	}
	if feed.Records[0].Action != ActionIDSEntrySave || feed.Records[0].EntityID != rec.EntityID {
		t.Fatalf("history feed record = %+v, want IDS save", feed.Records[0])
	}
}

func TestHistoryFeedIncludesOfficeActionMutationsOnly(t *testing.T) {
	logsDir := filepath.Join(t.TempDir(), "logs")
	rt, err := Open(logsDir, "daemon", "test-version")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })

	for _, rec := range []Record{
		{Action: ActionOfficeActionGet, Entity: EntityOfficeAction, EntityID: "oa-1", Status: StatusObserved},
		{Action: ActionOfficeActionList, Entity: EntityOfficeAction, EntityID: "p1", Status: StatusObserved},
		{Action: ActionOfficeActionImport, Entity: EntityOfficeAction, EntityID: "oa-1", Status: StatusCommitted},
		{Action: ActionOfficeActionSaveNotes, Entity: EntityOfficeAction, EntityID: "oa-1", Status: StatusCommitted},
		{Action: ActionOfficeActionUpdate, Entity: EntityOfficeAction, EntityID: "oa-1", Status: StatusCommitted},
		{Action: ActionOfficeActionDelete, Entity: EntityOfficeAction, EntityID: "oa-1", Status: StatusCommitted},
		{Action: ActionOfficeActionAssignPatents, Entity: EntityOfficeAction, EntityID: "oa-1", Status: StatusCommitted},
		{Action: ActionOfficeActionReleasePatents, Entity: EntityOfficeAction, EntityID: "oa-1", Status: StatusCommitted},
		{Action: ActionOfficeActionReviewStatus, Entity: EntityOfficeAction, EntityID: "oa-1", Status: StatusCommitted},
	} {
		if err := rt.Activity.Record(context.Background(), rec); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	feed, err := ReadHistoryFeed(logsDir, HistoryQuery{RawLimit: 10})
	if err != nil {
		t.Fatalf("ReadHistoryFeed: %v", err)
	}
	if feed.Returned != 7 {
		t.Fatalf("Returned = %d, want 7", feed.Returned)
	}
	for _, rec := range feed.Records {
		switch rec.Action {
		case ActionOfficeActionImport, ActionOfficeActionSaveNotes, ActionOfficeActionUpdate, ActionOfficeActionDelete,
			ActionOfficeActionAssignPatents, ActionOfficeActionReleasePatents, ActionOfficeActionReviewStatus:
		default:
			t.Fatalf("history included raw-only action %s", rec.Action)
		}
	}
}
