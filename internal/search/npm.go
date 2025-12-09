package search

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// NpmRegistryURL is the base URL for the npm registry API.
const NpmRegistryURL = "https://registry.npmjs.org"

// npmPackageResponse represents the npm registry response for a package.
type npmPackageResponse struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	DistTags    map[string]string `json:"dist-tags"`
	Repository  struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"repository"`
	Homepage string `json:"homepage"`
	License  string `json:"license"`
}

// searchNpm searches for a package in the npm registry.
// Returns nil if the package is not found.
func searchNpm(pkg string) (*Result, error) {
	url := fmt.Sprintf("%s/%s", NpmRegistryURL, url.PathEscape(pkg))
	logx.Info("query npm: %s", url)

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("npm request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil // Package not found
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("npm returned status %d", resp.StatusCode)
	}

	var data npmPackageResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode npm response: %w", err)
	}

	result := &Result{
		Manager: pm.Npm,
		Name:    data.Name,
		Info:    data.Description,
		Extra:   make(map[string]string),
	}

	if version, ok := data.DistTags["latest"]; ok {
		result.Extra["version"] = version
	}

	if data.Homepage != "" {
		result.Extra["homepage"] = data.Homepage
	}

	if data.License != "" {
		result.Extra["license"] = data.License
	}

	return result, nil
}

// SearchNpmPackages searches npm for packages matching a query.
// This uses the npm search API for broader results.
func SearchNpmPackages(query string, limit int) ([]Result, error) {
	url := fmt.Sprintf("https://registry.npmjs.org/-/v1/search?text=%s&size=%d", url.QueryEscape(query), limit)
	logx.Info("search npm: %s", url)

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("npm search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("npm search returned status %d", resp.StatusCode)
	}

	var data struct {
		Objects []struct {
			Package struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Version     string `json:"version"`
			} `json:"package"`
		} `json:"objects"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode npm search response: %w", err)
	}

	var results []Result
	for _, obj := range data.Objects {
		results = append(results, Result{
			Manager: pm.Npm,
			Name:    obj.Package.Name,
			Info:    obj.Package.Description,
			Extra:   map[string]string{"version": obj.Package.Version},
		})
	}

	return results, nil
}

