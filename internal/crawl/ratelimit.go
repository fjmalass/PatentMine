package crawl

import (
	"context"
	"sync"
	"time"
)

// limiter enforces a minimum interval between calls so web sources stay polite
// and avoid being blocked. A zero interval imposes no limit.
type limiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     time.Time
}

// newLimiter returns a limiter spacing calls by at least interval.
func newLimiter(interval time.Duration) *limiter {
	return &limiter{interval: interval}
}

// wait blocks until the next call is permitted, or until ctx is cancelled.
func (l *limiter) wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	var delay time.Duration
	if now.Before(l.next) {
		delay = l.next.Sub(now)
		l.next = l.next.Add(l.interval)
	} else {
		l.next = now.Add(l.interval)
	}
	l.mu.Unlock()

	if delay == 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
