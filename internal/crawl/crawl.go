package crawl

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
	defaultMaxDepth   = 4
	defaultNodeBudget = 200
)

// Progress is an incremental crawl update reported through the emit callback.
type Progress struct {
	CrawledCount    int      // Total full records saved to the database.
	DiscoveredCount int      // Total unique patent numbers seen (crawled + stubs).
	PendingCount    int      // Items currently in the crawl queue.
	Depth           int      // Current BFS depth being processed.
	MaxDepth        int      // Maximum BFS depth this crawl will reach.
	CitationsCount  int      // Total citation edges found.
	CitedByCount    int      // Total cited-by edges found.
	ParentsCount    int      // Total parent/continuation edges found.
	ChildrenCount   int      // Total child edges found.
	Message         string   // Human-readable note about the latest step.
	RecordNumber    string   // Record just saved in this event, if any.
	Stubs           []string // Numbers freshly stubbed in this event, if any.
	Sources         []string // Providers (uspto, google, ...) that contributed snapshots for RecordNumber.
	Mode            string   // Source-mode policy in effect during this crawl.
}

// CrawlConfig bounds a crawl.
type CrawlConfig struct {
	MaxDepth   int                 // BFS depth from the root; <= 0 uses defaultMaxDepth.
	NodeBudget int                 // Max records to fetch; <= 0 uses defaultNodeBudget.
	Profile    domain.CrawlProfile // Profile name (e.g., 'family', 'citations').
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

// Config returns the crawler's resolved limits after defaults are applied.
func (c *Crawler) Config() CrawlConfig {
	return c.cfg
}

// node is one queued patent number and its BFS depth from the root.
type node struct {
	number domain.PatentNumber
	depth  int
}

// fetch retrieves one patent. A force fetch skips the local file cache and
// the SQLite database so the crawl re-pulls from the web.
func (c *Crawler) fetch(ctx context.Context, number domain.PatentNumber, force bool) (Result, error) {
	return c.fetchFromSource(ctx, number, force, "")
}

func (c *Crawler) fetchFromSource(ctx context.Context, number domain.PatentNumber, force bool, source domain.Source) (Result, error) {
	if !force {
		p, err := c.repo.Patent(ctx, number)
		if err == nil && p.FetchState == domain.FetchCached {
			var rels []domain.Relation
			for _, kind := range []domain.RelationKind{
				domain.RelationCites, domain.RelationCitedBy,
				domain.RelationParent, domain.RelationChild,
			} {
				if kr, err := c.repo.Relations(ctx, number, kind); err == nil {
					rels = append(rels, kr...)
				}
			}
			return Result{Patent: p, Documents: p.Documents, Relations: rels}, nil
		}
	}
	if source != "" {
		return c.registry.FetchOnly(ctx, number, source)
	}
	if force {
		return c.registry.FetchExcluding(ctx, number, domain.SourceFile)
	}
	return c.registry.Fetch(ctx, number)
}

// Crawl profiles define which family-graph edges to follow during a crawl.
const (
	CrawlProfileCitations = "citations" // Follow cites only (depth 0 only)
	CrawlProfileCitedBy   = "citedby"   // Follow cited_by only (depth 0 only)
	CrawlProfileFamily    = "family"    // Follow parent/child recursion
	CrawlProfileAll       = "all"       // Combination of the above
)

// Crawl performs a bounded breadth-first walk from root. A negative maxDepth
// uses the configured depth; zero crawls the root only. force bypasses the
// local file cache. emit, which may be nil, receives progress.
func (c *Crawler) Crawl(ctx context.Context, root domain.PatentNumber, maxDepth int, profile domain.CrawlProfile, force bool, emit func(Progress)) error {
	return c.crawl(ctx, root, maxDepth, profile, force, "", emit)
}

// CrawlFromSource crawls using exactly one external provider. It bypasses the
// normal provider fallback order so callers can intentionally request USPTO or
// Google data.
func (c *Crawler) CrawlFromSource(ctx context.Context, root domain.PatentNumber, maxDepth int, profile domain.CrawlProfile, source domain.Source, emit func(Progress)) error {
	return c.crawl(ctx, root, maxDepth, profile, true, source, emit)
}

func (c *Crawler) crawl(ctx context.Context, root domain.PatentNumber, maxDepth int, profile domain.CrawlProfile, force bool, source domain.Source, emit func(Progress)) error {
	start := time.Now()
	failed := true
	log := observability.WithContextAttrs(ctx, c.logger)
	if log == nil {
		log = slog.Default()
	}

	defer func() {
		if c.metrics != nil {
			d := time.Since(start)
			c.metrics.ObserveDuration("crawl.crawler.crawl", d, failed)
			if d >= slowCrawlerRun {
				log.Warn("slow crawl",
					slog.String("root", root.String()),
					slog.Int64("duration_ms", d.Milliseconds()),
					slog.Bool("failed", failed))
			}
		}
	}()

	if root.IsZero() {
		return errors.New("crawl: crawl root must not be empty")
	}

	// A crawl makes thousands of writes; suspend the listing cache for its
	// duration so it does not serve rows that are stale within milliseconds,
	// and flush once when the crawl ends.
	if cc, ok := c.repo.(cacheController); ok {
		cc.SuspendCaching()
		defer cc.ResumeCaching()
	}

	log.Info("starting crawl",
		slog.String("root", root.String()),
		slog.Int("max_depth", maxDepth),
		slog.String("profile", string(profile)),
		slog.String("source", string(source)),
		slog.String("source_mode", c.registry.SourceMode()),
		slog.Bool("force", force))

	depthLimit := maxDepth
	if maxDepth < 0 {
		depthLimit = c.cfg.MaxDepth
	}

	type crawlStats struct {
		citations int
		citedBy   int
		parents   int
		children  int
	}
	stats := crawlStats{}

	report := func(p Progress) {
		p.CitationsCount = stats.citations
		p.CitedByCount = stats.citedBy
		p.ParentsCount = stats.parents
		p.ChildrenCount = stats.children

		log.Info("crawl progress",
			slog.String("root", root.String()),
			slog.Int("depth", p.Depth),
			slog.Int("max_depth", p.MaxDepth),
			slog.Int("crawled", p.CrawledCount),
			slog.Int("discovered", p.DiscoveredCount),
			slog.Int("pending", p.PendingCount),
			slog.String("message", p.Message))

		if emit != nil {
			emit(p)
		}
		if c.metrics != nil {
			c.metrics.SetGauge("crawl.crawler.crawled", int64(p.CrawledCount))
			c.metrics.SetGauge("crawl.crawler.discovered", int64(p.DiscoveredCount))
			c.metrics.SetGauge("crawl.crawler.pending", int64(p.PendingCount))
			c.metrics.SetGauge("crawl.crawler.depth", int64(p.Depth))
			c.metrics.SetGauge("crawl.crawler.max_depth", int64(p.MaxDepth))
			c.metrics.SetGauge("crawl.crawler.citations", int64(p.CitationsCount))
			c.metrics.SetGauge("crawl.crawler.cited_by", int64(p.CitedByCount))
			c.metrics.SetGauge("crawl.crawler.parents", int64(p.ParentsCount))
			c.metrics.SetGauge("crawl.crawler.children", int64(p.ChildrenCount))
		}
	}

	seen := map[domain.PatentNumber]bool{root: true}
	queue := []node{{number: root, depth: 0}}
	ingested := 0

	var curDepth int
	ctx = WithProgressReporter(ctx, func(msg string) {
		report(Progress{
			CrawledCount:    ingested,
			DiscoveredCount: len(seen),
			PendingCount:    len(queue),
			Depth:           curDepth,
			MaxDepth:        depthLimit,
			Message:         msg,
		})
	})

	for len(queue) > 0 && ingested < c.cfg.NodeBudget {
		if err := ctx.Err(); err != nil {
			log.Info("crawl cancelled", slog.String("error", err.Error()))
			return err
		}
		cur := queue[0]
		queue = queue[1:]
		curDepth = cur.depth

		ReportProgress(ctx, fmt.Sprintf("fetching %s...", cur.number))

		fstart := time.Now()
		res, err := c.fetchFromSource(ctx, cur.number, force, source)
		if c.metrics != nil {
			c.metrics.ObserveDuration("crawl.crawler.fetch", time.Since(fstart), err != nil)
		}

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			log.Info("fetch cancelled", slog.String("number", cur.number.String()))
			return err
		}
		if err != nil {
			log.Warn("fetch failed",
				slog.String("number", cur.number.String()),
				slog.String("error", err.Error()))

			// Root fetch failure — fail the job so cleanupIfNotFound can remove
			// the stub left by ensureRecord. Both "not available" (no provider
			// has the document) and hard errors (timeouts, 5xx, parse failures)
			// must surface — otherwise a transient hard error silently leaves
			// an empty stub in the database with no body data, which the user
			// sees as "Fetch state: stub" with no way to retry except L.
			if cur.depth == 0 {
				notAvail := errors.Is(err, ErrNotAvailable)
				if c.metrics != nil {
					if notAvail {
						c.metrics.IncCounter("crawl.root_not_found_total", 1)
					} else {
						c.metrics.IncCounter("crawl.root_hard_error_total", 1)
					}
				}
				log.Warn("root patent fetch failed",
					slog.String("number", cur.number.String()),
					slog.String("error", err.Error()),
					slog.Bool("not_available", notAvail))
				return fmt.Errorf("crawl: root %s fetch failed: %w", cur.number, err)
			}

			// A referenced patent that could not be fetched: record a stub
			// so the edge to it still resolves, and keep crawling.
			if _, stubErr := c.stubRecord(ctx, cur.number); stubErr != nil {
				return stubErr
			}
			report(Progress{
				CrawledCount: ingested, DiscoveredCount: len(seen), PendingCount: len(queue), Depth: cur.depth, MaxDepth: depthLimit,
				Message: fmt.Sprintf("%s unavailable: %v", cur.number, err),
				Stubs:   []string{cur.number.String()},
				Mode:    c.registry.SourceMode(),
			})
			continue
		}

		// Update stats for all relations found in this fetch, even if we've
		// seen the neighbour before, so the user sees a count of edges found.
		for _, rel := range res.Relations {
			switch rel.Kind {
			case domain.RelationCites:
				stats.citations++
			case domain.RelationCitedBy:
				stats.citedBy++
			case domain.RelationParent:
				stats.parents++
			case domain.RelationChild:
				stats.children++
			}
		}

		recordNumber, stubs, sources, err := c.ingestNode(ctx, res, cur.depth, depthLimit, profile, seen, &queue)
		if err != nil {
			log.Error("ingest node failed",
				slog.String("number", cur.number.String()),
				slog.String("error", err.Error()))
			return err
		}

		ingested++
		stubStrings := make([]string, 0, len(stubs))
		for _, s := range stubs {
			stubStrings = append(stubStrings, s.String())
		}
		report(Progress{
			CrawledCount: ingested, DiscoveredCount: len(seen), PendingCount: len(queue), Depth: cur.depth, MaxDepth: depthLimit,
			Message:      fmt.Sprintf("crawled %s", recordNumber),
			RecordNumber: recordNumber.String(),
			Stubs:        stubStrings,
			Sources:      sources,
			Mode:         c.registry.SourceMode(),
		})
	}

	reason := "exhausted"
	if ingested >= c.cfg.NodeBudget {
		reason = "budget reached"
	}

	log.Info("crawl finished",
		slog.String("root", root.String()),
		slog.String("source", string(source)),
		slog.String("source_mode", c.registry.SourceMode()),
		slog.String("reason", reason),
		slog.Int("crawled", ingested),
		slog.Int("discovered", len(seen)),
		slog.Int("citations", stats.citations),
		slog.Int("cited_by", stats.citedBy),
		slog.Int("parents", stats.parents),
		slog.Int("children", stats.children),
		slog.Duration("duration", time.Since(start)))

	recNum, err := c.repo.RecordOf(ctx, root)
	if err != nil {
		err = fmt.Errorf("crawl: root patent %s record mapping was not found in the repository: %w", root, err)
		log.Error("crawl failed", slog.String("root", root.String()), slog.String("error", err.Error()))
		return err
	}
	if p, err := c.repo.Patent(ctx, recNum); err == nil {
		if p.FetchState != domain.FetchCached {
			log.Warn("crawl root patent remained a stub, promoting to cached", slog.String("root", root.String()), slog.String("resolved", recNum.String()))
			p.FetchState = domain.FetchCached
			if serr := c.repo.SaveNode(ctx, store.NodeBatch{Patent: p}); serr != nil {
				log.Error("crawl failed to promote stub to cached", slog.String("root", root.String()), slog.String("error", serr.Error()))
			}
		}
	} else {
		err = fmt.Errorf("crawl: root patent %s (resolved to %s) was not found in the repository: %w", root, recNum, err)
		log.Error("crawl failed", slog.String("root", root.String()), slog.String("error", err.Error()))
		return err
	}

	failed = false
	return nil
}

