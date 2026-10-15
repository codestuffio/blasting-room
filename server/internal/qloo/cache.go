package qloo

import (
	"sync"
	"time"
)

type cacheEntry struct {
	body    []byte
	expires time.Time
}

// ttlCache is a process-local response cache keyed by request URL.
type ttlCache struct {
	ttl time.Duration
	mu  sync.RWMutex
	m   map[string]cacheEntry
}

func newTTLCache(ttl time.Duration) *ttlCache {
	return &ttlCache{ttl: ttl, m: map[string]cacheEntry{}}
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
	if c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	c.m[key] = cacheEntry{body: body, expires: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}
