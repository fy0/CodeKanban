package cache

import (
	"sync"
	"time"
)

// CacheItem stores value and expiration metadata.
type CacheItem struct {
	Value      any
	Expiration time.Time
}

// Cache is a simple TTL cache safe for concurrent use.
type Cache struct {
	items           sync.Map
	ttl             time.Duration
	cleanupInterval time.Duration
	stopCh          chan struct{}
}

// NewCache builds a Cache with the provided ttl.
func NewCache(ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	c := &Cache{
		ttl:             ttl,
		cleanupInterval: minDuration(ttl, 5*time.Minute),
		stopCh:          make(chan struct{}),
	}
	go c.startCleanup()
	return c
}

// Set stores key/value until the ttl expires.
func (c *Cache) Set(key string, value any) {
	if c == nil {
		return
	}
	c.items.Store(key, &CacheItem{
		Value:      value,
		Expiration: time.Now().Add(c.ttl),
	})
}

// Get returns the cached value if not expired.
func (c *Cache) Get(key string) (any, bool) {
	if c == nil {
		return nil, false
	}
	raw, ok := c.items.Load(key)
	if !ok {
		return nil, false
	}
	item := raw.(*CacheItem)
	if time.Now().After(item.Expiration) {
		c.items.Delete(key)
		return nil, false
	}
	return item.Value, true
}

// Delete removes a key from cache.
func (c *Cache) Delete(key string) {
	if c == nil {
		return
	}
	c.items.Delete(key)
}

// Close stops the cleanup goroutine.
func (c *Cache) Close() {
	if c == nil {
		return
	}
	select {
	case <-c.stopCh:
		// already closed
	default:
		close(c.stopCh)
	}
}

func (c *Cache) startCleanup() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanupExpired()
		case <-c.stopCh:
			return
		}
	}
}

func (c *Cache) cleanupExpired() {
	now := time.Now()
	c.items.Range(func(key, value any) bool {
		item := value.(*CacheItem)
		if now.After(item.Expiration) {
			c.items.Delete(key)
		}
		return true
	})
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
