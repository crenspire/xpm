// Package search provides functionality to search for packages across multiple registries.
//
// This package queries package registries (npm, PyPI, Packagist, crates.io, Maven Central)
// to find packages by name. Results include package metadata such as version and description.
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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// validatePackageNameForURL validates package name length for URL construction.
// Maximum URL length is typically 2048 characters, but we use a conservative limit.
// Returns an error if the package name is too long or empty.
//
// Edge cases:
//   - Empty string: returns error
//   - Names longer than 214 characters: returns error (npm package name limit)
//   - Valid names: returns nil
//
// This prevents DoS attacks via extremely long package names and ensures
// URLs remain within reasonable limits.
func validatePackageNameForURL(pkg string) error {
	if len(pkg) == 0 {
		return fmt.Errorf("package name cannot be empty")
	}
	// Conservative limit: 214 characters (npm package name limit)
	// This leaves room for the base URL and query parameters
	const maxPackageNameLength = 214
	if len(pkg) > maxPackageNameLength {
		return fmt.Errorf("package name too long (max %d characters)", maxPackageNameLength)
	}
	return nil
}

// existsInNpm checks if a package exists in the npm registry.
// Returns nil if the package is not found.
func existsInNpm(pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	url := fmt.Sprintf("https://registry.npmjs.org/%s", url.PathEscape(pkg))
	logx.Info("query npm: %s", url)
    resp, err := httpClient.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
		// Read error body for debugging
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 404 {
			return nil, nil // Package not found
		}
		return nil, fmt.Errorf("npm registry returned status %d: %s", resp.StatusCode, string(body))
    }
    var data struct {
        Description string            `json:"description"`
        DistTags    map[string]string `json:"dist-tags"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
        return nil, err
    }
    res := &Result{
        Manager: pm.Npm,
        Name:    pkg,
        Info:    data.Description,
        Extra:   map[string]string{},
    }
    if v, ok := data.DistTags["latest"]; ok {
        res.Extra["version"] = v
    }
    return res, nil
}

// existsInPip checks if a package exists in the Python Package Index (PyPI).
// Returns nil if the package is not found.
func existsInPip(pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	url := fmt.Sprintf("https://pypi.org/pypi/%s/json", url.PathEscape(pkg))
	logx.Info("query pypi: %s", url)
    resp, err := httpClient.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
		// Read error body for debugging
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 404 {
			return nil, nil // Package not found
		}
		return nil, fmt.Errorf("PyPI returned status %d: %s", resp.StatusCode, string(body))
    }
    var data struct {
        Info struct {
            Summary string `json:"summary"`
            Version string `json:"version"`
        } `json:"info"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
        return nil, err
    }
    return &Result{
        Manager: pm.Pip,
        Name:    pkg,
        Info:    data.Info.Summary,
        Extra:   map[string]string{"version": data.Info.Version},
    }, nil
}

// existsInComposer searches for a package in Packagist (PHP/Composer registry).
// Returns the first matching result or nil if no matches found.
func existsInComposer(pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	url := fmt.Sprintf("https://packagist.org/search.json?q=%s", url.QueryEscape(pkg))
	logx.Info("query packagist: %s", url)
    resp, err := httpClient.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
		// Read error body for debugging
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Packagist returned status %d: %s", resp.StatusCode, string(body))
    }
    var data struct {
        Results []struct {
            Name        string `json:"name"`
            Description string `json:"description"`
        } `json:"results"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
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

// existsInCrates checks if a crate exists in crates.io (Rust registry).
// Returns nil if the crate is not found.
func existsInCrates(pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	url := fmt.Sprintf("https://crates.io/api/v1/crates/%s", url.PathEscape(pkg))
	logx.Info("query crates.io: %s", url)
    resp, err := httpClient.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
		// Read error body for debugging
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == 404 {
			return nil, nil // Crate not found
		}
		return nil, fmt.Errorf("crates.io returned status %d: %s", resp.StatusCode, string(body))
    }
    var data struct {
        Crate struct {
            Description string `json:"description"`
            MaxVersion  string `json:"max_version"`
            Name        string `json:"name"`
        } `json:"crate"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
        return nil, err
    }
    return &Result{
        Manager: pm.Cargo,
        Name:    data.Crate.Name,
        Info:    data.Crate.Description,
        Extra:   map[string]string{"version": data.Crate.MaxVersion},
    }, nil
}

// existsInMaven searches for an artifact in Maven Central.
// Returns the first matching result or nil if no matches found.
func existsInMaven(pkg string) (*Result, error) {
	if err := validatePackageNameForURL(pkg); err != nil {
		return nil, fmt.Errorf("invalid package name: %w", err)
	}
	url := fmt.Sprintf("https://search.maven.org/solrsearch/select?q=%s&rows=5&wt=json", url.QueryEscape(pkg))
	logx.Info("query maven: %s", url)
    resp, err := httpClient.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
		// Read error body for debugging
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Maven Central returned status %d: %s", resp.StatusCode, string(body))
    }
    var data struct {
        Response struct {
            Docs []struct {
                ID       string `json:"id"`
                Latest   string `json:"latestVersion"`
                Group    string `json:"g"`
                Artifact string `json:"a"`
            } `json:"docs"`
        } `json:"response"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
        return nil, err
    }
    if len(data.Response.Docs) == 0 {
        return nil, nil
    }
    d := data.Response.Docs[0]
    coord := fmt.Sprintf("%s:%s", d.Group, d.Artifact)
    return &Result{
        Manager: pm.Maven,
        Name:    coord,
        Info:    "Maven artifact",
        Extra: map[string]string{
            "version":  d.Latest,
            "id":       d.ID,
            "group":    d.Group,
            "artifact": d.Artifact,
        },
    }, nil
}

// SearchEverywhere searches for a package across all enabled registries.
// It queries npm, PyPI, Packagist, crates.io, and Maven Central in sequence.
// Errors from individual registries are logged but don't stop the search.
// Returns a slice of all found results.
func SearchEverywhere(pkg string, opts Options) ([]Result, error) {
	var out []Result

    if Enabled(opts, pm.Npm) {
        if r, err := existsInNpm(pkg); err == nil && r != nil {
            out = append(out, *r)
        }
    }
    if Enabled(opts, pm.Pip) {
        if r, err := existsInPip(pkg); err == nil && r != nil {
            out = append(out, *r)
        }
    }
    if Enabled(opts, pm.Composer) {
        if r, err := existsInComposer(pkg); err == nil && r != nil {
            out = append(out, *r)
        }
    }
    if Enabled(opts, pm.Cargo) {
        if r, err := existsInCrates(pkg); err == nil && r != nil {
            out = append(out, *r)
        }
    }
    if Enabled(opts, pm.Maven) {
        if r, err := existsInMaven(pkg); err == nil && r != nil {
            out = append(out, *r)
        }
    }

    return out, nil
}
