package search

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/mod/module"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// latestConcurrency bounds how many registry requests LatestVersions has in
// flight at once.
const latestConcurrency = 8

// LatestQuery is one package to look up in one registry.
type LatestQuery struct {
	Manager pm.ID // pm.Npm, pm.Pip, pm.Composer, pm.Cargo, pm.Maven or pm.GoMod
	Name    string
}

// LatestResult is the newest version a registry reports for a query.
// Found=false with Err=nil means the registry answered "no such package".
type LatestResult struct {
	LatestQuery
	Version string
	Found   bool
	Err     error
}

// ErrRegistryDisabled marks a query whose registry is turned off in the config.
var ErrRegistryDisabled = errors.New("registry disabled in config")

// latestGoModule asks the Go module proxy for the newest version of module.
// Returns (nil, nil) if the proxy does not know it.
func latestGoModule(ctx context.Context, mod string) (*Result, error) {
	escaped, err := module.EscapePath(mod)
	if err != nil {
		return nil, fmt.Errorf("invalid module path: %w", err)
	}
	u := goProxyURL + "/" + escaped + "/@latest"
	logx.Info("query go proxy: %s", u)
	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("go proxy", resp)
	}
	var data struct {
		Version string `json:"Version"`
	}
	if err := decodeJSON("go proxy", mod, resp.Body, &data); err != nil {
		return nil, err
	}
	if data.Version == "" {
		return nil, nil
	}
	return &Result{Manager: pm.GoMod, Name: mod, Extra: map[string]string{"version": data.Version}}, nil
}

// latestComposer reads the newest stable version of a Packagist package from
// its p2 metadata (the search API used by existsInComposer carries no
// version). Returns (nil, nil) for unknown packages.
func latestComposer(ctx context.Context, pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	if strings.Count(pkg, "/") != 1 {
		return nil, nil
	}
	u := fmt.Sprintf("%s/p2/%s.json", packagistURL, strings.ToLower(pkg))
	logx.Info("query packagist: %s", u)
	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("packagist", resp)
	}
	var data struct {
		Packages map[string][]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	if err := decodeJSON("packagist", pkg, resp.Body, &data); err != nil {
		return nil, err
	}
	for name, versions := range data.Packages {
		best := ""
		for _, v := range versions { // newest first
			if v.Version == "" {
				continue
			}
			if best == "" {
				best = v.Version
			}
			if composerStable(v.Version) {
				best = v.Version
				break
			}
		}
		if best == "" {
			return nil, nil
		}
		return &Result{Manager: pm.Composer, Name: name, Extra: map[string]string{"version": best}}, nil
	}
	return nil, nil
}

func composerStable(v string) bool {
	v = strings.ToLower(v)
	for _, s := range []string{"dev", "alpha", "beta", "rc"} {
		if strings.Contains(v, s) {
			return false
		}
	}
	return true
}

var pepSeparators = regexp.MustCompile(`[-_.]+`)

// sameName reports whether a registry's answer is for the package asked about,
// under the registry's own naming rules.
func sameName(m pm.ID, query, got string) bool {
	switch m {
	case pm.Pip:
		return pepSeparators.ReplaceAllString(strings.ToLower(query), "-") ==
			pepSeparators.ReplaceAllString(strings.ToLower(got), "-")
	case pm.Cargo:
		return strings.EqualFold(strings.ReplaceAll(query, "_", "-"), strings.ReplaceAll(got, "_", "-"))
	case pm.Npm, pm.Composer:
		return strings.EqualFold(query, got)
	default: // Maven, Go: exact
		return query == got
	}
}

func latestLookupFor(m pm.ID) (lookup, bool) {
	if m == pm.GoMod {
		return lookup{id: pm.GoMod, fn: latestGoModule}, true
	}
	if m == pm.Composer {
		return lookup{id: pm.Composer, fn: latestComposer}, true
	}
	for _, l := range exactLookups {
		if l.id == m {
			return l, true
		}
	}
	return lookup{}, false
}

// LatestVersions looks up every query concurrently (at most latestConcurrency
// in flight), each bounded by opts' timeout for its registry, through the
// on-disk lookup cache. Results are in query order.
func LatestVersions(queries []LatestQuery, opts Options) []LatestResult {
	out := make([]LatestResult, len(queries))
	cacheDir := lookupCacheDir
	sem := make(chan struct{}, latestConcurrency)
	var wg sync.WaitGroup
	for i, q := range queries {
		out[i].LatestQuery = q
		l, ok := latestLookupFor(q.Manager)
		if !ok {
			out[i].Err = fmt.Errorf("no registry lookup for %s", q.Manager)
			continue
		}
		if q.Manager != pm.GoMod && !Enabled(opts, q.Manager) {
			out[i].Err = ErrRegistryDisabled
			continue
		}
		timeout := opts.timeoutFor(q.Manager)
		dir := cacheDir
		if dir != "" && q.Manager == pm.Composer {
			// Packagist's search lookup caches version-less results under
			// the same key; keep the version lookups apart.
			dir = filepath.Join(dir, "latest")
		}
		wg.Add(1)
		go func(i int, q LatestQuery) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			res, err := cachedLookup(ctx, dir, l, q.Name)
			if err != nil {
				out[i].Err = sanitizedError{asTimeout(err)}
				return
			}
			if res == nil || !sameName(q.Manager, q.Name, res.Name) {
				return
			}
			if v := SanitizeText(res.Extra["version"]); v != "" {
				out[i].Version, out[i].Found = v, true
			}
		}(i, q)
	}
	wg.Wait()
	return out
}
