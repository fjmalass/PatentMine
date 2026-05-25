package store_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"patentmine/internal/domain"
	"patentmine/internal/store"
	storesqlite "patentmine/internal/store/sqlite"
)

func openCachedTestRepo(t *testing.T) *store.Cache {
	t.Helper()
	repo, err := storesqlite.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return store.NewCache(repo)
}

func sampleCachedPatent(number string) domain.Patent {
	return domain.Patent{
		Number:     domain.MustParsePatentNumber(number),
		Title:      "A method for " + number,
		Abstract:   "abstract text",
		Assignee:   "Acme Corp",
		Inventors:  []domain.Inventor{"Ada Lovelace", "Alan Turing"},
		FetchState: domain.FetchCached,
		Source:     domain.SourceFile,
		GrantDate:  time.Date(2023, 3, 21, 0, 0, 0, 0, time.UTC),
		FetchedAt:  time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC),
	}
}

func TestCacheSeparatesInventorQueries(t *testing.T) {
	repo := openCachedTestRepo(t)
	ctx := context.Background()

	p := sampleCachedPatent("US11611785B2")
	p.Inventors = []domain.Inventor{"Ada Lovelace"}
	if err := repo.SavePatent(ctx, p); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}

	miss, err := repo.ListPatents(ctx, store.PatentQuery{Inventor: "Grace Hopper"})
	if err != nil {
		t.Fatalf("ListPatents inventor miss: %v", err)
	}
	if len(miss) != 0 {
		t.Fatalf("inventor miss returned %d rows, want 0", len(miss))
	}

	all, err := repo.ListPatents(ctx, store.PatentQuery{})
	if err != nil {
		t.Fatalf("ListPatents all: %v", err)
	}
	if len(all) != 1 || all[0].Number != p.Number {
		t.Fatalf("unfiltered list = %v, want patent %v", all, p.Number)
	}

	total, err := repo.CountPatents(ctx, store.PatentQuery{})
	if err != nil {
		t.Fatalf("CountPatents all: %v", err)
	}
	if total != 1 {
		t.Fatalf("CountPatents all = %d, want 1", total)
	}
}

func TestCacheSeparatesClassificationQueries(t *testing.T) {
	repo := openCachedTestRepo(t)
	ctx := context.Background()

	p := sampleCachedPatent("US11611786B2")
	p.Classifications = []string{"G06F16/00"}
	if err := repo.SavePatent(ctx, p); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}

	miss, err := repo.ListPatents(ctx, store.PatentQuery{Classification: "A61K"})
	if err != nil {
		t.Fatalf("ListPatents classification miss: %v", err)
	}
	if len(miss) != 0 {
		t.Fatalf("classification miss returned %d rows, want 0", len(miss))
	}

	all, err := repo.ListPatents(ctx, store.PatentQuery{})
	if err != nil {
		t.Fatalf("ListPatents all: %v", err)
	}
	if len(all) != 1 || all[0].Number != p.Number {
		t.Fatalf("unfiltered list = %v, want patent %v", all, p.Number)
	}

	total, err := repo.CountPatents(ctx, store.PatentQuery{})
	if err != nil {
		t.Fatalf("CountPatents all: %v", err)
	}
	if total != 1 {
		t.Fatalf("CountPatents all = %d, want 1", total)
	}
}

func TestCacheFlushesPatentRowsForIDSChanges(t *testing.T) {
	repo := openCachedTestRepo(t)
	ctx := context.Background()

	project := domain.Project{ID: "p-1", Name: "Case A"}
	if err := repo.SaveProject(ctx, project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	patent := sampleCachedPatent("US11611787B2")
	if err := repo.SavePatent(ctx, patent); err != nil {
		t.Fatalf("SavePatent: %v", err)
	}
	if err := repo.AddMembership(ctx, domain.Membership{Project: project.ID, Patent: patent.Number, ReviewState: domain.ReviewStateUnknown}); err != nil {
		t.Fatalf("AddMembership: %v", err)
	}

	rows, err := repo.ListPatents(ctx, store.PatentQuery{Project: project.ID})
	if err != nil {
		t.Fatalf("ListPatents initial: %v", err)
	}
	if len(rows) != 1 || rows[0].IDSEntry != nil {
		t.Fatalf("initial IDS row = %+v, want no IDS entry", rows)
	}

	if _, err := repo.SaveIDSEntry(ctx, domain.IDSEntry{Project: project.ID, Patent: patent.Number, Status: domain.IDSEntryPending}); err != nil {
		t.Fatalf("SaveIDSEntry pending: %v", err)
	}
	rows, err = repo.ListPatents(ctx, store.PatentQuery{Project: project.ID})
	if err != nil {
		t.Fatalf("ListPatents pending: %v", err)
	}
	if len(rows) != 1 || rows[0].IDSEntry == nil || rows[0].IDSEntry.Status != domain.IDSEntryPending {
		t.Fatalf("pending IDS row = %+v, want pending", rows)
	}

	if _, err := repo.SaveIDSEntry(ctx, domain.IDSEntry{Project: project.ID, Patent: patent.Number, Status: domain.IDSEntryAccepted}); err != nil {
		t.Fatalf("SaveIDSEntry accepted: %v", err)
	}
	rows, err = repo.ListPatents(ctx, store.PatentQuery{Project: project.ID})
	if err != nil {
		t.Fatalf("ListPatents accepted: %v", err)
	}
	if len(rows) != 1 || rows[0].IDSEntry == nil || rows[0].IDSEntry.Status != domain.IDSEntryAccepted {
		t.Fatalf("accepted IDS row = %+v, want accepted", rows)
	}

	if err := repo.DeleteIDSEntry(ctx, project.ID, patent.Number); err != nil {
		t.Fatalf("DeleteIDSEntry: %v", err)
	}
	rows, err = repo.ListPatents(ctx, store.PatentQuery{Project: project.ID})
	if err != nil {
		t.Fatalf("ListPatents deleted: %v", err)
	}
	if len(rows) != 1 || rows[0].IDSEntry != nil {
		t.Fatalf("deleted IDS row = %+v, want no IDS entry", rows)
	}
}
