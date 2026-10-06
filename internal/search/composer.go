package search

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// PackagistURL is the base URL for the Packagist API.
// Note: packagist.org hosts the public API endpoints (search.json, packages/list.json).
const PackagistURL = "https://packagist.org"

// packagistSearchResponse represents the Packagist search API response.
type packagistSearchResponse struct {
	Results []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		URL         string `json:"url"`
		Repository  string `json:"repository"`
		Downloads   int    `json:"downloads"`
		Favers      int    `json:"favers"`
	} `json:"results"`
	Total int `json:"total"`
}

// SearchPackagistPackages searches Packagist for multiple results.
// Packagist API: https://packagist.org/apidoc#search-packages
// The search endpoint is /search.json with query parameter 'q'
// Note: Packagist doesn't support per_page parameter, it returns all results
func SearchPackagistPackages(query string, limit int) ([]Result, error) {
	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	urlStr := fmt.Sprintf("%s/search.json?q=%s", PackagistURL, url.QueryEscape(query))
	logx.Info("search packagist: %s", urlStr)

	resp, err := httpClient.Get(urlStr)
	if err != nil {
		logx.Info("packagist request error: %v", err)
		return nil, fmt.Errorf("packagist search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		// Read error body for debugging
		body, _ := io.ReadAll(resp.Body)
		logx.Info("packagist returned status %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("packagist returned status %d: %s", resp.StatusCode, string(body))
	}

	var data packagistSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		logx.Info("packagist decode error: %v", err)
		return nil, fmt.Errorf("failed to decode packagist response: %w", err)
	}

	logx.Info("packagist search returned %d total results for query %q", data.Total, query)

	var results []Result
	for i, r := range data.Results {
		// Limit results
		if i >= limit {
			break
		}
		results = append(results, Result{
			Manager: pm.Composer,
			Name:    r.Name,
			Info:    r.Description,
			Extra: map[string]string{
				"url":       r.URL,
				"downloads": fmt.Sprintf("%d", r.Downloads),
			},
		})
	}

	logx.Info("packagist search returning %d results (limited from %d)", len(results), len(data.Results))
	return results, nil
}
