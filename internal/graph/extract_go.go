package graph

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GoExtractor extracts dependencies from Go modules.
type GoExtractor struct{}

func (e *GoExtractor) Name() string {
	return "go"
}

func (e *GoExtractor) Supports(file string) bool {
	return file == "go.mod" || file == "go.sum"
}

func (e *GoExtractor) Extract(dir string, _ ExtractOptions) (*DepGraph, error) {
	// Try using go list -m all for complete dependency tree
	if graph := e.extractGoList(dir); graph != nil {
		return graph, nil
	}

	// Fallback to parsing go.mod
	return e.extractGoMod(dir)
}

func (e *GoExtractor) extractGoList(dir string) *DepGraph {
	cmd := exec.Command("go", "list", "-m", "all")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	graph := NewGraph()
	lines := strings.Split(string(output), "\n")
	var rootNode *DepNode

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse module@version
		parts := strings.Split(line, " ")
		if len(parts) == 0 {
			continue
		}

		modulePath := parts[0]
		version := ""
		if len(parts) > 1 {
			version = parts[1]
		}

		// Extract module name from path
		moduleName := modulePath
		if idx := strings.LastIndex(modulePath, "/"); idx > 0 {
			moduleName = modulePath[idx+1:]
		}

		node := NewDepNode("go", moduleName, version)
		node.WithMetadata("path", modulePath)
		graph.AddNode(node)

		// First module is typically the root
		if i == 0 {
			rootNode = node
			graph.AddRoot(node.ID)
		} else if rootNode != nil {
			// Add edge from root to dependency
			graph.AddEdge(NewEdge(rootNode.ID, node.ID))
		}
	}

	return graph
}

func (e *GoExtractor) extractGoMod(dir string) (*DepGraph, error) {
	path := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	graph := NewGraph()
	lines := strings.Split(string(data), "\n")
	var rootModule string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			rootModule = strings.TrimSpace(strings.TrimPrefix(line, "module"))
			rootNode := NewDepNode("go", e.moduleNameFromPath(rootModule), "")
			rootNode.WithMetadata("path", rootModule)
			graph.AddNode(rootNode)
			graph.AddRoot(rootNode.ID)
		} else if strings.HasPrefix(line, "require ") {
			// Parse require line: require module/path v1.2.3
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				modulePath := parts[1]
				version := parts[2]
				moduleName := e.moduleNameFromPath(modulePath)

				depNode := NewDepNode("go", moduleName, version)
				depNode.WithMetadata("path", modulePath)
				graph.AddNode(depNode)

				if len(graph.Root) > 0 {
					graph.AddEdge(NewEdge(graph.Root[0], depNode.ID))
				}
			}
		}
	}

	if len(graph.Nodes) == 0 {
		return nil, fmt.Errorf("no dependencies found in go.mod")
	}

	return graph, nil
}

func (e *GoExtractor) moduleNameFromPath(path string) string {
	if idx := strings.LastIndex(path, "/"); idx > 0 {
		return path[idx+1:]
	}
	return path
}
