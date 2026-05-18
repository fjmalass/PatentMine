package ingest

import (
	"context"
	"errors"
	"fmt"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

// Crawl tuning. A crawl stops when it reaches either limit, so a dense citation
// graph cannot produce an unbounded job.
const (
	defaultMaxDepth   = 2
	defaultNodeBudget = 200
)

// Progress is an incremental crawl update reported through the emit callback.
type Progress struct {
	Fetched int    // Patents fetched (full body) so far.
	Found   int    // Distinct patents discovered so far (fetched + stub).
	Message string // Human-readable note about the latest step.
}

// CrawlConfig bounds a crawl.
type CrawlConfig struct {
	MaxDepth   int // BFS depth from the root; <= 0 uses defaultMaxDepth.
	NodeBudget int // Max patents to fetch; <= 0 uses defaultNodeBudget.
}

// Crawler walks the patent family graph breadth-first, fetching each node from
// the source registry and writing patents and relations to the store.
type Crawler struct {
	registry *Registry
	repo     store.Repository
	cfg      CrawlConfig
}

// NewCrawler builds a Crawler.
func NewCrawler(registry *Registry, repo store.Repository, cfg CrawlConfig) *Crawler {
	if cfg.MaxDepth <= 0 {
		cfg.MaxDepth = defaultMaxDepth
	}
	if cfg.NodeBudget <= 0 {
		cfg.NodeBudget = defaultNodeBudget
	}
	return &Crawler{registry: registry, repo: repo, cfg: cfg}
}

// node is one queued patent and its BFS depth from the root.
type node struct {
	number domain.PatentNumber
	depth  int
}

// Crawl performs a bounded breadth-first walk from root. A negative maxDepth
// uses the configured depth; zero crawls the root only. emit, which may be
// nil, receives progress.
func (c *Crawler) Crawl(ctx context.Context, root domain.PatentNumber, maxDepth int, emit func(Progress)) error {
	if root.IsZero() {
		return errors.New("ingest: crawl root must not be empty")
	}
	depthLimit := maxDepth
	if maxDepth < 0 {
		depthLimit = c.cfg.MaxDepth
	}
	report := func(p Progress) {
		if emit != nil {
			emit(p)
		}
	}

	seen := map[domain.PatentNumber]bool{root: true}
	queue := []node{{number: root, depth: 0}}
	fetched := 0

	for len(queue) > 0 && fetched < c.cfg.NodeBudget {
		if err := ctx.Err(); err != nil {
			return err
		}
		cur := queue[0]
		queue = queue[1:]

		res, err := c.registry.Fetch(ctx, cur.number)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if err != nil {
			// The patent is referenced but could not be fetched: record a stub
			// so the edge to it still resolves, and keep crawling.
			if stubErr := c.ensureStub(ctx, cur.number); stubErr != nil {
				return stubErr
			}
			report(Progress{
				Fetched: fetched, Found: len(seen),
				Message: fmt.Sprintf("%s unavailable: %v", cur.number, err),
			})
			continue
		}

		if err := c.save(ctx, res, &seen, &queue, cur.depth, depthLimit); err != nil {
			return err
		}
		fetched++
		report(Progress{
			Fetched: fetched, Found: len(seen),
			Message: fmt.Sprintf("fetched %s", cur.number),
		})
	}
	return nil
}

// save persists a fetched patent and its relations, enqueueing in-depth
// neighbors and recording out-of-depth neighbors as stubs.
func (c *Crawler) save(ctx context.Context, res Result, seen *map[domain.PatentNumber]bool, queue *[]node, depth, depthLimit int) error {
	patent := res.Patent
	patent.FetchState = domain.FetchCached
	if err := c.repo.SavePatent(ctx, patent); err != nil {
		return err
	}
	for _, rel := range res.Relations {
		if err := c.repo.SaveRelation(ctx, rel); err != nil {
			return err
		}
		neighbor := rel.To
		if neighbor.IsZero() || (*seen)[neighbor] {
			continue
		}
		(*seen)[neighbor] = true
		if depth < depthLimit {
			*queue = append(*queue, node{number: neighbor, depth: depth + 1})
		} else if err := c.ensureStub(ctx, neighbor); err != nil {
			return err
		}
	}
	return nil
}

// ensureStub records a placeholder patent when none exists yet, so a relation
// edge always points at a real row. An existing record is left untouched.
func (c *Crawler) ensureStub(ctx context.Context, number domain.PatentNumber) error {
	_, err := c.repo.Patent(ctx, number)
	if err == nil {
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	return c.repo.SavePatent(ctx, domain.Patent{
		Number:     number,
		FetchState: domain.FetchStub,
	})
}
