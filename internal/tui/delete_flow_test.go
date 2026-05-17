package tui

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"patentmine/internal/changes"
	"patentmine/internal/domain"
	"patentmine/internal/storage"
	"patentmine/internal/storage/sqlite"
)

// newSeededDeleteModel builds a viewList Model backed by an in-memory sqlite
// repo holding `seeded` stored patents (US0000..). More than one page (default
// pageSize 20) keeps the page full after a delete — the scenario users hit.
func newSeededDeleteModel(t *testing.T, seeded int) (*Model, storage.Repository, context.Context) {
	t.Helper()
	ctx := context.Background()
	repo, err := sqlite.Open(":memory:", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.Setup(ctx); err != nil {
		t.Fatal(err)
	}
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
		logger:     slog.Default(),
		ProjectID:  "default",
		mode:       viewList,
		text:       EnglishText(),
		listFilter: defaultPatentFilter(),
	}
	m = m.reloadPatents()
	if len(m.patents) != seeded {
		t.Fatalf("seed: expected %d patents, got %d", seeded, len(m.patents))
	}
	return m, repo, ctx
}

func listNumbers(t *testing.T, repo storage.Repository, ctx context.Context, filter string) map[string]bool {
	t.Helper()
	got, err := repo.ListPatents(ctx, "default", storage.ListPatentsOptions{ReviewStateFilter: filter})
	if err != nil {
		t.Fatal(err)
	}
	set := make(map[string]bool, len(got))
	for _, p := range got {
		set[p.Number] = true
	}
	return set
}

// TestListDeleteSoftDeletesPatent drives the delete flow (D -> y) and asserts
// the patent becomes a soft-delete tombstone: gone from the default list and
// the "all" filter, but still in the DB under the explicit deleted filter.
func TestListDeleteSoftDeletesPatent(t *testing.T) {
	const seeded = 25
	m, repo, ctx := newSeededDeleteModel(t, seeded)
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

	// Tombstone: hidden from the default (stored) list, but visible under the
	// unfiltered "none" view and the explicit deleted filter.
	if !listNumbers(t, repo, ctx, storage.ReviewStateFilterNone)[target] {
		t.Fatalf("deleted patent %s missing from the 'none' filter (none = no filter at all)", target)
	}
	if !listNumbers(t, repo, ctx, domain.ReviewStateDeleted)[target] {
		t.Fatalf("deleted patent %s missing from the deleted filter (should be a recoverable tombstone)", target)
	}
}

// TestVisualBulkDeleteSoftDeletesPatents drives the visual-selection delete
// flow (v -> j -> D -> y) and asserts the selected patents become tombstones —
// matching single-delete semantics.
func TestVisualBulkDeleteSoftDeletesPatents(t *testing.T) {
	const seeded = 25
	m, repo, ctx := newSeededDeleteModel(t, seeded)
	targets := []string{m.patents[0].Number, m.patents[1].Number}

	updated, _ := m.Update(teaKey(keyVisual))
	updated, _ = updated.(*Model).Update(teaKey(keyVimDown))
	updated, _ = updated.(*Model).Update(teaKey(keyDelete))
	if updated.(*Model).mode != viewBulkConfirm {
		t.Fatalf("visual D did not enter bulk-confirm, got %q", updated.(*Model).mode)
	}
	updated, _ = updated.(*Model).Update(teaKey(keyYes))
	got := updated.(*Model)

	if len(got.patents) != seeded-2 {
		t.Fatalf("expected %d patents after bulk delete, got %d", seeded-2, len(got.patents))
	}
	deleted := listNumbers(t, repo, ctx, domain.ReviewStateDeleted)
	all := listNumbers(t, repo, ctx, storage.ReviewStateFilterNone)
	for _, target := range targets {
		for _, p := range got.patents {
			if p.Number == target {
				t.Fatalf("bulk-deleted patent %s still present in default list", target)
			}
		}
		if !all[target] {
			t.Fatalf("bulk-deleted patent %s missing from the 'none' filter", target)
		}
		if !deleted[target] {
			t.Fatalf("bulk-deleted patent %s missing from the deleted filter", target)
		}
	}
}

// TestCompactPurgesDeletedPatents soft-deletes a patent, runs :compact, and
// asserts the tombstone is hard-deleted from the DB while survivors remain.
func TestCompactPurgesDeletedPatents(t *testing.T) {
	const seeded = 25
	m, repo, ctx := newSeededDeleteModel(t, seeded)
	target := m.patents[0].Number
	survivor := m.patents[1].Number

	updated, _ := m.Update(teaKey(keyDelete))
	updated, _ = updated.(*Model).Update(teaKey(keyYes))
	m = updated.(*Model)
	if !listNumbers(t, repo, ctx, domain.ReviewStateDeleted)[target] {
		t.Fatalf("setup: %s should be a tombstone before compact", target)
	}

	if _, _ = m.runCommand(ParseCommand(commandCompact)); m.err != "" {
		t.Fatalf("compact reported error: %s", m.err)
	}

	if listNumbers(t, repo, ctx, domain.ReviewStateDeleted)[target] {
		t.Fatalf("compact left tombstone %s behind", target)
	}
	if _, err := repo.GetPatent(ctx, "default", target); err == nil {
		t.Fatalf("compact did not hard-delete %s from the DB", target)
	}
	if _, err := repo.GetPatent(ctx, "default", survivor); err != nil {
		t.Fatalf("compact removed survivor %s: %v", survivor, err)
	}
}
