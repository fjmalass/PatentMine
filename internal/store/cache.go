package store

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"patentmine/internal/domain"
)

// Cache is a Repository wrapper that keeps frequently accessed listings in
// memory. It is a "write-through" cache: every modification is forwarded to
// the underlying store, and the listing cache is flushed to ensure
// consistency.
//
// While caching is suspended (see SuspendCaching) listings are read straight
// through and never stored, so a long burst of writes — a family crawl — can
// never serve stale rows. One flush on resume restores normal operation.
type Cache struct {
	Repository
	mu    sync.RWMutex
	lists map[string]listResult
	// suspend counts active SuspendCaching holders; >0 means bypass the cache.
	suspend atomic.Int32
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

// SuspendCaching turns the listing cache into a pass-through. Calls nest: each
// SuspendCaching must be paired with a ResumeCaching. The crawler brackets a
// crawl with this pair so the thousands of writes a crawl makes cannot leave
// stale listings behind.
func (c *Cache) SuspendCaching() {
	c.suspend.Add(1)
}

// ResumeCaching releases one suspension. When the last holder releases, the
// cache is flushed so the next listing reflects every write made meanwhile.
func (c *Cache) ResumeCaching() {
	if c.suspend.Add(-1) <= 0 {
		c.flush()
	}
}

// suspended reports whether the cache is currently a pass-through.
func (c *Cache) suspended() bool {
	return c.suspend.Load() > 0
}

func (c *Cache) ListPatents(ctx context.Context, q PatentQuery) ([]domain.PatentRow, error) {
	if c.suspended() {
		return c.Repository.ListPatents(ctx, q)
	}
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
	if c.suspended() {
		return c.Repository.CountPatents(ctx, q)
	}
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

func (c *Cache) SaveNode(ctx context.Context, batch NodeBatch) error {
	if err := c.Repository.SaveNode(ctx, batch); err != nil {
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

func (c *Cache) SetReviewStates(ctx context.Context, project domain.ProjectID, patents []domain.PatentNumber, state domain.ReviewState) error {
	if err := c.Repository.SetReviewStates(ctx, project, patents, state); err != nil {
		return err
	}
	c.flush()
	return nil
}

func (c *Cache) TagPatents(ctx context.Context, tagID int64, patents []domain.PatentNumber, assignedAt time.Time) error {
	if err := c.Repository.TagPatents(ctx, tagID, patents, assignedAt); err != nil {
		return err
	}
	c.flush()
	return nil
}

func (c *Cache) UntagPatents(ctx context.Context, tagID int64, patents []domain.PatentNumber) error {
	if err := c.Repository.UntagPatents(ctx, tagID, patents); err != nil {
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
	// Marshal the full query so newly added PatentQuery fields automatically
	// participate in cache keys instead of relying on a hand-maintained format.
	encoded, err := json.Marshal(q)
	if err != nil {
		return fmt.Sprintf("%s:%#v", prefix, q)
	}
	return prefix + ":" + string(encoded)
}
