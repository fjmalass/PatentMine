package citationlookup

import (
	"context"
	"sync"
	"time"
)

type Runner struct {
	Worker      *Worker
	Concurrency int
	IdleDelay   time.Duration
}

func (r Runner) Run(ctx context.Context) error {
	if r.Worker == nil {
		return nil
	}
	concurrency := r.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	idleDelay := r.IdleDelay
	if idleDelay <= 0 {
		idleDelay = time.Second
	}

	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				worked, err := r.Worker.WorkOnce(ctx)
				if err != nil {
					select {
					case errCh <- err:
					default:
					}
				}
				if !worked {
					timer := time.NewTimer(idleDelay)
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case <-timer.C:
					}
				}
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case err := <-errCh:
		return err
	case <-ctx.Done():
		<-done
		return ctx.Err()
	}
}
