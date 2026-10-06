package search

import (
	"sync"
	"time"

	"github.com/crenspire/xpm/internal/pm"
)

// CacheEntry represents a cached search result with expiration.
type CacheEntry struct {
	Result    *Result
	Timestamp time.Time
	TTL       time.Duration
}

// IsExpired checks if the cache entry has expired.
func (e *CacheEntry) IsExpired() bool {
	return time.Since(e.Timestamp) > e.TTL
}

// Cache provides thread-safe caching for search results.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
	ttl     time.Duration
	maxSize int
}

// CacheConfig holds cache configuration.
type CacheConfig struct {
	// TTL is the time-to-live for cache entries.
	TTL time.Duration
	// MaxSize is the maximum number of entries to cache.
	MaxSize int
	// Enabled controls whether caching is active.
	Enabled bool
}

// DefaultCacheConfig returns the default cache configuration.
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		TTL:     5 * time.Minute,
		MaxSize: 100,
		Enabled: true,
	}
}

// globalCache is the default cache instance.
// Access to globalCache itself is protected by cacheMu to prevent races
// when SetCacheConfig reassigns it.
var (
	globalCache *Cache
	cacheConfig = DefaultCacheConfig()
	cacheMu     sync.RWMutex // Protects globalCache variable reassignment
)

func init() {
	globalCache = NewCache(cacheConfig.TTL, cacheConfig.MaxSize)
}

// NewCache creates a new cache with the specified TTL and max size.
func NewCache(ttl time.Duration, maxSize int) *Cache {
	return &Cache{
		entries: make(map[string]*CacheEntry),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// cacheKey generates a cache key for a package and manager.
func cacheKey(pkg string, manager pm.ID) string {
	return string(manager) + ":" + pkg
}

// Get retrieves a result from the cache.
// Returns nil if not found or expired.
func (c *Cache) Get(pkg string, manager pm.ID) *Result {
	if !cacheConfig.Enabled {
		return nil
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	key := cacheKey(pkg, manager)
	entry, ok := c.entries[key]
	if !ok {
		return nil
	}

	if entry.IsExpired() {
		return nil
	}

	return entry.Result
}

// Set stores a result in the cache.
func (c *Cache) Set(pkg string, manager pm.ID, result *Result) {
	if !cacheConfig.Enabled || result == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict old entries if at max size
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	key := cacheKey(pkg, manager)
	c.entries[key] = &CacheEntry{
		Result:    result,
		Timestamp: time.Now(),
		TTL:       c.ttl,
	}
}

// evictOldest removes the oldest entry from the cache.
// Must be called with the lock held.
func (c *Cache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	first := true
	for key, entry := range c.entries {
		if first || entry.Timestamp.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.Timestamp
			first = false
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

// Clear removes all entries from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*CacheEntry)
}

// Cleanup removes expired entries from the cache.
func (c *Cache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.entries {
		if entry.IsExpired() {
			delete(c.entries, key)
		}
	}
}

// Size returns the current number of entries in the cache.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// SetCacheConfig sets the global cache configuration.
// Thread-safe: protects reassignment of globalCache.
func SetCacheConfig(cfg CacheConfig) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	cacheConfig = cfg
	if cfg.Enabled {
		globalCache = NewCache(cfg.TTL, cfg.MaxSize)
	} else {
		globalCache = nil
	}
}

// GetCacheConfig returns the current cache configuration.
func GetCacheConfig() CacheConfig {
	return cacheConfig
}

// ClearCache clears the global cache.
// Thread-safe: protects access to globalCache variable.
func ClearCache() {
	cacheMu.RLock()
	cache := globalCache
	cacheMu.RUnlock()

	if cache != nil {
		cache.Clear()
	}
}

// CacheStats returns statistics about the cache.
type CacheStats struct {
	Size      int
	MaxSize   int
	TTL       time.Duration
	Enabled   bool
	HitCount  int64
	MissCount int64
}

// stats tracks cache hits and misses.
var stats struct {
	mu        sync.Mutex
	hitCount  int64
	missCount int64
}

// recordHit records a cache hit.
func recordHit() {
	stats.mu.Lock()
	stats.hitCount++
	stats.mu.Unlock()
}

// recordMiss records a cache miss.
func recordMiss() {
	stats.mu.Lock()
	stats.missCount++
	stats.mu.Unlock()
}

// GetCacheStats returns current cache statistics.
// Thread-safe: protects access to globalCache variable.
func GetCacheStats() CacheStats {
	cacheMu.RLock()
	cache := globalCache
	cacheMu.RUnlock()

	stats.mu.Lock()
	defer stats.mu.Unlock()

	size := 0
	if cache != nil {
		size = cache.Size()
	}

	return CacheStats{
		Size:      size,
		MaxSize:   cacheConfig.MaxSize,
		TTL:       cacheConfig.TTL,
		Enabled:   cacheConfig.Enabled,
		HitCount:  stats.hitCount,
		MissCount: stats.missCount,
	}
}

// getCached attempts to get a cached result, recording stats.
// Thread-safe: protects access to globalCache variable.
func getCached(pkg string, manager pm.ID) *Result {
	cacheMu.RLock()
	cache := globalCache
	cacheMu.RUnlock()

	if cache == nil {
		return nil
	}

	result := cache.Get(pkg, manager)
	if result != nil {
		recordHit()
	} else {
		recordMiss()
	}
	return result
}

// setCached stores a result in the global cache.
// Thread-safe: protects access to globalCache variable.
func setCached(pkg string, manager pm.ID, result *Result) {
	cacheMu.RLock()
	cache := globalCache
	cacheMu.RUnlock()

	if cache == nil {
		return
	}

	cache.Set(pkg, manager, result)
}
