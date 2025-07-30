package cache

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestMemoryCache(t *testing.T) {
	ctx := context.Background()
	
	t.Run("Set and Get", func(t *testing.T) {
		cache := NewMemoryCache(nil)
		
		key := "test-key"
		value := []byte("test-value")
		
		err := cache.Set(ctx, key, value, time.Hour)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		
		retrieved, err := cache.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		
		if string(retrieved) != string(value) {
			t.Errorf("Get() = %s, expected %s", retrieved, value)
		}
	})
	
	t.Run("Get non-existent key", func(t *testing.T) {
		cache := NewMemoryCache(nil)
		
		_, err := cache.Get(ctx, "non-existent")
		if err == nil {
			t.Errorf("Get() expected error for non-existent key")
		}
	})
	
	t.Run("TTL expiration", func(t *testing.T) {
		cache := NewMemoryCache(nil)
		
		key := "expiring-key"
		value := []byte("expiring-value")
		
		err := cache.Set(ctx, key, value, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		
		// Should be available immediately
		_, err = cache.Get(ctx, key)
		if err != nil {
			t.Errorf("Get() error = %v, expected key to be available", err)
		}
		
		// Wait for expiration
		time.Sleep(200 * time.Millisecond)
		
		_, err = cache.Get(ctx, key)
		if err == nil {
			t.Errorf("Get() expected error for expired key")
		}
	})
	
	t.Run("Delete", func(t *testing.T) {
		cache := NewMemoryCache(nil)
		
		key := "delete-key"
		value := []byte("delete-value")
		
		cache.Set(ctx, key, value, time.Hour)
		
		// Verify it exists
		_, err := cache.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		
		// Delete it
		err = cache.Delete(ctx, key)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		
		// Verify it's gone
		_, err = cache.Get(ctx, key)
		if err == nil {
			t.Errorf("Get() expected error for deleted key")
		}
	})
	
	t.Run("Clear", func(t *testing.T) {
		cache := NewMemoryCache(nil)
		
		// Add multiple items
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("key-%d", i)
			value := []byte(fmt.Sprintf("value-%d", i))
			cache.Set(ctx, key, value, time.Hour)
		}
		
		// Clear all
		err := cache.Clear(ctx)
		if err != nil {
			t.Fatalf("Clear() error = %v", err)
		}
		
		// Verify all are gone
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("key-%d", i)
			_, err := cache.Get(ctx, key)
			if err == nil {
				t.Errorf("Get() expected error for cleared key %s", key)
			}
		}
	})
	
	t.Run("Size limits", func(t *testing.T) {
		config := &MemoryCacheConfig{
			MaxSize:  100, // Very small
			MaxItems: 2,   // Very few items
		}
		cache := NewMemoryCache(config)
		
		// Add items that should trigger eviction
		cache.Set(ctx, "key1", []byte("value1"), time.Hour)
		cache.Set(ctx, "key2", []byte("value2"), time.Hour)
		cache.Set(ctx, "key3", []byte("value3"), time.Hour) // Should evict key1
		
		// key1 should be evicted
		_, err := cache.Get(ctx, "key1")
		if err == nil {
			t.Errorf("Get() expected error for evicted key1")
		}
		
		// key2 and key3 should still exist
		_, err = cache.Get(ctx, "key2")
		if err != nil {
			t.Errorf("Get() error = %v, expected key2 to exist", err)
		}
		
		_, err = cache.Get(ctx, "key3")
		if err != nil {
			t.Errorf("Get() error = %v, expected key3 to exist", err)
		}
	})
	
	t.Run("Stats", func(t *testing.T) {
		cache := NewMemoryCache(nil)
		
		// Test hits and misses
		cache.Set(ctx, "key1", []byte("value1"), time.Hour)
		
		// Hit
		cache.Get(ctx, "key1")
		
		// Miss
		cache.Get(ctx, "non-existent")
		
		stats := cache.Stats()
		if stats.Hits != 1 {
			t.Errorf("Stats.Hits = %d, expected 1", stats.Hits)
		}
		if stats.Misses != 1 {
			t.Errorf("Stats.Misses = %d, expected 1", stats.Misses)
		}
		if stats.ItemCount != 1 {
			t.Errorf("Stats.ItemCount = %d, expected 1", stats.ItemCount)
		}
	})
}

func TestCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		key1     *CacheKey
		key2     *CacheKey
		expected bool // true if keys should be equal
	}{
		{
			name: "identical keys",
			key1: NewCacheKey("GET", "/api/test", map[string]string{"Host": "example.com"}),
			key2: NewCacheKey("GET", "/api/test", map[string]string{"Host": "example.com"}),
			expected: true,
		},
		{
			name: "different methods",
			key1: NewCacheKey("GET", "/api/test", nil),
			key2: NewCacheKey("POST", "/api/test", nil),
			expected: false,
		},
		{
			name: "different URIs",
			key1: NewCacheKey("GET", "/api/test1", nil),
			key2: NewCacheKey("GET", "/api/test2", nil),
			expected: false,
		},
		{
			name: "different headers",
			key1: NewCacheKey("GET", "/api/test", map[string]string{"Host": "example.com"}),
			key2: NewCacheKey("GET", "/api/test", map[string]string{"Host": "test.com"}),
			expected: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key1Str := tt.key1.String()
			key2Str := tt.key2.String()
			
			equal := key1Str == key2Str
			if equal != tt.expected {
				t.Errorf("CacheKey equality = %v, expected %v", equal, tt.expected)
			}
		})
	}
}

func TestTieredCache(t *testing.T) {
	ctx := context.Background()
	
	l1 := NewMemoryCache(&MemoryCacheConfig{MaxSize: 100, MaxItems: 2})
	l2 := NewMemoryCache(&MemoryCacheConfig{MaxSize: 1000, MaxItems: 10})
	
	tiered := NewTieredCache(l1, l2)
	
	t.Run("Set stores in both levels", func(t *testing.T) {
		err := tiered.Set(ctx, "key1", []byte("value1"), time.Hour)
		if err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		
		// Should be in L1
		_, err = l1.Get(ctx, "key1")
		if err != nil {
			t.Errorf("L1.Get() error = %v", err)
		}
		
		// Should be in L2
		_, err = l2.Get(ctx, "key1")
		if err != nil {
			t.Errorf("L2.Get() error = %v", err)
		}
	})
	
	t.Run("Get from L2 when not in L1", func(t *testing.T) {
		// Put directly in L2
		l2.Set(ctx, "key2", []byte("value2"), time.Hour)
		
		// Get from tiered cache should find it in L2 and promote to L1
		value, err := tiered.Get(ctx, "key2")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		
		if string(value) != "value2" {
			t.Errorf("Get() = %s, expected value2", value)
		}
		
		// Should now be in L1 too
		_, err = l1.Get(ctx, "key2")
		if err != nil {
			t.Errorf("L1.Get() error = %v after promotion", err)
		}
	})
	
	t.Run("Stats combine both levels", func(t *testing.T) {
		stats := tiered.Stats()
		
		// Should have combined stats from both levels
		if stats.ItemCount < 1 {
			t.Errorf("Stats.ItemCount = %d, expected > 1", stats.ItemCount)
		}
	})
}

func TestNoOpCache(t *testing.T) {
	ctx := context.Background()
	cache := NewNoOpCache()
	
	t.Run("Get always returns error", func(t *testing.T) {
		_, err := cache.Get(ctx, "any-key")
		if err == nil {
			t.Errorf("Get() expected error from NoOpCache")
		}
	})
	
	t.Run("Set always succeeds", func(t *testing.T) {
		err := cache.Set(ctx, "any-key", []byte("any-value"), time.Hour)
		if err != nil {
			t.Errorf("Set() error = %v", err)
		}
	})
	
	t.Run("Delete always succeeds", func(t *testing.T) {
		err := cache.Delete(ctx, "any-key")
		if err != nil {
			t.Errorf("Delete() error = %v", err)
		}
	})
	
	t.Run("Clear always succeeds", func(t *testing.T) {
		err := cache.Clear(ctx)
		if err != nil {
			t.Errorf("Clear() error = %v", err)
		}
	})
	
	t.Run("Stats returns empty", func(t *testing.T) {
		stats := cache.Stats()
		if stats.Hits != 0 || stats.Misses != 0 || stats.ItemCount != 0 {
			t.Errorf("Stats() = %+v, expected all zeros", stats)
		}
	})
}

func BenchmarkMemoryCache_Set(b *testing.B) {
	cache := NewMemoryCache(nil)
	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i)
		value := []byte(fmt.Sprintf("value-%d", i))
		cache.Set(ctx, key, value, time.Hour)
	}
}

func BenchmarkMemoryCache_Get(b *testing.B) {
	cache := NewMemoryCache(nil)
	ctx := context.Background()
	
	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		key := fmt.Sprintf("key-%d", i)
		value := []byte(fmt.Sprintf("value-%d", i))
		cache.Set(ctx, key, value, time.Hour)
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%1000)
		cache.Get(ctx, key)
	}
}

func BenchmarkCacheKey_String(b *testing.B) {
	key := NewCacheKey("GET", "/api/v1/users/123", map[string]string{
		"Host":         "api.example.com",
		"User-Agent":   "Mozilla/5.0",
		"Accept":       "application/json",
		"Content-Type": "application/json",
	})
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = key.String()
	}
}