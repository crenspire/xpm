package graph

import (
	"fmt"
	"io"
	"sort"
)

// Warning represents a dependency warning.
type Warning struct {
	Type    string
	Message string
	Package string
	Details []string
}

// DetectWarnings detects various issues in the dependency graph.
func DetectWarnings(graph *DepGraph) []Warning {
	var warnings []Warning

	warnings = append(warnings, DetectVersionConflicts(graph)...)
	warnings = append(warnings, DetectEcosystemConflicts(graph)...)
	warnings = append(warnings, DetectMissingDependencies(graph)...)

	return warnings
}

// DetectVersionConflicts finds multiple versions of the same package.
func DetectVersionConflicts(graph *DepGraph) []Warning {
	var warnings []Warning

	// Group nodes by ecosystem and name
	byEcosystemAndName := make(map[string]map[string][]*DepNode)
	for _, node := range graph.Nodes {
		key := node.Ecosystem
		if byEcosystemAndName[key] == nil {
			byEcosystemAndName[key] = make(map[string][]*DepNode)
		}
		byEcosystemAndName[key][node.Name] = append(byEcosystemAndName[key][node.Name], node)
	}

	// Check for multiple versions
	for ecosystem, byName := range byEcosystemAndName {
		for name, nodes := range byName {
			if len(nodes) > 1 {
				versions := make([]string, len(nodes))
				for i, node := range nodes {
					versions[i] = node.Version
				}
				sort.Strings(versions)

				warnings = append(warnings, Warning{
					Type:    "version_conflict",
					Message: fmt.Sprintf("Multiple versions of %s detected in %s", name, ecosystem),
					Package: name,
					Details: versions,
				})
			}
		}
	}

	return warnings
}

// DetectEcosystemConflicts finds the same package in different ecosystems.
func DetectEcosystemConflicts(graph *DepGraph) []Warning {
	var warnings []Warning

	// Group nodes by name
	byName := make(map[string][]*DepNode)
	for _, node := range graph.Nodes {
		byName[node.Name] = append(byName[node.Name], node)
	}

	// Check for same name in different ecosystems
	for name, nodes := range byName {
		if len(nodes) <= 1 {
			continue
		}

		ecosystems := make(map[string]bool)
		for _, node := range nodes {
			ecosystems[node.Ecosystem] = true
		}

		if len(ecosystems) > 1 {
			ecoList := make([]string, 0, len(ecosystems))
			for eco := range ecosystems {
				ecoList = append(ecoList, eco)
			}
			sort.Strings(ecoList)

			warnings = append(warnings, Warning{
				Type:    "ecosystem_conflict",
				Message: fmt.Sprintf("Package %s found in multiple ecosystems", name),
				Package: name,
				Details: ecoList,
			})
		}
	}

	return warnings
}

// DetectMissingDependencies finds potential missing dependencies.
func DetectMissingDependencies(graph *DepGraph) []Warning {
	var warnings []Warning

	// Check for edges pointing to non-existent nodes
	for _, edge := range graph.Edges {
		if graph.GetNode(edge.To) == nil {
			warnings = append(warnings, Warning{
				Type:    "missing_dependency",
				Message: fmt.Sprintf("Dependency %s referenced but not found", edge.To),
				Package: edge.From,
				Details: []string{edge.To},
			})
		}
		if graph.GetNode(edge.From) == nil {
			warnings = append(warnings, Warning{
				Type:    "missing_dependency",
				Message: fmt.Sprintf("Source node %s not found", edge.From),
				Package: edge.To,
				Details: []string{edge.From},
			})
		}
	}

	return warnings
}

// PrintWarnings prints warnings to the writer.
func PrintWarnings(warnings []Warning, w io.Writer) {
	if len(warnings) == 0 {
		return
	}

	fmt.Fprintf(w, "\n⚠ Warnings:\n\n")

	for _, warning := range warnings {
		fmt.Fprintf(w, "  ⚠ %s\n", warning.Message)
		if warning.Package != "" {
			fmt.Fprintf(w, "     Package: %s\n", warning.Package)
		}
		if len(warning.Details) > 0 {
			fmt.Fprintf(w, "     Details: %v\n", warning.Details)
		}
		fmt.Fprintln(w)
	}
}

