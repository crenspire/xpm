package graph

import (
	"encoding/json"
	"fmt"
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

func (e *NodeExtractor) Extract(dir string) (*DepGraph, error) {
	graph := NewGraph()

	// Try package-lock.json first
	if err := e.extractPackageLock(dir, graph); err == nil {
		return graph, nil
	}

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

func (e *NodeExtractor) extractPackageLock(dir string, graph *DepGraph) error {
	path := filepath.Join(dir, "package-lock.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var lockfile struct {
		Name         string `json:"name"`
		Version      string `json:"version"`
		Packages     map[string]interface{} `json:"packages"`
		Dependencies map[string]interface{} `json:"dependencies"`
	}

	if err := json.Unmarshal(data, &lockfile); err != nil {
		return err
	}

	// Add root package
	if lockfile.Name != "" {
		rootNode := NewDepNode("node", lockfile.Name, lockfile.Version)
		graph.AddNode(rootNode)
		graph.AddRoot(rootNode.ID)
	}

	// Extract from packages (npm v7+ format)
	if lockfile.Packages != nil {
		for pkgPath, pkgData := range lockfile.Packages {
			if pkgPath == "" {
				continue // Skip root
			}

			pkgMap, ok := pkgData.(map[string]interface{})
			if !ok {
				continue
			}

			name, _ := pkgMap["name"].(string)
			version, _ := pkgMap["version"].(string)
			if name == "" || version == "" {
				continue
			}

			node := NewDepNode("node", name, version)
			if resolved, ok := pkgMap["resolved"].(string); ok {
				node.WithMetadata("resolved", resolved)
			}
			if integrity, ok := pkgMap["integrity"].(string); ok {
				node.WithMetadata("integrity", integrity)
			}
			graph.AddNode(node)

			// Extract dependencies
			if deps, ok := pkgMap["dependencies"].(map[string]interface{}); ok {
				for depName := range deps {
					// Validate depName to prevent path traversal
					if depName == "" || strings.Contains(depName, "..") {
						continue
					}
					// Validate pkgPath before using it
					cleanPkgPath := filepath.Clean(pkgPath)
					if strings.Contains(cleanPkgPath, "..") {
						continue
					}
					// Find dependency node
					depPath := filepath.Join(cleanPkgPath, "node_modules", depName)
					// Validate the constructed path doesn't escape
					if !strings.HasPrefix(depPath, cleanPkgPath) {
						continue
					}
					if depNode := e.findNodeInPackages(lockfile.Packages, depPath); depNode != nil {
						graph.AddEdge(NewEdge(node.ID, depNode.ID))
					}
				}
			}
		}
	}

	// Fallback to dependencies (npm v6 format)
	if lockfile.Dependencies != nil {
		e.extractDependenciesRecursive(lockfile.Dependencies, "", graph)
	}

	return nil
}

func (e *NodeExtractor) findNodeInPackages(packages map[string]interface{}, path string) *DepNode {
	if pkgData, ok := packages[path]; ok {
		pkgMap, ok := pkgData.(map[string]interface{})
		if !ok {
			return nil
		}
		name, _ := pkgMap["name"].(string)
		version, _ := pkgMap["version"].(string)
		if name != "" && version != "" {
			return NewDepNode("node", name, version)
		}
	}
	return nil
}

func (e *NodeExtractor) extractDependenciesRecursive(deps map[string]interface{}, parentID string, graph *DepGraph) {
	for name, depData := range deps {
		depMap, ok := depData.(map[string]interface{})
		if !ok {
			continue
		}

		version, _ := depMap["version"].(string)
		if version == "" {
			continue
		}

		node := NewDepNode("node", name, version)
		if resolved, ok := depMap["resolved"].(string); ok {
			node.WithMetadata("resolved", resolved)
		}
		graph.AddNode(node)

		if parentID != "" {
			graph.AddEdge(NewEdge(parentID, node.ID))
		}

		// Recursively extract nested dependencies
		if nestedDeps, ok := depMap["dependencies"].(map[string]interface{}); ok {
			e.extractDependenciesRecursive(nestedDeps, node.ID, graph)
		}
	}
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

