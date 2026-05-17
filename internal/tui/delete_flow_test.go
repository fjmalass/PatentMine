package tui

import (
	"context"
	"fmt"
	"testing"

	"patentmine/internal/changes"
	"patentmine/internal/domain"
	"patentmine/internal/storage"
	"patentmine/internal/storage/sqlite"
)

// TestListDeleteRemovesPatentFromListAndDB drives the real delete flow
// (D -> confirm -> y) against an in-memory sqlite repo and asserts the patent
// leaves both m.patents and the stored DB list.
func TestListDeleteRemovesPatentFromListAndDB(t *testing.T) {
	ctx := context.Background()
	repo, err := sqlite.Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.Setup(ctx); err != nil {
		t.Fatal(err)
	}

	// Seed more than one page (default pageSize is 20) so the page stays full
	// after a delete — the scenario the user reported.
	const seeded = 25
	for i := 0; i < seeded; i++ {
		num := fmt.Sprintf("US%04d", i)
		if err := repo.UpsertPatentBundle(ctx, "default", domain.PatentBundle{
			Patent: domain.Patent{Number: num, Title: "Patent " + num},
		}); err != nil {
			t.Fatal(err)
		}
	}

	m := &Model{
		ctx:        ctx,
		repo:       repo,
		history:    changes.NewHistory(repo),
		ProjectID:  "default",
		mode:       viewList,
		text:       EnglishText(),
		listFilter: defaultPatentFilter(),
	}
	m = m.reloadPatents()
	if len(m.patents) != seeded {
		t.Fatalf("seed: expected %d patents, got %d", seeded, len(m.patents))
	}
	target := m.patents[0].Number

	updated, _ := m.Update(teaKey(keyDelete))
	if updated.(*Model).mode != viewConfirmDelete {
		t.Fatalf("D did not enter confirm-delete, got %q", updated.(*Model).mode)
	}
	updated, _ = updated.(*Model).Update(teaKey(keyYes))
	got := updated.(*Model)

	if len(got.patents) != seeded-1 {
		t.Fatalf("expected %d patents after delete, got %d", seeded-1, len(got.patents))
	}
	for _, p := range got.patents {
		if p.Number == target {
			t.Fatalf("deleted patent %s still present in list", target)
		}
	}

	// Hard delete: gone from every review-state filter, not just "stored".
	all, err := repo.ListPatents(ctx, "default", storage.ListPatentsOptions{
		ReviewStateFilter: storage.ReviewStateFilterNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		if p.Number == target {
			t.Fatalf("deleted patent %s still in DB (project_patents row not removed)", target)
		}
	}
}
