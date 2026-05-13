package sqlite

import (
	"context"
	"testing"

	"patentmine/internal/domain"
)

func TestTagCRUD(t *testing.T) {
	ctx := context.Background()
	repo := newTestRepo(t)

	projectID := "default"
	tagName := "AI"
	tagColor := "blue"

	// Create
	tagID, err := repo.CreateTag(ctx, projectID, tagName, tagColor)
	if err != nil {
		t.Fatalf("failed to create tag: %v", err)
	}

	// List
	tags, err := repo.ListTagsWithCounts(ctx, projectID)
	if err != nil {
		t.Fatalf("failed to list tags: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
	if tags[0].Name != tagName || tags[0].Color != tagColor {
		t.Errorf("expected tag AI/blue, got %s/%s", tags[0].Name, tags[0].Color)
	}

	// Rename
	newName := "Machine Learning"
	if err := repo.RenameTag(ctx, tagID, newName); err != nil {
		t.Fatalf("failed to rename tag: %v", err)
	}
	tags, _ = repo.ListTagsWithCounts(ctx, projectID)
	if tags[0].Name != newName {
		t.Errorf("expected renamed tag %s, got %s", newName, tags[0].Name)
	}

	// Apply to patent
	patentNum := "US1234567"
	err = repo.UpsertPatentBundle(ctx, projectID, domain.PatentBundle{
		Patent: domain.Patent{Number: patentNum, Title: "Test Patent"},
	})
	if err != nil {
		t.Fatalf("failed to create test patent: %v", err)
	}

	if err := repo.ApplyTagToPatent(ctx, patentNum, tagID); err != nil {
		t.Fatalf("failed to apply tag: %v", err)
	}

	// Verify counts
	tags, _ = repo.ListTagsWithCounts(ctx, projectID)
	if tags[0].PatentCount != 1 {
		t.Errorf("expected patent count 1, got %d", tags[0].PatentCount)
	}

	// Get for patent
	patentTags, err := repo.GetPatentTags(ctx, patentNum)
	if err != nil {
		t.Fatalf("failed to get patent tags: %v", err)
	}
	if len(patentTags) != 1 || patentTags[0].ID != tagID {
		t.Errorf("expected 1 patent tag with ID %d, got %v", tagID, patentTags)
	}

	// Remove from patent
	if err := repo.RemoveTagFromPatent(ctx, patentNum, tagID); err != nil {
		t.Fatalf("failed to remove tag: %v", err)
	}
	tags, _ = repo.ListTagsWithCounts(ctx, projectID)
	if tags[0].PatentCount != 0 {
		t.Errorf("expected patent count 0 after removal, got %d", tags[0].PatentCount)
	}

	// Delete
	if err := repo.DeleteTag(ctx, tagID); err != nil {
		t.Fatalf("failed to delete tag: %v", err)
	}
	tags, _ = repo.ListTagsWithCounts(ctx, projectID)
	if len(tags) != 0 {
		t.Errorf("expected 0 tags after deletion, got %d", len(tags))
	}
}
