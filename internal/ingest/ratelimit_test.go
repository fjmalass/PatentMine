package ingest

import (
	"context"
	"testing"
	"time"
)

func TestLimiterSpacesCalls(t *testing.T) {
	const interval = 20 * time.Millisecond
	l := newLimiter(interval)

	start := time.Now()
	for range 5 {
		if err := l.wait(context.Background()); err != nil {
			t.Fatalf("wait: %v", err)
		}
	}
	// The first call is immediate; the next four are each spaced by interval.
	if elapsed := time.Since(start); elapsed < 3*interval {
		t.Fatalf("5 calls took %v, want at least %v", elapsed, 3*interval)
	}
}

func TestLimiterZeroIntervalNeverWaits(t *testing.T) {
	l := newLimiter(0)
	start := time.Now()
	for range 100 {
		if err := l.wait(context.Background()); err != nil {
			t.Fatalf("wait: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("zero-interval limiter waited %v", elapsed)
	}
}

func TestLimiterRespectsContext(t *testing.T) {
	l := newLimiter(time.Hour)
	if err := l.wait(context.Background()); err != nil {
		t.Fatalf("first wait: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.wait(ctx); err == nil {
		t.Fatal("wait on a cancelled context should return an error")
	}
}
