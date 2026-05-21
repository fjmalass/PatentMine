package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"patentmine/internal/observability"
	"patentmine/internal/proto"
)

// ErrQueueFull reports that no crawl slot is available right now.
var ErrQueueFull = errors.New("engine: crawl queue full")

// JobID identifies a background job for progress reporting and cancellation.
type JobID string

// Job is one unit of background work — typically a family-graph crawl.
// It receives its own JobID so progress events can be attributed; emit
// publishes those events. The pool wraps every job with a final done event.
type Job interface {
	Run(ctx context.Context, id JobID, emit func(proto.Event)) error
}

// JobFunc adapts a plain function to the Job interface.
type JobFunc func(ctx context.Context, id JobID, emit func(proto.Event)) error

// Run implements Job.
func (f JobFunc) Run(ctx context.Context, id JobID, emit func(proto.Event)) error {
	return f(ctx, id, emit)
}

// queuedJob pairs a job with its cancellable context and id.
type queuedJob struct {
	id  JobID
	ctx context.Context
	job Job
}

// workerPool runs jobs on a bounded set of goroutines. Each job has its own
// context so it can be cancelled independently and outlives any client.
type workerPool struct {
	bus     *Bus
	baseCtx context.Context
	queue   chan queuedJob
	seq     atomic.Uint64

	mu      sync.Mutex
	cancels map[JobID]context.CancelFunc
	metrics *observability.Metrics

	wg sync.WaitGroup
}

const queueDepth = 64

// newWorkerPool starts workers goroutines draining the job queue.
func newWorkerPool(ctx context.Context, workers int, bus *Bus, metrics *observability.Metrics) *workerPool {
	p := &workerPool{
		bus:     bus,
		baseCtx: ctx,
		queue:   make(chan queuedJob, queueDepth),
		cancels: make(map[JobID]context.CancelFunc),
		metrics: metrics,
	}
	for range workers {
		p.wg.Add(1)
		go p.loop()
	}
	return p
}

func (p *workerPool) loop() {
	defer p.wg.Done()
	for qj := range p.queue {
		p.run(qj)
	}
}

func (p *workerPool) run(qj queuedJob) {
	start := time.Now()
	emit := func(ev proto.Event) { p.bus.Publish(ev) }
	err := qj.job.Run(qj.ctx, qj.id, emit)

	p.mu.Lock()
	if cancel, ok := p.cancels[qj.id]; ok {
		cancel()
		delete(p.cancels, qj.id)
	}
	p.mu.Unlock()

	done := proto.CrawlDone{JobID: string(qj.id)}
	if err != nil {
		done.Error = err.Error()
	}
	if p.metrics != nil {
		p.metrics.ObserveDuration("engine.worker.job_run", time.Since(start), err != nil)
		p.metrics.SetGauge("engine.worker.queue_depth", int64(len(p.queue)))
	}
	p.bus.Publish(proto.NewEvent(proto.EventCrawlDone, done))
}

// submit enqueues a job and returns its id. The job's context is a child of
// the pool's base context, so a daemon shutdown cancels every running job.
func (p *workerPool) submit(job Job) (JobID, error) {
	id := JobID(fmt.Sprintf("job-%d", p.seq.Add(1)))
	ctx, cancel := context.WithCancel(p.baseCtx)

	p.mu.Lock()
	p.cancels[id] = cancel
	p.mu.Unlock()

	select {
	case p.queue <- queuedJob{id: id, ctx: ctx, job: job}:
		if p.metrics != nil {
			p.metrics.IncCounter("engine.worker.submit_total", 1)
			p.metrics.SetGauge("engine.worker.queue_depth", int64(len(p.queue)))
		}
		return id, nil
	default:
		cancel()
		p.mu.Lock()
		delete(p.cancels, id)
		p.mu.Unlock()
		if p.metrics != nil {
			p.metrics.IncCounter("engine.worker.queue_full_total", 1)
			p.metrics.SetGauge("engine.worker.queue_depth", int64(len(p.queue)))
		}
		return "", ErrQueueFull
	}
}

// cancel stops a running or queued job. It reports whether the id was known.
func (p *workerPool) cancel(id JobID) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	cancel, ok := p.cancels[id]
	if ok {
		cancel()
		delete(p.cancels, id)
	}
	if p.metrics != nil {
		p.metrics.IncCounter("engine.worker.cancel_total", 1)
		p.metrics.SetGauge("engine.worker.queue_depth", int64(len(p.queue)))
	}
	return ok
}

// shutdown stops accepting jobs and waits for in-flight jobs to finish.
func (p *workerPool) shutdown() {
	close(p.queue)
	p.wg.Wait()
}
