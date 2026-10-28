package qloo

import (
	"fmt"
	"testing"
	"time"
)

func TestCacheStaysWithinItsByteBudget(t *testing.T) {
	c := newTTLCache(time.Hour)
	c.maxBytes = 100
	for i := range 5 {
		c.set(fmt.Sprintf("k%d", i), make([]byte, 30))
		time.Sleep(time.Millisecond) // distinct expiries, so "oldest" is well defined
	}
	if c.bytes > 100 || len(c.m) != 3 {
		t.Fatalf("bytes = %d, entries = %d, want <= 100 bytes in 3 entries", c.bytes, len(c.m))
	}
	for _, k := range []string{"k0", "k1"} {
		if _, ok := c.get(k); ok {
			t.Fatalf("%s should have been evicted first", k)
		}
	}
	if _, ok := c.get("k4"); !ok {
		t.Fatal("newest entry missing")
	}

	// Replacing a key doesn't count its old body twice.
	c.set("k4", make([]byte, 10))
	if c.bytes != 70 {
		t.Fatalf("bytes after replace = %d, want 70", c.bytes)
	}
	// A body bigger than the whole budget is not cached.
	c.set("huge", make([]byte, 101))
	if _, ok := c.get("huge"); ok {
		t.Fatal("oversized body was cached")
	}
}

func TestCacheDropsExpiredEntriesBeforeLiveOnes(t *testing.T) {
	c := newTTLCache(time.Hour)
	c.maxBytes = 100
	c.set("live", make([]byte, 40))
	c.m["stale"] = cacheEntry{body: make([]byte, 40), expires: time.Now().Add(-time.Minute)}
	c.bytes += 40
	c.set("new", make([]byte, 40))
	if _, ok := c.m["stale"]; ok {
		t.Fatal("expired entry kept")
	}
	if _, ok := c.get("live"); !ok {
		t.Fatal("live entry evicted while an expired one was there")
	}
}
