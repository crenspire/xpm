package search

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// CratesIOURL is the base URL for the crates.io API.
const CratesIOURL = "https://crates.io/api/v1"

// SearchCratesIO searches crates.io for crates matching a query.
func SearchCratesIO(ctx context.Context, query string, limit int) ([]Result, error) {
	u := fmt.Sprintf("%s/crates?q=%s&per_page=%d", cratesAPIURL, url.QueryEscape(query), limit)
	logx.Info("search crates.io: %s", u)

	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("crates.io search failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("crates.io", resp)
	}

	var data struct {
		Crates []struct {
			Name           string `json:"name"`
			Description    string `json:"description"`
			MaxVersion     string `json:"max_version"`
			DefaultVersion string `json:"default_version"`
			Downloads      int    `json:"downloads"`
		} `json:"crates"`
	}
	if err := decodeJSON("crates.io", query, resp.Body, &data); err != nil {
		return nil, err
	}

	var results []Result
	for _, c := range data.Crates {
		version := c.DefaultVersion
		if version == "" {
			version = c.MaxVersion
		}
		results = append(results, Result{
			Manager: pm.Cargo,
			Name:    c.Name,
			Info:    c.Description,
			Extra: map[string]string{
				"version":   version,
				"downloads": fmt.Sprintf("%d", c.Downloads),
			},
		})
	}
	return results, nil
}
