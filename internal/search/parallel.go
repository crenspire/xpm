package search

import (
	"context"

	"github.com/crenspire/xpm/internal/pm"
)

// multiLookup is one registry's multi-result search. fn must honour ctx.
type multiLookup struct {
	id pm.ID
	fn func(ctx context.Context, query string) ([]Result, error)
}

// multiLookups is the registry table used by SearchReport (the TUI and
// `xpm search`), in result order. Tests replace it with fakes.
var multiLookups = []multiLookup{
	{id: pm.Npm, fn: searchNpmMultiple},
	{id: pm.Pip, fn: searchPipMultiple},
	{id: pm.Composer, fn: searchComposerMultiple},
	{id: pm.Cargo, fn: searchCargoMultiple},
	{id: pm.Maven, fn: searchMavenMultiple},
}

// SearchReport runs every enabled registry's search API concurrently, with
// the same deadline and disk cache as SearchEverywhereReport.
func SearchReport(query string, opts Options) (Report, error) {
	cacheDir := lookupCacheDir
	var calls []registryCall
	for _, m := range multiLookups {
		if !Enabled(opts, m.id) {
			continue
		}
		calls = append(calls, registryCall{id: m.id, timeout: lookupDeadline, fn: func(ctx context.Context) ([]Result, error) {
			return cachedSearch(ctx, cacheDir, m, query)
		}})
	}
	return runReport(calls)
}

// SearchEverywhereParallel returns SearchReport's results, or an error if
// every registry failed.
func SearchEverywhereParallel(query string, opts Options) ([]Result, error) {
	rep, err := SearchReport(query, opts)
	if err != nil {
		return nil, err
	}
	return rep.Results, nil
}

func searchNpmMultiple(ctx context.Context, query string) ([]Result, error) {
	return SearchNpmPackages(ctx, query, 20)
}

// searchPipMultiple uses the exact lookup: PyPI has no search API.
func searchPipMultiple(ctx context.Context, query string) ([]Result, error) {
	result, err := existsInPip(ctx, query)
	if err != nil || result == nil {
		return nil, err
	}
	return []Result{*result}, nil
}

func searchComposerMultiple(ctx context.Context, query string) ([]Result, error) {
	return SearchPackagistPackages(ctx, query, 20)
}

func searchCargoMultiple(ctx context.Context, query string) ([]Result, error) {
	return SearchCratesIO(ctx, query, 20)
}

func searchMavenMultiple(ctx context.Context, query string) ([]Result, error) {
	return SearchMavenCentral(ctx, query, 20)
}
