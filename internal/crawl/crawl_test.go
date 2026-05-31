package crawl

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/observability"
	"patentmine/internal/store"
	"patentmine/internal/store/sqlite"
)

func openRepo(t *testing.T) *sqlite.Repo {
	t.Helper()
	repo, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "crawl.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func newFileCrawler(t *testing.T, repo *sqlite.Repo, cfg CrawlConfig) *Crawler {
	t.Helper()
	registry := NewRegistry(NewFileSource("testdata"))
	return NewCrawler(registry, repo, cfg)
}

func TestCrawlWalksFamilyToDepth(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	crawler := newFileCrawler(t, repo, CrawlConfig{})

	var last Progress
	if err := crawler.Crawl(ctx, domain.MustParsePatentNumber("US0000001B2"), 1, domain.CrawlProfileAll, false,
		func(p Progress) { last = p }); err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	// US1 (root), US2, US3 fetched in full; US4 is beyond depth -> stub.
	for _, num := range []string{"US0000001B2", "US0000002B2", "US0000003B2"} {
		p, err := repo.Patent(ctx, domain.MustParsePatentNumber(num))
		if err != nil {
			t.Fatalf("patent %s missing: %v", num, err)
		}
		if p.FetchState != domain.FetchCached {
			t.Fatalf("patent %s fetch state = %s, want cached", num, p.FetchState)
		}
	}
	stub, err := repo.Patent(ctx, domain.MustParsePatentNumber("US0000004B2"))
	if err != nil {
		t.Fatalf("stub US0000004B2 missing: %v", err)
	}
	if stub.FetchState != domain.FetchStub {
		t.Fatalf("US0000004B2 fetch state = %s, want stub", stub.FetchState)
	}

	if last.CrawledCount != 3 || last.DiscoveredCount != 4 {
		t.Fatalf("final progress = %+v, want ingested 3 discovered 4", last)
	}

	// Family edges were recorded.
	cites, err := repo.Relations(ctx, domain.MustParsePatentNumber("US0000001B2"), domain.RelationCites)
	if err != nil {
		t.Fatalf("Relations: %v", err)
	}
	if len(cites) != 1 || !cites[0].To.Equal(domain.MustParsePatentNumber("US0000002B2")) {
		t.Fatalf("US1 cites = %v, want one edge to US2", cites)
	}
	parents, err := repo.Relations(ctx, domain.MustParsePatentNumber("US0000001B2"), domain.RelationParent)
	if err != nil {
		t.Fatalf("Relations: %v", err)
	}
	if len(parents) != 1 {
		t.Fatalf("US1 parents = %v, want one", parents)
	}
}

func TestCrawlDepthZeroFetchesOnlyRoot(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	crawler := newFileCrawler(t, repo, CrawlConfig{})

	var last Progress
	if err := crawler.Crawl(ctx, domain.MustParsePatentNumber("US0000001B2"), 0, domain.CrawlProfileAll, false,
		func(p Progress) { last = p }); err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if last.CrawledCount != 1 {
		t.Fatalf("depth-0 crawl fetched %d, want 1", last.CrawledCount)
	}
	// Neighbours are recorded as stubs, not fetched.
	neighbour, err := repo.Patent(ctx, domain.MustParsePatentNumber("US0000002B2"))
	if err != nil {
		t.Fatalf("neighbour stub missing: %v", err)
	}
	if neighbour.FetchState != domain.FetchStub {
		t.Fatalf("neighbour fetch state = %s, want stub", neighbour.FetchState)
	}
}

func TestCrawlRespectsNodeBudget(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	crawler := newFileCrawler(t, repo, CrawlConfig{NodeBudget: 2})

	var last Progress
	if err := crawler.Crawl(ctx, domain.MustParsePatentNumber("US0000001B2"), 5, domain.CrawlProfileAll, false,
		func(p Progress) { last = p }); err != nil {
		t.Fatalf("Crawl: %v", err)
	}
	if last.CrawledCount != 2 {
		t.Fatalf("crawl fetched %d, want 2 (node budget)", last.CrawledCount)
	}
}

