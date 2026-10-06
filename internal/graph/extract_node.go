package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// NodeExtractor extracts dependencies from Node.js lockfiles.
type NodeExtractor struct{}

func (e *NodeExtractor) Name() string {
	return "node"
}

func (e *NodeExtractor) Supports(file string) bool {
	return file == "package-lock.json" || file == "yarn.lock" ||
		file == "pnpm-lock.yaml" || file == "bun.lockb"
}

// Extract parses package-lock.json when present. yarn.lock and
// pnpm-lock.yaml still go through the legacy readers below until Task 4.
func (e *NodeExtractor) Extract(dir string, _ ExtractOptions) (*DepGraph, error) {
	data, err := os.ReadFile(filepath.Join(dir, "package-lock.json"))
	if err == nil {
		manifest, err := readOptional(filepath.Join(dir, "package.json"))
		if err != nil {
			return nil, err
		}
		return parseNpmLock(data, manifest, dirName(dir))
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	graph := NewGraph()

	// Try yarn.lock
	if err := e.extractYarnLock(dir, graph); err == nil {
		return graph, nil
	}

	// Try pnpm-lock.yaml
	if err := e.extractPnpmLock(dir, graph); err == nil {
		return graph, nil
	}

	// bun.lockb is binary, skip for now
	return graph, fmt.Errorf("no supported Node.js lockfile found")
}

func (e *NodeExtractor) extractYarnLock(dir string, graph *DepGraph) error {
	path := filepath.Join(dir, "yarn.lock")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Try to read package.json to identify root dependencies
	rootDeps := make(map[string]bool)
	packageJSONPath := filepath.Join(dir, "package.json")
	if pkgData, err := os.ReadFile(packageJSONPath); err == nil {
		var pkgJSON struct {
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		}
		if err := json.Unmarshal(pkgData, &pkgJSON); err == nil {
			// Collect all root dependencies
			for dep := range pkgJSON.Dependencies {
				rootDeps[dep] = true
			}
			for dep := range pkgJSON.DevDependencies {
				rootDeps[dep] = true
			}
		}
	}

	// Yarn lockfile is a custom format, parse line by line
	lines := strings.Split(string(data), "\n")
	var currentName, currentVersion string
	var currentDeps []string
	allPackages := make(map[string]*DepNode) // Track all packages by name
	dependencyOf := make(map[string]bool)    // Track which packages are dependencies

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, `"`) && strings.Contains(line, "@") {
			// Parse package name and version
			parts := strings.SplitN(line, `"`, 3)
			if len(parts) >= 2 {
				fullName := parts[1]
				if idx := strings.LastIndex(fullName, "@"); idx > 0 {
					currentName = fullName[:idx]
					currentVersion = fullName[idx+1:]
				}
			}
		} else if strings.HasPrefix(line, "dependencies:") {
			// Start collecting dependencies
			currentDeps = []string{}
		} else if strings.HasPrefix(line, "  ") && currentDeps != nil {
			// Dependency line
			depLine := strings.TrimSpace(line)
			if idx := strings.Index(depLine, " "); idx > 0 {
				depName := depLine[:idx]
				currentDeps = append(currentDeps, depName)
			}
		} else if line == "" && currentName != "" {
			// End of package entry
			node := NewDepNode("node", currentName, currentVersion)
			graph.AddNode(node)
			allPackages[currentName] = node

			// Mark dependencies
			for _, depName := range currentDeps {
				dependencyOf[depName] = true
				// Find dependency node
				depNode := graph.FindNodeByName(depName)
				if len(depNode) > 0 {
					graph.AddEdge(NewEdge(node.ID, depNode[0].ID))
				}
			}

			currentName = ""
			currentVersion = ""
			currentDeps = nil
		}
	}

	// Identify root packages: those in package.json dependencies or not dependencies of anything
	for name, node := range allPackages {
		if rootDeps[name] || !dependencyOf[name] {
			graph.AddRoot(node.ID)
		}
	}

	return nil
}

func (e *NodeExtractor) extractPnpmLock(dir string, graph *DepGraph) error {
	path := filepath.Join(dir, "pnpm-lock.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var lockfile struct {
		Packages map[string]interface{} `yaml:"packages"`
	}

	if err := yaml.Unmarshal(data, &lockfile); err != nil {
		return err
	}

	for pkgPath, pkgData := range lockfile.Packages {
		pkgMap, ok := pkgData.(map[string]interface{})
		if !ok {
			continue
		}

		// Extract name and version from path or data
		parts := strings.Split(pkgPath, "/")
		if len(parts) < 2 {
			continue
		}

		name := parts[0]
		version := parts[1]

		node := NewDepNode("node", name, version)
		graph.AddNode(node)

		// Extract dependencies
		if deps, ok := pkgMap["dependencies"].(map[string]interface{}); ok {
			for depName := range deps {
				depNode := graph.FindNodeByName(depName)
				if len(depNode) > 0 {
					graph.AddEdge(NewEdge(node.ID, depNode[0].ID))
				}
			}
		}
	}

	return nil
}
