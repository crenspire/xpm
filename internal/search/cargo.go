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

// cratesIOResponse represents the crates.io API response for a crate.
type cratesIOResponse struct {
	Crate struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		MaxVersion    string `json:"max_version"`
		Homepage      string `json:"homepage"`
		Repository    string `json:"repository"`
		Documentation string `json:"documentation"`
		Downloads     int    `json:"downloads"`
	} `json:"crate"`
}

// searchCargo checks if a crate exists in crates.io (Rust registry).
// Returns nil if the crate is not found.
func searchCargo(pkg string) (*Result, error) {
	url := fmt.Sprintf("%s/crates/%s", CratesIOURL, url.PathEscape(pkg))
	logx.Info("query crates.io: %s", url)

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("crates.io request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil // Crate not found
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("crates.io returned status %d", resp.StatusCode)
	}

	var data cratesIOResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode crates.io response: %w", err)
	}

	result := &Result{
		Manager: pm.Cargo,
		Name:    data.Crate.Name,
		Info:    data.Crate.Description,
		Extra: map[string]string{
			"version":   data.Crate.MaxVersion,
			"downloads": fmt.Sprintf("%d", data.Crate.Downloads),
		},
	}

	if data.Crate.Homepage != "" {
		result.Extra["homepage"] = data.Crate.Homepage
	}

	if data.Crate.Repository != "" {
		result.Extra["repository"] = data.Crate.Repository
	}

	if data.Crate.Documentation != "" {
		result.Extra["docs"] = data.Crate.Documentation
	}

	return result, nil
}

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
