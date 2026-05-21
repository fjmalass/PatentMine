package citationlookup

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

type GlobalLimiter struct {
	tokens   chan struct{}
	inflight chan struct{}
	jitter   time.Duration
	randMu   sync.Mutex
	rand     *rand.Rand
}

func NewGlobalLimiter(requestsPerSecond int, maxInflight int, jitter time.Duration) *GlobalLimiter {
	if requestsPerSecond <= 0 {
		requestsPerSecond = 1
	}
	if maxInflight <= 0 {
		maxInflight = 1
	}
	limiter := &GlobalLimiter{
		tokens:   make(chan struct{}, requestsPerSecond),
		inflight: make(chan struct{}, maxInflight),
		jitter:   jitter,
		rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	for i := 0; i < requestsPerSecond; i++ {
		limiter.tokens <- struct{}{}
	}
	interval := time.Second / time.Duration(requestsPerSecond)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			select {
			case limiter.tokens <- struct{}{}:
			default:
			}
		}
	}()
	return limiter
}

func (l *GlobalLimiter) Acquire(ctx context.Context) (func(), error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-l.tokens:
	}
	if l.jitter > 0 {
		l.randMu.Lock()
		delay := time.Duration(l.rand.Int63n(int64(l.jitter)))
		l.randMu.Unlock()
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case l.inflight <- struct{}{}:
		var once sync.Once
		release := func() {
			once.Do(func() {
				<-l.inflight
			})
		}
		return release, nil
	}
}

type NoopLimiter struct{}

func (NoopLimiter) Acquire(context.Context) (func(), error) {
	return func() {}, nil
}
