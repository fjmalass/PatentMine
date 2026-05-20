package store

import (
	"context"
	"fmt"
	"sync"

	"patentmine/internal/domain"
)

// Cache is a Repository wrapper that keeps frequently accessed listings in
// memory. It is a "write-through" cache: every modification is forwarded to
// the underlying store, and the listing cache is flushed to ensure
// consistency.
type Cache struct {
	Repository
	mu    sync.RWMutex
	lists map[string]listResult
}

type listResult struct {
	patents []domain.PatentRow
	total   int
}

// NewCache builds a caching wrapper around repo.
func NewCache(repo Repository) *Cache {
	return &Cache{
		Repository: repo,
		lists:      make(map[string]listResult),
	}
}

// flush clears every cached listing.
func (c *Cache) flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.lists)
}

func (c *Cache) ListPatents(ctx context.Context, q PatentQuery) ([]domain.PatentRow, error) {
	key := queryKey("list", q)
	c.mu.RLock()
	res, ok := c.lists[key]
	c.mu.RUnlock()
	if ok {
		return res.patents, nil
	}

	patents, err := c.Repository.ListPatents(ctx, q)
	if err != nil {
		return nil, err
	}

	// We can't cache the patents without the total if we want to satisfy
	// both ListPatents and CountPatents from the cache.
	total, err := c.Repository.CountPatents(ctx, q)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.lists[key] = listResult{patents: patents, total: total}
	c.mu.Unlock()
	return patents, nil
}

func (c *Cache) CountPatents(ctx context.Context, q PatentQuery) (int, error) {
	key := queryKey("list", q)
	c.mu.RLock()
	res, ok := c.lists[key]
	c.mu.RUnlock()
	if ok {
		return res.total, nil
	}

	total, err := c.Repository.CountPatents(ctx, q)
	if err != nil {
		return 0, err
	}

	// Speculatively load the first page to populate the cache if it's empty?
	// For now, just return the total. If ListPatents is called next, it will
	// populate the cache.
	return total, nil
}

// --- Write operations flush the cache ---

func (c *Cache) SavePatent(ctx context.Context, p domain.Patent) error {
	if err := c.Repository.SavePatent(ctx, p); err != nil {
		return err
	}
	c.flush()
	return nil
}

func (c *Cache) DeletePatent(ctx context.Context, n domain.PatentNumber) error {
	if err := c.Repository.DeletePatent(ctx, n); err != nil {
		return err
	}
	c.flush()
	return nil
}

func (c *Cache) SaveRelation(ctx context.Context, r domain.Relation) error {
	if err := c.Repository.SaveRelation(ctx, r); err != nil {
		return err
	}
	c.flush()
	return nil
}

func (c *Cache) SetReviewState(ctx context.Context, project domain.ProjectID, patent domain.PatentNumber, state domain.ReviewState) error {
	if err := c.Repository.SetReviewState(ctx, project, patent, state); err != nil {
		return err
	}
	c.flush()
	return nil
}

func (c *Cache) TagPatent(ctx context.Context, tagID int64, patent domain.PatentNumber) error {
	if err := c.Repository.TagPatent(ctx, tagID, patent); err != nil {
		return err
	}
	c.flush()
	return nil
}

func (c *Cache) UntagPatent(ctx context.Context, tagID int64, patent domain.PatentNumber) error {
	if err := c.Repository.UntagPatent(ctx, tagID, patent); err != nil {
		return err
	}
	c.flush()
	return nil
}

func (c *Cache) SaveDocument(ctx context.Context, recordNumber domain.PatentNumber, doc domain.Document) error {
	if err := c.Repository.SaveDocument(ctx, recordNumber, doc); err != nil {
		return err
	}
	c.flush()
	return nil
}

func (c *Cache) MergeRecords(ctx context.Context, keep, absorb domain.PatentNumber) error {
	if err := c.Repository.MergeRecords(ctx, keep, absorb); err != nil {
		return err
	}
	c.flush()
	return nil
}

func (c *Cache) AddMembership(ctx context.Context, m domain.Membership) error {
	if err := c.Repository.AddMembership(ctx, m); err != nil {
		return err
	}
	c.flush()
	return nil
}

func (c *Cache) CreateTag(ctx context.Context, project domain.ProjectID, name string) (domain.Tag, error) {
	tag, err := c.Repository.CreateTag(ctx, project, name)
	if err != nil {
		return domain.Tag{}, err
	}
	c.flush()
	return tag, nil
}

func (c *Cache) DeleteTag(ctx context.Context, project domain.ProjectID, name string) error {
	if err := c.Repository.DeleteTag(ctx, project, name); err != nil {
		return err
	}
	c.flush()
	return nil
}

// queryKey builds a stable string key for a listing query.
func queryKey(prefix string, q PatentQuery) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s:%s:%d:%d:%s:%v",
		prefix, q.Project, q.ReviewState, q.Relation, q.RelationKind,
		q.Search, q.Limit, q.Offset, q.SortColumn, q.SortAscending)
}
