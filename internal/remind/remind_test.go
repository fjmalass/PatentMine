package remind

import (
	"strings"
	"testing"
	"time"
)

func TestNewNotifierSelection(t *testing.T) {
	// Disabled → noop.
	if n := NewNotifier(EmailConfig{Enabled: false, Host: "smtp.example.com", To: "a@b.com"}); n.Enabled() {
		t.Error("disabled config should yield a disabled (noop) notifier")
	}
	// Enabled but no host → noop.
	if n := NewNotifier(EmailConfig{Enabled: true, To: "a@b.com"}); n.Enabled() {
		t.Error("missing host should yield a disabled notifier")
	}
	// Enabled but no recipient → noop.
	if n := NewNotifier(EmailConfig{Enabled: true, Host: "smtp.example.com"}); n.Enabled() {
		t.Error("missing recipient should yield a disabled notifier")
	}
	// Fully configured → email.
	n := NewNotifier(EmailConfig{Enabled: true, Host: "smtp.example.com", To: "a@b.com"})
	if !n.Enabled() || n.Channel() != "email" {
		t.Errorf("configured notifier: enabled=%v channel=%q", n.Enabled(), n.Channel())
	}
}

func TestBuildDigest(t *testing.T) {
	now := time.Now()
	rs := []Reminder{
		{Title: "B fee", Kind: "Maintenance fee", DueDate: now.AddDate(0, 0, 10), DaysUntil: 10, EntitySize: "small"},
		{Title: "A overdue", Kind: "OA response", DueDate: now.AddDate(0, 0, -3), DaysUntil: -3},
	}
	msg := buildDigest("from@x.com", "to@y.com", rs)
	if !strings.Contains(msg, "To: to@y.com") || !strings.Contains(msg, "Subject: PatentMine: 2 upcoming") {
		t.Fatalf("digest headers wrong:\n%s", msg)
	}
	// Soonest (overdue) must sort before the 10-day one.
	if strings.Index(msg, "A overdue") > strings.Index(msg, "B fee") {
		t.Fatalf("digest not sorted soonest-first:\n%s", msg)
	}
	if !strings.Contains(msg, "OVERDUE") {
		t.Fatalf("overdue item not flagged:\n%s", msg)
	}
	if !strings.Contains(msg, "(small entity)") {
		t.Fatalf("expected digest to contain entity size '(small entity)', got:\n%s", msg)
	}
}

func TestSplitRecipients(t *testing.T) {
	got := splitRecipients("a@x.com, b@y.com ; c@z.com")
	if len(got) != 3 {
		t.Fatalf("splitRecipients = %v, want 3", got)
	}
}
