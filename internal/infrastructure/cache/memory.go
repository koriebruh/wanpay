package cache

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const defaultMaxItems = 100_000

type memItem struct {
	value     []byte
	expiresAt time.Time // zero = no expiry
}

func (m memItem) expired() bool {
	return !m.expiresAt.IsZero() && time.Now().After(m.expiresAt)
}

// memoryCache is a thread-safe in-memory Cache with TTL support.
// Used as the fallback when Redis is disabled — idempotency and other
// cache-dependent features stay functional in single-instance deployments.
// NOTE: state is not shared across multiple instances; use Redis in production.
type memoryCache struct {
	mu       sync.RWMutex
	items    map[string]memItem
	maxItems int
	stopCh   chan struct{}
}

func newMemoryCache() *memoryCache {
	c := &memoryCache{
		items:    make(map[string]memItem),
		maxItems: defaultMaxItems,
		stopCh:   make(chan struct{}),
	}
	go c.cleanupLoop()
	return c
}

// Shutdown stops the background cleanup goroutine.
func (c *memoryCache) Shutdown() error {
	close(c.stopCh)
	return nil
}

// Ping always returns nil — in-memory cache is always reachable.
func (c *memoryCache) Ping(_ context.Context) error { return nil }

func (c *memoryCache) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.evictExpired()
		case <-c.stopCh:
			return
		}
	}
}

func (c *memoryCache) evictExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range c.items {
		if v.expired() {
			delete(c.items, k)
		}
	}
}

func (c *memoryCache) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()

	if !ok {
		return nil, ErrCacheMiss
	}
	if item.expired() {
		// Lazy delete — re-check under write lock to avoid racing with a concurrent Set.
		c.mu.Lock()
		if cur, still := c.items[key]; still && cur.expired() {
			delete(c.items, key)
		}
		c.mu.Unlock()
		return nil, ErrCacheMiss
	}
	return item.value, nil
}

func (c *memoryCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, exists := c.items[key]
	if !exists && len(c.items) >= c.maxItems {
		return fmt.Errorf("cache: capacity limit reached (%d items)", c.maxItems)
	}
	c.items[key] = memItem{value: value, expiresAt: expiry(ttl)}
	return nil
}

// SetNX stores value only if the key does not exist or has expired.
// The check-and-set is atomic under the write lock — no TOCTOU race.
func (c *memoryCache) SetNX(_ context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if item, ok := c.items[key]; ok && !item.expired() {
		return false, nil
	}
	if len(c.items) >= c.maxItems {
		return false, fmt.Errorf("cache: capacity limit reached (%d items)", c.maxItems)
	}
	c.items[key] = memItem{value: value, expiresAt: expiry(ttl)}
	return true, nil
}

func (c *memoryCache) Del(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.items, k)
	}
	return nil
}

func (c *memoryCache) Exists(_ context.Context, key string) (bool, error) {
	c.mu.RLock()
	item, ok := c.items[key]
	c.mu.RUnlock()
	return ok && !item.expired(), nil
}

func expiry(ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{} // zero = no expiry
	}
	return time.Now().Add(ttl)
}
