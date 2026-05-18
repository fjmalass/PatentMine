package ingest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/store"
)

const slowCrawlerRun = 500 * time.Millisecond

// Crawl tuning. A crawl stops when it reaches either limit, so a dense citation
// graph cannot produce an unbounded job.
const (
	defaultMaxDepth   = 2
	defaultNodeBudget = 200
)

// Progress is an incremental crawl update reported through the emit callback.
type Progress struct {
	Fetched int    // Records fetched (full body) so far.
	Found   int    // Distinct numbers discovered so far (fetched + stub).
	Message string // Human-readable note about the latest step.
}

// CrawlConfig bounds a crawl.
type CrawlConfig struct {
	MaxDepth   int // BFS depth from the root; <= 0 uses defaultMaxDepth.
	NodeBudget int // Max records to fetch; <= 0 uses defaultNodeBudget.
}

// Crawler walks the patent family graph breadth-first, fetching each node from
// the source registry and writing records, documents, and relations to the
// store.
type Crawler struct {
	registry *Registry
	repo     store.Repository
	cfg      CrawlConfig
	metrics  *observability.Metrics
	logger   *slog.Logger
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

// WithMetrics attaches a phase-1 in-process metrics recorder.
func (c *Crawler) WithMetrics(metrics *observability.Metrics) *Crawler {
	c.metrics = metrics
	return c
}

// WithLogger attaches a logger for slow crawl warnings.
func (c *Crawler) WithLogger(logger *slog.Logger) *Crawler {
	c.logger = logger
	return c
}

// node is one queued patent number and its BFS depth from the root.
type node struct {
	number domain.PatentNumber
	depth  int
}

// fetch retrieves one patent. A force fetch skips the local file cache so the
// crawl re-pulls from the web even when a cached file exists.
func (c *Crawler) fetch(ctx context.Context, number domain.PatentNumber, force bool) (Result, error) {
	if force {
		return c.registry.FetchExcluding(ctx, number, domain.SourceFile)
	}
	return c.registry.Fetch(ctx, number)
}

// Crawl performs a bounded breadth-first walk from root. A negative maxDepth
// uses the configured depth; zero crawls the root only. force bypasses the
// local file cache. emit, which may be nil, receives progress.
func (c *Crawler) Crawl(ctx context.Context, root domain.PatentNumber, maxDepth int, force bool, emit func(Progress)) error {
	start := time.Now()
	failed := true
	defer func() {
		if c.metrics != nil {
			d := time.Since(start)
			c.metrics.ObserveDuration("ingest.crawler.crawl", d, failed)
			if c.logger != nil && d >= slowCrawlerRun {
				c.logger.Warn("slow ingest crawl",
					slog.String("root", root.String()),
					slog.Int64("duration_ms", d.Milliseconds()),
					slog.Bool("failed", failed))
			}
		}
	}()
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

		res, err := c.fetch(ctx, cur.number, force)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if err != nil {
			// The patent is referenced but could not be fetched: record a stub
			// so the edge to it still resolves, and keep crawling.
			if _, stubErr := c.ensureRecord(ctx, cur.number); stubErr != nil {
				return stubErr
			}
			report(Progress{
				Fetched: fetched, Found: len(seen),
				Message: fmt.Sprintf("%s unavailable: %v", cur.number, err),
			})
			continue
		}

		recordNumber, err := c.saveRecord(ctx, res)
		if err != nil {
			return err
		}
		if err := c.saveRelations(ctx, recordNumber, res.Relations, cur.depth, depthLimit, seen, &queue); err != nil {
			return err
		}
		fetched++
		report(Progress{
			Fetched: fetched, Found: len(seen),
			Message: fmt.Sprintf("fetched %s", recordNumber),
		})
	}
	failed = false
	return nil
}

// saveRecord stores a fetched Result as one record and returns the record's
// permanent number. When the fetched documents already belong to one or more
// records, the Result is folded into them (merging records that collide).
func (c *Crawler) saveRecord(ctx context.Context, res Result) (domain.PatentNumber, error) {
	recordNumber, err := c.resolveRecord(ctx, candidateNumbers(res))
	if err != nil {
		return domain.PatentNumber{}, err
	}
	if recordNumber.IsZero() {
		recordNumber = res.Patent.Number
	}

	existing, err := c.repo.Documents(ctx, recordNumber)
	if err != nil {
		return domain.PatentNumber{}, err
	}

	patent := res.Patent
	patent.Number = recordNumber
	patent.FetchState = domain.FetchCached
	patent.Documents = mergeDocuments(existing, res.Documents)
	patent.DisplayNumber = patent.NumberToShow()
	if err := c.repo.SavePatent(ctx, patent); err != nil {
		return domain.PatentNumber{}, err
	}
	for _, doc := range res.Documents {
		if err := c.repo.SaveDocument(ctx, recordNumber, doc); err != nil {
			return domain.PatentNumber{}, err
		}
	}
	return recordNumber, nil
}

