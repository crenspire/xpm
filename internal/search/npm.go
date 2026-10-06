package search

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// NpmRegistryURL is the base URL for the npm registry API.
const NpmRegistryURL = "https://registry.npmjs.org"

// SearchNpmPackages searches npm for packages matching a query.
// This uses the npm search API for broader results.
func SearchNpmPackages(ctx context.Context, query string, limit int) ([]Result, error) {
	u := fmt.Sprintf("%s/-/v1/search?text=%s&size=%d", npmRegistryURL, url.QueryEscape(query), limit)
	logx.Info("search npm: %s", u)

	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("npm search failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("npm", resp)
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
	if err := decodeJSON("npm", query, resp.Body, &data); err != nil {
		return nil, err
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
