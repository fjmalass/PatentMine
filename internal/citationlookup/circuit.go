package citationlookup

import (
	"fmt"
	"sync"
	"time"
)

type CircuitBreaker struct {
	mu          sync.Mutex
	threshold   int
	cooldown    time.Duration
	failures    int
	openUntil   time.Time
	lastFailure string
}

func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 3
	}
	if cooldown <= 0 {
		cooldown = time.Minute
	}
	return &CircuitBreaker{threshold: threshold, cooldown: cooldown}
}

func (b *CircuitBreaker) Allow(now time.Time) (time.Time, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.openUntil.IsZero() || !now.Before(b.openUntil) {
		return time.Time{}, true
	}
	return b.openUntil, false
}

func (b *CircuitBreaker) RecordSuccess(time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.openUntil = time.Time{}
	b.lastFailure = ""
}

func (b *CircuitBreaker) RecordFailure(now time.Time, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	if err != nil {
		b.lastFailure = err.Error()
	}
	if b.failures >= b.threshold {
		b.openUntil = now.Add(b.cooldown)
	}
}

func (b *CircuitBreaker) OpenError(openUntil time.Time) error {
	b.mu.Lock()
	lastFailure := b.lastFailure
	b.mu.Unlock()
	if lastFailure == "" {
		return fmt.Errorf("upstream circuit open until %s", openUntil.Format(time.RFC3339))
	}
	return fmt.Errorf("upstream circuit open until %s after %s", openUntil.Format(time.RFC3339), lastFailure)
}
