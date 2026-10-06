package search

import (
	"sync"
	"testing"

	"github.com/crenspire/xpm/internal/pm"
)

// TestCacheRaceCondition tests that cache operations are thread-safe.
func TestCacheRaceCondition(t *testing.T) {
	// Create a new cache for testing
	cache := NewCache(5*60*1000000000, 100) // 5 minutes TTL, max 100 entries

	// Test concurrent get and set operations
	const numGoroutines = 100
	const numOperations = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2) // get and set operations

	// Concurrent sets
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				pkg := pm.ID("npm")
				result := &Result{
					Manager: pm.Npm,
					Name:    "test-package",
					Info:    "Test package",
				}
				cache.Set("test-package", pkg, result)
			}
		}(i)
	}

	// Concurrent gets
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				pkg := pm.ID("npm")
				_ = cache.Get("test-package", pkg)
			}
		}(i)
	}

	wg.Wait()

	// Verify cache still works after concurrent access
	result := cache.Get("test-package", pm.Npm)
	if result == nil {
		t.Error("Cache should contain test-package after concurrent operations")
	}
}

// TestGetCachedSetCachedRace tests the global cache functions for race conditions.
func TestGetCachedSetCachedRace(t *testing.T) {
	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Concurrent getCached calls
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			_ = getCached("test-pkg", pm.Npm)
		}(i)
	}

	// Concurrent setCached calls
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			result := &Result{
				Manager: pm.Npm,
				Name:    "test-pkg",
				Info:    "Test",
			}
			setCached("test-pkg", pm.Npm, result)
		}(i)
	}

	wg.Wait()

	// Verify no panic occurred and cache is accessible
	result := getCached("test-pkg", pm.Npm)
	if result == nil {
		t.Error("getCached should return result after concurrent operations")
	}
}
