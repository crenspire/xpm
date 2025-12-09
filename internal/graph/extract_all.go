package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ExtractAll runs all extractors in parallel and merges the results.
func ExtractAll(dir string) (*DepGraph, error) {
	extractors := []Extractor{
		&NodeExtractor{},
		&PythonExtractor{},
		&ComposerExtractor{},
		&CargoExtractor{},
		&GoExtractor{},
		&JavaExtractor{},
	}

	// Detect which ecosystems are present
	detected := DetectEcosystems(dir)

	// Run extractors in parallel
	type result struct {
		graph *DepGraph
		err   error
		name  string
	}

	results := make(chan result, len(extractors))
	var wg sync.WaitGroup

	for _, extractor := range extractors {
		// Only run extractor if ecosystem is detected
		if !isEcosystemDetected(extractor, detected) {
			continue
		}

		wg.Add(1)
		go func(ext Extractor) {
			defer wg.Done()
			graph, err := ext.Extract(dir)
			results <- result{graph: graph, err: err, name: ext.Name()}
		}(extractor)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Merge all graphs
	merged := NewGraph()
	var errors []error

	for res := range results {
		if res.err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", res.name, res.err))
			continue
		}
		if res.graph != nil {
			merged.Merge(res.graph)
		}
	}

	if len(merged.Nodes) == 0 && len(errors) > 0 {
		return nil, fmt.Errorf("failed to extract dependencies: %v", errors)
	}

	return merged, nil
}

// isEcosystemDetected checks if an extractor's ecosystem is detected.
func isEcosystemDetected(extractor Extractor, detected map[string]bool) bool {
	name := extractor.Name()
	return detected[name]
}

// DetectEcosystems detects which ecosystems are present in the directory.
func DetectEcosystems(dir string) map[string]bool {
	detected := make(map[string]bool)

	// Check for lockfiles and dependency files
	files := []struct {
		file       string
		ecosystem  string
	}{
		{"package-lock.json", "node"},
		{"yarn.lock", "node"},
		{"pnpm-lock.yaml", "node"},
		{"bun.lockb", "node"},
		{"poetry.lock", "python"},
		{"requirements.txt", "python"},
		{"pyproject.toml", "python"},
		{"composer.lock", "php"},
		{"Cargo.lock", "rust"},
		{"go.mod", "go"},
		{"pom.xml", "java"},
		{"build.gradle", "java"},
		{"build.gradle.kts", "java"},
	}

	for _, f := range files {
		path := filepath.Join(dir, f.file)
		if _, err := os.Stat(path); err == nil {
			detected[f.ecosystem] = true
		}
	}

	return detected
}

// ExtractForPackage extracts dependencies for a specific package.
func ExtractForPackage(dir string, packageName string) (*DepGraph, error) {
	graph, err := ExtractAll(dir)
	if err != nil {
		return nil, err
	}

	// Find the package node
	nodes := graph.FindNodeByName(packageName)
	if len(nodes) == 0 {
		return nil, fmt.Errorf("package %s not found", packageName)
	}

	// Create a subgraph starting from this package
	subgraph := NewGraph()
	targetNode := nodes[0]
	subgraph.AddNode(targetNode)
	subgraph.AddRoot(targetNode.ID)

	// Get all transitive dependencies
	transitive := graph.GetTransitive(targetNode.ID)
	for _, depID := range transitive {
		if depNode := graph.GetNode(depID); depNode != nil {
			subgraph.AddNode(depNode)
		}
	}

	// Add edges for the subgraph
	for _, edge := range graph.Edges {
		if edge.From == targetNode.ID || contains(transitive, edge.From) {
			if contains(transitive, edge.To) || edge.To == targetNode.ID {
				subgraph.AddEdge(edge)
			}
		}
	}

	return subgraph, nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

