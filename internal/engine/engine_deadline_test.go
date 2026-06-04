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
