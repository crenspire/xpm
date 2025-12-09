package search

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// PyPIURL is the base URL for the PyPI JSON API.
const PyPIURL = "https://pypi.org/pypi"

// pypiPackageResponse represents the PyPI API response.
type pypiPackageResponse struct {
	Info struct {
		Name            string `json:"name"`
		Summary         string `json:"summary"`
		Version         string `json:"version"`
		License         string `json:"license"`
		Author          string `json:"author"`
		AuthorEmail     string `json:"author_email"`
		HomePage        string `json:"home_page"`
		ProjectURL      string `json:"project_url"`
		RequiresPython  string `json:"requires_python"`
		PackageURL      string `json:"package_url"`
	} `json:"info"`
}

// searchPip searches for a package in the Python Package Index (PyPI).
// Returns nil if the package is not found.
func searchPip(pkg string) (*Result, error) {
	url := fmt.Sprintf("%s/%s/json", PyPIURL, url.PathEscape(pkg))
	logx.Info("query pypi: %s", url)

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("pypi request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil // Package not found
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("pypi returned status %d", resp.StatusCode)
	}

	var data pypiPackageResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode pypi response: %w", err)
	}

	result := &Result{
		Manager: pm.Pip,
		Name:    data.Info.Name,
		Info:    data.Info.Summary,
		Extra: map[string]string{
			"version": data.Info.Version,
		},
	}

	if data.Info.License != "" {
		result.Extra["license"] = data.Info.License
	}

	if data.Info.Author != "" {
		result.Extra["author"] = data.Info.Author
	}

	if data.Info.HomePage != "" {
		result.Extra["homepage"] = data.Info.HomePage
	}

	if data.Info.RequiresPython != "" {
		result.Extra["python_requires"] = data.Info.RequiresPython
	}

	return result, nil
}

// SearchPyPIPackages searches PyPI for packages.
// Note: PyPI deprecated the search API, so this is a simple lookup.
func SearchPyPIPackages(query string) (*Result, error) {
	return searchPip(query)
}