// cacheController is satisfied by store.Cache: a crawl brackets itself with
// these calls so the listing cache becomes a pass-through for its duration.
type cacheController interface {
	SuspendCaching()
	ResumeCaching()
}

// ingestNode persists one fetched Result and every edge it carries as a single
// atomic batch. It resolves the record number (merging colliding records),
// merges the record's documents, resolves each neighbour to a record number —
// noting a stub for any neighbour not yet known — queues neighbours per the
// crawl profile, and issues exactly one SaveNode. A node with hundreds of
// citations therefore costs one transaction, not hundreds.
func (c *Crawler) ingestNode(ctx context.Context, res Result, depth, depthLimit int, profile domain.CrawlProfile, seen map[domain.PatentNumber]bool, queue *[]node) (domain.PatentNumber, []domain.PatentNumber, []string, error) {
	recordNumber, err := c.resolveRecord(ctx, candidateNumbers(res))
	if err != nil {
		return domain.PatentNumber{}, nil, nil, err
	}
	if recordNumber.IsZero() {
		recordNumber = res.Patent.Number
	}

	existing, err := c.repo.Documents(ctx, recordNumber)
	if err != nil {
		return domain.PatentNumber{}, nil, nil, err
	}

	patent := res.Patent
	patent.Number = recordNumber
	patent.FetchState = domain.FetchCached
	patent.Documents = mergeDocuments(existing, res.Documents)
	patent.DisplayNumber = patent.NumberToShow()

	batch := store.NodeBatch{
		Patent:               patent,
		Documents:            res.Documents,
		AuthorityIdentifiers: stampAuthorityIdentifiers(res.AuthorityIdentifiers, recordNumber),
		USPTOApplication:     stampUSPTOApplication(res.USPTOApplication, recordNumber),
		USPTOParties:         res.USPTOParties,
		USPTOEvents:          res.USPTOEvents,
		USPTOContinuities:    stampUSPTOContinuities(res.USPTOContinuities, recordNumber),
		USPTOForeignPriority: res.USPTOForeignPriority,
		SourceSnapshots:      stampSourceSnapshots(res.SourceSnapshots, recordNumber),
		SourceDiffs:          stampSourceDiffs(res.SourceDiffs, recordNumber),
	}
	stubbed := map[domain.PatentNumber]bool{}

	for _, rel := range res.Relations {
		neighbour := rel.To
		neighbourRecord, err := c.repo.RecordOf(ctx, neighbour)
		switch {
		case errors.Is(err, store.ErrNotFound):
			// Unknown neighbour: its record number is itself, and a stub is
			// created in the same batch so the edge always resolves.
			neighbourRecord = neighbour
			if !stubbed[neighbour] {
				stubbed[neighbour] = true
				batch.Stubs = append(batch.Stubs, store.StubRecord{
					Number: neighbour,
					Stage:  domain.GuessStage(neighbour),
				})
			}
		case err != nil:
			return domain.PatentNumber{}, nil, nil, err
		}
		batch.Relations = append(batch.Relations, domain.Relation{
			From: recordNumber, To: neighbourRecord, Kind: rel.Kind,
		})
		if !seen[neighbour] {
			seen[neighbour] = true
			if depth < depthLimit && shouldQueue(profile, rel.Kind, depth) {
				*queue = append(*queue, node{number: neighbour, depth: depth + 1})
			}
		}
	}

	if err := c.repo.SaveNode(ctx, batch); err != nil {
		return domain.PatentNumber{}, nil, nil, err
	}
	stubsOut := make([]domain.PatentNumber, 0, len(stubbed))
	for n := range stubbed {
		stubsOut = append(stubsOut, n)
	}
	sourceSet := map[string]bool{}
	var sourcesOut []string
	for _, snap := range batch.SourceSnapshots {
		if snap.Source == "" || sourceSet[snap.Source] {
			continue
		}
		sourceSet[snap.Source] = true
		sourcesOut = append(sourcesOut, snap.Source)
	}
	if len(sourcesOut) == 0 && patent.Source != "" {
		sourcesOut = append(sourcesOut, string(patent.Source))
	}
	return recordNumber, stubsOut, sourcesOut, nil
}

