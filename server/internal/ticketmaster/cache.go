package ticketmaster

import (
	"sync"
	"time"
)

// cache holds response bodies in memory for a short while, so re-running a
// demo venue doesn't spend the daily quota. Nothing is written to disk.
type cache struct {
	ttl time.Duration
	max int
	mu  sync.Mutex
	m   map[string]cached
}

type cached struct {
	body    []byte
	expires time.Time
}

func newCache(ttl time.Duration, max int) *cache {
	return &cache{ttl: ttl, max: max, m: map[string]cached{}}
}

func (c *cache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[key]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.body, true
}

// set drops expired entries when full, then the soonest to expire.
func (c *cache) set(key string, body []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if _, ok := c.m[key]; !ok && len(c.m) >= c.max {
		for k, e := range c.m {
			if now.After(e.expires) {
				delete(c.m, k)
			}
		}
		for len(c.m) >= c.max {
			var oldest string
			for k, e := range c.m {
				if oldest == "" || e.expires.Before(c.m[oldest].expires) {
					oldest = k
				}
			}
			delete(c.m, oldest)
		}
	}
	c.m[key] = cached{body: body, expires: now.Add(c.ttl)}
}
