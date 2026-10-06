package graph

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// CargoExtractor extracts dependencies from Rust Cargo lockfiles.
type CargoExtractor struct{}

func (e *CargoExtractor) Name() string {
	return "rust"
}

func (e *CargoExtractor) Supports(file string) bool {
	return file == "Cargo.lock"
}

func (e *CargoExtractor) Extract(dir string) (*DepGraph, error) {
	path := filepath.Join(dir, "Cargo.lock")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lockfile struct {
		Package []struct {
			Name         string   `toml:"name"`
			Version      string   `toml:"version"`
			Dependencies []string `toml:"dependencies"`
		} `toml:"package"`
	}

	if _, err := toml.Decode(string(data), &lockfile); err != nil {
		return nil, err
	}

	graph := NewGraph()
	nodeMap := make(map[string]*DepNode)

	// First pass: create all nodes
	for _, pkg := range lockfile.Package {
		node := NewDepNode("rust", pkg.Name, pkg.Version)
		graph.AddNode(node)
		nodeMap[pkg.Name] = node

		// Mark first package as root
		if len(graph.Root) == 0 {
			graph.AddRoot(node.ID)
		}
	}

	// Second pass: create edges
	for _, pkg := range lockfile.Package {
		if node, ok := nodeMap[pkg.Name]; ok {
			for _, depSpec := range pkg.Dependencies {
				depName := e.parseDependencySpec(depSpec)
				if depNode, ok := nodeMap[depName]; ok {
					graph.AddEdge(NewEdge(node.ID, depNode.ID))
				}
			}
		}
	}

	return graph, nil
}

func (e *CargoExtractor) parseDependencySpec(spec string) string {
	// Format: "name version (source)" or just "name version"
	parts := strings.Fields(spec)
	if len(parts) > 0 {
		return parts[0]
	}
	return spec
}
