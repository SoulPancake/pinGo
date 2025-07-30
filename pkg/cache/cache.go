package cache

import (
	"context"
	"crypto/md5"
	"fmt"
	"sync"
	"time"

	"github.com/SoulPancake/pinGo/pkg/errors"
)

type Cache interface {
	Get(ctx context.Context, key string) ([]byte, *errors.Error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) *errors.Error
	Delete(ctx context.Context, key string) *errors.Error
	Clear(ctx context.Context) *errors.Error
	Stats() CacheStats
}

type CacheStats struct {
	Hits        int64
	Misses      int64
	Evictions   int64
	Size        int64
	MaxSize     int64
	ItemCount   int64
	MaxItems    int64
}

type CacheEntry struct {
	Value     []byte
	ExpiresAt time.Time
	AccessedAt time.Time
	CreatedAt time.Time
	Size      int64
}

func (e *CacheEntry) IsExpired() bool {
	if e.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(e.ExpiresAt)
}

type MemoryCache struct {
	entries   map[string]*CacheEntry
	maxSize   int64
	maxItems  int64
	currentSize int64
	stats     CacheStats
	mu        sync.RWMutex
	locks     map[string]*sync.Mutex
	locksMu   sync.Mutex
}

type MemoryCacheConfig struct {
	MaxSize  int64
	MaxItems int64
}

func NewMemoryCache(config *MemoryCacheConfig) *MemoryCache {
	if config == nil {
		config = &MemoryCacheConfig{
			MaxSize:  100 * 1024 * 1024, // 100MB
			MaxItems: 10000,
		}
	}

	return &MemoryCache{
		entries:  make(map[string]*CacheEntry),
		maxSize:  config.MaxSize,
		maxItems: config.MaxItems,
		locks:    make(map[string]*sync.Mutex),
		stats: CacheStats{
			MaxSize:  config.MaxSize,
			MaxItems: config.MaxItems,
		},
	}
}

func (c *MemoryCache) Get(ctx context.Context, key string) ([]byte, *errors.Error) {
	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		c.mu.Lock()
		c.stats.Misses++
		c.mu.Unlock()
		return nil, errors.New(errors.CacheError, "key not found")
	}

	if entry.IsExpired() {
		c.Delete(ctx, key)
		c.mu.Lock()
		c.stats.Misses++
		c.mu.Unlock()
		return nil, errors.New(errors.CacheError, "key expired")
	}

	c.mu.Lock()
	entry.AccessedAt = time.Now()
	c.stats.Hits++
	c.mu.Unlock()

	return entry.Value, nil
}

func (c *MemoryCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) *errors.Error {
	keyLock := c.getKeyLock(key)
	keyLock.Lock()
	defer keyLock.Unlock()

	size := int64(len(value))
	now := time.Now()

	entry := &CacheEntry{
		Value:      make([]byte, len(value)),
		CreatedAt:  now,
		AccessedAt: now,
		Size:       size,
	}

	copy(entry.Value, value)

	if ttl > 0 {
		entry.ExpiresAt = now.Add(ttl)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, exists := c.entries[key]; exists {
		c.currentSize -= existing.Size
	} else {
		c.stats.ItemCount++
	}

	c.currentSize += size
	c.entries[key] = entry

	if err := c.evictIfNeeded(); err != nil {
		return err
	}

	c.stats.Size = c.currentSize
	return nil
}

func (c *MemoryCache) Delete(ctx context.Context, key string) *errors.Error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.entries[key]; exists {
		c.currentSize -= entry.Size
		c.stats.ItemCount--
		delete(c.entries, key)
	}

	c.stats.Size = c.currentSize
	return nil
}

func (c *MemoryCache) Clear(ctx context.Context) *errors.Error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CacheEntry)
	c.currentSize = 0
	c.stats.ItemCount = 0
	c.stats.Size = 0

	return nil
}

