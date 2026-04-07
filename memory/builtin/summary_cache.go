package builtin

import (
	"container/list"
	"sync"
	"time"
)

const (
	defaultSessionSummaryCacheTTL        = 10 * time.Minute
	defaultSessionSummaryCacheMaxEntries = 10000
)

type sessionSummaryCache struct {
	ttl        time.Duration
	maxEntries int

	mu      sync.Mutex
	entries map[string]*list.Element // key -> element
	order   *list.List               // LRU order: front = most recent, back = least recent
}

type sessionSummaryCacheEntry struct {
	key       string
	summary   *SessionSummary
	expiresAt time.Time
}

func newSessionSummaryCache(ttl time.Duration, maxEntries int) *sessionSummaryCache {
	if ttl <= 0 {
		ttl = defaultSessionSummaryCacheTTL
	}
	if maxEntries <= 0 {
		maxEntries = defaultSessionSummaryCacheMaxEntries
	}
	return &sessionSummaryCache{
		ttl:        ttl,
		maxEntries: maxEntries,
		entries:    make(map[string]*list.Element),
		order:      list.New(),
	}
}

func (c *sessionSummaryCache) Get(key string) (*SessionSummary, bool) {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	entry := elem.Value.(*sessionSummaryCacheEntry)
	if now.After(entry.expiresAt) {
		c.removeElement(elem)
		return nil, false
	}

	// Move to front (most recently accessed)
	c.order.MoveToFront(elem)
	return cloneSessionSummary(entry.summary), true
}

func (c *sessionSummaryCache) Set(key string, summary *SessionSummary) {
	if summary == nil {
		return
	}

	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	// If key already exists, remove old entry first
	if elem, ok := c.entries[key]; ok {
		c.removeElement(elem)
	}

	entry := &sessionSummaryCacheEntry{
		key:       key,
		summary:   cloneSessionSummary(summary),
		expiresAt: now.Add(c.ttl),
	}
	elem := c.order.PushFront(entry)
	c.entries[key] = elem

	c.evict()
}

func (c *sessionSummaryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[key]; ok {
		c.removeElement(elem)
	}
}

func (c *sessionSummaryCache) removeElement(elem *list.Element) {
	c.order.Remove(elem)
	entry := elem.Value.(*sessionSummaryCacheEntry)
	delete(c.entries, entry.key)
}

// evict removes all expired entries and then evicts LRU entries if over capacity.
// Must be called with c.mu held.
func (c *sessionSummaryCache) evict() {
	now := time.Now()

	// Expiration is independent from LRU ordering, so scan the full list.
	for elem := c.order.Back(); elem != nil; {
		prev := elem.Prev()
		entry := elem.Value.(*sessionSummaryCacheEntry)
		if now.After(entry.expiresAt) {
			c.removeElement(elem)
		}
		elem = prev
	}

	// Evict LRU entries if still over capacity
	for c.order.Len() > c.maxEntries {
		c.removeElement(c.order.Back())
	}
}

func cloneSessionSummary(summary *SessionSummary) *SessionSummary {
	if summary == nil {
		return nil
	}
	cloned := *summary
	return &cloned
}
