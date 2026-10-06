package search

import (
	"context"
	"fmt"
	"net/http"
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

// SearchPackagistPackages searches Packagist for up to limit results.
// Packagist API: https://packagist.org/apidoc#search-packages
func SearchPackagistPackages(ctx context.Context, query string, limit int) ([]Result, error) {
	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	u := fmt.Sprintf("%s/search.json?q=%s", packagistURL, url.QueryEscape(query))
	logx.Info("search packagist: %s", u)

	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("packagist search failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("packagist", resp)
	}

	var data packagistSearchResponse
	if err := decodeJSON("packagist", query, resp.Body, &data); err != nil {
		return nil, err
	}

	var results []Result
	for i, r := range data.Results {
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
	return results, nil
}