func (c *MemoryCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

func (c *MemoryCache) getKeyLock(key string) *sync.Mutex {
	c.locksMu.Lock()
	defer c.locksMu.Unlock()

	if lock, exists := c.locks[key]; exists {
		return lock
	}

	lock := &sync.Mutex{}
	c.locks[key] = lock
	return lock
}

func (c *MemoryCache) evictIfNeeded() *errors.Error {
	for (c.maxSize > 0 && c.currentSize > c.maxSize) ||
		(c.maxItems > 0 && c.stats.ItemCount > c.maxItems) {

		if err := c.evictLRU(); err != nil {
			return err
		}
	}
	return nil
}

func (c *MemoryCache) evictLRU() *errors.Error {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.AccessedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.AccessedAt
		}
	}

	if oldestKey != "" {
		c.currentSize -= c.entries[oldestKey].Size
		c.stats.ItemCount--
		c.stats.Evictions++
		delete(c.entries, oldestKey)
	}

	return nil
}

type CacheKey struct {
	Method string
	URI    string
	Headers map[string]string
}

func (k *CacheKey) String() string {
	h := md5.New()
	h.Write([]byte(k.Method))
	h.Write([]byte(k.URI))
	
	for name, value := range k.Headers {
		h.Write([]byte(name))
		h.Write([]byte(value))
	}
	
	return fmt.Sprintf("%x", h.Sum(nil))
}

func NewCacheKey(method, uri string, headers map[string]string) *CacheKey {
	return &CacheKey{
		Method:  method,
		URI:     uri,
		Headers: headers,
	}
}

type TieredCache struct {
	l1 Cache
	l2 Cache
}

func NewTieredCache(l1, l2 Cache) *TieredCache {
	return &TieredCache{
		l1: l1,
		l2: l2,
	}
}

func (t *TieredCache) Get(ctx context.Context, key string) ([]byte, *errors.Error) {
	value, err := t.l1.Get(ctx, key)
	if err == nil {
		return value, nil
	}

	value, err = t.l2.Get(ctx, key)
	if err == nil {
		t.l1.Set(ctx, key, value, time.Hour)
		return value, nil
	}

	return nil, err
}

func (t *TieredCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) *errors.Error {
	if err := t.l1.Set(ctx, key, value, ttl); err != nil {
		return err
	}

	return t.l2.Set(ctx, key, value, ttl)
}

func (t *TieredCache) Delete(ctx context.Context, key string) *errors.Error {
	t.l1.Delete(ctx, key)
	return t.l2.Delete(ctx, key)
}

func (t *TieredCache) Clear(ctx context.Context) *errors.Error {
	t.l1.Clear(ctx)
	return t.l2.Clear(ctx)
}

func (t *TieredCache) Stats() CacheStats {
	l1Stats := t.l1.Stats()
	l2Stats := t.l2.Stats()
	
	return CacheStats{
		Hits:      l1Stats.Hits + l2Stats.Hits,
		Misses:    l1Stats.Misses + l2Stats.Misses,
		Evictions: l1Stats.Evictions + l2Stats.Evictions,
		Size:      l1Stats.Size + l2Stats.Size,
		MaxSize:   l1Stats.MaxSize + l2Stats.MaxSize,
		ItemCount: l1Stats.ItemCount + l2Stats.ItemCount,
		MaxItems:  l1Stats.MaxItems + l2Stats.MaxItems,
	}
}

type NoOpCache struct{}

func NewNoOpCache() *NoOpCache {
	return &NoOpCache{}
}

func (n *NoOpCache) Get(ctx context.Context, key string) ([]byte, *errors.Error) {
	return nil, errors.New(errors.CacheError, "cache disabled")
}

func (n *NoOpCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) *errors.Error {
	return nil
}

func (n *NoOpCache) Delete(ctx context.Context, key string) *errors.Error {
	return nil
}

func (n *NoOpCache) Clear(ctx context.Context) *errors.Error {
	return nil
}

func (n *NoOpCache) Stats() CacheStats {
	return CacheStats{}
}