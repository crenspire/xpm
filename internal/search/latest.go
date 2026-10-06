package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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

// ErrNotChecked marks a query that was deliberately not sent to a registry.
var ErrNotChecked = errors.New("not checked")

// goProxyFor returns the proxy base URL for mod, honouring GOPRIVATE,
// GONOPROXY and GOPROXY (first entry). Private modules and GOPROXY=off or
// direct are not looked up, so private module paths never leak.
func goProxyFor(mod string) (string, error) {
	if module.MatchPrefixPatterns(os.Getenv("GOPRIVATE"), mod) || module.MatchPrefixPatterns(os.Getenv("GONOPROXY"), mod) {
		return "", fmt.Errorf("%w: private module (GOPRIVATE/GONOPROXY)", ErrNotChecked)
	}
	env := os.Getenv("GOPROXY")
	if env == "" {
		return goProxyURL, nil
	}
	parts := strings.FieldsFunc(env, func(r rune) bool { return r == ',' || r == '|' })
	if len(parts) == 0 {
		return goProxyURL, nil
	}
	first := strings.TrimSpace(parts[0])
	if first == "off" || first == "direct" || first == "" {
		return "", fmt.Errorf("%w: GOPROXY=%s", ErrNotChecked, first)
	}
	return strings.TrimRight(first, "/"), nil
}

// latestGoModule asks the Go module proxy for the newest version of module.
// Returns (nil, nil) if the proxy does not know it.
func latestGoModule(ctx context.Context, mod string) (*Result, error) {
	escaped, err := module.EscapePath(mod)
	if err != nil {
		return nil, fmt.Errorf("invalid module path: %w", err)
	}
	base, err := goProxyFor(mod)
	if err != nil {
		return nil, err
	}
	u := base + "/" + escaped + "/@latest"
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

// maxComposerBytes caps what latestComposer reads from a Packagist p2 file.
// The big ones are a few MB (aws/aws-sdk-php is ~1.6 MB); they are streamed,
// not held in memory.
const maxComposerBytes = 32 << 20

// latestComposer reads the newest version of a Packagist package from its p2
// metadata (the search API used by existsInComposer carries no version). The
// p2 file lists versions newest first, so the stream stops at the first
// stable version; if the package has none, the newest pre-release is
// returned. Returns (nil, nil) for unknown packages.
func latestComposer(ctx context.Context, pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	vendor, name, ok := strings.Cut(strings.ToLower(pkg), "/")
	if !ok || vendor == "" || name == "" || strings.Contains(name, "/") {
		return nil, fmt.Errorf("invalid package name: %q is not vendor/name", pkg)
	}
	key := vendor + "/" + name
	u := fmt.Sprintf("%s/p2/%s/%s.json", packagistURL, url.PathEscape(vendor), url.PathEscape(name))
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
	version, err := streamComposerVersion(io.LimitReader(resp.Body, maxComposerBytes), key)
	if err != nil {
		return nil, fmt.Errorf("packagist: decode %s: %w", pkg, err)
	}
	if version == "" {
		return nil, nil
	}
	return &Result{Manager: pm.Composer, Name: key, Extra: map[string]string{"version": version}}, nil
}

// streamComposerVersion walks {"packages":{key:[{"version":...},...]}} and
// returns the first stable version of key (else the first version at all).
func streamComposerVersion(r io.Reader, key string) (string, error) {
	dec := json.NewDecoder(r)
	expect := func(want json.Delim) error {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		if d, ok := t.(json.Delim); !ok || d != want {
			return fmt.Errorf("unexpected token %v", t)
		}
		return nil
	}
	if err := expect('{'); err != nil {
		return "", err
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return "", err
		}
		if k, _ := t.(string); k != "packages" {
			var skip json.RawMessage
			if err := dec.Decode(&skip); err != nil {
				return "", err
			}
			continue
		}
		if err := expect('{'); err != nil {
			return "", err
		}
		for dec.More() {
			t, err := dec.Token()
			if err != nil {
				return "", err
			}
			if k, _ := t.(string); k != key {
				var skip json.RawMessage
				if err := dec.Decode(&skip); err != nil {
					return "", err
				}
				continue
			}
			if err := expect('['); err != nil {
				return "", err
			}
			first := ""
			for dec.More() {
				var v struct {
					Version string `json:"version"`
				}
				if err := dec.Decode(&v); err != nil {
					return "", err
				}
				if v.Version == "" {
					continue
				}
				if first == "" {
					first = v.Version
				}
				if composerStable(v.Version) {
					return v.Version, nil
				}
			}
			return first, nil
		}
	}
	return "", nil
}

// composerPreRelease matches Composer's unstable suffixes (dev, alpha/a,
// beta/b, RC); patch-level suffixes (p, pl, patch) are stable.
var composerPreRelease = regexp.MustCompile(`(?i)\d[._-]?(dev|alpha|a|beta|b|rc)(?:[.-]?\d+)*$`)

func composerStable(v string) bool {
	v = strings.ToLower(v)
	if strings.HasPrefix(v, "dev-") || strings.HasSuffix(v, "-dev") {
		return false
	}
	return !composerPreRelease.MatchString(v)
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
		sem <- struct{}{}
		wg.Add(1)
		go func(i int, q LatestQuery) {
			defer wg.Done()
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