func stampAuthorityIdentifiers(in []domain.AuthorityIdentifier, record domain.PatentNumber) []domain.AuthorityIdentifier {
	out := make([]domain.AuthorityIdentifier, 0, len(in))
	for _, ident := range in {
		if ident.RecordNumber.IsZero() {
			ident.RecordNumber = record
		}
		out = append(out, ident)
	}
	return out
}

func stampUSPTOApplication(app *domain.USPTOApplication, record domain.PatentNumber) *domain.USPTOApplication {
	if app == nil {
		return nil
	}
	copy := *app
	if copy.RecordNumber.IsZero() {
		copy.RecordNumber = record
	}
	return &copy
}

func stampUSPTOContinuities(in []domain.USPTOContinuity, record domain.PatentNumber) []domain.USPTOContinuity {
	out := make([]domain.USPTOContinuity, 0, len(in))
	for _, c := range in {
		if c.ChildRecordNumber.IsZero() {
			c.ChildRecordNumber = record
		}
		out = append(out, c)
	}
	return out
}

func stampSourceSnapshots(in []domain.SourceSnapshot, record domain.PatentNumber) []domain.SourceSnapshot {
	out := make([]domain.SourceSnapshot, 0, len(in))
	for _, snap := range in {
		if snap.PatentNumber.IsZero() {
			snap.PatentNumber = record
		}
		out = append(out, snap)
	}
	return out
}

