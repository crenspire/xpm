package search

import (
	"errors"
	"testing"
	"time"

	"github.com/crenspire/xpm/internal/pm"
)

// withLookups swaps in fake registries for one test.
func withLookups(t *testing.T, ls []lookup) {
	t.Helper()
	orig := exactLookups
	exactLookups = ls
	t.Cleanup(func() { exactLookups = orig })
}

func withDeadline(t *testing.T, d time.Duration) {
	t.Helper()
	orig := lookupDeadline
	lookupDeadline = d
	t.Cleanup(func() { lookupDeadline = orig })
}

func fakeLookup(id pm.ID, delay time.Duration, err error) lookup {
	return lookup{id: id, fn: func(pkg string) (*Result, error) {
		time.Sleep(delay)
		if err != nil {
			return nil, err
		}
		return &Result{Manager: id, Name: pkg}, nil
	}}
}

var allIDs = []pm.ID{pm.Npm, pm.Pip, pm.Composer, pm.Cargo, pm.Maven}

func TestSearchEverywhereIsParallelAndOrdered(t *testing.T) {
	var ls []lookup
	for _, id := range allIDs {
		ls = append(ls, fakeLookup(id, 200*time.Millisecond, nil))
	}
	withLookups(t, ls)

	start := time.Now()
	got, err := SearchEverywhere("x", Options{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed > 600*time.Millisecond {
		t.Fatalf("took %v for 5×200ms lookups; registries were queried sequentially", elapsed)
	}
	if len(got) != len(allIDs) {
		t.Fatalf("got %d results, want %d", len(got), len(allIDs))
	}
	for i, r := range got {
		if r.Manager != allIDs[i] {
			t.Errorf("result %d = %s, want %s (order must be deterministic)", i, r.Manager, allIDs[i])
		}
	}
}

func TestSearchEverywhereDeadlineDropsStragglers(t *testing.T) {
	withDeadline(t, 150*time.Millisecond)
	withLookups(t, []lookup{
		fakeLookup(pm.Npm, 0, nil),
		fakeLookup(pm.Maven, 5*time.Second, nil), // stalls like search.maven.org
	})
	start := time.Now()
	got, err := SearchEverywhere("x", Options{})
	if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
		t.Fatalf("took %v; a stalled registry held up the result", elapsed)
	}
	if err != nil {
		t.Fatalf("one straggler must not be an error, got %v", err)
	}
	if len(got) != 1 || got[0].Manager != pm.Npm {
		t.Fatalf("got %+v, want only the npm result", got)
	}
}

func TestSearchEverywhereAllTimedOutIsAnError(t *testing.T) {
	withDeadline(t, 50*time.Millisecond)
	withLookups(t, []lookup{fakeLookup(pm.Npm, time.Second, nil)})
	_, err := SearchEverywhere("x", Options{})
	if !errors.Is(err, ErrAllRegistriesFailed) || !errors.Is(err, ErrRegistryTimeout) {
		t.Fatalf("err = %v, want ErrAllRegistriesFailed wrapping ErrRegistryTimeout", err)
	}
}

func TestSearchEverywherePartialFailureKeepsResults(t *testing.T) {
	withLookups(t, []lookup{
		fakeLookup(pm.Npm, 0, errors.New("boom")),
		fakeLookup(pm.Pip, 0, nil),
	})
	got, err := SearchEverywhere("x", Options{})
	if err != nil {
		t.Fatalf("partial failure must not be an error, got %v", err)
	}
	if len(got) != 1 || got[0].Manager != pm.Pip {
		t.Fatalf("got %+v, want only the pip result", got)
	}
}

func TestSearchEverywhereAllFailedIsAnError(t *testing.T) {
	withLookups(t, []lookup{
		fakeLookup(pm.Npm, 0, errors.New("offline")),
		fakeLookup(pm.Pip, 0, errors.New("offline")),
	})
	_, err := SearchEverywhere("x", Options{})
	if !errors.Is(err, ErrAllRegistriesFailed) {
		t.Fatalf("err = %v, want ErrAllRegistriesFailed", err)
	}
}

func TestSearchEverywhereSkipsDisabled(t *testing.T) {
	called := false
	withLookups(t, []lookup{
		{id: pm.Npm, fn: func(string) (*Result, error) { called = true; return nil, nil }},
	})
	got, err := SearchEverywhere("x", Options{Enable: map[pm.ID]bool{pm.Npm: false}})
	if err != nil || len(got) != 0 || called {
		t.Fatalf("disabled registry was queried (called=%v got=%v err=%v)", called, got, err)
	}
}
