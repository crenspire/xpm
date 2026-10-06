package search

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crenspire/xpm/internal/pm"
)

func TestSearchEverywhereReportListsUnavailable(t *testing.T) {
	withDeadline(t, 150*time.Millisecond)
	withLookups(t, []lookup{
		fakeLookup(pm.Npm, 0, nil),
		fakeLookup(pm.Pip, 0, errors.New("pypi registry returned status 503")),
		fakeLookup(pm.Maven, 5*time.Second, nil),
	})
	rep, err := SearchEverywhereReport("x", Options{})
	if err != nil {
		t.Fatalf("partial failure must not be an error, got %v", err)
	}
	if len(rep.Results) != 1 || rep.Results[0].Manager != pm.Npm {
		t.Fatalf("Results = %+v, want only npm", rep.Results)
	}
	ids := rep.UnavailableIDs()
	if len(ids) != 2 || ids[0] != pm.Pip || ids[1] != pm.Maven {
		t.Fatalf("Unavailable = %v, want [pip maven] in table order", ids)
	}
	if rep.Unavailable[0].TimedOut() {
		t.Error("pip answered with an error; it must not be reported as timed out")
	}
	if !rep.Unavailable[1].TimedOut() {
		t.Error("maven missed the deadline; it must be reported as timed out")
	}
}

func TestSearchEverywhereReportAllFailedStillReports(t *testing.T) {
	withLookups(t, []lookup{
		fakeLookup(pm.Npm, 0, errors.New("offline")),
		fakeLookup(pm.Pip, 0, errors.New("offline")),
	})
	rep, err := SearchEverywhereReport("x", Options{})
	if !errors.Is(err, ErrAllRegistriesFailed) {
		t.Fatalf("err = %v, want ErrAllRegistriesFailed", err)
	}
	if len(rep.Unavailable) != 2 {
		t.Fatalf("Unavailable = %+v, want both registries", rep.Unavailable)
	}
}

// withMultiLookups swaps the multi-result search table and disables the cache.
func withMultiLookups(t *testing.T, ms []multiLookup) {
	t.Helper()
	orig, origDir := multiLookups, lookupCacheDir
	multiLookups, lookupCacheDir = ms, ""
	t.Cleanup(func() { multiLookups, lookupCacheDir = orig, origDir })
}

func fakeMulti(id pm.ID, delay time.Duration, names ...string) multiLookup {
	return multiLookup{id: id, fn: func(ctx context.Context, q string) ([]Result, error) {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		var out []Result
		for _, n := range names {
			out = append(out, Result{Manager: id, Name: n})
		}
		return out, nil
	}}
}

func TestSearchReportUsesTheLookupDeadline(t *testing.T) {
	withDeadline(t, 150*time.Millisecond)
	withMultiLookups(t, []multiLookup{
		fakeMulti(pm.Npm, 0, "axios", "axios-retry"),
		fakeMulti(pm.Maven, 10*time.Second, "never"),
	})
	start := time.Now()
	rep, err := SearchReport("axios", Options{})
	if d := time.Since(start); d > 400*time.Millisecond {
		t.Fatalf("took %v; the TUI search must not wait 10 s for a stalled registry", d)
	}
	if err != nil || len(rep.Results) != 2 {
		t.Fatalf("got (%+v, %v)", rep, err)
	}
	if ids := rep.UnavailableIDs(); len(ids) != 1 || ids[0] != pm.Maven {
		t.Fatalf("Unavailable = %v, want [maven]", ids)
	}
}

func TestSearchReportAllFailedIsAnError(t *testing.T) {
	withDeadline(t, 50*time.Millisecond)
	withMultiLookups(t, []multiLookup{fakeMulti(pm.Npm, time.Second)})
	if _, err := SearchEverywhereParallel("x", Options{}); !errors.Is(err, ErrAllRegistriesFailed) {
		t.Fatalf("err = %v, want ErrAllRegistriesFailed", err)
	}
}

func TestCachedSearchServesRepeatsFromDisk(t *testing.T) {
	var calls int32
	m := multiLookup{id: pm.Npm, fn: func(context.Context, string) ([]Result, error) {
		atomic.AddInt32(&calls, 1)
		return []Result{{Manager: pm.Npm, Name: "axios"}}, nil
	}}
	dir := t.TempDir()
	for i := 0; i < 2; i++ {
		got, err := cachedSearch(context.Background(), dir, m, "axios")
		if err != nil || len(got) != 1 || got[0].Name != "axios" {
			t.Fatalf("got (%+v, %v)", got, err)
		}
	}
	if calls != 1 {
		t.Fatalf("registry searched %d times, want 1", calls)
	}
}
