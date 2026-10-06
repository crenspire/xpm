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

// Options controls search behavior. Build it with OptionsFromConfig.
type Options struct {
	// Enable specifies which package managers to include in the search.
	// If nil or a key is missing, that ecosystem is searched by default.
	Enable map[pm.ID]bool
	// Timeout bounds each registry and therefore the whole fan-out;
	// 0 means lookupDeadline (2.5 s).
	Timeout time.Duration
	// RegistryTimeout overrides Timeout for individual registries.
	RegistryTimeout map[pm.ID]time.Duration
}

// DefaultTimeout is the HTTP request timeout for registry queries.
const DefaultTimeout = 4 * time.Second

// httpClient is the shared HTTP client for all registry queries. Every
// request carries a context deadline (Options.timeoutFor); the client
// timeout is only a backstop for callers that pass context.Background().
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
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

// existsInComposer searches Packagist and returns the hit named like the
// query: "<q>" or "<q>/<q>" (monolog -> monolog/monolog), then "*/<q>",
// ignoring case. Packagist ranks by popularity, so that hit is often not
// first; without one the first hit is returned, which may be unrelated
// (axios -> swlib/saber). Returns (nil, nil) if there are no hits.
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
	names := make([]string, len(data.Results))
	for i, r := range data.Results {
		names[i] = r.Name
	}
	best := data.Results[composerBestHit(pkg, names)]
	return &Result{
		Manager: pm.Composer,
		Name:    best.Name,
		Info:    best.Description,
		Extra:   map[string]string{},
	}, nil
}

// composerBestHit returns the index of the Packagist name that best
// matches query: "<q>" or "<q>/<q>", then "<vendor>/<q>", else the first.
func composerBestHit(query string, names []string) int {
	q := strings.ToLower(query)
	vendorMatch := -1
	for i, n := range names {
		n = strings.ToLower(n)
		if n == q || n == q+"/"+q {
			return i
		}
		if vendorMatch == -1 && strings.HasSuffix(n, "/"+q) {
			vendorMatch = i
		}
	}
	return max(vendorMatch, 0)
}

// mavenRows is how many Maven Central hits an exact lookup scans for the
// artifact named like the query.
const mavenRows = 10

// existsInMaven searches Maven Central and returns the first hit whose
// artifactId is the query (case-sensitive: "Express" is not "express") or
// whose group:artifact is the query; without one, the first hit, which may
// be unrelated. Returns (nil, nil) if there are no hits.
func existsInMaven(ctx context.Context, pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	u := fmt.Sprintf("%s?q=%s&rows=%d&wt=json", mavenSearchURL, url.QueryEscape(pkg), mavenRows)
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
	for _, doc := range data.Response.Docs {
		if doc.Artifact == pkg || doc.Group+":"+doc.Artifact == pkg {
			d = doc
			break
		}
	}
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
