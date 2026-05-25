package sqlite

import (
	"context"
	"errors"
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

func TestSavedTableViewRoundTripAndDefault(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	first, err := repo.SaveTableView(ctx, domain.SavedTableView{
		Owner:     "alice",
		TableType: domain.TablePatents,
		Name:      "Expiring soon",
		IsDefault: true,
		ViewJSON:  []byte(`{"search":"sensor","sort":[{"field":"expirationDate","direction":"asc"}]}`),
	})
	if err != nil {
		t.Fatalf("SaveTableView first: %v", err)
	}
	if first.ID == "" || !first.IsDefault || first.CreatedAt.IsZero() || first.UpdatedAt.IsZero() {
		t.Fatalf("saved first = %+v", first)
	}

	second, err := repo.SaveTableView(ctx, domain.SavedTableView{
		Owner:     "alice",
		TableType: domain.TablePatents,
		Name:      "Recently filed",
		IsDefault: true,
		ViewJSON:  []byte(`{"filters":{"status":["active"]}}`),
	})
	if err != nil {
		t.Fatalf("SaveTableView second: %v", err)
	}

	views, err := repo.ListTableViews(ctx, "alice", domain.TablePatents)
	if err != nil {
		t.Fatalf("ListTableViews: %v", err)
	}
	if len(views) != 2 {
		t.Fatalf("ListTableViews returned %d, want 2", len(views))
	}
	defaults := 0
	for _, view := range views {
		if view.IsDefault {
			defaults++
			if view.ID != second.ID {
				t.Fatalf("default view id = %q, want %q", view.ID, second.ID)
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("default count = %d, want 1", defaults)
	}

	got, err := repo.TableView(ctx, "alice", first.ID)
	if err != nil {
		t.Fatalf("TableView: %v", err)
	}
	if got.Name != first.Name || got.TableType != domain.TablePatents {
		t.Fatalf("TableView = %+v, want %+v", got, first)
	}
}

func TestDeleteTableView(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	view, err := repo.SaveTableView(ctx, domain.SavedTableView{
		TableType: domain.TableIDSActivityHistory,
		Name:      "Needs Attention",
		ViewJSON:  []byte(`{"filters":{"status":["failed"]}}`),
	})
	if err != nil {
		t.Fatalf("SaveTableView: %v", err)
	}
	if err := repo.DeleteTableView(ctx, domain.DefaultTableViewOwner, view.ID); err != nil {
		t.Fatalf("DeleteTableView: %v", err)
	}
	if _, err := repo.TableView(ctx, domain.DefaultTableViewOwner, view.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("TableView after delete err = %v, want ErrNotFound", err)
	}
}
