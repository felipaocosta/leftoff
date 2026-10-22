package fetch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/felipaocosta/leftoff/internal/domain"
)

// freshKey marks a context whose requests skip the cache.
type freshKey struct{}

// Fresh returns a context whose fetches always go to the site (a refresh
// the user asked for must see a chapter released a minute ago); the
// answers still update the cache.
func Fresh(ctx context.Context) context.Context { return context.WithValue(ctx, freshKey{}, true) }

func isFresh(ctx context.Context) bool { v, _ := ctx.Value(freshKey{}).(bool); return v }

// responseCache keeps successful responses in memory for a short TTL so the
// preview → save flow does not hit the site twice. Refreshes skip it (Fresh).
type responseCache struct {
	ttl        time.Duration
	maxEntries int
	clock      domain.Clock

	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	resp    *Response
	expires time.Time
}

func newResponseCache(ttl time.Duration, maxEntries int, clock domain.Clock) *responseCache {
	return &responseCache{ttl: ttl, maxEntries: maxEntries, clock: clock, entries: map[string]cacheEntry{}}
}

func cacheKey(req *Request) string {
	sum := sha256.Sum256(req.Body)
	return req.Method + " " + req.URL + " " + hex.EncodeToString(sum[:8])
}

func (c *responseCache) get(key string) (*Response, bool) {
	if c.ttl <= 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || !c.clock.Now().Before(e.expires) {
		return nil, false
	}
	return e.resp, true
}

func (c *responseCache) put(key string, resp *Response) {
	if c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock.Now()
	if len(c.entries) >= c.maxEntries {
		c.evict(now)
	}
	c.entries[key] = cacheEntry{resp: resp, expires: now.Add(c.ttl)}
}

// evict drops expired entries and, if still full, the one expiring soonest.
func (c *responseCache) evict(now time.Time) {
	var oldestKey string
	var oldest time.Time
	for k, e := range c.entries {
		if !now.Before(e.expires) {
			delete(c.entries, k)
			continue
		}
		if oldestKey == "" || e.expires.Before(oldest) {
			oldestKey, oldest = k, e.expires
		}
	}
	if len(c.entries) >= c.maxEntries {
		delete(c.entries, oldestKey)
	}
}
