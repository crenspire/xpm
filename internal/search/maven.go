package search

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/crenspire/xpm/internal/logx"
	"github.com/crenspire/xpm/internal/pm"
)

// MavenSearchURL is the URL for Maven Central search API.
const MavenSearchURL = "https://central.sonatype.com/solrsearch/select"

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

// SearchMavenCentral searches Maven Central for up to limit artifacts.
func SearchMavenCentral(ctx context.Context, query string, limit int) ([]Result, error) {
	u := fmt.Sprintf("%s?q=%s&rows=%d&wt=json", mavenSearchURL, url.QueryEscape(query), limit)
	logx.Info("search maven: %s", u)

	resp, err := httpGet(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("maven search failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("maven", resp)
	}

	var data mavenSearchResponse
	if err := decodeJSON("maven", query, resp.Body, &data); err != nil {
		return nil, err
	}

	var results []Result
	for _, doc := range data.Response.Docs {
		results = append(results, Result{
			Manager: pm.Maven,
			Name:    doc.Group + ":" + doc.Artifact,
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
