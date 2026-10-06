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
