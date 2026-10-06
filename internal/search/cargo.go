package search

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// CratesIOURL is the base URL for the crates.io API.
const CratesIOURL = "https://crates.io/api/v1"

// SearchCratesIO searches crates.io for crates matching a query.
func SearchCratesIO(query string, limit int) ([]Result, error) {
	url := fmt.Sprintf("%s/crates?q=%s&per_page=%d", CratesIOURL, url.QueryEscape(query), limit)
	logx.Info("search crates.io: %s", url)

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("crates.io search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("crates.io returned status %d", resp.StatusCode)
	}

	var data struct {
		Crates []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			MaxVersion  string `json:"max_version"`
			Downloads   int    `json:"downloads"`
		} `json:"crates"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode crates.io response: %w", err)
	}

	var results []Result
	for _, c := range data.Crates {
		results = append(results, Result{
			Manager: pm.Cargo,
			Name:    c.Name,
			Info:    c.Description,
			Extra: map[string]string{
				"version":   c.MaxVersion,
				"downloads": fmt.Sprintf("%d", c.Downloads),
			},
		})
	}

	return results, nil
}
