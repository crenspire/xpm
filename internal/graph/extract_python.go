package graph

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// PythonExtractor extracts dependencies from Python lockfiles.
type PythonExtractor struct{}

func (e *PythonExtractor) Name() string {
	return "python"
}

func (e *PythonExtractor) Supports(file string) bool {
	return file == "requirements.txt" || file == "requirements.lock" ||
		file == "pyproject.toml" || file == "poetry.lock"
}

func (e *PythonExtractor) Extract(dir string) (*DepGraph, error) {
	graph := NewGraph()

	// Try poetry.lock first
	if err := e.extractPoetryLock(dir, graph); err == nil {
		return graph, nil
	}

	// Try pyproject.toml
	if err := e.extractPyproject(dir, graph); err == nil {
		return graph, nil
	}

	// Try requirements.txt
	if err := e.extractRequirements(dir, graph); err == nil {
		return graph, nil
	}

	return graph, fmt.Errorf("no supported Python dependency file found")
}

func (e *PythonExtractor) extractPoetryLock(dir string, graph *DepGraph) error {
	path := filepath.Join(dir, "poetry.lock")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var lockfile struct {
		Package []struct {
			Name         string   `toml:"name"`
			Version      string   `toml:"version"`
			Dependencies map[string]interface{} `toml:"dependencies"`
		} `toml:"package"`
	}

	if _, err := toml.Decode(string(data), &lockfile); err != nil {
		return err
	}

	for _, pkg := range lockfile.Package {
		node := NewDepNode("python", pkg.Name, pkg.Version)
		graph.AddNode(node)

		// Mark first package as root
		if len(graph.Root) == 0 {
			graph.AddRoot(node.ID)
		}

		// Extract dependencies
		for depName := range pkg.Dependencies {
			depNode := graph.FindNodeByName(depName)
			if len(depNode) > 0 {
				graph.AddEdge(NewEdge(node.ID, depNode[0].ID))
			}
		}
	}

	return nil
}

func (e *PythonExtractor) extractPyproject(dir string, graph *DepGraph) error {
	path := filepath.Join(dir, "pyproject.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var project struct {
		Project struct {
			Name         string   `toml:"name"`
			Version      string   `toml:"version"`
			Dependencies []string `toml:"dependencies"`
		} `toml:"project"`
	}

	if _, err := toml.Decode(string(data), &project); err != nil {
		return err
	}

	// Add root package
	if project.Project.Name != "" {
		rootNode := NewDepNode("python", project.Project.Name, project.Project.Version)
		graph.AddNode(rootNode)
		graph.AddRoot(rootNode.ID)

		// Parse dependencies
		for _, depSpec := range project.Project.Dependencies {
			depName, depVersion := e.parseDependencySpec(depSpec)
			if depName != "" {
				depNode := NewDepNode("python", depName, depVersion)
				graph.AddNode(depNode)
				graph.AddEdge(NewEdge(rootNode.ID, depNode.ID))
			}
		}
	}

	return nil
}

func (e *PythonExtractor) extractRequirements(dir string, graph *DepGraph) error {
	path := filepath.Join(dir, "requirements.txt")
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Try to find project name from setup.py or pyproject.toml
	projectName := "project"
	if data, err := os.ReadFile(filepath.Join(dir, "pyproject.toml")); err == nil {
		var proj struct {
			Project struct {
				Name string `toml:"name"`
			} `toml:"project"`
		}
		if _, err := toml.Decode(string(data), &proj); err == nil && proj.Project.Name != "" {
			projectName = proj.Project.Name
		}
	}

	rootNode := NewDepNode("python", projectName, "")
	graph.AddNode(rootNode)
	graph.AddRoot(rootNode.ID)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		depName, depVersion := e.parseDependencySpec(line)
		if depName != "" {
			depNode := NewDepNode("python", depName, depVersion)
			graph.AddNode(depNode)
			graph.AddEdge(NewEdge(rootNode.ID, depNode.ID))
		}
	}

	return nil
}

func (e *PythonExtractor) parseDependencySpec(spec string) (name, version string) {
	// Parse formats like: package==1.0.0, package>=1.0.0, package~=1.0.0
	parts := regexp.MustCompile(`([^=<>!~]+)([=<>!~]+)(.+)`).FindStringSubmatch(spec)
	if len(parts) >= 4 {
		return strings.TrimSpace(parts[1]), strings.TrimSpace(parts[3])
	}

	// Just package name
	return strings.TrimSpace(spec), ""
}

