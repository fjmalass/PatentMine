package sqlite

import (
	"context"
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

func TestListPatentsSortByKind(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	// Create a few patents of different stages/kinds
	// p1: grant (US11795648B2 has kind B2 -> rank 1)
	p1 := domain.Patent{
		Number:     domain.MustParsePatentNumber("US11795648B2"),
		Title:      "Grant Patent",
		FetchState: domain.FetchCached,
	}
	// p2: application (US15603387 has kind "" and digits length 8 -> rank 3)
	p2 := domain.Patent{
		Number:     domain.MustParsePatentNumber("US15603387"),
		Title:      "Application Patent",
		FetchState: domain.FetchCached,
	}
	// p3: publication (US20040260504A1 has kind A1 -> rank 2)
	p3 := domain.Patent{
		Number:     domain.MustParsePatentNumber("US20040260504A1"),
		Title:      "Publication Patent",
		FetchState: domain.FetchCached,
	}

	for _, p := range []domain.Patent{p1, p2, p3} {
		if err := repo.SavePatent(ctx, p); err != nil {
			t.Fatalf("SavePatent: %v", err)
		}
	}

	// 1. Sort by KIND ASC (rank order: grant (1) -> publication (2) -> application (3))
	rows, err := repo.ListPatents(ctx, store.PatentQuery{
		SortColumn:    domain.SortByKind,
		SortAscending: true,
	})
	if err != nil {
		t.Fatalf("ListPatents sort by kind ASC: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[0].Number != p1.Number || rows[1].Number != p3.Number || rows[2].Number != p2.Number {
		t.Fatalf("expected ASC order: grant, publication, application. Got: %v, %v, %v", rows[0].Number, rows[1].Number, rows[2].Number)
	}

	// 2. Sort by KIND DESC (rank order: application (3) -> publication (2) -> grant (1))
	rows, err = repo.ListPatents(ctx, store.PatentQuery{
		SortColumn:    domain.SortByKind,
		SortAscending: false,
	})
	if err != nil {
		t.Fatalf("ListPatents sort by kind DESC: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[0].Number != p2.Number || rows[1].Number != p3.Number || rows[2].Number != p1.Number {
		t.Fatalf("expected DESC order: application, publication, grant. Got: %v, %v, %v", rows[0].Number, rows[1].Number, rows[2].Number)
	}
}
