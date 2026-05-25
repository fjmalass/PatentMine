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
		{Action: ActionUIFocus, Entity: "patent", EntityID: "US10000000B2", Status: "observed", Metadata: map[string]any{"scope": "detail"}},
		{Action: ActionUIFocus, Entity: "patent", EntityID: "US10000000B2", Status: "observed", Metadata: map[string]any{"scope": "detail"}},
		{Action: ActionIDSEntrySave, Entity: "ids_entry", EntityID: "p-1/US10000000B2", Status: "committed", Metadata: map[string]any{"prior_status": "ignored", "status": "pending"}},
		{Action: ActionIDSEntrySave, Entity: "ids_entry", EntityID: "p-1/US10000000B2", Status: "committed", Metadata: map[string]any{"prior_status": "pending", "status": "submitted"}},
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