func TestCrawlCancellation(t *testing.T) {
	repo := openRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	crawler := newFileCrawler(t, repo, CrawlConfig{})

	err := crawler.Crawl(ctx, domain.MustParsePatentNumber("US0000001B2"), 2, domain.CrawlProfileAll, false, nil)
	if err == nil {
		t.Fatal("Crawl on a cancelled context should return an error")
	}
}

func TestCrawlerImportFile(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	crawler := newFileCrawler(t, repo, CrawlConfig{})

	if err := crawler.ImportFile(ctx, "testdata/US0000001B2.json"); err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	root := domain.MustParsePatentNumber("US0000001B2")
	p, err := repo.Patent(ctx, root)
	if err != nil {
		t.Fatalf("Patent after import: %v", err)
	}
	if p.FetchState != domain.FetchCached {
		t.Fatalf("imported patent fetch state = %s, want cached", p.FetchState)
	}
	cites, err := repo.Relations(ctx, root, domain.RelationCites)
	if err != nil {
		t.Fatalf("Relations: %v", err)
	}
	if len(cites) != 1 {
		t.Fatalf("imported citation edges = %v, want one", cites)
	}
}

func TestCrawlReportsDepthMetricsAndLogs(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	metrics := observability.NewMetrics()
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	crawler := newFileCrawler(t, repo, CrawlConfig{}).WithMetrics(metrics).WithLogger(logger)

	if err := crawler.Crawl(ctx, domain.MustParsePatentNumber("US0000001B2"), 1, domain.CrawlProfileAll, false, nil); err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	snap := metrics.Snapshot()
	if snap.Gauges["crawl.crawler.depth"] != 1 {
		t.Fatalf("depth gauge = %d, want 1", snap.Gauges["crawl.crawler.depth"])
	}
	if snap.Gauges["crawl.crawler.max_depth"] != 1 {
		t.Fatalf("max depth gauge = %d, want 1", snap.Gauges["crawl.crawler.max_depth"])
	}
	logs := logBuf.String()
	if !strings.Contains(logs, `"msg":"crawl progress"`) || !strings.Contains(logs, `"depth":1`) || !strings.Contains(logs, `"max_depth":1`) {
		t.Fatalf("crawl logs missing depth fields\n%s", logs)
	}
}

func TestFileSourceMissingPatent(t *testing.T) {
	src := NewFileSource("testdata")
	_, err := src.Fetch(context.Background(), domain.MustParsePatentNumber("US9999999B2"))
	if err != ErrNotAvailable {
		t.Fatalf("missing file err = %v, want ErrNotAvailable", err)
	}
}

// TestIngestNodeRestampsSourceDataToResolvedRecord guards the citation-load
// path: a stub already exists under one number (e.g. a publication number a
// citation referenced) while the fetched Result is pre-stamped with a different
// record number (the USPTO path keys off the application number). ingestNode
// must resolve to the existing stub and rebind every source-data row to it, or
// the authority_identifier FK insert fails with constraint 787.
func TestIngestNodeRestampsSourceDataToResolvedRecord(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	crawler := newFileCrawler(t, repo, CrawlConfig{})

	appNumber := domain.MustParsePatentNumber("US10059406") // empty kind = application
	pubNumber := domain.MustParsePatentNumber("US20020106191A1")

	// A citation discovered the patent under its publication number, leaving a stub.
	if err := repo.SaveNode(ctx, store.NodeBatch{
		Stubs: []store.StubRecord{{Number: pubNumber, Stage: domain.StagePublication}},
	}); err != nil {
		t.Fatalf("seed stub: %v", err)
	}

	// The fetched result is keyed by the application number and pre-stamps its
	// source data with that number, but carries the publication document.
	res := Result{
		Patent: domain.Patent{Number: appNumber, Title: "Widget", FetchState: domain.FetchCached},
		Documents: []domain.Document{
			{Number: appNumber, Stage: domain.StageApplication},
			{Number: pubNumber, Stage: domain.StagePublication},
		},
		AuthorityIdentifiers: []domain.AuthorityIdentifier{{
			Authority: "US", IdentifierType: "application", Identifier: "10059406",
			RecordNumber: appNumber, Confidence: 100,
		}},
		USPTOApplication: &domain.USPTOApplication{ApplicationNumber: "10059406", RecordNumber: appNumber},
		SourceSnapshots:  []domain.SourceSnapshot{{ID: "snap-1", PatentNumber: appNumber, Source: "uspto_file_wrapper"}},
	}

	seen := map[domain.PatentNumber]bool{}
	var queue []node
	record, _, _, err := crawler.ingestNode(ctx, res, 0, 0, domain.CrawlProfileAll, seen, &queue)
	if err != nil {
		t.Fatalf("ingestNode: %v", err)
	}
	if !record.Equal(pubNumber) {
		t.Fatalf("resolved record = %s, want %s (the existing stub)", record, pubNumber)
	}

	p, err := repo.Patent(ctx, pubNumber)
	if err != nil {
		t.Fatalf("patent %s missing: %v", pubNumber, err)
	}
	if p.FetchState != domain.FetchCached || p.Title != "Widget" {
		t.Fatalf("patent state=%s title=%q, want cached/Widget", p.FetchState, p.Title)
	}
	// The application number must now resolve to the same record.
	if rec, err := repo.RecordOf(ctx, appNumber); err != nil || !rec.Equal(pubNumber) {
		t.Fatalf("RecordOf(%s) = %s, %v; want %s", appNumber, rec, err, pubNumber)
	}
}

