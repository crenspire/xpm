package search

import (
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
// Set XPM_NO_CACHE=1 to bypass.
var (
	lookupCacheDir = defaultLookupCacheDir()
	positiveTTL    = time.Hour        // package found
	negativeTTL    = 15 * time.Minute // package not found (may be published soon)
)

func defaultLookupCacheDir() string {
	if os.Getenv("XPM_NO_CACHE") != "" {
		return ""
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
func cachedLookup(dir string, l lookup, pkg string) (*Result, error) {
	if dir == "" {
		return l.fn(pkg)
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

	res, err := l.fn(pkg)
	if err != nil {
		return nil, err
	}
	writeCacheEntry(path, cacheEntry{Found: res != nil, Result: res, At: time.Now()})
	return res, nil
}

// writeCacheEntry writes atomically (temp file + rename) so concurrent xpm
// processes never read a half-written entry. Failures are ignored: the cache
// is an optimisation only.
func writeCacheEntry(path string, e cacheEntry) {
	data, err := json.Marshal(e)
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