// resolveRecord finds which existing record (if any) the candidate numbers
// belong to. When they belong to several records, those records are merged and
// the survivor is returned. A zero number means none of them is known yet.
func (c *Crawler) resolveRecord(ctx context.Context, candidates []domain.PatentNumber) (domain.PatentNumber, error) {
	var records []domain.PatentNumber
	seen := map[domain.PatentNumber]bool{}
	for _, n := range candidates {
		rec, err := c.repo.RecordOf(ctx, n)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return domain.PatentNumber{}, err
		}
		if !seen[rec] {
			seen[rec] = true
			records = append(records, rec)
		}
	}
	if len(records) == 0 {
		return domain.PatentNumber{}, nil
	}
	keep := records[0]
	for _, other := range records[1:] {
		if err := c.repo.MergeRecords(ctx, keep, other); err != nil {
			return domain.PatentNumber{}, err
		}
	}
	return keep, nil
}

// saveRelations records the fetched edges and queues neighbours. Every edge
// endpoint is resolved to a record number, creating a stub record when the
// neighbour is new, so an edge always points at real records.
func (c *Crawler) saveRelations(ctx context.Context, from domain.PatentNumber, relations []domain.Relation, depth, depthLimit int, seen map[domain.PatentNumber]bool, queue *[]node) error {
	for _, rel := range relations {
		neighbour := rel.To
		neighbourRecord, err := c.ensureRecord(ctx, neighbour)
		if err != nil {
			return err
		}
		if err := c.repo.SaveRelation(ctx, domain.Relation{
			From: from, To: neighbourRecord, Kind: rel.Kind,
		}); err != nil {
			return err
		}
		if !seen[neighbour] {
			seen[neighbour] = true
			if depth < depthLimit {
				*queue = append(*queue, node{number: neighbour, depth: depth + 1})
			}
		}
	}
	return nil
}

// ensureRecord returns the record a number belongs to, creating a stub record
// (a patent row plus one document) when the number is not known yet.
func (c *Crawler) ensureRecord(ctx context.Context, number domain.PatentNumber) (domain.PatentNumber, error) {
	rec, err := c.repo.RecordOf(ctx, number)
	if err == nil {
		return rec, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return domain.PatentNumber{}, err
	}
	stub := domain.Patent{
		Number:        number,
		DisplayNumber: number,
		FetchState:    domain.FetchStub,
	}
	if err := c.repo.SavePatent(ctx, stub); err != nil {
		return domain.PatentNumber{}, err
	}
	if err := c.repo.SaveDocument(ctx, number, domain.Document{
		Number: number,
		Stage:  domain.GuessStage(number),
	}); err != nil {
		return domain.PatentNumber{}, err
	}
	return number, nil
}

// candidateNumbers lists every number that could identify a fetched Result's
// record: the fetched number and each of its documents.
func candidateNumbers(res Result) []domain.PatentNumber {
	numbers := []domain.PatentNumber{res.Patent.Number}
	for _, d := range res.Documents {
		numbers = append(numbers, d.Number)
	}
	return numbers
}

// mergeDocuments combines a record's stored documents with newly fetched ones,
// keyed by number so a fetched document replaces its stored version.
func mergeDocuments(existing, fetched []domain.Document) []domain.Document {
	byNumber := make(map[domain.PatentNumber]domain.Document, len(existing)+len(fetched))
	order := make([]domain.PatentNumber, 0, len(existing)+len(fetched))
	for _, group := range [][]domain.Document{existing, fetched} {
		for _, d := range group {
			if _, ok := byNumber[d.Number]; !ok {
				order = append(order, d.Number)
			}
			byNumber[d.Number] = d
		}
	}
	out := make([]domain.Document, 0, len(order))
	for _, n := range order {
		out = append(out, byNumber[n])
	}
	return out
}
