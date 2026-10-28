package qloo

import (
	"sync"
	"time"
)

// cacheMaxBytes bounds the response bodies the cache holds. A compare
// response alone is ~44 KB, and the demo runs for weeks.
const cacheMaxBytes = 64 << 20

type cacheEntry struct {
	body    []byte
	expires time.Time
}

// ttlCache is a process-local response cache keyed by request URL. When a
// new body would push it past maxBytes, it drops expired entries and then,
// if still full, the entries closest to expiring, which are the oldest.
type ttlCache struct {
	ttl      time.Duration
	maxBytes int
	mu       sync.RWMutex
	m        map[string]cacheEntry
	bytes    int
}

func newTTLCache(ttl time.Duration) *ttlCache {
	return &ttlCache{ttl: ttl, maxBytes: cacheMaxBytes, m: map[string]cacheEntry{}}
}

func (c *ttlCache) get(key string) ([]byte, bool) {
	if c.ttl <= 0 {
		return nil, false
	}
	c.mu.RLock()
	e, ok := c.m[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.body, true
}

func (c *ttlCache) set(key string, body []byte) {
	if c.ttl <= 0 || len(body) > c.maxBytes {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.remove(key)
	if c.bytes+len(body) > c.maxBytes {
		c.evict(c.maxBytes - len(body))
	}
	c.m[key] = cacheEntry{body: body, expires: time.Now().Add(c.ttl)}
	c.bytes += len(body)
}

// evict shrinks the cache to at most target bytes. Callers hold mu.
func (c *ttlCache) evict(target int) {
	now := time.Now()
	for k, e := range c.m {
		if now.After(e.expires) {
			c.remove(k)
		}
	}
	for c.bytes > target {
		var oldest string
		var at time.Time
		for k, e := range c.m {
			if oldest == "" || e.expires.Before(at) {
				oldest, at = k, e.expires
			}
		}
		c.remove(oldest)
	}
}

func (c *ttlCache) remove(key string) {
	if e, ok := c.m[key]; ok {
		c.bytes -= len(e.body)
		delete(c.m, key)
	}
}