func stampSourceDiffs(in []domain.SourceDiff, record domain.PatentNumber) []domain.SourceDiff {
	out := make([]domain.SourceDiff, 0, len(in))
	for _, diff := range in {
		if diff.PatentNumber.IsZero() {
			diff.PatentNumber = record
		}
		out = append(out, diff)
	}
	return out
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

// shouldQueue reports whether a neighbour reached by an edge of the given kind
// should be enqueued for further crawling, given the crawl profile and the
// current BFS depth.
func shouldQueue(profile domain.CrawlProfile, kind domain.RelationKind, depth int) bool {
	switch profile {
	case domain.CrawlProfileCitations:
		return kind == domain.RelationCites && depth == 0
	case domain.CrawlProfileCitedBy:
		return kind == domain.RelationCitedBy && depth == 0
	case domain.CrawlProfileFamily:
		return kind == domain.RelationParent || kind == domain.RelationChild
	case domain.CrawlProfileAll, "":
		if kind == domain.RelationCites || kind == domain.RelationCitedBy {
			return depth == 0
		}
		return true
	default:
		return false
	}
}

// stubRecord returns the record a number belongs to, creating a stub record (a
// patent row plus one document) in one batch when the number is not known yet.
func (c *Crawler) stubRecord(ctx context.Context, number domain.PatentNumber) (domain.PatentNumber, error) {
	rec, err := c.repo.RecordOf(ctx, number)
	if err == nil {
		return rec, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return domain.PatentNumber{}, err
	}
	if err := c.repo.SaveNode(ctx, store.NodeBatch{
		Stubs: []store.StubRecord{{Number: number, Stage: domain.GuessStage(number)}},
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

type progressReporterKey struct{}

func WithProgressReporter(ctx context.Context, report func(string)) context.Context {
	return context.WithValue(ctx, progressReporterKey{}, report)
}

func ReportProgress(ctx context.Context, msg string) {
	if report, ok := ctx.Value(progressReporterKey{}).(func(string)); ok {
		report(msg)
	}
}

