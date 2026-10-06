package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	path := filepath.Join(dir, "composer.lock")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lockfile struct {
		Packages    []PackageInfo `json:"packages"`
		PackagesDev []PackageInfo `json:"packages-dev"`
	}

	if err := json.Unmarshal(data, &lockfile); err != nil {
		return nil, err
	}

	graph := NewGraph()

	// Add root package (from composer.json if available)
	if rootName, rootVersion := e.getRootPackage(dir); rootName != "" {
		rootNode := NewDepNode("php", rootName, rootVersion)
		graph.AddNode(rootNode)
		graph.AddRoot(rootNode.ID)
	}

	// Process packages
	allPackages := append(lockfile.Packages, lockfile.PackagesDev...)
	nodeMap := make(map[string]*DepNode)

	for _, pkg := range allPackages {
		node := NewDepNode("php", pkg.Name, pkg.Version)
		if pkg.Source != nil {
			node.WithMetadata("source", pkg.Source.URL)
		}
		graph.AddNode(node)
		nodeMap[pkg.Name] = node
	}

	// Add edges for dependencies
	for _, pkg := range allPackages {
		if node, ok := nodeMap[pkg.Name]; ok {
			for depName := range pkg.Require {
				if depNode, ok := nodeMap[depName]; ok {
					graph.AddEdge(NewEdge(node.ID, depNode.ID))
				}
			}
		}
	}

	return graph, nil
}

func (e *ComposerExtractor) getRootPackage(dir string) (name, version string) {
	path := filepath.Join(dir, "composer.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}

	var composer struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	if err := json.Unmarshal(data, &composer); err != nil {
		return "", ""
	}

	return composer.Name, composer.Version
}

// PackageInfo represents a Composer package.
type PackageInfo struct {
	Name    string            `json:"name"`
	Version string            `json:"version"`
	Require map[string]string `json:"require"`
	Source  *struct {
		URL string `json:"url"`
	} `json:"source"`
}
