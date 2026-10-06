package search

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/crenspire/xpm/internal/pm"
)

// Exact-lookup results are memoised on disk so repeat commands
// (`xpm which x` then `xpm install x`) skip the network entirely.
// Set XPM_NO_CACHE (any non-empty value, e.g. 1) to bypass, or XPM_CACHE_DIR=<dir> to move the cache
// (entries then live in <dir>/lookups).
var (
	lookupCacheDir = defaultLookupCacheDir()
	positiveTTL    = time.Hour        // package found
	negativeTTL    = 15 * time.Minute // package not found (may be published soon)
)

func defaultLookupCacheDir() string {
	if os.Getenv("XPM_NO_CACHE") != "" {
		return ""
	}
	if dir := os.Getenv("XPM_CACHE_DIR"); dir != "" {
		return filepath.Join(dir, "lookups")
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "xpm", "lookups")
}

type cacheEntry struct {
	Found  bool      `json:"found"`
	Result *Result   `json:"result,omitempty"`
	At     time.Time `json:"at"`
}

func cachePath(dir string, id pm.ID, pkg string) string {
	sum := sha256.Sum256([]byte(pkg))
	return filepath.Join(dir, string(id), hex.EncodeToString(sum[:16])+".json")
}

// cachedLookup answers from the cache in dir when a fresh entry exists,
// otherwise calls l.fn and stores the outcome. Errors are never cached.
// dir is passed in (not read from lookupCacheDir) because lookups that miss
// the deadline keep running in the background after SearchEverywhere returns.
// An empty dir disables caching.
func cachedLookup(ctx context.Context, dir string, l lookup, pkg string) (*Result, error) {
	if dir == "" {
		return l.fn(ctx, pkg)
	}
	path := cachePath(dir, l.id, pkg)
	if data, err := os.ReadFile(path); err == nil {
		var e cacheEntry
		if json.Unmarshal(data, &e) == nil {
			ttl := negativeTTL
			if e.Found {
				ttl = positiveTTL
			}
			if !e.At.After(time.Now()) && time.Since(e.At) < ttl {
				return e.Result, nil
			}
		}
	}

	res, err := l.fn(ctx, pkg)
	if err != nil {
		return nil, err
	}
	writeJSONAtomic(path, cacheEntry{Found: res != nil, Result: res, At: time.Now()})
	return res, nil
}

// searchTTL is how long multi-result searches (TUI, `xpm search`) are cached.
var searchTTL = 15 * time.Minute

type searchCacheEntry struct {
	Results []Result  `json:"results"`
	At      time.Time `json:"at"`
}

// cachedSearch is cachedLookup for multi-result searches. Entries live under
// <dir>/search/<id>/. Errors are never cached; an empty dir disables caching.
func cachedSearch(ctx context.Context, dir string, m multiLookup, query string) ([]Result, error) {
	if dir == "" {
		return m.fn(ctx, query)
	}
	path := cachePath(filepath.Join(dir, "search"), m.id, query)
	if data, err := os.ReadFile(path); err == nil {
		var e searchCacheEntry
		if json.Unmarshal(data, &e) == nil && !e.At.After(time.Now()) && time.Since(e.At) < searchTTL {
			return e.Results, nil
		}
	}
	res, err := m.fn(ctx, query)
	if err != nil {
		return nil, err
	}
	writeJSONAtomic(path, searchCacheEntry{Results: res, At: time.Now()})
	return res, nil
}

// writeJSONAtomic writes v as JSON atomically (temp file + rename) so
// concurrent xpm processes never read a half-written entry. Failures are
// ignored: the cache is an optimisation only.
func writeJSONAtomic(path string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.CreateTemp(filepath.Dir(path), "*.tmp")
	if err != nil {
		return
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(f.Name())
		return
	}
	if err := os.Rename(f.Name(), path); err != nil {
		_ = os.Remove(f.Name())
	}
}
