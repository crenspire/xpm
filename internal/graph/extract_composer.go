package graph

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ComposerExtractor extracts dependencies from PHP Composer lockfiles.
type ComposerExtractor struct{}

func (e *ComposerExtractor) Name() string {
	return "php"
}

func (e *ComposerExtractor) Supports(file string) bool {
	return file == "composer.lock"
}

func (e *ComposerExtractor) Extract(dir string, _ ExtractOptions) (*DepGraph, error) {
	data, err := os.ReadFile(filepath.Join(dir, "composer.lock"))
	if err != nil {
		return nil, err
	}
	manifest, err := readOptional(filepath.Join(dir, "composer.json"))
	if err != nil {
		return nil, err
	}
	return parseComposerLock(data, manifest, dirName(dir))
}

type composerPackage struct {
	Name       string            `json:"name"`
	Version    string            `json:"version"`
	Require    map[string]string `json:"require"`
	RequireDev map[string]string `json:"require-dev"`
	Source     *struct {
		URL string `json:"url"`
	} `json:"source"`
}

// composerPlatform reports platform requirements (php, ext-*, lib-*,
// composer-plugin-api, ...): every real package name is "vendor/name".
func composerPlatform(name string) bool {
	return !strings.Contains(name, "/")
}

// parseComposerLock builds the graph of a composer.lock ("packages" and
// "packages-dev", edges from each "require"). Names match case-insensitively
// and platform requirements are skipped. The root is the project from
// composer.json (manifest, else fallback) with edges to its require and
// require-dev packages.
func parseComposerLock(data, manifest []byte, fallback string) (*DepGraph, error) {
	var lock struct {
		Packages    []composerPackage `json:"packages"`
		PackagesDev []composerPackage `json:"packages-dev"`
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("composer.lock: %w", err)
	}
	var root composerPackage
	if len(manifest) > 0 {
		if err := json.Unmarshal(manifest, &root); err != nil {
			return nil, fmt.Errorf("composer.json: %w", err)
		}
	}
	g := NewGraph()
	byName := map[string]string{} // lower-case name -> node ID
	all := append(append([]composerPackage{}, lock.Packages...), lock.PackagesDev...)
	for _, p := range all {
		if p.Name == "" || p.Version == "" {
			return nil, fmt.Errorf("composer.lock: package without name or version")
		}
		n := NewDepNode("php", p.Name, p.Version)
		if p.Source != nil && p.Source.URL != "" {
			n.WithMetadata("source", p.Source.URL)
		}
		g.AddNode(n)
		byName[strings.ToLower(p.Name)] = n.ID
	}
	link := func(from string, req map[string]string) {
		for _, kv := range sortedPairs(req) {
			if composerPlatform(kv[0]) {
				continue
			}
			if to, ok := byName[strings.ToLower(kv[0])]; ok {
				g.AddEdge(NewEdge(from, to))
			}
		}
	}
	for _, p := range all {
		link(NodeID("php", p.Name, p.Version), p.Require)
	}
	project := addProject(g, "php", root.Name, root.Version, fallback)
	link(project, root.Require)
	link(project, root.RequireDev)
	return g, nil
}
