package ingest

import (
	"context"
	"path/filepath"
	"testing"

	"patentmine/internal/domain"
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

	if last.IngestedCount != 3 || last.DiscoveredCount != 4 {
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
	if last.IngestedCount != 1 {
		t.Fatalf("depth-0 crawl fetched %d, want 1", last.IngestedCount)
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
	if last.IngestedCount != 2 {
		t.Fatalf("crawl fetched %d, want 2 (node budget)", last.IngestedCount)
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

func TestFileSourceMissingPatent(t *testing.T) {
	src := NewFileSource("testdata")
	_, err := src.Fetch(context.Background(), domain.MustParsePatentNumber("US9999999B2"))
	if err != ErrNotAvailable {
		t.Fatalf("missing file err = %v, want ErrNotAvailable", err)
	}
}
