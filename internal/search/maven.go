package search

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// MavenSearchURL is the URL for Maven Central search API.
const MavenSearchURL = "https://search.maven.org/solrsearch/select"

// mavenSearchResponse represents the Maven Central search response.
type mavenSearchResponse struct {
	Response struct {
		NumFound int `json:"numFound"`
		Docs     []struct {
			ID            string `json:"id"`
			Group         string `json:"g"`
			Artifact      string `json:"a"`
			LatestVersion string `json:"latestVersion"`
			RepositoryID  string `json:"repositoryId"`
			Timestamp     int64  `json:"timestamp"`
		} `json:"docs"`
	} `json:"response"`
}

// searchMaven searches for an artifact in Maven Central.
// Returns the first matching result or nil if no matches found.
func searchMaven(pkg string) (*Result, error) {
	url := fmt.Sprintf("%s?q=%s&rows=5&wt=json", MavenSearchURL, url.QueryEscape(pkg))
	logx.Info("query maven: %s", url)

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("maven request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("maven returned status %d", resp.StatusCode)
	}

	var data mavenSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode maven response: %w", err)
	}

	if len(data.Response.Docs) == 0 {
		return nil, nil // No results
	}

	doc := data.Response.Docs[0]
	coordinate := fmt.Sprintf("%s:%s", doc.Group, doc.Artifact)

	return &Result{
		Manager: pm.Maven,
		Name:    coordinate,
		Info:    "Maven artifact",
		Extra: map[string]string{
			"version":  doc.LatestVersion,
			"id":       doc.ID,
			"group":    doc.Group,
			"artifact": doc.Artifact,
		},
	}, nil
}

// SearchMavenCentral searches Maven Central for artifacts.
func SearchMavenCentral(query string, limit int) ([]Result, error) {
	url := fmt.Sprintf("%s?q=%s&rows=%d&wt=json", MavenSearchURL, url.QueryEscape(query), limit)
	logx.Info("search maven: %s", url)

	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("maven search failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("maven returned status %d", resp.StatusCode)
	}

	var data mavenSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode maven response: %w", err)
	}

	var results []Result
	for _, doc := range data.Response.Docs {
		coordinate := fmt.Sprintf("%s:%s", doc.Group, doc.Artifact)
		results = append(results, Result{
			Manager: pm.Maven,
			Name:    coordinate,
			Info:    "Maven artifact",
			Extra: map[string]string{
				"version":  doc.LatestVersion,
				"id":       doc.ID,
				"group":    doc.Group,
				"artifact": doc.Artifact,
			},
		})
	}

	return results, nil
}

// GetMavenDependencyXML returns the Maven dependency XML snippet for a result.
func GetMavenDependencyXML(r Result) string {
	group := r.Extra["group"]
	artifact := r.Extra["artifact"]
	version := r.Extra["version"]

	if version == "" {
		version = "LATEST"
	}

	return fmt.Sprintf(`<dependency>
    <groupId>%s</groupId>
    <artifactId>%s</artifactId>
    <version>%s</version>
</dependency>`, group, artifact, version)
}

// GetGradleDependency returns the Gradle dependency string for a result.
func GetGradleDependency(r Result) string {
	group := r.Extra["group"]
	artifact := r.Extra["artifact"]
	version := r.Extra["version"]

	if version == "" {
		version = "+"
	}

	return fmt.Sprintf("implementation '%s:%s:%s'", group, artifact, version)
}

