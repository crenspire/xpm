// Package search provides functionality to search for packages across multiple registries.
//
// This package queries package registries (npm, PyPI, Packagist, crates.io, Maven Central)
// concurrently to find packages by name. Results include package metadata such as version and description.
//
// Example usage:
//
//	opts := search.Options{
//	    Enable: map[pm.ID]bool{
//	        pm.Npm: true,
//	        pm.Pip: true,
//	    },
//	}
//	results, err := search.SearchEverywhere("lodash", opts)
package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// Result represents a package found in a registry search.
type Result struct {
	// Manager is the package manager that found this result.
	Manager pm.ID
	// Name is the package name (may differ from search query for fuzzy matches).
	Name string
	// Info is a short description of the package.
	Info string
	// Extra contains additional metadata like version, group (Maven), etc.
	Extra map[string]string
}

// Options controls search behavior.
type Options struct {
	// Enable specifies which package managers to include in the search.
	// If nil or a key is missing, that ecosystem is searched by default.
	Enable map[pm.ID]bool
}

// DefaultTimeout is the HTTP request timeout for registry queries.
const DefaultTimeout = 4 * time.Second

// httpClient is the shared HTTP client for all registry queries.
var httpClient = &http.Client{
	Timeout: DefaultTimeout,
}

// Enabled checks if a package manager is enabled in the given options.
// Returns true if the ecosystem should be searched.
func Enabled(opts Options, id pm.ID) bool {
	if opts.Enable == nil {
		return true
	}
	v, ok := opts.Enable[id]
	if !ok {
		return true
	}
	return v
}

// validatePackageNameForURL rejects names that are empty, too long, or could
// change the meaning of the registry URL they are spliced into
// (query/fragment/escape characters, whitespace, control characters, or
// "." / ".." path segments). Scoped names like "@types/node" and composer
// names like "vendor/pkg" are allowed.
func validatePackageNameForURL(pkg string) error {
	if len(pkg) == 0 {
		return fmt.Errorf("package name cannot be empty")
	}
	const maxPackageNameLength = 214 // npm's limit; the strictest registry
	if len(pkg) > maxPackageNameLength {
		return fmt.Errorf("package name too long (max %d characters)", maxPackageNameLength)
	}
	if strings.ContainsAny(pkg, "?#%\\") {
		return fmt.Errorf("invalid package name %q: contains a URL-reserved character", pkg)
	}
	for _, r := range pkg {
		if r <= ' ' || r == 0x7f {
			return fmt.Errorf("invalid package name %q: contains whitespace or control characters", pkg)
		}
	}
	for _, seg := range strings.Split(pkg, "/") {
		if seg == "." || seg == ".." {
			return fmt.Errorf("invalid package name %q: contains a relative path segment", pkg)
		}
	}
	return nil
}

// decodeJSON decodes at most maxMetadataBytes of body into v. Errors name
// the registry and package so "unexpected EOF" is never the whole message.
func decodeJSON(registry, pkg string, body io.Reader, v any) error {
	if err := json.NewDecoder(io.LimitReader(body, maxMetadataBytes)).Decode(v); err != nil {
		return fmt.Errorf("%s: decode %s: %w", registry, pkg, err)
	}
	return nil
}

// existsInNpm checks if a package exists in the npm registry.
// It fetches /<pkg>/latest (~2-4 KB) instead of the full packument, which is
// 15 MB+ for packages like typescript and regularly blew the 4s timeout.
// Returns (nil, nil) if the package is not found.
func existsInNpm(ctx context.Context, pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	u := fmt.Sprintf("%s/%s/latest", npmRegistryURL, url.PathEscape(pkg))
	logx.Info("query npm: %s", u)
	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("npm", resp)
	}
	var data struct {
		Version     string `json:"version"`
		Description string `json:"description"`
	}
	if err := decodeJSON("npm", pkg, resp.Body, &data); err != nil {
		return nil, err
	}
	return &Result{
		Manager: pm.Npm,
		Name:    pkg,
		Info:    data.Description,
		Extra:   map[string]string{"version": data.Version},
	}, nil
}

