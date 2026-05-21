package crawl

import (
	"context"
	"errors"
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

func TestCrawlUnifiesApplicationAndGrant(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	crawler := newFileCrawler(t, repo, CrawlConfig{})

	// The grant file lists both the application and the grant as documents.
	grant := domain.MustParsePatentNumber("US0000010B2")
	application := domain.MustParsePatentNumber("US16000010")
	if err := crawler.Crawl(ctx, grant, 0, domain.CrawlProfileAll, false, nil); err != nil {
		t.Fatalf("Crawl: %v", err)
	}

	// One record carries both documents.
	record, err := repo.Patent(ctx, grant)
	if err != nil {
		t.Fatalf("Patent: %v", err)
	}
	if len(record.Documents) != 2 {
		t.Fatalf("record has %d documents, want 2", len(record.Documents))
	}
	// It is shown by the grant number.
	if !record.DisplayNumber.Equal(grant) {
		t.Fatalf("display number = %s, want the grant %s", record.DisplayNumber, grant)
	}
	// Either number resolves to the one record.
	for _, n := range []domain.PatentNumber{application, grant} {
		got, err := repo.RecordOf(ctx, n)
		if err != nil {
			t.Fatalf("RecordOf(%s): %v", n, err)
		}
		if !got.Equal(grant) {
			t.Fatalf("RecordOf(%s) = %s, want %s", n, got, grant)
		}
	}
}

func TestCrawlFoldsGrantIntoExistingApplicationRecord(t *testing.T) {
	repo := openRepo(t)
	ctx := context.Background()
	crawler := newFileCrawler(t, repo, CrawlConfig{})

	application := domain.MustParsePatentNumber("US16000010")
	grant := domain.MustParsePatentNumber("US0000010B2")

	// The application is loaded one day...
	if err := crawler.Crawl(ctx, application, 0, domain.CrawlProfileAll, false, nil); err != nil {
		t.Fatalf("Crawl application: %v", err)
	}
	// ...and the grant later. It must fold into the existing record.
	if err := crawler.Crawl(ctx, grant, 0, domain.CrawlProfileAll, false, nil); err != nil {
		t.Fatalf("Crawl grant: %v", err)
	}

	// The record key never changed: it is still the application number.
	record, err := repo.Patent(ctx, application)
	if err != nil {
		t.Fatalf("Patent(application): %v", err)
	}
	if len(record.Documents) != 2 {
		t.Fatalf("record has %d documents, want 2 after folding in the grant", len(record.Documents))
	}
	if !record.DisplayNumber.Equal(grant) {
		t.Fatalf("display number = %s, want the grant %s", record.DisplayNumber, grant)
	}
	// There is exactly one record — the grant number is not its own record.
	if _, err := repo.Patent(ctx, grant); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("grant should not be a separate record: err = %v", err)
	}
}