type mockSource struct {
	name    domain.Source
	fetches []domain.PatentNumber
	results map[string]Result
	errs    map[string]error
}

func (m *mockSource) Name() domain.Source {
	return m.name
}

func (m *mockSource) Fetch(ctx context.Context, number domain.PatentNumber) (Result, error) {
	m.fetches = append(m.fetches, number)
	if err, ok := m.errs[number.Normalized()]; ok {
		return Result{}, err
	}
	return m.results[number.Normalized()], nil
}

func TestRegistryCompareWithGoogleFallback(t *testing.T) {
	appNumber := domain.MustParsePatentNumber("US12345678") // empty kind = StageApplication
	pubNumber := domain.MustParsePatentNumber("US20261234567A1")

	// USPTO source returns a result containing the application number and the publication document
	usptoSource := &mockSource{
		name: domain.SourceUSPTO,
		results: map[string]Result{
			appNumber.Normalized(): {
				Patent: domain.Patent{
					Number: appNumber,
				},
				Documents: []domain.Document{
					{Number: appNumber, Stage: domain.StageApplication},
					{Number: pubNumber, Stage: domain.StagePublication},
				},
			},
		},
	}

	// Google source expects to be fetched with pubNumber, not appNumber!
	googleSource := &mockSource{
		name: domain.SourceGoogle,
		results: map[string]Result{
			pubNumber.Normalized(): {
				Patent: domain.Patent{
					Number: pubNumber,
				},
				SourceSnapshots: []domain.SourceSnapshot{
					{PatentNumber: pubNumber, Source: "google"},
				},
			},
		},
		errs: map[string]error{
			appNumber.Normalized(): ErrNotAvailable, // appNumber will fail if fetched directly!
		},
	}

	registry := NewRegistry(usptoSource, googleSource)
	_ = registry.SetSourceMode(string(domain.SourceModeCompare))

	res, err := registry.Fetch(context.Background(), appNumber)
	if err != nil {
		t.Fatalf("registry.Fetch failed: %v", err)
	}

	// Verify that googleSource was fetched with pubNumber, not appNumber!
	if len(googleSource.fetches) != 1 {
		t.Fatalf("expected 1 fetch on Google, got %d", len(googleSource.fetches))
	}
	if !googleSource.fetches[0].Equal(pubNumber) {
		t.Errorf("expected Google to be fetched with %s, but got %s", pubNumber, googleSource.fetches[0])
	}

	// Verify that the merged snapshots reference the original appNumber
	if len(res.SourceSnapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(res.SourceSnapshots))
	}
	if !res.SourceSnapshots[0].PatentNumber.Equal(appNumber) {
		t.Errorf("expected snapshot PatentNumber to be rewritten to original %s, got %s", appNumber, res.SourceSnapshots[0].PatentNumber)
	}
}
