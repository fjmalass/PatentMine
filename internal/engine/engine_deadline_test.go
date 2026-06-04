package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/remind"
)

type fakeNotifier struct {
	sent [][]remind.Reminder
}

func (f *fakeNotifier) Send(_ context.Context, rs []remind.Reminder) error {
	f.sent = append(f.sent, rs)
	return nil
}
func (f *fakeNotifier) Channel() string { return "test" }
func (f *fakeNotifier) Enabled() bool   { return true }

func TestSendDueRemindersFiresOnceThenDedups(t *testing.T) {
	ctx := context.Background()
	eng, repo := newTestEngine(t, nil)
	fake := &fakeNotifier{}
	WithNotifier(fake)(eng)

	now := time.Now().UTC()
	// Due in 5 days → within the 7-day threshold.
	d := domain.Deadline{
		ID: "dl-1", Kind: domain.DeadlineMaintenanceFee, PatentNumber: "US10000000B2",
		Title: "maintenance fee", DueDate: now.AddDate(0, 0, 5), Status: domain.DeadlinePending,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.SaveDeadline(ctx, d); err != nil {
		t.Fatalf("SaveDeadline: %v", err)
	}

	n, err := eng.SendDueReminders(ctx)
	if err != nil {
		t.Fatalf("SendDueReminders: %v", err)
	}
	if n != 1 || len(fake.sent) != 1 || len(fake.sent[0]) != 1 {
		t.Fatalf("first send: n=%d sent=%v, want 1 reminder", n, fake.sent)
	}

	// A second run is deduped — the 7-day reminder was already sent.
	n2, err := eng.SendDueReminders(ctx)
	if err != nil {
		t.Fatalf("SendDueReminders 2: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("second send: n=%d, want 0 (deduped)", n2)
	}
}

func TestSendDueRemindersNoopWhenDisabled(t *testing.T) {
	ctx := context.Background()
	eng, repo := newTestEngine(t, nil)
	// Default notifier is the no-op (disabled).
	now := time.Now().UTC()
	_ = repo.SaveDeadline(ctx, domain.Deadline{
		ID: "dl-x", Kind: domain.DeadlineCustom, Title: "x", DueDate: now.AddDate(0, 0, 1),
		Status: domain.DeadlinePending, CreatedAt: now, UpdatedAt: now,
	})
	n, err := eng.SendDueReminders(ctx)
	if err != nil || n != 0 {
		t.Fatalf("disabled notifier should send 0 (n=%d err=%v)", n, err)
	}
}

func TestListDeadlinesMergesOfficeActionResponse(t *testing.T) {
	ctx := context.Background()
	eng, proj := newDraftEngine(t, nil)

	src := filepath.Join(t.TempDir(), "oa.txt")
	if err := os.WriteFile(src, []byte("rejection"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A non-final OA seeds a response deadline 3 months out, status open.
	if _, err := eng.ImportOfficeAction(ctx, ImportOfficeActionInput{
		Project: proj, Type: domain.OANonFinal, MailDate: time.Now().UTC(), SourcePath: src,
	}); err != nil {
		t.Fatalf("ImportOfficeAction: %v", err)
	}

	deadlines, err := eng.ListDeadlines(ctx)
	if err != nil {
		t.Fatalf("ListDeadlines: %v", err)
	}
	var found bool
	for _, d := range deadlines {
		if d.Kind == domain.DeadlineOAResponse && d.OfficeActionID != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a synthesized OA-response deadline, got %+v", deadlines)
	}
}

func TestTrackAndUntrackRenewals(t *testing.T) {
	ctx := context.Background()
	eng, repo := newTestEngine(t, nil)

	// Save a patent in the repository so TrackRenewals can load it.
	p := domain.Patent{
		Number:     domain.PatentNumber{Country: "US", Serial: "10000000", Kind: "B2"},
		Title:      "Test Patent",
		GrantDate:  time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC),
		FetchState: domain.FetchCached,
	}
	if err := repo.SavePatent(ctx, p); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}

	// 1. Track renewals with custom entity size
	deadlines, err := eng.TrackRenewals(ctx, "US10000000B2", "small")
	if err != nil {
		t.Fatalf("TrackRenewals: %v", err)
	}
	if len(deadlines) != 3 {
		t.Fatalf("got %d deadlines, want 3", len(deadlines))
	}

	// Verify renewal config exists and is_tracked is true, and entity size is small
	ren, err := repo.PatentRenewal(ctx, p.Number)
	if err != nil {
		t.Fatalf("PatentRenewal: %v", err)
	}
	if !ren.IsTracked {
		t.Error("expected IsTracked to be true")
	}
	if ren.EntitySize != "small" {
		t.Errorf("expected EntitySize to be small, got %q", ren.EntitySize)
	}

	// 2. Track renewals again with empty entity size -> preserves existing size
	_, err = eng.TrackRenewals(ctx, "US10000000B2", "")
	if err != nil {
		t.Fatalf("TrackRenewals empty: %v", err)
	}
	renAfter, err := repo.PatentRenewal(ctx, p.Number)
	if err != nil {
		t.Fatalf("PatentRenewal: %v", err)
	}
	if renAfter.EntitySize != "small" {
		t.Errorf("expected EntitySize to remain small, got %q", renAfter.EntitySize)
	}

	// 3. Track renewals with invalid entity size -> error
	_, err = eng.TrackRenewals(ctx, "US10000000B2", "invalid")
	if err == nil {
		t.Fatal("expected error tracking with invalid entity size")
	}

	// 4. Untrack renewals
	if err := eng.UntrackRenewals(ctx, "US10000000B2"); err != nil {
		t.Fatalf("UntrackRenewals: %v", err)
	}

	// Verify deadlines are gone
	listed, err := eng.DeadlinesForPatent(ctx, "US10000000B2")
	if err != nil {
		t.Fatalf("DeadlinesForPatent: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected 0 deadlines after untrack, got %d", len(listed))
	}

	// Verify renewal config is updated to is_tracked = false
	ren2, err := repo.PatentRenewal(ctx, p.Number)
	if err != nil {
		t.Fatalf("PatentRenewal: %v", err)
	}
	if ren2.IsTracked {
		t.Error("expected IsTracked to be false after untrack")
	}
}

func TestRenewalReminderEntitySize(t *testing.T) {
	ctx := context.Background()
	eng, repo := newTestEngine(t, nil)
	fake := &fakeNotifier{}
	WithNotifier(fake)(eng)

	// Save patent & renewal configuration
	p := domain.Patent{
		Number:     domain.PatentNumber{Country: "US", Serial: "10000000", Kind: "B2"},
		Title:      "Test Patent",
		GrantDate:  time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC),
		FetchState: domain.FetchCached,
	}
	if err := repo.SavePatent(ctx, p); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}

	_, err := eng.TrackRenewals(ctx, "US10000000B2", "micro")
	if err != nil {
		t.Fatalf("TrackRenewals: %v", err)
	}

	// Update the deadlines' due dates to make one due soon (e.g. 5 days from now)
	deadlines, err := eng.DeadlinesForPatent(ctx, "US10000000B2")
	if err != nil {
		t.Fatalf("DeadlinesForPatent: %v", err)
	}
	if len(deadlines) == 0 {
		t.Fatal("expected deadlines")
	}

	now := time.Now().UTC()
	deadlines[0].DueDate = now.AddDate(0, 0, 5)
	if err := repo.SaveDeadline(ctx, deadlines[0]); err != nil {
		t.Fatalf("SaveDeadline: %v", err)
	}

	// Trigger due reminders
	_, err = eng.SendDueReminders(ctx)
	if err != nil {
		t.Fatalf("SendDueReminders: %v", err)
	}

	if len(fake.sent) != 1 || len(fake.sent[0]) != 1 {
		t.Fatalf("expected 1 reminder sent, got %v", fake.sent)
	}

	r := fake.sent[0][0]
	if r.EntitySize != "micro" {
		t.Errorf("expected reminder EntitySize to be 'micro', got %q", r.EntitySize)
	}
}
