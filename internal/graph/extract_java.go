package graph

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// JavaExtractor extracts dependencies from Java projects (Maven/Gradle).
type JavaExtractor struct{}

func (e *JavaExtractor) Name() string {
	return "java"
}

func (e *JavaExtractor) Supports(file string) bool {
	return file == "pom.xml" || file == "build.gradle" || file == "build.gradle.kts"
}

func (e *JavaExtractor) Extract(dir string, _ ExtractOptions) (*DepGraph, error) {
	// Try Maven first
	if graph, err := e.extractMaven(dir); err == nil && graph != nil {
		return graph, nil
	}

	// Try Gradle
	if graph, err := e.extractGradle(dir); err == nil && graph != nil {
		return graph, nil
	}

	return nil, fmt.Errorf("no supported Java dependency file found")
}

func (e *JavaExtractor) extractMaven(dir string) (*DepGraph, error) {
	pomPath := filepath.Join(dir, "pom.xml")
	if _, err := os.Stat(pomPath); err != nil {
		return nil, err
	}

	// Run mvn dependency:tree
	cmd := exec.Command("mvn", "dependency:tree", "-DoutputType=tgf")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		// Fallback to parsing pom.xml directly
		return e.extractPomXML(pomPath)
	}

	return e.parseTGF(string(output))
}

func (e *JavaExtractor) extractPomXML(path string) (*DepGraph, error) {
	// Simple XML parsing for pom.xml (basic implementation)
	// In production, use proper XML parser
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	graph := NewGraph()
	content := string(data)

	// Extract groupId, artifactId, version
	groupId := e.extractXMLTag(content, "groupId")
	artifactId := e.extractXMLTag(content, "artifactId")
	version := e.extractXMLTag(content, "version")

	if groupId == "" {
		groupId = e.extractXMLTag(content, "parent", "groupId")
	}
	if version == "" {
		version = e.extractXMLTag(content, "parent", "version")
	}

	if artifactId != "" {
		rootNode := NewDepNode("java", groupId+":"+artifactId, version)
		graph.AddNode(rootNode)
		graph.AddRoot(rootNode.ID)
	}

	return graph, nil
}

func (e *JavaExtractor) extractXMLTag(content, tag string, parents ...string) string {
	// Simple regex-based extraction (basic implementation)
	pattern := fmt.Sprintf(`<%s>([^<]+)</%s>`, tag, tag)
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(content)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func (e *JavaExtractor) parseTGF(tgf string) (*DepGraph, error) {
	graph := NewGraph()
	lines := strings.Split(tgf, "\n")
	nodeMap := make(map[string]*DepNode)
	inNodes := true

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if line == "#" {
			inNodes = false
			continue
		}

		if inNodes {
			// Node format: ID label
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				id := parts[0]
				label := strings.Join(parts[1:], " ")
				// Parse groupId:artifactId:version from label
				node := e.parseMavenLabel(label)
				if node != nil {
					graph.AddNode(node)
					nodeMap[id] = node
					if len(graph.Root) == 0 {
						graph.AddRoot(node.ID)
					}
				}
			}
		} else {
			// Edge format: FROM TO
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				fromID := parts[0]
				toID := parts[1]
				if fromNode, ok := nodeMap[fromID]; ok {
					if toNode, ok := nodeMap[toID]; ok {
						graph.AddEdge(NewEdge(fromNode.ID, toNode.ID))
					}
				}
			}
		}
	}

	return graph, nil
}

func (e *JavaExtractor) parseMavenLabel(label string) *DepNode {
	// Format: groupId:artifactId:type:version or groupId:artifactId:version
	parts := strings.Split(label, ":")
	if len(parts) >= 3 {
		groupId := parts[0]
		artifactId := parts[1]
		version := parts[len(parts)-1]
		name := groupId + ":" + artifactId
		return NewDepNode("java", name, version)
	}
	return nil
}

func (e *JavaExtractor) extractGradle(dir string) (*DepGraph, error) {
	buildGradle := filepath.Join(dir, "build.gradle")
	buildGradleKts := filepath.Join(dir, "build.gradle.kts")
	if _, err := os.Stat(buildGradle); err != nil {
		if _, err := os.Stat(buildGradleKts); err != nil {
			return nil, err
		}
	}

	// Run gradle dependencies
	cmd := exec.Command("gradle", "dependencies", "--console=plain")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return e.parseGradleOutput(string(output))
}

func (e *JavaExtractor) parseGradleOutput(output string) (*DepGraph, error) {
	graph := NewGraph()
	lines := strings.Split(output, "\n")
	var currentPath []string
	nodeMap := make(map[string]*DepNode)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ">") {
			continue
		}

		// Parse dependency line (indentation indicates depth)
		depth := 0
		for i, r := range line {
			if r != ' ' && r != '+' && r != '|' && r != '\\' {
				depth = i
				break
			}
		}

		// Extract dependency info
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			depSpec := parts[len(parts)-1]
			// Format: group:artifact:version
			node := e.parseMavenLabel(depSpec)
			if node != nil {
				graph.AddNode(node)
				nodeMap[depSpec] = node

				// Build dependency tree
				if depth > 0 && len(currentPath) >= depth {
					parentSpec := currentPath[depth-1]
					if parentNode, ok := nodeMap[parentSpec]; ok {
						graph.AddEdge(NewEdge(parentNode.ID, node.ID))
					}
				}

				// Update current path
				if depth < len(currentPath) {
					currentPath = currentPath[:depth]
				}
				currentPath = append(currentPath, depSpec)
			}
		}
	}

	return graph, nil
}
