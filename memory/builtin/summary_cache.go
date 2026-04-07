package builtin

import (
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
	entries map[string]*sessionSummaryCacheEntry
}

type sessionSummaryCacheEntry struct {
	summary    *SessionSummary
	expiresAt  time.Time
	lastAccess time.Time
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
		entries:    make(map[string]*sessionSummaryCacheEntry),
	}
}

func (c *sessionSummaryCache) Get(key string) (*SessionSummary, bool) {
	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if now.After(entry.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}

	entry.lastAccess = now
	return cloneSessionSummary(entry.summary), true
}

func (c *sessionSummaryCache) Set(key string, summary *SessionSummary) {
	if summary == nil {
		return
	}

	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &sessionSummaryCacheEntry{
		summary:    cloneSessionSummary(summary),
		expiresAt:  now.Add(c.ttl),
		lastAccess: now,
	}
	c.prune(now)
}

func (c *sessionSummaryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

func (c *sessionSummaryCache) prune(now time.Time) {
	if len(c.entries) == 0 {
		return
	}

	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}

	for len(c.entries) > c.maxEntries {
		var oldestKey string
		var oldest time.Time
		first := true
		for key, entry := range c.entries {
			if first || entry.lastAccess.Before(oldest) {
				first = false
				oldestKey = key
				oldest = entry.lastAccess
			}
		}
		if oldestKey == "" {
			return
		}
		delete(c.entries, oldestKey)
	}
}

func cloneSessionSummary(summary *SessionSummary) *SessionSummary {
	if summary == nil {
		return nil
	}
	cloned := *summary
	return &cloned
}
