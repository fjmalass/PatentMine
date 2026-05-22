package sqlite

import (
	"context"
	"errors"
	"testing"

	"patentmine/internal/domain"
	"patentmine/internal/store"
)

func TestClassificationPersistence(t *testing.T) {
	repo := openTestRepo(t)
	ctx := context.Background()

	// 1. Initial list should be empty
	list, err := repo.ListClassificationDefinitions(ctx)
	if err != nil {
		t.Fatalf("ListClassificationDefinitions: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d definitions", len(list))
	}

	// 2. Querying non-existent definition should return store.ErrNotFound
	_, err = repo.ClassificationDefinition(ctx, "CPC", "G06F17/30")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}

	// 3. Save a new classification definition
	c := domain.Classification{
		System:      "CPC",
		Code:        "G06F17/30",
		Section:     "G",
		Class:       "06",
		Subclass:    "F",
		MainGroup:   "17",
		Subgroup:    "30",
		Description: "Information retrieval",
	}

	err = repo.SaveClassificationDefinition(ctx, c)
	if err != nil {
		t.Fatalf("SaveClassificationDefinition: %v", err)
	}

	// 4. Retrieve it and check matching fields
	got, err := repo.ClassificationDefinition(ctx, "CPC", "G06F17/30")
	if err != nil {
		t.Fatalf("ClassificationDefinition: %v", err)
	}
	if got.Description != c.Description || got.System != c.System || got.Subclass != c.Subclass {
		t.Errorf("ClassificationDefinition returned unexpected content: %+v", got)
	}

	// 5. Test ListClassificationDefinitions
	list, err = repo.ListClassificationDefinitions(ctx)
	if err != nil {
		t.Fatalf("ListClassificationDefinitions: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 definition, got %d", len(list))
	}
	if list[0].Code != "G06F17/30" {
		t.Errorf("expected code G06F17/30, got %s", list[0].Code)
	}

	// 6. Save update/conflict
	c.Description = "Updated description"
	err = repo.SaveClassificationDefinition(ctx, c)
	if err != nil {
		t.Fatalf("SaveClassificationDefinition update: %v", err)
	}

	got, err = repo.ClassificationDefinition(ctx, "CPC", "G06F17/30")
	if err != nil {
		t.Fatalf("ClassificationDefinition after update: %v", err)
	}
	if got.Description != "Updated description" {
		t.Errorf("expected 'Updated description', got %q", got.Description)
	}

	// 7. Delete
	err = repo.DeleteClassificationDefinition(ctx, "CPC", "G06F17/30")
	if err != nil {
		t.Fatalf("DeleteClassificationDefinition: %v", err)
	}

	_, err = repo.ClassificationDefinition(ctx, "CPC", "G06F17/30")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("expected ErrNotFound after deletion, got %v", err)
	}
}
