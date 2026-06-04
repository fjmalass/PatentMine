package pane

import (
	"strings"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/tui/render"
)

func TestResponseDueLabel(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		oa   domain.OfficeAction
		want string
	}{
		{"no deadline", domain.OfficeAction{}, "—"},
		{"overdue", domain.OfficeAction{Status: domain.OAStatusOpen, ResponseDue: now.AddDate(0, 0, -3)}, "OVERDUE"},
		{"due soon shows day count", domain.OfficeAction{Status: domain.OAStatusOpen, ResponseDue: now.AddDate(0, 0, 5)}, "d ·"},
		{"responded shows plain date", domain.OfficeAction{Status: domain.OAStatusResponded, ResponseDue: now.AddDate(0, 0, -3)}, now.AddDate(0, 0, -3).Format(domain.DateLayout)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := responseDueLabel(c.oa)
			if !strings.Contains(got, c.want) {
				t.Fatalf("responseDueLabel = %q, want it to contain %q", got, c.want)
			}
		})
	}
}

func TestFmtDuration(t *testing.T) {
	cases := map[int]string{
		0:     "0:00",
		42:    "0:42",
		75:    "1:15",
		3600:  "1:00",
		3725:  "1:02",
	}
	for secs, want := range cases {
		if got := fmtDuration(secs); got != want {
			t.Errorf("fmtDuration(%d) = %q, want %q", secs, got, want)
		}
	}
}

func TestFormatTimeSummaryOrdersActivities(t *testing.T) {
	s := domain.TimeSummary{Seconds: map[domain.TimeActivity]int{
		domain.TimeWriting: 75, domain.TimeReading: 42, domain.TimeAI: 8,
	}}
	got := formatTimeSummary(s)
	// Reading must precede writing must precede ai.
	ri, wi, ai := strings.Index(got, "Reading"), strings.Index(got, "Writing"), strings.Index(got, "AI")
	if !(ri >= 0 && ri < wi && wi < ai) {
		t.Fatalf("formatTimeSummary ordering wrong: %q", got)
	}
}

func TestOfficeActionsTableRenders(t *testing.T) {
	o := NewOfficeActions(nil, render.NewTheme(), "p1")
	o, _ = asOfficeActions(o.Update(officeActionsLoadedMsg{items: []domain.OfficeAction{
		{Examiner: "Menefee", Type: domain.OANonFinal, MailDate: time.Date(2026, 1, 9, 0, 0, 0, 0, time.UTC), Status: domain.OAStatusOpen},
	}}))
	out := o.View(100, 20)
	if !strings.Contains(out, "Menefee") || !strings.Contains(out, "2026-01-09") {
		t.Fatalf("table view missing row content:\n%s", out)
	}
}

func asOfficeActions(p Pane, cmd interface{}) (*OfficeActions, interface{}) {
	return p.(*OfficeActions), cmd
}
