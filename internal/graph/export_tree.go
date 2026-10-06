package graph

import (
	"fmt"
	"io"
	"strings"
)

// PrintTree prints the dependency tree starting from root nodes.
func PrintTree(graph *DepGraph, w io.Writer, showVersions, showEcosystem bool, maxDepth int) {
	if len(graph.Root) == 0 {
		fmt.Fprintln(w, "No root packages found.")
		return
	}

	visited := make(map[string]bool)
	for _, rootID := range graph.Root {
		if rootNode := graph.GetNode(rootID); rootNode != nil {
			printTreeNode(graph, rootNode, w, "", true, visited, showVersions, showEcosystem, maxDepth, 0)
		}
	}
}

// printTreeNode prints a single node and its children recursively.
func printTreeNode(graph *DepGraph, node *DepNode, w io.Writer, prefix string, isLast bool, visited map[string]bool, showVersions, showEcosystem bool, maxDepth, currentDepth int) {
	if currentDepth >= maxDepth && maxDepth > 0 {
		return
	}

	// Format node label
	label := formatNodeLabel(node, showVersions, showEcosystem)

	// Print node
	if prefix == "" {
		// Root node
		fmt.Fprintf(w, "%s\n", label)
	} else {
		connector := "├─ "
		if isLast {
			connector = "└─ "
		}
		fmt.Fprintf(w, "%s%s%s\n", prefix, connector, label)
	}

	// Mark as visited to avoid cycles
	visited[node.ID] = true

	// Get children
	children := graph.GetChildren(node.ID)
	if len(children) == 0 {
		return
	}

	// Filter out already visited nodes to avoid cycles
	var unvisitedChildren []string
	for _, childID := range children {
		if !visited[childID] {
			unvisitedChildren = append(unvisitedChildren, childID)
		}
	}

	if len(unvisitedChildren) == 0 {
		return
	}

	// Print children
	for i, childID := range unvisitedChildren {
		childNode := graph.GetNode(childID)
		if childNode == nil {
			continue
		}

		isChildLast := i == len(unvisitedChildren)-1
		childPrefix := prefix
		if prefix != "" {
			if isLast {
				childPrefix += "   "
			} else {
				childPrefix += "│  "
			}
		}

		printTreeNode(graph, childNode, w, childPrefix, isChildLast, visited, showVersions, showEcosystem, maxDepth, currentDepth+1)
	}

	// Unmark for other paths
	delete(visited, node.ID)
}

// formatNodeLabel formats a node for display.
func formatNodeLabel(node *DepNode, showVersions, showEcosystem bool) string {
	var parts []string

	if showVersions && node.Version != "" {
		parts = append(parts, fmt.Sprintf("%s@%s", node.Name, node.Version))
	} else {
		parts = append(parts, node.Name)
	}

	if showEcosystem {
		parts = append(parts, fmt.Sprintf("(%s)", node.Ecosystem))
	}

	return strings.Join(parts, " ")
}

// PrintTreeForPackage prints the dependency tree for a specific package.
func PrintTreeForPackage(graph *DepGraph, packageName string, w io.Writer, showVersions, showEcosystem bool, maxDepth int) {
	nodes := graph.FindNodeByName(packageName)
	if len(nodes) == 0 {
		fmt.Fprintf(w, "Package %s not found.\n", packageName)
		return
	}

	// Print tree for each matching node
	for _, node := range nodes {
		visited := make(map[string]bool)
		printTreeNode(graph, node, w, "", true, visited, showVersions, showEcosystem, maxDepth, 0)
		if len(nodes) > 1 {
			fmt.Fprintln(w)
		}
	}
}
