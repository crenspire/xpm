package search

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crenspire/xpm/internal/pm"
)

func countingLookup(id pm.ID, res *Result, err error) (lookup, *int32) {
	var n int32
	return lookup{id: id, fn: func(string) (*Result, error) {
		atomic.AddInt32(&n, 1)
		return res, err
	}}, &n
}

func TestCachedLookupServesRepeatsFromDisk(t *testing.T) {
	dir := t.TempDir()
	l, calls := countingLookup(pm.Npm, &Result{Manager: pm.Npm, Name: "axios", Extra: map[string]string{"version": "1.2.3"}}, nil)

	first, err := cachedLookup(dir, l, "axios")
	if err != nil {
		t.Fatal(err)
	}
	second, err := cachedLookup(dir, l, "axios")
	if err != nil {
		t.Fatal(err)
	}
	if *calls != 1 {
		t.Fatalf("registry called %d times, want 1", *calls)
	}
	if second == nil || second.Extra["version"] != first.Extra["version"] {
		t.Fatalf("cached result %+v != original %+v", second, first)
	}
}

func TestCachedLookupCachesNotFound(t *testing.T) {
	dir := t.TempDir()
	l, calls := countingLookup(pm.Pip, nil, nil)
	for i := 0; i < 2; i++ {
		if r, err := cachedLookup(dir, l, "nope"); r != nil || err != nil {
			t.Fatalf("got (%v, %v), want (nil, nil)", r, err)
		}
	}
	if *calls != 1 {
		t.Fatalf("registry called %d times, want 1", *calls)
	}
}

func TestCachedLookupNeverCachesErrors(t *testing.T) {
	dir := t.TempDir()
	l, calls := countingLookup(pm.Cargo, nil, errors.New("timeout"))
	cachedLookup(dir, l, "serde")
	cachedLookup(dir, l, "serde")
	if *calls != 2 {
		t.Fatalf("registry called %d times, want 2 (errors must not be cached)", *calls)
	}
}

func TestCachedLookupRefetchesExpiredEntries(t *testing.T) {
	dir := t.TempDir()
	l, calls := countingLookup(pm.Npm, &Result{Manager: pm.Npm, Name: "x"}, nil)
	path := cachePath(dir, pm.Npm, "x")
	stale, _ := json.Marshal(cacheEntry{Found: true, Result: &Result{Name: "old"}, At: time.Now().Add(-2 * positiveTTL)})
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, stale, 0o644)

	r, _ := cachedLookup(dir, l, "x")
	if *calls != 1 || r == nil || r.Name != "x" {
		t.Fatalf("expired entry was served (calls=%d result=%+v)", *calls, r)
	}
}

func TestCacheDisabledByEnv(t *testing.T) {
	t.Setenv("XPM_NO_CACHE", "1")
	if dir := defaultLookupCacheDir(); dir != "" {
		t.Fatalf("XPM_NO_CACHE=1 must disable the cache, got dir %q", dir)
	}
}
