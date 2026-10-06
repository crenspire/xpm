package search

import (
	"context"
	"sync"
	"time"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// SearchResult wraps a result with its source manager and any error.
type SearchResult struct {
	Result  *Result
	Manager pm.ID
	Err     error
}

// SearchFunc is a function that searches for a package in a registry.
type SearchFunc func(pkg string) (*Result, error)

// ParallelSearchConfig configures parallel search behavior.
type ParallelSearchConfig struct {
	// Timeout is the maximum time to wait for all searches.
	Timeout time.Duration
	// ContinueOnError controls whether to continue if some searches fail.
	ContinueOnError bool
}

// DefaultParallelSearchConfig returns the default parallel search configuration.
func DefaultParallelSearchConfig() ParallelSearchConfig {
	return ParallelSearchConfig{
		Timeout:         10 * time.Second,
		ContinueOnError: true,
	}
}

// SearchEverywhereParallel searches all enabled registries in parallel.
// This is an optimized version of SearchEverywhere that uses goroutines.
func SearchEverywhereParallel(pkg string, opts Options) ([]Result, error) {
	return SearchEverywhereParallelWithConfig(pkg, opts, DefaultParallelSearchConfig())
}

// SearchEverywhereParallelWithConfig searches with custom configuration.
func SearchEverywhereParallelWithConfig(pkg string, opts Options, cfg ParallelSearchConfig) ([]Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// Define search functions for each registry
	// Use proper search APIs that return multiple results instead of existence checks
	searches := []struct {
		manager pm.ID
		fn      func(string) ([]Result, error)
	}{
		{pm.Npm, searchNpmMultiple},
		{pm.Pip, searchPipMultiple},
		{pm.Composer, searchComposerMultiple},
		{pm.Cargo, searchCargoMultiple},
		{pm.Maven, searchMavenMultiple},
	}

	// Filter to only enabled searches
	var enabledSearches []struct {
		manager pm.ID
		fn      func(string) ([]Result, error)
	}
	for _, s := range searches {
		if Enabled(opts, s.manager) {
			enabledSearches = append(enabledSearches, s)
		}
	}

	if len(enabledSearches) == 0 {
		return nil, nil
	}

	// Results channel
	resultCh := make(chan SearchResult, len(enabledSearches))

	// WaitGroup to track goroutines
	var wg sync.WaitGroup

	// Launch goroutines for each search
	for _, s := range enabledSearches {
		wg.Add(1)
		go func(manager pm.ID, fn func(string) ([]Result, error)) {
			defer wg.Done()

			// Check context before starting
			select {
			case <-ctx.Done():
				logx.Info("search for %s in %s cancelled before start", pkg, manager)
				return
			default:
			}

			// Perform search (this may take time, but we check context after)
			results, err := fn(pkg)

			// Check context before sending results
			select {
			case <-ctx.Done():
				logx.Info("search for %s in %s cancelled after completion", pkg, manager)
				return
			default:
			}

			// Send all results
			if err != nil {
				select {
				case resultCh <- SearchResult{Result: nil, Manager: manager, Err: err}:
				case <-ctx.Done():
					logx.Info("search for %s in %s cancelled while sending error", pkg, manager)
				}
				return
			}

			// Send each result individually
			for _, result := range results {
				select {
				case resultCh <- SearchResult{Result: &result, Manager: manager, Err: nil}:
				case <-ctx.Done():
					logx.Info("search for %s in %s cancelled while sending results", pkg, manager)
					return
				}
			}

			// If no results were sent (empty slice), send a completion marker
			// This ensures the search is marked as completed even with no results
			if len(results) == 0 {
				select {
				case resultCh <- SearchResult{Result: nil, Manager: manager, Err: nil}:
				case <-ctx.Done():
					logx.Info("search for %s in %s cancelled while sending empty result", pkg, manager)
				}
			}
		}(s.manager, s.fn)
	}

	// Close results channel when all searches complete
	// Use a separate goroutine to avoid deadlock if all goroutines are blocked
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Collect results with timeout protection to prevent deadlock
	var results []Result
	var errors []error
	completedSearches := make(map[pm.ID]bool)
	expectedSearches := len(enabledSearches)

	// Collect results until all searches complete or timeout
	for len(completedSearches) < expectedSearches {
		select {
		case sr, ok := <-resultCh:
			if !ok {
				// Channel closed, all searches complete
				if len(errors) > 0 && !cfg.ContinueOnError {
					return results, errors[0]
				}
				return results, nil
			}

			if sr.Err != nil {
				logx.Info("search error for %s in %s: %v", pkg, sr.Manager, sr.Err)
				errors = append(errors, sr.Err)
				// Mark this search as completed (even if it errored)
				completedSearches[sr.Manager] = true
				continue
			}

			if sr.Result != nil {
				results = append(results, *sr.Result)
			} else {
				// No result but no error - search completed with no results
				// Mark as completed to avoid infinite loop
				if !completedSearches[sr.Manager] {
					completedSearches[sr.Manager] = true
					logx.Info("search for %s in %s completed with no results", pkg, sr.Manager)
				}
			}

		case <-ctx.Done():
			logx.Info("parallel search timed out")
			return results, ctx.Err()
		}
	}

	// All searches completed, return
	if len(errors) > 0 && !cfg.ContinueOnError {
		return results, errors[0]
	}
	return results, nil
}

// SearchRegistriesParallel searches specific registries in parallel.
func SearchRegistriesParallel(pkg string, managers []pm.ID) ([]Result, error) {
	opts := Options{
		Enable: make(map[pm.ID]bool),
	}

	// Disable all
	for _, m := range []pm.ID{pm.Npm, pm.Pip, pm.Composer, pm.Cargo, pm.Maven} {
		opts.Enable[m] = false
	}

	// Enable requested
	for _, m := range managers {
		opts.Enable[m] = true
	}

	return SearchEverywhereParallel(pkg, opts)
}

// Wrapper functions that use proper search APIs instead of existence checks
func searchNpmMultiple(query string) ([]Result, error) {
	return SearchNpmPackages(query, 20) // Limit to 20 results
}

func searchPipMultiple(query string) ([]Result, error) {
	// PyPI doesn't have a proper search API, so we try exact match
	// and also try a few common variations
	result, err := existsInPip(query)
	if err != nil {
		return nil, err
	}
	if result != nil {
		return []Result{*result}, nil
	}
	return nil, nil
}

func searchComposerMultiple(query string) ([]Result, error) {
	return SearchPackagistPackages(query, 20) // Limit to 20 results
}

func searchCargoMultiple(query string) ([]Result, error) {
	return SearchCratesIO(query, 20) // Limit to 20 results
}

func searchMavenMultiple(query string) ([]Result, error) {
	return SearchMavenCentral(query, 20) // Limit to 20 results
}

// BatchSearch searches for multiple packages in parallel.
// Returns a copy of the results map to prevent race conditions when the caller accesses it.
func BatchSearch(packages []string, opts Options) map[string][]Result {
	results := make(map[string][]Result)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, pkg := range packages {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()

			res, err := SearchEverywhereParallel(p, opts)
			if err != nil {
				logx.Info("batch search error for %s: %v", p, err)
				return
			}

			mu.Lock()
			results[p] = res
			mu.Unlock()
		}(pkg)
	}

	wg.Wait()

	// Return a copy to prevent race conditions when caller accesses the map
	mu.Lock()
	resultCopy := make(map[string][]Result, len(results))
	for k, v := range results {
		// Copy the slice as well
		vCopy := make([]Result, len(v))
		copy(vCopy, v)
		resultCopy[k] = vCopy
	}
	mu.Unlock()

	return resultCopy
}
