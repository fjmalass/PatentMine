package overlay

import (
	"testing"

	"patentmine/internal/domain"
)

func TestNextActivityCycles(t *testing.T) {
	got := nextActivity(domain.TimeReading)
	if got != domain.TimeWriting {
		t.Fatalf("nextActivity(reading) = %q, want writing", got)
	}
	// Wraps from the last back to the first.
	if last := nextActivity(domain.TimeAdmin); last != domain.TimeReading {
		t.Fatalf("nextActivity(admin) = %q, want reading (wrap)", last)
	}
	// An unknown activity resets to the first in the cycle.
	if def := nextActivity(domain.TimeActivity("bogus")); def != domain.TimeReading {
		t.Fatalf("nextActivity(unknown) = %q, want reading", def)
	}
}

func TestReviewAndSummaryDuration(t *testing.T) {
	if got := reviewDuration(75); got != "1:15" {
		t.Errorf("reviewDuration(75) = %q, want 1:15", got)
	}
	if got := summaryDuration(75); got != "1m 15s" {
		t.Errorf("summaryDuration(75) = %q, want 1m 15s", got)
	}
	if got := summaryDuration(3725); got != "1h 02m" {
		t.Errorf("summaryDuration(3725) = %q, want 1h 02m", got)
	}
}

func TestFocusTimerAttributesToCurrentSide(t *testing.T) {
	f := newFocusTimer(true) // start on writing
	f.switchTo(false)        // now reading; writing got the first slice
	f.switchTo(true)         // back to writing; reading got the middle slice
	// Both sides should have accrued some (tiny) non-negative duration and the
	// timer must not panic; exact values are wall-clock dependent.
	if f.reading < 0 || f.writing < 0 {
		t.Fatalf("focus timer accrued negative durations: reading=%v writing=%v", f.reading, f.writing)
	}
}