// existsInPip checks if a package exists in the Python Package Index (PyPI).
// Returns (nil, nil) if the package is not found.
func existsInPip(ctx context.Context, pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	u := fmt.Sprintf("%s/%s/json", pypiURL, url.PathEscape(pkg))
	logx.Info("query pypi: %s", u)
	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("pypi", resp)
	}
	var data struct {
		Info struct {
			Name    string `json:"name"`
			Summary string `json:"summary"`
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := decodeJSON("pypi", pkg, resp.Body, &data); err != nil {
		return nil, err
	}
	name := data.Info.Name
	if name == "" {
		name = pkg
	}
	return &Result{
		Manager: pm.Pip,
		Name:    name,
		Info:    data.Info.Summary,
		Extra:   map[string]string{"version": data.Info.Version},
	}, nil
}

// existsInComposer searches Packagist and returns the first hit, which may
// be a fuzzy match ("monolog" -> "monolog/monolog"). Returns (nil, nil) if
// there are no hits.
func existsInComposer(ctx context.Context, pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	u := fmt.Sprintf("%s/search.json?q=%s", packagistURL, url.QueryEscape(pkg))
	logx.Info("query packagist: %s", u)
	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("packagist", resp)
	}
	var data struct {
		Results []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"results"`
	}
	if err := decodeJSON("packagist", pkg, resp.Body, &data); err != nil {
		return nil, err
	}
	if len(data.Results) == 0 {
		return nil, nil
	}
	first := data.Results[0]
	return &Result{
		Manager: pm.Composer,
		Name:    first.Name,
		Info:    first.Description,
		Extra:   map[string]string{},
	}, nil
}

// existsInMaven searches Maven Central and returns the first hit.
// Returns (nil, nil) if there are no hits.
func existsInMaven(ctx context.Context, pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	u := fmt.Sprintf("%s?q=%s&rows=5&wt=json", mavenSearchURL, url.QueryEscape(pkg))
	logx.Info("query maven: %s", u)
	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("maven", resp)
	}
	var data mavenSearchResponse
	if err := decodeJSON("maven", pkg, resp.Body, &data); err != nil {
		return nil, err
	}
	if len(data.Response.Docs) == 0 {
		return nil, nil
	}
	d := data.Response.Docs[0]
	return &Result{
		Manager: pm.Maven,
		Name:    d.Group + ":" + d.Artifact,
		Info:    "Maven artifact",
		Extra: map[string]string{
			"version":  d.LatestVersion,
			"id":       d.ID,
			"group":    d.Group,
			"artifact": d.Artifact,
		},
	}, nil
}

// lookup is one registry's exact-name check. fn must honour ctx.
type lookup struct {
	id pm.ID
	fn func(ctx context.Context, pkg string) (*Result, error)
}

// exactLookups is the registry table used by SearchEverywhere, in result order.
// Tests replace it with fakes.
var exactLookups = []lookup{
	{id: pm.Npm, fn: existsInNpm},
	{id: pm.Pip, fn: existsInPip},
	{id: pm.Composer, fn: existsInComposer},
	{id: pm.Cargo, fn: existsInCrates},
	{id: pm.Maven, fn: existsInMaven},
}

// lookupDeadline bounds the whole fan-out. Registry APIs have long tails
// (crates.io has been measured at 46s, Maven search stalls without ever
// answering), so a registry that misses the deadline is reported as timed
// out instead of holding up the answer.
var lookupDeadline = 2500 * time.Millisecond

var (
	// ErrAllRegistriesFailed is returned when every enabled registry errored
	// or timed out, so callers can tell "offline" from "no such package".
	ErrAllRegistriesFailed = errors.New("all registries failed")
	// ErrRegistryTimeout marks a registry that missed lookupDeadline.
	ErrRegistryTimeout = errors.New("registry did not answer in time")
)

// SearchEverywhere checks every enabled registry concurrently for an exact
// package name and returns within lookupDeadline. Results are returned in
// table order. A failing or slow registry is logged and skipped; only if all
// of them fail is an error (wrapping ErrAllRegistriesFailed) returned.
// Requests still running at the deadline are cancelled.
func SearchEverywhere(pkg string, opts Options) ([]Result, error) {
	var enabled []lookup
	for _, l := range exactLookups {
		if Enabled(opts, l.id) {
			enabled = append(enabled, l)
		}
	}
	if len(enabled) == 0 {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), lookupDeadline)
	defer cancel()

	type outcome struct {
		i   int
		res *Result
		err error
	}
	// Buffered so goroutines that finish after the deadline never block.
	ch := make(chan outcome, len(enabled))
	cacheDir := lookupCacheDir
	for i, l := range enabled {
		go func(i int, l lookup) {
			res, err := cachedLookup(ctx, cacheDir, l, pkg)
			ch <- outcome{i: i, res: res, err: err}
		}(i, l)
	}

	outcomes := make([]outcome, len(enabled))
	for i := range outcomes {
		outcomes[i] = outcome{i: i, err: ErrRegistryTimeout}
	}
	timer := time.NewTimer(lookupDeadline)
	defer timer.Stop()
collect:
	for received := 0; received < len(enabled); received++ {
		select {
		case o := <-ch:
			outcomes[o.i] = o
		case <-timer.C:
			break collect
		}
	}

	var out []Result
	var errs []error
	for i, o := range outcomes {
		if o.err != nil {
			logx.Info("lookup %s in %s failed: %v", pkg, enabled[i].id, o.err)
			errs = append(errs, fmt.Errorf("%s: %w", enabled[i].id, o.err))
			continue
		}
		if o.res != nil {
			out = append(out, *o.res)
		}
	}
	if len(errs) == len(enabled) {
		return nil, fmt.Errorf("%w: %w", ErrAllRegistriesFailed, errors.Join(errs...))
	}
	return out, nil
}
